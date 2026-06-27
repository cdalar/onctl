// Package providerfc provides the host-side implementations (process
// management, networking, rootfs preparation and API access) backing
// cloud.ProviderFC, plus configuration loading from viper.
package providerfc

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/cdalar/onctl/pkg/cloud"
	"github.com/spf13/viper"
)

// expandHome expands a leading "~" or "~/" in path to the user's home
// directory. Config values come from a YAML file, so the shell never gets a
// chance to expand "~" itself.
func expandHome(path string) string {
	home, err := os.UserHomeDir()
	if err != nil {
		return path
	}
	if path == "~" {
		return home
	}
	if strings.HasPrefix(path, "~/") {
		return filepath.Join(home, path[2:])
	}
	return path
}

// GetConfig reads fc.* settings from viper, applying defaults for
// anything that isn't set.
func GetConfig() cloud.FCConfig {
	stateDir := viper.GetString("fc.stateDir")
	if stateDir == "" {
		if home, err := os.UserHomeDir(); err == nil {
			stateDir = filepath.Join(home, ".onctl", "firecracker")
		}
	} else {
		stateDir = expandHome(stateDir)
	}

	vcpu := viper.GetInt64("fc.vcpuCount")
	if vcpu == 0 {
		vcpu = 1
	}
	mem := viper.GetInt64("fc.memSizeMib")
	if mem == 0 {
		mem = 2048
	}
	bridge := viper.GetString("fc.network.bridge")
	if bridge == "" {
		bridge = "fcbr0"
	}
	cidr := viper.GetString("fc.network.cidr")
	if cidr == "" {
		cidr = "172.16.0.1/24"
	}
	username := viper.GetString("fc.vm.username")
	if username == "" {
		username = "root"
	}
	binPath := viper.GetString("fc.binPath")
	if binPath == "" {
		binPath = "firecracker"
	}

	return cloud.FCConfig{
		KernelImage: expandHome(viper.GetString("fc.kernelImage")),
		RootfsImage: expandHome(viper.GetString("fc.rootfsImage")),
		KernelArgs:  viper.GetString("fc.kernelArgs"),
		VCPUCount:   vcpu,
		MemSizeMib:  mem,
		Bridge:      bridge,
		CIDR:        cidr,
		Username:    username,
		BinPath:     binPath,
		StateDir:    stateDir,
	}
}

// unixHTTPClient returns an HTTP client that talks to the Firecracker API
// over its Unix domain socket.
func unixHTTPClient(socketPath string) *http.Client {
	return &http.Client{
		Transport: &http.Transport{
			DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
				return (&net.Dialer{}).DialContext(ctx, "unix", socketPath)
			},
		},
		Timeout: 5 * time.Second,
	}
}

func fcRequest(client *http.Client, method, path string, body any) error {
	data, err := json.Marshal(body)
	if err != nil {
		return err
	}
	req, err := http.NewRequest(method, "http://unix"+path, bytes.NewReader(data))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode >= 300 {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("firecracker API %s %s returned %s: %s", method, path, resp.Status, string(respBody))
	}
	return nil
}

// configureAndBoot configures a freshly started firecracker VMM over its API
// socket and starts the microVM.
func configureAndBoot(socketPath string, cfg cloud.FCVMConfig) error {
	client := unixHTTPClient(socketPath)

	if err := fcRequest(client, http.MethodPut, "/boot-source", map[string]string{
		"kernel_image_path": cfg.KernelImage,
		"boot_args":         cfg.KernelArgs,
	}); err != nil {
		return fmt.Errorf("boot-source: %w", err)
	}

	if err := fcRequest(client, http.MethodPut, "/drives/rootfs", map[string]any{
		"drive_id":       "rootfs",
		"path_on_host":   cfg.RootfsPath,
		"is_root_device": true,
		"is_read_only":   false,
	}); err != nil {
		return fmt.Errorf("drives/rootfs: %w", err)
	}

	if err := fcRequest(client, http.MethodPut, "/network-interfaces/eth0", map[string]string{
		"iface_id":      "eth0",
		"host_dev_name": cfg.TapDevice,
		"guest_mac":     cfg.MacAddress,
	}); err != nil {
		return fmt.Errorf("network-interfaces: %w", err)
	}

	if err := fcRequest(client, http.MethodPut, "/machine-config", map[string]any{
		"vcpu_count":   cfg.VCPUCount,
		"mem_size_mib": cfg.MemSizeMib,
	}); err != nil {
		return fmt.Errorf("machine-config: %w", err)
	}

	if err := fcRequest(client, http.MethodPut, "/actions", map[string]string{
		"action_type": "InstanceStart",
	}); err != nil {
		return fmt.Errorf("start instance: %w", err)
	}

	return nil
}

func waitForSocket(path string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if _, err := os.Stat(path); err == nil {
			return nil
		}
		time.Sleep(50 * time.Millisecond)
	}
	return fmt.Errorf("timed out waiting for firecracker API socket %q", path)
}

// ProcessManager is the real cloud.FCProcess implementation: it
// spawns the firecracker binary, configures it over its API socket and
// manages the resulting OS process.
type ProcessManager struct {
	BinPath string
}

// NewProcessManager returns a cloud.FCProcess backed by the
// firecracker binary at binPath ("firecracker" if empty).
func NewProcessManager(binPath string) cloud.FCProcess {
	if binPath == "" {
		binPath = "firecracker"
	}
	return ProcessManager{BinPath: binPath}
}

func (m ProcessManager) Start(socketPath string, cfg cloud.FCVMConfig, logFile string) (int, error) {
	_ = os.Remove(socketPath)

	logFd, err := os.OpenFile(logFile, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0644)
	if err != nil {
		return 0, fmt.Errorf("failed to open log file %q: %w", logFile, err)
	}
	defer func() { _ = logFd.Close() }()

	cmd := exec.Command(m.BinPath, "--api-sock", socketPath)
	cmd.Stdout = logFd
	cmd.Stderr = logFd
	setSysProcAttr(cmd)
	if err := cmd.Start(); err != nil {
		return 0, fmt.Errorf("failed to start %s: %w", m.BinPath, err)
	}
	pid := cmd.Process.Pid
	// Detach: the microVM process must outlive this onctl invocation.
	if err := cmd.Process.Release(); err != nil {
		return 0, err
	}

	if err := waitForSocket(socketPath, 5*time.Second); err != nil {
		_ = m.Stop(pid)
		return 0, err
	}

	if err := configureAndBoot(socketPath, cfg); err != nil {
		_ = m.Stop(pid)
		return 0, err
	}

	return pid, nil
}

func (m ProcessManager) Stop(pid int) error {
	if pid <= 0 {
		return nil
	}
	process, err := os.FindProcess(pid)
	if err != nil {
		return err
	}
	if err := process.Signal(syscall.SIGTERM); err != nil {
		if errors.Is(err, os.ErrProcessDone) {
			return nil
		}
		return err
	}
	for i := 0; i < 50; i++ {
		if !m.IsRunning(pid) {
			return nil
		}
		time.Sleep(100 * time.Millisecond)
	}
	return process.Signal(syscall.SIGKILL)
}

func (m ProcessManager) IsRunning(pid int) bool {
	if pid <= 0 {
		return false
	}
	process, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	return process.Signal(syscall.Signal(0)) == nil
}

// Owns reports whether pid is a firecracker process bound to socketPath, by
// checking its command line for a "--api-sock socketPath" argument pair.
// This guards against a persisted PID having been reused by an unrelated
// host process after a VMM exit or host reboot.
func (m ProcessManager) Owns(pid int, socketPath string) bool {
	if pid <= 0 || socketPath == "" {
		return false
	}
	data, err := os.ReadFile(fmt.Sprintf("/proc/%d/cmdline", pid))
	if err != nil {
		return false
	}
	args := strings.Split(strings.TrimRight(string(data), "\x00"), "\x00")
	for i, arg := range args {
		if arg == "--api-sock" && i+1 < len(args) && args[i+1] == socketPath {
			return true
		}
	}
	return false
}

// APIClient is the real cloud.FCAPI implementation.
type APIClient struct{}

// NewAPIClient returns a cloud.FCAPI that talks to firecracker over
// its API socket.
func NewAPIClient() cloud.FCAPI {
	return APIClient{}
}

func (APIClient) SetState(socketPath, state string) error {
	client := unixHTTPClient(socketPath)
	return fcRequest(client, http.MethodPatch, "/vm", map[string]string{"state": state})
}

// LinuxNetworkManager is the real cloud.NetworkManager implementation,
// shelling out to `ip` (iproute2). Creating bridges and TAP devices requires
// CAP_NET_ADMIN (typically root).
type LinuxNetworkManager struct{}

// NewNetworkManager returns a cloud.NetworkManager backed by `ip`.
func NewNetworkManager() cloud.NetworkManager {
	return LinuxNetworkManager{}
}

func (LinuxNetworkManager) EnsureBridge(bridge, cidr string) error {
	if linkExists(bridge) {
		return nil
	}
	if err := runIP("link", "add", "name", bridge, "type", "bridge"); err != nil {
		return err
	}
	if err := runIP("addr", "add", cidr, "dev", bridge); err != nil {
		return err
	}
	return runIP("link", "set", bridge, "up")
}

func (LinuxNetworkManager) CreateTap(tapName, bridge string) error {
	if linkExists(tapName) {
		return nil
	}
	if err := runIP("tuntap", "add", "dev", tapName, "mode", "tap"); err != nil {
		return err
	}
	if err := runIP("link", "set", tapName, "master", bridge); err != nil {
		return err
	}
	return runIP("link", "set", tapName, "up")
}

func (LinuxNetworkManager) DeleteTap(tapName string) error {
	if !linkExists(tapName) {
		return nil
	}
	return runIP("link", "delete", tapName)
}

func linkExists(name string) bool {
	return exec.Command("ip", "link", "show", name).Run() == nil
}

func runIP(args ...string) error {
	out, err := exec.Command("ip", args...).CombinedOutput()
	if err != nil {
		return fmt.Errorf("ip %s: %w: %s", strings.Join(args, " "), err, strings.TrimSpace(string(out)))
	}
	return nil
}

// DebugfsRootfsPreparer is the real cloud.RootfsPreparer implementation: it
// copies the base rootfs image and injects the SSH public key with debugfs,
// avoiding the need to mount the image (and therefore root) on the host.
type DebugfsRootfsPreparer struct{}

// NewRootfsPreparer returns a cloud.RootfsPreparer backed by `debugfs`.
func NewRootfsPreparer() cloud.RootfsPreparer {
	return DebugfsRootfsPreparer{}
}

func (DebugfsRootfsPreparer) Prepare(baseImage, destPath, sshPublicKey, username string) error {
	if baseImage == "" {
		return errors.New("fc.rootfsImage is not configured")
	}
	if err := copyFile(baseImage, destPath); err != nil {
		return fmt.Errorf("failed to copy base rootfs %q: %w", baseImage, err)
	}
	if sshPublicKey == "" {
		return nil
	}
	return injectSSHKey(destPath, sshPublicKey, username)
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer func() { _ = in.Close() }()

	out, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0600)
	if err != nil {
		return err
	}
	defer func() { _ = out.Close() }()

	_, err = io.Copy(out, in)
	return err
}

// injectSSHKey writes publicKey to <home>/.ssh/authorized_keys inside the
// ext-family image at rootfsPath using debugfs, without mounting the image.
func injectSSHKey(rootfsPath, publicKey, username string) error {
	homeDir := "/root"
	if username != "root" {
		homeDir = "/home/" + username
	}
	sshDir := homeDir + "/.ssh"

	keyFile, err := os.CreateTemp("", "onctl-authorized-keys-*")
	if err != nil {
		return err
	}
	defer func() { _ = os.Remove(keyFile.Name()) }()
	if _, err := keyFile.WriteString(publicKey + "\n"); err != nil {
		_ = keyFile.Close()
		return err
	}
	if err := keyFile.Close(); err != nil {
		return err
	}

	script := fmt.Sprintf(
		"mkdir %s\nrm %s/authorized_keys\nwrite %s %s/authorized_keys\nsif %s/authorized_keys mode 0100600\nsif %s mode 040700\n",
		sshDir, sshDir, keyFile.Name(), sshDir, sshDir, sshDir,
	)
	scriptFile, err := os.CreateTemp("", "onctl-debugfs-*.script")
	if err != nil {
		return err
	}
	defer func() { _ = os.Remove(scriptFile.Name()) }()
	if _, err := scriptFile.WriteString(script); err != nil {
		_ = scriptFile.Close()
		return err
	}
	if err := scriptFile.Close(); err != nil {
		return err
	}

	out, err := exec.Command("debugfs", "-w", "-f", scriptFile.Name(), rootfsPath).CombinedOutput()
	if err != nil {
		return fmt.Errorf("debugfs failed: %w: %s", err, strings.TrimSpace(string(out)))
	}
	return nil
}
