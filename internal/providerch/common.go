// Package providerch provides the host-side implementations (process
// management, networking, rootfs preparation) backing cloud.ProviderCH,
// plus configuration loading from viper.
package providerch

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

// defaultCHStateDir is used when ch.stateDir isn't configured. Cloud
// Hypervisor microVM state is host-local, not per-user (Deploy needs
// CAP_NET_ADMIN to set up the bridge/TAP devices anyway, so it's only ever
// usable as root or a similarly privileged account) — a fixed system path
// avoids every user account ending up with its own, inconsistent view of the
// same host's VMs.
const defaultCHStateDir = "/opt/ch"

// GetConfig reads ch.* settings from viper, applying defaults for
// anything that isn't set.
func GetConfig() cloud.CHConfig {
	stateDir := viper.GetString("ch.stateDir")
	if stateDir == "" {
		stateDir = defaultCHStateDir
	} else {
		stateDir = expandHome(stateDir)
	}

	vcpu := viper.GetInt64("ch.vcpuCount")
	if vcpu == 0 {
		vcpu = 1
	}
	mem := viper.GetInt64("ch.memSizeMib")
	if mem == 0 {
		mem = 2048
	}

	bridge := viper.GetString("ch.network.bridge")
	cidr := viper.GetString("ch.network.cidr")

	if bridge != "" && strings.Contains(bridge, "/") {
		parts := strings.SplitN(bridge, "/", 2)
		bridge = parts[0]
		if parts[1] != "" {
			cidr = parts[1]
		}
	}

	if bridge == "" {
		bridge = "chbr0"
	}
	if cidr == "" {
		cidr = "172.17.0.1/24"
	}

	username := viper.GetString("ch.vm.username")
	if username == "" {
		username = "root"
	}
	binPath := viper.GetString("ch.binPath")
	if binPath == "" {
		binPath = "cloud-hypervisor"
	}

	return cloud.CHConfig{
		KernelImage: expandHome(viper.GetString("ch.kernelImage")),
		RootfsImage: expandHome(viper.GetString("ch.rootfsImage")),
		KernelArgs:  viper.GetString("ch.kernelArgs"),
		VCPUCount:   vcpu,
		MemSizeMib:  mem,
		Bridge:      bridge,
		CIDR:        cidr,
		Username:    username,
		BinPath:     binPath,
		StateDir:    stateDir,
	}
}

// unixHTTPClient returns an HTTP client that talks to the Cloud Hypervisor
// API over its Unix domain socket.
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

// chAPIBase is the fixed base path Cloud Hypervisor's REST API is served
// under, regardless of the (irrelevant, since it's a Unix socket) host/port
// in the URL.
const chAPIBase = "http://localhost/api/v1"

// chRequest issues an HTTP request against the Cloud Hypervisor API socket.
// A nil body sends the request with no payload, for endpoints that take
// none (e.g. vm.boot).
func chRequest(client *http.Client, method, path string, body any) error {
	var reader io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			return err
		}
		reader = bytes.NewReader(data)
	}
	req, err := http.NewRequest(method, chAPIBase+path, reader)
	if err != nil {
		return err
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode >= 300 {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("cloud-hypervisor API %s %s returned %s: %s", method, path, resp.Status, string(respBody))
	}
	return nil
}

// chVMConfigPayload is the body of a PUT /vm.create request. Only the fields
// onctl needs are modeled; Cloud Hypervisor defaults everything else.
type chVMConfigPayload struct {
	CPUs    chCPUsConfig    `json:"cpus"`
	Memory  chMemoryConfig  `json:"memory"`
	Payload chPayloadConfig `json:"payload"`
	Disks   []chDiskConfig  `json:"disks"`
	Net     []chNetConfig   `json:"net"`
	Serial  chConsoleConfig `json:"serial"`
	Console chConsoleConfig `json:"console"`
}

type chCPUsConfig struct {
	BootVCPUs int64 `json:"boot_vcpus"`
	MaxVCPUs  int64 `json:"max_vcpus"`
}

type chMemoryConfig struct {
	// Size is in bytes.
	Size int64 `json:"size"`
}

// chPayloadConfig is Cloud Hypervisor's PayloadConfig: unlike Firecracker,
// the kernel path and boot command line are plain string fields nested
// under "payload", not top-level "kernel"/"cmdline" objects.
type chPayloadConfig struct {
	Kernel  string `json:"kernel"`
	Cmdline string `json:"cmdline,omitempty"`
}

type chDiskConfig struct {
	Path string `json:"path"`
}

type chNetConfig struct {
	Tap string `json:"tap"`
	Mac string `json:"mac,omitempty"`
}

type chConsoleConfig struct {
	Mode string `json:"mode"`
}

// configureAndBoot configures a freshly started cloud-hypervisor VMM over
// its API socket and boots the microVM. Unlike Firecracker's per-resource
// PUT calls, Cloud Hypervisor takes the whole VM config in one vm.create
// call and boots it with a separate, bodyless vm.boot call.
func configureAndBoot(socketPath string, cfg cloud.CHVMConfig) error {
	client := unixHTTPClient(socketPath)

	payload := chVMConfigPayload{
		CPUs:    chCPUsConfig{BootVCPUs: cfg.VCPUCount, MaxVCPUs: cfg.VCPUCount},
		Memory:  chMemoryConfig{Size: cfg.MemSizeMib * 1024 * 1024},
		Payload: chPayloadConfig{Kernel: cfg.KernelImage, Cmdline: cfg.KernelArgs},
		Disks:   []chDiskConfig{{Path: cfg.RootfsPath}},
		Net:     []chNetConfig{{Tap: cfg.TapDevice, Mac: cfg.MacAddress}},
		// Tty: guest serial console output is written to the VMM process's
		// own stdout, which spawn() redirects to logFile, mirroring how
		// Firecracker's console output ends up in its log file.
		Serial:  chConsoleConfig{Mode: "Tty"},
		Console: chConsoleConfig{Mode: "Off"},
	}
	if err := chRequest(client, http.MethodPut, "/vm.create", payload); err != nil {
		return fmt.Errorf("vm.create: %w", err)
	}
	if err := chRequest(client, http.MethodPut, "/vm.boot", nil); err != nil {
		return fmt.Errorf("vm.boot: %w", err)
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
	return fmt.Errorf("timed out waiting for cloud-hypervisor API socket %q", path)
}

// ProcessManager is the real cloud.CHProcess implementation: it spawns the
// cloud-hypervisor binary, configures it over its API socket and manages the
// resulting OS process.
type ProcessManager struct {
	BinPath string
}

// NewProcessManager returns a cloud.CHProcess backed by the cloud-hypervisor
// binary at binPath ("cloud-hypervisor" if empty).
func NewProcessManager(binPath string) cloud.CHProcess {
	if binPath == "" {
		binPath = "cloud-hypervisor"
	}
	return ProcessManager{BinPath: binPath}
}

func (m ProcessManager) Start(socketPath string, cfg cloud.CHVMConfig, logFile string) (int, error) {
	pid, err := m.spawn(socketPath, logFile)
	if err != nil {
		return 0, err
	}
	if err := configureAndBoot(socketPath, cfg); err != nil {
		_ = m.Stop(pid)
		return 0, err
	}
	return pid, nil
}

// spawn launches the cloud-hypervisor binary bound to socketPath and waits
// for its API socket to come up, without configuring or booting it.
func (m ProcessManager) spawn(socketPath, logFile string) (int, error) {
	_ = os.Remove(socketPath)

	// 0600: guest serial console output can contain sensitive boot/runtime
	// data, and now lives under the world-traversable shared StateDir.
	logFd, err := os.OpenFile(logFile, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0600)
	if err != nil {
		return 0, fmt.Errorf("failed to open log file %q: %w", logFile, err)
	}
	defer func() { _ = logFd.Close() }()

	cmd := exec.Command(m.BinPath, "--api-socket", socketPath)
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
	// Signaling with 0 delivers nothing but still probes existence. See
	// providerfc.ProcessManager.IsRunning for why EPERM also counts as alive.
	err = process.Signal(syscall.Signal(0))
	return err == nil || errors.Is(err, syscall.EPERM)
}

// Owns reports whether pid is a cloud-hypervisor process bound to
// socketPath, by checking its command line for a "--api-socket socketPath"
// argument pair. This guards against a persisted PID having been reused by
// an unrelated host process after a VMM exit or host reboot.
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
		if arg == "--api-socket" && i+1 < len(args) && args[i+1] == socketPath {
			return true
		}
	}
	return false
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

func (DebugfsRootfsPreparer) Prepare(baseImage, destPath, hostname, sshPublicKey, username string) error {
	if baseImage == "" {
		return errors.New("ch.rootfsImage is not configured")
	}
	if err := copyRootfs(baseImage, destPath); err != nil {
		return fmt.Errorf("failed to copy base rootfs %q: %w", baseImage, err)
	}
	if hostname != "" {
		if err := injectHostname(destPath, hostname); err != nil {
			return err
		}
	}
	if sshPublicKey == "" {
		return nil
	}
	return injectSSHKey(destPath, sshPublicKey, username)
}

// copyRootfs creates dst as a copy of src, preferring a copy-on-write
// reflink (near-instant and disk-space-free regardless of image size) and
// falling back to a full byte-for-byte copy when the host filesystem
// doesn't support reflinks (e.g. plain ext4, or non-Linux).
func copyRootfs(src, dst string) error {
	if err := reflinkCopy(src, dst); err == nil {
		// `cp --reflink` preserves src's mode (typically a world/group-readable
		// base image), but dst is a per-VM writable rootfs holding guest
		// secrets under the shared, world-traversable StateDir — lock it down
		// to match copyFile's fallback below, which os.OpenFile's it 0600.
		return os.Chmod(dst, 0600)
	}
	return copyFile(src, dst)
}

func reflinkCopy(src, dst string) error {
	out, err := exec.Command("cp", "--reflink=always", src, dst).CombinedOutput()
	if err != nil {
		return fmt.Errorf("cp --reflink=always %s %s failed (requires a CoW filesystem like btrfs): %w: %s", src, dst, err, strings.TrimSpace(string(out)))
	}
	return nil
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
	return runDebugfsScript(rootfsPath, script)
}

// injectHostname writes hostname to /etc/hostname inside the ext-family
// image at rootfsPath using debugfs, without mounting the image.
func injectHostname(rootfsPath, hostname string) error {
	hostnameFile, err := os.CreateTemp("", "onctl-hostname-*")
	if err != nil {
		return err
	}
	defer func() { _ = os.Remove(hostnameFile.Name()) }()
	if _, err := hostnameFile.WriteString(hostname + "\n"); err != nil {
		_ = hostnameFile.Close()
		return err
	}
	if err := hostnameFile.Close(); err != nil {
		return err
	}

	script := fmt.Sprintf(
		"rm /etc/hostname\nwrite %s /etc/hostname\nsif /etc/hostname mode 0100644\n",
		hostnameFile.Name(),
	)
	return runDebugfsScript(rootfsPath, script)
}

// runDebugfsScript writes script to a temp file and runs `debugfs -w` with
// it against rootfsPath, shared by injectSSHKey and injectHostname.
func runDebugfsScript(rootfsPath, script string) error {
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
