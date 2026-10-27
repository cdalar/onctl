package tools

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// A shell on the VM gets a TERM it has terminfo for: Ghostty's or kitty's
// own one isn't on the image, and tmux refuses to start with it.
func TestSSHIntoVM_UsesACommonTERM(t *testing.T) {
	dir := t.TempDir()
	out := filepath.Join(dir, "term")
	fake := "#!/bin/sh\nprintf '%s' \"$TERM\" >" + out + "\n"
	if err := os.WriteFile(filepath.Join(dir, "ssh"), []byte(fake), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("TERM", "xterm-ghostty")

	SSHIntoVM(SSHIntoVMRequest{IPAddress: "vm", User: "root", Port: 22, PrivateKeyFile: "key"})

	got, err := os.ReadFile(out)
	if err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(string(got)) != "xterm-256color" {
		t.Errorf("TERM on the VM = %q, want xterm-256color", got)
	}
}
