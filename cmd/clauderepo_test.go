package cmd

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// --repo takes owner/name and nothing else: a URL or a bare name would
// be cloned from the wrong place, or nowhere.
func TestParseRepo(t *testing.T) {
	for in, want := range map[string]string{
		"cdalar/onctl":      "cdalar/onctl",
		" cdalar/onctl.git": "cdalar/onctl",
		"my-org/my.repo_1":  "my-org/my.repo_1",
	} {
		if got, err := parseRepo(in); err != nil || got != want {
			t.Errorf("parseRepo(%q) = %q, %v; want %q", in, got, err, want)
		}
	}
	for _, in := range []string{"", "onctl", "https://github.com/cdalar/onctl", "a/b/c", "a b/c"} {
		if got, err := parseRepo(in); err == nil {
			t.Errorf("parseRepo(%q) = %q, want an error", in, got)
		}
	}
}

// The clone lands at /root/<name> and the box is named for it the way
// every Claude box is, so `onctl claude --repo a/x` and `b/x` share one.
func TestRepoDirAndBox(t *testing.T) {
	if got := repoDir("cdalar/onctl"); got != "/root/onctl" {
		t.Errorf("repoDir = %q", got)
	}
	if got := claudeBoxName(repoDir("cdalar/My.Repo")); got != "claude-my-repo" {
		t.Errorf("box = %q", got)
	}
	b := repoBranch()
	if !strings.HasPrefix(b, "onctl/") || len(b) != len("onctl/")+6 || b == repoBranch() {
		t.Errorf("branch = %q, want onctl/<6 hex>, new each time", b)
	}
}

// TestCloneScript runs the box-side script for real against a local
// repository: the default branch comes down, every branch is mapped for
// later fetches, and Claude starts on a branch of its own.
func TestCloneScript(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("no git")
	}
	origin := filepath.Join(t.TempDir(), "origin")
	run(t, t.TempDir(), "git", "init", "-q", "-b", "main", origin)
	write(t, filepath.Join(origin, "README"), "hi\n")
	run(t, origin, "git", "-c", "user.name=t", "-c", "user.email=t@x", "add", ".")
	run(t, origin, "git", "-c", "user.name=t", "-c", "user.email=t@x", "commit", "-q", "-m", "init")
	run(t, origin, "git", "branch", "other")

	dir := filepath.Join(t.TempDir(), "work", "onctl")
	c := exec.Command("sh", "-c", cloneScript(origin, dir, "onctl/ab12cd"))
	if out, err := c.CombinedOutput(); err != nil {
		t.Fatalf("clone script: %v\n%s", err, out)
	}
	branch, _ := exec.Command("git", "-C", dir, "branch", "--show-current").Output()
	if strings.TrimSpace(string(branch)) != "onctl/ab12cd" {
		t.Errorf("on branch %q", branch)
	}
	if _, err := os.Stat(filepath.Join(dir, "README")); err != nil {
		t.Errorf("README missing: %v", err)
	}
	// --single-branch fetched main only, but a later fetch sees other.
	run(t, dir, "git", "fetch", "-q", "origin")
	refs, _ := exec.Command("git", "-C", dir, "branch", "-r").Output()
	if !strings.Contains(string(refs), "origin/other") {
		t.Errorf("remote branches after fetch: %q", refs)
	}
	// A second clone into the same place would clone into the checkout.
	if out, err := exec.Command("sh", "-c", cloneScript(origin, dir, "onctl/ef34ab")).CombinedOutput(); err == nil || !strings.Contains(string(out), "already exists") {
		t.Errorf("cloning over an existing directory: %v, %s", err, out)
	}
}

// --repo is a project of its own on the box, so the flags about a
// project here are refused up front rather than half-applied.
func TestRepoConflicts(t *testing.T) {
	defer func() { claudeTask, claudeOwnBox, claudeHandoff, claudeProject = "", false, "", "" }()
	if got := repoConflicts(); len(got) != 0 {
		t.Errorf("nothing set: %v", got)
	}
	claudeTask, claudeHandoff = "auth", "abc"
	if got := strings.Join(repoConflicts(), " "); got != "--task --handoff" {
		t.Errorf("conflicts = %q", got)
	}
}

// The way back to a --repo box is the same --repo (a --box from a
// directory here would be another project), on the same provider.
func TestRepoAttachHint(t *testing.T) {
	withCloudProvider(t, "hetzner")
	if got := repoAttachHint("cdalar/onctl"); got != "onctl -p hetzner claude --repo cdalar/onctl" {
		t.Errorf("hint = %q", got)
	}
}
