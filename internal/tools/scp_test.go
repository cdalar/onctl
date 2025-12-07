package tools

import (
	"os"
	"testing"
)

func TestSCPCopyFileWithProgress_NonExistentFile(t *testing.T) {
	r := &Remote{
		Username:   "test",
		IPAddress:  "127.0.0.1",
		SSHPort:    22,
		PrivateKey: "fake-key",
	}

	err := r.SCPCopyFileWithProgress("nonexistent-file.txt", "remote.txt", nil)
	if err == nil {
		t.Error("Expected error for non-existent file, got nil")
	}
}

func TestSCPCopyFileWithProgress_EmptyCallback(t *testing.T) {
	// Create a temporary file
	tmpFile, err := os.CreateTemp("", "scp-test-")
	if err != nil {
		t.Fatalf("Failed to create temp file: %v", err)
	}
	defer func() { _ = os.Remove(tmpFile.Name()) }()

	// Write some content
	content := []byte("test content for scp")
	if _, err := tmpFile.Write(content); err != nil {
		t.Fatalf("Failed to write to temp file: %v", err)
	}
	_ = tmpFile.Close()

	r := &Remote{
		Username:   "test",
		IPAddress:  "127.0.0.1",
		SSHPort:    22,
		PrivateKey: "fake-key",
	}

	// This will fail because we can't actually connect, but we're testing
	// that the function handles nil callback properly and gets the file size
	err = r.SCPCopyFileWithProgress(tmpFile.Name(), "remote.txt", nil)
	// We expect an error because we can't actually create the SSH connection
	// but the function should not panic with nil callback
	if err == nil {
		t.Error("Expected error without valid SSH connection, got nil")
	}
}

func TestSCPCopyFileWithProgress_WithCallback(t *testing.T) {
	// Create a temporary file
	tmpFile, err := os.CreateTemp("", "scp-test-")
	if err != nil {
		t.Fatalf("Failed to create temp file: %v", err)
	}
	defer func() { _ = os.Remove(tmpFile.Name()) }()

	// Write some content
	content := []byte("test content for scp")
	if _, err := tmpFile.Write(content); err != nil {
		t.Fatalf("Failed to write to temp file: %v", err)
	}
	_ = tmpFile.Close()

	callbackCalled := false
	progressCallback := func(current, total int64) {
		callbackCalled = true
		if current < 0 || total < 0 {
			t.Errorf("Invalid progress values: current=%d, total=%d", current, total)
		}
	}

	r := &Remote{
		Username:   "test",
		IPAddress:  "127.0.0.1",
		SSHPort:    22,
		PrivateKey: "fake-key",
	}

	// This will fail because we can't actually connect
	err = r.SCPCopyFileWithProgress(tmpFile.Name(), "remote.txt", progressCallback)
	// We expect an error because we can't actually create the SSH connection
	if err == nil {
		t.Error("Expected error without valid SSH connection, got nil")
	}

	// The callback should not have been called since the connection failed
	// before the actual transfer
	if callbackCalled {
		t.Error("Callback was called despite connection error")
	}
}

func TestSSHCopyFileWithProgress_NoCallback(t *testing.T) {
	// Create a temporary file
	tmpFile, err := os.CreateTemp("", "scp-test-")
	if err != nil {
		t.Fatalf("Failed to create temp file: %v", err)
	}
	defer func() { _ = os.Remove(tmpFile.Name()) }()

	// Write some content
	content := []byte("test content")
	if _, err := tmpFile.Write(content); err != nil {
		t.Fatalf("Failed to write to temp file: %v", err)
	}
	_ = tmpFile.Close()

	r := &Remote{
		Username:   "test",
		IPAddress:  "127.0.0.1",
		SSHPort:    22,
		PrivateKey: "fake-key",
	}

	// This will fail because we can't actually connect
	err = r.SSHCopyFileWithProgress(tmpFile.Name(), "remote.txt", nil)
	if err == nil {
		t.Error("Expected error without valid SSH connection, got nil")
	}
}

func TestSSHCopyFileWithProgress_EmptyFile(t *testing.T) {
	// Create an empty temporary file
	tmpFile, err := os.CreateTemp("", "scp-test-empty-")
	if err != nil {
		t.Fatalf("Failed to create temp file: %v", err)
	}
	defer func() { _ = os.Remove(tmpFile.Name()) }()
	_ = tmpFile.Close()

	progressCallback := func(current, total int64) {
		if total != 0 {
			t.Errorf("Expected total=0 for empty file, got %d", total)
		}
	}

	r := &Remote{
		Username:   "test",
		IPAddress:  "127.0.0.1",
		SSHPort:    22,
		PrivateKey: "fake-key",
	}

	// This will fail because we can't actually connect
	err = r.SSHCopyFileWithProgress(tmpFile.Name(), "remote.txt", progressCallback)
	if err == nil {
		t.Error("Expected error without valid SSH connection, got nil")
	}
}

func TestDownloadFile_NonExistentFile(t *testing.T) {
	r := &Remote{
		Username:   "test",
		IPAddress:  "127.0.0.1",
		SSHPort:    22,
		PrivateKey: "fake-key",
	}

	// This will fail because we can't actually connect
	err := r.DownloadFile("remote.txt", "local.txt")
	if err == nil {
		t.Error("Expected error without valid SSH connection, got nil")
	}
}
