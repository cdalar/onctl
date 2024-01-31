package tools

import (
	"testing"
)

func Test_variablesToEnvVars(t *testing.T) {
	tests := []struct {
		name     string
		vars     []string
		expected string
	}{
		{
			name:     "Empty vars",
			vars:     []string{},
			expected: "",
		},
		{
			name:     "Single var",
			vars:     []string{"KEY=value"},
			expected: "KEY=value ",
		},
		{
			name:     "Multiple vars",
			vars:     []string{"KEY1=value1", "KEY2=value2", "KEY3=value3"},
			expected: "KEY1=value1 KEY2=value2 KEY3=value3 ",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := variablesToEnvVars(tt.vars)
			if got != tt.expected {
				t.Errorf("variablesToEnvVars() = %s, want %s", got, tt.expected)
			}
		})
	}
}
func Test_exists(t *testing.T) {
	tests := []struct {
		name     string
		path     string
		expected bool
	}{
		{
			name:     "Existing path",
			path:     "/tmp",
			expected: true,
		},
		{
			name:     "Non-existing path",
			path:     "/path/to/non-existing/file",
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := exists(tt.path)
			if err != nil {
				t.Errorf("exists() returned an error: %v", err)
			}
			if got != tt.expected {
				t.Errorf("exists() = %v, want %v", got, tt.expected)
			}
		})
	}
}
