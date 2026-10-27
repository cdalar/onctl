package cmd

import (
	"context"
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
	if main.dir != "/w/app" || main.name != "claude" || main.ghSocket != boxGitHubSocket {
		t.Errorf("main: %+v", main)
	}
	if main.attachHint("claude-app") != "onctl claude --box claude-app" {
		t.Error(main.attachHint("claude-app"))
	}
	auth := newTaskSession("/w/app", "auth")
	if auth.dir != "/w/app@auth" || auth.name != "claude-auth" || auth.ghSocket != "/root/.boxctl/gh-auth.sock" {
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

// noSavedClaudeToken makes loadClaudeToken find nothing: no token in the
// environment, an empty HOME, and (on macOS) a Keychain that has no entry
// -- a stand-in security, so the real Keychain is never read.
func noSavedClaudeToken(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("CLAUDE_CODE_OAUTH_TOKEN", "")
	bin := t.TempDir()
	write(t, filepath.Join(bin, "security"), "#!/bin/sh\nexit 44\n")
	if err := os.Chmod(filepath.Join(bin, "security"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+":/usr/bin:/bin")
	return home
}

// --claude-auth token is a promise the box starts logged in, so with no
// token it must fail here, not drop into /login on the box; auto is
// allowed to fall back to /login (no env, no error).
func TestClaudeEnvWithoutSavedToken(t *testing.T) {
	noSavedClaudeToken(t)
	if env, _, err := claudeEnv(claudeAuthToken); err == nil || env != "" || !strings.Contains(err.Error(), "onctl claude login") {
		t.Errorf("token mode with no token: env %q err %v", env, err)
	}
	if env, key, err := claudeEnv(claudeAuthAuto); err != nil || env != "" || key != "" {
		t.Errorf("auto with no token should fall back to /login: %q %q %v", env, key, err)
	}
}

// The token `onctl claude login` saved (here: its file) is what token
// mode hands the box -- not only one in the environment.
func TestClaudeEnvUsesSavedToken(t *testing.T) {
	noSavedClaudeToken(t)
	p, err := claudeTokenPath()
	if err != nil {
		t.Fatal(err)
	}
	write(t, p, "sk-ant-oat01-saved\n")
	env, key, err := claudeEnv(claudeAuthToken)
	if err != nil || key != "" || env != "export CLAUDE_CODE_OAUTH_TOKEN='sk-ant-oat01-saved'\n" {
		t.Errorf("token mode with a saved token: %q %q %v", env, key, err)
	}
}

// The env file is sourced by sh on the box, so a key must arrive exactly
// -- trimmed of the newline a pasted export carries, and with shell
// characters inert -- and the key returned for pre-approval must be the
// same one Claude sees, or its suffix won't match and Claude asks again.
func TestClaudeEnvAPIKeyIsSourcedExactly(t *testing.T) {
	raw := "sk-ant-api03-it's$HOME`x`-abcdefghijklmnop"
	t.Setenv("ANTHROPIC_API_KEY", "  "+raw+"\n")
	env, key, err := claudeEnv(claudeAuthAPIKey)
	if err != nil {
		t.Fatal(err)
	}
	if key != raw {
		t.Errorf("returned key %q, want %q", key, raw)
	}
	c := exec.Command("sh", "-c", env+`printf %s "$ANTHROPIC_API_KEY"`)
	c.Env = []string{"PATH=/usr/bin:/bin", "HOME=/nonexistent"}
	out, err := c.Output()
	if err != nil {
		t.Fatal(err)
	}
	if string(out) != raw {
		t.Errorf("claude would see %q, want %q", out, raw)
	}
	t.Setenv("ANTHROPIC_API_KEY", "   \n")
	if _, _, err := claudeEnv(claudeAuthAPIKey); err == nil {
		t.Error("a whitespace-only ANTHROPIC_API_KEY was accepted")
	}
}

// Claude Code files an approved key under its last 20 characters; any
// other length and the approval never matches.
func TestAPIKeySuffix(t *testing.T) {
	long := "sk-ant-api03-" + strings.Repeat("x", 10) + "ABCDEFGHIJKLMNOPQRST"
	for in, want := range map[string]string{
		long:                   "ABCDEFGHIJKLMNOPQRST",
		"ABCDEFGHIJKLMNOPQRST": "ABCDEFGHIJKLMNOPQRST",
		"short":                "short",
		"":                     "",
	} {
		if got := apiKeySuffix(in); got != want {
			t.Errorf("apiKeySuffix(%q) = %q, want %q", in, got, want)
		}
	}
}

// fakeSSH puts an ssh on PATH that logs its arguments and runs the remote
// command locally with sh, stdin passed through.
func fakeSSH(t *testing.T) (argsLog string) {
	t.Helper()
	bin := t.TempDir()
	argsLog = filepath.Join(t.TempDir(), "ssh-args")
	write(t, filepath.Join(bin, "ssh"), `#!/bin/sh
printf '%s\n' "$@" >>`+shellQuote(argsLog)+`
for a; do last=$a; done
exec sh -c "$last"
`)
	if err := os.Chmod(filepath.Join(bin, "ssh"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+":/usr/bin:/bin")
	return argsLog
}

// stageClaudeEnv must land the secrets exactly where the session's
// command reads them, readable by root alone, and send them over stdin:
// on ssh's command line any user on either machine could read them in ps.
func TestStageClaudeEnv(t *testing.T) {
	argsLog := fakeSSH(t)
	_, runDir := boxPaths(t)
	box := &boxSSH{name: "b", args: []string{"root@b"}}
	s := newTaskSession("/w/app", "auth")
	env := "export CLAUDE_CODE_OAUTH_TOKEN='sk-ant-oat01-secret'\n"

	if err := stageClaudeEnv(context.Background(), box, s, env); err != nil {
		t.Fatal(err)
	}
	if s.envFile() != filepath.Join(runDir, "claude-auth.env") {
		t.Fatalf("env file %s is not in the run dir %s", s.envFile(), runDir)
	}
	got, err := os.ReadFile(s.envFile())
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != env {
		t.Errorf("staged %q, want %q", got, env)
	}
	fi, err := os.Stat(s.envFile())
	if err != nil {
		t.Fatal(err)
	}
	if fi.Mode().Perm() != 0o600 {
		t.Errorf("env file mode %v, want 0600", fi.Mode().Perm())
	}
	if args, _ := os.ReadFile(argsLog); strings.Contains(string(args), "sk-ant") {
		t.Errorf("the secret is on ssh's command line: %s", args)
	}

	// Login mode has nothing to stage: no file, and no round trip to the box.
	if err := os.Remove(argsLog); err != nil {
		t.Fatal(err)
	}
	main := newTaskSession("/w/app", "")
	if err := stageClaudeEnv(context.Background(), box, main, ""); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(argsLog); !os.IsNotExist(err) {
		t.Error("an empty env still ran ssh")
	}
	if _, err := os.Stat(main.envFile()); !os.IsNotExist(err) {
		t.Error("an empty env left a file")
	}
}

// Without a local theme or an API key, the script must leave the box's
// own choices alone -- not blank its theme or touch its approved keys --
// and it must work on a box whose ~/.claude.json doesn't exist yet.
// Approving the same key on every start must not grow the list.
func TestClaudeStateScriptKeepsBoxState(t *testing.T) {
	if _, err := exec.LookPath("jq"); err != nil {
		t.Skip("no jq")
	}
	runScript := func(home, script string) map[string]any {
		t.Helper()
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
			t.Fatalf("%v: %s", err, data)
		}
		return got
	}

	// A fresh box: no file at all; a directory with shell and jq metacharacters.
	dir := `/w/it's "my" $app`
	got := runScript(t.TempDir(), claudeStateScript([]string{dir}, "", ""))
	if got["hasCompletedOnboarding"] != true || got["projects"].(map[string]any)[dir].(map[string]any)["hasTrustDialogAccepted"] != true {
		t.Errorf("fresh box: %v", got)
	}
	if _, ok := got["theme"]; ok {
		t.Errorf("no local theme still set one: %v", got["theme"])
	}

	home := t.TempDir()
	write(t, filepath.Join(home, ".claude.json"), `{"theme":"light","customApiKeyResponses":{"approved":["OTHERKEYSUFFIX"]}}`)
	got = runScript(home, claudeStateScript([]string{"/w/app"}, "", ""))
	if got["theme"] != "light" {
		t.Errorf("box theme overwritten: %v", got["theme"])
	}
	if !reflect.DeepEqual(got["customApiKeyResponses"], map[string]any{"approved": []any{"OTHERKEYSUFFIX"}}) {
		t.Errorf("approved keys touched without a key: %v", got["customApiKeyResponses"])
	}

	key := "sk-ant-api03-abcdefghijklmnopqrstuvwxyz"
	runScript(home, claudeStateScript([]string{"/w/app"}, "", key))
	got = runScript(home, claudeStateScript([]string{"/w/app"}, "", key))
	approved := got["customApiKeyResponses"].(map[string]any)["approved"].([]any)
	if !reflect.DeepEqual(approved, []any{"OTHERKEYSUFFIX", apiKeySuffix(key)}) {
		t.Errorf("approved after two starts: %v", approved)
	}
}

// A task name goes into a git branch, a worktree path, a socket path and
// -- unquoted -- the worktree script's refs/heads/ check, so the pattern
// is the only thing keeping it a plain, short, branch-safe word.
func TestTaskNamePattern(t *testing.T) {
	for _, ok := range []string{"auth", "fix-ci", "a", "9lives", strings.Repeat("a", 30)} {
		if !taskNamePattern.MatchString(ok) {
			t.Errorf("%q rejected", ok)
		}
	}
	for _, bad := range []string{"", "-auth", "Auth", "fix_ci", "a b", "a/b", "a;rm -rf", "a..b", "a\nb", strings.Repeat("a", 31)} {
		if taskNamePattern.MatchString(bad) {
			t.Errorf("%q accepted", bad)
		}
	}
}

// A task needs a git repository to branch from: outside one the script
// must stop with a clear error and make nothing, not leave a stray
// <dir>@<task> directory behind.
func TestWorktreeScriptNeedsGit(t *testing.T) {
	root := t.TempDir()
	proj := filepath.Join(root, "app")
	if err := os.MkdirAll(proj, 0o755); err != nil {
		t.Fatal(err)
	}
	out, err := exec.Command("sh", "-c", worktreeScript(proj, "auth")).CombinedOutput()
	if err == nil {
		t.Fatalf("worked outside a git repository: %s", out)
	}
	if !strings.Contains(string(out), "tasks need a git repository") {
		t.Errorf("error %q", out)
	}
	if _, err := os.Stat(taskDir(proj, "auth")); !os.IsNotExist(err) {
		t.Error("a worktree directory was made anyway")
	}
}

// Project paths come from the local machine, where spaces are common;
// the script must quote them everywhere or git gets split arguments.
func TestWorktreeScriptProjectWithSpace(t *testing.T) {
	t.Setenv("GIT_CONFIG_GLOBAL", os.DevNull)
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	proj := filepath.Join(root, "my app")
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
	if _, err := os.Stat(filepath.Join(root, "my")); !os.IsNotExist(err) {
		t.Error("the path was split at the space")
	}
}
