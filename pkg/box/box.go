// Package box is onctl as a library: what a provider does to a box (a VM,
// microVM or container) -- create, list, destroy, pause, resume and ssh --
// for a provider picked by its ID, as `onctl -p <id>` does. The onctl CLI
// is one caller of it; other programs (the boxctl host service) are meant
// to be others, without running onctl and parsing its output.
//
// The provider adapters are in internal/ and pkg/cloud; this package only
// picks and builds one, and holds the logic the CLI used to keep to itself
// (what counts as paused, deleting a paused VM's snapshot). VMs are
// cloud.Vm values.
package box

import (
	"errors"
	"fmt"
	"log"
	"os"
	"os/exec"
	"strings"

	"github.com/cdalar/onctl/internal/providerac"
	"github.com/cdalar/onctl/internal/provideraws"
	"github.com/cdalar/onctl/internal/providerazure"
	"github.com/cdalar/onctl/internal/providerboxes"
	"github.com/cdalar/onctl/internal/providerch"
	"github.com/cdalar/onctl/internal/providerfc"
	"github.com/cdalar/onctl/internal/providergcp"
	"github.com/cdalar/onctl/internal/providerhtz"
	"github.com/cdalar/onctl/internal/provideropenshell"
	"github.com/cdalar/onctl/internal/providerovh"
	"github.com/cdalar/onctl/internal/tools"
	"github.com/cdalar/onctl/pkg/cloud"

	"github.com/spf13/viper"
)

// Providers are the provider IDs New accepts.
var Providers = []string{"aws", "hetzner", "azure", "gcp", "ovh", "fc", "ch", "ac", "static", "boxes", "openshell"}

// ErrExists is what Create returns when a live VM already has the name.
var ErrExists = errors.New("a VM with this name exists")

// Options picks and configures a provider for New.
type Options struct {
	// Provider is one of Providers.
	Provider string
	// Settings are onctl.yaml values keyed by their dotted path, e.g.
	// "fc.stateDir" or "hetzner.vm.type"; see internal/files/init/onctl.yaml
	// for the keys. They override what's already loaded. The adapters read
	// onctl's process-wide configuration, so Settings are applied to it:
	// one process gets one set of Settings, whatever the number of Clients.
	// Credentials are read as the CLI reads them (HCLOUD_TOKEN, the
	// cloud CLIs' own config, `onctl login` for boxes).
	Settings map[string]any
	// OnctlBin is the onctl binary the boxes and openshell providers put in
	// the ssh ProxyCommand. Empty means this executable, which is right
	// only when the caller is onctl itself.
	OnctlBin string
}

// Client does what a provider does to boxes.
type Client struct {
	id string
	p  cloud.CloudProviderInterface
}

// New builds the provider opts.Provider names. A provider whose
// credentials are missing (HCLOUD_TOKEN for hetzner, the SDK's credential
// chain for gcp, azure and ovh) fails it with an error.
func New(opts Options) (*Client, error) {
	if !tools.Contains(Providers, opts.Provider) {
		return nil, fmt.Errorf("unsupported provider %q; use one of: %s", opts.Provider, strings.Join(Providers, ", "))
	}
	for k, v := range opts.Settings {
		viper.Set(k, v)
	}
	bin := opts.OnctlBin
	if bin == "" {
		var err error
		if bin, err = os.Executable(); err != nil {
			bin = "onctl"
		}
	}
	p, err := build(opts.Provider, bin)
	if err != nil {
		return nil, err
	}
	return &Client{id: opts.Provider, p: p}, nil
}

// Wrap makes a Client of a provider built elsewhere: a test's fake, or the
// CLI's, which other commands share.
func Wrap(id string, p cloud.CloudProviderInterface) *Client {
	return &Client{id: id, p: p}
}

// ID is the provider's ID.
func (c *Client) ID() string { return c.id }

// Provider is the provider itself, for what this package doesn't cover
// (images, sizes, restart, dialing a VM).
func (c *Client) Provider() cloud.CloudProviderInterface { return c.p }

// Exists reports whether a live VM is named name. A dead fc or ch microVM
// (its VMM process gone, e.g. after a host reboot) doesn't count: Create
// recreates it.
func (c *Client) Exists(name string) (bool, error) {
	list, err := c.p.List()
	for _, vm := range list.List {
		if (vm.Provider == "fc" || vm.Provider == "ch") && vm.Status == "dead" {
			continue
		}
		if vm.Name == name {
			return true, err
		}
	}
	return false, err
}

// AddSSHKey puts the public key in publicKeyFile on the provider (where it
// keeps keys) and returns the ID to set as Vm.SSHKeyID for Create and
// Resume.
func (c *Client) AddSSHKey(publicKeyFile string) (keyID string, err error) {
	return c.p.CreateSSHKey(publicKeyFile)
}

// Create deploys vm and returns it as the provider made it. An empty
// vm.Name gets a generated one. A live VM of that name fails it with
// ErrExists; one that can't be listed doesn't stop it, as a provider
// whose List fails can still deploy. It returns once the provider has
// the VM, not when ssh or cloud-init are ready.
func (c *Client) Create(vm cloud.Vm) (cloud.Vm, error) {
	if vm.Name == "" {
		vm.Name = tools.GenerateMachineUniqueName()
	}
	exists, err := c.Exists(vm.Name)
	if err != nil {
		log.Println("[DEBUG] listing VMs before create:", err)
	}
	if exists {
		return cloud.Vm{}, fmt.Errorf("%w: %s", ErrExists, vm.Name)
	}
	return c.p.Deploy(vm)
}

// List is the provider's VMs split into running and paused: ListPaused's,
// plus the stopped VMs List returned, so every provider shows the split.
// A failed call is reported through err; what the other call returned is
// still handed back.
func (c *Client) List() (running, paused cloud.VmList, err error) {
	all, listErr := c.p.List()
	log.Println("[DEBUG] VM List: ", all)
	snapshots, pausedErr := c.p.ListPaused()
	log.Println("[DEBUG] Paused List: ", snapshots)
	live := map[string]bool{}
	for _, vm := range all.List {
		live[vm.Name] = true
		if PausedStatus(vm.Status) {
			paused.List = append(paused.List, vm)
		} else {
			running.List = append(running.List, vm)
		}
	}
	// Mid pause/resume the server and its snapshot coexist (Hetzner
	// snapshots before deleting, and creates before deleting the
	// snapshot); the server is the current record, so skip its snapshot.
	for _, vm := range snapshots.List {
		if !live[vm.Name] {
			paused.List = append(paused.List, vm)
		}
	}
	return running, paused, errors.Join(listErr, pausedErr)
}

// PausedStatus reports whether a VM status means stopped/paused rather
// than running. AWS uses "stopped", Hetzner "off" (a server powered off
// outside onctl pause, which snapshots and deletes it), GCP uses
// "TERMINATED" (stopped, not deleted), Azure uses "VM deallocated", OVH
// uses "SHELVED"/"SHELVED_OFFLOADED".
func PausedStatus(status string) bool {
	return status == "stopped" ||
		status == "off" ||
		status == "TERMINATED" ||
		status == "SHELVED" ||
		status == "SHELVED_OFFLOADED" ||
		strings.Contains(strings.ToLower(status), "deallocated")
}

// Destroy deletes the VM named name, for good. A paused VM that's only a
// snapshot (Hetzner's pause deletes the server) has its snapshot deleted,
// so it can never be resumed.
func (c *Client) Destroy(name string) error {
	if sd, ok := c.p.(cloud.SnapshotDestroyer); ok && c.isSnapshot(name) {
		return sd.DestroySnapshot(name)
	}
	return c.p.Destroy(cloud.Vm{Name: name})
}

// isSnapshot tells whether name is among the provider's paused VMs.
func (c *Client) isSnapshot(name string) bool {
	paused, err := c.p.ListPaused()
	if err != nil {
		log.Println("[DEBUG] listing paused VMs: ", err)
		return false
	}
	for _, vm := range paused.List {
		if vm.Name == name {
			return true
		}
	}
	return false
}

// Pause stops the VM named name so it no longer costs compute; see
// cloud.CloudProviderInterface.Pause for hot.
func (c *Client) Pause(name string, hot bool) error {
	return c.p.Pause(cloud.Vm{Name: name}, hot)
}

// Resume brings the paused VM named name back, with the key AddSSHKey
// returned, and returns it.
func (c *Client) Resume(name, keyID string) (cloud.Vm, error) {
	return c.p.Resume(cloud.Vm{Name: name, SSHKeyID: keyID})
}

// SSH runs command on the VM named name over ssh, or opens a shell when
// command is empty, wired to stdio (nil fields are this process's own
// streams; pass buffers to capture the output). port 0 and an empty
// privateKeyFile mean the provider's own (imported hosts').
//
// exitCode is the command's (or the shell's) exit status; ssh exits 255
// when it can't connect. err is for an ssh that couldn't run at all: the
// VM can't be found, isn't running, or there's no ssh binary.
func (c *Client) SSH(name string, port int, privateKeyFile string, command []string, stdio cloud.Stdio) (exitCode int, err error) {
	err = c.p.SSHInto(name, port, privateKeyFile, command, stdio)
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		return exitErr.ExitCode(), nil
	}
	return 0, err
}

// build is the provider id names, configured from onctl's configuration.
func build(id, onctlBin string) (cloud.CloudProviderInterface, error) {
	switch id {
	case "hetzner":
		client, err := providerhtz.GetClient()
		if err != nil {
			return nil, err
		}
		return &cloud.ProviderHetzner{
			Client: client,
			Config: cloud.HetznerConfig{
				Location:      viper.GetString("hetzner.location"),
				VMType:        viper.GetString("hetzner.vm.type"),
				Image:         viper.GetString("hetzner.vm.image"),
				Username:      viper.GetString("hetzner.vm.username"),
				SSHPrivateKey: viper.GetString("ssh.privateKey"),
			},
		}, nil
	case "gcp":
		client, err := providergcp.GetClient()
		if err != nil {
			return nil, err
		}
		groupClient, err := providergcp.GetGroupClient()
		if err != nil {
			return nil, err
		}
		return &cloud.ProviderGcp{
			Client:      client,
			GroupClient: groupClient,
		}, nil
	case "aws":
		return &cloud.ProviderAws{
			Client: provideraws.GetClient(),
		}, nil
	case "azure":
		p := &cloud.ProviderAzure{}
		var errs [7]error
		p.ResourceGraphClient, errs[0] = providerazure.GetResourceGraphClient()
		p.VmClient, errs[1] = providerazure.GetVmClient()
		p.NicClient, errs[2] = providerazure.GetNicClient()
		p.PublicIPClient, errs[3] = providerazure.GetIPClient()
		p.SSHKeyClient, errs[4] = providerazure.GetSSHKeyClient()
		p.VnetClient, errs[5] = providerazure.GetVnetClient()
		p.NSGClient, errs[6] = providerazure.GetNSGClient()
		if err := errors.Join(errs[:]...); err != nil {
			return nil, err
		}
		return p, nil
	case "ovh":
		client, err := providerovh.GetClient()
		if err != nil {
			return nil, err
		}
		return &cloud.ProviderOvh{
			Client: client,
			Config: cloud.OvhConfig{
				ServiceName:   viper.GetString("ovh.serviceName"),
				Region:        viper.GetString("ovh.location"),
				VMType:        viper.GetString("ovh.vm.type"),
				Image:         viper.GetString("ovh.vm.image"),
				Username:      viper.GetString("ovh.vm.username"),
				SSHPrivateKey: viper.GetString("ssh.privateKey"),
			},
		}, nil
	case "fc":
		fcConfig := providerfc.GetConfig()
		return &cloud.ProviderFC{
			Config:  fcConfig,
			Process: providerfc.NewProcessManager(fcConfig.BinPath),
			API:     providerfc.NewAPIClient(),
			Net:     providerfc.NewNetworkManager(),
			Rootfs:  providerfc.NewRootfsPreparer(),
			Cache:   providerfc.NewCacheDiskPreparer(),
		}, nil
	case "ch":
		chConfig := providerch.GetConfig()
		return &cloud.ProviderCH{
			Config:       chConfig,
			Process:      providerch.NewProcessManager(chConfig.BinPath),
			Net:          providerch.NewNetworkManager(),
			Rootfs:       providerch.NewRootfsPreparer(),
			WindowsGuest: providerch.NewWindowsGuestPreparer(),
			DHCP:         providerch.NewDHCPManager(chConfig.StateDir),
		}, nil
	case "ac":
		acConfig := providerac.GetConfig()
		return &cloud.ProviderAC{
			Config: acConfig,
			CLI:    providerac.NewRunner(acConfig.BinPath),
		}, nil
	case "boxes":
		creds, err := providerboxes.LoadCredentials(viper.GetString("boxes.apiURL"))
		if err != nil {
			return nil, err
		}
		return &cloud.ProviderBoxes{
			Client: providerboxes.New(creds.APIURL, creds.Token),
			Config: cloud.BoxesConfig{
				VMType:   viper.GetString("boxes.vm.type"),
				Image:    viper.GetString("boxes.vm.image"),
				Username: viper.GetString("boxes.vm.username"),
				OnctlBin: onctlBin,
			},
		}, nil
	case "openshell":
		gw, err := provideropenshell.Connect(viper.GetString("openshell.gateway"))
		if err != nil {
			return nil, err
		}
		// The relay logs in as the sandbox's own user whatever is asked
		// for; the ssh client just needs a name to send.
		viper.SetDefault("openshell.vm.username", "sandbox")
		// Without an onctl.yaml there's no key path at all; onctl.yaml's
		// own default lets the CLI fall back to whichever key type
		// exists. The relay doesn't check the key, but the ssh client
		// needs one to offer.
		viper.SetDefault("ssh.publicKey", "~/.ssh/id_rsa.pub")
		viper.SetDefault("ssh.privateKey", "~/.ssh/id_rsa")
		return &cloud.ProviderOpenShell{
			Sandboxes: gw.Client.Sandboxes(),
			SSH:       gw.Client.SSH(),
			TCP:       gw.Client.TCP(),
			Config: cloud.OpenShellConfig{
				Gateway:   gw.Name,
				Workspace: viper.GetString("openshell.workspace"),
				Image:     viper.GetString("openshell.vm.image"),
				Username:  viper.GetString("openshell.vm.username"),
				OnctlBin:  onctlBin,
			},
		}, nil
	case "static":
		path, err := tools.OnctlSSHConfigPath()
		if err != nil {
			return nil, err
		}
		return &cloud.ProviderStatic{InventoryPath: path}, nil
	}
	return nil, fmt.Errorf("unsupported provider %q", id)
}
