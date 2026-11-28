package providerhtz

import (
	"testing"
)

// With a token set, GetClient must hand back a usable client: every Hetzner
// command in cmd/ dereferences it immediately.
func TestGetClient_WithToken(t *testing.T) {
	t.Setenv("HCLOUD_TOKEN", "test-token")
	c, err := GetClient()
	if err != nil || c == nil {
		t.Fatalf("GetClient() = %v, %v with HCLOUD_TOKEN set", c, err)
	}
}

// Without a token GetClient must return an error, not exit the process (a
// server using pkg/box would go down with it) and not hand back a nil client
// that panics later with a confusing nil-pointer error.
func TestGetClient_MissingToken(t *testing.T) {
	t.Setenv("HCLOUD_TOKEN", "")
	c, err := GetClient()
	if err == nil || c != nil {
		t.Fatalf("GetClient() = %v, %v; want nil client and an error", c, err)
	}
}
