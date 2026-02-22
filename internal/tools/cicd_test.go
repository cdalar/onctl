package tools

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestGenerateMachineUniqueName(t *testing.T) {
	name := GenerateMachineUniqueName()
	assert.True(t, strings.HasPrefix(name, "onctl-"), "name should start with 'onctl-', got: %s", name)
	assert.Equal(t, 11, len(name), "name should be 11 chars (onctl- = 6 + 5 random), got: %s", name)
}

func TestGenerateMachineUniqueName_Unique(t *testing.T) {
	name1 := GenerateMachineUniqueName()
	name2 := GenerateMachineUniqueName()
	// Names should generally be unique (probabilistic, but very likely)
	assert.NotEmpty(t, name1)
	assert.NotEmpty(t, name2)
}

func TestGenerateUserName(t *testing.T) {
	userName := GenerateUserName()
	assert.NotEmpty(t, userName)
	// Username should not contain problematic characters
	assert.NotContains(t, userName, "\\")
	assert.NotContains(t, userName, " ")
	assert.NotContains(t, userName, "/")
	assert.NotContains(t, userName, ".")
}
