package cmd

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestRootCmd_CommandProperties(t *testing.T) {
	// Test that the command has the expected properties
	assert.Equal(t, "onctl", rootCmd.Use)
	assert.Equal(t, "onctl is a tool to manage cross platform resources in cloud", rootCmd.Short)
	assert.Equal(t, "onctl is a tool to manage cross platform resources in cloud", rootCmd.Long)
	assert.Contains(t, rootCmd.Example, "onctl ls")
	assert.Contains(t, rootCmd.Example, "onctl create")
	assert.Contains(t, rootCmd.Example, "onctl ssh")
	assert.Contains(t, rootCmd.Example, "onctl destroy")
}

func TestRootCmd_HasSubCommands(t *testing.T) {
	// Test that root command has the expected subcommands
	subCommands := rootCmd.Commands()
	commandNames := make(map[string]bool)
	for _, cmd := range subCommands {
		commandNames[cmd.Name()] = true
	}

	expectedCommands := []string{"version", "ls", "create", "destroy", "ssh", "init", "templates"}
	for _, expected := range expectedCommands {
		assert.True(t, commandNames[expected], "root command should have '%s' subcommand", expected)
	}
}

func TestCheckCloudProvider_WithEnvVar(t *testing.T) {
	// Save original env var
	originalEnv := os.Getenv("ONCTL_CLOUD")
	defer func() { _ = os.Setenv("ONCTL_CLOUD", originalEnv) }()

	// Test with valid cloud provider
	_ = os.Setenv("ONCTL_CLOUD", "aws")
	result := checkCloudProvider()
	assert.Equal(t, "aws", result)

	// Test with another valid provider
	_ = os.Setenv("ONCTL_CLOUD", "hetzner")
	result = checkCloudProvider()
	assert.Equal(t, "hetzner", result)
}

func TestCloudProviderList(t *testing.T) {
	// Test that cloud provider list contains expected providers
	expectedProviders := []string{"aws", "hetzner", "azure", "gcp","proxmox"}
	assert.Equal(t, expectedProviders, cloudProviderList)
}

func TestVariableDeclarations(t *testing.T) {
	// Test that global variables are properly declared
	assert.NotNil(t, rootCmd)
	assert.NotEmpty(t, cloudProviderList)
}

func TestExecute_Function(t *testing.T) {
	// Test that Execute function exists and is callable
	// We can't actually call it in tests as it would try to read config files
	// but we can verify it's properly declared
	assert.NotNil(t, Execute)
}
