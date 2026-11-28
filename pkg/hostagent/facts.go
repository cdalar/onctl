package hostagent

import (
	"bufio"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
)

// Facts is a host's health and capacity self-report, the body of register
// and every heartbeat (boxctl's internal/vm/hosts.go HostFacts). Every
// number is what the host says right now, uninterpreted: what's committed,
// what still fits and whether the host is healthy are the server's call.
// A value that can't be read is 0 or empty, never an error.
type Facts struct {
	Version          string            `json:"version"`
	OnctlVersion     string            `json:"onctl_version"`
	MemTotalMiB      int64             `json:"mem_total_mib"`
	MemAvailableMiB  int64             `json:"mem_available_mib"`
	SwapTotalMiB     int64             `json:"swap_total_mib"`
	SwapFreeMiB      int64             `json:"swap_free_mib"`
	CPUThreads       int               `json:"cpu_threads"`
	Load1            float64           `json:"load1"`
	UptimeSeconds    int64             `json:"uptime_seconds"`
	StateDir         string            `json:"state_dir"`
	StateDirTotalMiB int64             `json:"state_dir_total_mib"`
	StateDirFreeMiB  int64             `json:"state_dir_free_mib"`
	KVM              bool              `json:"kvm"`
	Providers        []string          `json:"providers"` // nil: not known yet (no VM report so far)
	Labels           map[string]string `json:"labels"`
	Images           map[string]string `json:"images"`            // image path -> sha256 ("" when unknown)
	ImageGenerations map[string]string `json:"image_generations"` // sha256 -> kept previous version's path
	Capabilities     []string          `json:"capabilities"`
}

// imageExts are the files an image dir holds: rootfs images and the box
// kernel (boxctl-kernel.vmlinux), which the server fetches like a rootfs.
var imageExts = []string{".ext4", ".raw", ".vmlinux"}

// facts collects this host's Facts; providers is which providers
// answered the last VM report (nil before the first).
func (a *Agent) facts(providers []string) Facts {
	f := Facts{
		Version:      a.cfg.Version,
		OnctlVersion: a.cfg.OnctlVersion,
		CPUThreads:   runtime.NumCPU(),
		StateDir:     a.cfg.StateDir,
		Providers:    providers,
		Labels:       a.cfg.Labels,
		Images:       map[string]string{},
		Capabilities: a.capabilities(),
	}
	mem := meminfo()
	f.MemTotalMiB, f.MemAvailableMiB = mem["MemTotal"]/1024, mem["MemAvailable"]/1024
	f.SwapTotalMiB, f.SwapFreeMiB = mem["SwapTotal"]/1024, mem["SwapFree"]/1024
	if b, err := os.ReadFile("/proc/loadavg"); err == nil {
		if fields := strings.Fields(string(b)); len(fields) > 0 {
			f.Load1, _ = strconv.ParseFloat(fields[0], 64)
		}
	}
	if b, err := os.ReadFile("/proc/uptime"); err == nil {
		if fields := strings.Fields(string(b)); len(fields) > 0 {
			up, _ := strconv.ParseFloat(fields[0], 64)
			f.UptimeSeconds = int64(up)
		}
	}
	f.StateDirTotalMiB, f.StateDirFreeMiB = diskMiB(a.cfg.StateDir)
	f.KVM = kvmUsable()
	f.Images, f.ImageGenerations = images(a.cfg.ImageDirs)
	if f.Labels == nil {
		f.Labels = map[string]string{}
	}
	return f
}

// capabilities is what this host can do that an older agent couldn't, so
// the server can tell before asking:
//
//	image-generations  previous image versions are kept (fetch-image.sh)
//	typed-jobs         Job.Op jobs run through pkg/box (OpCreate, ...)
//	ingress            frpc and the ingress sync script are installed
//	disk-grow          the disk-grow script is installed
func (a *Agent) capabilities() []string {
	caps := []string{"image-generations", "typed-jobs"}
	if executable(filepath.Join(a.cfg.BinDir, "boxctl-frpc")) && executable(filepath.Join(a.cfg.BinDir, "boxctl-ingress-sync.sh")) {
		caps = append(caps, "ingress")
	}
	if executable(filepath.Join(a.cfg.BinDir, "boxctl-grow-disk.sh")) {
		caps = append(caps, "disk-grow")
	}
	return caps
}

func executable(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir() && info.Mode()&0o111 != 0
}

// meminfo is /proc/meminfo's kB values by name; empty off Linux.
func meminfo() map[string]int64 {
	out := map[string]int64{}
	f, err := os.Open("/proc/meminfo")
	if err != nil {
		return out
	}
	defer func() { _ = f.Close() }()
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		name, rest, ok := strings.Cut(sc.Text(), ":")
		if !ok {
			continue
		}
		if fields := strings.Fields(rest); len(fields) > 0 {
			out[name], _ = strconv.ParseInt(fields[0], 10, 64)
		}
	}
	return out
}

// images maps each image in dirs to its sha256, read from the
// <image>.sha256sum sidecar beside it ("<sha> <size> ..."); it never
// hashes anything, as these files are many GiB and this runs on every
// heartbeat. A sidecar whose recorded size isn't the file's is stale: the
// image is reported with "" (present, checksum unknown). generations maps
// the sha256 of each kept previous version (<dir>/.generations/) to its
// path, those with a valid sidecar only.
func images(dirs []string) (current, generations map[string]string) {
	current, generations = map[string]string{}, map[string]string{}
	for _, dir := range dirs {
		for _, img := range imageFiles(dir) {
			current[img] = sidecarSHA(img)
		}
		for _, img := range imageFiles(filepath.Join(dir, ".generations")) {
			if sha := sidecarSHA(img); sha != "" {
				generations[sha] = img
			}
		}
	}
	return current, generations
}

func imageFiles(dir string) []string {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	var out []string
	for _, e := range entries {
		if e.Type().IsRegular() {
			for _, ext := range imageExts {
				if strings.HasSuffix(e.Name(), ext) {
					out = append(out, filepath.Join(dir, e.Name()))
				}
			}
		}
	}
	sort.Strings(out)
	return out
}

func sidecarSHA(img string) string {
	b, err := os.ReadFile(img + ".sha256sum")
	if err != nil {
		return ""
	}
	fields := strings.Fields(string(b))
	if len(fields) < 2 {
		return ""
	}
	info, err := os.Stat(img)
	if err != nil || strconv.FormatInt(info.Size(), 10) != fields[1] {
		return ""
	}
	return fields[0]
}
