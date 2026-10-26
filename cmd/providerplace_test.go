package cmd

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestProviderPlaces: every provider onctl offers has a place, so a
// create never falls back to the bare short name -- the point of the
// table is that "on ac" read like a cloud region when it is a container
// on the user's own Mac.
func TestProviderPlaces(t *testing.T) {
	for _, name := range cloudProviderList {
		p, ok := providerPlaces[name]
		assert.True(t, ok, "provider %s has no place", name)
		assert.NotEmpty(t, p.name, "provider %s has no long name", name)
	}
}

// TestCreatingMessage: a local provider's message says it is this
// machine and what kind of machine; a cloud's says it is a cloud, by its
// full name; the rest name the place; an unknown name is still usable.
func TestCreatingMessage(t *testing.T) {
	saved := cloudProvider
	t.Cleanup(func() { cloudProvider = saved })
	for _, tc := range []struct{ provider, want string }{
		{"ac", "Creating demo as an Apple container on this machine..."},
		{"fc", "Creating demo as a Firecracker microVM on this machine..."},
		{"hetzner", "Creating demo in the cloud on Hetzner Cloud..."},
		{"aws", "Creating demo in the cloud on Amazon Web Services..."},
		{"boxes", "Creating demo on the boxes service..."},
		{"nope", "Creating demo on nope..."},
	} {
		cloudProvider = tc.provider
		assert.Equal(t, tc.want, creatingMessage("demo"), tc.provider)
	}
}
