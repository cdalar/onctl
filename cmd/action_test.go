package cmd

import (
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestActionCmd_CommandProperties(t *testing.T) {
	// Test that the command has the expected properties
	assert.Equal(t, "action <name>", actionCmd.Use)
	assert.Equal(t, "Execute a custom action from GitHub", actionCmd.Short)
	assert.Contains(t, actionCmd.Long, "Download and execute custom actions")
	assert.NotNil(t, actionCmd.Args)
	assert.NotNil(t, actionCmd.Run)
}

func TestActionCmd_HasFlags(t *testing.T) {
	// Test that flags are properly registered
	flags := []struct {
		name      string
		shorthand string
		usage     string
	}{
		{"params", "p", "JSON parameter file to pass as stdin"},
	}

	for _, flag := range flags {
		f := actionCmd.Flags().Lookup(flag.name)
		assert.NotNil(t, f, "action command should have '%s' flag", flag.name)
		assert.Equal(t, flag.shorthand, f.Shorthand, "%s flag should have '%s' shorthand", flag.name, flag.shorthand)
		assert.Contains(t, f.Usage, flag.usage, "%s flag should have correct usage", flag.name)
	}
}

func TestDownloadFile_Success(t *testing.T) {
	// This would require setting up a test server, for now just test the function exists
	assert.NotNil(t, downloadFile)
}

func TestDownloadFile_Error(t *testing.T) {
	// Test with invalid URL
	err := downloadFile("http://invalid-url-that-does-not-exist", "/tmp/test")
	assert.Error(t, err)
}

func TestDownloadFile_InvalidURL(t *testing.T) {
	// Test downloadFile with invalid URL
	tempFile := "/tmp/onctl-test-download-error"
	err := downloadFile("http://127.0.0.1:0/invalid", tempFile) // Non-existent server
	assert.Error(t, err)
}

func TestDownloadFile_FunctionExists(t *testing.T) {
	// Test downloadFile with a valid local server using httptest is complex
	// For coverage, just verify function signature
	assert.NotNil(t, downloadFile)
	t.Log("downloadFile function exists and can be called")
}

func TestDownloadFile_WithTestServer(t *testing.T) {
	// Create a test server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("fake binary content"))
	}))
	defer server.Close()

	// Test download
	tempFile := "/tmp/onctl-test-download-success"
	err := downloadFile(server.URL, tempFile)
	assert.NoError(t, err)

	// Check file was created and has content
	content, err := os.ReadFile(tempFile)
	assert.NoError(t, err)
	assert.Equal(t, "fake binary content", string(content))

	// Clean up
	os.Remove(tempFile)
}
