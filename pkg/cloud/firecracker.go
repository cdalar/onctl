package cloud

import (
	"crypto/md5"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/cdalar/onctl/internal/tools"
	"golang.org/x/crypto/ssh"
)

// defaultFirecrackerKernelArgs are the boot args used when
// firecracker.kernelArgs is not set. The ip= parameter is appended at deploy
// time once the guest IP is known.
const defaultFirecrackerKernelArgs = "console=ttyS0 reboot=k panic=1 pci=off"

const (
	firecrackerStatusRunning = "running"
	firecrackerStatusPaused  = "paused"
)

// FirecrackerConfig holds configuration for the local Firecracker microVM provider.
type FirecrackerConfig struct {
	// KernelImage is the path to the uncompressed Linux kernel image (vmlinux).
	KernelImage string
	// RootfsImage is the path to the base rootfs image used as a template for
	// new microVMs (copied per-VM, never modified in place).
	RootfsImage string
	// KernelArgs are extra kernel boot arguments (network config is appended
	// automatically).
	KernelArgs string
	// VCPUCount is the default vCPU count for new microVMs.
	VCPUCount int64
	// MemSizeMib is the default memory size (in MiB) for new microVMs.
	MemSizeMib int64
	// Bridge is the name of the host bridge device microVM TAP devices attach to.
	Bridge string
	// CIDR is the bridge's address and subnet (e.g. "172.16.0.1/24"). The
	// bridge address is used as the gateway for microVMs.
	CIDR string
	// Username is the SSH user configured in the rootfs image.
	Username string
	// BinPath is the path to the firecracker binary.
	BinPath string
	// StateDir is the directory onctl stores microVM state under
	// (default ~/.onctl/firecracker).
	StateDir string
}

// FirecrackerVMConfig describes a microVM to be configured and booted by a
// FirecrackerProcess.
type FirecrackerVMConfig struct {
	KernelImage string
	KernelArgs  string
	RootfsPath  string
	VCPUCount   int64
	MemSizeMib  int64
	TapDevice   string
	MacAddress  string
}

// FirecrackerProcess starts and stops firecracker VMM processes.
type FirecrackerProcess interface {
	// Start launches a firecracker process bound to socketPath, configures it
	// per cfg and boots it. It returns the PID of the running process.
	Start(socketPath string, cfg FirecrackerVMConfig, logFile string) (pid int, err error)
	// Stop terminates the firecracker process with the given PID.
	Stop(pid int) error
	// IsRunning reports whether a process with the given PID is alive.
	IsRunning(pid int) bool
}

// FirecrackerAPI issues runtime control requests to a running firecracker
// process over its API socket.
type FirecrackerAPI interface {
	// SetState transitions the microVM's state (e.g. "Paused" or "Resumed").
	SetState(socketPath, state string) error
}

// NetworkManager creates/destroys the host-side networking for microVMs.
type NetworkManager interface {
	// EnsureBridge creates the bridge device with the given CIDR if it
	// doesn't already exist.
	EnsureBridge(bridge, cidr string) error
	// CreateTap creates a TAP device attached to bridge.
	CreateTap(tapName, bridge string) error
	// DeleteTap removes a TAP device.
	DeleteTap(tapName string) error
}

// RootfsPreparer creates a per-VM writable rootfs image from the configured
// base image and injects the SSH public key for the configured user.
type RootfsPreparer interface {
	// Prepare creates destPath as a writable copy of baseImage. If
	// sshPublicKey is non-empty it is added to username's authorized_keys.
	Prepare(baseImage, destPath, sshPublicKey, username string) error
}

// ProviderFirecracker manages local Firecracker microVMs as onctl-managed VMs.
// Unlike the other providers, there is no remote API: state is tracked on
// disk under Config.StateDir, and Net/Process/API are local-host operations.
type ProviderFirecracker struct {
	Config  FirecrackerConfig
	Process FirecrackerProcess
	API     FirecrackerAPI
	Net     NetworkManager
	Rootfs  RootfsPreparer
}

// firecrackerVM is the on-disk metadata persisted for each managed microVM.
type firecrackerVM struct {
	Name        string    `json:"name"`
	PID         int       `json:"pid"`
	SocketPath  string    `json:"socketPath"`
	TapDevice   string    `json:"tapDevice"`
	IPAddress   string    `json:"ipAddress"`
	MacAddress  string    `json:"macAddress"`
	VCPUCount   int64     `json:"vcpuCount"`
	MemSizeMib  int64     `json:"memSizeMib"`
	Status      string    `json:"status"`
	KernelImage string    `json:"kernelImage"`
	RootfsPath  string    `json:"rootfsPath"`
	CreatedAt   time.Time `json:"createdAt"`
}

func (p ProviderFirecracker) vmDir(name string) string {
	return filepath.Join(p.Config.StateDir, "vms", name)
}

func (p ProviderFirecracker) metadataPath(name string) string {
	return filepath.Join(p.vmDir(name), "metadata.json")
}

func loadFirecrackerMetadata(path string) (firecrackerVM, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return firecrackerVM{}, err
	}
	var vm firecrackerVM
	if err := json.Unmarshal(data, &vm); err != nil {
		return firecrackerVM{}, err
	}
	return vm, nil
}

func saveFirecrackerMetadata(path string, vm firecrackerVM) error {
	data, err := json.MarshalIndent(vm, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0600)
}

func mapFirecrackerVM(vm firecrackerVM) Vm {
	return Vm{
		Provider:  "firecracker",
		ID:        vm.Name,
		Name:      vm.Name,
		IP:        vm.IPAddress,
		Type:      fmt.Sprintf("%dvcpu-%dmb", vm.VCPUCount, vm.MemSizeMib),
		Image:     vm.RootfsPath,
		Status:    vm.Status,
		CreatedAt: vm.CreatedAt,
	}
}

// parseFirecrackerType parses a "<vcpu>vcpu-<mem>mb" type string (e.g.
// "2vcpu-1024mb"). If t is empty or doesn't match, the provided defaults are
// returned.
func parseFirecrackerType(t string, defaultVCPU, defaultMem int64) (vcpu, mem int64) {
	if t == "" {
		return defaultVCPU, defaultMem
	}
	var v, m int64
	if _, err := fmt.Sscanf(strings.ToLower(t), "%dvcpu-%dmb", &v, &m); err == nil && v > 0 && m > 0 {
		return v, m
	}
	return defaultVCPU, defaultMem
}

// firecrackerTapName derives a deterministic, <=15 char TAP device name from
// the VM name (the Linux interface name length limit).
func firecrackerTapName(vmName string) string {
	sum := md5.Sum([]byte(vmName))
	return "fc" + fmt.Sprintf("%x", sum)[:13]
}

// firecrackerMAC derives a deterministic, locally-administered MAC address
// from the VM name so it stays stable across pause/resume.
func firecrackerMAC(vmName string) string {
	sum := md5.Sum([]byte(vmName))
	return fmt.Sprintf("02:FC:%02x:%02x:%02x:%02x", sum[0], sum[1], sum[2], sum[3])
}

// bridgeGatewayAndMask returns the gateway IP and dotted-decimal netmask for
// a bridge CIDR such as "172.16.0.1/24".
func bridgeGatewayAndMask(cidr string) (gateway, mask string, err error) {
	ip, ipNet, err := net.ParseCIDR(cidr)
	if err != nil {
		return "", "", fmt.Errorf("invalid network %q: %w", cidr, err)
	}
	if ip.To4() == nil {
		return "", "", fmt.Errorf("only IPv4 networks are supported, got %q", cidr)
	}
	return ip.String(), net.IP(ipNet.Mask).String(), nil
}

// allocateFirecrackerIP returns the next free IPv4 address in cidr, skipping
// the network address, the gateway (first usable address), the broadcast
// address and any address in used.
func allocateFirecrackerIP(cidr string, used map[string]bool) (string, error) {
	gateway, ipNet, err := net.ParseCIDR(cidr)
	if err != nil {
		return "", fmt.Errorf("invalid network %q: %w", cidr, err)
	}
	gateway = gateway.To4()
	if gateway == nil {
		return "", fmt.Errorf("only IPv4 networks are supported, got %q", cidr)
	}
	broadcast := make(net.IP, len(ipNet.IP.To4()))
	for i := range broadcast {
		broadcast[i] = ipNet.IP.To4()[i] | ^ipNet.Mask[i]
	}

	candidate := make(net.IP, len(gateway))
	copy(candidate, gateway)
	for ipNet.Contains(candidate) {
		if !candidate.Equal(gateway) && !candidate.Equal(broadcast) && !used[candidate.String()] {
			return candidate.String(), nil
		}
		incIP(candidate)
	}
	return "", fmt.Errorf("no free IP addresses available in %s", cidr)
}

func incIP(ip net.IP) {
	for i := len(ip) - 1; i >= 0; i-- {
		ip[i]++
		if ip[i] != 0 {
			return
		}
	}
}

// usedIPs returns the set of IP addresses already assigned to managed microVMs.
func (p ProviderFirecracker) usedIPs() (map[string]bool, error) {
	all, err := p.listAll()
	if err != nil {
		return nil, err
	}
	used := make(map[string]bool, len(all))
	for _, vm := range all {
		if vm.IPAddress != "" {
			used[vm.IPAddress] = true
		}
	}
	return used, nil
}

// listAll returns the metadata for every managed microVM, regardless of status.
func (p ProviderFirecracker) listAll() ([]firecrackerVM, error) {
	root := filepath.Join(p.Config.StateDir, "vms")
	entries, err := os.ReadDir(root)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	vms := make([]firecrackerVM, 0, len(entries))
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		vm, err := loadFirecrackerMetadata(filepath.Join(root, e.Name(), "metadata.json"))
		if err != nil {
			log.Println("[DEBUG] skipping " + e.Name() + ": " + err.Error())
			continue
		}
		vms = append(vms, vm)
	}
	return vms, nil
}

// Deploy creates and boots a new microVM. If a microVM with the same name
// already exists, its current state is returned unchanged.
func (p ProviderFirecracker) Deploy(server Vm) (Vm, error) {
	if server.Name == "" {
		return Vm{}, errors.New("vm name is required")
	}

	if existing, err := loadFirecrackerMetadata(p.metadataPath(server.Name)); err == nil {
		log.Println("[DEBUG] microVM " + server.Name + " already exists")
		return mapFirecrackerVM(existing), nil
	}

	kernelImage := p.Config.KernelImage
	rootfsImage := p.Config.RootfsImage
	if server.Image != "" {
		rootfsImage = server.Image
	}
	if kernelImage == "" || rootfsImage == "" {
		return Vm{}, errors.New("firecracker.kernelImage and firecracker.rootfsImage must be configured (see onctl init)")
	}

	dir := p.vmDir(server.Name)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return Vm{}, fmt.Errorf("failed to create vm directory: %w", err)
	}

	vcpu, mem := parseFirecrackerType(server.Type, p.Config.VCPUCount, p.Config.MemSizeMib)

	username := p.Config.Username
	if username == "" {
		username = "root"
	}

	var sshPublicKey string
	if server.SSHKeyID != "" {
		data, err := os.ReadFile(server.SSHKeyID)
		if err != nil {
			_ = os.RemoveAll(dir)
			return Vm{}, fmt.Errorf("failed to read SSH public key %q: %w", server.SSHKeyID, err)
		}
		sshPublicKey = strings.TrimSpace(string(data))
	}

	rootfsPath := filepath.Join(dir, "rootfs.ext4")
	if err := p.Rootfs.Prepare(rootfsImage, rootfsPath, sshPublicKey, username); err != nil {
		_ = os.RemoveAll(dir)
		return Vm{}, fmt.Errorf("failed to prepare rootfs: %w", err)
	}

	bridge := p.Config.Bridge
	if bridge == "" {
		bridge = "fcbr0"
	}
	cidr := p.Config.CIDR
	if cidr == "" {
		cidr = "172.16.0.1/24"
	}
	if err := p.Net.EnsureBridge(bridge, cidr); err != nil {
		_ = os.RemoveAll(dir)
		return Vm{}, fmt.Errorf("failed to set up bridge %q: %w", bridge, err)
	}

	tapDevice := firecrackerTapName(server.Name)
	if err := p.Net.CreateTap(tapDevice, bridge); err != nil {
		_ = os.RemoveAll(dir)
		return Vm{}, fmt.Errorf("failed to create tap device: %w", err)
	}

	used, err := p.usedIPs()
	if err != nil {
		_ = p.Net.DeleteTap(tapDevice)
		_ = os.RemoveAll(dir)
		return Vm{}, err
	}
	ip, err := allocateFirecrackerIP(cidr, used)
	if err != nil {
		_ = p.Net.DeleteTap(tapDevice)
		_ = os.RemoveAll(dir)
		return Vm{}, err
	}
	gateway, mask, err := bridgeGatewayAndMask(cidr)
	if err != nil {
		_ = p.Net.DeleteTap(tapDevice)
		_ = os.RemoveAll(dir)
		return Vm{}, err
	}

	kernelArgs := strings.TrimSpace(p.Config.KernelArgs)
	if kernelArgs == "" {
		kernelArgs = defaultFirecrackerKernelArgs
	}
	kernelArgs = fmt.Sprintf("%s ip=%s::%s:%s::eth0:off", kernelArgs, ip, gateway, mask)

	mac := firecrackerMAC(server.Name)
	socketPath := filepath.Join(dir, "firecracker.sock")
	logFile := filepath.Join(dir, "firecracker.log")

	pid, err := p.Process.Start(socketPath, FirecrackerVMConfig{
		KernelImage: kernelImage,
		KernelArgs:  kernelArgs,
		RootfsPath:  rootfsPath,
		VCPUCount:   vcpu,
		MemSizeMib:  mem,
		TapDevice:   tapDevice,
		MacAddress:  mac,
	}, logFile)
	if err != nil {
		_ = p.Net.DeleteTap(tapDevice)
		_ = os.RemoveAll(dir)
		return Vm{}, fmt.Errorf("failed to start microVM: %w", err)
	}

	vm := firecrackerVM{
		Name:        server.Name,
		PID:         pid,
		SocketPath:  socketPath,
		TapDevice:   tapDevice,
		IPAddress:   ip,
		MacAddress:  mac,
		VCPUCount:   vcpu,
		MemSizeMib:  mem,
		Status:      firecrackerStatusRunning,
		KernelImage: kernelImage,
		RootfsPath:  rootfsPath,
		CreatedAt:   time.Now(),
	}
	if err := saveFirecrackerMetadata(p.metadataPath(server.Name), vm); err != nil {
		return Vm{}, fmt.Errorf("microVM started but failed to persist metadata: %w", err)
	}
	return mapFirecrackerVM(vm), nil
}

// Destroy stops the microVM (if running), removes its TAP device and deletes
// its on-disk state.
func (p ProviderFirecracker) Destroy(server Vm) error {
	if server.Name == "" {
		return errors.New("vm name is required")
	}
	vm, err := loadFirecrackerMetadata(p.metadataPath(server.Name))
	if err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("no microVM found with name %q", server.Name)
		}
		return err
	}
	if vm.PID > 0 && p.Process.IsRunning(vm.PID) {
		if err := p.Process.Stop(vm.PID); err != nil {
			return fmt.Errorf("failed to stop microVM: %w", err)
		}
	}
	if vm.TapDevice != "" {
		if err := p.Net.DeleteTap(vm.TapDevice); err != nil {
			log.Println("[DEBUG] failed to delete tap device " + vm.TapDevice + ": " + err.Error())
		}
	}
	return os.RemoveAll(p.vmDir(server.Name))
}

// Pause freezes the microVM's vCPUs via the Firecracker API. The hot flag is
// accepted for interface symmetry: a Firecracker pause is always a live,
// in-memory vCPU freeze (no snapshot-to-disk), so there is no "cold" variant.
func (p ProviderFirecracker) Pause(server Vm, hot bool) error {
	vm, err := loadFirecrackerMetadata(p.metadataPath(server.Name))
	if err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("no microVM found with name %q", server.Name)
		}
		return err
	}
	if vm.Status == firecrackerStatusPaused {
		return nil
	}
	if err := p.API.SetState(vm.SocketPath, "Paused"); err != nil {
		return fmt.Errorf("failed to pause microVM: %w", err)
	}
	vm.Status = firecrackerStatusPaused
	return saveFirecrackerMetadata(p.metadataPath(server.Name), vm)
}

// Resume unfreezes a paused microVM's vCPUs via the Firecracker API.
func (p ProviderFirecracker) Resume(server Vm) (Vm, error) {
	vm, err := loadFirecrackerMetadata(p.metadataPath(server.Name))
	if err != nil {
		if os.IsNotExist(err) {
			return Vm{}, fmt.Errorf("no microVM found with name %q", server.Name)
		}
		return Vm{}, err
	}
	if vm.Status != firecrackerStatusPaused {
		return mapFirecrackerVM(vm), nil
	}
	if err := p.API.SetState(vm.SocketPath, "Resumed"); err != nil {
		return Vm{}, fmt.Errorf("failed to resume microVM: %w", err)
	}
	vm.Status = firecrackerStatusRunning
	if err := saveFirecrackerMetadata(p.metadataPath(server.Name), vm); err != nil {
		return Vm{}, err
	}
	return mapFirecrackerVM(vm), nil
}

// List returns all running (non-paused) managed microVMs.
func (p ProviderFirecracker) List() (VmList, error) {
	all, err := p.listAll()
	if err != nil {
		return VmList{}, err
	}
	var list VmList
	for _, vm := range all {
		if vm.Status == firecrackerStatusPaused {
			continue
		}
		list.List = append(list.List, mapFirecrackerVM(vm))
	}
	return list, nil
}

// ListPaused returns all paused managed microVMs.
func (p ProviderFirecracker) ListPaused() (VmList, error) {
	all, err := p.listAll()
	if err != nil {
		return VmList{}, err
	}
	var list VmList
	for _, vm := range all {
		if vm.Status == firecrackerStatusPaused {
			list.List = append(list.List, mapFirecrackerVM(vm))
		}
	}
	return list, nil
}

// GetByName returns the named microVM, or a zero-value Vm if it doesn't exist.
func (p ProviderFirecracker) GetByName(serverName string) (Vm, error) {
	vm, err := loadFirecrackerMetadata(p.metadataPath(serverName))
	if err != nil {
		if os.IsNotExist(err) {
			return Vm{}, nil
		}
		return Vm{}, err
	}
	return mapFirecrackerVM(vm), nil
}

// CreateSSHKey validates the given public key file and returns its absolute
// path. Unlike the remote providers there is no key registry to upload to:
// Deploy reads the key directly from this path and injects it into the
// microVM's rootfs.
func (p ProviderFirecracker) CreateSSHKey(publicKeyFile string) (keyID string, err error) {
	data, err := os.ReadFile(publicKeyFile)
	if err != nil {
		return "", err
	}
	if _, _, _, _, err := ssh.ParseAuthorizedKey(data); err != nil {
		return "", fmt.Errorf("invalid SSH public key %q: %w", publicKeyFile, err)
	}
	return filepath.Abs(publicKeyFile)
}

// SSHInto connects to the microVM over its TAP device IP address.
func (p ProviderFirecracker) SSHInto(serverName string, port int, privateKey string, command []string) {
	vm, err := p.GetByName(serverName)
	if err != nil || vm.Name == "" {
		log.Fatalln("no microVM found with name " + serverName)
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
