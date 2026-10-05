package cmd

import (
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func capturePluginHint(t *testing.T) string {
	t.Helper()
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	orig := os.Stdout
	os.Stdout = w
	printPluginHint()
	os.Stdout = orig
	_ = w.Close()
	out, _ := io.ReadAll(r)
	return string(out)
}

func TestPrintPluginHint(t *testing.T) {
	bin := t.TempDir()
	if err := os.WriteFile(filepath.Join(bin, "claude"), []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}

	t.Setenv("CLAUDECODE", "")
	t.Setenv("PATH", t.TempDir())
	if out := capturePluginHint(t); out != "" {
		t.Errorf("no claude on PATH: printed %q", out)
	}

	t.Setenv("PATH", bin)
	if out := capturePluginHint(t); !strings.Contains(out, "claude plugin install onctl@onctl") {
		t.Errorf("claude on PATH: got %q", out)
	}

	t.Setenv("CLAUDECODE", "1")
	if out := capturePluginHint(t); out != "" {
		t.Errorf("inside Claude Code: printed %q", out)
	}
}
