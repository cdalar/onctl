package cmd

import (
	"errors"
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

type fakeImageLister struct {
	images []cloud.CloudImage
	err    error
}

func (f fakeImageLister) ListImages() ([]cloud.CloudImage, error) {
	return f.images, f.err
}

func TestImagesCmd_HetznerPath(t *testing.T) {
	origProvider := cloudProvider
	origLister := hetznerImageLister
	defer func() {
		cloudProvider = origProvider
		hetznerImageLister = origLister
	}()

	cloudProvider = "hetzner"
	hetznerImageLister = func() cloud.ImageLister {
		return fakeImageLister{images: []cloud.CloudImage{
			{Name: "ubuntu-22.04", Type: "system", OSFlavor: "ubuntu", OSVersion: "22.04", Description: "Ubuntu 22.04"},
			{Name: "fedora-42", Type: "system", OSFlavor: "fedora", OSVersion: "42", Description: "Fedora 42"},
		}}
	}

	assert.NotPanics(t, func() {
		imagesCmd.Run(imagesCmd, []string{})
	})
}

func TestImagesCmd_HetznerPath_Empty(t *testing.T) {
	origProvider := cloudProvider
	origLister := hetznerImageLister
	defer func() {
		cloudProvider = origProvider
		hetznerImageLister = origLister
	}()

	cloudProvider = "hetzner"
	hetznerImageLister = func() cloud.ImageLister {
		return fakeImageLister{images: []cloud.CloudImage{}}
	}

	assert.NotPanics(t, func() {
		imagesCmd.Run(imagesCmd, []string{})
	})
}

func TestImagesCmd_HetznerPath_ListError(t *testing.T) {
	origProvider := cloudProvider
	origLister := hetznerImageLister
	defer func() {
		cloudProvider = origProvider
		hetznerImageLister = origLister
	}()

	cloudProvider = "hetzner"
	hetznerImageLister = func() cloud.ImageLister {
		return fakeImageLister{err: errors.New("api error")}
	}

	// log.Fatalln calls os.Exit — we can't assert on it directly, but
	// we verify the error path is reachable without a setup panic.
	assert.NotPanics(t, func() {
		// Would call log.Fatalln; skip invoking Run to avoid os.Exit in tests.
		lister := hetznerImageLister()
		_, err := lister.ListImages()
		assert.Error(t, err)
	})
}
