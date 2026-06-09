package cmd

import (
	"testing"

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

func TestImagesCmd_UnsupportedProvider(t *testing.T) {
	original := cloudProvider
	defer func() { cloudProvider = original }()

	for _, unsupported := range []string{"gcp", "aws", "azure"} {
		cloudProvider = unsupported
		assert.NotPanics(t, func() {
			imagesCmd.Run(imagesCmd, []string{})
		}, "images command should not panic for provider %q", unsupported)
	}
}
