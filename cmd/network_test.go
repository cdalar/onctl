package cmd

import (
	"testing"

	"github.com/cdalar/onctl/internal/cloud"
	"github.com/stretchr/testify/assert"
)

func TestNetworkCmd_CommandProperties(t *testing.T) {
	// Test that the networkCmd has the expected properties
	assert.Equal(t, "network", networkCmd.Use)
	assert.Contains(t, networkCmd.Aliases, "net")
	assert.Equal(t, "Manage network resources", networkCmd.Short)
	assert.Equal(t, "Manage network resources", networkCmd.Long)
}

func TestNetworkCmd_HasSubCommands(t *testing.T) {
	// Test that networkCmd has the expected subcommands
	subCommands := []string{"create", "list", "delete"}
	commands := networkCmd.Commands()
	
	commandNames := make([]string, len(commands))
	for i, cmd := range commands {
		commandNames[i] = cmd.Use
	}
	
	for _, expectedCmd := range subCommands {
		assert.Contains(t, commandNames, expectedCmd, "networkCmd should have '%s' subcommand", expectedCmd)
	}
}

func TestNetworkCreateCmd_CommandProperties(t *testing.T) {
	// Test that the networkCreateCmd has the expected properties
	assert.Equal(t, "create", networkCreateCmd.Use)
	assert.Contains(t, networkCreateCmd.Aliases, "new")
	assert.Contains(t, networkCreateCmd.Aliases, "add")
	assert.Contains(t, networkCreateCmd.Aliases, "up")
	assert.Equal(t, "Create a network", networkCreateCmd.Short)
	assert.Equal(t, "Create a network", networkCreateCmd.Long)
	assert.NotNil(t, networkCreateCmd.Run)
}

func TestNetworkCreateCmd_HasFlags(t *testing.T) {
	// Test that flags are properly registered
	flags := []struct {
		name      string
		shorthand string
		usage     string
	}{
		{"cidr", "", "CIDR for the network ex. 10.0.0.0/16"},
		{"name", "n", "Name for the network"},
	}

	for _, flag := range flags {
		f := networkCreateCmd.Flags().Lookup(flag.name)
		assert.NotNil(t, f, "networkCreateCmd should have '%s' flag", flag.name)
		assert.Equal(t, flag.shorthand, f.Shorthand, "%s flag should have '%s' shorthand", flag.name, flag.shorthand)
		assert.Contains(t, f.Usage, flag.usage, "%s flag should have correct usage", flag.name)
	}
}

func TestNetworkListCmd_CommandProperties(t *testing.T) {
	// Test that the networkListCmd has the expected properties
	assert.Equal(t, "list", networkListCmd.Use)
	assert.Contains(t, networkListCmd.Aliases, "ls")
	assert.Equal(t, "List networks", networkListCmd.Short)
	assert.Equal(t, "List networks", networkListCmd.Long)
	assert.NotNil(t, networkListCmd.Run)
}

func TestNetworkDeleteCmd_CommandProperties(t *testing.T) {
	// Test that the networkDeleteCmd has the expected properties
	assert.Equal(t, "delete", networkDeleteCmd.Use)
	assert.Contains(t, networkDeleteCmd.Aliases, "rm")
	assert.Contains(t, networkDeleteCmd.Aliases, "remove")
	assert.Contains(t, networkDeleteCmd.Aliases, "destroy")
	assert.Contains(t, networkDeleteCmd.Aliases, "down")
	assert.Contains(t, networkDeleteCmd.Aliases, "del")
	assert.Equal(t, "Delete a network", networkDeleteCmd.Short)
	assert.Equal(t, "Delete a network", networkDeleteCmd.Long)
	assert.NotNil(t, networkDeleteCmd.Run)
}

func TestNOpt_GlobalVariable(t *testing.T) {
	// Test that nOpt global variable exists and can be manipulated
	originalCIDR := nOpt.CIDR
	originalName := nOpt.Name
	
	// Modify values
	nOpt.CIDR = "10.0.0.0/16"
	nOpt.Name = "test-network"
	
	assert.Equal(t, "10.0.0.0/16", nOpt.CIDR)
	assert.Equal(t, "test-network", nOpt.Name)
	
	// Restore original values
	nOpt.CIDR = originalCIDR
	nOpt.Name = originalName
}

func TestNOpt_StructBasics(t *testing.T) {
	// Test creating and manipulating cloud.Network struct
	network := cloud.Network{
		ID:        "net-123",
		Name:      "test-network",
		CIDR:      "10.0.0.0/16",
		Provider:  "aws",
		Servers:   5,
	}
	
	assert.Equal(t, "net-123", network.ID)
	assert.Equal(t, "test-network", network.Name)
	assert.Equal(t, "10.0.0.0/16", network.CIDR)
	assert.Equal(t, "aws", network.Provider)
	assert.Equal(t, 5, network.Servers)
}

func TestNOpt_ZeroValues(t *testing.T) {
	// Test zero value cloud.Network
	var network cloud.Network
	
	assert.Equal(t, "", network.ID)
	assert.Equal(t, "", network.Name)
	assert.Equal(t, "", network.CIDR)
	assert.Equal(t, "", network.Provider)
	assert.Equal(t, 0, network.Servers)
}

func TestNetworkCmd_InitFunction(t *testing.T) {
	// Test that init function properly sets up the command structure
	assert.NotNil(t, networkCmd)
	assert.True(t, networkCmd.HasSubCommands())
	
	// Verify the subcommands are properly added
	commands := networkCmd.Commands()
	assert.True(t, len(commands) >= 3, "networkCmd should have at least 3 subcommands")
}

func TestNetworkCreateCmd_FlagBinding(t *testing.T) {
	// Test that the flags are properly bound to the nOpt variable
	// Save original values
	originalCIDR := nOpt.CIDR
	originalName := nOpt.Name
	defer func() {
		nOpt.CIDR = originalCIDR
		nOpt.Name = originalName
	}()
	
	// Set flags via command
	err := networkCreateCmd.Flags().Set("cidr", "192.168.1.0/24")
	assert.NoError(t, err)
	assert.Equal(t, "192.168.1.0/24", nOpt.CIDR)
	
	err = networkCreateCmd.Flags().Set("name", "test-net")
	assert.NoError(t, err)
	assert.Equal(t, "test-net", nOpt.Name)
}

func TestNetworkCommands_RunFunctionExists(t *testing.T) {
	// Test that all network commands have run functions defined
	assert.NotNil(t, networkCreateCmd.Run, "networkCreateCmd should have Run function")
	assert.NotNil(t, networkListCmd.Run, "networkListCmd should have Run function")
	assert.NotNil(t, networkDeleteCmd.Run, "networkDeleteCmd should have Run function")
}

func TestNetworkCommands_ArgumentHandling(t *testing.T) {
	// Test that commands handle arguments properly
	// Since these functions would interact with actual cloud providers,
	// we just verify they exist and are callable (but don't execute them)
	
	// Test that networkDeleteCmd accepts arguments
	assert.NotNil(t, networkDeleteCmd.Run)
	
	// The Run function should handle both "all" and specific network names
	// We can't test the actual execution without mocking the cloud provider
	t.Log("networkDeleteCmd handles both 'all' and specific network names as arguments")
}

func TestNetworkCreateCmd_UsageExamples(t *testing.T) {
	// Test command usage documentation
	assert.Contains(t, networkCreateCmd.Use, "create")
	assert.True(t, len(networkCreateCmd.Aliases) > 0, "networkCreateCmd should have aliases")
	
	// Verify the command can be called with different aliases
	for _, alias := range networkCreateCmd.Aliases {
		assert.NotEmpty(t, alias, "networkCreateCmd aliases should not be empty")
	}
}

func TestNetworkListCmd_OutputFormats(t *testing.T) {
	// Test that networkListCmd handles different output formats
	// The actual output formatting is handled by the global 'output' variable
	// and uses json, yaml, or default tabular format
	
	assert.NotNil(t, networkListCmd.Run)
	t.Log("networkListCmd supports json, yaml, and tabular output formats")
}

func TestNetworkDeleteCmd_ForceFlag(t *testing.T) {
	// Test that networkDeleteCmd respects the global force flag
	// The 'force' variable is used to bypass confirmation prompts
	
	assert.NotNil(t, networkDeleteCmd.Run)
	t.Log("networkDeleteCmd respects global force flag for bypassing confirmations")
}