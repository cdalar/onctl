package cmd

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestVersionCmd_CommandProperties(t *testing.T) {
	// Test that the command has the expected properties
	assert.Equal(t, "version", versionCmd.Use)
	assert.Equal(t, "Print the version number of onctl", versionCmd.Short)
	assert.NotNil(t, versionCmd.Run)
}

func TestVersion_Variable(t *testing.T) {
	// Test that Version variable is properly declared
	assert.Equal(t, "Not Set", Version)
}

func TestVersionCmd_RunFunction(t *testing.T) {
	// Test that the Run function is not nil and callable
	assert.NotNil(t, versionCmd.Run)
	
	// We can test that the function doesn't panic when called
	assert.NotPanics(t, func() {
		versionCmd.Run(versionCmd, []string{})
	})
}
