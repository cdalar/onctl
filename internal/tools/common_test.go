package tools

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestContains_Found(t *testing.T) {
	slice := []string{"aws", "azure", "gcp", "hetzner"}
	assert.True(t, Contains(slice, "aws"))
	assert.True(t, Contains(slice, "gcp"))
	assert.True(t, Contains(slice, "hetzner"))
}

func TestContains_NotFound(t *testing.T) {
	slice := []string{"aws", "azure", "gcp"}
	assert.False(t, Contains(slice, "hetzner"))
	assert.False(t, Contains(slice, ""))
}

func TestContains_EmptySlice(t *testing.T) {
	assert.False(t, Contains([]string{}, "aws"))
}

func TestWhichCloudProvider_AWS_AccessKey(t *testing.T) {
	os.Unsetenv("AWS_ACCESS_KEY_ID")
	os.Unsetenv("AWS_PROFILE")
	os.Unsetenv("AZURE_CLIENT_ID")
	os.Unsetenv("GOOGLE_CREDENTIALS")
	os.Unsetenv("HCLOUD_TOKEN")

	t.Setenv("AWS_ACCESS_KEY_ID", "test-key")
	assert.Equal(t, "aws", WhichCloudProvider())
}

func TestWhichCloudProvider_AWS_Profile(t *testing.T) {
	os.Unsetenv("AWS_ACCESS_KEY_ID")
	os.Unsetenv("AZURE_CLIENT_ID")
	os.Unsetenv("GOOGLE_CREDENTIALS")
	os.Unsetenv("HCLOUD_TOKEN")

	t.Setenv("AWS_PROFILE", "default")
	assert.Equal(t, "aws", WhichCloudProvider())
}

func TestWhichCloudProvider_Azure(t *testing.T) {
	os.Unsetenv("AWS_ACCESS_KEY_ID")
	os.Unsetenv("AWS_PROFILE")
	os.Unsetenv("GOOGLE_CREDENTIALS")
	os.Unsetenv("HCLOUD_TOKEN")

	t.Setenv("AZURE_CLIENT_ID", "test-client")
	assert.Equal(t, "azure", WhichCloudProvider())
}

func TestWhichCloudProvider_GCP(t *testing.T) {
	os.Unsetenv("AWS_ACCESS_KEY_ID")
	os.Unsetenv("AWS_PROFILE")
	os.Unsetenv("AZURE_CLIENT_ID")
	os.Unsetenv("HCLOUD_TOKEN")

	t.Setenv("GOOGLE_CREDENTIALS", "test-creds")
	assert.Equal(t, "gcp", WhichCloudProvider())
}

func TestWhichCloudProvider_Hetzner(t *testing.T) {
	os.Unsetenv("AWS_ACCESS_KEY_ID")
	os.Unsetenv("AWS_PROFILE")
	os.Unsetenv("AZURE_CLIENT_ID")
	os.Unsetenv("GOOGLE_CREDENTIALS")

	t.Setenv("HCLOUD_TOKEN", "test-token")
	assert.Equal(t, "hetzner", WhichCloudProvider())
}

func TestWhichCloudProvider_None(t *testing.T) {
	os.Unsetenv("AWS_ACCESS_KEY_ID")
	os.Unsetenv("AWS_PROFILE")
	os.Unsetenv("AZURE_CLIENT_ID")
	os.Unsetenv("GOOGLE_CREDENTIALS")
	os.Unsetenv("HCLOUD_TOKEN")

	assert.Equal(t, "none", WhichCloudProvider())
}
