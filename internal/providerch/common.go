// Package providerch provides the host-side implementations (process
// management, networking, rootfs preparation) backing cloud.ProviderCH,
// plus configuration loading from viper.
package providerch

import (
	"bytes"
	"context"
	"crypto/md5"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
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

	guestOS := strings.ToLower(strings.TrimSpace(viper.GetString("ch.os")))
	if guestOS == "" {
		guestOS = "linux"
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
		OS:          guestOS,
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
	// KvmHyperv enables Hyper-V enlightenments, required for Windows
	// guests (confirmed against cloud-hypervisor's CpusConfig OpenAPI
	// schema; omitted entirely for Linux guests via omitempty).
	KvmHyperv bool `json:"kvm_hyperv,omitempty"`
}

type chMemoryConfig struct {
	// Size is in bytes.
	Size int64 `json:"size"`
}

// chPayloadConfig is Cloud Hypervisor's PayloadConfig: unlike Firecracker,
// the kernel path and boot command line are plain string fields nested
// under "payload", not top-level "kernel"/"cmdline" objects. Firmware is
// the UEFI boot path (mutually exclusive with Kernel/Cmdline, confirmed
// against PayloadConfig's OpenAPI schema) used for Windows guests, which
// have no direct-kernel-boot path.
type chPayloadConfig struct {
	Firmware string `json:"firmware,omitempty"`
	Kernel   string `json:"kernel,omitempty"`
	Cmdline  string `json:"cmdline,omitempty"`
}

type chDiskConfig struct {
	Path string `json:"path"`
	// Readonly marks a disk as read-only — used for the Windows NoCloud
	// seed ISO, which must never be written to.
	Readonly bool `json:"readonly,omitempty"`
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

	payloadCfg := chPayloadConfig{Kernel: cfg.KernelImage, Cmdline: cfg.KernelArgs}
	if cfg.Firmware {
		payloadCfg = chPayloadConfig{Firmware: cfg.KernelImage}
	}

	disks := []chDiskConfig{{Path: cfg.RootfsPath}}
	if cfg.SeedDiskPath != "" {
		disks = append(disks, chDiskConfig{Path: cfg.SeedDiskPath, Readonly: true})
	}

	payload := chVMConfigPayload{
		CPUs:    chCPUsConfig{BootVCPUs: cfg.VCPUCount, MaxVCPUs: cfg.VCPUCount, KvmHyperv: cfg.KvmHyperv},
		Memory:  chMemoryConfig{Size: cfg.MemSizeMib * 1024 * 1024},
		Payload: payloadCfg,
		Disks:   disks,
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

// ConfigDriveSeedPreparer is the real cloud.WindowsGuestPreparer
// implementation: it copies the base Windows disk image unmodified (Windows
// disks are NTFS, which onctl has no in-process tooling to modify — unlike
// DebugfsRootfsPreparer's ext4 debugfs trick, so it never touches the guest
// disk at all) and builds an OpenStack config-drive-format seed ISO for
// cloudbase-init (which must be pre-installed in the base image, alongside
// virtio-win drivers — see docs) to apply hostname/SSH-key configuration
// from at boot.
//
// UNVERIFIED: this has not been exercised against a real cloudbase-init
// installation. The config-drive layout and meta_data.json fields below
// (openstack/latest/meta_data.json + public_keys) match OpenStack's and
// cloudbase-init's own documented ConfigDrive datasource, but should be
// confirmed by hand-booting one seed ISO against the actual cloudbase-init
// version in use before relying on this (see the plan's Verification § 0).
type ConfigDriveSeedPreparer struct{}

// NewWindowsGuestPreparer returns a cloud.WindowsGuestPreparer backed by
// ConfigDriveSeedPreparer.
func NewWindowsGuestPreparer() cloud.WindowsGuestPreparer {
	return ConfigDriveSeedPreparer{}
}

func (ConfigDriveSeedPreparer) PrepareDisk(baseImage, destPath string) error {
	if baseImage == "" {
		return errors.New("ch.rootfsImage is not configured")
	}
	return copyRootfs(baseImage, destPath)
}

// configDriveMetadata mirrors OpenStack's meta_data.json (the subset
// cloudbase-init's ConfigDrive datasource reads).
type configDriveMetadata struct {
	UUID        string            `json:"uuid"`
	Hostname    string            `json:"hostname"`
	Name        string            `json:"name"`
	PublicKeys  map[string]string `json:"public_keys,omitempty"`
	LaunchIndex int               `json:"launch_index"`
}

func (ConfigDriveSeedPreparer) BuildSeedISO(destPath, hostname, sshPublicKey, username string) error {
	seedDir, err := os.MkdirTemp("", "onctl-ch-seed-*")
	if err != nil {
		return err
	}
	defer func() { _ = os.RemoveAll(seedDir) }()

	osDir := filepath.Join(seedDir, "openstack", "latest")
	if err := os.MkdirAll(osDir, 0755); err != nil {
		return err
	}

	meta := configDriveMetadata{
		UUID:     configDriveUUID(hostname),
		Hostname: hostname,
		Name:     hostname,
	}
	if sshPublicKey != "" {
		if username == "" {
			username = "root"
		}
		meta.PublicKeys = map[string]string{username: sshPublicKey}
	}
	metaJSON, err := json.MarshalIndent(meta, "", "  ")
	if err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(osDir, "meta_data.json"), metaJSON, 0644); err != nil {
		return err
	}
	// user_data is optional per the config-drive spec; an empty file is
	// enough for cloudbase-init to skip it rather than error out.
	if err := os.WriteFile(filepath.Join(osDir, "user_data"), []byte{}, 0644); err != nil {
		return err
	}

	return buildISO(seedDir, destPath, "config-2")
}

// configDriveUUID derives a deterministic, UUID-shaped instance id from
// hostname. It only needs to be a stable, plausible-looking identifier —
// cloudbase-init doesn't validate it against anything — so a random UUID
// isn't necessary.
func configDriveUUID(hostname string) string {
	sum := md5.Sum([]byte(hostname))
	return fmt.Sprintf("%x-%x-%x-%x-%x", sum[0:4], sum[4:6], sum[6:8], sum[8:10], sum[10:16])
}

// isoBuilders are candidate ISO9660-building tools, tried in order; the
// first one found on PATH is used. All three accept the same genisoimage
// -style flags (xorriso via its "-as genisoimage" emulation mode).
var isoBuilders = []string{"genisoimage", "mkisofs", "xorriso"}

// buildISO packages sourceDir as an ISO9660+Joliet+RockRidge volume at
// destPath, labeled volLabel.
func buildISO(sourceDir, destPath, volLabel string) error {
	for _, bin := range isoBuilders {
		if _, err := exec.LookPath(bin); err != nil {
			continue
		}
		args := []string{"-o", destPath, "-V", volLabel, "-J", "-R", sourceDir}
		if bin == "xorriso" {
			args = append([]string{"-as", "genisoimage"}, args...)
		}
		out, err := exec.Command(bin, args...).CombinedOutput()
		if err != nil {
			return fmt.Errorf("%s failed: %w: %s", bin, err, strings.TrimSpace(string(out)))
		}
		return nil
	}
	return errors.New("no ISO-building tool found on PATH (tried genisoimage, mkisofs, xorriso) — install one to use ch.os=windows")
}

// DnsmasqDHCPManager is the real cloud.DHCPManager implementation: it runs
// one dnsmasq instance per bridge, serving DHCP only (--port=0 disables its
// built-in DNS server, since onctl has no use for it and it would otherwise
// compete with the host's own resolver on port 53).
//
// UNVERIFIED: like ConfigDriveSeedPreparer, this has not been run against a
// real dnsmasq/cloud-hypervisor/Windows-guest chain.
type DnsmasqDHCPManager struct {
	// StateDir is where per-bridge dnsmasq PID/lease/hosts files live
	// (Config.StateDir from CHConfig).
	StateDir string
}

// NewDHCPManager returns a cloud.DHCPManager backed by dnsmasq, storing its
// per-bridge state files under stateDir.
func NewDHCPManager(stateDir string) cloud.DHCPManager {
	return DnsmasqDHCPManager{StateDir: stateDir}
}

func (m DnsmasqDHCPManager) dhcpDir(bridge string) string {
	return filepath.Join(m.StateDir, "dhcp", bridge)
}

func (m DnsmasqDHCPManager) pidFile(bridge string) string {
	return filepath.Join(m.dhcpDir(bridge), "dnsmasq.pid")
}

func (m DnsmasqDHCPManager) hostsFile(bridge string) string {
	return filepath.Join(m.dhcpDir(bridge), "hosts")
}

func (m DnsmasqDHCPManager) leaseFile(bridge string) string {
	return filepath.Join(m.dhcpDir(bridge), "leases")
}

// cidrFile persists the cidr EnsureDHCP was last called with for bridge, so
// restartDnsmasq (see RemoveReservation) can restart dnsmasq with the same
// --dhcp-range without needing the cidr threaded through RemoveReservation's
// signature (which cloud.DHCPManager's interface — and every call site —
// only ever passes a MAC to).
func (m DnsmasqDHCPManager) cidrFile(bridge string) string {
	return filepath.Join(m.dhcpDir(bridge), "cidr")
}

// EnsureDHCP starts a dnsmasq instance bound to bridge if one isn't already
// running, serving the whole of cidr's host range minus the gateway
// address. Idempotent, like NetworkManager.EnsureBridge.
func (m DnsmasqDHCPManager) EnsureDHCP(bridge, cidr string) error {
	dir := m.dhcpDir(bridge)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}
	// Recorded unconditionally (even if dnsmasq is already running) so it's
	// always current for the next restartDnsmasq call, same spirit as
	// AddReservation/RemoveReservation always rewriting the hosts file
	// rather than only on first creation.
	if err := os.WriteFile(m.cidrFile(bridge), []byte(cidr), 0644); err != nil {
		return err
	}
	if m.isRunning(bridge) {
		return nil
	}
	hostsFile := m.hostsFile(bridge)
	if _, err := os.Stat(hostsFile); os.IsNotExist(err) {
		if err := os.WriteFile(hostsFile, []byte{}, 0644); err != nil {
			return err
		}
	}
	return m.startDnsmasq(bridge, cidr)
}

// startDnsmasq launches dnsmasq for bridge and returns once it's running
// (dnsmasq daemonizes itself, so CombinedOutput returning without error is
// sufficient — this does not itself check isRunning). Split out of
// EnsureDHCP so restartDnsmasq (see RemoveReservation) can reuse the exact
// same command construction after killing a stale instance.
func (m DnsmasqDHCPManager) startDnsmasq(bridge, cidr string) error {
	rangeStart, rangeEnd, err := dhcpRange(cidr)
	if err != nil {
		return err
	}
	cmd := exec.Command("dnsmasq",
		"--port=0", // DHCP only; no DNS service.
		"--interface="+bridge,
		"--bind-interfaces",
		"--dhcp-range="+rangeStart+","+rangeEnd+",12h",
		"--dhcp-hostsfile="+m.hostsFile(bridge),
		// Scoped under our own state dir rather than dnsmasq's system-wide
		// default (/var/lib/misc/dnsmasq.leases): that default is shared
		// with any other dnsmasq usage on the host (another bridge, a
		// manual test, libvirt, ...), and a stale lease there for an
		// address a --dhcp-hostsfile reservation now claims silently wins
		// — dnsmasq won't hand out an address it still has recorded as
		// leased to a different MAC, no error, it just allocates from the
		// free pool instead. A private lease file per bridge avoids ever
		// colliding with unrelated leases from a DIFFERENT bridge/tool —
		// see restartDnsmasq for the remaining case this doesn't cover
		// (a stale lease from a previously destroyed VM on this SAME
		// bridge).
		"--dhcp-leasefile="+m.leaseFile(bridge),
		"--pid-file="+m.pidFile(bridge),
		"--except-interface=lo",
	)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to start dnsmasq for %q: %w: %s", bridge, err, strings.TrimSpace(string(out)))
	}
	return nil
}

func (m DnsmasqDHCPManager) isRunning(bridge string) bool {
	data, err := os.ReadFile(m.pidFile(bridge))
	if err != nil {
		return false
	}
	pid, err := strconv.Atoi(strings.TrimSpace(string(data)))
	if err != nil || pid <= 0 {
		return false
	}
	process, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	return process.Signal(syscall.Signal(0)) == nil
}

// AddReservation appends a "mac,ip" line to the bridge's dnsmasq
// --dhcp-hostsfile and asks dnsmasq to reread it. It relies on the caller
// (ProviderCH.Deploy) having called EnsureDHCP first for the same bridge —
// there is no bridge parameter here because CHVMConfig/DHCPManager's
// Deploy-time call sites don't thread it through per-reservation; if that
// turns out to be needed in practice (e.g. multiple ch.network.bridge
// values in use), this will need a bridge parameter added.
//
// mac is lowercased before writing: dnsmasq's --dhcp-hostsfile matching is
// case-sensitive against the lowercase-normalized MAC a DHCP client
// actually sends, so any mixed-case address here would silently never
// match its own reservation (caught via chMAC, which used to emit
// "02:C4:..." — see its doc comment).
func (m DnsmasqDHCPManager) AddReservation(mac, ip string) error {
	mac = strings.ToLower(mac)
	return m.withEachBridgeHostsFile(func(bridge, hostsFile string) error {
		f, err := os.OpenFile(hostsFile, os.O_APPEND|os.O_WRONLY, 0644)
		if err != nil {
			return err
		}
		defer func() { _ = f.Close() }()
		if _, err := fmt.Fprintf(f, "%s,%s\n", mac, ip); err != nil {
			return err
		}
		return m.reload(bridge)
	})
}

// RemoveReservation removes a "mac,..." line previously added by
// AddReservation, plus that MAC's own lease-file entry (if any), and
// restarts dnsmasq so both changes actually take effect.
//
// The lease-file cleanup (not just the hosts-file one) matters because
// dnsmasq treats its lease file as its own persistent state, populated at
// startup and then owned/rewritten by the running process from then on —
// SIGHUP makes it reread the hosts file (see reload, still used by
// AddReservation) but does NOT make it forget or reread already-granted
// leases. Without this, a VM's MAC/IP stays "leased" in dnsmasq's memory
// after onctl destroy has removed the VM and its reservation, and the
// next VM that reuses that IP (chIPs allocates low addresses first, so
// this is the common case, not an edge case) has its real DHCP handshake
// silently redirected to a different address than the one onctl thinks it
// reserved — the new VM never gets its expected IP, and onctl create times
// out waiting for SSH on an address the guest never actually holds.
// Restarting dnsmasq (rather than a lighter-weight in-place fix — there is
// no SIGHUP-equivalent for "forget this one lease", short of adding a new
// dhcp_release/dnsmasq-utils host dependency) is what actually clears that
// in-memory state; see restartDnsmasq.
func (m DnsmasqDHCPManager) RemoveReservation(mac string) error {
	mac = strings.ToLower(mac)
	return m.withEachBridgeHostsFile(func(bridge, hostsFile string) error {
		data, err := os.ReadFile(hostsFile)
		if err != nil {
			return err
		}
		lines := strings.Split(string(data), "\n")
		kept := lines[:0]
		found := false
		for _, line := range lines {
			if strings.HasPrefix(line, mac+",") {
				found = true
				continue
			}
			if line != "" {
				kept = append(kept, line)
			}
		}
		leaseRemoved, err := m.removeLeaseFileEntry(bridge, mac)
		if err != nil {
			return err
		}
		if !found && !leaseRemoved {
			return nil
		}
		content := strings.Join(kept, "\n")
		if content != "" {
			content += "\n"
		}
		if err := os.WriteFile(hostsFile, []byte(content), 0644); err != nil {
			return err
		}
		return m.restartDnsmasq(bridge)
	})
}

// removeLeaseFileEntry strips any line for mac from bridge's dnsmasq lease
// file (format: "<expiry> <mac> <ip> <hostname> <client-id>", one per
// line — see dnsmasq(8)) and reports whether anything was removed. A
// missing lease file (dnsmasq never started, or never granted a lease on
// this bridge yet) is not an error — nothing to clean up.
func (m DnsmasqDHCPManager) removeLeaseFileEntry(bridge, mac string) (bool, error) {
	leaseFile := m.leaseFile(bridge)
	data, err := os.ReadFile(leaseFile)
	if err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, err
	}
	lines := strings.Split(string(data), "\n")
	kept := lines[:0]
	found := false
	for _, line := range lines {
		fields := strings.Fields(line)
		if len(fields) >= 2 && strings.ToLower(fields[1]) == mac {
			found = true
			continue
		}
		if line != "" {
			kept = append(kept, line)
		}
	}
	if !found {
		return false, nil
	}
	content := strings.Join(kept, "\n")
	if content != "" {
		content += "\n"
	}
	if err := os.WriteFile(leaseFile, []byte(content), 0644); err != nil {
		return false, err
	}
	return true, nil
}

// restartDnsmasq kills bridge's running dnsmasq (if any, via its recorded
// PID) and starts a fresh one from the cidr EnsureDHCP last recorded (see
// cidrFile) — the only way to make dnsmasq forget an in-memory lease
// short of the dhcp_release helper (see RemoveReservation's doc comment).
// A brief DHCP gap on the bridge during the restart is harmless in
// practice: every other VM on it already holds its IP and doesn't need
// the server again until its next lease renewal.
func (m DnsmasqDHCPManager) restartDnsmasq(bridge string) error {
	if data, err := os.ReadFile(m.pidFile(bridge)); err == nil {
		if pid, err := strconv.Atoi(strings.TrimSpace(string(data))); err == nil && pid > 0 {
			if process, err := os.FindProcess(pid); err == nil {
				_ = process.Signal(syscall.SIGTERM)
				for i := 0; i < 20 && m.isRunning(bridge); i++ {
					time.Sleep(50 * time.Millisecond)
				}
			}
		}
	}
	cidr, err := os.ReadFile(m.cidrFile(bridge))
	if err != nil {
		if os.IsNotExist(err) {
			// EnsureDHCP was never called for this bridge (or predates
			// cidrFile) -- nothing to restart into, and RemoveReservation
			// is a no-op on a bridge with no dnsmasq state anyway.
			return nil
		}
		return err
	}
	return m.startDnsmasq(bridge, strings.TrimSpace(string(cidr)))
}

// withEachBridgeHostsFile applies fn to every bridge's hosts file under
// m.StateDir/dhcp. AddReservation/RemoveReservation don't know which
// bridge a VM used (see the comment on AddReservation) — in the common
// case of a single ch.network.bridge this is exactly one file, and fn is a
// no-op for any other bridge's file that doesn't contain mac.
func (m DnsmasqDHCPManager) withEachBridgeHostsFile(fn func(bridge, hostsFile string) error) error {
	root := filepath.Join(m.StateDir, "dhcp")
	entries, err := os.ReadDir(root)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		if err := fn(e.Name(), m.hostsFile(e.Name())); err != nil {
			return err
		}
	}
	return nil
}

// reload asks the bridge's dnsmasq process to reread its hosts file via
// SIGHUP, dnsmasq's documented mechanism for picking up
// --dhcp-hostsfile changes without a restart.
func (m DnsmasqDHCPManager) reload(bridge string) error {
	data, err := os.ReadFile(m.pidFile(bridge))
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	pid, err := strconv.Atoi(strings.TrimSpace(string(data)))
	if err != nil || pid <= 0 {
		return nil
	}
	process, err := os.FindProcess(pid)
	if err != nil {
		return nil
	}
	if err := process.Signal(syscall.SIGHUP); err != nil && !errors.Is(err, os.ErrProcessDone) {
		return err
	}
	return nil
}

// dhcpRange returns a usable DHCP range covering cidr, excluding the
// gateway address (bridgeGatewayAndMask's first usable address, which the
// bridge itself owns).
func dhcpRange(cidr string) (start, end string, err error) {
	ip, ipnet, err := net.ParseCIDR(cidr)
	if err != nil {
		return "", "", fmt.Errorf("invalid CIDR %q: %w", cidr, err)
	}
	ip4 := ip.To4()
	if ip4 == nil {
		return "", "", fmt.Errorf("only IPv4 CIDRs are supported, got %q", cidr)
	}
	ones, bits := ipnet.Mask.Size()
	if bits != 32 || ones >= 31 {
		return "", "", fmt.Errorf("CIDR %q is too small for a DHCP range", cidr)
	}
	broadcast := make(net.IP, 4)
	for i := range ip4 {
		broadcast[i] = ip4[i] | ^ipnet.Mask[i]
	}
	startIP := make(net.IP, 4)
	copy(startIP, ip4)
	startIP[3]++ // first address after the gateway (ip4 itself)
	endIP := make(net.IP, 4)
	copy(endIP, broadcast)
	endIP[3]--
	return startIP.String(), endIP.String(), nil
}
