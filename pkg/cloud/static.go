package cloud

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/cdalar/onctl/internal/tools"
)

// StaticHost is one entry in the imported-hosts inventory: a server onctl
// did not create and has no lifecycle API for, registered via `onctl import`
// so ssh/ls can reach it.
type StaticHost struct {
	Name       string
	IP         string
	Username   string
	SSHPort    int
	PrivateKey string
	ImportedAt time.Time
}

type StaticInventory struct {
	Hosts []StaticHost
}

// onctlImportedAtDirective is the comment onctl writes below each Host block
// to remember when it was imported, since ssh_config has no native field for
// it. It is a plain comment, so the file stays valid ssh_config and hosts in
// it are reachable with plain `ssh <name>` once ~/.ssh/config includes it.
const onctlImportedAtDirective = "# onctl-imported-at"

// SplitSSHConfigFields tokenizes one ssh_config line into whitespace-separated
// fields, treating a double-quoted field as one token even if it contains
// whitespace (e.g. `IdentityFile "/path with spaces/key"`). Needed because a
// plain strings.Fields split would break IdentityFile/HostName values (paths,
// usernames) that contain spaces, both here and when writing them back out.
func SplitSSHConfigFields(line string) []string {
	var fields []string
	var cur strings.Builder
	inQuotes := false
	for i := 0; i < len(line); i++ {
		c := line[i]
		switch {
		case c == '"':
			inQuotes = !inQuotes
		case (c == ' ' || c == '\t') && !inQuotes:
			if cur.Len() > 0 {
				fields = append(fields, cur.String())
				cur.Reset()
			}
		default:
			cur.WriteByte(c)
		}
	}
	if cur.Len() > 0 {
		fields = append(fields, cur.String())
	}
	return fields
}

// QuoteSSHConfigValue wraps v in double quotes if it contains whitespace, so it
// round-trips through SplitSSHConfigFields as a single field.
func QuoteSSHConfigValue(v string) string {
	if strings.ContainsAny(v, " \t") {
		return `"` + v + `"`
	}
	return v
}

// ProviderStatic implements CloudProviderInterface over a local inventory
// file instead of a cloud API, for servers imported with `onctl import`
// (e.g. Hetzner auction/dedicated boxes, or any other unmanaged host).
type ProviderStatic struct {
	InventoryPath string
}

var errStaticUnsupported = errors.New("not supported for imported hosts; use 'onctl import' to add one, or manage the underlying machine directly")

// staticInventoryMu serializes load-modify-save of the inventory file so
// concurrent destroys (e.g. `destroy all` launches one goroutine per VM)
// don't race and clobber each other's writes.
var staticInventoryMu sync.Mutex

// LoadInventory parses p.InventoryPath as an ssh_config file: one Host block
// per imported host (HostName/User/Port/IdentityFile), so the same file can
// be `Include`d from ~/.ssh/config and used with plain `ssh <name>` too.
// Directives onctl doesn't recognize are ignored, so hand-added ones don't
// break parsing but are also dropped on the next SaveInventory rewrite.
func (p ProviderStatic) LoadInventory() (StaticInventory, error) {
	var inv StaticInventory
	data, err := os.ReadFile(p.InventoryPath)
	if errors.Is(err, os.ErrNotExist) {
		return inv, nil
	}
	if err != nil {
		return inv, fmt.Errorf("failed to read %s: %w", p.InventoryPath, err)
	}

	var current *StaticHost
	flush := func() {
		if current != nil {
			inv.Hosts = append(inv.Hosts, *current)
			current = nil
		}
	}
	for _, line := range strings.Split(string(data), "\n") {
		fields := SplitSSHConfigFields(line)
		if len(fields) == 0 {
			continue
		}
		switch strings.ToLower(fields[0]) {
		case "host":
			flush()
			current = &StaticHost{Name: strings.Join(fields[1:], " ")}
		case "hostname":
			if current != nil && len(fields) > 1 {
				current.IP = fields[1]
			}
		case "user":
			if current != nil && len(fields) > 1 {
				current.Username = fields[1]
			}
		case "port":
			if current != nil && len(fields) > 1 {
				if port, err := strconv.Atoi(fields[1]); err == nil {
					current.SSHPort = port
				}
			}
		case "identityfile":
			if current != nil && len(fields) > 1 {
				current.PrivateKey = fields[1]
			}
		case "#":
			if current != nil && len(fields) > 2 && fields[1] == onctlImportedAtDirective[2:] {
				if t, err := time.Parse(time.RFC3339, fields[2]); err == nil {
					current.ImportedAt = t
				}
			}
		}
	}
	flush()
	return inv, nil
}

// SaveInventory rewrites p.InventoryPath from scratch as an ssh_config file.
func (p ProviderStatic) SaveInventory(inv StaticInventory) error {
	var b strings.Builder
	b.WriteString("# Managed by onctl (`onctl import` / `onctl destroy`). Hand edits outside\n")
	b.WriteString("# the Host/HostName/User/Port/IdentityFile shape are lost on the next write.\n")
	for _, h := range inv.Hosts {
		fmt.Fprintf(&b, "\nHost %s\n", QuoteSSHConfigValue(h.Name))
		fmt.Fprintf(&b, "    HostName %s\n", QuoteSSHConfigValue(h.IP))
		if h.Username != "" {
			fmt.Fprintf(&b, "    User %s\n", QuoteSSHConfigValue(h.Username))
		}
		if h.SSHPort != 0 {
			fmt.Fprintf(&b, "    Port %d\n", h.SSHPort)
		}
		if h.PrivateKey != "" {
			fmt.Fprintf(&b, "    IdentityFile %s\n", QuoteSSHConfigValue(h.PrivateKey))
		}
		if !h.ImportedAt.IsZero() {
			fmt.Fprintf(&b, "    %s %s\n", onctlImportedAtDirective, h.ImportedAt.Format(time.RFC3339))
		}
	}
	if err := os.MkdirAll(filepath.Dir(p.InventoryPath), 0700); err != nil {
		return fmt.Errorf("failed to create %s: %w", filepath.Dir(p.InventoryPath), err)
	}
	return os.WriteFile(p.InventoryPath, []byte(b.String()), 0600)
}

func mapStaticHost(h StaticHost) Vm {
	return Vm{
		Provider:  "static",
		Name:      h.Name,
		IP:        h.IP,
		Status:    "imported",
		Location:  "N/A",
		Type:      "N/A",
		CreatedAt: h.ImportedAt,
	}
}

func (p ProviderStatic) List() (VmList, error) {
	inv, err := p.LoadInventory()
	if err != nil {
		return VmList{}, err
	}
	list := make([]Vm, 0, len(inv.Hosts))
	for _, h := range inv.Hosts {
		list = append(list, mapStaticHost(h))
	}
	return VmList{List: list}, nil
}

func (p ProviderStatic) ListPaused() (VmList, error) {
	return VmList{}, nil
}

func (p ProviderStatic) GetByName(serverName string) (Vm, error) {
	inv, err := p.LoadInventory()
	if err != nil {
		return Vm{}, err
	}
	for _, h := range inv.Hosts {
		if h.Name == serverName {
			return mapStaticHost(h), nil
		}
	}
	return Vm{}, fmt.Errorf("no imported host found with name: %s", serverName)
}

// GetHost is like GetByName but returns the raw StaticHost (with
// username/port/key) needed to actually connect.
func (p ProviderStatic) GetHost(serverName string) (StaticHost, error) {
	inv, err := p.LoadInventory()
	if err != nil {
		return StaticHost{}, err
	}
	for _, h := range inv.Hosts {
		if h.Name == serverName {
			return h, nil
		}
	}
	return StaticHost{}, fmt.Errorf("no imported host found with name: %s", serverName)
}

func (p ProviderStatic) SSHInto(serverName string, port int, privateKey string, command []string) {
	host, err := p.GetHost(serverName)
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
	if privateKey == "" {
		privateKey = host.PrivateKey
	}
	sshPort := port
	if sshPort == 0 {
		sshPort = host.SSHPort
	}
	tools.SSHIntoVM(tools.SSHIntoVMRequest{
		IPAddress:      host.IP,
		User:           host.Username,
		Port:           sshPort,
		PrivateKeyFile: privateKey,
		Command:        command,
	})
}

// Destroy removes the named host from the local inventory. It does not
// touch the underlying machine: onctl did not create it and has no
// lifecycle API for it, so "destroying" here only means onctl forgets it.
func (p ProviderStatic) Destroy(server Vm) error {
	staticInventoryMu.Lock()
	defer staticInventoryMu.Unlock()
	inv, err := p.LoadInventory()
	if err != nil {
		return err
	}
	kept := make([]StaticHost, 0, len(inv.Hosts))
	found := false
	for _, h := range inv.Hosts {
		if h.Name == server.Name {
			found = true
			continue
		}
		kept = append(kept, h)
	}
	if !found {
		return fmt.Errorf("no imported host found with name: %s", server.Name)
	}
	inv.Hosts = kept
	return p.SaveInventory(inv)
}

func (p ProviderStatic) Deploy(_ Vm) (Vm, error) {
	return Vm{}, errStaticUnsupported
}

func (p ProviderStatic) Pause(_ Vm, _ bool) error {
	return errStaticUnsupported
}

func (p ProviderStatic) Resume(_ Vm) (Vm, error) {
	return Vm{}, errStaticUnsupported
}

func (p ProviderStatic) CreateSSHKey(_ string) (string, error) {
	return "", errStaticUnsupported
}
