package providerhtz

import (
	"testing"
)

// With a token set, GetClient must hand back a usable client rather than
// an error: every Hetzner command in cmd/ dereferences it immediately.
func TestGetClient_WithToken(t *testing.T) {
	t.Setenv("HCLOUD_TOKEN", "test-token")
	client, err := GetClient()
	if err != nil {
		t.Fatalf("unexpected error with HCLOUD_TOKEN set: %v", err)
	}
	if client == nil {
		t.Fatal("GetClient() returned nil with HCLOUD_TOKEN set")
	}
}

// Without a token GetClient must return an error instead of returning a
// nil client that would panic later with a confusing nil-pointer error.
func TestGetClient_MissingToken(t *testing.T) {
	t.Setenv("HCLOUD_TOKEN", "")
	client, err := GetClient()
	if err == nil {
		t.Fatal("expected error when HCLOUD_TOKEN is unset, got nil")
	}
	if client != nil {
		t.Fatalf("expected nil client when HCLOUD_TOKEN is unset, got %v", client)
	}
}
