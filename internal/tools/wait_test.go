package tools

import (
	"strings"
	"testing"
	"time"
)

// The waits run inside the boxctl host service through pkg/box, so a VM that
// never becomes reachable must come back as an error the caller can report,
// not end the process (log.Fatal) and take every other box's work with it.
// A Remote without a private key never connects, so each wait runs to its
// timeout.

func TestWaitForSSH_TimeoutReturnsError(t *testing.T) {
	r := &Remote{IPAddress: "192.0.2.1", SSHPort: 22}
	start := time.Now()
	err := r.WaitForSSH("300ms")
	if err == nil || !strings.Contains(err.Error(), "timeout") {
		t.Fatalf("WaitForSSH = %v; want a timeout error", err)
	}
	if time.Since(start) > 5*time.Second {
		t.Fatalf("WaitForSSH took %v; the 300ms timeout wasn't honoured", time.Since(start))
	}
}

func TestWaitForSSH_BadTimeoutReturnsError(t *testing.T) {
	if err := (&Remote{}).WaitForSSH("soon"); err == nil {
		t.Fatal("WaitForSSH(\"soon\") = nil; want an invalid-timeout error")
	}
}

func TestWaitForCloudInit_TimeoutReturnsError(t *testing.T) {
	r := &Remote{IPAddress: "192.0.2.1", SSHPort: 22}
	err := r.WaitForCloudInit("100ms")
	if err == nil || !strings.Contains(err.Error(), "cloud-init") {
		t.Fatalf("WaitForCloudInit = %v; want a cloud-init timeout error", err)
	}
}

func TestWaitForCloudInit_BadTimeoutReturnsError(t *testing.T) {
	if err := (&Remote{}).WaitForCloudInit("soon"); err == nil {
		t.Fatal("WaitForCloudInit(\"soon\") = nil; want an invalid-timeout error")
	}
}
