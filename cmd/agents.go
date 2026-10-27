package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"
	"time"
)

// Claude sessions on boxes, for the tui's :agents view (tuiagents.go): a
// proof of concept of working the way herdr does, each session a box of
// its own. A new session is `onctl claude --repo` detached on a new box,
// in herdr (whose agent status says when Claude is waiting) with a
// GitHub token stored there (so Claude can open the PR it's asked to
// open when it's done). The view polls every box over ssh, and a desktop
// notification goes out when a Claude starts waiting for you. On a
// provider other than boxes only the VMs started from the view are
// polled: connecting to a VM gives root ssh on it (connectClaudeVM),
// which isn't for VMs onctl claude never touched.

// agentsPRInstruction is added to every task: making the PR is the
// point of the session, and writing its title and body is Claude's to do.
const agentsPRInstruction = "\n\nWhen the task is done: commit your work on the current branch, push it to origin, and open a pull request with `gh pr create`, with a clear title and a body that says what changed and why. Then reply with the pull request's URL."

// agentSession is a box with a Claude project on it, as the list shows it.
type agentSession struct {
	box, repo, dir, branch, mux, status, pr, err string
}

// waiting reports whether Claude needs you: herdr's blocked (asking for
// a permission, a question) or done (finished its turn).
func (s agentSession) waiting() bool { return s.status == "blocked" || s.status == "done" }

// newlyWaiting is the sessions that started waiting since the last poll,
// whose statuses were prev (nil on the first poll: nothing is new then,
// or every waiting session would notify each time the view opens).
func newlyWaiting(prev map[string]string, now []agentSession) []agentSession {
	if prev == nil {
		return nil
	}
	var out []agentSession
	for _, s := range now {
		if was, seen := prev[s.box]; s.waiting() && (!seen || was != s.status) {
			out = append(out, s)
		}
	}
	return out
}

// agentStatusScript prints what the list shows of the box's project as
// key=value lines, or nothing on a box without one.
func agentStatusScript() string {
	return `d=$(cat ` + projectMarker + ` 2>/dev/null); [ -n "$d" ] || exit 0
gh=/usr/bin/gh; [ -x "$gh" ] || gh=gh
echo "dir=$d"
echo "mux=$(cat ` + muxMarker + ` 2>/dev/null)"
echo "origin=$(git -C "$d" remote get-url origin 2>/dev/null)"
echo "branch=$(git -C "$d" branch --show-current 2>/dev/null)"
echo "pr=$(cd "$d" && timeout 5 "$gh" pr view --json url -q .url 2>/dev/null)"
echo "herdr=$(herdr workspace list 2>/dev/null | tr -d '\n')"`
}

// parseAgentStatus reads agentStatusScript's output for the box named
// box; ok is false when the box holds no project.
func parseAgentStatus(box, out string) (s agentSession, ok bool) {
	kv := map[string]string{}
	for _, line := range strings.Split(out, "\n") {
		if k, v, found := strings.Cut(line, "="); found {
			kv[k] = strings.TrimSpace(v)
		}
	}
	if kv["dir"] == "" {
		return s, false
	}
	s = agentSession{box: box, dir: kv["dir"], branch: kv["branch"], mux: kv["mux"], pr: kv["pr"], repo: originRepo(kv["origin"])}
	switch s.mux {
	case "herdr":
		s.status = herdrMux{}.parseList(kv["herdr"])[claudeSession]
		if s.status == "" {
			s.status = "stopped"
		}
	case "":
		s.status = "not started"
	default:
		s.status = s.mux // tmux tells nothing of what Claude is doing
	}
	return s, true
}

// originRepo is a GitHub remote URL as owner/name, or the URL itself.
func originRepo(url string) string {
	for _, prefix := range []string{"git@github.com:", "ssh://git@github.com/", "https://github.com/"} {
		if rest, ok := strings.CutPrefix(url, prefix); ok {
			return strings.TrimSuffix(rest, ".git")
		}
	}
	return url
}

// --- boxes started here, and connections -------------------------------------

// agentsRegistryPath, ~/.onctl/agents.json, lists the boxes started from
// this view (box name -> repo). A variable for tests.
var agentsRegistryPath = func() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".onctl", "agents.json"), nil
}

func readAgentsRegistry() map[string]string {
	reg := map[string]string{}
	if p, err := agentsRegistryPath(); err == nil {
		if data, err := os.ReadFile(p); err == nil {
			_ = json.Unmarshal(data, &reg)
		}
	}
	return reg
}

func addToAgentsRegistry(box, repo string) error {
	p, err := agentsRegistryPath()
	if err != nil {
		return err
	}
	reg := readAgentsRegistry()
	reg[box] = repo
	data, err := json.Marshal(reg)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(p), 0o700); err != nil {
		return err
	}
	return os.WriteFile(p, data, 0o600)
}

// agentConns keeps one ssh connection per box for the view's life: the
// provider and key setup that makes it is a few API calls, and the
// connection is a control master that makes each poll cheap. A box that
// couldn't be reached is remembered (nil) and not tried again.
var agentConns = struct {
	sync.Mutex
	m map[string]*boxSSH
}{m: map[string]*boxSSH{}}

func agentConn(ctx context.Context, name string) *boxSSH {
	agentConns.Lock()
	b, tried := agentConns.m[name]
	agentConns.Unlock()
	if tried {
		return b
	}
	var err error
	if onBoxes() {
		b, err = connectClaudeBox(ctx, name, false, false)
	} else {
		b, _, err = connectClaudeVM(ctx, name, false)
	}
	if err != nil {
		b = nil
	}
	agentConns.Lock()
	agentConns.m[name] = b
	agentConns.Unlock()
	return b
}

// pollAgents lists the sessions: the provider's running VMs (on another
// provider than boxes, those started here) that hold a project.
func pollAgents() ([]agentSession, error) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	vms, err := listClaudeVMs(ctx)
	if err != nil {
		return nil, err
	}
	reg := readAgentsRegistry()
	var (
		mu  sync.Mutex
		wg  sync.WaitGroup
		out []agentSession
	)
	for _, vm := range vms {
		if _, ours := reg[vm.name]; vm.state != "running" || (!onBoxes() && !ours) {
			continue
		}
		wg.Add(1)
		go func(name string) {
			defer wg.Done()
			b := agentConn(ctx, name)
			if b == nil {
				return
			}
			raw, err := b.output(ctx, agentStatusScript())
			s, ok := parseAgentStatus(name, string(raw))
			if err != nil {
				s, ok = agentSession{box: name, repo: reg[name], err: lastLine(err.Error())}, true
			}
			if !ok {
				return
			}
			mu.Lock()
			out = append(out, s)
			mu.Unlock()
		}(vm.name)
	}
	wg.Wait()
	sort.Slice(out, func(i, j int) bool { return out[i].box < out[j].box })
	return out, nil
}

// notifyDesktop shows a desktop notification. A variable for tests.
var notifyDesktop = func(title, body string) {
	switch runtime.GOOS {
	case "darwin":
		_ = exec.Command("osascript", "-e", fmt.Sprintf("display notification %q with title %q sound name \"Glass\"", body, title)).Run()
	case "linux":
		_ = exec.Command("notify-send", title, body).Run()
	}
}

// agentsStartArgs is the `onctl claude` command line that starts a
// session: repo cloned on the new box, Claude in herdr (for its status),
// the GitHub token stored there (a detached Claude has no forwarding to
// open its PR with), and the task as the first message.
func agentsStartArgs(repo, box, task string) []string {
	return tuiChildArgs("claude", "--repo", repo, "--box", box, "--mux", "herdr",
		"--github", githubStore, "--detach", "--prompt", task+agentsPRInstruction)
}

// agentAttachScript is what attaching runs on the box: herdr focused on
// the session's workspace, or its tmux session.
func agentAttachScript(s agentSession) string {
	if s.mux != "herdr" {
		return "exec tmux -u attach -t =" + claudeSession
	}
	return `ws=$(herdr workspace list 2>/dev/null | grep -o ` + shellQuote(`"label":"`+claudeSession+`"[^}]*"workspace_id":"[^"]*"`) + ` | sed 's/.*"workspace_id":"//; s/"$//')
[ -z "$ws" ] || herdr workspace focus "$ws" >/dev/null
exec herdr`
}

// openPRScript pushes the box's branch and opens a pull request from
// what's committed, printing its URL (the existing one's, if there is).
func openPRScript(dir string) string {
	return `cd ` + shellQuote(dir) + ` || exit 1
gh=/usr/bin/gh; [ -x "$gh" ] || gh=gh
git push -u origin HEAD >&2 || exit 1
url=$("$gh" pr view --json url -q .url 2>/dev/null) || { "$gh" pr create --fill >&2 && url=$("$gh" pr view --json url -q .url); } && echo "$url"`
}
