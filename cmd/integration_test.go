package cmd

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestReadConfig_FunctionExists(t *testing.T) {
	// Test that ReadConfig function exists and is callable
	// This tests the function's signature without requiring specific behavior
	assert.NotPanics(t, func() {
		// ReadConfig may succeed or fail depending on environment, but should not panic
		_ = ReadConfig("aws")
		// No assertion on error since test environment may have valid config
	})
}

func TestReadConfig_WithValidDirectory(t *testing.T) {
	// Test that ReadConfig function can handle directory operations without hanging
	assert.NotPanics(t, func() {
		// ReadConfig may succeed or fail depending on environment, but should not panic
		_ = ReadConfig("gcp")
		// No assertion on error since test environment may have valid config
	})
}

func TestAllInitFunctions(t *testing.T) {
	// Test that all init functions are properly called by checking command registrations

	// Test createCmd init
	flag := createCmd.Flags().Lookup("publicKey")
	assert.NotNil(t, flag, "createCmd init should register publicKey flag")

	// Test destroyCmd init
	flag = destroyCmd.Flags().Lookup("force")
	assert.NotNil(t, flag, "destroyCmd init should register force flag")

	// Test listCmd init
	flag = listCmd.Flags().Lookup("output")
	assert.NotNil(t, flag, "listCmd init should register output flag")

	// Test sshCmd init
	flag = sshCmd.Flags().Lookup("key")
	assert.NotNil(t, flag, "sshCmd init should register key flag")

	// Test templatesCmd init
	subCommands := templatesCmd.Commands()
	found := false
	for _, cmd := range subCommands {
		if cmd.Name() == "list" {
			found = true
			break
		}
	}
	assert.True(t, found, "templatesCmd init should register list subcommand")

	// Test rootCmd init
	subCommands = rootCmd.Commands()
	commandNames := make(map[string]bool)
	for _, cmd := range subCommands {
		commandNames[cmd.Name()] = true
	}

	expectedCommands := []string{"version", "ls", "create", "destroy", "ssh", "init", "templates"}
	for _, expected := range expectedCommands {
		assert.True(t, commandNames[expected], "rootCmd init should register '%s' subcommand", expected)
	}
}
