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

// defaultFCKernelArgs are the boot args used when
// fc.kernelArgs is not set. The ip= parameter is appended at deploy
// time once the guest IP is known.
const defaultFCKernelArgs = "console=ttyS0 reboot=k panic=1 pci=off"

const (
	fcStatusRunning = "running"
	fcStatusPaused  = "paused"
	// fcStatusDead marks a microVM whose firecracker process is no longer
	// alive even though its persisted metadata was last written as "running"
	// or "paused" (e.g. the host rebooted, or the process crashed).
	// Firecracker VMMs have no restart-from-disk: recovering from this state
	// requires destroying and redeploying the microVM. Deliberately not
	// "stopped" — cmd.isPausedStatus treats that string as the cross-provider
	// paused/resumable convention (AWS "stopped", GCP "TERMINATED", Azure
	// "deallocated"), which "dead" is not: there is nothing to resume.
	fcStatusDead = "dead"
)

// FCConfig holds configuration for the local Firecracker microVM provider.
type FCConfig struct {
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

// FCVMConfig describes a microVM to be configured and booted by a
// FCProcess.
type FCVMConfig struct {
	KernelImage string
	KernelArgs  string
	RootfsPath  string
	VCPUCount   int64
	MemSizeMib  int64
	TapDevice   string
	MacAddress  string
}

// FCProcess starts and stops firecracker VMM processes.
type FCProcess interface {
	// Start launches a firecracker process bound to socketPath, configures it
	// per cfg and boots it. It returns the PID of the running process.
	Start(socketPath string, cfg FCVMConfig, logFile string) (pid int, err error)
	// Stop terminates the firecracker process with the given PID.
	Stop(pid int) error
	// IsRunning reports whether a process with the given PID is alive.
	IsRunning(pid int) bool
	// Owns reports whether the process with the given PID is the firecracker
	// VMM bound to socketPath, guarding against a persisted PID having been
	// reused by an unrelated process after a VMM exit or host reboot.
	Owns(pid int, socketPath string) bool
}

// FCAPI issues runtime control requests to a running firecracker
// process over its API socket.
type FCAPI interface {
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

// ProviderFC manages local Firecracker microVMs as onctl-managed VMs.
// Unlike the other providers, there is no remote API: state is tracked on
// disk under Config.StateDir, and Net/Process/API are local-host operations.
type ProviderFC struct {
	Config  FCConfig
	Process FCProcess
	API     FCAPI
	Net     NetworkManager
	Rootfs  RootfsPreparer
}

// fcVM is the on-disk metadata persisted for each managed microVM.
type fcVM struct {
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

func (p ProviderFC) vmDir(name string) string {
	return filepath.Join(p.Config.StateDir, "vms", name)
}

func (p ProviderFC) metadataPath(name string) string {
	return filepath.Join(p.vmDir(name), "metadata.json")
}

func loadFCMetadata(path string) (fcVM, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return fcVM{}, err
	}
	var vm fcVM
	if err := json.Unmarshal(data, &vm); err != nil {
		return fcVM{}, err
	}
	return vm, nil
}

func saveFCMetadata(path string, vm fcVM) error {
	data, err := json.MarshalIndent(vm, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0600)
}

// isAlive reports whether vm's firecracker process is actually running and
// still bound to vm's socket, guarding against a persisted PID that is
// either dead or was reused by an unrelated process after a VMM exit or
// host reboot.
func (p ProviderFC) isAlive(vm fcVM) bool {
	return vm.PID > 0 && p.Process.IsRunning(vm.PID) && p.Process.Owns(vm.PID, vm.SocketPath)
}

// loadAndReconcile loads a microVM's on-disk metadata and, if its status
// claims the process is running or paused but the firecracker process is no
// longer alive, rewrites the persisted status to "dead". Without this,
// status read straight off disk (written once at Deploy/Pause/Resume time)
// keeps reporting a microVM as live indefinitely after the process dies
// out-of-band, e.g. a host reboot, which every running firecracker process
// dies to.
func (p ProviderFC) loadAndReconcile(path string) (fcVM, error) {
	vm, err := loadFCMetadata(path)
	if err != nil {
		return fcVM{}, err
	}
	if vm.Status != fcStatusDead && !p.isAlive(vm) {
		vm.Status = fcStatusDead
		if err := saveFCMetadata(path, vm); err != nil {
			log.Println("[DEBUG] failed to persist reconciled status for " + vm.Name + ": " + err.Error())
		}
	}
	return vm, nil
}

func mapFCVM(vm fcVM) Vm {
	return Vm{
		Provider:  "fc",
		ID:        vm.Name,
		Name:      vm.Name,
		IP:        vm.IPAddress,
		Type:      fmt.Sprintf("%dvcpu-%dmb", vm.VCPUCount, vm.MemSizeMib),
		Image:     vm.RootfsPath,
		Status:    vm.Status,
		CreatedAt: vm.CreatedAt,
	}
}

// parseFCType parses a "<vcpu>vcpu-<mem>mb" type string (e.g.
// "2vcpu-1024mb"). If t is empty or doesn't match, the provided defaults are
// returned.
func parseFCType(t string, defaultVCPU, defaultMem int64) (vcpu, mem int64) {
	if t == "" {
		return defaultVCPU, defaultMem
	}
	var v, m int64
	if _, err := fmt.Sscanf(strings.ToLower(t), "%dvcpu-%dmb", &v, &m); err == nil && v > 0 && m > 0 {
		return v, m
	}
	return defaultVCPU, defaultMem
}

// fcTapName derives a deterministic, <=15 char TAP device name from
// the VM name (the Linux interface name length limit).
func fcTapName(vmName string) string {
	sum := md5.Sum([]byte(vmName))
	return "fc" + fmt.Sprintf("%x", sum)[:13]
}

// fcMAC derives a deterministic, locally-administered MAC address
// from the VM name so it stays stable across pause/resume.
func fcMAC(vmName string) string {
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

// allocateFCIP returns the next free IPv4 address in cidr, skipping
// the network address, the gateway (first usable address), the broadcast
// address and any address in used.
func allocateFCIP(cidr string, used map[string]bool) (string, error) {
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
func (p ProviderFC) usedIPs() (map[string]bool, error) {
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
func (p ProviderFC) listAll() ([]fcVM, error) {
	root := filepath.Join(p.Config.StateDir, "vms")
	entries, err := os.ReadDir(root)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	vms := make([]fcVM, 0, len(entries))
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		vm, err := p.loadAndReconcile(filepath.Join(root, e.Name(), "metadata.json"))
		if err != nil {
			log.Println("[DEBUG] skipping " + e.Name() + ": " + err.Error())
			continue
		}
		vms = append(vms, vm)
	}
	return vms, nil
}

// Deploy creates and boots a new microVM. If a microVM with the same name
// already exists and is alive, its current state is returned unchanged. If
// its record is stale (the firecracker process is no longer running, e.g.
// after a host reboot), the stale state is cleared and a fresh microVM is
// deployed in its place.
func (p ProviderFC) Deploy(server Vm) (Vm, error) {
	if server.Name == "" {
		return Vm{}, errors.New("vm name is required")
	}

	if existing, err := p.loadAndReconcile(p.metadataPath(server.Name)); err == nil {
		if existing.Status != fcStatusDead {
			log.Println("[DEBUG] microVM " + server.Name + " already exists")
			return mapFCVM(existing), nil
		}
		log.Println("[DEBUG] microVM " + server.Name + " has a stale record (firecracker process is no longer running); recreating")
		if existing.TapDevice != "" {
			if err := p.Net.DeleteTap(existing.TapDevice); err != nil {
				log.Println("[DEBUG] failed to delete stale tap device " + existing.TapDevice + ": " + err.Error())
			}
		}
		if err := os.RemoveAll(p.vmDir(server.Name)); err != nil {
			return Vm{}, fmt.Errorf("failed to remove stale microVM state: %w", err)
		}
	}

	kernelImage := p.Config.KernelImage
	rootfsImage := p.Config.RootfsImage
	if server.Image != "" {
		rootfsImage = server.Image
	}
	if kernelImage == "" || rootfsImage == "" {
		return Vm{}, errors.New("fc.kernelImage and fc.rootfsImage must be configured (see onctl init)")
	}

	dir := p.vmDir(server.Name)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return Vm{}, fmt.Errorf("failed to create vm directory: %w", err)
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

	tapDevice := fcTapName(server.Name)
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
	ip, err := allocateFCIP(cidr, used)
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
		kernelArgs = defaultFCKernelArgs
	}
	kernelArgs = fmt.Sprintf("%s ip=%s::%s:%s::eth0:off", kernelArgs, ip, gateway, mask)

	mac := fcMAC(server.Name)
	socketPath := filepath.Join(dir, "fc.sock")
	logFile := filepath.Join(dir, "fc.log")

	pid, err := p.Process.Start(socketPath, FCVMConfig{
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

	vm := fcVM{
		Name:        server.Name,
		PID:         pid,
		SocketPath:  socketPath,
		TapDevice:   tapDevice,
		IPAddress:   ip,
		MacAddress:  mac,
		VCPUCount:   vcpu,
		MemSizeMib:  mem,
		Status:      fcStatusRunning,
		KernelImage: kernelImage,
		RootfsPath:  rootfsPath,
		CreatedAt:   time.Now(),
	}
	if err := saveFCMetadata(p.metadataPath(server.Name), vm); err != nil {
		return Vm{}, fmt.Errorf("microVM started but failed to persist metadata: %w", err)
	}
	return mapFCVM(vm), nil
}

// Destroy stops the microVM (if running), removes its TAP device and deletes
// its on-disk state.
func (p ProviderFC) Destroy(server Vm) error {
	if server.Name == "" {
		return errors.New("vm name is required")
	}
	vm, err := loadFCMetadata(p.metadataPath(server.Name))
	if err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("no microVM found with name %q", server.Name)
		}
		return err
	}
	if vm.PID > 0 && p.Process.IsRunning(vm.PID) {
		if !p.Process.Owns(vm.PID, vm.SocketPath) {
			log.Println("[DEBUG] persisted PID " + fmt.Sprint(vm.PID) + " for " + server.Name + " is no longer the firecracker process for this microVM; skipping stop")
		} else if err := p.Process.Stop(vm.PID); err != nil {
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
func (p ProviderFC) Pause(server Vm, hot bool) error {
	vm, err := loadFCMetadata(p.metadataPath(server.Name))
	if err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("no microVM found with name %q", server.Name)
		}
		return err
	}
	if vm.Status == fcStatusPaused {
		return nil
	}
	if err := p.API.SetState(vm.SocketPath, "Paused"); err != nil {
		return fmt.Errorf("failed to pause microVM: %w", err)
	}
	vm.Status = fcStatusPaused
	return saveFCMetadata(p.metadataPath(server.Name), vm)
}

// Resume unfreezes a paused microVM's vCPUs via the Firecracker API.
func (p ProviderFC) Resume(server Vm) (Vm, error) {
	vm, err := loadFCMetadata(p.metadataPath(server.Name))
	if err != nil {
		if os.IsNotExist(err) {
			return Vm{}, fmt.Errorf("no microVM found with name %q", server.Name)
		}
		return Vm{}, err
	}
	if vm.Status != fcStatusPaused {
		return mapFCVM(vm), nil
	}
	if err := p.API.SetState(vm.SocketPath, "Resumed"); err != nil {
		return Vm{}, fmt.Errorf("failed to resume microVM: %w", err)
	}
	vm.Status = fcStatusRunning
	if err := saveFCMetadata(p.metadataPath(server.Name), vm); err != nil {
		return Vm{}, err
	}
	return mapFCVM(vm), nil
}

// List returns all non-paused managed microVMs (see ListPaused for paused
// ones). A microVM whose firecracker process has died out-of-band (e.g. a
// host reboot, which no firecracker VMM survives) is included with status
// "dead" rather than the stale "running" last written to disk.
func (p ProviderFC) List() (VmList, error) {
	all, err := p.listAll()
	if err != nil {
		return VmList{}, err
	}
	var list VmList
	for _, vm := range all {
		if vm.Status == fcStatusPaused {
			continue
		}
		list.List = append(list.List, mapFCVM(vm))
	}
	return list, nil
}

// ListPaused returns all paused managed microVMs.
func (p ProviderFC) ListPaused() (VmList, error) {
	all, err := p.listAll()
	if err != nil {
		return VmList{}, err
	}
	var list VmList
	for _, vm := range all {
		if vm.Status == fcStatusPaused {
			list.List = append(list.List, mapFCVM(vm))
		}
	}
	return list, nil
}

// GetByName returns the named microVM, or a zero-value Vm if it doesn't exist.
func (p ProviderFC) GetByName(serverName string) (Vm, error) {
	vm, err := p.loadAndReconcile(p.metadataPath(serverName))
	if err != nil {
		if os.IsNotExist(err) {
			return Vm{}, nil
		}
		return Vm{}, err
	}
	return mapFCVM(vm), nil
}

// CreateSSHKey validates the given public key file and returns its absolute
// path. Unlike the remote providers there is no key registry to upload to:
// Deploy reads the key directly from this path and injects it into the
// microVM's rootfs.
func (p ProviderFC) CreateSSHKey(publicKeyFile string) (keyID string, err error) {
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
func (p ProviderFC) SSHInto(serverName string, port int, privateKey string, command []string) {
	vm, err := p.GetByName(serverName)
	if err != nil || vm.Name == "" {
		log.Fatalln("no microVM found with name " + serverName)
	}
	if vm.Status == fcStatusDead {
		log.Fatalln("microVM " + serverName + " is not running (its firecracker process is gone, e.g. after a host reboot) — destroy and recreate it")
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
