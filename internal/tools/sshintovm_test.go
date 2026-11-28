package tools

import (
	"bytes"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// SSHIntoVM used to os.Exit with ssh's status, which ended any program
// calling it (the boxctl host service through pkg/box). It must hand the
// status back as an *exec.ExitError and send output to the given writers.
// A fake ssh on PATH stands in for the real one.
func TestSSHIntoVM_ReturnsExitStatusAndUsesStdio(t *testing.T) {
	dir := t.TempDir()
	script := "#!/bin/sh\necho \"ran: $*\"\necho warn >&2\nexit 7\n"
	if err := os.WriteFile(filepath.Join(dir, "ssh"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))

	var out, errOut bytes.Buffer
	err := SSHIntoVM(SSHIntoVMRequest{
		IPAddress: "10.0.0.2", User: "root", Port: 22, PrivateKeyFile: "/k",
		Command: []string{"sync"}, Stdin: &bytes.Buffer{}, Stdout: &out, Stderr: &errOut,
	})
	var exitErr *exec.ExitError
	if !errors.As(err, &exitErr) || exitErr.ExitCode() != 7 {
		t.Fatalf("SSHIntoVM = %v; want an *exec.ExitError with code 7", err)
	}
	if !bytes.Contains(out.Bytes(), []byte("10.0.0.2 -p 22 -- sync")) {
		t.Errorf("stdout = %q; want the ssh args ending in the host, port and command", out.String())
	}
	if errOut.String() != "warn\n" {
		t.Errorf("stderr = %q; want %q", errOut.String(), "warn\n")
	}
}
