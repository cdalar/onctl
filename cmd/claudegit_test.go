package cmd

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func sh(t *testing.T, script string) string {
	t.Helper()
	out, err := exec.Command("sh", "-c", script).CombinedOutput()
	if err != nil {
		t.Fatalf("%v: %s", err, out)
	}
	return strings.TrimSpace(string(out))
}

func gitOut(t *testing.T, dir string, args ...string) string {
	t.Helper()
	out, err := exec.Command("git", append([]string{"-C", dir}, args...)...).Output()
	if err != nil {
		t.Fatalf("git %v: %v", args, err)
	}
	return strings.TrimSpace(string(out))
}

// TestWipSnapshot runs the box-side script for real, in a local repo.
func TestWipSnapshot(t *testing.T) {
	t.Setenv("GIT_CONFIG_GLOBAL", os.DevNull)
	dir := t.TempDir()
	run(t, dir, "git", "init", "-q")
	run(t, dir, "git", "config", "user.name", "Box User")
	run(t, dir, "git", "config", "user.email", "box@example.com")
	write(t, filepath.Join(dir, ".gitignore"), "build/\n")
	write(t, filepath.Join(dir, "a.txt"), "a")
	run(t, dir, "git", "add", "-A")
	run(t, dir, "git", "commit", "-qm", "init")
	head := gitOut(t, dir, "rev-parse", "HEAD")

	// Clean: nothing to snapshot.
	if got := sh(t, wipSnapshotScript(dir, wipRef)); got != "none" {
		t.Fatalf("clean tree: got %q", got)
	}

	write(t, filepath.Join(dir, "a.txt"), "a, edited")
	write(t, filepath.Join(dir, "new.txt"), "new")
	write(t, filepath.Join(dir, "build/out.bin"), "ignored")
	run(t, dir, "git", "add", "a.txt") // a staged change, to check the real index survives
	indexBefore, _ := os.ReadFile(filepath.Join(dir, ".git", "index"))

	c := sh(t, wipSnapshotScript(dir, wipRef))
	if gitOut(t, dir, "rev-parse", wipRef) != c {
		t.Fatalf("%s doesn't point at %s", wipRef, c)
	}
	if parent := gitOut(t, dir, "rev-parse", c+"^"); parent != head {
		t.Errorf("wip parent %s, want HEAD %s", parent, head)
	}
	files := gitOut(t, dir, "ls-tree", "-r", "--name-only", c)
	if !strings.Contains(files, "new.txt") || strings.Contains(files, "build/") {
		t.Errorf("wip tree: %q (want untracked new.txt, not ignored build/)", files)
	}
	if got := gitOut(t, dir, "show", c+":a.txt"); got != "a, edited" {
		t.Errorf("wip a.txt = %q", got)
	}
	if author := gitOut(t, dir, "log", "-1", "--format=%an <%ae>", c); author != "Box User <box@example.com>" {
		t.Errorf("author %q", author)
	}
	// Nothing on the box changes: same index, same HEAD, same status.
	indexAfter, _ := os.ReadFile(filepath.Join(dir, ".git", "index"))
	if string(indexBefore) != string(indexAfter) {
		t.Error("the real index changed")
	}
	if gitOut(t, dir, "rev-parse", "HEAD") != head {
		t.Error("HEAD moved")
	}
	if st := gitOut(t, dir, "status", "--porcelain"); !strings.Contains(st, "M  a.txt") || !strings.Contains(st, "?? new.txt") {
		t.Errorf("status changed: %q", st)
	}

	// Committing everything makes the snapshot go away again.
	run(t, dir, "git", "add", "-A")
	run(t, dir, "git", "commit", "-qm", "more")
	if got := sh(t, wipSnapshotScript(dir, wipRef)); got != "none" {
		t.Fatalf("after commit: %q", got)
	}
	if exec.Command("git", "-C", dir, "rev-parse", "-q", "--verify", wipRef).Run() == nil {
		t.Errorf("%s survived a clean tree", wipRef)
	}
}

func TestWipSnapshotWithoutCommits(t *testing.T) {
	t.Setenv("GIT_CONFIG_GLOBAL", os.DevNull)
	dir := t.TempDir()
	run(t, dir, "git", "init", "-q")
	write(t, filepath.Join(dir, "first.txt"), "x")
	c := sh(t, wipSnapshotScript(dir, wipRef))
	if n := gitOut(t, dir, "rev-list", "--count", c); n != "1" {
		t.Fatalf("got %s commits", n)
	}
	if a := gitOut(t, dir, "log", "-1", "--format=%an", c); a != "onctl" {
		t.Errorf("no identity: author %q, want onctl", a)
	}
}

// TestProjectCheck runs the box-side project check for real.
func TestProjectCheck(t *testing.T) {
	root := t.TempDir()
	marker := filepath.Join(root, "boxctl", "project")
	proj := filepath.Join(root, "work", "my proj")

	if got := sh(t, projectCheckScriptAt(marker, proj)); got != "copy" {
		t.Fatalf("empty box: %q", got)
	}
	// After a copy (or on a box from before the marker): recorded.
	if err := os.MkdirAll(proj, 0o755); err != nil {
		t.Fatal(err)
	}
	if got := sh(t, projectCheckScriptAt(marker, proj)); got != "ok" {
		t.Fatalf("project there: %q", got)
	}
	if b, _ := os.ReadFile(marker); string(b) != proj {
		t.Fatalf("marker = %q", b)
	}
	other := filepath.Join(root, "elsewhere")
	if got := sh(t, projectCheckScriptAt(marker, other)); got != "other "+proj {
		t.Fatalf("another project: %q", got)
	}
}

func TestGitSSHCommandLeavesTheHostToGit(t *testing.T) {
	b := &boxSSH{name: "x", args: []string{"-i", "/k ey", "-o", "ProxyCommand='/b' ssh-proxy 'x'", "root@x"}}
	got := b.gitSSHCommand()
	if strings.Contains(got, "root@x") {
		t.Fatalf("%q includes the destination; git adds it from the URL", got)
	}
	// It must survive the shell git runs it through.
	out := sh(t, "set -- "+strings.TrimPrefix(got, "ssh ")+`; printf '[%s]' "$@"`)
	want := `[-o][BatchMode=yes][-i][/k ey][-o][ProxyCommand='/b' ssh-proxy 'x']`
	if out != want {
		t.Fatalf("got %s, want %s", out, want)
	}
}

func TestBoxRemoteURL(t *testing.T) {
	if got := boxRemoteURL("claude-web", "/Users/me/web"); got != "root@claude-web.box:/Users/me/web" {
		t.Fatal(got)
	}
	if sshProxyHost("claude-web.box") != "claude-web" {
		t.Fatal("ssh-proxy must strip the .box the remote URL adds")
	}
}

func TestClaudeSubcommands(t *testing.T) {
	for _, name := range []string{"fetch", "push"} {
		c, _, err := rootCmd.Find([]string{"claude", name})
		if err != nil || c.Name() != name {
			t.Errorf("onctl claude %s: %v %v", name, c, err)
		}
	}
}

func TestClaudeProjectKey(t *testing.T) {
	for dir, want := range map[string]string{
		"/Users/cd/cdalar/boxctl-vms":                  "-Users-cd-cdalar-boxctl-vms",
		"/Users/cd/.herdr/worktrees/boxctl-vms/wt-1a2": "-Users-cd--herdr-worktrees-boxctl-vms-wt-1a2",
		"/home/me/my_proj v2":                          "-home-me-my-proj-v2",
	} {
		if got := claudeProjectKey(dir); got != want {
			t.Errorf("claudeProjectKey(%q) = %q, want %q", dir, got, want)
		}
	}
}

func TestFindTranscript(t *testing.T) {
	projects := t.TempDir()
	id := "e62ceefd-b2d9-4e12-8261-b114765d7977"
	write(t, filepath.Join(projects, "-Users-me-app", id+".jsonl"), "{}\n")
	write(t, filepath.Join(projects, "-Users-me-app-sub", "other.jsonl"), "{}\n")
	got, err := findTranscript(projects, id)
	if err != nil || got != filepath.Join(projects, "-Users-me-app", id+".jsonl") {
		t.Fatalf("got %q, %v", got, err)
	}
	if _, err := findTranscript(projects, "11111111-2222-3333-4444-555555555555"); err == nil {
		t.Error("found a session that doesn't exist")
	}
	for _, bad := range []string{"", "../../etc/passwd", "*", "a/b", "short"} {
		if _, err := findTranscript(projects, bad); err == nil || !strings.Contains(err.Error(), "isn't a Claude Code session ID") {
			t.Errorf("%q: %v", bad, err)
		}
	}
}

func TestProjectDirFlag(t *testing.T) {
	t.Setenv("GIT_CONFIG_GLOBAL", os.DevNull)
	repo := t.TempDir()
	run(t, repo, "git", "init", "-q")
	sub := filepath.Join(repo, "a", "b")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	prev := claudeProject
	t.Cleanup(func() { claudeProject = prev })

	claudeProject = sub
	got, err := projectDir()
	want, _ := filepath.EvalSymlinks(repo)
	if gotReal, _ := filepath.EvalSymlinks(got); err != nil || gotReal != want {
		t.Fatalf("--project in a subdirectory: got %q, %v; want the repository, %q", got, err, want)
	}
	plain := t.TempDir()
	claudeProject = plain
	if got, err := projectDir(); err != nil || got != plain {
		t.Fatalf("outside git: got %q, %v", got, err)
	}
	claudeProject = filepath.Join(plain, "missing")
	if _, err := projectDir(); err == nil {
		t.Fatal("a missing --project was accepted")
	}
}
