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
	defer os.Remove(tmpFile.Name()) // Clean up the temporary file

	_, err = tmpFile.Write([]byte(configContent))
	assert.NoError(t, err)
	tmpFile.Close()

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
