package cmd

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestPauseCmd_CommandProperties(t *testing.T) {
	assert.Equal(t, "pause <name>", pauseCmd.Use)
	assert.Contains(t, pauseCmd.Aliases, "stop")
	assert.NotEmpty(t, pauseCmd.Short)
	assert.NotNil(t, pauseCmd.Run)
}

func TestPauseCmd_HasFlags(t *testing.T) {
	force := pauseCmd.Flags().Lookup("force")
	assert.NotNil(t, force, "pause command should have 'force' flag")
	assert.Equal(t, "f", force.Shorthand)
	assert.Equal(t, "false", force.DefValue)

	hot := pauseCmd.Flags().Lookup("hot")
	assert.NotNil(t, hot, "pause command should have 'hot' flag")
	assert.Equal(t, "false", hot.DefValue)
}

func TestPauseCmd_FlagDefaults(t *testing.T) {
	assert.False(t, pauseForce, "force flag should default to false")
	assert.False(t, pauseHot, "hot flag should default to false")
}

// TestPauseCmd_NoArgsReturnsEarly verifies the guard that prevents touching the
// (nil-in-tests) provider when no VM name is given.
func TestPauseCmd_NoArgsReturnsEarly(t *testing.T) {
	assert.NotPanics(t, func() {
		pauseCmd.Run(pauseCmd, []string{})
	})
}
