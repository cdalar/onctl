package tools

import (
	"encoding/base64"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFileToBase64_Empty(t *testing.T) {
	result := FileToBase64("")
	assert.Equal(t, "", result)
}

func TestFileToBase64_NonExistent(t *testing.T) {
	result := FileToBase64("/nonexistent/path/to/file.txt")
	assert.Equal(t, "", result)
}

func TestFileToBase64_ValidFile(t *testing.T) {
	tmpFile, err := os.CreateTemp("", "base64-test-")
	require.NoError(t, err)
	defer os.Remove(tmpFile.Name())

	content := []byte("hello world")
	_, err = tmpFile.Write(content)
	require.NoError(t, err)
	tmpFile.Close()

	result := FileToBase64(tmpFile.Name())
	expected := base64.StdEncoding.EncodeToString(content)
	assert.Equal(t, expected, result)
}

func TestFileToBase64_EmptyFile(t *testing.T) {
	tmpFile, err := os.CreateTemp("", "base64-empty-")
	require.NoError(t, err)
	defer os.Remove(tmpFile.Name())
	tmpFile.Close()

	result := FileToBase64(tmpFile.Name())
	assert.Equal(t, base64.StdEncoding.EncodeToString([]byte{}), result)
}
