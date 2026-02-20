package cmd

import (
	"testing"
)

func TestEnvCmdExists(t *testing.T) {
	if envCmd == nil {
		t.Fatal("envCmd is nil")
	}

	if envCmd.Use != "env" {
		t.Errorf("envCmd.Use = %q, want %q", envCmd.Use, "env")
	}

	if envCmd.Short == "" {
		t.Error("envCmd.Short is empty")
	}

	if envCmd.Long == "" {
		t.Error("envCmd.Long is empty")
	}
}

func TestEnvCreateCmdExists(t *testing.T) {
	if envCreateCmd == nil {
		t.Fatal("envCreateCmd is nil")
	}

	if envCreateCmd.Use != "create" {
		t.Errorf("envCreateCmd.Use = %q, want %q", envCreateCmd.Use, "create")
	}

	// Check aliases
	expectedAliases := []string{"start", "up"}
	if len(envCreateCmd.Aliases) != len(expectedAliases) {
		t.Errorf("envCreateCmd.Aliases length = %d, want %d", len(envCreateCmd.Aliases), len(expectedAliases))
	}

	for i, alias := range expectedAliases {
		if i < len(envCreateCmd.Aliases) && envCreateCmd.Aliases[i] != alias {
			t.Errorf("envCreateCmd.Aliases[%d] = %q, want %q", i, envCreateCmd.Aliases[i], alias)
		}
	}

	// Check that template flag exists
	templateFlag := envCreateCmd.Flags().Lookup("template")
	if templateFlag == nil {
		t.Error("template flag not found on envCreateCmd")
	}
}

func TestEnvDestroyCmdExists(t *testing.T) {
	if envDestroyCmd == nil {
		t.Fatal("envDestroyCmd is nil")
	}

	if envDestroyCmd.Use != "destroy" {
		t.Errorf("envDestroyCmd.Use = %q, want %q", envDestroyCmd.Use, "destroy")
	}

	// Check aliases
	expectedAliases := []string{"down", "delete", "remove", "rm"}
	if len(envDestroyCmd.Aliases) != len(expectedAliases) {
		t.Errorf("envDestroyCmd.Aliases length = %d, want %d", len(envDestroyCmd.Aliases), len(expectedAliases))
	}

	for i, alias := range expectedAliases {
		if i < len(envDestroyCmd.Aliases) && envDestroyCmd.Aliases[i] != alias {
			t.Errorf("envDestroyCmd.Aliases[%d] = %q, want %q", i, envDestroyCmd.Aliases[i], alias)
		}
	}

	// Check that template flag exists
	templateFlag := envDestroyCmd.Flags().Lookup("template")
	if templateFlag == nil {
		t.Error("template flag not found on envDestroyCmd")
	}

	// Check that force flag exists
	forceFlag := envDestroyCmd.Flags().Lookup("force")
	if forceFlag == nil {
		t.Error("force flag not found on envDestroyCmd")
	}
}

func TestEnvCmdStructure(t *testing.T) {
	// Verify that envCreateCmd and envDestroyCmd are subcommands of envCmd
	hasCreateCmd := false
	hasDestroyCmd := false

	for _, cmd := range envCmd.Commands() {
		if cmd.Name() == "create" {
			hasCreateCmd = true
		}
		if cmd.Name() == "destroy" {
			hasDestroyCmd = true
		}
	}

	if !hasCreateCmd {
		t.Error("envCmd does not have create subcommand")
	}

	if !hasDestroyCmd {
		t.Error("envCmd does not have destroy subcommand")
	}
}

func TestEnvCreateTemplateFlag(t *testing.T) {
	templateFlag := envCreateCmd.Flags().Lookup("template")
	if templateFlag == nil {
		t.Fatal("template flag not found on envCreateCmd")
	}

	if templateFlag.Shorthand != "t" {
		t.Errorf("template flag shorthand = %q, want %q", templateFlag.Shorthand, "t")
	}
}

func TestEnvDestroyTemplateFlag(t *testing.T) {
	templateFlag := envDestroyCmd.Flags().Lookup("template")
	if templateFlag == nil {
		t.Fatal("template flag not found on envDestroyCmd")
	}

	if templateFlag.Shorthand != "t" {
		t.Errorf("template flag shorthand = %q, want %q", templateFlag.Shorthand, "t")
	}

	forceFlag := envDestroyCmd.Flags().Lookup("force")
	if forceFlag == nil {
		t.Fatal("force flag not found on envDestroyCmd")
	}

	if forceFlag.Shorthand != "f" {
		t.Errorf("force flag shorthand = %q, want %q", forceFlag.Shorthand, "f")
	}
}

func TestEnvDestroyForceFlag(t *testing.T) {
	// Test that force flag is a boolean
	forceFlag := envDestroyCmd.Flags().Lookup("force")
	if forceFlag == nil {
		t.Fatal("force flag not found")
	}

	if forceFlag.Value.Type() != "bool" {
		t.Errorf("force flag type = %q, want %q", forceFlag.Value.Type(), "bool")
	}
}
