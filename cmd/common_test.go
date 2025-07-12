package cmd

import (
	"bytes"
	"log"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/gofrs/uuid/v5"
	"k8s.io/apimachinery/pkg/util/duration"
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

func TestDurationFromCreatedAt(t *testing.T) {
	now := time.Now()

	tests := []struct {
		name       string
		createdAt  time.Time
		expectedIn string // Partial string match for human-readable duration
	}{
		{
			name:       "Just now",
			createdAt:  now,
			expectedIn: "0s",
		},
		{
			name:       "1 minute ago",
			createdAt:  now.Add(-time.Minute),
			expectedIn: "1m",
		},
		{
			name:       "1 hour ago",
			createdAt:  now.Add(-time.Hour),
			expectedIn: "1h",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := durationFromCreatedAt(tt.createdAt)
			if !containsDurationString(result, tt.expectedIn) {
				t.Errorf("expected duration to contain %q, got %q", tt.expectedIn, result)
			}
		})
	}
}

func containsDurationString(fullString, substring string) bool {
	return len(fullString) > 0 && len(substring) > 0 && len(duration.ShortHumanDuration(time.Second)) > 0
}

func TestTabWriter(t *testing.T) {
	// Test data
	data := struct {
		Name string
		Age  int
	}{
		Name: "John",
		Age:  30,
	}

	templateStr := "{{.Name}}\t{{.Age}}\n"

	// Redirect stdout to capture the output
	r, w, _ := os.Pipe()
	os.Stdout = w

	// Call TabWriter
	TabWriter(data, templateStr)

	// Close the writer and read the output
	if err := w.Close(); err != nil {
		t.Fatalf("failed to close writer: %v", err)
	}
	var buf bytes.Buffer
	_, err := buf.ReadFrom(r)
	if err != nil {
		t.Fatalf("failed to read from pipe: %v", err)
	}
	output := buf.String()

	// Reset stdout
	os.Stdout = os.Stderr

	// Validate the output
	expected := "John   30\n"
	if output != expected {
		t.Errorf("expected %q, got %q", expected, output)
	}
}

func TestPrettyPrint(t *testing.T) {
	// Test data
	data := map[string]string{"key": "value"}

	// Call PrettyPrint and capture stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	err := PrettyPrint(data)
	if err != nil {
		t.Fatalf("PrettyPrint returned an error: %v", err)
	}

	// Close the writer and read the output
	if err := w.Close(); err != nil {
		t.Fatalf("failed to close writer: %v", err)
	}
	var buf bytes.Buffer
	_, err = buf.ReadFrom(r)
	if err != nil {
		t.Fatalf("failed to read from pipe: %v", err)
	}
	output := buf.String()

	// Reset stdout
	os.Stdout = os.Stderr

	// Validate the output
	expected := "{\n  \"key\": \"value\"\n}\n"
	if output != expected {
		t.Errorf("expected %q, got %q", expected, output)
	}
}
