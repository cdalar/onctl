package cloud

import (
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/cdalar/onctl/internal/tools"
	"golang.org/x/crypto/ssh"
)

// The ac provider runs sandboxes as Apple containers on a macOS host: each
// container is its own lightweight Linux VM on Virtualization.framework,
// driven through Apple's `container` CLI (https://github.com/apple/container,
// macOS 26+). It is the macOS counterpart of fc/ch -- no cloud account, VMs
// on the local machine -- but with no VMM process, TAP device or rootfs of
// onctl's own to manage: the `container` service owns all of that, so it is
// also the only source of truth about which sandboxes exist (List reads it
// back; nothing is persisted on disk by onctl).

const (
	acStatusRunning = "running"
	// acStatusStopped is what `container` reports for a stopped container.
	// Deliberately the same word as the cross-provider paused/resumable
	// convention (cmd.isPausedStatus): Pause is `container stop`, Resume is
	// `container start`, and a container the host rebooted under comes back
	// as "stopped" too, which Resume brings back as-is -- unlike an fc
	// microVM, nothing is lost but memory.
	acStatusStopped = "stopped"

	// acLabelManaged marks the containers onctl created, so List doesn't
	// report containers the operator runs by hand with the same CLI.
	acLabelManaged = "onctl.managed"
	// acLabelCreatedAt records the creation time (RFC 3339): `container
	// inspect` has no creation timestamp of its own.
	acLabelCreatedAt = "onctl.createdAt"

	// acEnvPublicKey and acEnvUser carry the ssh public key and user to
	// acBootstrap inside the container.
	acEnvPublicKey = "ONCTL_SSH_PUBLIC_KEY"
	acEnvUser      = "ONCTL_SSH_USER"
)

// acBootstrap is the container's init process: it puts an sshd in front of
// whatever image the sandbox was made from, so `onctl ssh` and the apply/
// upload steps work the same as on every other provider. An image without
// sshd or sudo gets them installed on first boot (apk, apt-get or dnf); an
// image that ships both boots in a moment. sudo is needed because the
// apply step (tools.applyCommand) runs every script through `sudo -E`, as
// the shipped action scripts expect; stock OCI images don't have it. On
// alpine the same step also needs bash and GNU tail (`--pid`), which
// busybox lacks. The container's own hostname goes into /etc/hosts, or
// every sudo call warns "unable to resolve host". The ssh user gets the
// public key from the environment,
// and is created if the image doesn't have it. The container's filesystem
// survives stop/start, so a later Resume re-runs this on an
// already-provisioned image and just starts sshd again.
const acBootstrap = `set -e
user="${ONCTL_SSH_USER:-root}"
if ! command -v sshd >/dev/null 2>&1 || ! command -v sudo >/dev/null 2>&1; then
  if command -v apk >/dev/null 2>&1; then apk add --no-cache openssh sudo bash coreutils >/dev/null
  elif command -v apt-get >/dev/null 2>&1; then export DEBIAN_FRONTEND=noninteractive; apt-get update -qq >/dev/null && apt-get install -y -qq openssh-server sudo >/dev/null
  elif command -v dnf >/dev/null 2>&1; then dnf install -y -q openssh-server sudo >/dev/null
  else echo "onctl: image has no sshd/sudo and no apk/apt-get/dnf to install them" >&2; exit 1
  fi
fi
ssh-keygen -A >/dev/null 2>&1 || true
h=$(hostname 2>/dev/null) && [ -n "$h" ] && ! grep -q " $h$" /etc/hosts 2>/dev/null && echo "127.0.1.1 $h" >> /etc/hosts || true
mkdir -p /run/sshd
if [ "$user" = root ]; then
  home=/root
else
  if ! id "$user" >/dev/null 2>&1; then
    useradd -m -s /bin/sh "$user" 2>/dev/null || adduser -D "$user"
  fi
  home=$(getent passwd "$user" | cut -d: -f6)
fi
mkdir -p "$home/.ssh" && chmod 700 "$home/.ssh"
printf '%s\n' "$ONCTL_SSH_PUBLIC_KEY" > "$home/.ssh/authorized_keys" && chmod 600 "$home/.ssh/authorized_keys"
chown -R "$user" "$home/.ssh"
exec /usr/sbin/sshd -D -e -o PermitRootLogin=prohibit-password
`

// ACConfig holds configuration for the Apple container provider.
type ACConfig struct {
	// Image is the OCI image new sandboxes run, unless Vm.Image says
	// otherwise. Any image with a shell and a package manager will do (see
	// acBootstrap); one that already ships sshd boots fastest.
	Image string
	// VCPUCount is the default vCPU count for new sandboxes.
	VCPUCount int64
	// MemSizeMib is the default memory size (in MiB) for new sandboxes.
	MemSizeMib int64
	// Username is the ssh user in the sandbox.
	Username string
	// BinPath is the path to the `container` CLI.
	BinPath string
}

// ACRunner runs Apple's `container` CLI.
type ACRunner interface {
	// Run executes the CLI with args and returns its standard output. A
	// non-zero exit is an error carrying the CLI's standard error.
	Run(args ...string) ([]byte, error)
}

// ProviderAC manages Apple containers on the local macOS host as
// onctl-managed VMs.
type ProviderAC struct {
	Config ACConfig
	CLI    ACRunner
	// IPTimeout bounds how long Deploy/Resume wait for a container to get
	// its address after starting (default acDefaultIPTimeout).
	IPTimeout time.Duration
	// IPPollInterval is how often they ask (default acDefaultIPPollInterval).
	IPPollInterval time.Duration
}

const (
	acDefaultIPTimeout      = 30 * time.Second
	acDefaultIPPollInterval = 500 * time.Millisecond
)

// acContainer is the part of `container ls/inspect --format json` output
// onctl reads.
type acContainer struct {
	Status        string
	Configuration acConfiguration
	Networks      []acNetwork
}

type acConfiguration struct {
	ID        string            `json:"id"`
	Labels    map[string]string `json:"labels"`
	Resources struct {
		CPUs          int64 `json:"cpus"`
		MemoryInBytes int64 `json:"memoryInBytes"`
	} `json:"resources"`
	Image struct {
		Reference string `json:"reference"`
	} `json:"image"`
}

type acNetwork struct {
	// Address is the container's address in CIDR notation. The CLI
	// has called it both names across releases.
	Address     string `json:"address"`
	IPv4Address string `json:"ipv4Address"`
}

// UnmarshalJSON accepts both CLI schemas: up to 1.5 "status" is a string
// and "networks" a top-level list; from 1.6 "status" is an object holding
// "state" and the "networks" list.
func (c *acContainer) UnmarshalJSON(data []byte) error {
	var raw struct {
		Status        json.RawMessage `json:"status"`
		Configuration acConfiguration `json:"configuration"`
		Networks      []acNetwork     `json:"networks"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	c.Configuration = raw.Configuration
	c.Networks = raw.Networks
	if len(raw.Status) == 0 {
		return nil
	}
	if err := json.Unmarshal(raw.Status, &c.Status); err == nil {
		return nil
	}
	var status struct {
		State    string      `json:"state"`
		Networks []acNetwork `json:"networks"`
	}
	if err := json.Unmarshal(raw.Status, &status); err != nil {
		return fmt.Errorf("status: %w", err)
	}
	c.Status = status.State
	if len(c.Networks) == 0 {
		c.Networks = status.Networks
	}
	return nil
}

// ip returns the container's IPv4 address, or "" while it has none (stopped,
// or not yet attached to its network).
func (c acContainer) ip() string {
	for _, n := range c.Networks {
		addr := n.Address
		if addr == "" {
			addr = n.IPv4Address
		}
		if addr == "" {
			continue
		}
		if ip, _, err := net.ParseCIDR(addr); err == nil {
			return ip.String()
		}
		if ip := net.ParseIP(addr); ip != nil {
			return ip.String()
		}
	}
	return ""
}

func mapACContainer(c acContainer) Vm {
	createdAt, _ := time.Parse(time.RFC3339, c.Configuration.Labels[acLabelCreatedAt])
	return Vm{
		Provider:  "ac",
		ID:        c.Configuration.ID,
		Name:      c.Configuration.ID,
		IP:        c.ip(),
		Type:      fmt.Sprintf("%dvcpu-%dmb", c.Configuration.Resources.CPUs, c.Configuration.Resources.MemoryInBytes>>20),
		Image:     c.Configuration.Image.Reference,
		Status:    c.Status,
		SSHPort:   22,
		SSHReady:  c.Status == acStatusRunning,
		CreatedAt: createdAt,
	}
}

func parseACContainers(data []byte) ([]acContainer, error) {
	var list []acContainer
	if err := json.Unmarshal(data, &list); err != nil {
		return nil, fmt.Errorf("unexpected output from the container CLI: %w", err)
	}
	return list, nil
}

// listAll returns every container onctl made, whatever its status.
func (p ProviderAC) listAll() ([]acContainer, error) {
	out, err := p.CLI.Run("ls", "--all", "--format", "json")
	if err != nil {
		return nil, err
	}
	all, err := parseACContainers(out)
	if err != nil {
		return nil, err
	}
	var managed []acContainer
	for _, c := range all {
		if c.Configuration.Labels[acLabelManaged] == "true" {
			managed = append(managed, c)
		}
	}
	return managed, nil
}

// find returns the named container, or nil if onctl has none by that name.
func (p ProviderAC) find(name string) (*acContainer, error) {
	all, err := p.listAll()
	if err != nil {
		return nil, err
	}
	for i := range all {
		if all[i].Configuration.ID == name {
			return &all[i], nil
		}
	}
	return nil, nil
}

// waitForIP polls the container until it has an address, which `container`
// assigns a moment after the VM starts.
func (p ProviderAC) waitForIP(name string) (acContainer, error) {
	timeout, interval := p.IPTimeout, p.IPPollInterval
	if timeout == 0 {
		timeout = acDefaultIPTimeout
	}
	if interval == 0 {
		interval = acDefaultIPPollInterval
	}
	deadline := time.Now().Add(timeout)
	for {
		c, err := p.find(name)
		if err != nil {
			return acContainer{}, err
		}
		if c == nil {
			return acContainer{}, fmt.Errorf("container %q disappeared while starting", name)
		}
		if c.Status == acStatusStopped {
			// The init process exited (acBootstrap failed, most likely), or
			// someone stopped it. Read its output now: Deploy removes the
			// container right after, so the logs are gone by the time the
			// user could look.
			return acContainer{}, fmt.Errorf("container %q exited while starting%s", name, p.logsTail(name))
		}
		if c.ip() != "" {
			return *c, nil
		}
		if time.Now().After(deadline) {
			return acContainer{}, fmt.Errorf("container %q got no IP address within %s", name, timeout)
		}
		time.Sleep(interval)
	}
}

// logsTail returns the last lines of the container's output, formatted to
// follow an error message, or "" if there are none to be had.
func (p ProviderAC) logsTail(name string) string {
	out, err := p.CLI.Run("logs", name)
	if err != nil {
		return ""
	}
	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
	if len(lines) == 1 && lines[0] == "" {
		return ""
	}
	const keep = 20
	if len(lines) > keep {
		lines = lines[len(lines)-keep:]
	}
	return ":\n  " + strings.Join(lines, "\n  ")
}

// Deploy creates and starts a new sandbox. If one with the same name already
// exists its current state is returned unchanged.
func (p ProviderAC) Deploy(server Vm) (Vm, error) {
	if server.Name == "" {
		return Vm{}, errors.New("vm name is required")
	}
	if existing, err := p.find(server.Name); err != nil {
		return Vm{}, err
	} else if existing != nil {
		log.Println("[DEBUG] container " + server.Name + " already exists")
		return mapACContainer(*existing), nil
	}

	image := p.Config.Image
	if server.Image != "" {
		image = server.Image
	}
	if image == "" {
		return Vm{}, errors.New("ac.image must be configured (see onctl init) or --image given")
	}
	vcpu, mem := parseFCType(server.Type, p.Config.VCPUCount, p.Config.MemSizeMib)
	username := p.Config.Username
	if username == "" {
		username = "root"
	}

	var sshPublicKey string
	if server.SSHKeyID != "" {
		data, err := os.ReadFile(server.SSHKeyID)
		if err != nil {
			return Vm{}, fmt.Errorf("failed to read SSH public key %q: %w", server.SSHKeyID, err)
		}
		sshPublicKey = strings.TrimSpace(string(data))
	}

	args := []string{
		"run", "--detach",
		"--name", server.Name,
		"--label", acLabelManaged + "=true",
		"--label", acLabelCreatedAt + "=" + time.Now().UTC().Format(time.RFC3339),
		"--cpus", strconv.FormatInt(vcpu, 10),
		"--memory", strconv.FormatInt(mem, 10) + "M",
		"--env", acEnvPublicKey + "=" + sshPublicKey,
		"--env", acEnvUser + "=" + username,
		"--entrypoint", "/bin/sh",
		image, "-c", acBootstrap,
	}
	if _, err := p.CLI.Run(args...); err != nil {
		return Vm{}, fmt.Errorf("failed to start container %q: %w", server.Name, err)
	}
	c, err := p.waitForIP(server.Name)
	if err != nil {
		// Don't leave a half-made sandbox behind: a retried create would
		// otherwise find it and report it as existing.
		_, _ = p.CLI.Run("stop", server.Name)
		_, _ = p.CLI.Run("rm", server.Name)
		return Vm{}, err
	}
	return mapACContainer(c), nil
}

// Destroy stops the sandbox (if running) and deletes it.
func (p ProviderAC) Destroy(server Vm) error {
	c, err := p.find(server.Name)
	if err != nil {
		return err
	}
	if c == nil {
		return fmt.Errorf("no container found with name %q", server.Name)
	}
	if c.Status != acStatusStopped {
		if _, err := p.CLI.Run("stop", server.Name); err != nil {
			return fmt.Errorf("failed to stop container %q: %w", server.Name, err)
		}
	}
	if _, err := p.CLI.Run("rm", server.Name); err != nil {
		return fmt.Errorf("failed to delete container %q: %w", server.Name, err)
	}
	return nil
}

// Pause stops the sandbox. Its filesystem is kept; memory is not (`container`
// has no snapshots), so this is a shutdown the guest sees, not a freeze --
// hot makes no difference.
func (p ProviderAC) Pause(server Vm, hot bool) error {
	c, err := p.find(server.Name)
	if err != nil {
		return err
	}
	if c == nil {
		return fmt.Errorf("no container found with name %q", server.Name)
	}
	if c.Status == acStatusStopped {
		return nil
	}
	if _, err := p.CLI.Run("stop", server.Name); err != nil {
		return fmt.Errorf("failed to stop container %q: %w", server.Name, err)
	}
	return nil
}

// Resume starts a stopped sandbox again from its own filesystem. It keeps
// its name and, in practice, its address; acBootstrap runs again and brings
// sshd back.
func (p ProviderAC) Resume(server Vm) (Vm, error) {
	c, err := p.find(server.Name)
	if err != nil {
		return Vm{}, err
	}
	if c == nil {
		return Vm{}, fmt.Errorf("no container found with name %q", server.Name)
	}
	if c.Status != acStatusStopped {
		return mapACContainer(*c), nil
	}
	if _, err := p.CLI.Run("start", server.Name); err != nil {
		return Vm{}, fmt.Errorf("failed to start container %q: %w", server.Name, err)
	}
	started, err := p.waitForIP(server.Name)
	if err != nil {
		return Vm{}, err
	}
	return mapACContainer(started), nil
}

// List returns every sandbox onctl made, stopped ones included (their status
// reads "stopped", which `onctl ls` shows under PAUSED).
func (p ProviderAC) List() (VmList, error) {
	all, err := p.listAll()
	if err != nil {
		return VmList{}, err
	}
	var list VmList
	for _, c := range all {
		list.List = append(list.List, mapACContainer(c))
	}
	return list, nil
}

// ListPaused returns an empty list: List already includes stopped sandboxes.
func (p ProviderAC) ListPaused() (VmList, error) {
	return VmList{}, nil
}

// GetByName returns the named sandbox, or a zero-value Vm if there is none.
func (p ProviderAC) GetByName(serverName string) (Vm, error) {
	c, err := p.find(serverName)
	if err != nil || c == nil {
		return Vm{}, err
	}
	return mapACContainer(*c), nil
}

// CreateSSHKey validates the given public key file and returns its absolute
// path: there is no key registry, Deploy reads the key from this path and
// hands it to the sandbox's init process.
func (p ProviderAC) CreateSSHKey(publicKeyFile string) (keyID string, err error) {
	data, err := os.ReadFile(publicKeyFile)
	if err != nil {
		return "", err
	}
	if _, _, _, _, err := ssh.ParseAuthorizedKey(data); err != nil {
		return "", fmt.Errorf("invalid SSH public key %q: %w", publicKeyFile, err)
	}
	return filepath.Abs(publicKeyFile)
}

// SSHInto connects to the sandbox at its container-network address.
func (p ProviderAC) SSHInto(serverName string, port int, privateKey string, command []string) {
	vm, err := p.GetByName(serverName)
	if err != nil || vm.Name == "" {
		log.Fatalln("no container found with name " + serverName)
	}
	if vm.Status != acStatusRunning {
		log.Fatalln("container " + serverName + " is " + vm.Status + " -- resume it first")
	}
	username := p.Config.Username
	if username == "" {
		username = "root"
	}
	tools.SSHIntoVM(tools.SSHIntoVMRequest{
		IPAddress:      vm.IP,
		User:           username,
		Port:           port,
		PrivateKeyFile: privateKey,
		Command:        command,
	})
}
