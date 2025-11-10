package cmd

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestListCmd_CommandProperties(t *testing.T) {
	// Test that the command has the expected properties
	assert.Equal(t, "ls", listCmd.Use)
	assert.Contains(t, listCmd.Aliases, "list")
	assert.Equal(t, "List VMs", listCmd.Short)
	assert.NotNil(t, listCmd.Run)
}

func TestListCmd_HasFlags(t *testing.T) {
	// Test that flags are properly registered
	flag := listCmd.Flags().Lookup("output")
	assert.NotNil(t, flag, "list command should have 'output' flag")
	assert.Equal(t, "o", flag.Shorthand, "output flag should have 'o' shorthand")
	assert.Equal(t, "tab", flag.DefValue, "output flag should have 'tab' default value")
	assert.Equal(t, "output format (tab, json, yaml, puppet, ansible)", flag.Usage)
}

func TestListCmd_FlagDefaults(t *testing.T) {
	// Test that default values are correct
	assert.Equal(t, "tab", output, "output flag should default to 'tab'")
}
