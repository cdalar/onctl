package tools

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestContains(t *testing.T) {
	tests := []struct {
		name        string
		slice       []string
		searchValue string
		expected    bool
	}{
		{"found", []string{"a", "b", "c"}, "b", true},
		{"not found", []string{"a", "b", "c"}, "d", false},
		{"empty slice", []string{}, "a", false},
		{"empty search", []string{"a", ""}, "", true},
		{"single match", []string{"hello"}, "hello", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := Contains(tt.slice, tt.searchValue)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestWhichCloudProvider(t *testing.T) {
	// Isolate from any real onctl home or cwd yamls so file-based detection is off.
	// We set HOME to a clean dir and chdir to an empty cwd.
	cleanHome := t.TempDir()
	t.Setenv("HOME", cleanHome)
	origDir, _ := os.Getwd()
	tmp := t.TempDir()
	os.Chdir(tmp)
	defer os.Chdir(origDir)

	// Clear all cloud provider env vars first
	envVars := []string{
		"AWS_ACCESS_KEY_ID", "AWS_PROFILE",
		"AZURE_CLIENT_ID", "GOOGLE_CREDENTIALS", "HCLOUD_TOKEN",
	}
	for _, v := range envVars {
		require.NoError(t, os.Unsetenv(v))
	}

	t.Run("none", func(t *testing.T) {
		result := WhichCloudProvider()
		assert.Equal(t, "none", result)
	})

	t.Run("aws via access key", func(t *testing.T) {
		t.Setenv("AWS_ACCESS_KEY_ID", "AKIAIOSFODNN7EXAMPLE")
		result := WhichCloudProvider()
		assert.Equal(t, "aws", result)
	})

	t.Run("aws via profile", func(t *testing.T) {
		require.NoError(t, os.Unsetenv("AWS_ACCESS_KEY_ID"))
		t.Setenv("AWS_PROFILE", "default")
		result := WhichCloudProvider()
		assert.Equal(t, "aws", result)
	})

	t.Run("azure", func(t *testing.T) {
		require.NoError(t, os.Unsetenv("AWS_PROFILE"))
		t.Setenv("AZURE_CLIENT_ID", "some-client-id")
		result := WhichCloudProvider()
		assert.Equal(t, "azure", result)
	})

	t.Run("gcp", func(t *testing.T) {
		require.NoError(t, os.Unsetenv("AZURE_CLIENT_ID"))
		t.Setenv("GOOGLE_CREDENTIALS", `{"type":"service_account"}`)
		result := WhichCloudProvider()
		assert.Equal(t, "gcp", result)
	})

	t.Run("hetzner", func(t *testing.T) {
		require.NoError(t, os.Unsetenv("GOOGLE_CREDENTIALS"))
		t.Setenv("HCLOUD_TOKEN", "some-hcloud-token")
		result := WhichCloudProvider()
		assert.Equal(t, "hetzner", result)
	})
}

func TestDetectCloudProviders(t *testing.T) {
	// Isolate from real onctl config dirs. Point $HOME at temp + clean cwd.
	cleanHome := t.TempDir()
	t.Setenv("HOME", cleanHome)
	origDir, _ := os.Getwd()
	tmp := t.TempDir()
	os.Chdir(tmp)
	defer os.Chdir(origDir)

	envVars := []string{
		"AWS_ACCESS_KEY_ID", "AWS_PROFILE",
		"AZURE_CLIENT_ID", "GOOGLE_CREDENTIALS", "HCLOUD_TOKEN",
	}
	for _, v := range envVars {
		require.NoError(t, os.Unsetenv(v))
	}

	t.Run("none set", func(t *testing.T) {
		assert.Empty(t, DetectCloudProviders())
	})

	t.Run("one set", func(t *testing.T) {
		t.Setenv("HCLOUD_TOKEN", "some-hcloud-token")
		assert.Equal(t, []string{"hetzner"}, DetectCloudProviders())
	})

	t.Run("multiple set, returned in priority order", func(t *testing.T) {
		t.Setenv("HCLOUD_TOKEN", "some-hcloud-token")
		t.Setenv("AWS_ACCESS_KEY_ID", "AKIAIOSFODNN7EXAMPLE")
		assert.Equal(t, []string{"aws", "hetzner"}, DetectCloudProviders())
	})

	t.Run("detected from onctl yaml even without cred env", func(t *testing.T) {
		// Use the isolated HOME's .onctl (we set HOME to cleanHome above).
		onctlDir := filepath.Join(cleanHome, ".onctl")
		require.NoError(t, os.MkdirAll(onctlDir, 0o755))
		require.NoError(t, os.WriteFile(filepath.Join(onctlDir, "aws.yaml"), []byte("aws:\n  vm:\n    username: ubuntu\n"), 0o644))
		require.NoError(t, os.WriteFile(filepath.Join(onctlDir, "hetzner.yaml"), []byte("hetzner:\n  vm:\n    username: root\n"), 0o644))
		// ensure no cred envs interfere
		for _, v := range []string{"AWS_ACCESS_KEY_ID", "AWS_PROFILE", "HCLOUD_TOKEN", "AZURE_CLIENT_ID", "GOOGLE_CREDENTIALS"} {
			require.NoError(t, os.Unsetenv(v))
		}
		got := DetectCloudProviders()
		// append order is aws then hetzner for the hardcoded yaml scan list
		assert.Equal(t, []string{"aws", "hetzner"}, got)
	})
}
