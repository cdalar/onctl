package cloud

import (
	"crypto/ed25519"
	"crypto/rand"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/crypto/ssh"
)

// fakeFCProcess is a test double for FCProcess.
type fakeFCProcess struct {
	pid        int
	startCalls int
	startErr   error
	stopCalls  []int
	running    map[int]bool
	notOwned   map[int]bool
}

func (f *fakeFCProcess) Start(_ string, _ FCVMConfig, _ string) (int, error) {
	f.startCalls++
	if f.startErr != nil {
		return 0, f.startErr
	}
	if f.running == nil {
		f.running = map[int]bool{}
	}
	f.running[f.pid] = true
	return f.pid, nil
}

func (f *fakeFCProcess) Stop(pid int) error {
	f.stopCalls = append(f.stopCalls, pid)
	delete(f.running, pid)
	return nil
}

func (f *fakeFCProcess) IsRunning(pid int) bool {
	return f.running[pid]
}

func (f *fakeFCProcess) Owns(pid int, _ string) bool {
	return !f.notOwned[pid]
}

// fakeFCAPI is a test double for FCAPI.
type fakeFCAPI struct {
	states []string
}

func (f *fakeFCAPI) SetState(_ string, state string) error {
	f.states = append(f.states, state)
	return nil
}

// fakeNetworkManager is a test double for NetworkManager.
type fakeNetworkManager struct {
	bridges []string
	taps    []string
	deleted []string
}

func (f *fakeNetworkManager) EnsureBridge(bridge, _ string) error {
	f.bridges = append(f.bridges, bridge)
	return nil
}

func (f *fakeNetworkManager) CreateTap(tapName, _ string) error {
	f.taps = append(f.taps, tapName)
	return nil
}

func (f *fakeNetworkManager) DeleteTap(tapName string) error {
	f.deleted = append(f.deleted, tapName)
	return nil
}

// fakeRootfsPreparer is a test double for RootfsPreparer.
type fakeRootfsPreparer struct {
	calls []string
}

func (f *fakeRootfsPreparer) Prepare(_, destPath, _, _ string) error {
	f.calls = append(f.calls, destPath)
	return os.WriteFile(destPath, []byte("rootfs"), 0600)
}

func newTestFCProvider(t *testing.T) (ProviderFC, *fakeFCProcess, *fakeFCAPI, *fakeNetworkManager, *fakeRootfsPreparer) {
	t.Helper()
	proc := &fakeFCProcess{pid: 12345}
	api := &fakeFCAPI{}
	net := &fakeNetworkManager{}
	rootfs := &fakeRootfsPreparer{}
	p := ProviderFC{
		Config: FCConfig{
			KernelImage: "/images/vmlinux",
			RootfsImage: "/images/rootfs.ext4",
			VCPUCount:   1,
			MemSizeMib:  512,
			Bridge:      "fcbr0",
			CIDR:        "172.16.0.1/24",
			Username:    "root",
			StateDir:    t.TempDir(),
		},
		Process: proc,
		API:     api,
		Net:     net,
		Rootfs:  rootfs,
	}
	return p, proc, api, net, rootfs
}

func generateTestPublicKey(t *testing.T) string {
	t.Helper()
	pub, _, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)
	sshPub, err := ssh.NewPublicKey(pub)
	require.NoError(t, err)
	return string(ssh.MarshalAuthorizedKey(sshPub))
}

func writeTestPublicKey(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "id_ed25519.pub")
	require.NoError(t, os.WriteFile(path, []byte(generateTestPublicKey(t)), 0644))
	return path
}

func TestParseFCType(t *testing.T) {
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
		vcpu, mem := parseFCType(tt.in, 1, 512)
		assert.Equal(t, tt.wantVCPU, vcpu, "vcpu for %q", tt.in)
		assert.Equal(t, tt.wantMem, mem, "mem for %q", tt.in)
	}
}

func TestFCTapName(t *testing.T) {
	name := fcTapName("my-test-vm")
	assert.LessOrEqual(t, len(name), 15)
	assert.True(t, strings.HasPrefix(name, "fc"))
	assert.Equal(t, name, fcTapName("my-test-vm"))
	assert.NotEqual(t, name, fcTapName("other-vm"))
}

func TestFCMAC(t *testing.T) {
	mac := fcMAC("my-test-vm")
	assert.Regexp(t, `^02:FC:[0-9A-Fa-f]{2}:[0-9A-Fa-f]{2}:[0-9A-Fa-f]{2}:[0-9A-Fa-f]{2}$`, mac)
	assert.Equal(t, mac, fcMAC("my-test-vm"))
}

func TestBridgeGatewayAndMask(t *testing.T) {
	gw, mask, err := bridgeGatewayAndMask("172.16.0.1/24")
	require.NoError(t, err)
	assert.Equal(t, "172.16.0.1", gw)
	assert.Equal(t, "255.255.255.0", mask)

	_, _, err = bridgeGatewayAndMask("not-a-cidr")
	assert.Error(t, err)
}

func TestAllocateFCIP(t *testing.T) {
	ip, err := allocateFCIP("172.16.0.1/24", nil)
	require.NoError(t, err)
	assert.Equal(t, "172.16.0.2", ip)

	ip, err = allocateFCIP("172.16.0.1/24", map[string]bool{"172.16.0.2": true, "172.16.0.3": true})
	require.NoError(t, err)
	assert.Equal(t, "172.16.0.4", ip)

	// /30 network: .0 is the network address, .1 is the gateway, .3 is the
	// broadcast address, leaving only .2 usable.
	_, err = allocateFCIP("172.16.0.1/30", map[string]bool{"172.16.0.2": true})
	assert.Error(t, err)
}

func TestProviderFC_Deploy(t *testing.T) {
	p, proc, _, netMgr, rootfs := newTestFCProvider(t)
	pubKeyFile := writeTestPublicKey(t)

	vm, err := p.Deploy(Vm{Name: "test-vm", SSHKeyID: pubKeyFile})
	require.NoError(t, err)

	assert.Equal(t, "fc", vm.Provider)
	assert.Equal(t, "test-vm", vm.Name)
	assert.Equal(t, "running", vm.Status)
	assert.Equal(t, "1vcpu-512mb", vm.Type)
	assert.Equal(t, "172.16.0.2", vm.IP)
	assert.Equal(t, 1, proc.startCalls)
	assert.Equal(t, []string{"fcbr0"}, netMgr.bridges)
	assert.Equal(t, []string{fcTapName("test-vm")}, netMgr.taps)
	assert.Len(t, rootfs.calls, 1)

	meta, err := loadFCMetadata(p.metadataPath("test-vm"))
	require.NoError(t, err)
	assert.Equal(t, 12345, meta.PID)
	assert.Equal(t, "172.16.0.2", meta.IPAddress)
	assert.Equal(t, fcStatusRunning, meta.Status)
}

func TestProviderFC_Deploy_CustomType(t *testing.T) {
	p, _, _, _, _ := newTestFCProvider(t)
	vm, err := p.Deploy(Vm{Name: "big-vm", Type: "2vcpu-1024mb"})
	require.NoError(t, err)
	assert.Equal(t, "2vcpu-1024mb", vm.Type)
}

func TestProviderFC_Deploy_Idempotent(t *testing.T) {
	p, proc, _, _, _ := newTestFCProvider(t)

	_, err := p.Deploy(Vm{Name: "test-vm"})
	require.NoError(t, err)
	assert.Equal(t, 1, proc.startCalls)

	vm2, err := p.Deploy(Vm{Name: "test-vm"})
	require.NoError(t, err)
	assert.Equal(t, "test-vm", vm2.Name)
	assert.Equal(t, 1, proc.startCalls)
}

func TestProviderFC_Deploy_MissingImages(t *testing.T) {
	p, _, _, _, _ := newTestFCProvider(t)
	p.Config.KernelImage = ""
	_, err := p.Deploy(Vm{Name: "test-vm"})
	assert.Error(t, err)
}

func TestProviderFC_Deploy_StartFailureCleansUp(t *testing.T) {
	p, proc, _, netMgr, _ := newTestFCProvider(t)
	proc.startErr = errors.New("boom")

	_, err := p.Deploy(Vm{Name: "test-vm"})
	assert.Error(t, err)
	assert.Contains(t, netMgr.deleted, fcTapName("test-vm"))

	_, statErr := os.Stat(p.vmDir("test-vm"))
	assert.True(t, os.IsNotExist(statErr))
}

// TestProviderFC_List_ReconcilesDeadProcess verifies that List() does not
// keep reporting a microVM as running once its firecracker process is gone
// out-of-band (e.g. the host rebooted, which every firecracker VMM dies to,
// leaving the on-disk "running" status stale).
func TestProviderFC_List_ReconcilesDeadProcess(t *testing.T) {
	p, proc, _, _, _ := newTestFCProvider(t)
	_, err := p.Deploy(Vm{Name: "test-vm"})
	require.NoError(t, err)

	running, err := p.List()
	require.NoError(t, err)
	require.Len(t, running.List, 1)
	assert.Equal(t, fcStatusRunning, running.List[0].Status)

	// Simulate a host reboot: the firecracker process is gone.
	proc.running = map[int]bool{}

	running, err = p.List()
	require.NoError(t, err)
	require.Len(t, running.List, 1, "a dead microVM is still surfaced, just with an honest status")
	assert.Equal(t, fcStatusStopped, running.List[0].Status)

	meta, err := loadFCMetadata(p.metadataPath("test-vm"))
	require.NoError(t, err)
	assert.Equal(t, fcStatusStopped, meta.Status, "status should self-heal on disk")
}

// TestProviderFC_Deploy_RecreatesStaleRecord verifies that Deploy() does not
// silently no-op when a same-named microVM's record exists but its process
// is dead — it should clean up the stale state and boot a fresh microVM.
func TestProviderFC_Deploy_RecreatesStaleRecord(t *testing.T) {
	p, proc, _, netMgr, _ := newTestFCProvider(t)
	_, err := p.Deploy(Vm{Name: "test-vm"})
	require.NoError(t, err)
	assert.Equal(t, 1, proc.startCalls)

	// Simulate a host reboot: the firecracker process is gone.
	proc.running = map[int]bool{}

	vm, err := p.Deploy(Vm{Name: "test-vm"})
	require.NoError(t, err)
	assert.Equal(t, 2, proc.startCalls, "Deploy should recreate a stale microVM instead of no-op'ing")
	assert.Equal(t, fcStatusRunning, vm.Status)
	assert.Contains(t, netMgr.deleted, fcTapName("test-vm"), "the stale tap device should be cleaned up")
}

// TestProviderFC_GetByName_ReconcilesDeadProcess verifies GetByName reports
// a dead microVM's status as stopped rather than the stale "running" value
// last persisted before the process died.
func TestProviderFC_GetByName_ReconcilesDeadProcess(t *testing.T) {
	p, proc, _, _, _ := newTestFCProvider(t)
	_, err := p.Deploy(Vm{Name: "test-vm"})
	require.NoError(t, err)

	proc.running = map[int]bool{}

	vm, err := p.GetByName("test-vm")
	require.NoError(t, err)
	assert.Equal(t, fcStatusStopped, vm.Status)
}

func TestProviderFC_Destroy(t *testing.T) {
	p, proc, _, netMgr, _ := newTestFCProvider(t)
	_, err := p.Deploy(Vm{Name: "test-vm"})
	require.NoError(t, err)

	require.NoError(t, p.Destroy(Vm{Name: "test-vm"}))
	assert.Contains(t, proc.stopCalls, 12345)
	assert.Contains(t, netMgr.deleted, fcTapName("test-vm"))

	_, statErr := os.Stat(p.vmDir("test-vm"))
	assert.True(t, os.IsNotExist(statErr))
}

// TestProviderFC_Destroy_StalePID verifies that Destroy does not
// signal a running process whose PID was persisted for this microVM but no
// longer belongs to it (e.g. reused after a host reboot).
func TestProviderFC_Destroy_StalePID(t *testing.T) {
	p, proc, _, netMgr, _ := newTestFCProvider(t)
	_, err := p.Deploy(Vm{Name: "test-vm"})
	require.NoError(t, err)
	proc.notOwned = map[int]bool{12345: true}

	require.NoError(t, p.Destroy(Vm{Name: "test-vm"}))
	assert.NotContains(t, proc.stopCalls, 12345)
	assert.Contains(t, netMgr.deleted, fcTapName("test-vm"))

	_, statErr := os.Stat(p.vmDir("test-vm"))
	assert.True(t, os.IsNotExist(statErr))
}

func TestProviderFC_Destroy_NotFound(t *testing.T) {
	p, _, _, _, _ := newTestFCProvider(t)
	assert.Error(t, p.Destroy(Vm{Name: "nope"}))
}

func TestProviderFC_PauseResume(t *testing.T) {
	p, _, api, _, _ := newTestFCProvider(t)
	_, err := p.Deploy(Vm{Name: "test-vm"})
	require.NoError(t, err)

	require.NoError(t, p.Pause(Vm{Name: "test-vm"}, true))
	assert.Equal(t, []string{"Paused"}, api.states)

	paused, err := p.ListPaused()
	require.NoError(t, err)
	require.Len(t, paused.List, 1)
	assert.Equal(t, "paused", paused.List[0].Status)

	running, err := p.List()
	require.NoError(t, err)
	assert.Empty(t, running.List)

	vm, err := p.Resume(Vm{Name: "test-vm"})
	require.NoError(t, err)
	assert.Equal(t, "running", vm.Status)
	assert.Equal(t, []string{"Paused", "Resumed"}, api.states)

	running, err = p.List()
	require.NoError(t, err)
	require.Len(t, running.List, 1)
}

func TestProviderFC_Pause_NotFound(t *testing.T) {
	p, _, _, _, _ := newTestFCProvider(t)
	assert.Error(t, p.Pause(Vm{Name: "nope"}, true))
}

func TestProviderFC_GetByName_NotFound(t *testing.T) {
	p, _, _, _, _ := newTestFCProvider(t)
	vm, err := p.GetByName("nope")
	require.NoError(t, err)
	assert.Equal(t, Vm{}, vm)
}

func TestProviderFC_CreateSSHKey(t *testing.T) {
	p, _, _, _, _ := newTestFCProvider(t)
	keyFile := writeTestPublicKey(t)

	keyID, err := p.CreateSSHKey(keyFile)
	require.NoError(t, err)
	assert.True(t, filepath.IsAbs(keyID))
}

func TestProviderFC_CreateSSHKey_Invalid(t *testing.T) {
	p, _, _, _, _ := newTestFCProvider(t)
	keyFile := filepath.Join(t.TempDir(), "bad.pub")
	require.NoError(t, os.WriteFile(keyFile, []byte("not a key"), 0644))

	_, err := p.CreateSSHKey(keyFile)
	assert.Error(t, err)
}
