// Phase 2 spike: autoscaling controller for onctl-provisioned GitHub Actions
// runners. Listens for GitHub's workflow_job webhook and provisions a
// JIT-config runner VM on "queued", then destroys it on "completed".
//
// See ../PHASE2.md for the plan and ../ROADMAP.md items #1 and #2.
package main

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"time"
)

type config struct {
	port            string
	ghRepo          string
	webhookSecret   string
	runnerLabels    []string
	bootstrapScript string
	onctlBin        string
	ghBin           string
}

func loadConfig() config {
	cfg := config{
		port:            getenv("PORT", "8080"),
		ghRepo:          os.Getenv("GH_REPO"),
		webhookSecret:   os.Getenv("WEBHOOK_SECRET"),
		runnerLabels:    splitCSV(getenv("RUNNER_LABELS", "self-hosted,onctl")),
		bootstrapScript: getenv("BOOTSTRAP_SCRIPT", "../github-runner-jit.sh"),
		onctlBin:        getenv("ONCTL_BIN", "onctl"),
		ghBin:           getenv("GH_BIN", "gh"),
	}
	if cfg.ghRepo == "" || cfg.webhookSecret == "" {
		log.Fatal("GH_REPO and WEBHOOK_SECRET are required")
	}
	return cfg
}

func getenv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func splitCSV(s string) []string {
	var out []string
	for _, part := range strings.Split(s, ",") {
		part = strings.TrimSpace(part)
		if part != "" {
			out = append(out, part)
		}
	}
	return out
}

// workflowJobEvent is the subset of GitHub's workflow_job webhook payload
// the controller acts on.
type workflowJobEvent struct {
	Action      string `json:"action"`
	WorkflowJob struct {
		ID         int64    `json:"id"`
		Labels     []string `json:"labels"`
		RunnerName string   `json:"runner_name"`
	} `json:"workflow_job"`
}

type server struct {
	cfg config
}

func newServer(cfg config) *server {
	return &server{cfg: cfg}
}

func (s *server) routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/webhook", s.handleWebhook)
	return mux
}

func (s *server) handleWebhook(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "error reading body", http.StatusBadRequest)
		return
	}

	if !validSignature(body, r.Header.Get("X-Hub-Signature-256"), s.cfg.webhookSecret) {
		http.Error(w, "invalid signature", http.StatusUnauthorized)
		return
	}

	switch r.Header.Get("X-GitHub-Event") {
	case "ping":
		w.WriteHeader(http.StatusOK)
	case "workflow_job":
		s.handleWorkflowJob(w, body)
	default:
		w.WriteHeader(http.StatusAccepted)
	}
}

func (s *server) handleWorkflowJob(w http.ResponseWriter, body []byte) {
	var event workflowJobEvent
	if err := json.Unmarshal(body, &event); err != nil {
		http.Error(w, "invalid payload", http.StatusBadRequest)
		return
	}

	if !hasAllLabels(event.WorkflowJob.Labels, s.cfg.runnerLabels) {
		w.WriteHeader(http.StatusAccepted)
		return
	}

	switch event.Action {
	case "queued":
		go s.provision(event.WorkflowJob.ID)
	case "completed":
		go s.teardown(event.WorkflowJob.RunnerName)
	}
	w.WriteHeader(http.StatusAccepted)
}

// validSignature checks the X-Hub-Signature-256 header (HMAC-SHA256 of the
// raw body, hex-encoded, prefixed with "sha256=") against the shared secret.
func validSignature(body []byte, header, secret string) bool {
	const prefix = "sha256="
	if !strings.HasPrefix(header, prefix) {
		return false
	}
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)
	expected := hex.EncodeToString(mac.Sum(nil))
	return hmac.Equal([]byte(expected), []byte(strings.TrimPrefix(header, prefix)))
}

// hasAllLabels reports whether jobLabels contains every label in required
// (case-insensitive), so the controller ignores jobs not destined for it
// (e.g. GitHub-hosted "ubuntu-latest" runners).
func hasAllLabels(jobLabels, required []string) bool {
	set := make(map[string]bool, len(jobLabels))
	for _, l := range jobLabels {
		set[strings.ToLower(l)] = true
	}
	for _, r := range required {
		if !set[strings.ToLower(r)] {
			return false
		}
	}
	return true
}

// runnerName derives a deterministic onctl VM/runner name from the job ID,
// so "queued" and "completed" events for the same job agree on a name
// without any shared state.
func runnerName(jobID int64) string {
	return fmt.Sprintf("gh-runner-%d", jobID)
}

// provision generates a JIT runner config and provisions a VM for the job.
// Runs in a goroutine: onctl create takes ~1 minute, well past GitHub's 10s
// webhook delivery timeout.
func (s *server) provision(jobID int64) {
	name := runnerName(jobID)
	t0 := time.Now()
	logf := func(format string, args ...any) {
		log.Printf("[provision %s +%s] %s", name, time.Since(t0).Round(time.Second), fmt.Sprintf(format, args...))
	}

	logf("generating JIT config")
	jitConfig, err := s.generateJITConfig(name)
	if err != nil {
		logf("generate-jitconfig failed: %v", err)
		return
	}

	logf("creating VM")
	cmd := exec.Command(s.cfg.onctlBin, "create",
		"-n", name,
		"-a", s.cfg.bootstrapScript,
		"-e", "JIT_CONFIG="+jitConfig,
	)
	out, err := cmd.CombinedOutput()
	if err != nil {
		logf("onctl create failed: %v\n%s", err, out)
		return
	}
	logf("onctl create done")
}

// generateJITConfig shells out to `gh api` to create a single-use,
// pre-scoped runner credential (see ../PHASE1.md).
func (s *server) generateJITConfig(name string) (string, error) {
	cmd := exec.Command(s.cfg.ghBin, "api", "-X", "POST",
		fmt.Sprintf("repos/%s/actions/runners/generate-jitconfig", s.cfg.ghRepo),
		"-f", "name="+name,
		"-F", "runner_group_id=1",
		"-f", "labels[]=self-hosted",
		"-f", "labels[]=onctl",
		"-q", ".encoded_jit_config",
	)
	out, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

// teardown destroys the VM for a completed job (ROADMAP #2: the JIT runner
// has already self-deregistered from GitHub; this reclaims the VM so it
// stops billing).
//
// name is workflow_job.runner_name from the completed event, not derived
// from the completed job's own ID: a runner isn't bound to the job that
// triggered its creation, it picks up whatever queued job matches its
// labels, so the completed job's ID may belong to a different VM (or none).
// runner_name is the name we set via generate-jitconfig and is the VM name.
func (s *server) teardown(name string) {
	if name == "" {
		log.Println("[teardown] completed event has no runner_name (job never ran on a runner), nothing to destroy")
		return
	}
	log.Printf("[teardown %s] destroying VM", name)
	cmd := exec.Command(s.cfg.onctlBin, "destroy", name, "-f")
	out, err := cmd.CombinedOutput()
	if err != nil {
		log.Printf("[teardown %s] onctl destroy failed: %v\n%s", name, err, out)
		return
	}
	log.Printf("[teardown %s] done", name)
}

func main() {
	cfg := loadConfig()
	s := newServer(cfg)
	addr := ":" + cfg.port
	log.Printf("listening on %s (repo=%s labels=%v bootstrap=%s)", addr, cfg.ghRepo, cfg.runnerLabels, cfg.bootstrapScript)
	log.Fatal(http.ListenAndServe(addr, s.routes()))
}
