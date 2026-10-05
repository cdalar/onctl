package cloud

import (
	"context"
	"fmt"
	"log"
	"net"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/cdalar/onctl/internal/providerboxes"
	"github.com/cdalar/onctl/internal/tools"
)

// BoxesConfig is the boxes provider's settings (onctl.yaml's boxes:).
type BoxesConfig struct {
	// VMType is the default size (small/medium/large); empty leaves it to
	// the service.
	VMType string
	// Image is the default boot image, from `onctl images -p boxes`.
	Image    string
	Username string
	// OnctlBin is this onctl's own path, for the ssh ProxyCommand.
	OnctlBin string
}

// ProviderBoxes is the hosted boxes service (boxctl.io): Firecracker
// microVMs run for you, reached only through the service -- a box has no
// address of its own you can connect to, so ssh goes through the
// service's tunnel (DialVM, and `onctl ssh-proxy` for the system ssh).
type ProviderBoxes struct {
	Client *providerboxes.Client
	Config BoxesConfig
}

var (
	_ CloudProviderInterface = (*ProviderBoxes)(nil)
	_ Dialer                 = (*ProviderBoxes)(nil)
	_ ImageLister            = (*ProviderBoxes)(nil)
	_ KeyAuthorizer          = (*ProviderBoxes)(nil)
)

// AuthorizeKey makes sure the box is running and publicKey can log in.
func (p *ProviderBoxes) AuthorizeKey(vm Vm, publicKey string) error {
	ctx := context.Background()
	if err := p.ensureRunning(ctx, vm.Name); err != nil {
		return err
	}
	return p.authorizeKey(ctx, vm.Name, publicKey)
}

const (
	// boxesReadyTimeout bounds waiting for a created or resumed box.
	boxesReadyTimeout = 3 * time.Minute
	// boxesExecTimeout bounds the one-line commands this provider runs on
	// a box itself (authorizing a key).
	boxesExecTimeout  = 30 * time.Second
	boxesDefaultImage = "debian-slim"
)

func mapBoxesVM(b providerboxes.VM) Vm {
	return Vm{
		ID:   b.Name,
		Name: b.Name,
		// A box has no public address: its IP is private to its host, and
		// it's reached through the service (DialVM, ssh-proxy).
		IP:        "N/A",
		PrivateIP: b.IP,
		Type:      b.Size,
		Image:     b.Image,
		Status:    b.State,
		SSHReady:  b.State == "running" && b.Ready,
		Location:  "boxes",
		SSHPort:   22,
		CreatedAt: b.CreatedAt,
		Provider:  "boxes",
	}
}

// Deploy creates a box, waits for it to boot, and authorizes the ssh key
// (server.SSHKeyID, which CreateSSHKey set to the public key itself) --
// the service takes no key at create, so it's added over exec, the one
// way in that needs none.
func (p *ProviderBoxes) Deploy(server Vm) (Vm, error) {
	ctx := context.Background()
	size := server.Type
	if size == "" {
		size = p.Config.VMType
	}
	image := server.Image
	if image == "" {
		image = p.Config.Image
	}
	if image == "" {
		image = boxesDefaultImage
	}
	if _, err := p.Client.Create(ctx, server.Name, image, size); err != nil {
		return Vm{}, fmt.Errorf("creating box %s: %w", server.Name, err)
	}
	box, err := p.Client.WaitReady(ctx, server.Name, boxesReadyTimeout)
	if err != nil {
		return Vm{}, err
	}
	if server.SSHKeyID != "" {
		if err := p.authorizeKey(ctx, server.Name, server.SSHKeyID); err != nil {
			return Vm{}, err
		}
	}
	return mapBoxesVM(*box), nil
}

// authorizeKey appends publicKey to root's authorized_keys on the box,
// once.
func (p *ProviderBoxes) authorizeKey(ctx context.Context, name, publicKey string) error {
	publicKey = strings.TrimSpace(publicKey)
	if publicKey == "" || strings.ContainsAny(publicKey, "'\n") {
		return fmt.Errorf("refusing to authorize a malformed public key on %s", name)
	}
	cmd := "set -e; umask 077; mkdir -p ~/.ssh; touch ~/.ssh/authorized_keys; " +
		"grep -qxF '" + publicKey + "' ~/.ssh/authorized_keys || printf '%s\\n' '" + publicKey + "' >> ~/.ssh/authorized_keys"
	res, err := p.Client.Exec(ctx, name, cmd, boxesExecTimeout)
	if err != nil {
		return fmt.Errorf("authorizing your ssh key on %s: %w", name, err)
	}
	if res.Error != "" || res.ExitCode != 0 {
		return fmt.Errorf("authorizing your ssh key on %s: exit %d %s%s", name, res.ExitCode, res.Error, strings.TrimSpace(res.Stderr))
	}
	return nil
}

func (p *ProviderBoxes) Destroy(server Vm) error {
	return p.Client.Destroy(context.Background(), server.Name)
}

// Pause snapshots the box's memory; hot doesn't apply.
func (p *ProviderBoxes) Pause(server Vm, hot bool) error {
	_, err := p.Client.Pause(context.Background(), server.Name)
	return err
}

func (p *ProviderBoxes) Resume(server Vm) (Vm, error) {
	ctx := context.Background()
	if _, err := p.Client.Resume(ctx, server.Name); err != nil {
		return Vm{}, err
	}
	box, err := p.Client.WaitReady(ctx, server.Name, boxesReadyTimeout)
	if err != nil {
		return Vm{}, err
	}
	return mapBoxesVM(*box), nil
}

// List returns every box the token's owner has, paused ones included.
func (p *ProviderBoxes) List() (VmList, error) {
	boxes, err := p.Client.List(context.Background())
	if err != nil {
		return VmList{}, err
	}
	list := VmList{}
	for _, b := range boxes {
		list.List = append(list.List, mapBoxesVM(b))
	}
	// The service returns boxes in no particular order; list them oldest
	// first, as the cloud providers' APIs do.
	sort.SliceStable(list.List, func(i, j int) bool {
		a, b := list.List[i], list.List[j]
		if !a.CreatedAt.Equal(b.CreatedAt) {
			return a.CreatedAt.Before(b.CreatedAt)
		}
		return a.Name < b.Name
	})
	return list, nil
}

// ListPaused is empty: List already includes paused boxes.
func (p *ProviderBoxes) ListPaused() (VmList, error) { return VmList{}, nil }

func (p *ProviderBoxes) GetByName(serverName string) (Vm, error) {
	list, err := p.List()
	if err != nil {
		return Vm{}, err
	}
	for _, vm := range list.List {
		if vm.Name == serverName {
			return vm, nil
		}
	}
	return Vm{}, fmt.Errorf("no box named %s", serverName)
}

// CreateSSHKey has no registry to talk to: the "key ID" is the public key
// itself, which Deploy authorizes on the box.
func (p *ProviderBoxes) CreateSSHKey(publicKeyFile string) (string, error) {
	data, err := os.ReadFile(publicKeyFile)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(data)), nil
}

// SSHInto runs the system ssh with `onctl ssh-proxy` as its
// ProxyCommand. The key (privateKey's .pub) is authorized first, so a box
// made elsewhere -- the dashboard -- works too.
func (p *ProviderBoxes) SSHInto(serverName string, port int, privateKey string, command []string) {
	if port == 0 {
		port = 22
	}
	if pub, err := os.ReadFile(privateKey + ".pub"); err == nil {
		ctx := context.Background()
		if err := p.ensureRunning(ctx, serverName); err != nil {
			log.Fatalln(err)
		}
		if err := p.authorizeKey(ctx, serverName, string(pub)); err != nil {
			log.Fatalln(err)
		}
	} else {
		log.Println("[DEBUG] no public key beside", privateKey, "- not authorizing it:", err)
	}
	username := p.Config.Username
	if username == "" {
		username = "root"
	}
	tools.SSHIntoVM(tools.SSHIntoVMRequest{
		IPAddress:      serverName,
		User:           username,
		Port:           port,
		PrivateKeyFile: privateKey,
		Command:        command,
		ProxyCommand:   p.proxyCommand(serverName, port),
	})
}

// proxyCommand is the ssh ProxyCommand reaching name's port.
func (p *ProviderBoxes) proxyCommand(name string, port int) string {
	bin := p.Config.OnctlBin
	if bin == "" {
		bin = "onctl"
	}
	return fmt.Sprintf("%s ssh-proxy -p boxes %s %d", shellQuote(bin), shellQuote(name), port)
}

// shellQuote quotes s for the shell ssh runs ProxyCommand with.
func shellQuote(s string) string {
	if s != "" && strings.Trim(s, "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789-_./") == "" {
		return s
	}
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

// DialVM opens a connection to port inside the box through the service's
// tunnel, resuming the box first if it's paused.
func (p *ProviderBoxes) DialVM(ctx context.Context, vm Vm, port int) (net.Conn, error) {
	if err := p.ensureRunning(ctx, vm.Name); err != nil {
		return nil, err
	}
	conn, err := p.Client.DialPort(ctx, vm.Name, port)
	if err != nil {
		return nil, fmt.Errorf("connecting to %s:%d: %w", vm.Name, port, err)
	}
	return conn, nil
}

// ensureRunning resumes name if it's paused and waits until it's ready.
func (p *ProviderBoxes) ensureRunning(ctx context.Context, name string) error {
	boxes, err := p.Client.List(ctx)
	if err != nil {
		return err
	}
	for _, b := range boxes {
		if b.Name != name {
			continue
		}
		switch b.State {
		case "running":
			if b.Ready {
				return nil
			}
		case "paused":
			fmt.Fprintf(os.Stderr, "Resuming %s...\n", name)
			if _, err := p.Client.Resume(ctx, name); err != nil {
				return fmt.Errorf("resuming %s: %w", name, err)
			}
		default:
			return fmt.Errorf("%s is %s", name, b.State)
		}
		_, err := p.Client.WaitReady(ctx, name, boxesReadyTimeout)
		return err
	}
	return fmt.Errorf("no box named %s", name)
}

// ListImages is `onctl images -p boxes`.
func (p *ProviderBoxes) ListImages() ([]CloudImage, error) {
	images, err := p.Client.ListImages(context.Background())
	if err != nil {
		return nil, err
	}
	var out []CloudImage
	for _, img := range images {
		out = append(out, CloudImage{Name: img.Name, Description: img.Description})
	}
	return out, nil
}

var _ SizeLister = (*ProviderBoxes)(nil)

// ListSizes is `onctl sizes -p boxes`.
func (p *ProviderBoxes) ListSizes() ([]CloudSize, error) {
	sizes, err := p.Client.ListSizes(context.Background())
	if err != nil {
		return nil, err
	}
	out := make([]CloudSize, 0, len(sizes))
	for _, s := range sizes {
		out = append(out, CloudSize{
			Name:      s.Name,
			VCPU:      s.VCPU,
			MemoryMiB: s.MemMiB,
			DiskGiB:   s.DiskMiB / 1024,
			Default:   s.Default,
		})
	}
	return out, nil
}
