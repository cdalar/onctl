package tools

import (
	"os"
	"path/filepath"
	"testing"
)

// Each apply gets its own numbered directory so earlier runs' files are never
// overwritten: the first call creates .onctl/apply00 and later ones count up.
func TestNextApplyDir_NumbersDirectoriesSequentially(t *testing.T) {
	t.Chdir(t.TempDir())

	for i, want := range []string{
		"./.onctl/apply00",
		"./.onctl/apply01",
		"./.onctl/apply02",
	} {
		got, err := NextApplyDir("")
		if err != nil {
			t.Fatalf("call %d: unexpected error: %v", i, err)
		}
		if got != want {
			t.Fatalf("call %d: got %q, want %q", i, got, want)
		}
		if info, err := os.Stat(got); err != nil || !info.IsDir() {
			t.Fatalf("call %d: %q was not created as a directory (err=%v)", i, got, err)
		}
	}
}

// Numbering continues from the highest existing apply dir, not from the
// count of entries, and ignores entries that are not applyNN directories.
func TestNextApplyDir_ContinuesFromHighestExisting(t *testing.T) {
	root := t.TempDir()
	for _, d := range []string{"apply03", "apply07", "scratch"} {
		if err := os.MkdirAll(filepath.Join(root, ONCTLDIR, d), 0755); err != nil {
			t.Fatal(err)
		}
	}
	t.Chdir(root)

	got, err := NextApplyDir(".")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if want := "./.onctl/apply08"; got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}
