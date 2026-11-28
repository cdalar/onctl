package cmd

import (
	"fmt"
	"log"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/cdalar/onctl/pkg/hostagent"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var hostAgentCmd = &cobra.Command{
	Use:   "host-agent",
	Short: "Run this machine as a box host for a control plane (boxctl, onctl-runners)",
	Long: `Registers this host with a control plane, heartbeats its facts (memory,
CPU, disk, images, KVM), pushes its VM list, and runs the jobs the control
plane sends it, until stopped. Meant to run under systemd. It replaces the
bash agents (boxctl-hostd.sh, onctl-host-agent.sh) and reads the same
environment:

  CONTROLLER_URL               the control plane, e.g. https://boxctl.example (required)
  HOST_NAME                    this host's name there (required)
  AGENT_TOKEN                  its bearer token (required)
  HEARTBEAT_INTERVAL_SECONDS   default 15
  VMS_REPORT_INTERVAL_SECONDS  default 5; 0 never pushes the VM list
  MAX_CONCURRENT_JOBS          default 4 (MAX_PARALLEL_JOBS also read)
  STATE_DIR                    default fc.stateDir from onctl.yaml, else /opt/fc
  IMAGE_DIRS                   default "/opt/fc/images /opt/win-images"
  HOST_LABELS                  k=v,k=v reported as labels
  AGENT_VERSION                reported as the agent's version (default onctl's)`,
	Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := ReadConfig(); err != nil {
			log.Println("[DEBUG] host-agent: no onctl.yaml, using defaults:", err)
		}
		cfg, err := hostAgentConfig(os.Getenv)
		if err != nil {
			return err
		}
		ctx, stop := signal.NotifyContext(cmd.Context(), os.Interrupt, syscall.SIGTERM)
		defer stop()
		_ = hostagent.New(cfg).Run(ctx)
		return nil
	},
}

func init() {
	rootCmd.AddCommand(hostAgentCmd)
}

// hostAgentConfig is the agent's Config from the environment getenv reads.
func hostAgentConfig(getenv func(string) string) (hostagent.Config, error) {
	cfg := hostagent.Config{
		ServerURL:       strings.TrimRight(getenv("CONTROLLER_URL"), "/"),
		Host:            getenv("HOST_NAME"),
		Token:           getenv("AGENT_TOKEN"),
		Version:         firstSet(getenv("AGENT_VERSION"), Version),
		OnctlVersion:    Version,
		ReportProviders: []string{"fc", "ch"},
		StateDir:        firstSet(getenv("STATE_DIR"), expandHome(viper.GetString("fc.stateDir")), "/opt/fc"),
		ImageDirs:       strings.Fields(firstSet(getenv("IMAGE_DIRS"), "/opt/fc/images /opt/win-images")),
		BinDir:          "/usr/local/bin",
		Labels:          parseLabels(getenv("HOST_LABELS")),
		Logf: func(format string, args ...any) {
			fmt.Fprintf(os.Stderr, "[onctl host-agent] "+format+"\n", args...)
		},
	}
	for name, v := range map[string]*string{"CONTROLLER_URL": &cfg.ServerURL, "HOST_NAME": &cfg.Host, "AGENT_TOKEN": &cfg.Token} {
		if *v == "" {
			return cfg, fmt.Errorf("%s is required", name)
		}
	}
	var err error
	if cfg.HeartbeatInterval, err = seconds(getenv, "HEARTBEAT_INTERVAL_SECONDS", 15); err != nil {
		return cfg, err
	}
	if cfg.ReportInterval, err = seconds(getenv, "VMS_REPORT_INTERVAL_SECONDS", 5); err != nil {
		return cfg, err
	}
	jobs := firstSet(getenv("MAX_CONCURRENT_JOBS"), getenv("MAX_PARALLEL_JOBS"), "4")
	if cfg.MaxJobs, err = strconv.Atoi(jobs); err != nil || cfg.MaxJobs < 1 {
		return cfg, fmt.Errorf("MAX_CONCURRENT_JOBS: %q isn't a positive number", jobs)
	}
	return cfg, nil
}

func seconds(getenv func(string) string, name string, def int) (time.Duration, error) {
	v := getenv(name)
	if v == "" {
		return time.Duration(def) * time.Second, nil
	}
	n, err := strconv.Atoi(v)
	if err != nil || n < 0 {
		return 0, fmt.Errorf("%s: %q isn't a number of seconds", name, v)
	}
	return time.Duration(n) * time.Second, nil
}

// parseLabels is HOST_LABELS' k=v,k=v (a value may hold "="; entries
// without one, or with an empty key, are skipped).
func parseLabels(s string) map[string]string {
	out := map[string]string{}
	for _, kv := range strings.Split(s, ",") {
		k, v, ok := strings.Cut(kv, "=")
		if ok && k != "" {
			out[k] = v
		}
	}
	return out
}

// firstSet is the first of vals that isn't empty.
func firstSet(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
}

func expandHome(p string) string {
	if strings.HasPrefix(p, "~") {
		if home, err := os.UserHomeDir(); err == nil {
			return home + p[1:]
		}
	}
	return p
}
