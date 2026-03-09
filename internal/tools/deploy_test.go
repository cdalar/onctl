package tools

import (
	"encoding/json"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestStringSliceToPointerSlice(t *testing.T) {
	tests := []struct {
		name  string
		input []string
	}{
		{"empty", []string{}},
		{"single", []string{"a"}},
		{"multiple", []string{"a", "b", "c"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := StringSliceToPointerSlice(tt.input)
			assert.Len(t, result, len(tt.input))
			for i, ptr := range result {
				require.NotNil(t, ptr)
				assert.Equal(t, tt.input[i], *ptr)
			}
		})
	}
}

func TestCreateDeployOutputFile(t *testing.T) {
	// Change to a temp dir so file is created there
	tmpDir := t.TempDir()
	origDir, err := os.Getwd()
	require.NoError(t, err)
	require.NoError(t, os.Chdir(tmpDir))
	defer func() { _ = os.Chdir(origDir) }()

	output := &DeployOutput{
		Username:   "testuser",
		PublicURL:  "https://example.com",
		PublicIP:   "1.2.3.4",
		DockerHost: "tcp://1.2.3.4:2375",
	}
	CreateDeployOutputFile(output)

	data, err := os.ReadFile("onctl-deploy.json")
	require.NoError(t, err)

	var parsed DeployOutput
	err = json.Unmarshal(data, &parsed)
	require.NoError(t, err)
	assert.Equal(t, "testuser", parsed.Username)
	assert.Equal(t, "https://example.com", parsed.PublicURL)
	assert.Equal(t, "1.2.3.4", parsed.PublicIP)
	assert.Equal(t, "tcp://1.2.3.4:2375", parsed.DockerHost)
}
