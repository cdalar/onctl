package cloud

import (
	"crypto/md5"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/cdalar/onctl/internal/tools"
	"golang.org/x/crypto/ssh"
)

// defaultCHKernelArgs are the boot args used when ch.kernelArgs is not set.
// Unlike Firecracker (MMIO, no PCI), Cloud Hypervisor's virtio devices are
// PCI, so the guest interface name depends on udev's predictable-naming
// rules unless disabled — net.ifnames=0/biosdevname=0 keeps it "eth0", which
// the ip= parameter (appended at deploy time) and the rest of onctl assume.
// Cloud Hypervisor also has no Firecracker-style "is_root_device" flag: the
// boot disk is just the first entry in the disks list, so root=/dev/vda is
// required explicitly.
const defaultCHKernelArgs = "console=ttyS0 reboot=k panic=1 root=/dev/vda rw net.ifnames=0 biosdevname=0"

const (
	chStatusRunning = "running"
	// chStatusDead marks a microVM whose cloud-hypervisor process is no
	// longer alive even though its persisted metadata was last written as
	// "running" (e.g. the host rebooted, or the process crashed). There is
	// no pause/resume support for this provider, so unlike fc there is no
	// "paused" status to distinguish this from.
	chStatusDead = "dead"
)

// CHConfig holds configuration for the local Cloud Hypervisor microVM
// provider.
type CHConfig struct {
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
	// CIDR is the bridge's address and subnet (e.g. "172.17.0.1/24"). The
	// bridge address is used as the gateway for microVMs.
	CIDR string
	// Username is the SSH user configured in the rootfs image.
	Username string
	// BinPath is the path to the cloud-hypervisor binary.
	BinPath string
	// StateDir is the directory onctl stores microVM state under
	// (default /opt/ch).
	StateDir string
	// OS selects the guest boot path: "linux" (default) boots KernelImage
	// as a direct Linux kernel; "windows" boots it as UEFI firmware
	// (CLOUDHV.fd) instead. See chOSWindows.
	OS string
}

// chOSWindows is the CHConfig.OS / chVM.OS value selecting the Windows
// (UEFI firmware, no kernel cmdline, NoCloud seed ISO) boot path over the
// default direct-kernel-boot Linux one. This integration has not been
// exercised against a real cloud-hypervisor binary or a real Windows guest
// (see docs/windows-ch.md) — treat it as unverified until someone has.
const chOSWindows = "windows"

// CHVMConfig describes a microVM to be configured and booted by a CHProcess.
type CHVMConfig struct {
	// KernelImage is a direct Linux kernel path (Firmware false) or a UEFI
	// firmware path such as CLOUDHV.fd (Firmware true) — see PayloadConfig
	// in internal/providerch/common.go for how this maps onto the wire.
	KernelImage string
	KernelArgs  string
	// Firmware selects payload.firmware over payload.kernel/cmdline for
	// KernelImage/KernelArgs — required for Windows guests, which have no
	// direct-kernel-boot path.
	Firmware bool
	// KvmHyperv enables Hyper-V enlightenments (cpus.kvm_hyperv), required
	// for Windows guests.
	KvmHyperv  bool
	RootfsPath string
	// SeedDiskPath, if set, is attached as a second, read-only disk — a
	// NoCloud-format seed ISO for cloudbase-init to apply hostname/SSH-key
	// configuration to a Windows guest that onctl cannot write into
	// directly (see WindowsGuestPreparer).
	SeedDiskPath string
	VCPUCount    int64
	MemSizeMib   int64
	TapDevice    string
	MacAddress   string
}

// WindowsGuestPreparer prepares a per-VM Windows disk and its accompanying
// NoCloud seed ISO. Unlike RootfsPreparer (ext4/debugfs-based), it never
// writes into the guest disk itself — Windows disks are NTFS, which onctl
// has no in-process tooling to modify — so all guest customization (
// hostname, SSH public key) is delivered via the seed ISO instead, for
// cloudbase-init (which must be pre-installed in the base image) to apply
// at boot.
type WindowsGuestPreparer interface {
	// PrepareDisk creates destPath as a plain copy of baseImage (no
	// modification), mirroring RootfsPreparer.Prepare's copy-not-mutate
	// contract for the base image.
	PrepareDisk(baseImage, destPath string) error
	// BuildSeedISO creates destPath as a NoCloud-format ISO9660 volume
	// (label "cidata") containing meta-data/user-data set from hostname,
	// sshPublicKey and username.
	BuildSeedISO(destPath, hostname, sshPublicKey, username string) error
}

// DHCPManager hands out DHCP leases on a bridge for guests that can't be
// configured via a Linux kernel cmdline (i.e. Windows). Only used on the
// chOSWindows path — the default Linux path keeps using the existing
// ip=... kernel cmdline and never touches this.
type DHCPManager interface {
	// EnsureDHCP starts (or confirms already running) a DHCP server scoped
	// to bridge/cidr, idempotent like NetworkManager.EnsureBridge.
	EnsureDHCP(bridge, cidr string) error
	// AddReservation hands mac a fixed lease of ip, so a guest's address
	// always matches what onctl's metadata says it is.
	AddReservation(mac, ip string) error
	// RemoveReservation removes a reservation added by AddReservation.
	RemoveReservation(mac string) error
}

// CHProcess starts and stops cloud-hypervisor VMM processes.
type CHProcess interface {
	// Start launches a cloud-hypervisor process bound to socketPath,
	// configures it per cfg over its API socket and boots it. It returns the
	// PID of the running process.
	Start(socketPath string, cfg CHVMConfig, logFile string) (pid int, err error)
	// Stop terminates the cloud-hypervisor process with the given PID.
	Stop(pid int) error
	// IsRunning reports whether a process with the given PID is alive.
	IsRunning(pid int) bool
	// Owns reports whether the process with the given PID is the
	// cloud-hypervisor VMM bound to socketPath, guarding against a persisted
	// PID having been reused by an unrelated process after a VMM exit or
	// host reboot.
	Owns(pid int, socketPath string) bool
}

// ProviderCH manages local Cloud Hypervisor microVMs as onctl-managed VMs.
// Unlike the other providers, there is no remote API: state is tracked on
// disk under Config.StateDir, and Net/Process/Rootfs are local-host
// operations. There is no pause/resume/snapshot support (see fc for that).
type ProviderCH struct {
	Config  CHConfig
	Process CHProcess
	Net     NetworkManager
	Rootfs  RootfsPreparer
	// WindowsGuest and DHCP are only used on the chOSWindows path; nil is
	// fine for an all-Linux setup (the zero value of CHConfig.OS).
	WindowsGuest WindowsGuestPreparer
	DHCP         DHCPManager
}

// chVM is the on-disk metadata persisted for each managed microVM.
type chVM struct {
	Name        string `json:"name"`
	PID         int    `json:"pid"`
	SocketPath  string `json:"socketPath"`
	TapDevice   string `json:"tapDevice"`
	IPAddress   string `json:"ipAddress"`
	MacAddress  string `json:"macAddress"`
	VCPUCount   int64  `json:"vcpuCount"`
	MemSizeMib  int64  `json:"memSizeMib"`
	Status      string `json:"status"`
	KernelImage string `json:"kernelImage"`
	RootfsPath  string `json:"rootfsPath"`
	// OS is chOSWindows for a Windows guest, empty/"linux" otherwise.
	OS string `json:"os,omitempty"`
	// SeedDiskPath is the NoCloud seed ISO path for a Windows guest (see
	// WindowsGuestPreparer), empty for a Linux guest.
	SeedDiskPath string    `json:"seedDiskPath,omitempty"`
	CreatedAt    time.Time `json:"createdAt"`
}

func (p ProviderCH) vmDir(name string) string {
	return filepath.Join(p.Config.StateDir, "vms", name)
}

func (p ProviderCH) metadataPath(name string) string {
	return filepath.Join(p.vmDir(name), "metadata.json")
}

func loadCHMetadata(path string) (chVM, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return chVM{}, err
	}
	var vm chVM
	if err := json.Unmarshal(data, &vm); err != nil {
		return chVM{}, err
	}
	return vm, nil
}

func saveCHMetadata(path string, vm chVM) error {
	data, err := json.MarshalIndent(vm, "", "  ")
	if err != nil {
		return err
	}
	// 0644: metadata holds no secrets (paths, PID, IP/MAC, status) and must
	// stay readable by other local accounts so `onctl ls` sees VMs deployed
	// by root under the shared StateDir.
	return os.WriteFile(path, data, 0644)
}

// isAlive reports whether vm's cloud-hypervisor process is actually running
// and still bound to vm's socket, guarding against a persisted PID that is
// either dead or was reused by an unrelated process after a VMM exit or
// host reboot.
func (p ProviderCH) isAlive(vm chVM) bool {
	return vm.PID > 0 && p.Process.IsRunning(vm.PID) && p.Process.Owns(vm.PID, vm.SocketPath)
}

// loadAndReconcile loads a microVM's on-disk metadata and, if its status
// claims the process is running but the cloud-hypervisor process is no
// longer alive, rewrites the persisted status to "dead". See
// ProviderFC.loadAndReconcile for the full rationale (identical here, minus
// the paused-status exclusion, which doesn't apply to this provider).
func (p ProviderCH) loadAndReconcile(path string) (chVM, error) {
	vm, err := loadCHMetadata(path)
	if err != nil {
		return chVM{}, err
	}
	if vm.Status != chStatusDead && !p.isAlive(vm) {
		vm.Status = chStatusDead
		if err := saveCHMetadata(path, vm); err != nil {
			log.Println("[DEBUG] failed to persist reconciled status for " + vm.Name + ": " + err.Error())
		}
	}
	return vm, nil
}

func mapCHVM(vm chVM) Vm {
	return Vm{
		Provider:  "ch",
		ID:        vm.Name,
		Name:      vm.Name,
		IP:        vm.IPAddress,
		Type:      fmt.Sprintf("%dvcpu-%dmb", vm.VCPUCount, vm.MemSizeMib),
		Image:     vm.RootfsPath,
		Status:    vm.Status,
		CreatedAt: vm.CreatedAt,
	}
}

// parseCHType parses a "<vcpu>vcpu-<mem>mb" type string (e.g.
// "2vcpu-1024mb"). If t is empty or doesn't match, the provided defaults are
// returned.
func parseCHType(t string, defaultVCPU, defaultMem int64) (vcpu, mem int64) {
	if t == "" {
		return defaultVCPU, defaultMem
	}
	var v, m int64
	if _, err := fmt.Sscanf(strings.ToLower(t), "%dvcpu-%dmb", &v, &m); err == nil && v > 0 && m > 0 {
		return v, m
	}
	return defaultVCPU, defaultMem
}

// chTapName derives a deterministic, <=15 char TAP device name from the VM
// name (the Linux interface name length limit).
func chTapName(vmName string) string {
	sum := md5.Sum([]byte(vmName))
	return "ch" + fmt.Sprintf("%x", sum)[:13]
}

// chMAC derives a deterministic, locally-administered MAC address from the
// VM name. Unlike fcMAC, the second octet can't spell out the provider name
// in hex ("CH" isn't a valid hex byte — H isn't a hex digit), so it uses a
// fixed placeholder octet instead. Lowercase throughout (not just "c4"):
// dnsmasq's --dhcp-hostsfile reservation matching (used on the Windows
// path, see DHCPManager) is case-sensitive against the lowercase-normalized
// MAC a DHCP client actually sends, so a mixed-case address here silently
// never matches its own reservation and the guest gets a random pool
// address instead of the one onctl's metadata says it has.
func chMAC(vmName string) string {
	sum := md5.Sum([]byte(vmName))
	return fmt.Sprintf("02:c4:%02x:%02x:%02x:%02x", sum[0], sum[1], sum[2], sum[3])
}

// usedIPs returns the set of IP addresses already assigned to managed microVMs.
func (p ProviderCH) usedIPs() (map[string]bool, error) {
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

// allocateAndReserveIP picks the next free IPv4 address in cidr and
// immediately persists it as a metadata.json stub for name, all under an
// exclusive lock. See ProviderFC.allocateAndReserveIP for the full
// rationale (identical here).
func (p ProviderCH) allocateAndReserveIP(name, cidr string) (string, error) {
	lockPath := filepath.Join(p.Config.StateDir, "vms", ".ip.lock")
	if err := os.MkdirAll(filepath.Dir(lockPath), 0755); err != nil {
		return "", err
	}
	lockFile, err := os.OpenFile(lockPath, os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return "", fmt.Errorf("failed to open IP allocation lock %q: %w", lockPath, err)
	}
	defer func() { _ = lockFile.Close() }()
	if err := tools.Flock(lockFile); err != nil {
		return "", fmt.Errorf("failed to lock %q: %w", lockPath, err)
	}
	defer func() { _ = tools.Funlock(lockFile) }()

	used, err := p.usedIPs()
	if err != nil {
		return "", err
	}
	ip, err := allocateFCIP(cidr, used)
	if err != nil {
		return "", err
	}
	if err := saveCHMetadata(p.metadataPath(name), chVM{Name: name, IPAddress: ip, CreatedAt: time.Now()}); err != nil {
		return "", fmt.Errorf("failed to reserve IP %s: %w", ip, err)
	}
	return ip, nil
}

// listAll returns the metadata for every managed microVM, regardless of status.
func (p ProviderCH) listAll() ([]chVM, error) {
	root := filepath.Join(p.Config.StateDir, "vms")
	entries, err := os.ReadDir(root)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	vms := make([]chVM, 0, len(entries))
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
// its record is stale (the cloud-hypervisor process is no longer running,
// e.g. after a host reboot), the stale state is cleared and a fresh microVM
// is deployed in its place.
func (p ProviderCH) Deploy(server Vm) (Vm, error) {
	if server.Name == "" {
		return Vm{}, errors.New("vm name is required")
	}

	if existing, err := p.loadAndReconcile(p.metadataPath(server.Name)); err == nil {
		if existing.Status != chStatusDead {
			log.Println("[DEBUG] microVM " + server.Name + " already exists")
			return mapCHVM(existing), nil
		}
		log.Println("[DEBUG] microVM " + server.Name + " has a stale record (cloud-hypervisor process is no longer running); recreating")
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
		return Vm{}, errors.New("ch.kernelImage and ch.rootfsImage must be configured (see onctl init)")
	}

	dir := p.vmDir(server.Name)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return Vm{}, fmt.Errorf("failed to create vm directory: %w", err)
	}

	vcpu, mem := parseCHType(server.Type, p.Config.VCPUCount, p.Config.MemSizeMib)

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

	isWindows := p.Config.OS == chOSWindows

	var rootfsPath, seedDiskPath string
	if isWindows {
		if p.WindowsGuest == nil {
			_ = os.RemoveAll(dir)
			return Vm{}, errors.New("ch.os=windows requires a WindowsGuestPreparer to be configured (internal error)")
		}
		rootfsPath = filepath.Join(dir, "disk.raw")
		if err := p.WindowsGuest.PrepareDisk(rootfsImage, rootfsPath); err != nil {
			_ = os.RemoveAll(dir)
			return Vm{}, fmt.Errorf("failed to prepare Windows disk: %w", err)
		}
		seedDiskPath = filepath.Join(dir, "seed.iso")
		if err := p.WindowsGuest.BuildSeedISO(seedDiskPath, sanitizeGuestHostname(server.Name), sshPublicKey, username); err != nil {
			_ = os.RemoveAll(dir)
			return Vm{}, fmt.Errorf("failed to build seed ISO: %w", err)
		}
	} else {
		rootfsPath = filepath.Join(dir, "rootfs.ext4")
		if err := p.Rootfs.Prepare(rootfsImage, rootfsPath, sanitizeGuestHostname(server.Name), sshPublicKey, username); err != nil {
			_ = os.RemoveAll(dir)
			return Vm{}, fmt.Errorf("failed to prepare rootfs: %w", err)
		}
	}

	bridge := p.Config.Bridge
	if bridge == "" {
		bridge = "chbr0"
	}
	cidr := p.Config.CIDR
	if cidr == "" {
		cidr = "172.17.0.1/24"
	}
	if err := p.Net.EnsureBridge(bridge, cidr); err != nil {
		_ = os.RemoveAll(dir)
		return Vm{}, fmt.Errorf("failed to set up bridge %q: %w", bridge, err)
	}

	tapDevice := chTapName(server.Name)
	if err := p.Net.CreateTap(tapDevice, bridge); err != nil {
		_ = os.RemoveAll(dir)
		return Vm{}, fmt.Errorf("failed to create tap device: %w", err)
	}

	ip, err := p.allocateAndReserveIP(server.Name, cidr)
	if err != nil {
		_ = p.Net.DeleteTap(tapDevice)
		_ = os.RemoveAll(dir)
		return Vm{}, err
	}
	mac := chMAC(server.Name)

	var kernelArgs string
	if isWindows {
		// Windows has no Linux kernel cmdline to parse networking from;
		// the guest gets its address via DHCP instead (see DHCPManager).
		// The seed ISO's meta-data still carries the hostname and SSH key
		// for cloudbase-init to apply.
		if p.DHCP == nil {
			_ = p.Net.DeleteTap(tapDevice)
			_ = os.RemoveAll(dir)
			return Vm{}, errors.New("ch.os=windows requires a DHCPManager to be configured (internal error)")
		}
		if err := p.DHCP.EnsureDHCP(bridge, cidr); err != nil {
			_ = p.Net.DeleteTap(tapDevice)
			_ = os.RemoveAll(dir)
			return Vm{}, fmt.Errorf("failed to set up DHCP on %q: %w", bridge, err)
		}
		if err := p.DHCP.AddReservation(mac, ip); err != nil {
			_ = p.Net.DeleteTap(tapDevice)
			_ = os.RemoveAll(dir)
			return Vm{}, fmt.Errorf("failed to reserve DHCP lease for %s: %w", ip, err)
		}
	} else {
		gateway, mask, err := bridgeGatewayAndMask(cidr)
		if err != nil {
			_ = p.Net.DeleteTap(tapDevice)
			_ = os.RemoveAll(dir)
			return Vm{}, err
		}
		kernelArgs = strings.TrimSpace(p.Config.KernelArgs)
		if kernelArgs == "" {
			kernelArgs = defaultCHKernelArgs
		}
		kernelArgs = fmt.Sprintf("%s ip=%s::%s:%s::eth0:off", kernelArgs, ip, gateway, mask)
	}

	socketPath := filepath.Join(dir, "ch.sock")
	logFile := filepath.Join(dir, "ch.log")

	pid, err := p.Process.Start(socketPath, CHVMConfig{
		KernelImage:  kernelImage,
		KernelArgs:   kernelArgs,
		Firmware:     isWindows,
		KvmHyperv:    isWindows,
		RootfsPath:   rootfsPath,
		SeedDiskPath: seedDiskPath,
		VCPUCount:    vcpu,
		MemSizeMib:   mem,
		TapDevice:    tapDevice,
		MacAddress:   mac,
	}, logFile)
	if err != nil {
		if isWindows {
			if rmErr := p.DHCP.RemoveReservation(mac); rmErr != nil {
				log.Println("[DEBUG] failed to remove DHCP reservation for " + mac + ": " + rmErr.Error())
			}
		}
		_ = p.Net.DeleteTap(tapDevice)
		_ = os.RemoveAll(dir)
		return Vm{}, fmt.Errorf("failed to start microVM: %w", err)
	}

	vm := chVM{
		Name:         server.Name,
		PID:          pid,
		SocketPath:   socketPath,
		TapDevice:    tapDevice,
		IPAddress:    ip,
		MacAddress:   mac,
		VCPUCount:    vcpu,
		MemSizeMib:   mem,
		Status:       chStatusRunning,
		KernelImage:  kernelImage,
		RootfsPath:   rootfsPath,
		OS:           p.Config.OS,
		SeedDiskPath: seedDiskPath,
		CreatedAt:    time.Now(),
	}
	if err := saveCHMetadata(p.metadataPath(server.Name), vm); err != nil {
		return Vm{}, fmt.Errorf("microVM started but failed to persist metadata: %w", err)
	}
	return mapCHVM(vm), nil
}

// Destroy stops the microVM (if running), removes its TAP device and deletes
// its on-disk state.
func (p ProviderCH) Destroy(server Vm) error {
	if server.Name == "" {
		return errors.New("vm name is required")
	}
	vm, err := loadCHMetadata(p.metadataPath(server.Name))
	if err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("no microVM found with name %q", server.Name)
		}
		return err
	}
	if vm.PID > 0 && p.Process.IsRunning(vm.PID) {
		if !p.Process.Owns(vm.PID, vm.SocketPath) {
			log.Println("[DEBUG] persisted PID " + fmt.Sprint(vm.PID) + " for " + server.Name + " is no longer the cloud-hypervisor process for this microVM; skipping stop")
		} else if err := p.Process.Stop(vm.PID); err != nil {
			return fmt.Errorf("failed to stop microVM: %w", err)
		}
	}
	if vm.TapDevice != "" {
		if err := p.Net.DeleteTap(vm.TapDevice); err != nil {
			log.Println("[DEBUG] failed to delete tap device " + vm.TapDevice + ": " + err.Error())
		}
	}
	if vm.OS == chOSWindows && vm.MacAddress != "" && p.DHCP != nil {
		if err := p.DHCP.RemoveReservation(vm.MacAddress); err != nil {
			log.Println("[DEBUG] failed to remove DHCP reservation for " + vm.MacAddress + ": " + err.Error())
		}
	}
	// The seed ISO (if any) lives under vmDir alongside everything else
	// removed below; no separate cleanup needed.
	return os.RemoveAll(p.vmDir(server.Name))
}

// Pause is not supported for the ch provider: there is no snapshot/restore
// support (see fc for that). Use 'onctl destroy' instead.
func (p ProviderCH) Pause(_ Vm, _ bool) error {
	return errChUnsupported
}

// Resume is not supported for the ch provider (see Pause).
func (p ProviderCH) Resume(_ Vm) (Vm, error) {
	return Vm{}, errChUnsupported
}

var errChUnsupported = errors.New("not supported for the ch provider: pause/resume requires snapshot support, which this provider does not implement; use 'onctl destroy' and 'onctl create' instead")

// List returns all managed microVMs. A microVM whose cloud-hypervisor
// process has died out-of-band (e.g. a host reboot) is included with status
// "dead" rather than the stale "running" last written to disk.
func (p ProviderCH) List() (VmList, error) {
	all, err := p.listAll()
	if err != nil {
		return VmList{}, err
	}
	var list VmList
	for _, vm := range all {
		list.List = append(list.List, mapCHVM(vm))
	}
	return list, nil
}

// ListPaused always returns an empty list: this provider has no pause support.
func (p ProviderCH) ListPaused() (VmList, error) {
	return VmList{}, nil
}

// GetByName returns the named microVM, or a zero-value Vm if it doesn't exist.
func (p ProviderCH) GetByName(serverName string) (Vm, error) {
	vm, err := p.loadAndReconcile(p.metadataPath(serverName))
	if err != nil {
		if os.IsNotExist(err) {
			return Vm{}, nil
		}
		return Vm{}, err
	}
	return mapCHVM(vm), nil
}

// CreateSSHKey validates the given public key file and returns its absolute
// path. Unlike the remote providers there is no key registry to upload to:
// Deploy reads the key directly from this path and injects it into the
// microVM's rootfs.
func (p ProviderCH) CreateSSHKey(publicKeyFile string) (keyID string, err error) {
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
func (p ProviderCH) SSHInto(serverName string, port int, privateKey string, command []string) {
	vm, err := p.GetByName(serverName)
	if err != nil || vm.Name == "" {
		log.Fatalln("no microVM found with name " + serverName)
	}
	if vm.Status == chStatusDead {
		log.Fatalln("microVM " + serverName + " is not running (its cloud-hypervisor process is gone, e.g. after a host reboot) — destroy and recreate it")
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
