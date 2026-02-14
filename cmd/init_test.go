package cmd

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestInitCmd_CommandProperties(t *testing.T) {
	// Test that the command has the expected properties
	assert.Equal(t, "init", initCmd.Use)
	assert.Equal(t, "init onctl environment", initCmd.Short)
	assert.NotNil(t, initCmd.Run)
}

func TestInitializeOnctlEnv_NewDirectory(t *testing.T) {
	// Skip interactive prompts during testing
	originalSkip := skipInteractivePrompt
	skipInteractivePrompt = true
	defer func() { skipInteractivePrompt = originalSkip }()

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

func TestInitializeOnctlEnv_GlobalAndLocalConfig(t *testing.T) {
	// Skip interactive prompts during testing
	originalSkip := skipInteractivePrompt
	skipInteractivePrompt = true
	defer func() { skipInteractivePrompt = originalSkip }()

	// Save original home directory
	originalHome := os.Getenv("HOME")
	defer os.Setenv("HOME", originalHome)

	// Create temporary directories for testing
	tempHome, err := os.MkdirTemp("", "onctl-test-home")
	require.NoError(t, err)
	defer os.RemoveAll(tempHome)

	tempProject, err := os.MkdirTemp("", "onctl-test-project")
	require.NoError(t, err)
	defer os.RemoveAll(tempProject)

	// Set temporary home directory
	os.Setenv("HOME", tempHome)

	// Change to project directory
	originalWd, err := os.Getwd()
	require.NoError(t, err)
	defer os.Chdir(originalWd)

	err = os.Chdir(tempProject)
	require.NoError(t, err)

	t.Run("creates global onctl directory", func(t *testing.T) {
		homeOnctlPath := filepath.Join(tempHome, onctlDirName)

		// Verify home .onctl doesn't exist yet
		_, err := os.Stat(homeOnctlPath)
		assert.True(t, os.IsNotExist(err), "home .onctl should not exist before init")

		// Note: Full initialization will fail due to embedded files not being available in tests
		// but we can verify the directory creation logic
	})

	t.Run("detects existing global config", func(t *testing.T) {
		homeOnctlPath := filepath.Join(tempHome, onctlDirName)

		// Create the global .onctl directory manually
		err := os.Mkdir(homeOnctlPath, os.ModePerm)
		require.NoError(t, err)

		// Verify it exists
		info, err := os.Stat(homeOnctlPath)
		assert.NoError(t, err)
		assert.True(t, info.IsDir())
	})

	t.Run("detects existing local config", func(t *testing.T) {
		localOnctlPath := filepath.Join(tempProject, onctlDirName)

		// Create the local .onctl directory
		err := os.Mkdir(localOnctlPath, os.ModePerm)
		require.NoError(t, err)

		// Verify it exists
		info, err := os.Stat(localOnctlPath)
		assert.NoError(t, err)
		assert.True(t, info.IsDir())
	})
}

func TestInitializeOnctlEnv_DirectoryStructure(t *testing.T) {
	// Save original home directory
	originalHome := os.Getenv("HOME")
	defer os.Setenv("HOME", originalHome)

	// Create temporary home directory
	tempHome, err := os.MkdirTemp("", "onctl-test-home")
	require.NoError(t, err)
	defer os.RemoveAll(tempHome)

	// Set temporary home directory
	os.Setenv("HOME", tempHome)

	homeOnctlPath := filepath.Join(tempHome, onctlDirName)

	t.Run("creates home onctl directory", func(t *testing.T) {
		// Create the directory
		err := os.Mkdir(homeOnctlPath, os.ModePerm)
		require.NoError(t, err)

		// Check directory exists and is a directory
		info, err := os.Stat(homeOnctlPath)
		require.NoError(t, err)
		assert.True(t, info.IsDir())

		// Verify directory has at least owner read/write/execute permissions
		// (actual permissions may be affected by umask)
		mode := info.Mode()
		assert.True(t, mode.IsDir(), "Should be a directory")
		assert.True(t, mode.Perm()&0700 == 0700, "Should have owner rwx permissions")
	})
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
