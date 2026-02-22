package tools

import (
	"encoding/base64"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestFileToBase64_EmptyPath(t *testing.T) {
	result := FileToBase64("")
	assert.Equal(t, "", result)
}

func TestFileToBase64_NonExistentFile(t *testing.T) {
	result := FileToBase64("/nonexistent/path/file.txt")
	assert.Equal(t, "", result)
}

func TestFileToBase64_ValidFile(t *testing.T) {
	tmpFile, err := os.CreateTemp("", "cloud-init-test-*.txt")
	assert.NoError(t, err)
	defer func() { _ = os.Remove(tmpFile.Name()) }()

	content := []byte("#!/bin/bash\necho hello")
	_, err = tmpFile.Write(content)
	assert.NoError(t, err)
	_ = tmpFile.Close()

	result := FileToBase64(tmpFile.Name())
	assert.NotEmpty(t, result)

	decoded, err := base64.StdEncoding.DecodeString(result)
	assert.NoError(t, err)
	assert.Equal(t, content, decoded)
}

func TestFileToBase64_EmptyFile(t *testing.T) {
	tmpFile, err := os.CreateTemp("", "cloud-init-empty-*.txt")
	assert.NoError(t, err)
	defer func() { _ = os.Remove(tmpFile.Name()) }()
	_ = tmpFile.Close()

	result := FileToBase64(tmpFile.Name())
	assert.Equal(t, "", result)
}
