package cloud

import (
	"crypto/ed25519"
	"crypto/md5"
	"crypto/rand"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/crypto/ssh"
)

// fakeFCProcess is a test double for FCProcess.
type fakeFCProcess struct {
	pid            int
	startCalls     int
	startCfgs      []FCVMConfig
	startErr       error
	startBareCalls int
	startBareErr   error
	stopCalls      []int
	running        map[int]bool
	notOwned       map[int]bool
}

func (f *fakeFCProcess) Start(_ string, cfg FCVMConfig, _ string) (int, error) {
	f.startCalls++
	f.startCfgs = append(f.startCfgs, cfg)
	if f.startErr != nil {
		return 0, f.startErr
	}
	if f.running == nil {
		f.running = map[int]bool{}
	}
	f.running[f.pid] = true
	return f.pid, nil
}

func (f *fakeFCProcess) StartBare(_ string, _ string) (int, error) {
	f.startBareCalls++
	if f.startBareErr != nil {
		return 0, f.startBareErr
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
	states            []string
	snapshotCalls     []string // "snapshotPath|memFilePath"
	createSnapshotErr error
	loadCalls         []string // "snapshotPath|memFilePath"
	loadSnapshotErr   error
}

func (f *fakeFCAPI) SetState(_ string, state string) error {
	f.states = append(f.states, state)
	return nil
}

func (f *fakeFCAPI) CreateSnapshot(_ string, snapshotPath, memFilePath string) error {
	f.snapshotCalls = append(f.snapshotCalls, snapshotPath+"|"+memFilePath)
	if f.createSnapshotErr != nil {
		return f.createSnapshotErr
	}
	if err := os.WriteFile(snapshotPath, []byte("state"), 0600); err != nil {
		return err
	}
	return os.WriteFile(memFilePath, []byte("mem"), 0600)
}

func (f *fakeFCAPI) LoadSnapshot(_ string, snapshotPath, memFilePath string, _ bool, _ []FCNetworkOverride) error {
	f.loadCalls = append(f.loadCalls, snapshotPath+"|"+memFilePath)
	return f.loadSnapshotErr
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
	calls     []string
	hostnames []string
	// identityErr, when set, is returned by BaseImageIdentity instead of
	// the fixed fake hash/size below -- lets tests exercise Deploy's
	// soft-fail path (a hashing failure must not fail Deploy itself).
	identityErr error
}

func (f *fakeRootfsPreparer) Prepare(_, destPath, hostname, _, _ string) error {
	f.calls = append(f.calls, destPath)
	f.hostnames = append(f.hostnames, hostname)
	return os.WriteFile(destPath, []byte("rootfs"), 0600)
}

func (f *fakeRootfsPreparer) BaseImageIdentity(baseImage string) (string, int64, error) {
	if f.identityErr != nil {
		return "", 0, f.identityErr
	}
	return "fakehash-" + baseImage, 42, nil
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

// fakeCacheDiskPreparer is a test double for CacheDiskPreparer.
type fakeCacheDiskPreparer struct {
	prepareCalls   []string // "goldenImage->destPath"
	prepareErr     error
	mergeBackCalls []string // "vmCachePath->goldenImage"
	mergeBackErr   error
}

func (f *fakeCacheDiskPreparer) Prepare(goldenImage, destPath string, _ int64) error {
	f.prepareCalls = append(f.prepareCalls, goldenImage+"->"+destPath)
	if f.prepareErr != nil {
		return f.prepareErr
	}
	return os.WriteFile(destPath, []byte("cache"), 0600)
}

func (f *fakeCacheDiskPreparer) MergeBack(vmCachePath, goldenImage string) error {
	f.mergeBackCalls = append(f.mergeBackCalls, vmCachePath+"->"+goldenImage)
	return f.mergeBackErr
}

// newTestFCProviderWithCache is newTestFCProvider plus a fake
// CacheDiskPreparer and Config.CacheImage set, for tests exercising the
// cache-disk feature specifically.
func newTestFCProviderWithCache(t *testing.T) (ProviderFC, *fakeCacheDiskPreparer) {
	t.Helper()
	p, _, _, _, _ := newTestFCProvider(t)
	cache := &fakeCacheDiskPreparer{}
	p.Config.CacheImage = filepath.Join(t.TempDir(), "golden.ext4")
	p.Config.CacheSizeMib = 4096
	p.Cache = cache
	return p, cache
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

// TestProviderFC_AllocateAndReserveIP_ConcurrentCallsGetDistinctIPs
// reproduces the race found while testing the cache-disk feature: two
// concurrent `onctl create` calls (e.g. a Build and a Lint job for the
// same repo, dispatched at once) reading usedIPs() before either has
// persisted its own metadata could previously compute and allocate the
// exact same "next free" address. allocateAndReserveIP fixes this by
// reserving under an exclusive lock; this test drives it directly (not
// through the full Deploy, whose other fakes aren't goroutine-safe) with
// real concurrent goroutines against a real temp StateDir.
func TestProviderFC_AllocateAndReserveIP_ConcurrentCallsGetDistinctIPs(t *testing.T) {
	p := ProviderFC{Config: FCConfig{StateDir: t.TempDir()}}
	const n = 8
	ips := make([]string, n)
	errs := make([]error, n)
	var wg sync.WaitGroup
	for i := 0; i < n; i++ {
		name := fmt.Sprintf("vm-%d", i)
		require.NoError(t, os.MkdirAll(p.vmDir(name), 0755)) // Deploy always creates this before allocating an IP
		wg.Add(1)
		go func(i int, name string) {
			defer wg.Done()
			ips[i], errs[i] = p.allocateAndReserveIP(name, "172.16.0.1/24")
		}(i, name)
	}
	wg.Wait()

	seen := make(map[string]bool, n)
	for i, err := range errs {
		require.NoError(t, err)
		require.False(t, seen[ips[i]], "IP %s allocated to more than one VM", ips[i])
		seen[ips[i]] = true
	}
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
	assert.Equal(t, []string{"test-vm"}, rootfs.hostnames)

	meta, err := loadFCMetadata(p.metadataPath("test-vm"))
	require.NoError(t, err)
	assert.Equal(t, 12345, meta.PID)
	assert.Equal(t, "172.16.0.2", meta.IPAddress)
	assert.Equal(t, fcStatusRunning, meta.Status)
	assert.Equal(t, "/images/rootfs.ext4", meta.BaseImagePath)
	assert.Equal(t, "fakehash-/images/rootfs.ext4", meta.BaseImageSHA256)
	assert.Equal(t, int64(42), meta.BaseImageSizeBytes)
}

// TestProviderFC_Deploy_BaseImageIdentityFailureIsNonFatal covers Deploy's
// soft-fail path: BaseImageIdentity is purely an optimization for a later,
// unrelated feature (diff-based export) and must never block VM creation.
func TestProviderFC_Deploy_BaseImageIdentityFailureIsNonFatal(t *testing.T) {
	p, _, _, _, rootfs := newTestFCProvider(t)
	rootfs.identityErr = errors.New("boom")

	vm, err := p.Deploy(Vm{Name: "test-vm"})
	require.NoError(t, err)
	assert.Equal(t, "test-vm", vm.Name)

	meta, err := loadFCMetadata(p.metadataPath("test-vm"))
	require.NoError(t, err)
	assert.Empty(t, meta.BaseImagePath)
	assert.Empty(t, meta.BaseImageSHA256)
	assert.Zero(t, meta.BaseImageSizeBytes)
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
	assert.Equal(t, fcStatusDead, running.List[0].Status)

	meta, err := loadFCMetadata(p.metadataPath("test-vm"))
	require.NoError(t, err)
	assert.Equal(t, fcStatusDead, meta.Status, "status should self-heal on disk")
}

// TestProviderFC_List_SSHReadyMirrorsStatus is a regression test for a
// deliberate difference from ch: fc's List() must NOT TCP-probe for
// SSHReady (see mapFCVM's doc comment -- a Firecracker microVM boots in
// milliseconds, so Status=="running" is already an accurate-enough proxy,
// and probing would only add needless latency to every List() call). A
// freshly deployed VM here has no real, reachable IP at all (the fake
// test harness never assigns one) -- if List() were still probing, this
// VM would incorrectly come back SSHReady=false; asserting it comes back
// true instead proves the probe path is gone, not just untested.
func TestProviderFC_List_SSHReadyMirrorsStatus(t *testing.T) {
	p, _, _, _, _ := newTestFCProvider(t)
	_, err := p.Deploy(Vm{Name: "test-vm"})
	require.NoError(t, err)

	list, err := p.List()
	require.NoError(t, err)
	require.Len(t, list.List, 1)
	assert.True(t, list.List[0].SSHReady, "fc SSHReady must mirror Status==running, not a real probe")
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
	assert.Equal(t, fcStatusDead, vm.Status)
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

// TestProviderFC_Deploy_NoCacheImage_SkipsCacheDisk verifies that Deploy
// never touches Cache when FCConfig.CacheImage is unset (the default;
// nil Cache must be safe here since most callers never configure this).
func TestProviderFC_Deploy_NoCacheImage_SkipsCacheDisk(t *testing.T) {
	p, _, _, _, _ := newTestFCProvider(t)
	_, err := p.Deploy(Vm{Name: "test-vm"})
	require.NoError(t, err)

	vm, err := loadFCMetadata(p.metadataPath("test-vm"))
	require.NoError(t, err)
	assert.Empty(t, vm.CachePath)
	assert.Empty(t, vm.CacheImage)
}

// TestProviderFC_Deploy_PreparesCacheDisk verifies that Deploy attaches a
// per-VM cache disk clone (drive config threaded through FCVMConfig,
// covered by TestConfigureAndBoot-level behavior in providerfc) and
// persists both CachePath and CacheImage for Destroy's later merge-back.
func TestProviderFC_Deploy_PreparesCacheDisk(t *testing.T) {
	p, cache := newTestFCProviderWithCache(t)
	_, err := p.Deploy(Vm{Name: "test-vm"})
	require.NoError(t, err)

	wantCachePath := filepath.Join(p.vmDir("test-vm"), "cache.ext4")
	require.Len(t, cache.prepareCalls, 1)
	assert.Equal(t, p.Config.CacheImage+"->"+wantCachePath, cache.prepareCalls[0])

	vm, err := loadFCMetadata(p.metadataPath("test-vm"))
	require.NoError(t, err)
	assert.Equal(t, wantCachePath, vm.CachePath)
	assert.Equal(t, p.Config.CacheImage, vm.CacheImage)
}

// TestProviderFC_Deploy_CacheDiskPrepareFailureCleansUp mirrors
// TestProviderFC_Deploy_StartFailureCleansUp for the cache-disk step: a
// Prepare failure must not leave a half-created VM directory behind.
func TestProviderFC_Deploy_CacheDiskPrepareFailureCleansUp(t *testing.T) {
	p, cache := newTestFCProviderWithCache(t)
	cache.prepareErr = errors.New("boom")

	_, err := p.Deploy(Vm{Name: "test-vm"})
	require.Error(t, err)

	_, statErr := os.Stat(p.vmDir("test-vm"))
	assert.True(t, os.IsNotExist(statErr))
}

// TestProviderFC_Destroy_MergesCacheDiskBack verifies that Destroy folds
// a VM's cache-disk changes back into the golden image before tearing
// the VM down, so a later VM created against the same CacheImage
// benefits from this job's work.
func TestProviderFC_Destroy_MergesCacheDiskBack(t *testing.T) {
	p, cache := newTestFCProviderWithCache(t)
	_, err := p.Deploy(Vm{Name: "test-vm"})
	require.NoError(t, err)

	wantCachePath := filepath.Join(p.vmDir("test-vm"), "cache.ext4")
	require.NoError(t, p.Destroy(Vm{Name: "test-vm"}))

	require.Len(t, cache.mergeBackCalls, 1)
	assert.Equal(t, wantCachePath+"->"+p.Config.CacheImage, cache.mergeBackCalls[0])
}

// TestProviderFC_Destroy_CacheMergeBackFailureDoesNotBlockDestroy verifies
// that a MergeBack error is logged, not propagated — a cache-disk hiccup
// must never prevent a VM from being torn down.
func TestProviderFC_Destroy_CacheMergeBackFailureDoesNotBlockDestroy(t *testing.T) {
	p, cache := newTestFCProviderWithCache(t)
	_, err := p.Deploy(Vm{Name: "test-vm"})
	require.NoError(t, err)
	cache.mergeBackErr = errors.New("disk full")

	require.NoError(t, p.Destroy(Vm{Name: "test-vm"}))
	_, statErr := os.Stat(p.vmDir("test-vm"))
	assert.True(t, os.IsNotExist(statErr))
}

func TestProviderFC_PauseResume(t *testing.T) {
	p, proc, api, net, _ := newTestFCProvider(t)
	_, err := p.Deploy(Vm{Name: "test-vm"})
	require.NoError(t, err)

	require.NoError(t, p.Pause(Vm{Name: "test-vm"}, true))
	assert.Equal(t, []string{"Paused"}, api.states)
	assert.Len(t, api.snapshotCalls, 1)
	// Pause stops the process and tears down the tap device — no compute
	// cost while paused, matching the other providers' delete-the-server
	// pause semantics.
	assert.Equal(t, []int{proc.pid}, proc.stopCalls)
	assert.Contains(t, net.deleted, fcTapName("test-vm"))

	paused, err := p.ListPaused()
	require.NoError(t, err)
	require.Len(t, paused.List, 1)
	assert.Equal(t, "paused", paused.List[0].Status)

	running, err := p.List()
	require.NoError(t, err)
	assert.Empty(t, running.List)

	// A paused VM's process is legitimately gone — GetByName must not
	// reconcile it to "dead".
	byName, err := p.GetByName("test-vm")
	require.NoError(t, err)
	assert.Equal(t, "paused", byName.Status)

	vm, err := p.Resume(Vm{Name: "test-vm"})
	require.NoError(t, err)
	assert.Equal(t, "running", vm.Status)
	assert.Len(t, api.loadCalls, 1)
	assert.Equal(t, 1, proc.startBareCalls)

	running, err = p.List()
	require.NoError(t, err)
	require.Len(t, running.List, 1)
}

// newTestFCProviderWithKernels is newTestFCProvider with real kernel files,
// since Restart checks the kernel it boots exists.
func newTestFCProviderWithKernels(t *testing.T) (ProviderFC, *fakeFCProcess, *fakeNetworkManager, *fakeRootfsPreparer, string, string) {
	t.Helper()
	p, proc, _, net, rootfs := newTestFCProvider(t)
	dir := t.TempDir()
	oldKernel := filepath.Join(dir, "vmlinux-old")
	newKernel := filepath.Join(dir, "vmlinux-new")
	require.NoError(t, os.WriteFile(oldKernel, []byte("old"), 0644))
	require.NoError(t, os.WriteFile(newKernel, []byte("new"), 0644))
	p.Config.KernelImage = oldKernel
	return p, proc, net, rootfs, oldKernel, newKernel
}

func TestProviderFC_Restart_RunningWithNewKernel(t *testing.T) {
	p, proc, net, rootfs, oldKernel, newKernel := newTestFCProviderWithKernels(t)
	deployed, err := p.Deploy(Vm{Name: "test-vm"})
	require.NoError(t, err)
	before, err := loadFCMetadata(p.metadataPath("test-vm"))
	require.NoError(t, err)
	assert.Equal(t, oldKernel, before.KernelImage)

	proc.pid = 23456 // the new VMM gets a new PID
	vm, err := p.Restart(Vm{Name: "test-vm"}, RestartOptions{KernelImage: newKernel})
	require.NoError(t, err)

	// The old process is stopped and a fresh VMM boots the same disk, IP and
	// MAC on the new kernel -- no new rootfs is prepared.
	assert.Equal(t, []int{12345}, proc.stopCalls)
	require.Len(t, proc.startCfgs, 2)
	cfg := proc.startCfgs[1]
	assert.Equal(t, newKernel, cfg.KernelImage)
	assert.Equal(t, before.RootfsPath, cfg.RootfsPath)
	assert.Equal(t, before.MacAddress, cfg.MacAddress)
	assert.Equal(t, proc.startCfgs[0].KernelArgs, cfg.KernelArgs, "same kernel command line, incl. the static IP")
	assert.Len(t, rootfs.calls, 1)
	assert.Contains(t, net.deleted, fcTapName("test-vm"))
	assert.Equal(t, []string{fcTapName("test-vm"), fcTapName("test-vm")}, net.taps)

	assert.Equal(t, "running", vm.Status)
	assert.Equal(t, deployed.IP, vm.IP)
	after, err := loadFCMetadata(p.metadataPath("test-vm"))
	require.NoError(t, err)
	assert.Equal(t, newKernel, after.KernelImage, "the new kernel sticks for later restarts")
	assert.Equal(t, 23456, after.PID)
}

func TestProviderFC_Restart_KeepsKernelByDefault(t *testing.T) {
	p, proc, _, _, oldKernel, _ := newTestFCProviderWithKernels(t)
	_, err := p.Deploy(Vm{Name: "test-vm"})
	require.NoError(t, err)

	_, err = p.Restart(Vm{Name: "test-vm"}, RestartOptions{})
	require.NoError(t, err)
	require.Len(t, proc.startCfgs, 2)
	assert.Equal(t, oldKernel, proc.startCfgs[1].KernelImage)
}

func TestProviderFC_Restart_DeadVM(t *testing.T) {
	// A host reboot kills every VMM; Restart must bring the VM back from its
	// disk rather than requiring destroy + redeploy.
	p, proc, _, rootfs, _, _ := newTestFCProviderWithKernels(t)
	_, err := p.Deploy(Vm{Name: "test-vm"})
	require.NoError(t, err)
	delete(proc.running, proc.pid)
	byName, err := p.GetByName("test-vm")
	require.NoError(t, err)
	require.Equal(t, "dead", byName.Status)

	vm, err := p.Restart(Vm{Name: "test-vm"}, RestartOptions{})
	require.NoError(t, err)
	assert.Equal(t, "running", vm.Status)
	assert.Empty(t, proc.stopCalls, "nothing to stop for a dead VM")
	assert.Len(t, rootfs.calls, 1, "the existing disk is reused")
}

func TestProviderFC_Restart_PausedVMDiscardsSnapshot(t *testing.T) {
	p, proc, _, _, _, newKernel := newTestFCProviderWithKernels(t)
	_, err := p.Deploy(Vm{Name: "test-vm"})
	require.NoError(t, err)
	require.NoError(t, p.Pause(Vm{Name: "test-vm"}, true))
	paused, err := loadFCMetadata(p.metadataPath("test-vm"))
	require.NoError(t, err)
	require.FileExists(t, paused.SnapshotMemFilePath)

	vm, err := p.Restart(Vm{Name: "test-vm"}, RestartOptions{KernelImage: newKernel})
	require.NoError(t, err)
	assert.Equal(t, "running", vm.Status)
	assert.Equal(t, 0, proc.startBareCalls, "a cold boot, not a snapshot restore")
	assert.NoFileExists(t, paused.SnapshotStatePath)
	assert.NoFileExists(t, paused.SnapshotMemFilePath)
	after, err := loadFCMetadata(p.metadataPath("test-vm"))
	require.NoError(t, err)
	assert.Empty(t, after.SnapshotStatePath)
	assert.Empty(t, after.SnapshotMemFilePath)
}

func TestProviderFC_Restart_MissingKernelLeavesVMAlone(t *testing.T) {
	p, proc, _, _, _, _ := newTestFCProviderWithKernels(t)
	_, err := p.Deploy(Vm{Name: "test-vm"})
	require.NoError(t, err)

	_, err = p.Restart(Vm{Name: "test-vm"}, RestartOptions{KernelImage: "/does/not/exist"})
	require.Error(t, err)
	assert.Empty(t, proc.stopCalls, "a bad kernel path must not take the running VM down")
}

func TestProviderFC_Restart_StartFailureMarksDead(t *testing.T) {
	p, proc, net, _, _, newKernel := newTestFCProviderWithKernels(t)
	_, err := p.Deploy(Vm{Name: "test-vm"})
	require.NoError(t, err)
	proc.startErr = errors.New("boom")

	_, err = p.Restart(Vm{Name: "test-vm"}, RestartOptions{KernelImage: newKernel})
	require.Error(t, err)
	after, err := loadFCMetadata(p.metadataPath("test-vm"))
	require.NoError(t, err)
	assert.Equal(t, "dead", after.Status)
	assert.Equal(t, 0, after.PID)
	assert.Contains(t, net.deleted, fcTapName("test-vm"))
}

func TestProviderFC_Restart_NotFound(t *testing.T) {
	p, _, _, _, _, _ := newTestFCProviderWithKernels(t)
	_, err := p.Restart(Vm{Name: "nope"}, RestartOptions{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no microVM found")
}

func TestProviderFC_ImplementsRestarter(t *testing.T) {
	var _ Restarter = ProviderFC{}
}

func TestProviderFC_Pause_NotAlive(t *testing.T) {
	p, proc, _, _, _ := newTestFCProvider(t)
	_, err := p.Deploy(Vm{Name: "test-vm"})
	require.NoError(t, err)
	proc.running[proc.pid] = false

	err = p.Pause(Vm{Name: "test-vm"}, true)
	assert.Error(t, err)
}

func TestProviderFC_Pause_SnapshotFailureResumesInstead(t *testing.T) {
	p, _, api, _, _ := newTestFCProvider(t)
	_, err := p.Deploy(Vm{Name: "test-vm"})
	require.NoError(t, err)
	api.createSnapshotErr = errors.New("disk full")

	err = p.Pause(Vm{Name: "test-vm"}, true)
	assert.Error(t, err)
	assert.Equal(t, []string{"Paused", "Resumed"}, api.states)

	vm, err := p.GetByName("test-vm")
	require.NoError(t, err)
	assert.Equal(t, "running", vm.Status)
}

func TestProviderFC_Resume_MissingSnapshotFiles(t *testing.T) {
	p, _, _, _, _ := newTestFCProvider(t)
	_, err := p.Deploy(Vm{Name: "test-vm"})
	require.NoError(t, err)
	require.NoError(t, p.Pause(Vm{Name: "test-vm"}, true))

	require.NoError(t, os.Remove(p.snapshotStatePath("test-vm")))

	_, err = p.Resume(Vm{Name: "test-vm"})
	assert.Error(t, err)
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

func TestSanitizeGuestHostname(t *testing.T) {
	longName := strings.Repeat("a", 100)
	longNameSum := md5.Sum([]byte(longName))
	longNameSuffix := fmt.Sprintf("%x", longNameSum)[:8]

	tests := []struct {
		name string
		want string
	}{
		{"gh-runner-12345", "gh-runner-12345"},
		{"Gh-Runner-12345", "gh-runner-12345"},
		{"my_vm.example.com", "my-vm-example-com"},
		{"  leading-trailing-space  ", "leading-trailing-space"},
		{"---", ""},
		{"", ""},
		{longName, strings.Repeat("a", 54) + "-" + longNameSuffix},
	}
	for _, tt := range tests {
		got := sanitizeGuestHostname(tt.name)
		assert.Equal(t, tt.want, got, "input %q", tt.name)
		assert.LessOrEqual(t, len(got), 63, "input %q", tt.name)
	}
}

func TestSanitizeGuestHostname_TruncationPreservesUniqueness(t *testing.T) {
	prefix := strings.Repeat("gh-runner-", 6) // 60 bytes, shared by both names
	nameA := prefix + "run-aaaaaaaa"
	nameB := prefix + "run-bbbbbbbb"

	hostnameA := sanitizeGuestHostname(nameA)
	hostnameB := sanitizeGuestHostname(nameB)

	assert.NotEqual(t, hostnameA, hostnameB, "distinct VM names sharing a long common prefix must not collide after truncation")
	assert.LessOrEqual(t, len(hostnameA), 63)
	assert.LessOrEqual(t, len(hostnameB), 63)
}
