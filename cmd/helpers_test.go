package cmd

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/cdalar/onctl/pkg/cloud"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ago rounds to the unit a reader scans for: under two days it stays in
// hours ("47h ago"), because "1d ago" hides whether a session is 25 or
// 47 hours old.
func TestAgo(t *testing.T) {
	now := time.Now()
	for _, tc := range []struct {
		since time.Duration
		want  string
	}{
		{30 * time.Second, "just now"},
		{5 * time.Minute, "5m ago"},
		{3 * time.Hour, "3h ago"},
		{47 * time.Hour, "47h ago"},
		{72 * time.Hour, "3d ago"},
	} {
		assert.Equal(t, tc.want, ago(now.Add(-tc.since)), "since %s", tc.since)
	}
}

// The tui's flash line refreshes every second, so anything under two
// seconds reads "just now" instead of flickering "0s ago"/"1s ago".
func TestShortAgo(t *testing.T) {
	assert.Equal(t, "just now", shortAgo(time.Second))
	assert.Equal(t, "30s ago", shortAgo(30*time.Second))
	assert.Equal(t, "1m ago", shortAgo(90*time.Second))
}

// Every provider spells "stopped" differently (deallocated, SHELVED,
// TERMINATED); the STATE column must colour all of them paused-yellow,
// and leave statuses it doesn't know unstyled.
func TestStateStyle(t *testing.T) {
	paused := k9s.state["paused"].GetForeground()
	dead := k9s.state["dead"].GetForeground()

	st, ok := stateStyle("running")
	require.True(t, ok)
	assert.Equal(t, k9s.state["running"].GetForeground(), st.GetForeground())

	for _, s := range []string{"stopped", "SHELVED", "VM deallocated"} {
		st, ok := stateStyle(s)
		require.True(t, ok, s)
		assert.Equal(t, paused, st.GetForeground(), s)
	}
	st, ok = stateStyle("error")
	require.True(t, ok)
	assert.Equal(t, dead, st.GetForeground())

	_, ok = stateStyle("provisioning-ish")
	assert.False(t, ok, "unknown statuses keep the row's default colour")
}

// The hint printed after a session must bring the user back to the same
// box, so it names the provider unless it's boxes, the default.
func TestAttachHint(t *testing.T) {
	orig := cloudProvider
	t.Cleanup(func() { cloudProvider = orig })
	ws := workspace{name: "fix-ci"}

	withProvider(t, &cloud.ProviderBoxes{})
	assert.Equal(t, "onctl claude --box b1 --workspace fix-ci", attachHint(ws, "b1"))

	withProvider(t, &cloud.ProviderStatic{})
	cloudProvider = "ac"
	assert.Equal(t, "onctl -p ac claude --box b1 --workspace fix-ci", attachHint(ws, "b1"))
}

// Each workspace's WIP lives under its own ref so pushes from parallel workspaces
// don't clobber each other; the main checkout keeps the bare ref.
func TestBoxWorktreeWipRef(t *testing.T) {
	main := boxWorktree{}
	assert.Equal(t, wipRef, main.wipRef())
	assert.Equal(t, "wip", main.wipName())

	w := boxWorktree{name: "fix-ci"}
	assert.Equal(t, wipRef+"-fix-ci", w.wipRef())
	assert.Equal(t, "wip-fix-ci", w.wipName())
}

func TestClaudeTokenPath(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	p, err := claudeTokenPath()
	require.NoError(t, err)
	assert.Equal(t, filepath.Join(home, ".onctl", "claude", claudeTokenFile), p)
}

// A missing or unreadable ~/.claude.json must not block the session: the
// theme is a nicety, so every failure is an empty string, never an error.
func TestLocalClaudeTheme(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	cfg := filepath.Join(home, ".claude.json")

	assert.Equal(t, "", localClaudeTheme(), "no file")
	require.NoError(t, os.WriteFile(cfg, []byte("{not json"), 0o600))
	assert.Equal(t, "", localClaudeTheme(), "bad json")
	require.NoError(t, os.WriteFile(cfg, []byte(`{"theme":"dark-daltonized"}`), 0o600))
	assert.Equal(t, "dark-daltonized", localClaudeTheme())
}

// With ONCTL_CLOUD unset, the provider is inferred from the credentials
// in the environment and exported, so later code that reads ONCTL_CLOUD
// (and child processes) sees the same answer.
func TestCheckCloudProvider_AutoDetectsAndExports(t *testing.T) {
	orig := cloudProvider
	t.Cleanup(func() { cloudProvider = orig })
	for _, k := range []string{"ONCTL_CLOUD", "AWS_ACCESS_KEY_ID", "AWS_PROFILE", "AZURE_CLIENT_ID", "GOOGLE_CREDENTIALS", "OVH_APPLICATION_KEY"} {
		t.Setenv(k, "")
	}
	t.Setenv("HCLOUD_TOKEN", "x")

	assert.Equal(t, "hetzner", checkCloudProvider())
	assert.Equal(t, "hetzner", os.Getenv("ONCTL_CLOUD"))
}

