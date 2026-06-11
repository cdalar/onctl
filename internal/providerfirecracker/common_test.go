package providerfirecracker

// Tests for providerfirecracker package.

import (
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/cdalar/onctl/pkg/cloud"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fakeFirecrackerEnv, when set to "1", causes TestMain to turn this test
// binary into a fake firecracker process that serves the API endpoints
// configureAndBoot relies on. This lets ProcessManager.Start/Stop/IsRunning
// be exercised end-to-end via os.Args[0] re-exec, the standard pattern for
// testing exec.Command-based code.
const fakeFirecrackerEnv = "ONCTL_TEST_FAKE_FIRECRACKER"

func TestMain(m *testing.M) {
	if os.Getenv(fakeFirecrackerEnv) == "1" {
		runFakeFirecracker()
		return
	}
	os.Exit(m.Run())
}

func runFakeFirecracker() {
	var sock string
	for i, a := range os.Args {
		if a == "--api-sock" && i+1 < len(os.Args) {
			sock = os.Args[i+1]
		}
	}
	if sock == "" {
		os.Exit(1)
	}
	l, err := net.Listen("unix", sock)
	if err != nil {
		os.Exit(1)
	}
	mux := http.NewServeMux()
	ok := func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusNoContent) }
	mux.HandleFunc("/boot-source", ok)
	mux.HandleFunc("/drives/rootfs", ok)
	mux.HandleFunc("/network-interfaces/eth0", ok)
	mux.HandleFunc("/machine-config", ok)
	mux.HandleFunc("/actions", ok)
	mux.HandleFunc("/vm", ok)
	_ = http.Serve(l, mux)
}

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

// installFakeDebugfs puts a fake `debugfs` binary on PATH that copies the
// script file it's given (its 3rd argument, from `-f <script>`) to
// recordPath and exits with exitCode.
func installFakeDebugfs(t *testing.T, recordPath string, exitCode int) {
	t.Helper()
	binDir := t.TempDir()
	script := fmt.Sprintf("#!/bin/sh\ncat \"$3\" > %q\nexit %d\n", recordPath, exitCode)
	require.NoError(t, os.WriteFile(filepath.Join(binDir, "debugfs"), []byte(script), 0755))
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))
}

func TestExpandHome(t *testing.T) {
	home, err := os.UserHomeDir()
	require.NoError(t, err)

	assert.Equal(t, home, expandHome("~"))
	assert.Equal(t, filepath.Join(home, "foo/bar"), expandHome("~/foo/bar"))
	assert.Equal(t, "/abs/path", expandHome("/abs/path"))
	assert.Equal(t, "relative/path", expandHome("relative/path"))
	assert.Equal(t, "~user/foo", expandHome("~user/foo"))
}

func TestGetConfig_Defaults(t *testing.T) {
	viper.Reset()
	t.Cleanup(viper.Reset)

	home, err := os.UserHomeDir()
	require.NoError(t, err)

	cfg := GetConfig()
	assert.Equal(t, filepath.Join(home, ".onctl", "firecracker"), cfg.StateDir)
	assert.Equal(t, int64(1), cfg.VCPUCount)
	assert.Equal(t, int64(512), cfg.MemSizeMib)
	assert.Equal(t, "fcbr0", cfg.Bridge)
	assert.Equal(t, "172.16.0.1/24", cfg.CIDR)
	assert.Equal(t, "root", cfg.Username)
	assert.Equal(t, "firecracker", cfg.BinPath)
	assert.Empty(t, cfg.KernelImage)
	assert.Empty(t, cfg.RootfsImage)
}

func TestGetConfig_CustomValues(t *testing.T) {
	viper.Reset()
	t.Cleanup(viper.Reset)

	home, err := os.UserHomeDir()
	require.NoError(t, err)

	viper.Set("firecracker.stateDir", "~/custom-state")
	viper.Set("firecracker.kernelImage", "~/images/vmlinux")
	viper.Set("firecracker.rootfsImage", "~/images/rootfs.ext4")
	viper.Set("firecracker.kernelArgs", "console=ttyS0")
	viper.Set("firecracker.vcpuCount", 4)
	viper.Set("firecracker.memSizeMib", 2048)
	viper.Set("firecracker.network.bridge", "mybr0")
	viper.Set("firecracker.network.cidr", "10.0.0.1/24")
	viper.Set("firecracker.vm.username", "ubuntu")
	viper.Set("firecracker.binPath", "/usr/local/bin/firecracker")

	cfg := GetConfig()
	assert.Equal(t, filepath.Join(home, "custom-state"), cfg.StateDir)
	assert.Equal(t, filepath.Join(home, "images/vmlinux"), cfg.KernelImage)
	assert.Equal(t, filepath.Join(home, "images/rootfs.ext4"), cfg.RootfsImage)
	assert.Equal(t, "console=ttyS0", cfg.KernelArgs)
	assert.Equal(t, int64(4), cfg.VCPUCount)
	assert.Equal(t, int64(2048), cfg.MemSizeMib)
	assert.Equal(t, "mybr0", cfg.Bridge)
	assert.Equal(t, "10.0.0.1/24", cfg.CIDR)
	assert.Equal(t, "ubuntu", cfg.Username)
	assert.Equal(t, "/usr/local/bin/firecracker", cfg.BinPath)
}

func TestUnixHTTPClient(t *testing.T) {
	client := unixHTTPClient("/tmp/does-not-matter.sock")
	assert.Equal(t, 5*time.Second, client.Timeout)
}

func TestFirecrackerRequest(t *testing.T) {
	sock := filepath.Join(t.TempDir(), "api.sock")

	mux := http.NewServeMux()
	mux.HandleFunc("/ok", func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodPut, r.Method)
		w.WriteHeader(http.StatusNoContent)
	})
	mux.HandleFunc("/fail", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"fault_message":"boom"}`))
	})
	startUnixServer(t, sock, mux)

	client := unixHTTPClient(sock)

	require.NoError(t, firecrackerRequest(client, http.MethodPut, "/ok", map[string]string{"a": "b"}))

	err := firecrackerRequest(client, http.MethodPut, "/fail", nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "boom")
	assert.Contains(t, err.Error(), "/fail")
}

func TestFirecrackerRequest_MarshalError(t *testing.T) {
	client := unixHTTPClient("/tmp/does-not-matter.sock")
	err := firecrackerRequest(client, http.MethodPut, "/x", make(chan int))
	assert.Error(t, err)
}

func TestFirecrackerRequest_ConnectionError(t *testing.T) {
	client := unixHTTPClient(filepath.Join(t.TempDir(), "nonexistent.sock"))
	err := firecrackerRequest(client, http.MethodPut, "/x", nil)
	assert.Error(t, err)
}

func TestConfigureAndBoot(t *testing.T) {
	sock := filepath.Join(t.TempDir(), "api.sock")

	var gotPaths []string
	mux := http.NewServeMux()
	record := func(path string) func(http.ResponseWriter, *http.Request) {
		return func(w http.ResponseWriter, _ *http.Request) {
			gotPaths = append(gotPaths, path)
			w.WriteHeader(http.StatusNoContent)
		}
	}
	mux.HandleFunc("/boot-source", record("/boot-source"))
	mux.HandleFunc("/drives/rootfs", record("/drives/rootfs"))
	mux.HandleFunc("/network-interfaces/eth0", record("/network-interfaces/eth0"))
	mux.HandleFunc("/machine-config", record("/machine-config"))
	mux.HandleFunc("/actions", record("/actions"))
	startUnixServer(t, sock, mux)

	cfg := cloud.FirecrackerVMConfig{
		KernelImage: "/images/vmlinux",
		KernelArgs:  "console=ttyS0",
		RootfsPath:  "/vms/test/rootfs.ext4",
		VCPUCount:   1,
		MemSizeMib:  512,
		TapDevice:   "fc0123456789abc",
		MacAddress:  "02:FC:00:00:00:00",
	}
	require.NoError(t, configureAndBoot(sock, cfg))
	assert.Equal(t, []string{"/boot-source", "/drives/rootfs", "/network-interfaces/eth0", "/machine-config", "/actions"}, gotPaths)
}

func TestConfigureAndBoot_Error(t *testing.T) {
	sock := filepath.Join(t.TempDir(), "api.sock")

	mux := http.NewServeMux()
	mux.HandleFunc("/boot-source", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	})
	startUnixServer(t, sock, mux)

	err := configureAndBoot(sock, cloud.FirecrackerVMConfig{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "boot-source")
}

func TestWaitForSocket(t *testing.T) {
	path := filepath.Join(t.TempDir(), "exists.sock")
	require.NoError(t, os.WriteFile(path, nil, 0600))
	require.NoError(t, waitForSocket(path, time.Second))
}

func TestWaitForSocket_Timeout(t *testing.T) {
	path := filepath.Join(t.TempDir(), "missing.sock")
	err := waitForSocket(path, 100*time.Millisecond)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "timed out")
}

func TestNewProcessManager(t *testing.T) {
	assert.Equal(t, ProcessManager{BinPath: "firecracker"}, NewProcessManager(""))
	assert.Equal(t, ProcessManager{BinPath: "/usr/local/bin/firecracker"}, NewProcessManager("/usr/local/bin/firecracker"))
}

func TestProcessManager_Start_BinaryNotFound(t *testing.T) {
	dir := t.TempDir()
	pm := NewProcessManager(filepath.Join(dir, "no-such-binary"))
	_, err := pm.Start(filepath.Join(dir, "api.sock"), cloud.FirecrackerVMConfig{}, filepath.Join(dir, "fc.log"))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to start")
}

func TestProcessManager_IsRunning(t *testing.T) {
	pm := ProcessManager{}
	assert.False(t, pm.IsRunning(0))
	assert.False(t, pm.IsRunning(-1))
	assert.True(t, pm.IsRunning(os.Getpid()))
}

func TestProcessManager_Stop_InvalidPID(t *testing.T) {
	pm := ProcessManager{}
	assert.NoError(t, pm.Stop(0))
	assert.NoError(t, pm.Stop(-1))
}

// TestProcessManager_StartStopIsRunning re-execs the test binary as a fake
// firecracker process (see runFakeFirecracker) to exercise Start's full
// success path along with Stop and IsRunning.
func TestProcessManager_StartStopIsRunning(t *testing.T) {
	t.Setenv(fakeFirecrackerEnv, "1")

	// Use a short path under os.TempDir() rather than t.TempDir(): the long,
	// test-name-derived paths from t.TempDir() can exceed the ~104 byte
	// AF_UNIX path limit on macOS, causing net.Listen("unix", ...) to fail.
	dir, err := os.MkdirTemp("", "fcproc")
	require.NoError(t, err)
	t.Cleanup(func() { _ = os.RemoveAll(dir) })

	pm := NewProcessManager(os.Args[0])
	cfg := cloud.FirecrackerVMConfig{
		KernelImage: "/images/vmlinux",
		RootfsPath:  "/vms/test/rootfs.ext4",
		VCPUCount:   1,
		MemSizeMib:  512,
		TapDevice:   "fctest",
		MacAddress:  "02:FC:00:00:00:00",
	}

	pid, err := pm.Start(filepath.Join(dir, "api.sock"), cfg, filepath.Join(dir, "fc.log"))
	require.NoError(t, err)
	assert.Greater(t, pid, 0)
	assert.True(t, pm.IsRunning(pid))

	// pm.Start releases the child process, so the test process must reap it
	// itself once it exits, otherwise it lingers as a zombie and IsRunning
	// (which uses kill -0) keeps reporting it as running.
	reaped := make(chan struct{})
	go func() {
		if proc, err := os.FindProcess(pid); err == nil {
			_, _ = proc.Wait()
		}
		close(reaped)
	}()

	require.NoError(t, pm.Stop(pid))
	<-reaped
	assert.False(t, pm.IsRunning(pid))
}

func TestProcessManager_Owns(t *testing.T) {
	pm := ProcessManager{}
	assert.False(t, pm.Owns(0, "/tmp/api.sock"))
	assert.False(t, pm.Owns(1234, ""))
	assert.False(t, pm.Owns(999999999, "/tmp/api.sock"))

	if _, err := os.Stat("/proc/self/cmdline"); err != nil {
		t.Skip("/proc/<pid>/cmdline not available on this platform")
	}

	dir, err := os.MkdirTemp("", "fcowns")
	require.NoError(t, err)
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	sock := filepath.Join(dir, "api.sock")

	cmd := exec.Command(os.Args[0], "--api-sock", sock)
	cmd.Env = append(os.Environ(), fakeFirecrackerEnv+"=1")
	require.NoError(t, cmd.Start())
	pid := cmd.Process.Pid
	t.Cleanup(func() {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
	})

	require.NoError(t, waitForSocket(sock, 5*time.Second))

	assert.True(t, pm.Owns(pid, sock))
	assert.False(t, pm.Owns(pid, filepath.Join(dir, "other.sock")))
}

func TestNewAPIClient(t *testing.T) {
	assert.Equal(t, APIClient{}, NewAPIClient())
}

func TestAPIClient_SetState(t *testing.T) {
	sock := filepath.Join(t.TempDir(), "api.sock")

	var gotState string
	mux := http.NewServeMux()
	mux.HandleFunc("/vm", func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodPatch, r.Method)
		var body map[string]string
		require.NoError(t, json.NewDecoder(r.Body).Decode(&body))
		gotState = body["state"]
		w.WriteHeader(http.StatusNoContent)
	})
	startUnixServer(t, sock, mux)

	api := NewAPIClient()
	require.NoError(t, api.SetState(sock, "Paused"))
	assert.Equal(t, "Paused", gotState)
}

func TestNewNetworkManager(t *testing.T) {
	assert.Equal(t, LinuxNetworkManager{}, NewNetworkManager())
}

func TestLinkExists_NotFound(t *testing.T) {
	if _, err := exec.LookPath("ip"); err != nil {
		t.Skip("ip command not available")
	}
	assert.False(t, linkExists("onctl-test-nonexistent0"))
}

func TestRunIP_Error(t *testing.T) {
	if _, err := exec.LookPath("ip"); err != nil {
		t.Skip("ip command not available")
	}
	err := runIP("definitely-not-a-subcommand")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "ip definitely-not-a-subcommand")
}

func TestLinuxNetworkManager_DeleteTap_NotExist(t *testing.T) {
	if _, err := exec.LookPath("ip"); err != nil {
		t.Skip("ip command not available")
	}
	nm := LinuxNetworkManager{}
	assert.NoError(t, nm.DeleteTap("onctl-test-nonexistent0"))
}

func TestNewRootfsPreparer(t *testing.T) {
	assert.Equal(t, DebugfsRootfsPreparer{}, NewRootfsPreparer())
}

func TestCopyFile(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "src")
	dst := filepath.Join(dir, "dst")
	require.NoError(t, os.WriteFile(src, []byte("hello"), 0644))

	require.NoError(t, copyFile(src, dst))
	data, err := os.ReadFile(dst)
	require.NoError(t, err)
	assert.Equal(t, "hello", string(data))
}

func TestCopyFile_MissingSource(t *testing.T) {
	dir := t.TempDir()
	err := copyFile(filepath.Join(dir, "nope"), filepath.Join(dir, "dst"))
	assert.Error(t, err)
}

func TestDebugfsRootfsPreparer_Prepare_NoBaseImage(t *testing.T) {
	p := NewRootfsPreparer()
	err := p.Prepare("", filepath.Join(t.TempDir(), "rootfs.ext4"), "ssh-key", "root")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "rootfsImage is not configured")
}

func TestDebugfsRootfsPreparer_Prepare_CopyError(t *testing.T) {
	dir := t.TempDir()
	p := NewRootfsPreparer()
	err := p.Prepare(filepath.Join(dir, "no-such-base.ext4"), filepath.Join(dir, "rootfs.ext4"), "", "root")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to copy base rootfs")
}

func TestDebugfsRootfsPreparer_Prepare_NoSSHKey(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "base.ext4")
	dst := filepath.Join(dir, "rootfs.ext4")
	require.NoError(t, os.WriteFile(src, []byte("image-data"), 0644))

	p := NewRootfsPreparer()
	require.NoError(t, p.Prepare(src, dst, "", "root"))

	data, err := os.ReadFile(dst)
	require.NoError(t, err)
	assert.Equal(t, "image-data", string(data))
}

func TestDebugfsRootfsPreparer_Prepare_InjectsKey(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "base.ext4")
	dst := filepath.Join(dir, "rootfs.ext4")
	require.NoError(t, os.WriteFile(src, []byte("image-data"), 0644))

	calls := filepath.Join(dir, "debugfs-calls.txt")
	installFakeDebugfs(t, calls, 0)

	p := NewRootfsPreparer()
	require.NoError(t, p.Prepare(src, dst, "ssh-ed25519 AAAA... user@host", "root"))

	script, err := os.ReadFile(calls)
	require.NoError(t, err)
	assert.Contains(t, string(script), "mkdir /root/.ssh\n")
	assert.Contains(t, string(script), "rm /root/.ssh/authorized_keys\n")
	assert.Contains(t, string(script), "/root/.ssh/authorized_keys\n")
	assert.Contains(t, string(script), "sif /root/.ssh/authorized_keys mode 0100600\n")
	assert.Contains(t, string(script), "sif /root/.ssh mode 040700\n")
}

func TestDebugfsRootfsPreparer_Prepare_NonRootUser(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "base.ext4")
	dst := filepath.Join(dir, "rootfs.ext4")
	require.NoError(t, os.WriteFile(src, []byte("image-data"), 0644))

	calls := filepath.Join(dir, "debugfs-calls.txt")
	installFakeDebugfs(t, calls, 0)

	p := NewRootfsPreparer()
	require.NoError(t, p.Prepare(src, dst, "ssh-ed25519 AAAA... user@host", "ubuntu"))

	script, err := os.ReadFile(calls)
	require.NoError(t, err)
	assert.Contains(t, string(script), "/home/ubuntu/.ssh")
}

func TestDebugfsRootfsPreparer_Prepare_DebugfsFails(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "base.ext4")
	dst := filepath.Join(dir, "rootfs.ext4")
	require.NoError(t, os.WriteFile(src, []byte("image-data"), 0644))

	installFakeDebugfs(t, filepath.Join(dir, "calls.txt"), 1)

	p := NewRootfsPreparer()
	err := p.Prepare(src, dst, "ssh-ed25519 AAAA... user@host", "root")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "debugfs failed")
}
