package cmd

import (
	"log"
	"os"
	"strings"
	"testing"

	"github.com/gofrs/uuid/v5"
)

func TestGenerateIDToken(t *testing.T) {
	// Capture log output for validation
	var logOutput strings.Builder
	log.SetOutput(&logOutput)

	// Generate a UUID
	token := GenerateIDToken()

	// Validate the token is not nil
	if token == uuid.Nil {
		t.Fatalf("expected a valid UUID, got nil UUID")
	}

	// Validate that the log contains the expected debug message
	logContents := logOutput.String()
	expectedLogSubstring := "[DEBUG] ID Token generated"
	if !strings.Contains(logContents, expectedLogSubstring) {
		t.Fatalf("expected log to contain %q, got %q", expectedLogSubstring, logContents)
	}

	// Reset log output to default
	log.SetOutput(os.Stderr)
}
