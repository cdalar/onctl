package cmd

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestResumeCmd_CommandProperties(t *testing.T) {
	assert.Equal(t, "resume <name>", resumeCmd.Use)
	assert.NotEmpty(t, resumeCmd.Short)
	assert.NotNil(t, resumeCmd.Run)
}

func TestResumeCmd_HasFlags(t *testing.T) {
	key := resumeCmd.Flags().Lookup("publicKey")
	assert.NotNil(t, key, "resume command should have 'publicKey' flag")
	assert.Equal(t, "k", key.Shorthand)
}

// TestResumeCmd_NoArgsReturnsEarly verifies the guard that prevents touching the
// (nil-in-tests) provider when no VM name is given.
func TestResumeCmd_NoArgsReturnsEarly(t *testing.T) {
	assert.NotPanics(t, func() {
		resumeCmd.Run(resumeCmd, []string{})
	})
}
