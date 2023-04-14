package tools

import (
	"encoding/json"

	"os"
	"testing"
)

func TestGetGithubPRNumber(t *testing.T) {
	// Create a temporary file with JSON data
	tmpfile, err := os.CreateTemp("", "example.json")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(tmpfile.Name())

	// Write JSON data to the temporary file
	data := map[string]interface{}{"number": 42}
	jsonData, err := json.Marshal(data)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := tmpfile.Write(jsonData); err != nil {
		t.Fatal(err)
	}
	if err := tmpfile.Close(); err != nil {
		t.Fatal(err)
	}

	// Call the function with the temporary file name
	result := getGithubPRNumber(tmpfile.Name())

	// Check if the result is correct
	if result != "42" {
		t.Errorf("GetGithubPRNumber(%q) = %q, want %q", tmpfile.Name(), result, "42")
	}
}
