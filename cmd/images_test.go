package cmd

import (
	"io"
	"os"
	"testing"

	"github.com/cdalar/onctl/pkg/cloud"
	"github.com/stretchr/testify/assert"
)

func TestImagesCmd_CommandProperties(t *testing.T) {
	assert.Equal(t, "images", imagesCmd.Use)
	assert.Equal(t, "List available OS images for the current cloud provider", imagesCmd.Short)
	assert.NotNil(t, imagesCmd.Run)
}

func TestImagesCmd_IsRegistered(t *testing.T) {
	found := false
	for _, cmd := range rootCmd.Commands() {
		if cmd.Name() == "images" {
			found = true
			break
		}
	}
	assert.True(t, found, "images command should be registered with root")
}

// imagesOnly hides every optional capability of the provider it wraps:
// only cloud.CloudProviderInterface's methods are promoted.
type imagesOnly struct{ cloud.CloudProviderInterface }

func useProvider(t *testing.T, id string, p cloud.CloudProviderInterface) {
	t.Helper()
	prevID, prev := cloudProvider, provider
	cloudProvider, provider = id, p
	t.Cleanup(func() { cloudProvider, provider = prevID, prev })
}

// A provider that can't list images gets a message, not an error exit.
func TestImagesCmd_UnsupportedProvider(t *testing.T) {
	useProvider(t, "aws", imagesOnly{&tuiFakeProvider{}})
	out := captureStdout(t, func() { imagesCmd.Run(imagesCmd, nil) })
	assert.Contains(t, out, "does not support listing images")
}

// A provider that lists images gets them as the table.
func TestImagesCmd_ListsTheProvidersImages(t *testing.T) {
	useProvider(t, "boxes", &tuiFakeProvider{})
	out := captureStdout(t, func() { imagesCmd.Run(imagesCmd, nil) })
	assert.Contains(t, out, "debian-12")
	assert.Contains(t, out, "ubuntu-24.04")
}

func captureStdout(t *testing.T, f func()) string {
	t.Helper()
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	prev := os.Stdout
	os.Stdout = w
	f()
	os.Stdout = prev
	_ = w.Close()
	out, _ := io.ReadAll(r)
	return string(out)
}
