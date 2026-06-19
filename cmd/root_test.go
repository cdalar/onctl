package cmd

import (
	"os"
	"testing"

	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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
	expectedProviders := []string{"aws", "hetzner", "azure", "gcp", "fc"}
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

// setAzureIdentifierFlags drives azure.subscriptionId/azure.resourceGroup
// through the bound --subscription-id/--resource-group flags (persistent on
// rootCmd, since every Azure-touching command needs them, not just create)
// rather than viper.Set: viper has no Unset, so a direct Set creates an
// "override" that outranks the flag binding for the rest of the test
// binary's life (see find()'s precedence order), permanently breaking any
// later test -- e.g. TestCreateFlagsBindToViper -- that exercises those same
// flags.
func setAzureIdentifierFlags(t *testing.T, subscriptionID, resourceGroup string) {
	t.Helper()
	require.NoError(t, rootCmd.PersistentFlags().Set("subscription-id", subscriptionID))
	require.NoError(t, rootCmd.PersistentFlags().Set("resource-group", resourceGroup))
}

func TestResolveAzureIdentifiers(t *testing.T) {
	t.Cleanup(func() { setAzureIdentifierFlags(t, "", "") })

	t.Run("already set", func(t *testing.T) {
		setAzureIdentifierFlags(t, "explicit-sub", "explicit-rg")
		assert.NoError(t, resolveAzureIdentifiers())
		assert.Equal(t, "explicit-sub", viper.GetString("azure.subscriptionId"))
		assert.Equal(t, "explicit-rg", viper.GetString("azure.resourceGroup"))
	})

	t.Run("fails clearly when unresolvable", func(t *testing.T) {
		setAzureIdentifierFlags(t, "", "")
		t.Setenv("PATH", "") // no az on PATH
		err := resolveAzureIdentifiers()
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "azure.subscriptionId is required")
	})

	t.Run("subscriptionId resolved, resourceGroup still required", func(t *testing.T) {
		setAzureIdentifierFlags(t, "explicit-sub", "")
		t.Setenv("PATH", "") // no az on PATH
		err := resolveAzureIdentifiers()
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "azure.resourceGroup is required")
	})
}
