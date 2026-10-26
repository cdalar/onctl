package cmd

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestNewlyWaiting(t *testing.T) {
	blocked := agentSession{box: "a", status: "blocked"}
	done := agentSession{box: "a", status: "done"}
	working := agentSession{box: "a", status: "working"}

	// Opening the view must not fire a notification for every session
	// that was already waiting: you'd be told what you can see.
	assert.Empty(t, newlyWaiting(nil, []agentSession{blocked}))

	// The point of the view: Claude went from working to asking you.
	assert.Equal(t, []agentSession{blocked}, newlyWaiting(map[string]string{"a": "working"}, []agentSession{blocked}))

	// Still blocked on the next poll is the same question: one
	// notification per wait, not one every ten seconds.
	assert.Empty(t, newlyWaiting(map[string]string{"a": "blocked"}, []agentSession{blocked}))

	// Answering a permission and Claude then finishing is a new reason
	// to come back.
	assert.Equal(t, []agentSession{done}, newlyWaiting(map[string]string{"a": "blocked"}, []agentSession{done}))

	// Back to work needs nothing from you.
	assert.Empty(t, newlyWaiting(map[string]string{"a": "blocked"}, []agentSession{working}))

	// A session started after the view opened that is already waiting
	// by its first poll still needs you.
	assert.Equal(t, []agentSession{blocked}, newlyWaiting(map[string]string{}, []agentSession{blocked}))
}

func TestParseAgentStatus(t *testing.T) {
	// A box without a project isn't a session, whatever runs on it.
	_, ok := parseAgentStatus("plain", "")
	assert.False(t, ok)

	herdr := `{"result":{"workspaces":[{"label":"claude-other","agent_status":"working","workspace_id":"w2"},{"label":"claude","agent_status":"blocked","workspace_id":"w1"}]}}`
	out := strings.Join([]string{
		"dir=/root/onctl", "mux=herdr", "origin=https://github.com/cdalar/onctl.git",
		"branch=onctl/a1b2c3", "pr=https://github.com/cdalar/onctl/pull/7", "herdr=" + herdr,
	}, "\n")
	s, ok := parseAgentStatus("onctl-fix-x-1a2b", out)
	assert.True(t, ok)
	// The status is the main session's, not another workspace's on the
	// same box: that's the Claude this row starts and attaches to.
	assert.Equal(t, agentSession{
		box: "onctl-fix-x-1a2b", repo: "cdalar/onctl", dir: "/root/onctl", branch: "onctl/a1b2c3",
		mux: "herdr", status: "blocked", pr: "https://github.com/cdalar/onctl/pull/7",
	}, s)

	// Claude exited: its workspace is gone, and the row must not keep
	// showing it as working.
	s, _ = parseAgentStatus("b", "dir=/root/x\nmux=herdr\nherdr="+`{"result":{"workspaces":[]}}`)
	assert.Equal(t, "stopped", s.status)

	// tmux can't tell what Claude is doing, so such a box never claims
	// to be waiting (and never notifies).
	s, _ = parseAgentStatus("c", "dir=/root/x\nmux=tmux")
	assert.Equal(t, "tmux", s.status)
	assert.False(t, s.waiting())
}

func TestAgentsStartArgs(t *testing.T) {
	args := strings.Join(agentsStartArgs("cdalar/onctl", "onctl-x-1a2b", "fix the bug"), " ")
	// herdr is what reports blocked/done; with tmux nothing would notify.
	assert.Contains(t, args, "--mux herdr")
	// A detached Claude has no forwarded GitHub credentials, so without a
	// stored token it couldn't push or open the PR it's asked for.
	assert.Contains(t, args, "--github store")
	assert.Contains(t, args, "--detach")
	// A new box per session, never the repo's shared one.
	assert.Contains(t, args, "--box onctl-x-1a2b")
	assert.Contains(t, args, "fix the bug"+agentsPRInstruction)
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
