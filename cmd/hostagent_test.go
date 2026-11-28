package cmd

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// onctl host-agent reads the same environment as the bash agents it
// replaces, so their systemd env files keep working: boxctl-hostd's
// MAX_CONCURRENT_JOBS and onctl-host-agent's MAX_PARALLEL_JOBS both
// count, and the defaults are theirs.
func TestHostAgentConfigFromEnv(t *testing.T) {
	env := map[string]string{
		"CONTROLLER_URL":    "https://boxctl.example/",
		"HOST_NAME":         "h1",
		"AGENT_TOKEN":       "tok",
		"MAX_PARALLEL_JOBS": "2",
		"HOST_LABELS":       "zone=dc1,gpu=a=b,bad,=x",
		"STATE_DIR":         "/srv/fc",
	}
	cfg, err := hostAgentConfig(func(k string) string { return env[k] })
	require.NoError(t, err)
	assert.Equal(t, "https://boxctl.example", cfg.ServerURL, "a trailing slash would double up in the paths")
	assert.Equal(t, 2, cfg.MaxJobs)
	assert.Equal(t, 15*time.Second, cfg.HeartbeatInterval)
	assert.Equal(t, 5*time.Second, cfg.ReportInterval)
	assert.Equal(t, map[string]string{"zone": "dc1", "gpu": "a=b"}, cfg.Labels)
	assert.Equal(t, "/srv/fc", cfg.StateDir)
	assert.Equal(t, []string{"/opt/fc/images", "/opt/win-images"}, cfg.ImageDirs)
	assert.Equal(t, []string{"fc", "ch"}, cfg.ReportProviders)

	env["VMS_REPORT_INTERVAL_SECONDS"] = "0"
	cfg, err = hostAgentConfig(func(k string) string { return env[k] })
	require.NoError(t, err)
	assert.Zero(t, cfg.ReportInterval, "0 turns the VM report off, for onctl-runners' controller")

	for _, missing := range []string{"CONTROLLER_URL", "HOST_NAME", "AGENT_TOKEN"} {
		saved := env[missing]
		delete(env, missing)
		_, err := hostAgentConfig(func(k string) string { return env[k] })
		assert.ErrorContains(t, err, missing)
		env[missing] = saved
	}
	env["MAX_CONCURRENT_JOBS"] = "0"
	_, err = hostAgentConfig(func(k string) string { return env[k] })
	assert.Error(t, err)
}
