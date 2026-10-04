package cmd

import (
	"archive/tar"
	"bytes"
	"encoding/json"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestClaudeBoxName(t *testing.T) {
	cases := map[string]string{
		"/Users/cd/src/boxctl-vms":                "claude-boxctl-vms",
		"/home/x/My Project":                      "claude-my-project",
		"/tmp/a-very-long-project-directory-name": "claude-a-very-long-project",
		"/tmp/--weird--":                          "claude-weird",
		"/tmp/日本":                                 "claude-project",
	}
	for dir, want := range cases {
		if got := claudeBoxName(dir); got != want {
			t.Errorf("claudeBoxName(%q) = %q, want %q", dir, got, want)
		}
	}
}

func TestSSHProxyHost(t *testing.T) {
	for in, want := range map[string]string{"my-box.box": "my-box", "my-box": "my-box", ".box": ""} {
		if got := sshProxyHost(in); got != want {
			t.Errorf("sshProxyHost(%q) = %q, want %q", in, got, want)
		}
	}
}

// TestSessionCommandQuoting runs the generated command line through a
// real shell, so a prompt with quotes or $ reaches claude intact.
func TestSessionCommandQuoting(t *testing.T) {
	s := newTaskSession("/Users/me/my proj", "")
	prompt := `fix "it" -- don't touch $HOME`
	line := s.commandLine(prompt, []string{"--model", "opus"})
	cmd := s.tmuxCommand(line, false)
	if !strings.HasPrefix(cmd, "tmux -u new-session -A -s claude -c '/Users/me/my proj' ") {
		t.Fatalf("got %q", cmd)
	}
	if !strings.Contains(s.tmuxCommand(line, true), " -d -s claude ") {
		t.Fatal("detached mode doesn't use -d")
	}
	// Replay what tmux hands sh -c, with a claude that prints its words.
	bin := t.TempDir()
	write(t, filepath.Join(bin, "claude"), "#!/bin/sh\nprintf '[%s]' \"$@\"\n")
	if err := os.Chmod(filepath.Join(bin, "claude"), 0o755); err != nil {
		t.Fatal(err)
	}
	c := exec.Command("sh", "-c", line)
	c.Env = append(os.Environ(), "HOME="+t.TempDir(), "PATH="+bin+":/usr/bin:/bin")
	out, err := c.Output()
	if err != nil {
		t.Fatal(err)
	}
	if want := `[--model][opus][` + prompt + `]`; string(out) != want {
		t.Fatalf("claude would get %s, want %s", out, want)
	}
}

func run(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command(args[0], args[1:]...)
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("%v: %v: %s", args, err, out)
	}
}

func write(t *testing.T, p, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestProjectFilesFollowsGit(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("no git")
	}
	// Only the repository's own rules: a global excludesFile (most Macs
	// ignore .DS_Store there) would change what git lists.
	t.Setenv("GIT_CONFIG_GLOBAL", os.DevNull)
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	dir := t.TempDir()
	run(t, dir, "git", "init", "-q")
	write(t, filepath.Join(dir, ".gitignore"), "node_modules/\n*.log\n")
	write(t, filepath.Join(dir, "a.txt"), "a")
	write(t, filepath.Join(dir, "gone.txt"), "x")
	run(t, dir, "git", "add", "-A")
	run(t, dir, "git", "-c", "user.email=t@t", "-c", "user.name=t", "commit", "-qm", "init")
	write(t, filepath.Join(dir, "new/b.txt"), "b")                    // untracked
	write(t, filepath.Join(dir, "node_modules/x.js"), "x")            // ignored
	write(t, filepath.Join(dir, "debug.log"), "x")                    // ignored
	write(t, filepath.Join(dir, ".DS_Store"), "x")                    // untracked, but Finder's
	if err := os.Remove(filepath.Join(dir, "gone.txt")); err != nil { // deleted, not staged
		t.Fatal(err)
	}

	files, err := projectFiles(dir)
	if err != nil {
		t.Fatal(err)
	}
	var work []string
	hasGit := false
	for _, f := range files {
		if strings.HasPrefix(f, ".git/") {
			hasGit = true
			continue
		}
		work = append(work, f)
	}
	want := []string{".DS_Store", ".gitignore", "a.txt", "new/b.txt"}
	if !reflect.DeepEqual(work, want) {
		t.Errorf("working-tree files %v, want %v", work, want)
	}
	if !hasGit {
		t.Error(".git wasn't included")
	}
}

func TestProjectFilesOutsideGit(t *testing.T) {
	dir := t.TempDir()
	write(t, filepath.Join(dir, "a/b.txt"), "b")
	write(t, filepath.Join(dir, ".DS_Store"), "x")
	files, err := projectFiles(dir)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(files, []string{"a/b.txt"}) {
		t.Fatalf("got %v", files)
	}
}

func TestAddFilesKeepsModesAndLinks(t *testing.T) {
	dir := t.TempDir()
	write(t, filepath.Join(dir, "run.sh"), "#!/bin/sh\n")
	if err := os.Chmod(filepath.Join(dir, "run.sh"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("run.sh", filepath.Join(dir, "link")); err != nil {
		t.Fatal(err)
	}
	var buf bytes.Buffer
	tw := tar.NewWriter(&buf)
	if err := addFiles(tw, dir, []string{"link", "missing", "run.sh"}); err != nil {
		t.Fatal(err)
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	tr := tar.NewReader(&buf)
	got := map[string]*tar.Header{}
	for {
		h, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		got[h.Name] = h
	}
	if len(got) != 2 {
		t.Fatalf("entries %v: a file gone since listing must be skipped", got)
	}
	if h := got["run.sh"]; h.Mode&0o111 == 0 || h.Uname != "root" || h.Uid != 0 {
		t.Errorf("run.sh: %+v", h)
	}
	if h := got["link"]; h.Typeflag != tar.TypeSymlink || h.Linkname != "run.sh" {
		t.Errorf("link: %+v", h)
	}
}

func TestBoxSettingsDropsMachineSpecificKeys(t *testing.T) {
	p := filepath.Join(t.TempDir(), "settings.json")
	write(t, p, `{"model":"opus","permissions":{"allow":["Bash(ls)"]},"hooks":{"Stop":[]},"statusLine":{"command":"~/bin/x"},"env":{"SECRET":"1"},"enabledPlugins":{"x@y":true}}`)
	data, err := boxSettings(p)
	if err != nil {
		t.Fatal(err)
	}
	var got map[string]any
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got["model"] != "opus" || got["permissions"] == nil {
		t.Fatalf("got %v", got)
	}
	if data, err := boxSettings(filepath.Join(t.TempDir(), "none.json")); err != nil || data != nil {
		t.Fatalf("missing settings.json: %s %v", data, err)
	}
}

func TestIsClaudeCommand(t *testing.T) {
	if !isClaudeCommand(claudeCmd) {
		t.Error("claude itself")
	}
	for _, sub := range claudeCmd.Commands() {
		if !isClaudeCommand(sub) {
			t.Errorf("claude %s", sub.Name())
		}
	}
	if isClaudeCommand(sshCmd) || isClaudeCommand(rootCmd) {
		t.Error("ssh and the root aren't claude commands")
	}
}
