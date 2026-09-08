package cloud

import (
	"errors"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fakeCHProcess is a test double for CHProcess.
type fakeCHProcess struct {
	pid        int
	startCalls int
	startErr   error
	stopCalls  []int
	running    map[int]bool
	notOwned   map[int]bool
	// lastCfg is the CHVMConfig passed to the most recent Start call.
	lastCfg CHVMConfig
}

func (f *fakeCHProcess) Start(_ string, cfg CHVMConfig, _ string) (int, error) {
	f.startCalls++
	f.lastCfg = cfg
	if f.startErr != nil {
		return 0, f.startErr
	}
	if f.running == nil {
		f.running = map[int]bool{}
	}
	f.running[f.pid] = true
	return f.pid, nil
}

func (f *fakeCHProcess) Stop(pid int) error {
	f.stopCalls = append(f.stopCalls, pid)
	delete(f.running, pid)
	return nil
}

func (f *fakeCHProcess) IsRunning(pid int) bool {
	return f.running[pid]
}

func (f *fakeCHProcess) Owns(pid int, _ string) bool {
	return !f.notOwned[pid]
}

// newTestCHProvider reuses fakeNetworkManager and fakeRootfsPreparer from
// fc_test.go: both are generic (NetworkManager/RootfsPreparer have nothing
// Firecracker-specific about them), so there is no need for ch-specific
// duplicates.
func newTestCHProvider(t *testing.T) (ProviderCH, *fakeCHProcess, *fakeNetworkManager, *fakeRootfsPreparer) {
	t.Helper()
	proc := &fakeCHProcess{pid: 12345}
	net := &fakeNetworkManager{}
	rootfs := &fakeRootfsPreparer{}
	p := ProviderCH{
		Config: CHConfig{
			KernelImage: "/images/vmlinux",
			RootfsImage: "/images/rootfs.ext4",
			VCPUCount:   1,
			MemSizeMib:  512,
			Bridge:      "chbr0",
			CIDR:        "172.17.0.1/24",
			Username:    "root",
			StateDir:    t.TempDir(),
		},
		Process: proc,
		Net:     net,
		Rootfs:  rootfs,
	}
	return p, proc, net, rootfs
}

func TestParseCHType(t *testing.T) {
	tests := []struct {
		in       string
		wantVCPU int64
		wantMem  int64
	}{
		{"", 1, 512},
		{"2vcpu-1024mb", 2, 1024},
		{"4VCPU-2048MB", 4, 2048},
		{"garbage", 1, 512},
		{"0vcpu-512mb", 1, 512},
	}
	for _, tt := range tests {
		vcpu, mem := parseCHType(tt.in, 1, 512)
		assert.Equal(t, tt.wantVCPU, vcpu, "vcpu for %q", tt.in)
		assert.Equal(t, tt.wantMem, mem, "mem for %q", tt.in)
	}
}

func TestCHTapName(t *testing.T) {
	name := chTapName("my-test-vm")
	assert.LessOrEqual(t, len(name), 15)
	assert.True(t, strings.HasPrefix(name, "ch"))
	assert.Equal(t, name, chTapName("my-test-vm"))
	assert.NotEqual(t, name, chTapName("other-vm"))
}

func TestCHMAC(t *testing.T) {
	mac := chMAC("my-test-vm")
	// Lowercase throughout, not just valid hex: dnsmasq's --dhcp-hostsfile
	// reservation matching is case-sensitive against the lowercase MAC a
	// DHCP client actually sends (see chMAC's doc comment).
	assert.Regexp(t, `^02:c4:[0-9a-f]{2}:[0-9a-f]{2}:[0-9a-f]{2}:[0-9a-f]{2}$`, mac)
	assert.Equal(t, mac, chMAC("my-test-vm"))
	// Regression check for a bug caught in review: every octet must be
	// valid hex, or Cloud Hypervisor rejects the vm.create payload outright.
	_, err := net.ParseMAC(mac)
	assert.NoError(t, err)
}

func TestProviderCH_Deploy(t *testing.T) {
	p, proc, netMgr, rootfs := newTestCHProvider(t)
	pubKeyFile := writeTestPublicKey(t)

	vm, err := p.Deploy(Vm{Name: "test-vm", SSHKeyID: pubKeyFile})
	require.NoError(t, err)

	assert.Equal(t, "ch", vm.Provider)
	assert.Equal(t, "test-vm", vm.Name)
	assert.Equal(t, "running", vm.Status)
	assert.Equal(t, "1vcpu-512mb", vm.Type)
	assert.Equal(t, "172.17.0.2", vm.IP)
	assert.Equal(t, 1, proc.startCalls)
	assert.Equal(t, []string{"chbr0"}, netMgr.bridges)
	assert.Equal(t, []string{chTapName("test-vm")}, netMgr.taps)
	assert.Len(t, rootfs.calls, 1)
	assert.Equal(t, []string{"test-vm"}, rootfs.hostnames)

	meta, err := loadCHMetadata(p.metadataPath("test-vm"))
	require.NoError(t, err)
	assert.Equal(t, 12345, meta.PID)
	assert.Equal(t, "172.17.0.2", meta.IPAddress)
	assert.Equal(t, chStatusRunning, meta.Status)
}

func TestProviderCH_Deploy_CustomType(t *testing.T) {
	p, _, _, _ := newTestCHProvider(t)
	vm, err := p.Deploy(Vm{Name: "big-vm", Type: "2vcpu-1024mb"})
	require.NoError(t, err)
	assert.Equal(t, "2vcpu-1024mb", vm.Type)
}

func TestProviderCH_Deploy_Idempotent(t *testing.T) {
	p, proc, _, _ := newTestCHProvider(t)

	_, err := p.Deploy(Vm{Name: "test-vm"})
	require.NoError(t, err)
	assert.Equal(t, 1, proc.startCalls)

	vm2, err := p.Deploy(Vm{Name: "test-vm"})
	require.NoError(t, err)
	assert.Equal(t, "test-vm", vm2.Name)
	assert.Equal(t, 1, proc.startCalls)
}

func TestProviderCH_Deploy_MissingImages(t *testing.T) {
	p, _, _, _ := newTestCHProvider(t)
	p.Config.KernelImage = ""
	_, err := p.Deploy(Vm{Name: "test-vm"})
	assert.Error(t, err)
}

func TestProviderCH_Deploy_StartFailureCleansUp(t *testing.T) {
	p, proc, netMgr, _ := newTestCHProvider(t)
	proc.startErr = errors.New("boom")

	_, err := p.Deploy(Vm{Name: "test-vm"})
	assert.Error(t, err)
	assert.Contains(t, netMgr.deleted, chTapName("test-vm"))

	_, statErr := os.Stat(p.vmDir("test-vm"))
	assert.True(t, os.IsNotExist(statErr))
}

// TestProviderCH_List_ReconcilesDeadProcess verifies that List() does not
// keep reporting a microVM as running once its cloud-hypervisor process is
// gone out-of-band (e.g. the host rebooted).
func TestProviderCH_List_ReconcilesDeadProcess(t *testing.T) {
	p, proc, _, _ := newTestCHProvider(t)
	_, err := p.Deploy(Vm{Name: "test-vm"})
	require.NoError(t, err)

	running, err := p.List()
	require.NoError(t, err)
	require.Len(t, running.List, 1)
	assert.Equal(t, chStatusRunning, running.List[0].Status)

	// Simulate a host reboot: the cloud-hypervisor process is gone.
	proc.running = map[int]bool{}

	running, err = p.List()
	require.NoError(t, err)
	require.Len(t, running.List, 1, "a dead microVM is still surfaced, just with an honest status")
	assert.Equal(t, chStatusDead, running.List[0].Status)

	meta, err := loadCHMetadata(p.metadataPath("test-vm"))
	require.NoError(t, err)
	assert.Equal(t, chStatusDead, meta.Status, "status should self-heal on disk")
}

// TestProviderCH_Deploy_RecreatesStaleRecord verifies that Deploy() does not
// silently no-op when a same-named microVM's record exists but its process
// is dead — it should clean up the stale state and boot a fresh microVM.
func TestProviderCH_Deploy_RecreatesStaleRecord(t *testing.T) {
	p, proc, netMgr, _ := newTestCHProvider(t)
	_, err := p.Deploy(Vm{Name: "test-vm"})
	require.NoError(t, err)
	assert.Equal(t, 1, proc.startCalls)

	proc.running = map[int]bool{}

	vm, err := p.Deploy(Vm{Name: "test-vm"})
	require.NoError(t, err)
	assert.Equal(t, 2, proc.startCalls, "Deploy should recreate a stale microVM instead of no-op'ing")
	assert.Equal(t, chStatusRunning, vm.Status)
	assert.Contains(t, netMgr.deleted, chTapName("test-vm"), "the stale tap device should be cleaned up")
}

func TestProviderCH_GetByName_ReconcilesDeadProcess(t *testing.T) {
	p, proc, _, _ := newTestCHProvider(t)
	_, err := p.Deploy(Vm{Name: "test-vm"})
	require.NoError(t, err)

	proc.running = map[int]bool{}

	vm, err := p.GetByName("test-vm")
	require.NoError(t, err)
	assert.Equal(t, chStatusDead, vm.Status)
}

func TestProviderCH_GetByName_NotFound(t *testing.T) {
	p, _, _, _ := newTestCHProvider(t)
	vm, err := p.GetByName("nope")
	require.NoError(t, err)
	assert.Equal(t, Vm{}, vm)
}

func TestProviderCH_Destroy(t *testing.T) {
	p, proc, netMgr, _ := newTestCHProvider(t)
	_, err := p.Deploy(Vm{Name: "test-vm"})
	require.NoError(t, err)

	require.NoError(t, p.Destroy(Vm{Name: "test-vm"}))
	assert.Contains(t, proc.stopCalls, 12345)
	assert.Contains(t, netMgr.deleted, chTapName("test-vm"))

	_, statErr := os.Stat(p.vmDir("test-vm"))
	assert.True(t, os.IsNotExist(statErr))
}

// TestProviderCH_Destroy_StalePID verifies that Destroy does not signal a
// running process whose PID was persisted for this microVM but no longer
// belongs to it (e.g. reused after a host reboot).
func TestProviderCH_Destroy_StalePID(t *testing.T) {
	p, proc, netMgr, _ := newTestCHProvider(t)
	_, err := p.Deploy(Vm{Name: "test-vm"})
	require.NoError(t, err)
	proc.notOwned = map[int]bool{12345: true}

	require.NoError(t, p.Destroy(Vm{Name: "test-vm"}))
	assert.NotContains(t, proc.stopCalls, 12345)
	assert.Contains(t, netMgr.deleted, chTapName("test-vm"))

	_, statErr := os.Stat(p.vmDir("test-vm"))
	assert.True(t, os.IsNotExist(statErr))
}

func TestProviderCH_Destroy_NotFound(t *testing.T) {
	p, _, _, _ := newTestCHProvider(t)
	assert.Error(t, p.Destroy(Vm{Name: "nope"}))
}

func TestProviderCH_PauseResumeUnsupported(t *testing.T) {
	p, _, _, _ := newTestCHProvider(t)
	assert.ErrorIs(t, p.Pause(Vm{Name: "test-vm"}, true), errChUnsupported)
	_, err := p.Resume(Vm{Name: "test-vm"})
	assert.ErrorIs(t, err, errChUnsupported)

	paused, err := p.ListPaused()
	require.NoError(t, err)
	assert.Empty(t, paused.List)
}

func TestProviderCH_CreateSSHKey(t *testing.T) {
	p, _, _, _ := newTestCHProvider(t)
	keyFile := writeTestPublicKey(t)

	keyID, err := p.CreateSSHKey(keyFile)
	require.NoError(t, err)
	assert.True(t, filepath.IsAbs(keyID))
}

func TestProviderCH_CreateSSHKey_Invalid(t *testing.T) {
	p, _, _, _ := newTestCHProvider(t)
	keyFile := filepath.Join(t.TempDir(), "bad.pub")
	require.NoError(t, os.WriteFile(keyFile, []byte("not a key"), 0644))

	_, err := p.CreateSSHKey(keyFile)
	assert.Error(t, err)
}

// fakeWindowsGuestPreparer is a test double for WindowsGuestPreparer.
type fakeWindowsGuestPreparer struct {
	diskCalls int
	isoCalls  int
	hostnames []string
}

func (f *fakeWindowsGuestPreparer) PrepareDisk(_, destPath string) error {
	f.diskCalls++
	return os.WriteFile(destPath, []byte("fake-disk"), 0600)
}

func (f *fakeWindowsGuestPreparer) BuildSeedISO(destPath, hostname, _, _ string) error {
	f.isoCalls++
	f.hostnames = append(f.hostnames, hostname)
	return os.WriteFile(destPath, []byte("fake-seed"), 0600)
}

// fakeDHCPManager is a test double for DHCPManager.
type fakeDHCPManager struct {
	ensureCalls  []string // bridge names
	reservations map[string]string
	removed      []string
}

func (f *fakeDHCPManager) EnsureDHCP(bridge, _ string) error {
	f.ensureCalls = append(f.ensureCalls, bridge)
	return nil
}

func (f *fakeDHCPManager) AddReservation(mac, ip string) error {
	if f.reservations == nil {
		f.reservations = map[string]string{}
	}
	f.reservations[mac] = ip
	return nil
}

func (f *fakeDHCPManager) RemoveReservation(mac string) error {
	f.removed = append(f.removed, mac)
	delete(f.reservations, mac)
	return nil
}

// newTestCHProviderWindows is newTestCHProvider's counterpart for the
// ch.os=windows path.
func newTestCHProviderWindows(t *testing.T) (ProviderCH, *fakeCHProcess, *fakeNetworkManager, *fakeWindowsGuestPreparer, *fakeDHCPManager) {
	t.Helper()
	proc := &fakeCHProcess{pid: 12345}
	net := &fakeNetworkManager{}
	winGuest := &fakeWindowsGuestPreparer{}
	dhcp := &fakeDHCPManager{}
	p := ProviderCH{
		Config: CHConfig{
			KernelImage: "/images/CLOUDHV.fd",
			RootfsImage: "/images/windows.raw",
			VCPUCount:   2,
			MemSizeMib:  4096,
			Bridge:      "chbr0",
			CIDR:        "172.17.0.1/24",
			Username:    "Administrator",
			StateDir:    t.TempDir(),
			OS:          chOSWindows,
		},
		Process:      proc,
		Net:          net,
		WindowsGuest: winGuest,
		DHCP:         dhcp,
	}
	return p, proc, net, winGuest, dhcp
}

// TestProviderCH_Deploy_Windows verifies the ch.os=windows boot path: no
// Linux kernel cmdline is generated, CHVMConfig.Firmware/KvmHyperv are set,
// a seed ISO is built and attached, and the guest's DHCP reservation is
// added instead of a static ip= kernel argument.
func TestProviderCH_Deploy_Windows(t *testing.T) {
	p, proc, netMgr, winGuest, dhcp := newTestCHProviderWindows(t)
	pubKeyFile := writeTestPublicKey(t)

	vm, err := p.Deploy(Vm{Name: "win-vm", SSHKeyID: pubKeyFile})
	require.NoError(t, err)

	assert.Equal(t, "running", vm.Status)
	assert.Equal(t, 1, proc.startCalls)
	assert.True(t, proc.lastCfg.Firmware, "windows guests must boot via UEFI firmware, not a direct kernel")
	assert.True(t, proc.lastCfg.KvmHyperv, "windows guests require Hyper-V enlightenments")
	assert.Empty(t, proc.lastCfg.KernelArgs, "windows has no Linux kernel cmdline to parse")
	assert.Equal(t, "/images/CLOUDHV.fd", proc.lastCfg.KernelImage, "reinterpreted as the firmware path")
	assert.NotEmpty(t, proc.lastCfg.SeedDiskPath)

	assert.Equal(t, 1, winGuest.diskCalls)
	assert.Equal(t, 1, winGuest.isoCalls)
	assert.Equal(t, []string{"win-vm"}, winGuest.hostnames)

	assert.Equal(t, []string{"chbr0"}, dhcp.ensureCalls)
	mac := chMAC("win-vm")
	assert.Equal(t, "172.17.0.2", dhcp.reservations[mac])

	assert.Equal(t, []string{"chbr0"}, netMgr.bridges)

	meta, err := loadCHMetadata(p.metadataPath("win-vm"))
	require.NoError(t, err)
	assert.Equal(t, chOSWindows, meta.OS)
	assert.NotEmpty(t, meta.SeedDiskPath)
}

// TestProviderCH_Destroy_Windows verifies Destroy removes the guest's DHCP
// reservation on top of the usual tap/state cleanup.
func TestProviderCH_Destroy_Windows(t *testing.T) {
	p, _, _, _, dhcp := newTestCHProviderWindows(t)
	_, err := p.Deploy(Vm{Name: "win-vm"})
	require.NoError(t, err)

	require.NoError(t, p.Destroy(Vm{Name: "win-vm"}))
	assert.Contains(t, dhcp.removed, chMAC("win-vm"))
}
