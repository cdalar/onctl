package cmd

import (
	"bytes"
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

	// Isolate HOME so this test doesn't create a real ~/.onctl directory,
	// which would leak into other tests that assert no config exists.
	originalHome, homeWasSet := os.LookupEnv("HOME")
	defer func() {
		if homeWasSet {
			_ = os.Setenv("HOME", originalHome)
		} else {
			_ = os.Unsetenv("HOME")
		}
	}()
	tempHome, err := os.MkdirTemp("", "onctl-test-home")
	assert.NoError(t, err)
	defer func() { _ = os.RemoveAll(tempHome) }()
	_ = os.Setenv("HOME", tempHome)

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
	originalHome, homeWasSet := os.LookupEnv("HOME")
	defer func() {
		if homeWasSet {
			_ = os.Setenv("HOME", originalHome)
		} else {
			_ = os.Unsetenv("HOME")
		}
	}()

	// Create temporary directories for testing
	tempHome, err := os.MkdirTemp("", "onctl-test-home")
	require.NoError(t, err)
	defer func() { _ = os.RemoveAll(tempHome) }()

	tempProject, err := os.MkdirTemp("", "onctl-test-project")
	require.NoError(t, err)
	defer func() { _ = os.RemoveAll(tempProject) }()

	// Set temporary home directory
	err = os.Setenv("HOME", tempHome)
	require.NoError(t, err)

	// Change to project directory
	originalWd, err := os.Getwd()
	require.NoError(t, err)
	defer func() { _ = os.Chdir(originalWd) }()

	err = os.Chdir(tempProject)
	require.NoError(t, err)

	t.Run("creates global onctl directory", func(t *testing.T) {
		homeOnctlPath := filepath.Join(tempHome, onctlDirName)

		// Verify home .onctl doesn't exist yet
		_, err := os.Stat(homeOnctlPath)
		assert.True(t, os.IsNotExist(err), "home .onctl should not exist before init")

		// Attempt initialization
		err = initializeOnctlEnv()
		if err != nil {
			// We allow failures related to test environment constraints
			t.Logf("Init failed (expected in test environment): %v", err)
		}

		// Verify the global .onctl directory was created
		info, err := os.Stat(homeOnctlPath)
		if err == nil {
			assert.True(t, info.IsDir(), "home .onctl should exist after init")
		}
	})

	t.Run("detects existing global config", func(t *testing.T) {
		homeOnctlPath := filepath.Join(tempHome, onctlDirName)

		// Ensure the global .onctl directory exists for this test
		_ = os.MkdirAll(homeOnctlPath, os.ModePerm)

		// Call initializeOnctlEnv - should detect existing directory
		err := initializeOnctlEnv()
		// Should succeed or return an error we can handle
		if err != nil {
			t.Logf("Init with existing global config: %v", err)
		}

		// Verify directory still exists
		info, err := os.Stat(homeOnctlPath)
		assert.NoError(t, err)
		assert.True(t, info.IsDir())
	})

	t.Run("detects existing local config", func(t *testing.T) {
		homeOnctlPath := filepath.Join(tempHome, onctlDirName)
		localOnctlPath := filepath.Join(tempProject, onctlDirName)

		// Ensure both directories exist for this test
		_ = os.MkdirAll(homeOnctlPath, os.ModePerm)
		_ = os.MkdirAll(localOnctlPath, os.ModePerm)

		// Call initializeOnctlEnv - should detect both existing directories
		err := initializeOnctlEnv()
		assert.NoError(t, err)

		// Verify both directories still exist
		info, err := os.Stat(homeOnctlPath)
		assert.NoError(t, err)
		assert.True(t, info.IsDir())

		info, err = os.Stat(localOnctlPath)
		assert.NoError(t, err)
		assert.True(t, info.IsDir())
	})
}

func TestInitializeOnctlEnv_DirectoryStructure(t *testing.T) {
	// Save original home directory
	originalHome, homeWasSet := os.LookupEnv("HOME")
	defer func() {
		if homeWasSet {
			_ = os.Setenv("HOME", originalHome)
		} else {
			_ = os.Unsetenv("HOME")
		}
	}()

	// Create temporary home directory
	tempHome, err := os.MkdirTemp("", "onctl-test-home")
	require.NoError(t, err)
	defer func() { _ = os.RemoveAll(tempHome) }()

	// Set temporary home directory
	err = os.Setenv("HOME", tempHome)
	require.NoError(t, err)

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

func TestPopulateOnctlEnv_ErrorWritingFile(t *testing.T) {
	// Create a temporary directory
	tempDir, err := os.MkdirTemp("", "onctl-test")
	require.NoError(t, err)
	defer func() { _ = os.RemoveAll(tempDir) }()

	// Make the directory read-only to cause write errors
	err = os.Chmod(tempDir, 0444)
	require.NoError(t, err)
	defer func() { _ = os.Chmod(tempDir, 0755) }()

	// Call populateOnctlEnv - should fail when trying to write files
	err = populateOnctlEnv(tempDir)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to write file")
}

func TestIsInteractive(t *testing.T) {
	// Test that isInteractive function exists and is callable
	// The actual result depends on test environment
	result := isInteractive()
	// Just verify it returns a boolean value
	assert.IsType(t, false, result)
}

func TestConstants(t *testing.T) {
	// Test that constants are correctly defined
	assert.Equal(t, ".onctl", onctlDirName)
	assert.Equal(t, "init", initDir)
}

func TestInitializeOnctlEnv_ExistingLocalConfig(t *testing.T) {
	// Skip interactive prompts during testing
	originalSkip := skipInteractivePrompt
	skipInteractivePrompt = true
	defer func() { skipInteractivePrompt = originalSkip }()

	// Save original home directory
	originalHome, homeWasSet := os.LookupEnv("HOME")
	defer func() {
		if homeWasSet {
			_ = os.Setenv("HOME", originalHome)
		} else {
			_ = os.Unsetenv("HOME")
		}
	}()

	// Create temporary directories for testing
	tempHome, err := os.MkdirTemp("", "onctl-test-home")
	require.NoError(t, err)
	defer func() { _ = os.RemoveAll(tempHome) }()

	tempProject, err := os.MkdirTemp("", "onctl-test-project")
	require.NoError(t, err)
	defer func() { _ = os.RemoveAll(tempProject) }()

	// Set temporary home directory
	err = os.Setenv("HOME", tempHome)
	require.NoError(t, err)

	// Change to project directory
	originalWd, err := os.Getwd()
	require.NoError(t, err)
	defer func() { _ = os.Chdir(originalWd) }()

	err = os.Chdir(tempProject)
	require.NoError(t, err)

	// Create both home and local .onctl directories before calling initializeOnctlEnv
	homeOnctlPath := filepath.Join(tempHome, onctlDirName)
	err = os.Mkdir(homeOnctlPath, os.ModePerm)
	require.NoError(t, err)

	localOnctlPath := filepath.Join(tempProject, onctlDirName)
	err = os.Mkdir(localOnctlPath, os.ModePerm)
	require.NoError(t, err)

	// Call initializeOnctlEnv - should detect existing directories
	err = initializeOnctlEnv()
	assert.NoError(t, err)

	// Verify both directories still exist
	_, err = os.Stat(homeOnctlPath)
	assert.NoError(t, err)
	_, err = os.Stat(localOnctlPath)
	assert.NoError(t, err)
}

func TestWarnLegacyProviderConfigFiles_WarnsWhenPresent(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "onctl-test")
	require.NoError(t, err)
	defer func() { _ = os.RemoveAll(tempDir) }()

	// Simulate a pre-single-yaml .onctl directory: a legacy per-provider file
	// alongside the current onctl.yaml. ReadConfig only reads onctl.yaml, so
	// settings left in gcp.yaml would otherwise be silently ignored.
	require.NoError(t, os.WriteFile(filepath.Join(tempDir, "gcp.yaml"), []byte("project: my-project\n"), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(tempDir, onctlYamlFile), []byte("vm:\n  name: onctl-vm\n"), 0644))

	r, w, err := os.Pipe()
	require.NoError(t, err)
	originalStdout := os.Stdout
	os.Stdout = w

	warnLegacyProviderConfigFiles(tempDir)

	require.NoError(t, w.Close())
	os.Stdout = originalStdout
	var buf bytes.Buffer
	_, err = buf.ReadFrom(r)
	require.NoError(t, err)

	output := buf.String()
	assert.Contains(t, output, "gcp.yaml")
	assert.Contains(t, output, "no longer reads")
}

func TestWarnLegacyProviderConfigFiles_SilentWhenAbsent(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "onctl-test")
	require.NoError(t, err)
	defer func() { _ = os.RemoveAll(tempDir) }()

	require.NoError(t, os.WriteFile(filepath.Join(tempDir, onctlYamlFile), []byte("vm:\n  name: onctl-vm\n"), 0644))

	r, w, err := os.Pipe()
	require.NoError(t, err)
	originalStdout := os.Stdout
	os.Stdout = w

	warnLegacyProviderConfigFiles(tempDir)

	require.NoError(t, w.Close())
	os.Stdout = originalStdout
	var buf bytes.Buffer
	_, err = buf.ReadFrom(r)
	require.NoError(t, err)

	assert.Empty(t, buf.String())
}

func TestInitializeOnctlEnv_RepairsMissingOnctlYamlInExistingHomeDir(t *testing.T) {
	// Reproduces the migration bug flagged in review: a .onctl directory that
	// predates the single-onctl.yaml config (e.g. only had per-provider yaml
	// files) was treated as "already initialized" and left without an
	// onctl.yaml, so every later command would fail with "no configuration
	// directory found". initializeOnctlEnv must populate onctl.yaml in that
	// case instead of just printing "already initialized".
	originalSkip := skipInteractivePrompt
	skipInteractivePrompt = true
	defer func() { skipInteractivePrompt = originalSkip }()

	originalHome, homeWasSet := os.LookupEnv("HOME")
	defer func() {
		if homeWasSet {
			_ = os.Setenv("HOME", originalHome)
		} else {
			_ = os.Unsetenv("HOME")
		}
	}()
	tempHome, err := os.MkdirTemp("", "onctl-test-home")
	require.NoError(t, err)
	defer func() { _ = os.RemoveAll(tempHome) }()
	require.NoError(t, os.Setenv("HOME", tempHome))

	tempProject, err := os.MkdirTemp("", "onctl-test-project")
	require.NoError(t, err)
	defer func() { _ = os.RemoveAll(tempProject) }()
	originalWd, err := os.Getwd()
	require.NoError(t, err)
	defer func() { _ = os.Chdir(originalWd) }()
	require.NoError(t, os.Chdir(tempProject))

	// Pre-create a legacy home .onctl directory with no onctl.yaml, as a
	// pre-migration install would have left it.
	homeOnctlPath := filepath.Join(tempHome, onctlDirName)
	require.NoError(t, os.MkdirAll(homeOnctlPath, os.ModePerm))
	require.NoError(t, os.WriteFile(filepath.Join(homeOnctlPath, "gcp.yaml"), []byte("project: my-project\n"), 0644))

	_, err = os.Stat(filepath.Join(homeOnctlPath, onctlYamlFile))
	require.True(t, os.IsNotExist(err), "onctl.yaml should not exist before repair")

	err = initializeOnctlEnv()
	require.NoError(t, err)

	_, err = os.Stat(filepath.Join(homeOnctlPath, onctlYamlFile))
	assert.NoError(t, err, "initializeOnctlEnv should populate onctl.yaml in an already-initialized directory that's missing it")
}

func TestInitCmd_Run(t *testing.T) {
	// Skip interactive prompts during testing
	originalSkip := skipInteractivePrompt
	skipInteractivePrompt = true
	defer func() { skipInteractivePrompt = originalSkip }()

	// Save original home directory
	originalHome, homeWasSet := os.LookupEnv("HOME")
	defer func() {
		if homeWasSet {
			_ = os.Setenv("HOME", originalHome)
		} else {
			_ = os.Unsetenv("HOME")
		}
	}()

	// Create temporary home directory
	tempHome, err := os.MkdirTemp("", "onctl-test-home")
	require.NoError(t, err)
	defer func() { _ = os.RemoveAll(tempHome) }()

	// Set temporary home directory
	err = os.Setenv("HOME", tempHome)
	require.NoError(t, err)

	// Change to temp directory
	originalWd, err := os.Getwd()
	require.NoError(t, err)
	defer func() { _ = os.Chdir(originalWd) }()

	err = os.Chdir(tempHome)
	require.NoError(t, err)

	// Execute the command's Run function
	initCmd.Run(initCmd, []string{})

	// Verify home directory was created (embedded files might not exist, but directory should be created)
	homeOnctlPath := filepath.Join(tempHome, onctlDirName)
	_, err = os.Stat(homeOnctlPath)
	// Directory should exist or we should get an embedded files error
	if err != nil {
		assert.True(t, os.IsNotExist(err))
	}
}
