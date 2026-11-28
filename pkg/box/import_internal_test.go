package box

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/cdalar/onctl/pkg/cloud"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Import makes plain `ssh NAME` work by putting one Include of
// onctl_config at the top of ~/.ssh/config: added once however often
// hosts are imported, kept ahead of the user's own entries, and not
// fooled by a commented-out Include of the same file.
func TestImport_IncludesOnctlConfigOnce(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	sshDir := filepath.Join(home, ".ssh")
	require.NoError(t, os.MkdirAll(sshDir, 0o700))
	onctlConfig := filepath.Join(sshDir, "onctl_config")
	userConfig := "# Include " + onctlConfig + "\nHost work\n  User me\n"
	require.NoError(t, os.WriteFile(filepath.Join(sshDir, "config"), []byte(userConfig), 0o600))

	updated, err := Import(cloud.StaticHost{Name: "nas", IP: "10.0.0.5", Username: "root", SSHPort: 22})
	require.NoError(t, err)
	assert.False(t, updated)
	updated, err = Import(cloud.StaticHost{Name: "nas", IP: "10.0.0.6", Username: "root", SSHPort: 22})
	require.NoError(t, err)
	assert.True(t, updated, "the same name replaces the host")

	data, err := os.ReadFile(filepath.Join(sshDir, "config"))
	require.NoError(t, err)
	got := string(data)
	include := "Include " + cloud.QuoteSSHConfigValue(onctlConfig)
	assert.True(t, strings.HasPrefix(got, include+"\n"), "the Include goes first: %q", got)
	lines := 0
	for _, l := range strings.Split(got, "\n") {
		if l == include {
			lines++
		}
	}
	assert.Equal(t, 1, lines, "added once")
	assert.Contains(t, got, userConfig, "the user's own config is kept")

	inv, err := cloud.ProviderStatic{InventoryPath: onctlConfig}.LoadInventory()
	require.NoError(t, err)
	require.Len(t, inv.Hosts, 1)
	assert.Equal(t, "10.0.0.6", inv.Hosts[0].IP)
}
