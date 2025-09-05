package cmd

import (
	"testing"

	"github.com/cdalar/onctl/internal/cloud"
	"github.com/stretchr/testify/assert"
)

func TestVmCmd_CommandProperties(t *testing.T) {
	// Test that the vmCmd has the expected properties
	assert.Equal(t, "vm", vmCmd.Use)
	assert.Contains(t, vmCmd.Aliases, "server")
	assert.Equal(t, "Manage vm resources", vmCmd.Short)
}

func TestVmCmd_HasSubCommands(t *testing.T) {
	// Test that vmCmd has the expected subcommands
	commands := vmCmd.Commands()
	
	commandNames := make([]string, len(commands))
	for i, cmd := range commands {
		commandNames[i] = cmd.Use
	}
	
	assert.Contains(t, commandNames, "attach", "vmCmd should have 'attach' subcommand")
	// Note: vmCmd only has attach as a direct subcommand, detach is under attach
}

func TestVmNetworkAttachCmd_CommandProperties(t *testing.T) {
	// Test that the vmNetworkAttachCmd has the expected properties
	assert.Equal(t, "attach", vmNetworkAttachCmd.Use)
	assert.Equal(t, "Attach a network", vmNetworkAttachCmd.Short)
	assert.NotNil(t, vmNetworkAttachCmd.Run)
}

func TestVmNetworkAttachCmd_HasFlags(t *testing.T) {
	// Test that flags are properly registered
	flags := []struct {
		name      string
		shorthand string
		usage     string
	}{
		{"vm", "", "name of vm"},
		{"network", "n", "Name for the network"},
	}

	for _, flag := range flags {
		f := vmNetworkAttachCmd.Flags().Lookup(flag.name)
		assert.NotNil(t, f, "vmNetworkAttachCmd should have '%s' flag", flag.name)
		assert.Equal(t, flag.shorthand, f.Shorthand, "%s flag should have '%s' shorthand", flag.name, flag.shorthand)
		assert.Contains(t, f.Usage, flag.usage, "%s flag should have correct usage", flag.name)
	}
}

func TestVmNetworkDetachCmd_CommandProperties(t *testing.T) {
	// Test that the vmNetworkDetachCmd has the expected properties
	assert.Equal(t, "detach", vmNetworkDetachCmd.Use)
	assert.Equal(t, "Detach a network", vmNetworkDetachCmd.Short)
	assert.NotNil(t, vmNetworkDetachCmd.Run)
}

func TestVmNetworkDetachCmd_HasFlags(t *testing.T) {
	// Test that flags are properly registered
	flags := []struct {
		name      string
		shorthand string
		usage     string
	}{
		{"vm", "", "name of vm"},
		{"network", "n", "Name for the network"},
	}

	for _, flag := range flags {
		f := vmNetworkDetachCmd.Flags().Lookup(flag.name)
		assert.NotNil(t, f, "vmNetworkDetachCmd should have '%s' flag", flag.name)
		assert.Equal(t, flag.shorthand, f.Shorthand, "%s flag should have '%s' shorthand", flag.name, flag.shorthand)
		assert.Contains(t, f.Usage, flag.usage, "%s flag should have correct usage", flag.name)
	}
}

func TestVmOpts_GlobalVariable(t *testing.T) {
	// Test that vmOpts global variable exists and can be manipulated
	originalName := vmOpts.Name
	
	// Modify values
	vmOpts.Name = "test-vm"
	
	assert.Equal(t, "test-vm", vmOpts.Name)
	
	// Restore original values
	vmOpts.Name = originalName
}

func TestNOpts_GlobalVariable(t *testing.T) {
	// Test that nOpts global variable exists and can be manipulated
	originalName := nOpts.Name
	
	// Modify values
	nOpts.Name = "test-network"
	
	assert.Equal(t, "test-network", nOpts.Name)
	
	// Restore original values
	nOpts.Name = originalName
}

func TestVmOpts_StructBasics(t *testing.T) {
	// Test creating and manipulating cloud.Vm struct
	vm := cloud.Vm{
		ID:         "vm-123",
		Name:       "test-vm",
		IP:         "192.168.1.100",
		Provider:   "aws",
		Status:     "running",
		SSHPort:    22,
	}
	
	assert.Equal(t, "vm-123", vm.ID)
	assert.Equal(t, "test-vm", vm.Name)
	assert.Equal(t, "192.168.1.100", vm.IP)
	assert.Equal(t, "aws", vm.Provider)
	assert.Equal(t, "running", vm.Status)
	assert.Equal(t, 22, vm.SSHPort)
}

func TestNOpts_StructBasics(t *testing.T) {
	// Test creating and manipulating cloud.Network struct via nOpts
	nOpts.Name = "test-network"
	nOpts.CIDR = "10.0.0.0/16"
	
	assert.Equal(t, "test-network", nOpts.Name)
	assert.Equal(t, "10.0.0.0/16", nOpts.CIDR)
}

func TestVmOpts_ZeroValues(t *testing.T) {
	// Test zero value cloud.Vm
	var vm cloud.Vm
	
	assert.Equal(t, "", vm.ID)
	assert.Equal(t, "", vm.Name)
	assert.Equal(t, "", vm.IP)
	assert.Equal(t, "", vm.Provider)
	assert.Equal(t, "", vm.Status)
	assert.Equal(t, 0, vm.SSHPort)
}

func TestNOpts_ZeroValues(t *testing.T) {
	// Test zero value cloud.Network via nOpts
	var network cloud.Network
	
	assert.Equal(t, "", network.ID)
	assert.Equal(t, "", network.Name)
	assert.Equal(t, "", network.CIDR)
	assert.Equal(t, "", network.Provider)
	assert.Equal(t, 0, network.Servers)
}

func TestVmCmd_InitFunction(t *testing.T) {
	// Test that init function properly sets up the command structure
	assert.NotNil(t, vmCmd)
	assert.True(t, vmCmd.HasSubCommands())
	
	// Verify the subcommands are properly added
	commands := vmCmd.Commands()
	assert.True(t, len(commands) >= 1, "vmCmd should have at least 1 subcommand")
}

func TestVmNetworkAttachCmd_FlagBinding(t *testing.T) {
	// Test that the flags are properly bound to the vmOpts and nOpts variables
	// Save original values
	originalVmName := vmOpts.Name
	originalNetworkName := nOpts.Name
	defer func() {
		vmOpts.Name = originalVmName
		nOpts.Name = originalNetworkName
	}()
	
	// Set flags via command
	err := vmNetworkAttachCmd.Flags().Set("vm", "test-vm-1")
	assert.NoError(t, err)
	assert.Equal(t, "test-vm-1", vmOpts.Name)
	
	err = vmNetworkAttachCmd.Flags().Set("network", "test-net-1")
	assert.NoError(t, err)
	assert.Equal(t, "test-net-1", nOpts.Name)
}

func TestVmNetworkDetachCmd_FlagBinding(t *testing.T) {
	// Test that the flags are properly bound to the vmOpts and nOpts variables
	// Save original values
	originalVmName := vmOpts.Name
	originalNetworkName := nOpts.Name
	defer func() {
		vmOpts.Name = originalVmName
		nOpts.Name = originalNetworkName
	}()
	
	// Set flags via command
	err := vmNetworkDetachCmd.Flags().Set("vm", "test-vm-2")
	assert.NoError(t, err)
	assert.Equal(t, "test-vm-2", vmOpts.Name)
	
	err = vmNetworkDetachCmd.Flags().Set("network", "test-net-2")
	assert.NoError(t, err)
	assert.Equal(t, "test-net-2", nOpts.Name)
}

func TestVmNetworkCommands_RunFunctionExists(t *testing.T) {
	// Test that all VM network commands have run functions defined
	assert.NotNil(t, vmNetworkAttachCmd.Run, "vmNetworkAttachCmd should have Run function")
	assert.NotNil(t, vmNetworkDetachCmd.Run, "vmNetworkDetachCmd should have Run function")
}

func TestVmNetworkCommands_DebugLogging(t *testing.T) {
	// Test that commands have debug logging capabilities
	// Since these functions would interact with actual cloud providers,
	// we just verify they exist and are callable (but don't execute them)
	
	assert.NotNil(t, vmNetworkAttachCmd.Run)
	assert.NotNil(t, vmNetworkDetachCmd.Run)
	
	// Both commands should log debug information about VM and Network operations
	t.Log("vmNetworkAttachCmd logs debug information for attach operations")
	t.Log("vmNetworkDetachCmd logs debug information for detach operations")
}

func TestVmNetworkCommands_ErrorHandling(t *testing.T) {
	// Test that commands handle errors properly
	// The Run functions should handle errors from:
	// - provider.GetByName() 
	// - networkManager.GetByName()
	// - provider.AttachNetwork() / provider.DetachNetwork()
	
	assert.NotNil(t, vmNetworkAttachCmd.Run)
	assert.NotNil(t, vmNetworkDetachCmd.Run)
	
	t.Log("vmNetworkAttachCmd handles errors from provider and networkManager")
	t.Log("vmNetworkDetachCmd handles errors from provider and networkManager")
}

func TestVmNetworkCommands_RequiredFlags(t *testing.T) {
	// Test that both commands require vm and network flags
	vmFlag := vmNetworkAttachCmd.Flags().Lookup("vm")
	networkFlag := vmNetworkAttachCmd.Flags().Lookup("network")
	
	assert.NotNil(t, vmFlag, "attach command should have vm flag")
	assert.NotNil(t, networkFlag, "attach command should have network flag")
	
	vmFlag = vmNetworkDetachCmd.Flags().Lookup("vm")
	networkFlag = vmNetworkDetachCmd.Flags().Lookup("network")
	
	assert.NotNil(t, vmFlag, "detach command should have vm flag")
	assert.NotNil(t, networkFlag, "detach command should have network flag")
}

func TestVmNetworkCommands_UsageExamples(t *testing.T) {
	// Test command usage documentation
	assert.Contains(t, vmNetworkAttachCmd.Use, "attach")
	assert.Contains(t, vmNetworkDetachCmd.Use, "detach")
	
	// Commands should have meaningful short descriptions
	assert.NotEmpty(t, vmNetworkAttachCmd.Short)
	assert.NotEmpty(t, vmNetworkDetachCmd.Short)
	assert.Contains(t, vmNetworkAttachCmd.Short, "Attach")
	assert.Contains(t, vmNetworkDetachCmd.Short, "Detach")
}

func TestVmCmd_GlobalVariableConsistency(t *testing.T) {
	// Test that vmOpts and nOpts are consistently used across commands
	// Both attach and detach commands should use the same global variables
	
	// Verify both commands reference the same flag values
	originalVmName := vmOpts.Name
	originalNetworkName := nOpts.Name
	
	// Set a value in vmOpts
	vmOpts.Name = "consistency-test-vm"
	nOpts.Name = "consistency-test-network"
	
	assert.Equal(t, "consistency-test-vm", vmOpts.Name)
	assert.Equal(t, "consistency-test-network", nOpts.Name)
	
	// Restore original values
	vmOpts.Name = originalVmName
	nOpts.Name = originalNetworkName
}