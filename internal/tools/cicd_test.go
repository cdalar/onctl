package tools

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestGenerateMachineUniqueName(t *testing.T) {
	name := GenerateMachineUniqueName()
	assert.NotEmpty(t, name)
	assert.True(t, len(name) > len("onctl-"), "name should be longer than prefix")
	assert.Contains(t, name, "onctl-")
}

func TestGenerateMachineUniqueName_Unique(t *testing.T) {
	names := make(map[string]bool)
	for i := 0; i < 10; i++ {
		name := GenerateMachineUniqueName()
		names[name] = true
	}
	// With 5-char random suffix, collisions in 10 tries are very unlikely
	assert.Greater(t, len(names), 1)
}

func TestGenerateUserName(t *testing.T) {
	name := GenerateUserName()
	assert.NotEmpty(t, name)
	// Should not contain backslashes, spaces, forward slashes, or dots
	assert.NotContains(t, name, "\\")
	assert.NotContains(t, name, " ")
	assert.NotContains(t, name, "/")
	assert.NotContains(t, name, ".")
}
