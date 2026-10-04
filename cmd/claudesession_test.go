package cmd

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestTaskSession(t *testing.T) {
	main := newTaskSession("/w/app", "")
	if main.dir != "/w/app" || main.tmux != "claude" || main.ghSocket != boxGitHubSocket {
		t.Errorf("main: %+v", main)
	}
	if main.attachHint("claude-app") != "onctl claude --box claude-app" {
		t.Error(main.attachHint("claude-app"))
	}
	auth := newTaskSession("/w/app", "auth")
	if auth.dir != "/w/app@auth" || auth.tmux != "claude-auth" || auth.ghSocket != "/root/.boxctl/gh-auth.sock" {
		t.Errorf("task: %+v", auth)
	}
	if auth.attachHint("claude-app") != "onctl claude --box claude-app --task auth" {
		t.Error(auth.attachHint("claude-app"))
	}
	if auth.envFile() == main.envFile() || !strings.HasPrefix(auth.envFile(), "/run/") {
		t.Errorf("env files: %s %s", main.envFile(), auth.envFile())
	}
}

// TestSessionCommandTakesItsSecrets runs a session's command for real: it
// must source the staged env file, delete it, point the GitHub helpers at
// its socket, and run claude with the token in its environment -- never
// in its arguments.
func TestSessionCommandTakesItsSecrets(t *testing.T) {
	root := t.TempDir()
	s := newTaskSession("/w/app", "auth")
	envFile := filepath.Join(root, "claude-auth.env")
	line := strings.ReplaceAll(s.commandLine("", nil), shellQuote(s.envFile()), shellQuote(envFile))
	write(t, envFile, "export CLAUDE_CODE_OAUTH_TOKEN="+shellQuote("sk-ant-oat01-secret")+"\n")

	bin := t.TempDir()
	write(t, filepath.Join(bin, "claude"), `#!/bin/sh
echo "token=$CLAUDE_CODE_OAUTH_TOKEN socket=$BOXCTL_GH_SOCKET args=$*"
`)
	if err := os.Chmod(filepath.Join(bin, "claude"), 0o755); err != nil {
		t.Fatal(err)
	}
	c := exec.Command("sh", "-c", line)
	c.Env = []string{"HOME=" + root, "PATH=" + bin + ":/usr/bin:/bin"}
	out, err := c.Output()
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.TrimSpace(string(out)); got != "token=sk-ant-oat01-secret socket=/root/.boxctl/gh-auth.sock args=" {
		t.Errorf("claude saw %q", got)
	}
	if _, err := os.Stat(envFile); !os.IsNotExist(err) {
		t.Error("the env file outlived the start")
	}
	if strings.Contains(line, "sk-ant") {
		t.Error("the token is on the command line")
	}
	// Restarting without a staged file is fine: no secrets, no error.
	if err := exec.Command("sh", "-c", line).Run(); err != nil {
		t.Logf("(claude itself missing on a plain PATH is fine): %v", err)
	}
}

func TestClaudeEnv(t *testing.T) {
	t.Setenv("CLAUDE_CODE_OAUTH_TOKEN", "sk-ant-oat01-fromenv-xxxxxxxxxxxx")
	env, key, err := claudeEnv(claudeAuthAuto)
	if err != nil || key != "" || env != "export CLAUDE_CODE_OAUTH_TOKEN='sk-ant-oat01-fromenv-xxxxxxxxxxxx'\n" {
		t.Fatalf("auto with a token here: %q %q %v", env, key, err)
	}
	if env, _, _ := claudeEnv(claudeAuthLogin); env != "" {
		t.Error("login mode passed a credential")
	}
	t.Setenv("ANTHROPIC_API_KEY", "")
	if _, _, err := claudeEnv(claudeAuthAPIKey); err == nil {
		t.Error("api-key without ANTHROPIC_API_KEY")
	}
	t.Setenv("ANTHROPIC_API_KEY", "sk-ant-api03-abcdefghijklmnopqrstuvwxyz")
	env, key, err = claudeEnv(claudeAuthAPIKey)
	if err != nil || key == "" || !strings.HasPrefix(env, "export ANTHROPIC_API_KEY=") {
		t.Fatalf("api-key: %q %v", env, err)
	}
	if _, _, err := claudeEnv("bogus"); err == nil {
		t.Error("unknown mode accepted")
	}
}

func TestClaudeStateScript(t *testing.T) {
	if _, err := exec.LookPath("jq"); err != nil {
		t.Skip("no jq")
	}
	home := t.TempDir()
	write(t, filepath.Join(home, ".claude.json"), `{"numStartups":3,"projects":{"/other":{"x":1}}}`)
	script := claudeStateScript([]string{"/w/app", "/w/app@auth"}, "dark", "sk-ant-api03-abcdefghijklmnopqrstuvwxyz")
	c := exec.Command("sh", "-c", script)
	c.Env = append(os.Environ(), "HOME="+home)
	if out, err := c.CombinedOutput(); err != nil {
		t.Fatalf("%v: %s", err, out)
	}
	data, err := os.ReadFile(filepath.Join(home, ".claude.json"))
	if err != nil {
		t.Fatal(err)
	}
	var got map[string]any
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatal(err)
	}
	projects := got["projects"].(map[string]any)
	approved := got["customApiKeyResponses"].(map[string]any)["approved"].([]any)
	if got["hasCompletedOnboarding"] != true || got["theme"] != "dark" || got["numStartups"] != float64(3) ||
		projects["/w/app"].(map[string]any)["hasTrustDialogAccepted"] != true ||
		projects["/w/app@auth"].(map[string]any)["hasTrustDialogAccepted"] != true ||
		projects["/other"] == nil ||
		!reflect.DeepEqual(approved, []any{"defghijklmnopqrstuvwxyz"[3:]}) && !reflect.DeepEqual(approved, []any{apiKeySuffix("sk-ant-api03-abcdefghijklmnopqrstuvwxyz")}) {
		t.Fatalf("got %s", data)
	}
	if strings.Contains(string(data), "sk-ant-api03") {
		t.Error("the API key itself was stored")
	}
}

func TestWorktreeScript(t *testing.T) {
	t.Setenv("GIT_CONFIG_GLOBAL", os.DevNull)
	// git reports real paths; macOS's temp dir is behind a symlink.
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	proj := filepath.Join(root, "app")
	if err := os.MkdirAll(proj, 0o755); err != nil {
		t.Fatal(err)
	}
	run(t, proj, "git", "init", "-q", "-b", "main")
	write(t, filepath.Join(proj, "a.txt"), "a")
	run(t, proj, "git", "add", "-A")
	run(t, proj, "git", "-c", "user.email=t@t", "-c", "user.name=t", "commit", "-qm", "init")

	sh(t, worktreeScript(proj, "auth"))
	if gitOut(t, taskDir(proj, "auth"), "branch", "--show-current") != "auth" {
		t.Fatal("task worktree isn't on branch auth")
	}
	sh(t, worktreeScript(proj, "auth")) // again: already there, no error
	// An existing branch is checked out, not recreated.
	run(t, proj, "git", "branch", "billing")
	sh(t, worktreeScript(proj, "billing"))
	if gitOut(t, taskDir(proj, "billing"), "branch", "--show-current") != "billing" {
		t.Fatal("billing worktree")
	}

	ws := parseWorktrees(gitOut(t, proj, "worktree", "list", "--porcelain"), proj)
	var tasks []string
	for _, w := range ws {
		tasks = append(tasks, w.task+"="+w.wipName())
	}
	if want := []string{"=wip", "auth=wip-auth", "billing=wip-billing"}; !reflect.DeepEqual(tasks, want) {
		t.Fatalf("worktrees %v, want %v", tasks, want)
	}
	branches := worktreeBranches(gitOut(t, proj, "worktree", "list", "--porcelain"))
	if branches[taskDir(proj, "auth")] != "auth" {
		t.Errorf("branches %v", branches)
	}
}

func TestResolveTask(t *testing.T) {
	prevTask, prevOwn, prevBox, prevProj := claudeTask, claudeOwnBox, claudeBox, claudeProject
	t.Cleanup(func() { claudeTask, claudeOwnBox, claudeBox, claudeProject = prevTask, prevOwn, prevBox, prevProj })

	claudeProject = t.TempDir()
	claudeTask, claudeOwnBox, claudeBox = "Bad_Name", false, ""
	if resolveTask() == nil {
		t.Error("an invalid task name was accepted")
	}
	claudeTask, claudeOwnBox = "", true
	if resolveTask() == nil {
		t.Error("--own-box without --task was accepted")
	}
	claudeTask, claudeOwnBox, claudeBox = "auth", true, ""
	if err := resolveTask(); err != nil {
		t.Fatal(err)
	}
	if claudeTask != "" || !strings.HasSuffix(claudeBox, "-auth") || !strings.HasPrefix(claudeBox, "claude-") {
		t.Errorf("own box: task %q box %q", claudeTask, claudeBox)
	}
}
