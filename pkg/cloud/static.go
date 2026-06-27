package cloud

import (
	"errors"
	"fmt"
	"os"
	"sync"
	"time"

	"github.com/cdalar/onctl/internal/tools"
	"gopkg.in/yaml.v3"
)

// StaticHost is one entry in the imported-hosts inventory: a server onctl
// did not create and has no lifecycle API for, registered via `onctl import`
// so ssh/ls can reach it.
type StaticHost struct {
	Name       string    `yaml:"name"`
	IP         string    `yaml:"ip"`
	Username   string    `yaml:"username"`
	SSHPort    int       `yaml:"sshPort"`
	PrivateKey string    `yaml:"privateKey,omitempty"`
	ImportedAt time.Time `yaml:"importedAt"`
}

type StaticInventory struct {
	Hosts []StaticHost `yaml:"hosts"`
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

func (p ProviderStatic) LoadInventory() (StaticInventory, error) {
	var inv StaticInventory
	data, err := os.ReadFile(p.InventoryPath)
	if errors.Is(err, os.ErrNotExist) {
		return inv, nil
	}
	if err != nil {
		return inv, fmt.Errorf("failed to read %s: %w", p.InventoryPath, err)
	}
	if err := yaml.Unmarshal(data, &inv); err != nil {
		return inv, fmt.Errorf("failed to parse %s: %w", p.InventoryPath, err)
	}
	return inv, nil
}

func (p ProviderStatic) SaveInventory(inv StaticInventory) error {
	data, err := yaml.Marshal(inv)
	if err != nil {
		return err
	}
	return os.WriteFile(p.InventoryPath, data, 0644)
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
