package providerch

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Config values come from YAML, so "~" is never shell-expanded; expandHome
// must do it, but only for the current user's "~" / "~/" forms.
func TestExpandHome(t *testing.T) {
	home, err := os.UserHomeDir()
	require.NoError(t, err)

	assert.Equal(t, home, expandHome("~"))
	assert.Equal(t, filepath.Join(home, "images/vmlinux"), expandHome("~/images/vmlinux"))
	assert.Equal(t, "/abs/path", expandHome("/abs/path"))
	assert.Equal(t, "relative/path", expandHome("relative/path"))
	assert.Equal(t, "~other/foo", expandHome("~other/foo"), "~user form must not be treated as the current user's home")
}

// An empty config must still yield a bootable Linux guest on the chbr0
// bridge — distinct from Firecracker's fcbr0/172.16 so both can coexist.
func TestGetConfig_Defaults(t *testing.T) {
	viper.Reset()
	t.Cleanup(viper.Reset)

	cfg := GetConfig()
	assert.Equal(t, defaultCHStateDir, cfg.StateDir)
	assert.Equal(t, int64(1), cfg.VCPUCount)
	assert.Equal(t, int64(2048), cfg.MemSizeMib)
	assert.Equal(t, "chbr0", cfg.Bridge)
	assert.Equal(t, "172.17.0.1/24", cfg.CIDR)
	assert.Equal(t, "root", cfg.Username)
	assert.Equal(t, "cloud-hypervisor", cfg.BinPath)
	assert.Equal(t, "linux", cfg.OS)
	assert.Empty(t, cfg.KernelImage)
	assert.Empty(t, cfg.RootfsImage)
}

func TestGetConfig_ExplicitValues(t *testing.T) {
	viper.Reset()
	t.Cleanup(viper.Reset)
	home, err := os.UserHomeDir()
	require.NoError(t, err)

	viper.Set("ch.stateDir", "~/ch-state")
	viper.Set("ch.kernelImage", "~/k/vmlinux")
	viper.Set("ch.rootfsImage", "/img/rootfs.ext4")
	viper.Set("ch.kernelArgs", "console=ttyS0")
	viper.Set("ch.vcpuCount", 4)
	viper.Set("ch.memSizeMib", 4096)
	viper.Set("ch.vm.username", "ubuntu")
	viper.Set("ch.binPath", "/usr/local/bin/cloud-hypervisor")
	viper.Set("ch.os", "  Windows ")

	cfg := GetConfig()
	assert.Equal(t, filepath.Join(home, "ch-state"), cfg.StateDir)
	assert.Equal(t, filepath.Join(home, "k/vmlinux"), cfg.KernelImage)
	assert.Equal(t, "/img/rootfs.ext4", cfg.RootfsImage)
	assert.Equal(t, "console=ttyS0", cfg.KernelArgs)
	assert.Equal(t, int64(4), cfg.VCPUCount)
	assert.Equal(t, int64(4096), cfg.MemSizeMib)
	assert.Equal(t, "ubuntu", cfg.Username)
	assert.Equal(t, "/usr/local/bin/cloud-hypervisor", cfg.BinPath)
	// The guest boot path is switched on an exact "windows" match, so
	// user-typed casing/whitespace must be normalized.
	assert.Equal(t, "windows", cfg.OS)
}

// "bridge/cidr" shorthand: the CIDR half wins over ch.network.cidr; an
// empty CIDR half falls back to the configured or default CIDR.
func TestGetConfig_BridgeCidrShorthand(t *testing.T) {
	tests := []struct {
		name, bridge, cidr, wantBridge, wantCIDR string
	}{
		{"bridge and cidr", "mybr0/10.0.0.1/24", "", "mybr0", "10.0.0.1/24"},
		{"shorthand overrides cidr", "mybr0/10.0.0.1/24", "192.168.1.1/24", "mybr0", "10.0.0.1/24"},
		{"empty cidr half uses default", "mybr0/", "", "mybr0", "172.17.0.1/24"},
		{"empty cidr half keeps configured", "mybr0/", "192.168.1.1/24", "mybr0", "192.168.1.1/24"},
		{"plain bridge", "mybr0", "", "mybr0", "172.17.0.1/24"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			viper.Reset()
			t.Cleanup(viper.Reset)
			viper.Set("ch.network.bridge", tt.bridge)
			if tt.cidr != "" {
				viper.Set("ch.network.cidr", tt.cidr)
			}
			cfg := GetConfig()
			assert.Equal(t, tt.wantBridge, cfg.Bridge)
			assert.Equal(t, tt.wantCIDR, cfg.CIDR)
		})
	}
}
