package providerhtz

import (
	"os"
	"os/exec"
	"testing"
)

// With a token set, GetClient must hand back a usable client rather than
// exiting: every Hetzner command in cmd/ dereferences it immediately.
func TestGetClient_WithToken(t *testing.T) {
	t.Setenv("HCLOUD_TOKEN", "test-token")
	if GetClient() == nil {
		t.Fatal("GetClient() returned nil with HCLOUD_TOKEN set")
	}
}

// Without a token GetClient must fail loudly (exit 1) instead of returning a
// nil client that would panic later with a confusing nil-pointer error.
func TestGetClient_MissingTokenExits(t *testing.T) {
	if os.Getenv("PROVIDERHTZ_EXIT_HELPER") == "1" {
		GetClient()
		return // only reached if GetClient did not exit
	}
	cmd := exec.Command(os.Args[0], "-test.run=^TestGetClient_MissingTokenExits$")
	cmd.Env = append(os.Environ(), "PROVIDERHTZ_EXIT_HELPER=1", "HCLOUD_TOKEN=")
	err := cmd.Run()
	exitErr, ok := err.(*exec.ExitError)
	if !ok || exitErr.ExitCode() != 1 {
		t.Fatalf("expected exit code 1 when HCLOUD_TOKEN is unset, got err=%v", err)
	}
}
