package cmd

import (
	"testing"

	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
)

// TestGenericCreateFlagsExist verifies the YAML-replacing flags are registered.
func TestGenericCreateFlagsExist(t *testing.T) {
	for _, name := range []string{"type", "location", "username", "cloud-init-timeout", "image"} {
		assert.NotNil(t, createCmd.Flags().Lookup(name), "create should have --%s flag", name)
	}
	assert.NotNil(t, rootCmd.PersistentFlags().Lookup("provider"), "root should have persistent --provider flag")
	assert.NotNil(t, actionCmd.Flags().Lookup("github-owner"), "action should have --github-owner flag")
}

// TestCreateFlagsBindToViper verifies that setting a create flag is reflected
// through its viper binding (the path the rest of the code reads), and that the
// default value matches the old hetzner.yaml. Deterministic: it sets values
// explicitly and restores them.
func TestCreateFlagsBindToViper(t *testing.T) {
	cases := []struct{ flag, key, def, override string }{
		{"type", "hetzner.vm.type", "cpx21", "cpx31"},
		{"location", "hetzner.location", "fsn1", "nbg1"},
		{"username", "hetzner.vm.username", "root", "admin"},
		{"cloud-init-timeout", "vm.cloud-init.timeout", "3m", "180s"},
	}
	for _, c := range cases {
		// Default (flag unchanged) resolves via the binding.
		assert.Equal(t, c.def, viper.GetString(c.key), "default for %s", c.key)
		// Override flows through.
		assert.NoError(t, createCmd.Flags().Set(c.flag, c.override))
		assert.Equal(t, c.override, viper.GetString(c.key), "override for %s", c.key)
		// Restore so other tests see the default again.
		assert.NoError(t, createCmd.Flags().Set(c.flag, c.def))
	}
}
