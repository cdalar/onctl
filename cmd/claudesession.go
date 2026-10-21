package cmd

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// A Claude session on a box: the project's main checkout, or a task --
// a git worktree of its own beside it (`--task auth` -> <dir>@auth, on
// branch auth), so several Claudes can work on one project at once
// without touching each other's files. Each runs in its own tmux session
// and gets its own GitHub socket, so attaching to one never takes the
// other's forwarding away.

var taskNamePattern = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,29}$`)

type taskSession struct {
	task     string // "" for the main checkout
	dir      string // where Claude runs on the box
	tmux     string // its tmux session
	ghSocket string // where its GitHub forwarding lands on the box
}

func newTaskSession(project, task string) taskSession {
	if task == "" {
		return taskSession{dir: project, tmux: claudeSession, ghSocket: boxGitHubSocket}
	}
	return taskSession{
		task:     task,
		dir:      taskDir(project, task),
		tmux:     claudeSession + "-" + task,
		ghSocket: "/root/.boxctl/gh-" + task + ".sock",
	}
}

// taskDir is where a task's worktree lives on the box: beside the
// project, not inside it, so it never shows up as untracked files there.
func taskDir(project, task string) string { return project + "@" + task }

// attachHint is the command that comes back to this session.
func (s taskSession) attachHint(box string) string {
	hint := "onctl claude --box " + box
	if s.task != "" {
		hint += " --task " + s.task
	}
	return hint
}

// envFile is where this session's secrets wait for Claude to start: in
// /run, a tmpfs -- memory, never the box's disk -- read and deleted by the
// session's command the moment it runs.
func (s taskSession) envFile() string { return "/run/boxctl/" + s.tmux + ".env" }

// commandLine is the shell command tmux runs for this session: pick up
// and delete the secrets, point the GitHub helpers at this session's
// socket, then claude with the prompt and arguments.
func (s taskSession) commandLine(prompt string, args []string) string {
	f := shellQuote(s.envFile())
	words := []string{
		"if [ -f " + f + " ]; then . " + f + "; rm -f " + f + "; fi;",
		"export BOXCTL_GH_SOCKET=" + shellQuote(s.ghSocket) + ";",
		`PATH="$HOME/.local/bin:$PATH"`, "exec", "claude",
	}
	for _, a := range args {
		words = append(words, shellQuote(a))
	}
	if prompt != "" {
		words = append(words, shellQuote(prompt))
	}
	return strings.Join(words, " ")
}

// tmuxCommand starts this session's Claude in tmux, or with -A attaches
// to it if it's already running. -u: the box has no locale set, and
// Claude's interface is Unicode.
func (s taskSession) tmuxCommand(claudeLine string, detached bool) string {
	mode := "-A"
	if detached {
		mode = "-d"
	}
	return fmt.Sprintf("tmux -u new-session %s -s %s -c %s %s",
		mode, s.tmux, shellQuote(s.dir), shellQuote(claudeLine))
}

// worktreeScript makes the task's worktree if it isn't there: on branch
// <task>, new from what the box's main checkout has, or the existing
// branch of that name.
func worktreeScript(project, task string) string {
	p, t := shellQuote(taskDir(project, task)), shellQuote(task)
	return `set -e
cd ` + shellQuote(project) + `
[ -d .git ] || { echo "tasks need a git repository" >&2; exit 1; }
if [ ! -d ` + p + ` ]; then
  if git show-ref --verify -q refs/heads/` + task + `; then git worktree add -q ` + p + ` ` + t + `
  else git worktree add -q -b ` + t + ` ` + p + `; fi
  echo "Made worktree ` + taskDir(project, task) + ` on branch ` + task + `" >&2
fi`
}

// --- Claude credentials ---------------------------------------------------

// Claude auth modes, by --claude-auth.
const (
	claudeAuthAuto   = "auto"    // token if one is saved, else login
	claudeAuthToken  = "token"   // the token `onctl claude login` saved
	claudeAuthLogin  = "login"   // Claude's own /login on the box
	claudeAuthAPIKey = "api-key" // ANTHROPIC_API_KEY from here
)

// claudeEnv is what this session's Claude gets in its environment, as a
// sourceable file, plus the API key it was given (for pre-approving it),
// or nothing for login mode.
func claudeEnv(mode string) (env, apiKey string, err error) {
	switch mode {
	case claudeAuthAuto, claudeAuthToken:
		token, err := loadClaudeToken()
		if err != nil {
			return "", "", err
		}
		if token == "" {
			if mode == claudeAuthToken {
				return "", "", errors.New("no saved Claude token -- run onctl claude login first")
			}
			fmt.Fprintln(os.Stderr, "No saved Claude token, so log in on the box with /login. Run onctl claude login once to skip that on every box.")
			return "", "", nil
		}
		return "export CLAUDE_CODE_OAUTH_TOKEN=" + shellQuote(token) + "\n", "", nil
	case claudeAuthAPIKey:
		key := strings.TrimSpace(os.Getenv("ANTHROPIC_API_KEY"))
		if key == "" {
			return "", "", errors.New("--claude-auth api-key needs ANTHROPIC_API_KEY set here")
		}
		return "export ANTHROPIC_API_KEY=" + shellQuote(key) + "\n", key, nil
	case claudeAuthLogin:
		return "", "", nil
	}
	return "", "", fmt.Errorf("--claude-auth must be %s, %s, %s or %s", claudeAuthAuto, claudeAuthToken, claudeAuthLogin, claudeAuthAPIKey)
}

// stageClaudeEnv puts env where s's command picks it up, sending it over
// ssh's stdin -- never on a command line, where ps would show it.
func stageClaudeEnv(ctx context.Context, box *boxSSH, s taskSession, env string) error {
	if env == "" {
		return nil
	}
	return box.run(ctx, "umask 077 && mkdir -p /run/boxctl && cat >"+shellQuote(s.envFile()), strings.NewReader(env))
}

// claudeStateScript marks what a person would otherwise click through on
// the box's first run: onboarding (with your theme from here), trusting
// the session's directory -- your own project -- and, for an API key,
// approving it. Merged into the box's ~/.claude.json, so Claude's own
// state there is kept.
func claudeStateScript(dirs []string, theme, apiKey string) string {
	filter := `.hasCompletedOnboarding = true`
	if theme != "" {
		filter += ` | .theme = $theme`
	}
	for i := range dirs {
		filter += fmt.Sprintf(` | .projects[$d%d].hasTrustDialogAccepted = true`, i)
	}
	if apiKey != "" {
		filter += ` | .customApiKeyResponses.approved = (((.customApiKeyResponses.approved // []) + [$key]) | unique)`
	}
	args := []string{"--arg theme " + shellQuote(theme), "--arg key " + shellQuote(apiKeySuffix(apiKey))}
	for i, d := range dirs {
		args = append(args, fmt.Sprintf("--arg d%d %s", i, shellQuote(d)))
	}
	return `command -v jq >/dev/null || exit 0
f="$HOME/.claude.json"; [ -s "$f" ] || echo '{}' >"$f"
jq ` + strings.Join(args, " ") + ` ` + shellQuote(filter) + ` "$f" >"$f.tmp" && mv "$f.tmp" "$f"`
}

// apiKeySuffix is how Claude Code remembers an approved API key: its last
// 20 characters, not the key.
func apiKeySuffix(key string) string {
	if len(key) > 20 {
		return key[len(key)-20:]
	}
	return key
}

// localClaudeTheme is the theme picked here, from ~/.claude.json, or "".
func localClaudeTheme() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	data, err := os.ReadFile(filepath.Join(home, ".claude.json"))
	if err != nil {
		return ""
	}
	var cfg struct {
		Theme string `json:"theme"`
	}
	if json.Unmarshal(data, &cfg) != nil {
		return ""
	}
	return cfg.Theme
}
