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
		println(test.line)
		key, value, err := ParseEnvLine(test.line)

		fmt.Printf("Expected key: %s (len: %d, hex: %x)\n", test.expectedKey, len(test.expectedKey), test.expectedKey)
		fmt.Printf("Returned key: %s (len: %d, hex: %x)\n", key, len(key), key)

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
}
