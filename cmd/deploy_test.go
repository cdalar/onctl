package cmd

import (
	"testing"
)

func TestNormalizeArch(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{"x86_64", "x86_64", "amd64"},
		{"amd64", "amd64", "amd64"},
		{"aarch64", "aarch64", "arm64"},
		{"arm64", "arm64", "arm64"},
		{"armv7l", "armv7l", "arm"},
		{"arm", "arm", "arm"},
		{"i386", "i386", "386"},
		{"i686", "i686", "386"},
		{"386", "386", "386"},
		{"unknown", "unknown", "unknown"},
		{"uppercase", "X86_64", "amd64"},
		{"with spaces", " amd64 ", "amd64"},
		{"mixed case", "AArch64", "arm64"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := normalizeArch(tt.input)
			if result != tt.expected {
				t.Errorf("normalizeArch(%q) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}

func TestCheckDockerHubImage(t *testing.T) {
	tests := []struct {
		name          string
		image         string
		shouldSucceed bool
	}{
		{
			name:          "invalid image",
			image:         "this-image-definitely-does-not-exist-12345678",
			shouldSucceed: false,
		},
		{
			name:          "invalid characters",
			image:         "invalid@#$%^&*()",
			shouldSucceed: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := checkDockerHubImage(tt.image)
			if result != tt.shouldSucceed {
				t.Errorf("checkDockerHubImage(%q) = %v, want %v", tt.image, result, tt.shouldSucceed)
			}
		})
	}
}

func TestDeployCmdDeployOptions(t *testing.T) {
	// Test that deploy options are properly initialized
	opt := cmdDeployOptions{
		Image: "test-image:latest",
		Env:   []string{"ENV1=value1", "ENV2=value2"},
		Name:  "test-container",
	}

	if opt.Image != "test-image:latest" {
		t.Errorf("Image = %q, want %q", opt.Image, "test-image:latest")
	}

	if len(opt.Env) != 2 {
		t.Errorf("Env length = %d, want 2", len(opt.Env))
	}

	if opt.Name != "test-container" {
		t.Errorf("Name = %q, want %q", opt.Name, "test-container")
	}
}

func TestDeployCmdExists(t *testing.T) {
	if deployCmd == nil {
		t.Fatal("deployCmd is nil")
	}

	if deployCmd.Use != "deploy VM_NAME" {
		t.Errorf("deployCmd.Use = %q, want %q", deployCmd.Use, "deploy VM_NAME")
	}

	if deployCmd.Short == "" {
		t.Error("deployCmd.Short is empty")
	}

	// Check that required flags exist
	imageFlag := deployCmd.Flags().Lookup("image")
	if imageFlag == nil {
		t.Error("image flag not found")
	}

	envFlag := deployCmd.Flags().Lookup("env")
	if envFlag == nil {
		t.Error("env flag not found")
	}

	nameFlag := deployCmd.Flags().Lookup("name")
	if nameFlag == nil {
		t.Error("name flag not found")
	}
}

func TestDeployCmdMinimumArgs(t *testing.T) {
	if deployCmd.Args == nil {
		t.Fatal("deployCmd.Args is nil")
	}

	// Test that at least 1 argument is required
	err := deployCmd.Args(deployCmd, []string{})
	if err == nil {
		t.Error("Expected error when no VM name provided, got nil")
	}

	// Test that 1 argument is accepted
	err = deployCmd.Args(deployCmd, []string{"test-vm"})
	if err != nil {
		t.Errorf("Expected no error with 1 argument, got %v", err)
	}
}

func TestDeployCmdAliases(t *testing.T) {
	// Verify deploy command has proper structure
	if deployCmd.TraverseChildren != true {
		t.Error("deployCmd.TraverseChildren should be true")
	}

	if deployCmd.DisableFlagsInUseLine != true {
		t.Error("deployCmd.DisableFlagsInUseLine should be true")
	}
}
