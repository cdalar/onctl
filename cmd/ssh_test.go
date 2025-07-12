package cmd

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestParseSSHConfigFile(t *testing.T) {
	// Create a temporary YAML configuration file
	configContent := `
port: 2222
applyFiles:
  - "script1.sh"
  - "script2.sh"
downloadFiles:
  - "file1.txt"
  - "file2.txt"
uploadFiles:
  - "upload1.txt"
  - "upload2.txt"
key: "~/.ssh/custom_key"
dotEnvFile: ".env"
variables:
  - "VAR1=value1"
  - "VAR2=value2"
`
	tmpFile, err := os.CreateTemp("", "ssh-config-*.yaml")
	assert.NoError(t, err)
	defer func() {
		if err := os.Remove(tmpFile.Name()); err != nil {
			t.Logf("Failed to remove temp file: %v", err)
		}
	}()

	_, err = tmpFile.Write([]byte(configContent))
	assert.NoError(t, err)
	if err := tmpFile.Close(); err != nil {
		t.Fatalf("Failed to close temp file: %v", err)
	}

	// Call the function to parse the configuration file
	config, err := parseSSHConfigFile(tmpFile.Name())
	assert.NoError(t, err)

	// Validate the parsed configuration
	assert.Equal(t, 2222, config.Port)
	assert.Equal(t, []string{"script1.sh", "script2.sh"}, config.ApplyFiles)
	assert.Equal(t, []string{"file1.txt", "file2.txt"}, config.DownloadFiles)
	assert.Equal(t, []string{"upload1.txt", "upload2.txt"}, config.UploadFiles)
	assert.Equal(t, "~/.ssh/custom_key", config.Key)
	assert.Equal(t, ".env", config.DotEnvFile)
	assert.Equal(t, []string{"VAR1=value1", "VAR2=value2"}, config.Variables)
}

func TestParseSSHConfigFile_NonExistentFile(t *testing.T) {
	// Test with non-existent file
	_, err := parseSSHConfigFile("/non/existent/file.yaml")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to open config file")
}

func TestParseSSHConfigFile_InvalidYAML(t *testing.T) {
	// Create a temporary file with invalid YAML
	invalidYAML := `
port: 2222
applyFiles:
  - "script1.sh
  - "script2.sh"  # Missing quote
`
	tmpFile, err := os.CreateTemp("", "invalid-ssh-config-*.yaml")
	assert.NoError(t, err)
	defer func() {
		if err := os.Remove(tmpFile.Name()); err != nil {
			t.Logf("Failed to remove temp file: %v", err)
		}
	}()

	_, err = tmpFile.Write([]byte(invalidYAML))
	assert.NoError(t, err)
	if err := tmpFile.Close(); err != nil {
		t.Fatalf("Failed to close temp file: %v", err)
	}

	// Test parsing invalid YAML
	_, err = parseSSHConfigFile(tmpFile.Name())
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to parse config file")
}

func TestSSHCmd_CommandProperties(t *testing.T) {
	// Test that the command has the expected properties
	assert.Equal(t, "ssh VM_NAME", sshCmd.Use)
	assert.Equal(t, "Spawn an SSH connection to a VM", sshCmd.Short)
	assert.NotNil(t, sshCmd.Args)
	assert.True(t, sshCmd.TraverseChildren)
	assert.True(t, sshCmd.DisableFlagsInUseLine)
	assert.NotNil(t, sshCmd.Run)
	assert.NotNil(t, sshCmd.ValidArgsFunction)
}

func TestSSHCmd_HasFlags(t *testing.T) {
	// Test that flags are properly registered
	flags := []struct {
		name      string
		shorthand string
		usage     string
		defValue  string
	}{
		{"key", "k", "Path to privateKey file (default: ~/.ssh/id_rsa))", ""},
		{"port", "p", "ssh port", "22"},
		{"apply-file", "a", "bash script file(s) to run on remote", "[]"},
		{"download", "d", "List of files to download", "[]"},
		{"upload", "u", "List of files to upload", "[]"},
		{"dot-env", "", "dot-env (.env) file", ""},
		{"vars", "e", "Environment variables passed to the script", "[]"},
		{"file", "f", "Path to configuration YAML file", ""},
	}

	for _, flag := range flags {
		f := sshCmd.Flags().Lookup(flag.name)
		assert.NotNil(t, f, "ssh command should have '%s' flag", flag.name)
		assert.Equal(t, flag.shorthand, f.Shorthand, "%s flag should have '%s' shorthand", flag.name, flag.shorthand)
		assert.Contains(t, f.Usage, flag.usage, "%s flag should have correct usage", flag.name)
	}
}

func TestCmdSSHOptions_StructBasics(t *testing.T) {
	// Test cmdSSHOptions struct creation and field access
	opts := cmdSSHOptions{
		Port:          2222,
		ApplyFiles:    []string{"script1.sh", "script2.sh"},
		DownloadFiles: []string{"file1.txt"},
		UploadFiles:   []string{"upload1.txt"},
		Key:           "/path/to/key",
		DotEnvFile:    ".env",
		Variables:     []string{"VAR1=value1"},
		ConfigFile:    "config.yaml",
	}

	assert.Equal(t, 2222, opts.Port)
	assert.Equal(t, []string{"script1.sh", "script2.sh"}, opts.ApplyFiles)
	assert.Equal(t, []string{"file1.txt"}, opts.DownloadFiles)
	assert.Equal(t, []string{"upload1.txt"}, opts.UploadFiles)
	assert.Equal(t, "/path/to/key", opts.Key)
	assert.Equal(t, ".env", opts.DotEnvFile)
	assert.Equal(t, []string{"VAR1=value1"}, opts.Variables)
	assert.Equal(t, "config.yaml", opts.ConfigFile)
}

func TestCmdSSHOptions_ZeroValues(t *testing.T) {
	// Test zero value cmdSSHOptions
	var opts cmdSSHOptions

	assert.Equal(t, 0, opts.Port)
	assert.Nil(t, opts.ApplyFiles)
	assert.Nil(t, opts.DownloadFiles)
	assert.Nil(t, opts.UploadFiles)
	assert.Equal(t, "", opts.Key)
	assert.Equal(t, "", opts.DotEnvFile)
	assert.Nil(t, opts.Variables)
	assert.Equal(t, "", opts.ConfigFile)
}
