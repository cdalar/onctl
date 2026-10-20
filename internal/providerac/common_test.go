package providerac

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetConfig_Defaults(t *testing.T) {
	viper.Reset()
	t.Cleanup(viper.Reset)

	cfg := GetConfig()
	assert.Equal(t, defaultImage, cfg.Image)
	assert.Equal(t, int64(1), cfg.VCPUCount)
	assert.Equal(t, int64(2048), cfg.MemSizeMib)
	assert.Equal(t, "root", cfg.Username)
	assert.Equal(t, "container", cfg.BinPath)
}

func TestGetConfig_FromViper(t *testing.T) {
	viper.Reset()
	t.Cleanup(viper.Reset)

	viper.Set("ac.image", "docker.io/library/alpine:3.20")
	viper.Set("ac.vcpuCount", 4)
	viper.Set("ac.memSizeMib", 4096)
	viper.Set("ac.vm.username", "dev")
	viper.Set("ac.binPath", "/opt/bin/container")
	cfg := GetConfig()
	assert.Equal(t, "docker.io/library/alpine:3.20", cfg.Image)
	assert.Equal(t, int64(4), cfg.VCPUCount)
	assert.Equal(t, int64(4096), cfg.MemSizeMib)
	assert.Equal(t, "dev", cfg.Username)
	assert.Equal(t, "/opt/bin/container", cfg.BinPath)
}

// writeFakeCLI writes a shell script standing in for the container CLI.
func writeFakeCLI(t *testing.T, body string) string {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("shell script stand-in")
	}
	path := filepath.Join(t.TempDir(), "container")
	require.NoError(t, os.WriteFile(path, []byte("#!/bin/sh\n"+body), 0755))
	return path
}

// Run returns stdout on success and the CLI's own stderr message on failure,
// which is what the user needs to see (`container` explains itself there).
func TestRun(t *testing.T) {
	bin := writeFakeCLI(t, `if [ "$1" = ls ]; then echo '[]'; exit 0; fi
echo "Error: no such container: $2" >&2; exit 1`)
	r := NewRunner(bin)

	out, err := r.Run("ls", "--all", "--format", "json")
	require.NoError(t, err)
	assert.Equal(t, "[]\n", string(out))

	_, err = r.Run("stop", "nope")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "container stop: Error: no such container: nope")
}

// The service not running is the first thing every new user hits; say so.
func TestRun_XPCHint(t *testing.T) {
	bin := writeFakeCLI(t, `echo 'Error: interrupted: "XPC connection error: Connection invalid"' >&2; exit 1`)
	_, err := NewRunner(bin).Run("ls")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "container system start")
}

func TestRun_MissingBinary(t *testing.T) {
	_, err := NewRunner(filepath.Join(t.TempDir(), "definitely-not-here")).Run("ls")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "github.com/apple/container")
}
