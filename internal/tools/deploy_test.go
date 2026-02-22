package tools

import (
	"encoding/json"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestStringSliceToPointerSlice_Empty(t *testing.T) {
	result := StringSliceToPointerSlice([]string{})
	assert.NotNil(t, result)
	assert.Len(t, result, 0)
}

func TestStringSliceToPointerSlice_SingleElement(t *testing.T) {
	result := StringSliceToPointerSlice([]string{"hello"})
	assert.Len(t, result, 1)
	assert.Equal(t, "hello", *result[0])
}

func TestStringSliceToPointerSlice_MultipleElements(t *testing.T) {
	input := []string{"a", "b", "c"}
	result := StringSliceToPointerSlice(input)
	assert.Len(t, result, 3)
	assert.Equal(t, "a", *result[0])
	assert.Equal(t, "b", *result[1])
	assert.Equal(t, "c", *result[2])
}

func TestCreateDeployOutputFile(t *testing.T) {
	// Change to temp dir to avoid polluting project
	tmpDir, err := os.MkdirTemp("", "deploy-test-*")
	assert.NoError(t, err)
	defer func() { _ = os.RemoveAll(tmpDir) }()

	origDir, err := os.Getwd()
	assert.NoError(t, err)
	defer func() { _ = os.Chdir(origDir) }()

	err = os.Chdir(tmpDir)
	assert.NoError(t, err)

	output := &DeployOutput{
		Username:   "testuser",
		PublicURL:  "https://example.com",
		PublicIP:   "1.2.3.4",
		DockerHost: "tcp://1.2.3.4:2375",
	}

	CreateDeployOutputFile(output)

	data, err := os.ReadFile("onctl-deploy.json")
	assert.NoError(t, err)

	var parsed DeployOutput
	err = json.Unmarshal(data, &parsed)
	assert.NoError(t, err)
	assert.Equal(t, "testuser", parsed.Username)
	assert.Equal(t, "https://example.com", parsed.PublicURL)
	assert.Equal(t, "1.2.3.4", parsed.PublicIP)
	assert.Equal(t, "tcp://1.2.3.4:2375", parsed.DockerHost)
}
