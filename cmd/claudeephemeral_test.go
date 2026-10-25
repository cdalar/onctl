package cmd

import (
	"os/exec"
	"strings"
	"testing"
)

// TestEphemeralRun: a run gets a box of its own unless something says the
// box is meant to last -- --keep, a named box, a detached start (nobody
// is there when Claude exits) or a task (a worktree on the project's box).
func TestEphemeralRun(t *testing.T) {
	saved := []*bool{&claudeKeep, &claudeDetach}
	savedStr := []*string{&claudeBox, &claudeTask}
	t.Cleanup(func() {
		for _, b := range saved {
			*b = false
		}
		for _, s := range savedStr {
			*s = ""
		}
	})
	for name, set := range map[string]func(){
		"default":  func() {},
		"--keep":   func() { claudeKeep = true },
		"--box":    func() { claudeBox = "claude-x" },
		"--detach": func() { claudeDetach = true },
		"--task":   func() { claudeTask = "auth" },
	} {
		claudeKeep, claudeDetach, claudeBox, claudeTask = false, false, "", ""
		set()
		if got := ephemeralRun(); got != (name == "default") {
			t.Errorf("%s: ephemeralRun() = %v", name, got)
		}
	}
}

// TestCloneURL: a GitHub origin of any shape is cloned over https (the
// box's credential helper serves that); another origin as it is; no
// origin means the project is copied, not cloned.
func TestCloneURL(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("no git")
	}
	repo := func(origin string) string {
		dir := t.TempDir()
		run(t, dir, "git", "init", "-q")
		if origin != "" {
			run(t, dir, "git", "remote", "add", "origin", origin)
		}
		return dir
	}
	for origin, want := range map[string]string{
		"git@github.com:acme/api.git":         "https://github.com/acme/api.git",
		"ssh://git@github.com/acme/api":       "https://github.com/acme/api.git",
		"https://github.com/acme/api.git":     "https://github.com/acme/api.git",
		"git@gitlab.example.com:acme/api.git": "git@gitlab.example.com:acme/api.git",
		"":                                    "",
	} {
		if got := cloneURL(repo(origin)); got != want {
			t.Errorf("cloneURL(origin %q) = %q, want %q", origin, got, want)
		}
	}
	if got := cloneURL(t.TempDir()); got != "" {
		t.Errorf("cloneURL(not a repository) = %q", got)
	}
}

// TestSaveBoxWorkWithNothingToSave: a box the project never reached, or a
// project that isn't a repository, has nothing to fetch -- the box can go
// without a connection to it (box is nil here and must not be touched).
func TestSaveBoxWorkWithNothingToSave(t *testing.T) {
	if !saveBoxWork(t.Context(), &claudeTarget{dir: t.TempDir(), name: "x", needsCopy: true}) {
		t.Error("a box without the project must go")
	}
	if !saveBoxWork(t.Context(), &claudeTarget{dir: t.TempDir(), name: "x"}) {
		t.Error("a box with a non-git project must go")
	}
}

// TestRunWithArgs: a forward rides on a connection of its own, like an
// attach -- the shared control master was opened without it.
func TestRunWithArgs(t *testing.T) {
	b := &boxSSH{name: "b", args: []string{"-o", "ControlPath=/cm", "root@b"}}
	got := strings.Join(b.runWithArgs([]string{"-R", "/r:/l"}, "git clone"), " ")
	for _, want := range []string{"-o ControlPath=none", "-R /r:/l", "-o ControlPath=/cm root@b git clone"} {
		if !strings.Contains(got, want) {
			t.Errorf("no %q in %q", want, got)
		}
	}
	if !strings.Contains(got, "ControlPath=none -o ExitOnForwardFailure=no -R") || strings.Index(got, "ControlPath=none") > strings.Index(got, "ControlPath=/cm") {
		t.Errorf("the connection's own options must come first: %q", got)
	}
}
