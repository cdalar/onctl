package cmd

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/cdalar/onctl/internal/files"
	"github.com/cdalar/onctl/pkg/cloud"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// withProvider resets the package-level provider global around a test.
func withProvider(t *testing.T, p cloud.CloudProviderInterface) {
	t.Helper()
	orig := provider
	provider = p
	t.Cleanup(func() { provider = orig })
}

// withViper sets a viper key for one test and restores the previous value.
func withViper(t *testing.T, key string, value any) {
	t.Helper()
	orig := viper.Get(key)
	viper.Set(key, value)
	t.Cleanup(func() { viper.Set(key, orig) })
}

// ovh has no CLI to auto-resolve the service name from, so both an unset
// value and the untouched init-template placeholder must be rejected.
func TestResolveOvhServiceName(t *testing.T) {
	for _, bad := range []string{"", "<service-name>"} {
		withViper(t, "ovh.serviceName", bad)
		err := resolveOvhServiceName()
		require.Error(t, err, "value %q must be rejected", bad)
		assert.Contains(t, err.Error(), "--service-name")
	}

	withViper(t, "ovh.serviceName", "abc123")
	assert.NoError(t, resolveOvhServiceName())
}

// Shell completion calls ensureProvider on every keystroke; an already-built
// provider must be reused, never rebuilt (that would re-read config/creds).
func TestEnsureProvider_KeepsExisting(t *testing.T) {
	existing := &cloud.ProviderStatic{InventoryPath: "/nonexistent"}
	withProvider(t, existing)

	ensureProvider()
	assert.Same(t, existing, provider)
}

// The local providers (fc, ch, static) need no credentials, so their
// construction can be checked without network access.
func TestInitProvider_LocalProviders(t *testing.T) {
	withViper(t, "fc.binPath", "/opt/bin/firecracker")
	withViper(t, "ch.binPath", "/opt/bin/cloud-hypervisor")

	t.Run("fc", func(t *testing.T) {
		withProvider(t, nil)
		initProvider("fc")
		fc, ok := provider.(*cloud.ProviderFC)
		require.True(t, ok, "got %T", provider)
		assert.Equal(t, "/opt/bin/firecracker", fc.Config.BinPath, "fc config must come from viper")
		assert.NotNil(t, fc.Process)
		assert.NotNil(t, fc.API)
		assert.NotNil(t, fc.Net)
		assert.NotNil(t, fc.Rootfs)
		assert.NotNil(t, fc.Cache)
	})

	t.Run("ch", func(t *testing.T) {
		withProvider(t, nil)
		initProvider("ch")
		ch, ok := provider.(*cloud.ProviderCH)
		require.True(t, ok, "got %T", provider)
		assert.Equal(t, "/opt/bin/cloud-hypervisor", ch.Config.BinPath, "ch config must come from viper")
		assert.NotNil(t, ch.Process)
		assert.NotNil(t, ch.Net)
		assert.NotNil(t, ch.Rootfs)
		assert.NotNil(t, ch.WindowsGuest)
		assert.NotNil(t, ch.DHCP)
	})

	t.Run("static", func(t *testing.T) {
		withProvider(t, nil)
		initProvider("static")
		st, ok := provider.(*cloud.ProviderStatic)
		require.True(t, ok, "got %T", provider)
		want, err := onctlSSHConfigPath()
		require.NoError(t, err)
		assert.Equal(t, want, st.InventoryPath)
	})

	t.Run("unknown leaves provider unset", func(t *testing.T) {
		withProvider(t, nil)
		initProvider("nope")
		assert.Nil(t, provider)
	})
}

// Built-in templates (e.g. docker.sh) must work without network access: when
// the name isn't on disk, findSingleFile extracts the embedded copy.
func TestFindSingleFile_EmbeddedFallback(t *testing.T) {
	t.Chdir(t.TempDir()) // make sure no docker.sh exists on disk

	path := findSingleFile("docker.sh")
	require.NotEmpty(t, path)
	t.Cleanup(func() { _ = os.RemoveAll(filepath.Dir(path)) })

	assert.NotEqual(t, "docker.sh", path, "must be an extracted temp copy, not the bare name")
	got, err := os.ReadFile(path)
	require.NoError(t, err)
	want, err := files.EmbededFiles.ReadFile("docker.sh")
	require.NoError(t, err)
	assert.Equal(t, want, got)
}
