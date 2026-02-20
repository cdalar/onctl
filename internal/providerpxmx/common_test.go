package providerpxmx

import (
	"os"
	"testing"
)

// TestGetClient_MissingEnvVars tests that GetClient returns an error when required
// environment variables are not set.
func TestGetClient_MissingEnvVars(t *testing.T) {
	// Save and clear env vars
	vars := []string{"PROXMOX_API_URL", "PROXMOX_TOKEN_ID", "PROXMOX_SECRET"}
	originals := make(map[string]string, len(vars))
	for _, v := range vars {
		originals[v] = os.Getenv(v)
		_ = os.Unsetenv(v)
	}
	defer func() {
		for k, v := range originals {
			_ = os.Setenv(k, v)
		}
	}()

	_, err := GetClient()
	if err == nil {
		t.Error("expected error when env vars are missing, got nil")
	}
}

// TestGetClient_MissingAPIURL tests that GetClient errors when only PROXMOX_API_URL is unset.
func TestGetClient_MissingAPIURL(t *testing.T) {
	orig := os.Getenv("PROXMOX_API_URL")
	_ = os.Unsetenv("PROXMOX_API_URL")
	defer func() { _ = os.Setenv("PROXMOX_API_URL", orig) }()

	_ = os.Setenv("PROXMOX_TOKEN_ID", "token")
	defer func() { _ = os.Unsetenv("PROXMOX_TOKEN_ID") }()

	_ = os.Setenv("PROXMOX_SECRET", "secret")
	defer func() { _ = os.Unsetenv("PROXMOX_SECRET") }()

	_, err := GetClient()
	if err == nil {
		t.Error("expected error when PROXMOX_API_URL is missing, got nil")
	}
}

// TestGetClient_MissingTokenID tests that GetClient errors when PROXMOX_TOKEN_ID is unset.
func TestGetClient_MissingTokenID(t *testing.T) {
	_ = os.Setenv("PROXMOX_API_URL", "https://proxmox.example.com:8006/api2/json")
	defer func() { _ = os.Unsetenv("PROXMOX_API_URL") }()

	orig := os.Getenv("PROXMOX_TOKEN_ID")
	_ = os.Unsetenv("PROXMOX_TOKEN_ID")
	defer func() { _ = os.Setenv("PROXMOX_TOKEN_ID", orig) }()

	_ = os.Setenv("PROXMOX_SECRET", "secret")
	defer func() { _ = os.Unsetenv("PROXMOX_SECRET") }()

	_, err := GetClient()
	if err == nil {
		t.Error("expected error when PROXMOX_TOKEN_ID is missing, got nil")
	}
}

// TestGetClient_MissingSecret tests that GetClient errors when PROXMOX_SECRET is unset.
func TestGetClient_MissingSecret(t *testing.T) {
	_ = os.Setenv("PROXMOX_API_URL", "https://proxmox.example.com:8006/api2/json")
	defer func() { _ = os.Unsetenv("PROXMOX_API_URL") }()

	_ = os.Setenv("PROXMOX_TOKEN_ID", "token")
	defer func() { _ = os.Unsetenv("PROXMOX_TOKEN_ID") }()

	orig := os.Getenv("PROXMOX_SECRET")
	_ = os.Unsetenv("PROXMOX_SECRET")
	defer func() { _ = os.Setenv("PROXMOX_SECRET", orig) }()

	_, err := GetClient()
	if err == nil {
		t.Error("expected error when PROXMOX_SECRET is missing, got nil")
	}
}

// TestGetClient_AllEnvVarsSet tests that GetClient succeeds (returns client) when all env vars are set.
// It uses a bogus URL which is fine — the client creation itself doesn't make network calls.
func TestGetClient_AllEnvVarsSet(t *testing.T) {
	_ = os.Setenv("PROXMOX_API_URL", "https://proxmox.example.com:8006/api2/json")
	defer func() { _ = os.Unsetenv("PROXMOX_API_URL") }()

	_ = os.Setenv("PROXMOX_TOKEN_ID", "user@pam!mytoken")
	defer func() { _ = os.Unsetenv("PROXMOX_TOKEN_ID") }()

	_ = os.Setenv("PROXMOX_SECRET", "xxxxxxxx-xxxx-xxxx-xxxx-xxxxxxxxxxxx")
	defer func() { _ = os.Unsetenv("PROXMOX_SECRET") }()

	client, err := GetClient()
	if err != nil {
		t.Fatalf("GetClient returned unexpected error: %v", err)
	}
	if client == nil {
		t.Error("GetClient returned nil client without error")
	}
}
