package tools

import (
	"testing"
)

func TestVariablesToEnvVars(t *testing.T) {
	tests := []struct {
		name     string
		vars     []string
		expected string
	}{
		{
			name:     "Empty input",
			vars:     []string{},
			expected: "",
		},
		{
			name:     "Single variable",
			vars:     []string{"KEY=value"},
			expected: "KEY=\"value\" ",
		},
		{
			name:     "Multiple variables",
			vars:     []string{"KEY1=value1", "KEY2=value2"},
			expected: "KEY1=\"value1\" KEY2=\"value2\" ",
		},
		{
			name:     "Variable with spaces",
			vars:     []string{"KEY=value with spaces"},
			expected: "KEY=\"value with spaces\" ",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := variablesToEnvVars(tt.vars)
			if result != tt.expected {
				t.Errorf("expected %q, got %q", tt.expected, result)
			}
		})
	}
}
