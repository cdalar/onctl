package cmd

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestRestartCmd_CommandProperties(t *testing.T) {
	assert.Equal(t, "restart <name>", restartCmd.Use)
	assert.NotEmpty(t, restartCmd.Short)
	assert.NotNil(t, restartCmd.Run)
}

func TestRestartCmd_HasFlags(t *testing.T) {
	force := restartCmd.Flags().Lookup("force")
	assert.NotNil(t, force, "restart command should have 'force' flag")
	assert.Equal(t, "f", force.Shorthand)
	assert.Equal(t, "false", force.DefValue)

	kernel := restartCmd.Flags().Lookup("kernel-image")
	assert.NotNil(t, kernel, "restart command should have 'kernel-image' flag")
	assert.Equal(t, "", kernel.DefValue)
}

func TestRestartCmd_RequiresName(t *testing.T) {
	assert.Error(t, restartCmd.Args(restartCmd, []string{}))
	assert.NoError(t, restartCmd.Args(restartCmd, []string{"vm1"}))
}
