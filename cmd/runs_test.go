package cmd

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/cdalar/onctl/pkg/cloud"
	"github.com/stretchr/testify/assert"
)

func TestNewlyWaiting(t *testing.T) {
	blocked := boxRun{box: "a", status: "blocked"}
	done := boxRun{box: "a", status: "done"}
	working := boxRun{box: "a", status: "working"}

	// Opening the view must not fire a notification for every run
	// that was already waiting: you'd be told what you can see.
	assert.Empty(t, newlyWaiting(nil, []boxRun{blocked}))

	// The point of the view: Claude went from working to asking you.
	assert.Equal(t, []boxRun{blocked}, newlyWaiting(map[string]string{"a": "working"}, []boxRun{blocked}))

	// Still blocked on the next poll is the same question: one
	// notification per wait, not one every ten seconds.
	assert.Empty(t, newlyWaiting(map[string]string{"a": "blocked"}, []boxRun{blocked}))

	// Answering a permission and Claude then finishing is a new reason
	// to come back.
	assert.Equal(t, []boxRun{done}, newlyWaiting(map[string]string{"a": "blocked"}, []boxRun{done}))

	// Back to work needs nothing from you.
	assert.Empty(t, newlyWaiting(map[string]string{"a": "blocked"}, []boxRun{working}))

	// A run started after the view opened that is already waiting
	// by its first poll still needs you.
	assert.Equal(t, []boxRun{blocked}, newlyWaiting(map[string]string{}, []boxRun{blocked}))
}

func TestParseRunStatus(t *testing.T) {
	// A box without a project isn't a run, whatever runs on it.
	_, ok := parseRunStatus("plain", "")
	assert.False(t, ok)

	herdr := `{"result":{"workspaces":[{"label":"claude-other","agent_status":"working","workspace_id":"w2"},{"label":"claude","agent_status":"blocked","workspace_id":"w1"}]}}`
	out := strings.Join([]string{
		"dir=/root/onctl", "mux=herdr", "origin=https://github.com/cdalar/onctl.git",
		"branch=onctl/a1b2c3", "pr=https://github.com/cdalar/onctl/pull/7", "herdr=" + herdr,
	}, "\n")
	s, ok := parseRunStatus("onctl-fix-x-1a2b", out)
	assert.True(t, ok)
	// The status is the main session's, not another workspace's on the
	// same box: that's the Claude this row starts and attaches to.
	assert.Equal(t, boxRun{
		box: "onctl-fix-x-1a2b", repo: "cdalar/onctl", dir: "/root/onctl", branch: "onctl/a1b2c3",
		mux: "herdr", status: "blocked", pr: "https://github.com/cdalar/onctl/pull/7",
	}, s)

	// Claude exited: its workspace is gone, and the row must not keep
	// showing it as working.
	s, _ = parseRunStatus("b", "dir=/root/x\nmux=herdr\nherdr="+`{"result":{"workspaces":[]}}`)
	assert.Equal(t, "stopped", s.status)

	// tmux can't tell what Claude is doing, so such a box never claims
	// to be waiting (and never notifies).
	s, _ = parseRunStatus("c", "dir=/root/x\nmux=tmux")
	assert.Equal(t, "tmux", s.status)
	assert.False(t, s.waiting())
}

func TestRunStartArgs(t *testing.T) {
	args := strings.Join(runStartArgs("cdalar/onctl", "onctl-x-1a2b", "fix the bug"), " ")
	// herdr is what reports blocked/done; with tmux nothing would notify.
	assert.Contains(t, args, "--mux herdr")
	// A detached Claude has no forwarded GitHub credentials, so without a
	// stored token it couldn't push or open the PR it's asked for.
	assert.Contains(t, args, "--github store")
	assert.Contains(t, args, "--detach")
	// A new box per run, never the repo's shared one.
	assert.Contains(t, args, "--box onctl-x-1a2b")
	assert.Contains(t, args, "fix the bug"+runPRInstruction)
}

func TestOpenPRScriptNeverCommits(t *testing.T) {
	// P publishes what Claude committed; committing whatever is lying in
	// the working tree would put half-done work up for review.
	s := openPRScript("/root/onctl")
	assert.NotContains(t, s, "git commit")
	assert.NotContains(t, s, "git add")
	assert.Contains(t, s, "git push -u origin HEAD")
}

func TestAgentsRegistry(t *testing.T) {
	p := filepath.Join(t.TempDir(), "agents.json")
	prev := agentsRegistryPath
	agentsRegistryPath = func() (string, error) { return p, nil }
	defer func() { agentsRegistryPath = prev }()

	// Off the boxes provider only these VMs are polled, so a box started
	// here must be remembered across runs of the view.
	assert.NoError(t, addToAgentsRegistry("a", "o/a"))
	assert.NoError(t, addToAgentsRegistry("b", "o/b"))
	assert.Equal(t, map[string]string{"a": "o/a", "b": "o/b"}, readAgentsRegistry())
}

func TestOriginRepo(t *testing.T) {
	for _, url := range []string{"git@github.com:o/r.git", "https://github.com/o/r.git", "ssh://git@github.com/o/r"} {
		assert.Equal(t, "o/r", originRepo(url), url)
	}
	assert.Equal(t, "", originRepo(""))
}

// :runs in the tui: the runs come in with each poll, a desktop
// notification goes out only when a Claude starts waiting (not for what
// was already waiting when the view opened), n -> repo -> task starts a
// run on a new box and comes back to :runs, and :repos opened on
// its own still asks which box, as before.
func TestTUIRuns(t *testing.T) {
	tuiFlashFor = time.Millisecond
	withCloudProvider(t, "hetzner")
	dir := t.TempDir()
	origCache, origList, origReg, origNotify := repoCachePath, listGitHubRepos, agentsRegistryPath, notifyDesktop
	t.Cleanup(func() {
		repoCachePath, listGitHubRepos, agentsRegistryPath, notifyDesktop = origCache, origList, origReg, origNotify
	})
	repoCachePath = func() (string, error) { return filepath.Join(dir, "repos.json"), nil }
	listGitHubRepos = func(context.Context) ([]ghRepo, error) {
		return []ghRepo{{FullName: "acme/web", PushedAt: time.Now()}}, nil
	}
	agentsRegistryPath = func() (string, error) { return filepath.Join(dir, "agents.json"), nil }
	var notified []string
	var mu sync.Mutex
	notifyDesktop = func(title, _ string) { mu.Lock(); notified = append(notified, title); mu.Unlock() }
	notifications := func() []string {
		time.Sleep(20 * time.Millisecond) // notifications go out on their own goroutine
		mu.Lock()
		defer mu.Unlock()
		return append([]string(nil), notified...)
	}

	m := listed(t, tuiFixture(), 140, 40)
	_ = m.openView(runsView) // its poll is ssh; the test delivers the runs itself
	poll := func(runs ...boxRun) {
		next, _ := m.Update(tuiListMsg{resource: "runs", runs: runs})
		m = next.(tuiModel)
	}
	poll(boxRun{box: "db", status: "working"}, boxRun{box: "web", repo: "acme/web", status: "blocked"})
	assert.Equal(t, "db web", keys(m.rows()))
	assert.Empty(t, notifications(), "already waiting when the view opened")
	poll(boxRun{box: "db", status: "blocked"}, boxRun{box: "web", repo: "acme/web", status: "blocked"})
	assert.Equal(t, []string{"Claude is waiting: db"}, notifications())
	assert.Contains(t, m.View(), "blocked")

	// Polled from now on in any view, so the notification doesn't need
	// :runs on screen.
	assert.True(t, m.loaded["runs"])

	// n -> :repos on top of :runs -> enter asks for the task.
	m, cmd := press(t, m, "n")
	assert.Equal(t, reposView, m.top().view)
	for _, msg := range runCmd(cmd) {
		if lm, ok := msg.(tuiListMsg); ok {
			next, _ := m.Update(lm)
			m = next.(tuiModel)
		}
	}
	m, _ = press(t, m, "enter")
	assert.True(t, m.tasking, "enter asks for the task, not which box")
	assert.Nil(t, m.dialog)
	m = typeIn(t, m, "fix login")
	assert.Len(t, strings.Split(m.View(), "\n"), 40, "the task prompt line fits in the screen")
	m, cmd = press(t, m, "enter")
	assert.NotNil(t, cmd, "onctl claude runs")
	assert.False(t, m.tasking)
	assert.Equal(t, runsView, m.top().view, "back to the runs")
	reg := readAgentsRegistry()
	assert.Len(t, reg, 1)
	for box, repo := range reg {
		assert.True(t, strings.HasPrefix(box, "web-fix-login-"), box)
		assert.Equal(t, "acme/web", repo)
	}

	// ctrl-d on a run is the VM's destroy, asked first.
	m.top().selected = "web"
	m, _ = press(t, m, "ctrl+d")
	assert.NotNil(t, m.dialog)
	m.dialog = nil

	// :repos opened by itself keeps asking which box.
	_ = m.openView(reposView)
	m, _ = press(t, m, "enter")
	assert.False(t, m.tasking)
	assert.NotNil(t, m.dialog)
}

// --- the box side, run for real against a fake box -------------------------

// fakeRunBox makes "the box" a directory here, so the scripts the
// runs view sends run for real: ssh, herdr, gh, timeout and tmux on
// PATH are stand-ins, ssh runs the box's command with sh (its
// /root/.boxctl being <root>/boxctl), and a host named unreachable fails
// the way ssh does. herdr lists <root>/herdr.json; gh has a pull request
// once <root>/pr exists, and pr create makes it. Every stand-in logs its
// arguments to the returned log. agentConn connects to any box but one
// named gone, recording which.
func fakeRunBox(t *testing.T) (root string, log func() string, connected func() []string) {
	t.Helper()
	root = t.TempDir()
	bin := filepath.Join(root, "bin")
	logFile := filepath.Join(root, "log")
	scripts := map[string]string{
		"ssh": `for a; do last=$a; done
case " $* " in *" root@unreachable "*) echo "ssh: connect to host unreachable port 22: Connection refused" >&2; exit 255;; esac
exec sh -c "$(printf '%s' "$last" | sed "s#/root/.boxctl#$FAKE_ROOT/boxctl#g")"`,
		"herdr": `echo "herdr $*" >>"$FAKE_LOG"
[ "$1 $2" != "workspace list" ] || cat "$FAKE_ROOT/herdr.json"`,
		"gh": `echo "gh $*" >>"$FAKE_LOG"
case "$1 $2" in
"pr view") [ -f "$FAKE_ROOT/pr" ] && cat "$FAKE_ROOT/pr" || exit 1;;
"pr create") echo https://github.com/acme/web/pull/1 >"$FAKE_ROOT/pr";;
esac`,
		"timeout": `shift; exec "$@"`,
		"tmux":    `echo "tmux $*" >>"$FAKE_LOG"`,
	}
	assert.NoError(t, os.MkdirAll(bin, 0o755))
	assert.NoError(t, os.MkdirAll(filepath.Join(root, "boxctl"), 0o755))
	for name, body := range scripts {
		assert.NoError(t, os.WriteFile(filepath.Join(bin, name), []byte("#!/bin/sh\n"+body+"\n"), 0o755))
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	prevGH := boxGH
	t.Cleanup(func() { boxGH = prevGH })
	boxGH = filepath.Join(bin, "gh")
	t.Setenv("FAKE_ROOT", root)
	t.Setenv("FAKE_LOG", logFile)
	t.Setenv("HOME", root) // newVMSSH's control dir; and no global git config (signing)

	var mu sync.Mutex
	var names []string
	prev := agentConn
	t.Cleanup(func() { agentConn = prev })
	agentConn = func(_ context.Context, name string) *boxSSH {
		mu.Lock()
		names = append(names, name)
		mu.Unlock()
		if name == "gone" {
			return nil
		}
		b, err := newVMSSH(name, "root", name, 22, filepath.Join(root, "key"))
		assert.NoError(t, err)
		return b
	}
	log = func() string { data, _ := os.ReadFile(logFile); return string(data) }
	connected = func() []string {
		mu.Lock()
		defer mu.Unlock()
		sort.Strings(names)
		return append([]string(nil), names...)
	}
	return root, log, connected
}

// boxProject puts a project on the fake box at <root>/web: on branch
// onctl/abc with one commit, its origin GitHub's acme/web for reading and
// a bare repository here for pushing.
func boxProject(t *testing.T, root string) (dir, bare string) {
	t.Helper()
	dir, bare = filepath.Join(root, "web"), filepath.Join(root, "origin.git")
	for _, args := range [][]string{
		{"init", "-q", "--bare", bare},
		{"init", "-q", "-b", "onctl/abc", dir},
		{"-C", dir, "-c", "user.name=t", "-c", "user.email=t@x", "-c", "commit.gpgsign=false", "commit", "-q", "--allow-empty", "-m", "work"},
		{"-C", dir, "remote", "add", "origin", "https://github.com/acme/web.git"},
		{"-C", dir, "remote", "set-url", "--push", "origin", bare},
	} {
		out, err := exec.Command("git", args...).CombinedOutput()
		assert.NoError(t, err, "git %v: %s", args, out)
	}
	assert.NoError(t, os.WriteFile(filepath.Join(root, "boxctl", "project"), []byte(dir), 0o600))
	assert.NoError(t, os.WriteFile(filepath.Join(root, "boxctl", "mux"), []byte("herdr"), 0o600))
	return dir, bare
}

const herdrBlocked = `{"result":{"workspaces":[{"label":"claude-x","agent_status":"working","workspace_id":"w2"},{"label":"claude","agent_status":"blocked","workspace_id":"w1"}]}}`

// The status script and its parser agree on a real box layout: what the
// row shows is the project's repo, branch, the main session's herdr
// status and its PR -- and a box with no project prints nothing, so it
// never shows up as a run.
func TestRunStatusScriptOnABox(t *testing.T) {
	root, _, _ := fakeRunBox(t)
	dir, _ := boxProject(t, root)
	assert.NoError(t, os.WriteFile(filepath.Join(root, "herdr.json"), []byte(herdrBlocked), 0o600))
	assert.NoError(t, os.WriteFile(filepath.Join(root, "pr"), []byte("https://github.com/acme/web/pull/1\n"), 0o600))
	b, err := newVMSSH("web", "root", "web", 22, "key")
	assert.NoError(t, err)

	out, err := b.output(context.Background(), runStatusScript())
	assert.NoError(t, err)
	s, ok := parseRunStatus("web", string(out))
	assert.True(t, ok)
	assert.Equal(t, boxRun{box: "web", repo: "acme/web", dir: dir, branch: "onctl/abc", mux: "herdr",
		status: "blocked", pr: "https://github.com/acme/web/pull/1"}, s)

	assert.NoError(t, os.Remove(filepath.Join(root, "boxctl", "project")))
	out, err = b.output(context.Background(), runStatusScript())
	assert.NoError(t, err)
	_, ok = parseRunStatus("web", string(out))
	assert.False(t, ok)
}

// A box whose runs haven't started yet (no multiplexer recorded) says
// so rather than looking stopped.
func TestParseRunStatusNotStarted(t *testing.T) {
	s, ok := parseRunStatus("b", "dir=/root/x")
	assert.True(t, ok)
	assert.Equal(t, "not started", s.status)
}

// Off the boxes provider only the VMs started from the view are
// connected to -- connecting gives root ssh on a VM -- and only running
// ones; one that can't be reached is still listed, with why, instead of
// vanishing; the rows come sorted by box.
func TestPollRunsOnlyTouchesItsOwnVMs(t *testing.T) {
	root, _, connected := fakeRunBox(t)
	dir, _ := boxProject(t, root)
	assert.NoError(t, os.WriteFile(filepath.Join(root, "herdr.json"), []byte(herdrBlocked), 0o600))
	withCloudProvider(t, "hetzner")
	saved := provider
	t.Cleanup(func() { provider = saved })
	provider = &tuiFakeProvider{
		vms: []cloud.Vm{
			{Name: "web", Status: "running"}, {Name: "db", Status: "running"}, {Name: "unreachable", Status: "running"},
		},
		paused: []cloud.Vm{{Name: "old", Status: "paused"}},
	}
	prevReg := agentsRegistryPath
	t.Cleanup(func() { agentsRegistryPath = prevReg })
	agentsRegistryPath = func() (string, error) { return filepath.Join(root, "agents.json"), nil }
	for box, repo := range map[string]string{"web": "acme/web", "unreachable": "acme/api", "old": "acme/old"} {
		assert.NoError(t, addToAgentsRegistry(box, repo))
	}

	runs, err := pollRuns()
	assert.NoError(t, err)
	assert.Equal(t, []string{"unreachable", "web"}, connected(), "db isn't ours, old is paused")
	if assert.Len(t, runs, 2) {
		assert.Equal(t, "unreachable", runs[0].box)
		assert.Equal(t, "acme/api", runs[0].repo, "the repo it was started for")
		assert.NotEmpty(t, runs[0].err)
		assert.Equal(t, boxRun{box: "web", repo: "acme/web", dir: dir, branch: "onctl/abc", mux: "herdr", status: "blocked"}, runs[1])
	}
}

// Attaching lands on the session's own herdr workspace (labelled claude,
// not another workspace's) and then runs the herdr client; on a tmux box it's
// the session named exactly claude (=), not one that starts with it.
func TestRunAttachScript(t *testing.T) {
	root, log, _ := fakeRunBox(t)
	assert.NoError(t, os.WriteFile(filepath.Join(root, "herdr.json"), []byte(herdrBlocked), 0o600))
	out, err := exec.Command("sh", "-c", runAttachScript(boxRun{mux: "herdr"})).CombinedOutput()
	assert.NoError(t, err, string(out))
	assert.Equal(t, "herdr workspace list\nherdr workspace focus w1\nherdr \n", log())

	assert.NoError(t, os.Remove(filepath.Join(root, "log")))
	out, err = exec.Command("sh", "-c", runAttachScript(boxRun{mux: "tmux"})).CombinedOutput()
	assert.NoError(t, err, string(out))
	assert.Equal(t, "tmux -u attach -t =claude\n", log())
}

// P publishes what Claude committed -- the branch pushed, a pull request
// opened, its URL flashed -- and pressing it again doesn't open a second
// one; a box that can't be reached says so.
func TestTUIRunsPushPR(t *testing.T) {
	tuiFlashFor = time.Millisecond
	root, log, _ := fakeRunBox(t)
	dir, bare := boxProject(t, root)
	m := listed(t, tuiFixture(), 140, 40)
	_ = m.openView(runsView)
	next, _ := m.Update(tuiListMsg{resource: "runs", runs: []boxRun{{box: "gone"}, {box: "web", dir: dir}}})
	m = next.(tuiModel)

	pressP := func(box string) tuiModel {
		m.top().selected = box
		mm, cmd := press(t, m, "P")
		am := hasAction(cmd)
		if !assert.NotNil(t, am) {
			return mm
		}
		next, _ := mm.Update(*am)
		return next.(tuiModel)
	}
	m = pressP("web")
	assert.Equal(t, "pull request: https://github.com/acme/web/pull/1", m.flash)
	out, err := exec.Command("git", "-C", bare, "rev-parse", "--verify", "onctl/abc").CombinedOutput()
	assert.NoError(t, err, "the branch was pushed: %s", out)
	m = pressP("web")
	assert.Equal(t, "pull request: https://github.com/acme/web/pull/1", m.flash)
	assert.Equal(t, 1, strings.Count(log(), "gh pr create"), "one pull request, however often P is pressed")

	m = pressP("gone")
	assert.True(t, m.flashErr)
	assert.Contains(t, m.flash, "pushing gone: can't reach the box")
}

// The runs view's other keys, and what's around them.
func TestTUIRunsKeys(t *testing.T) {
	tuiFlashFor = time.Millisecond
	withCloudProvider(t, "hetzner")
	fakeRunBox(t)
	dir := t.TempDir()
	prevReg := agentsRegistryPath
	t.Cleanup(func() { agentsRegistryPath = prevReg })
	agentsRegistryPath = func() (string, error) { return filepath.Join(dir, "agents.json"), nil }

	m := listed(t, tuiFixture(), 140, 40)
	_ = m.openView(runsView)

	// No runs: the keys that act on one do nothing.
	for _, k := range []string{"enter", "o", "P"} {
		mm, cmd := press(t, m, k)
		assert.Nil(t, cmd, k)
		assert.Empty(t, mm.flash, k)
	}

	next, _ := m.Update(tuiListMsg{resource: "runs", runs: []boxRun{{box: "gone"}, {box: "web", mux: "herdr"}}})
	m = next.(tuiModel)

	// Polled on every refresh from now on, whichever view shows.
	m.openView(vmsView)
	m.refresh()
	assert.True(t, m.loading["runs"])
	delete(m.loading, "runs")
	_ = m.openView(runsView)

	// o with no pull request yet says so, rather than opening nothing.
	m.top().selected = "web"
	m, _ = press(t, m, "o")
	assert.True(t, m.flashErr)
	assert.Equal(t, "web has no pull request yet", m.flash)
	m = ack(t, m)

	// enter hands the terminal to the box; one that can't be reached says so.
	_, cmd := press(t, m, "enter")
	assert.NotNil(t, cmd)
	m.top().selected = "gone"
	m, cmd = press(t, m, "enter")
	assert.Equal(t, "can't reach gone", m.flash)
	assert.NotNil(t, cmd, "the flash's timer")
	m = ack(t, m)

	// The help page explains the view.
	m, _ = press(t, m, "?")
	assert.Contains(t, strings.Join(m.top().lines, "\n"), "n picks a repo in :repos, then asks for the task's description")
	m.pop()

	// A run's start coming back says what happens next.
	next, _ = m.Update(tuiChildDoneMsg{verb: "start", name: "web-x-1a2b"})
	m = next.(tuiModel)
	assert.Contains(t, m.flash, "Claude is working on web-x-1a2b")

	// The task prompt: esc cancels, an empty description starts nothing.
	ask := func() {
		next, _ := m.askTask("acme/web")
		m = next.(tuiModel)
	}
	ask()
	m, cmd = press(t, m, "enter")
	assert.True(t, m.tasking)
	assert.Nil(t, cmd)
	m, _ = press(t, m, "esc")
	assert.False(t, m.tasking)

	// A box that couldn't be remembered isn't started: off the boxes
	// provider it would never be polled, and nobody would be told when
	// its Claude waits.
	agentsRegistryPath = func() (string, error) { return "", errors.New("no home") }
	ask()
	m = typeIn(t, m, "fix login")
	m, cmd = press(t, m, "enter")
	assert.True(t, m.flashErr)
	assert.Contains(t, m.flash, "remembering web-fix-login-")
	for _, msg := range runCmd(cmd) {
		_, isExec := msg.(tuiChildDoneMsg)
		assert.False(t, isExec, "no onctl claude ran")
	}
}
