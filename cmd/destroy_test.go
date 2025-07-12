package cmd

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestDestroyCmd_CommandProperties(t *testing.T) {
	// Test that the command has the expected properties
	assert.Equal(t, "destroy", destroyCmd.Use)
	assert.Contains(t, destroyCmd.Aliases, "down")
	assert.Contains(t, destroyCmd.Aliases, "delete")
	assert.Contains(t, destroyCmd.Aliases, "remove")
	assert.Contains(t, destroyCmd.Aliases, "rm")
	assert.Equal(t, "Destroy VM(s)", destroyCmd.Short)
	assert.NotNil(t, destroyCmd.Run)
	assert.NotNil(t, destroyCmd.ValidArgsFunction)
}

func TestDestroyCmd_HasFlags(t *testing.T) {
	// Test that flags are properly registered
	flag := destroyCmd.Flags().Lookup("force")
	assert.NotNil(t, flag, "destroy command should have 'force' flag")
	assert.Equal(t, "f", flag.Shorthand, "force flag should have 'f' shorthand")
	assert.Equal(t, "false", flag.DefValue, "force flag should have false default value")
	assert.Equal(t, "force destroy VM(s) without confirmation", flag.Usage)
}

func TestDestroyCmd_FlagDefaults(t *testing.T) {
	// Test that default values are correct
	assert.False(t, force, "force flag should default to false")
}
