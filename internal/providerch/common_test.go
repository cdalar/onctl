package providerch

// Tests for providerch package.

import (
	"encoding/json"
	"net"
	"net/http"
	"os"
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
