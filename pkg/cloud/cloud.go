package cloud

import (
	"fmt"
	"net"
	"reflect"
	"sync"
	"time"
)

type VmList struct {
	List []Vm
}

type Price struct {
	// Currency is the currency of the price
	Currency string
	// Hourly is the hourly price
	Hourly string
	// Monthly is the monthly price
	Monthly string
}

type Vm struct {
	// ID is the ID of the instance
	ID string
	// Name is the name of the instance
	Name string `yaml:"name"`
	// IP is the public IP of the instance
	IP string
	//LocalIP is the local IP of the instance
	PrivateIP string
	// Type is the type of the instance
	Type string `yaml:"type"`
	// Image is the OS image to use for the instance
	Image string `yaml:"image"`
	// Status is the status of the instance
	Status string
	// SSHReady reports whether it's actually safe to open a terminal to
	// this VM -- Status alone only reflects "the VMM process is alive,"
	// not "the guest OS finished booting," and for a slow-booting guest
	// that gap can be minutes wide. Only ch (ProviderCH.reconcileSSHReady)
	// really TCP-probes for this (see ProbeTCP/ProbeSSHReady), since a
	// Cloud Hypervisor/Windows guest can take that long. fc derives it
	// straight from Status instead (mapFCVM) -- a Firecracker microVM
	// boots in milliseconds, so probing would only add latency to List()
	// (and, via boxctl-vms-agent.sh's periodic poll, to every dashboard
	// refresh) for a distinction that isn't real there. The remote-cloud
	// providers below never set it at all: their own Deploy already
	// blocks on WaitForCloudInit/WaitForSSH before ever returning, so by
	// the time such a VM shows up in List(), it was already fully ready.
	SSHReady bool
	// Location is the location of the instance
	Location string
	// SSHKeyID is the ID of the SSH key
	SSHKeyID string
	// SSHPort is the port to connect to the instance
	SSHPort int `yaml:"sshPort"`
	// CloudInit is the cloud-init file
	CloudInitFile string `yaml:"cloudInitFile"`
	// CreatedAt is the creation date of the instance
	CreatedAt time.Time
	// Provider is the cloud provider
	Provider string
	// Cost is the cost of the vm
	Cost CostStruct
}

type CostStruct struct {
	Currency        string
	CostPerHour     float64
	CostPerMonth    float64
	AccumulatedCost float64
}

// ProbeTCP reports whether a TCP connection to addr ("host:port") succeeds
// within timeout. Used by fc/ch's List() to compute Vm.SSHReady -- a plain
// TCP connect, not a full SSH handshake: WaitForSSH (internal/tools/
// remote-run.go), used at create time, already does the real handshake, but
// that needs the operator's actual SSH private key material threaded all
// the way into this package (pkg/cloud currently has none, by design --
// key handling lives in cmd/ and internal/tools) and, worse, its key-
// parsing path can block on an interactive passphrase prompt on stdin if
// the key is passphrase-protected. That's fine once, synchronously, in a
// CLI command a human is sitting at; it would be a real bug here, since
// List() also runs from a long-running background poll (see boxctl-vms-
// agent.sh) with no terminal attached, where blocking on stdin would hang
// forever. A successful TCP connect to the SSH port is a materially
// weaker signal (it doesn't prove key auth would succeed, only that
// sshd/OpenSSH is listening) but is what actually closes the multi-minute
// gap this exists for -- the "guest hasn't even finished booting /
// networking isn't up yet" phase -- without any of the above risk.
func ProbeTCP(addr string, timeout time.Duration) bool {
	conn, err := net.DialTimeout("tcp", addr, timeout)
	if err != nil {
		return false
	}
	_ = conn.Close()
	return true
}

// probeTimeout bounds each individual TCP probe ProbeSSHReady issues.
// Short, since these run inline in List() (called on every `onctl ls`,
// including a periodic poll every few seconds -- see boxctl-vms-agent.sh)
// and every not-yet-ready VM is probed concurrently, so this is close to
// the actual worst-case added latency, not a per-VM sum.
const probeTimeout = 300 * time.Millisecond

// ProbeSSHReady concurrently TCP-probes every candidate VM's IP:port (see
// ProbeTCP) and returns the set of indices (into candidates) that answered
// within probeTimeout. Shared by ProviderFC.List and ProviderCH.List,
// which each know how to persist SSHReady=true back to their own
// on-disk metadata for the returned indices -- this function has no
// opinion on persistence, just on which candidates are currently
// reachable.
func ProbeSSHReady(candidates []Vm) map[int]bool {
	ready := make(map[int]bool)
	var mu sync.Mutex
	var wg sync.WaitGroup
	for i, vm := range candidates {
		if vm.IP == "" {
			continue
		}
		port := vm.SSHPort
		if port == 0 {
			port = 22
		}
		wg.Add(1)
		go func(i int, addr string) {
			defer wg.Done()
			if ProbeTCP(addr, probeTimeout) {
				mu.Lock()
				ready[i] = true
				mu.Unlock()
			}
		}(i, net.JoinHostPort(vm.IP, fmt.Sprint(port)))
	}
	wg.Wait()
	return ready
}

func (v Vm) String() string {
	value := reflect.ValueOf(v)
	typeOfS := value.Type()
	ret := "\n"
	for i := 0; i < value.NumField(); i++ {
		ret = ret + fmt.Sprintf("%s:\t %v\n", typeOfS.Field(i).Name, value.Field(i).Interface())
	}
	return ret
}

type CloudImage struct {
	Name        string
	Description string
	Type        string
	OSFlavor    string
	OSVersion   string
}

// ImageLister is an optional interface for providers that support listing
// available OS images. Not all providers implement this.
type ImageLister interface {
	ListImages() ([]CloudImage, error)
}

type CloudProviderInterface interface {
	// Deploy deploys a new instance
	Deploy(Vm) (Vm, error)
	// Destroy destroys an instance
	Destroy(Vm) error
	// Pause stops the instance so it no longer accrues compute cost. On clouds
	// that bill stopped instances (e.g. Hetzner) this snapshots the disk and
	// deletes the instance; elsewhere it stops/deallocates. When hot is false the
	// instance is gracefully shut down first (only relevant to the snapshot path).
	Pause(server Vm, hot bool) error
	// Resume brings a paused instance back (from snapshot or by starting it).
	Resume(Vm) (Vm, error)
	// List lists all instances
	List() (VmList, error)
	// ListPaused lists servers that are paused but not returned by List (e.g.
	// Hetzner pause snapshots). Providers whose List already includes stopped
	// instances return an empty list.
	ListPaused() (VmList, error)
	// CreateSSHKey creates a new SSH key
	CreateSSHKey(publicKeyFile string) (keyID string, err error)
	// SSHInto connects to a VM
	SSHInto(serverName string, port int, privateKey string, command []string)
	// GetByName gets a VM by name
	GetByName(serverName string) (Vm, error)
}
