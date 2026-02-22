package cmd

import (
	"fmt"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestParseConfigFile(t *testing.T) {
	// Create a temporary YAML configuration file
	configContent := `
publicKeyFile: "~/.ssh/id_rsa.pub"
applyFiles:
  - "script1.sh"
  - "script2.sh"
dotEnvFile: ".env"
variables:
  - "VAR1=value1"
  - "VAR2=value2"
vm:
  name: "test-vm"
  sshPort: 2222
  cloudInitFile: "cloud-init.yaml"
domain: "example.com"
`
	tmpFile, err := os.CreateTemp("", "config-*.yaml")
	assert.NoError(t, err)
	defer func() {
		if err := os.Remove(tmpFile.Name()); err != nil {
			t.Logf("Failed to remove temp file: %v", err)
		}
	}() // Clean up the temporary file

	_, err = tmpFile.Write([]byte(configContent))
	assert.NoError(t, err)
	if err := tmpFile.Close(); err != nil {
		t.Fatalf("Failed to close temp file: %v", err)
	}

	// Call the function to parse the configuration file
	config, err := parseConfigFile(tmpFile.Name())
	assert.NoError(t, err)

	fmt.Printf("Parsed config: %+v\n", config)

	// Validate the parsed configuration
	assert.Equal(t, "~/.ssh/id_rsa.pub", config.PublicKeyFile)
	assert.Equal(t, []string{"script1.sh", "script2.sh"}, config.ApplyFiles)
	assert.Equal(t, ".env", config.DotEnvFile)
	assert.Equal(t, []string{"VAR1=value1", "VAR2=value2"}, config.Variables)
	assert.Equal(t, "test-vm", config.Vm.Name)
	assert.Equal(t, 2222, config.Vm.SSHPort)
	assert.Equal(t, "cloud-init.yaml", config.Vm.CloudInitFile)
	assert.Equal(t, "example.com", config.Domain)
}

func TestParseConfigFile_NonExistentFile(t *testing.T) {
	// Test with non-existent file
	_, err := parseConfigFile("/non/existent/file.yaml")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to open config file")
}

func TestParseConfigFile_InvalidYAML(t *testing.T) {
	// Create a temporary file with invalid YAML
	invalidYAML := `
publicKeyFile: "~/.ssh/id_rsa.pub
applyFiles:
  - "script1.sh"
  - "script2.sh"  # Missing quote above
`
	tmpFile, err := os.CreateTemp("", "invalid-config-*.yaml")
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
	_, err = parseConfigFile(tmpFile.Name())
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to parse config file")
}

func TestCreateCmd_CommandProperties(t *testing.T) {
	// Test that the command has the expected properties
	assert.Equal(t, "create", createCmd.Use)
	assert.Contains(t, createCmd.Aliases, "start")
	assert.Contains(t, createCmd.Aliases, "up")
	assert.Equal(t, "Create a VM", createCmd.Short)
	assert.Equal(t, "Create a VM with the specified options and run the cloud-init file on the remote.", createCmd.Long)
	assert.Contains(t, createCmd.Example, "onctl create")
	assert.NotNil(t, createCmd.Run)
}

func TestCreateCmd_HasFlags(t *testing.T) {
	// Test that flags are properly registered
	flags := []struct {
		name      string
		shorthand string
		usage     string
	}{
		{"publicKey", "k", "Path to publicKey file (default: ~/.ssh/id_rsa))"},
		{"apply-file", "a", "bash script file(s) to run on remote"},
		{"download", "d", "List of files to download"},
		{"upload", "u", "List of files to upload"},
		{"name", "n", "vm name"},
		{"ssh-port", "p", "ssh port"},
		{"cloud-init", "i", "cloud-init file"},
		{"dot-env", "", "dot-env (.env) file"},
		{"domain", "", "request a domain name for the VM"},
		{"vars", "e", "Environment variables passed to the script"},
		{"file", "f", "Path to configuration YAML file"},
	}

	for _, flag := range flags {
		f := createCmd.Flags().Lookup(flag.name)
		assert.NotNil(t, f, "create command should have '%s' flag", flag.name)
		assert.Equal(t, flag.shorthand, f.Shorthand, "%s flag should have '%s' shorthand", flag.name, flag.shorthand)
		assert.Contains(t, f.Usage, flag.usage, "%s flag should have correct usage", flag.name)
	}
}

func TestCreateCmd_HasUsageTemplate(t *testing.T) {
	// Test that create command has usage template with environment variables
	usage := createCmd.UsageTemplate()
	assert.Contains(t, usage, "Environment Variables")
	assert.Contains(t, usage, "CLOUDFLARE_API_TOKEN")
	assert.Contains(t, usage, "CLOUDFLARE_ZONE_ID")
}

func TestSSHCmd_ValidArgsFunction(t *testing.T) {
	// Test that SSH command's ValidArgsFunction exists and is callable
	assert.NotNil(t, sshCmd.ValidArgsFunction)

	// We can't test the actual function since it requires a provider,
	// but we can verify it's properly defined
}

func TestDestroyCmd_ValidArgsFunction(t *testing.T) {
	// Test that destroy command's ValidArgsFunction exists and is callable
	assert.NotNil(t, destroyCmd.ValidArgsFunction)
}

func TestCmdCreateOptions_StructBasics(t *testing.T) {
	// Test cmdCreateOptions struct creation and field access
	opts := cmdCreateOptions{
		PublicKeyFile: "/path/to/key.pub",
		ApplyFiles:    []string{"script1.sh", "script2.sh"},
		DotEnvFile:    ".env",
		Variables:     []string{"VAR1=value1", "VAR2=value2"},
		Domain:        "example.com",
		DownloadFiles: []string{"file1.txt"},
		UploadFiles:   []string{"file2.txt"},
		ConfigFile:    "config.yaml",
	}
	opts.Vm.Name = "test-vm"
	opts.Vm.SSHPort = 2222

	assert.Equal(t, "/path/to/key.pub", opts.PublicKeyFile)
	assert.Equal(t, []string{"script1.sh", "script2.sh"}, opts.ApplyFiles)
	assert.Equal(t, ".env", opts.DotEnvFile)
	assert.Equal(t, []string{"VAR1=value1", "VAR2=value2"}, opts.Variables)
	assert.Equal(t, "example.com", opts.Domain)
	assert.Equal(t, []string{"file1.txt"}, opts.DownloadFiles)
	assert.Equal(t, []string{"file2.txt"}, opts.UploadFiles)
	assert.Equal(t, "config.yaml", opts.ConfigFile)
	assert.Equal(t, "test-vm", opts.Vm.Name)
	assert.Equal(t, 2222, opts.Vm.SSHPort)
}

func TestCmdCreateOptions_ZeroValues(t *testing.T) {
	// Test zero value cmdCreateOptions
	var opts cmdCreateOptions

	assert.Equal(t, "", opts.PublicKeyFile)
	assert.Nil(t, opts.ApplyFiles)
	assert.Equal(t, "", opts.DotEnvFile)
	assert.Nil(t, opts.Variables)
	assert.Equal(t, "", opts.Domain)
	assert.Nil(t, opts.DownloadFiles)
	assert.Nil(t, opts.UploadFiles)
	assert.Equal(t, "", opts.ConfigFile)
	assert.Equal(t, "", opts.Vm.Name)
	assert.Equal(t, 0, opts.Vm.SSHPort)
}
