package tools

import (
	"fmt"
	"os"
	"testing"
)

func Test_ParseEnvLine(t *testing.T) {
	tests := []struct {
		line          string
		expectedKey   string
		expectedValue string
		expectedError error
	}{
		{
			line:          "KEY=VALUE",
			expectedKey:   "KEY",
			expectedValue: "VALUE",
			expectedError: nil,
		},
		{
			line:          "KEY='VALUE'",
			expectedKey:   "KEY",
			expectedValue: "VALUE",
			expectedError: nil,
		},
		{
			line:          `KEY="VALUE"`,
			expectedKey:   "KEY",
			expectedValue: "VALUE",
			expectedError: nil,
		},
		{
			line:          "KEY=VALUE=123",
			expectedKey:   "",
			expectedValue: "",
			expectedError: fmt.Errorf("invalid line: KEY=VALUE=123"),
		},
		{
			line:          "KEY",
			expectedKey:   "",
			expectedValue: "",
			expectedError: fmt.Errorf("invalid line: KEY"),
		},
	}

	for _, test := range tests {
		key, value, err := ParseEnvLine(test.line)

		if key != test.expectedKey {
			t.Errorf("Expected key: %s, got: %s", test.expectedKey, key)
		}
		if value != test.expectedValue {
			t.Errorf("Expected value: %s, got: %s", test.expectedValue, value)
		}
		if err != nil && test.expectedError == nil {
			t.Errorf("Unexpected error: %v", err)
		}
		if err == nil && test.expectedError != nil {
			t.Errorf("Expected error: %v, got nil", test.expectedError)
		}
		if err != nil && test.expectedError != nil && err.Error() != test.expectedError.Error() {
			t.Errorf("Expected error: %v, got: %v", test.expectedError, err)
		}
	}
}

func Test_ParseDotEnvFile(t *testing.T) {
	// Test with a valid .env file
	validDotEnvContent := `
# This is a comment
KEY1=VALUE1
KEY2='VALUE2'
KEY3="VALUE3"
KEY4=VALUE4 # This is a comment
`

	tmpFile, err := os.CreateTemp("", "valid_test.env")
	if err != nil {
		t.Fatalf("Failed to create temp file: %v", err)
	}
	defer os.Remove(tmpFile.Name())

	if _, err := tmpFile.Write([]byte(validDotEnvContent)); err != nil {
		t.Fatalf("Failed to write to temp file: %v", err)
	}
	if err := tmpFile.Close(); err != nil {
		t.Fatalf("Failed to close temp file: %v", err)
	}

	vars, err := ParseDotEnvFile(tmpFile.Name())
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	expectedVars := []string{
		"KEY1=VALUE1",
		"KEY2=VALUE2",
		"KEY3=VALUE3",
	}

	for i, expected := range expectedVars {
		if vars[i] != expected {
			t.Errorf("Expected %s, got %s", expected, vars[i])
		}
	}

	// Test with an invalid .env file
	invalidDotEnvContent := `
KEY1=VALUE1
KEY4=VALUE=123
`

	tmpFile, err = os.CreateTemp("", "invalid_test.env")
	if err != nil {
		t.Fatalf("Failed to create temp file: %v", err)
	}
	defer os.Remove(tmpFile.Name())

	if _, err := tmpFile.Write([]byte(invalidDotEnvContent)); err != nil {
		t.Fatalf("Failed to write to temp file: %v", err)
	}
	if err := tmpFile.Close(); err != nil {
		t.Fatalf("Failed to close temp file: %v", err)
	}

	_, err = ParseDotEnvFile(tmpFile.Name())
	if err == nil {
		t.Fatalf("Expected error for invalid line, got nil")
	}

	// Test with a non-existing file
	_, err = ParseDotEnvFile("non_existing_file.txt")
	if err == nil {
		t.Fatalf("Expected error for non-existing file, got nil")
	}
}
func Test_exists(t *testing.T) {
	// Test with an existing file
	existingFile := "existing_file.txt"
	_, err := os.Create(existingFile)
	if err != nil {
		t.Fatalf("Failed to create existing file: %v", err)
	}
	defer os.Remove(existingFile)

	ex, err := exists(existingFile)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	if !ex {
		t.Errorf("Expected file to exist, but it doesn't")
	}

	// Test with a non-existing file
	ex, err = exists("non_existing_file.txt")
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	if ex {
		t.Errorf("Expected file to not exist, but it does")
	}

	// Test with a directory
	existingDir := "existing_dir"
	err = os.Mkdir(existingDir, 0755)
	if err != nil {
		t.Fatalf("Failed to create existing directory: %v", err)
	}
	defer os.Remove(existingDir)

	ex, err = exists(existingDir)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	if !ex {
		t.Errorf("Expected directory to exist, but it doesn't")
	}
}
func Test_variablesToEnvVars(t *testing.T) {
	tests := []struct {
		vars     []string
		expected string
	}{
		{
			vars:     []string{"KEY1=VALUE1", "KEY2=VALUE2"},
			expected: "KEY1=\"VALUE1\" KEY2=\"VALUE2\" ",
		},
		{
			vars:     []string{"KEY1=VALUE1", "KEY2='VALUE2'"},
			expected: "KEY1=\"VALUE1\" KEY2=\"'VALUE2'\" ",
		},
		{
			vars:     []string{"KEY1=VALUE1", "KEY2=VALUE2", "KEY3=VALUE3"},
			expected: "KEY1=\"VALUE1\" KEY2=\"VALUE2\" KEY3=\"VALUE3\" ",
		},
		{
			vars:     []string{},
			expected: "",
		},
	}

	for _, test := range tests {
		result := variablesToEnvVars(test.vars)
		if result != test.expected {
			t.Errorf("Expected: %s, got: %s", test.expected, result)
		}
	}
}
