package providerch

// Tests for providerch package.

import (
	"encoding/json"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/cdalar/onctl/pkg/cloud"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// startUnixServer serves mux over a fresh unix socket at sock, closing it on
// test cleanup.
func startUnixServer(t *testing.T, sock string, mux *http.ServeMux) {
	t.Helper()
	l, err := net.Listen("unix", sock)
	require.NoError(t, err)
	server := &http.Server{Handler: mux}
	go func() { _ = server.Serve(l) }()
	t.Cleanup(func() { _ = server.Close() })
}

// shortTempDir is like t.TempDir() but short: the long, test-name-derived
// paths t.TempDir() produces can exceed the ~104 byte AF_UNIX path limit on
// macOS, causing net.Listen("unix", ...) to fail.
func shortTempDir(t *testing.T) string {
	t.Helper()
	dir, err := os.MkdirTemp("", "chsock")
	require.NoError(t, err)
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	return dir
}

// TestConfigureAndBoot_PayloadShape is a regression test for a review
// finding: Cloud Hypervisor's vm.create expects the kernel path and boot
// command line nested under a "payload" object ({"payload": {"kernel":
// "...", "cmdline": "..."}}), not top-level "kernel"/"cmdline" objects like
// an earlier draft sent. Decoding the exact bytes onctl puts on the wire
// (rather than just asserting on Go struct fields) is what would have
// caught this the first time.
func TestConfigureAndBoot_PayloadShape(t *testing.T) {
	sock := filepath.Join(shortTempDir(t), "api.sock")

	var createBody map[string]any
	var gotPaths []string
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/vm.create", func(w http.ResponseWriter, r *http.Request) {
		gotPaths = append(gotPaths, r.URL.Path)
		require.NoError(t, json.NewDecoder(r.Body).Decode(&createBody))
		w.WriteHeader(http.StatusNoContent)
	})
	mux.HandleFunc("/api/v1/vm.boot", func(w http.ResponseWriter, r *http.Request) {
		gotPaths = append(gotPaths, r.URL.Path)
		assert.Equal(t, http.NoBody, r.Body, "vm.boot must be sent with no body")
		w.WriteHeader(http.StatusNoContent)
	})
	startUnixServer(t, sock, mux)

	cfg := cloud.CHVMConfig{
		KernelImage: "/images/vmlinux",
		KernelArgs:  "console=ttyS0",
		RootfsPath:  "/vms/test/rootfs.ext4",
		VCPUCount:   2,
		MemSizeMib:  1024,
		TapDevice:   "ch0123456789abc",
		MacAddress:  "02:C4:00:00:00:00",
	}
	require.NoError(t, configureAndBoot(sock, cfg))
	assert.Equal(t, []string{"/api/v1/vm.create", "/api/v1/vm.boot"}, gotPaths)

	require.Contains(t, createBody, "payload")
	payload, ok := createBody["payload"].(map[string]any)
	require.True(t, ok, "payload must be an object, got %T", createBody["payload"])
	assert.Equal(t, cfg.KernelImage, payload["kernel"])
	assert.Equal(t, cfg.KernelArgs, payload["cmdline"])
	assert.NotContains(t, createBody, "kernel", "kernel must not be a top-level field")
	assert.NotContains(t, createBody, "cmdline", "cmdline must not be a top-level field")

	cpus, ok := createBody["cpus"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, float64(cfg.VCPUCount), cpus["boot_vcpus"])
	assert.Equal(t, float64(cfg.VCPUCount), cpus["max_vcpus"])

	memory, ok := createBody["memory"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, float64(cfg.MemSizeMib*1024*1024), memory["size"])

	disks, ok := createBody["disks"].([]any)
	require.True(t, ok)
	require.Len(t, disks, 1)
	assert.Equal(t, cfg.RootfsPath, disks[0].(map[string]any)["path"])

	nets, ok := createBody["net"].([]any)
	require.True(t, ok)
	require.Len(t, nets, 1)
	assert.Equal(t, cfg.TapDevice, nets[0].(map[string]any)["tap"])
	assert.Equal(t, cfg.MacAddress, nets[0].(map[string]any)["mac"])
}

// TestConfigureAndBoot_WindowsPayloadShape mirrors
// TestConfigureAndBoot_PayloadShape for the ch.os=windows path: Firmware
// must land under payload.firmware (never payload.kernel), cpus.kvm_hyperv
// must be set, and a second, read-only disk (the seed ISO) must be present.
func TestConfigureAndBoot_WindowsPayloadShape(t *testing.T) {
	sock := filepath.Join(shortTempDir(t), "api.sock")

	var createBody map[string]any
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/vm.create", func(w http.ResponseWriter, r *http.Request) {
		require.NoError(t, json.NewDecoder(r.Body).Decode(&createBody))
		w.WriteHeader(http.StatusNoContent)
	})
	mux.HandleFunc("/api/v1/vm.boot", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})
	startUnixServer(t, sock, mux)

	cfg := cloud.CHVMConfig{
		KernelImage:  "/images/CLOUDHV.fd",
		Firmware:     true,
		KvmHyperv:    true,
		RootfsPath:   "/vms/test/disk.raw",
		SeedDiskPath: "/vms/test/seed.iso",
		VCPUCount:    2,
		MemSizeMib:   4096,
		TapDevice:    "ch0123456789abc",
		MacAddress:   "02:C4:00:00:00:00",
	}
	require.NoError(t, configureAndBoot(sock, cfg))

	payload, ok := createBody["payload"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, cfg.KernelImage, payload["firmware"])
	assert.NotContains(t, payload, "kernel", "a firmware boot must not also send a kernel path")
	assert.NotContains(t, payload, "cmdline", "windows has no kernel cmdline")

	cpus, ok := createBody["cpus"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, true, cpus["kvm_hyperv"])

	disks, ok := createBody["disks"].([]any)
	require.True(t, ok)
	require.Len(t, disks, 2, "the seed ISO must be attached as a second disk")
	assert.Equal(t, cfg.RootfsPath, disks[0].(map[string]any)["path"])
	assert.Equal(t, cfg.SeedDiskPath, disks[1].(map[string]any)["path"])
	assert.Equal(t, true, disks[1].(map[string]any)["readonly"])
	assert.NotContains(t, disks[0].(map[string]any), "readonly", "the main disk must stay writable")
}

func TestConfigureAndBoot_CreateError(t *testing.T) {
	sock := filepath.Join(shortTempDir(t), "api.sock")

	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/vm.create", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	})
	startUnixServer(t, sock, mux)

	err := configureAndBoot(sock, cloud.CHVMConfig{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "vm.create")
}

func TestConfigureAndBoot_BootError(t *testing.T) {
	sock := filepath.Join(shortTempDir(t), "api.sock")

	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/vm.create", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})
	mux.HandleFunc("/api/v1/vm.boot", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	})
	startUnixServer(t, sock, mux)

	err := configureAndBoot(sock, cloud.CHVMConfig{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "vm.boot")
}

func TestDHCPRange(t *testing.T) {
	start, end, err := dhcpRange("172.17.0.1/24")
	require.NoError(t, err)
	assert.Equal(t, "172.17.0.2", start)
	assert.Equal(t, "172.17.0.254", end)
}

func TestDHCPRange_TooSmall(t *testing.T) {
	_, _, err := dhcpRange("172.17.0.1/31")
	assert.Error(t, err)
}

func TestDHCPRange_InvalidCIDR(t *testing.T) {
	_, _, err := dhcpRange("not-a-cidr")
	assert.Error(t, err)
}

func TestConfigDriveUUID(t *testing.T) {
	uuid := configDriveUUID("my-test-vm")
	assert.Regexp(t, `^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`, uuid)
	assert.Equal(t, uuid, configDriveUUID("my-test-vm"), "must be deterministic")
	assert.NotEqual(t, uuid, configDriveUUID("other-vm"))
}

// TestConfigDriveSeedPreparer_BuildSeedISO requires one of the ISO-building
// tools (genisoimage/mkisofs/xorriso) on PATH — the same defensive skip
// pattern providerfc's tests use for the `ip` binary.
func TestConfigDriveSeedPreparer_BuildSeedISO(t *testing.T) {
	found := false
	for _, bin := range isoBuilders {
		if _, err := exec.LookPath(bin); err == nil {
			found = true
			break
		}
	}
	if !found {
		t.Skip("no ISO-building tool (genisoimage/mkisofs/xorriso) available")
	}

	dest := filepath.Join(t.TempDir(), "seed.iso")
	prep := ConfigDriveSeedPreparer{}
	require.NoError(t, prep.BuildSeedISO(dest, "win-test", "ssh-ed25519 AAAA test", "Administrator"))

	info, err := os.Stat(dest)
	require.NoError(t, err)
	assert.Greater(t, info.Size(), int64(0))
}

func TestConfigDriveSeedPreparer_PrepareDisk(t *testing.T) {
	src := filepath.Join(t.TempDir(), "base.raw")
	require.NoError(t, os.WriteFile(src, []byte("fake-windows-disk"), 0600))
	dest := filepath.Join(t.TempDir(), "disk.raw")

	prep := ConfigDriveSeedPreparer{}
	require.NoError(t, prep.PrepareDisk(src, dest))

	got, err := os.ReadFile(dest)
	require.NoError(t, err)
	assert.Equal(t, "fake-windows-disk", string(got))
}

func TestConfigDriveSeedPreparer_PrepareDisk_NoBaseImage(t *testing.T) {
	prep := ConfigDriveSeedPreparer{}
	assert.Error(t, prep.PrepareDisk("", filepath.Join(t.TempDir(), "disk.raw")))
}

// TestDnsmasqDHCPManager_ReservationRoundTrip exercises AddReservation/
// RemoveReservation's hosts-file bookkeeping directly, without starting a
// real dnsmasq process (EnsureDHCP is not called here — reload() is a
// no-op when no dnsmasq pid file exists yet, which is exactly the state
// this test runs in).
func TestDnsmasqDHCPManager_ReservationRoundTrip(t *testing.T) {
	stateDir := t.TempDir()
	m := DnsmasqDHCPManager{StateDir: stateDir}
	bridge := "chbr0"
	require.NoError(t, os.MkdirAll(m.dhcpDir(bridge), 0755))
	require.NoError(t, os.WriteFile(m.hostsFile(bridge), []byte{}, 0644))

	require.NoError(t, m.AddReservation("02:C4:00:00:00:01", "172.17.0.2"))
	require.NoError(t, m.AddReservation("02:C4:00:00:00:02", "172.17.0.3"))

	// Regression check for a real bug: written lowercase regardless of
	// input case, since dnsmasq's --dhcp-hostsfile matching is
	// case-sensitive against the lowercase MAC a DHCP client actually
	// sends — a mixed-case entry here silently never matches its own
	// reservation and the guest gets a random pool address instead.
	data, err := os.ReadFile(m.hostsFile(bridge))
	require.NoError(t, err)
	assert.Contains(t, string(data), "02:c4:00:00:00:01,172.17.0.2")
	assert.Contains(t, string(data), "02:c4:00:00:00:02,172.17.0.3")
	assert.NotContains(t, string(data), "02:C4")

	// RemoveReservation must also match regardless of the case it's called
	// with.
	require.NoError(t, m.RemoveReservation("02:C4:00:00:00:01"))

	data, err = os.ReadFile(m.hostsFile(bridge))
	require.NoError(t, err)
	assert.NotContains(t, string(data), "00:00:00:01")
	assert.Contains(t, string(data), "02:c4:00:00:00:02,172.17.0.3")
}

// TestDnsmasqDHCPManager_RemoveReservation_ClearsStaleLease is a
// regression test for a real bug found live: RemoveReservation used to
// only strip the hosts-file reservation, leaving the corresponding
// dnsmasq lease-file entry in place. Since dnsmasq only reads its lease
// file at startup and then owns/rewrites it from memory, that stale entry
// silently survived a VM's destroy and blocked the next VM that reused
// the same low IP (chIPs allocates low addresses first) from ever
// actually getting it via DHCP -- onctl create would report the expected
// IP but the guest's real DHCP handshake got redirected elsewhere,
// hanging onctl's SSH-wait until timeout. No real dnsmasq process is
// started here (no EnsureDHCP/cidr file), so restartDnsmasq's actual
// process kill/restart is exercised as a no-op -- this test only proves
// the lease-file bookkeeping itself, matching
// TestDnsmasqDHCPManager_ReservationRoundTrip's existing no-real-dnsmasq
// pattern above.
func TestDnsmasqDHCPManager_RemoveReservation_ClearsStaleLease(t *testing.T) {
	stateDir := t.TempDir()
	m := DnsmasqDHCPManager{StateDir: stateDir}
	bridge := "chbr0"
	require.NoError(t, os.MkdirAll(m.dhcpDir(bridge), 0755))
	require.NoError(t, os.WriteFile(m.hostsFile(bridge), []byte("02:c4:00:00:00:01,172.17.0.2\n"), 0644))
	require.NoError(t, os.WriteFile(m.leaseFile(bridge), []byte(
		"1234567890 02:c4:00:00:00:01 172.17.0.2 my-old-vm 01:02:c4:00:00:00:01\n"+
			"1234567891 02:c4:00:00:00:02 172.17.0.3 other-vm 01:02:c4:00:00:00:02\n",
	), 0644))

	require.NoError(t, m.RemoveReservation("02:C4:00:00:00:01"))

	data, err := os.ReadFile(m.leaseFile(bridge))
	require.NoError(t, err)
	assert.NotContains(t, string(data), "00:00:00:01")
	assert.Contains(t, string(data), "02:c4:00:00:00:02 172.17.0.3")

	hostsData, err := os.ReadFile(m.hostsFile(bridge))
	require.NoError(t, err)
	assert.NotContains(t, string(hostsData), "00:00:00:01")
}

// TestDnsmasqDHCPManager_RemoveReservation_NoMatch checks that removing a
// MAC with no hosts-file reservation and no lease-file entry is a clean
// no-op (not an error, and doesn't rewrite either file with equivalent
// content) -- Destroy calls this unconditionally, including for VMs whose
// DHCP setup never fully completed.
func TestDnsmasqDHCPManager_RemoveReservation_NoMatch(t *testing.T) {
	stateDir := t.TempDir()
	m := DnsmasqDHCPManager{StateDir: stateDir}
	bridge := "chbr0"
	require.NoError(t, os.MkdirAll(m.dhcpDir(bridge), 0755))
	require.NoError(t, os.WriteFile(m.hostsFile(bridge), []byte("02:c4:00:00:00:02,172.17.0.3\n"), 0644))

	require.NoError(t, m.RemoveReservation("02:c4:00:00:00:99"))

	data, err := os.ReadFile(m.hostsFile(bridge))
	require.NoError(t, err)
	assert.Equal(t, "02:c4:00:00:00:02,172.17.0.3\n", string(data))
}
