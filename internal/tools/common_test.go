package tools

import (
	"os"
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
