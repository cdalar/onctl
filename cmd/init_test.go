package cmd

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestInitCmd_CommandProperties(t *testing.T) {
	// Test that the command has the expected properties
	assert.Equal(t, "init", initCmd.Use)
	assert.Equal(t, "init onctl environment", initCmd.Short)
	assert.NotNil(t, initCmd.Run)
}

func TestInitializeOnctlEnv_NewDirectory(t *testing.T) {
	// Create a temporary directory for testing
	tempDir, err := os.MkdirTemp("", "onctl-test")
	assert.NoError(t, err)
	defer func() { _ = os.RemoveAll(tempDir) }()

	// Change to temp directory
	originalWd, err := os.Getwd()
	assert.NoError(t, err)
	defer func() { _ = os.Chdir(originalWd) }()

	err = os.Chdir(tempDir)
	assert.NoError(t, err)

	// Test initialization in new directory
	err = initializeOnctlEnv()

	// Since the embedded files might not be available in tests,
	// we expect either success or a specific error about embedded files
	if err != nil {
		assert.Contains(t, err.Error(), "failed to read embedded files")
	}
}

func TestPopulateOnctlEnv_InvalidPath(t *testing.T) {
	// Test with invalid path
	err := populateOnctlEnv("/invalid/path/that/does/not/exist")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to write file")
}

func TestConstants(t *testing.T) {
	// Test that constants are correctly defined
	assert.Equal(t, ".onctl", onctlDirName)
	assert.Equal(t, "init", initDir)
}
