// Package hostagent is the daemon a box host runs for a control plane
// (the boxctl server, or onctl-runners' controller): it registers, sends
// its facts on every heartbeat, pushes its VM list, and long-polls for
// jobs to run. It replaces boxctl's boxctl-hostd.sh and onctl-runners'
// onctl-host-agent.sh, speaking the same protocol:
//
//	POST /hosts/{name}/register        Facts; 204 when accepted
//	POST /hosts/{name}/heartbeat       Facts
//	POST /hosts/{name}/vms-report      the VMs, as `onctl ls -o json` prints them
//	GET  /hosts/{name}/jobs/next       200 a Job, or 204 none waiting
//	POST /hosts/{name}/jobs/{id}/result  Result; the reply may say {"abandoned": true}
//
// all with "Authorization: Bearer <token>".
package hostagent

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"sync"
	"time"

	"github.com/cdalar/onctl/pkg/box"
	"github.com/cdalar/onctl/pkg/cloud"
)

// Config is how an Agent reaches its server and describes its host.
type Config struct {
	ServerURL string // e.g. https://boxctl.example
	Host      string // this host's name on the server
	Token     string // the raw bearer token
	// Version is the agent's, OnctlVersion onctl's; both are reported.
	Version, OnctlVersion string
	// HeartbeatInterval defaults to 15s.
	HeartbeatInterval time.Duration
	// ReportInterval is how often the VM list is pushed (after every job
	// too); 0 never pushes it, for servers without vms-report.
	ReportInterval time.Duration
	// ReportProviders are the providers whose VMs are reported (fc, ch).
	ReportProviders []string
	// MaxJobs is how many jobs run at once; defaults to 4. A job isn't
	// claimed until there's room for it.
	MaxJobs int
	// StateDir is fc.stateDir, whose disk is reported; ImageDirs are where
	// rootfs images and kernels live; BinDir is where the host bundle's
	// scripts are installed (/usr/local/bin), for Capabilities.
	StateDir  string
	ImageDirs []string
	BinDir    string
	Labels    map[string]string
	// NewClient builds a box client for a provider; box.New by default.
	NewClient func(provider string) (*box.Client, error)
	// HTTP defaults to http.DefaultClient; each call sets its own timeout.
	HTTP *http.Client
	// Logf defaults to log.Printf.
	Logf func(format string, args ...any)
}

// Agent runs Config's host for its server.
type Agent struct {
	cfg Config

	mu        sync.Mutex
	providers []string // which ReportProviders answered the last report; nil before one
	reportMu  sync.Mutex
}

// New is an Agent for cfg, with its defaults filled in.
func New(cfg Config) *Agent {
	if cfg.HeartbeatInterval == 0 {
		cfg.HeartbeatInterval = 15 * time.Second
	}
	if cfg.MaxJobs <= 0 {
		cfg.MaxJobs = 4
	}
	if cfg.NewClient == nil {
		cfg.NewClient = func(p string) (*box.Client, error) { return box.New(box.Options{Provider: p}) }
	}
	if cfg.HTTP == nil {
		cfg.HTTP = http.DefaultClient
	}
	if cfg.Logf == nil {
		cfg.Logf = log.Printf
	}
	return &Agent{cfg: cfg}
}

// Run registers (retrying until the server accepts), then heartbeats,
// reports VMs and runs jobs until ctx ends. Jobs already running when it
// ends are let finish and report.
func (a *Agent) Run(ctx context.Context) error {
	for {
		err := a.register(ctx)
		if err == nil {
			break
		}
		a.cfg.Logf("register failed (%v), retrying in 5s", err)
		if !sleep(ctx, 5*time.Second) {
			return ctx.Err()
		}
	}
	a.cfg.Logf("registered with %s as %s", a.cfg.ServerURL, a.cfg.Host)

	var wg sync.WaitGroup
	wg.Add(1)
	go func() { defer wg.Done(); a.heartbeatLoop(ctx) }()
	if a.cfg.ReportInterval > 0 {
		wg.Add(1)
		go func() { defer wg.Done(); a.reportLoop(ctx) }()
	}
	a.jobLoop(ctx)
	wg.Wait()
	return ctx.Err()
}

func (a *Agent) register(ctx context.Context) error {
	status, _, err := a.post(ctx, "/register", a.facts(a.lastProviders()), 15*time.Second)
	if err != nil {
		return err
	}
	if status != http.StatusNoContent {
		return fmt.Errorf("http %d", status)
	}
	return nil
}

func (a *Agent) heartbeatLoop(ctx context.Context) {
	for sleep(ctx, a.cfg.HeartbeatInterval) {
		if _, _, err := a.post(ctx, "/heartbeat", a.facts(a.lastProviders()), 15*time.Second); err != nil {
			a.cfg.Logf("heartbeat failed: %v", err)
		}
	}
}

func (a *Agent) reportLoop(ctx context.Context) {
	for {
		a.reportVMs(ctx)
		if !sleep(ctx, a.cfg.ReportInterval) {
			return
		}
	}
}

// reportVMs pushes ReportProviders' VMs, running and paused, as `onctl ls
// -o json` prints them. A provider that fails to list (not set up on this
// host) adds none and isn't among Facts.Providers, without spoiling the
// others' report.
func (a *Agent) reportVMs(ctx context.Context) {
	if a.cfg.ReportInterval == 0 {
		return
	}
	a.reportMu.Lock()
	defer a.reportMu.Unlock()
	vms := []cloud.Vm{}
	answered := []string{}
	for _, p := range a.cfg.ReportProviders {
		c, err := a.cfg.NewClient(p)
		if err != nil {
			continue
		}
		running, paused, err := c.List()
		if err != nil {
			continue
		}
		answered = append(answered, p)
		vms = append(append(vms, running.List...), paused.List...)
	}
	a.mu.Lock()
	a.providers = answered
	a.mu.Unlock()
	if _, _, err := a.post(ctx, "/vms-report", vms, 15*time.Second); err != nil {
		a.cfg.Logf("vms-report failed: %v", err)
	}
}

func (a *Agent) lastProviders() []string {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.providers
}

// post sends body as JSON to the host's path and returns the status and
// reply body.
func (a *Agent) post(ctx context.Context, path string, body any, timeout time.Duration) (int, []byte, error) {
	data, err := json.Marshal(body)
	if err != nil {
		return 0, nil, err
	}
	return a.do(ctx, http.MethodPost, path, data, timeout)
}

func (a *Agent) do(ctx context.Context, method, path string, data []byte, timeout time.Duration) (int, []byte, error) {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	var body io.Reader
	if data != nil {
		body = bytes.NewReader(data)
	}
	req, err := http.NewRequestWithContext(ctx, method, a.cfg.ServerURL+"/hosts/"+a.cfg.Host+path, body)
	if err != nil {
		return 0, nil, err
	}
	req.Header.Set("Authorization", "Bearer "+a.cfg.Token)
	if data != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := a.cfg.HTTP.Do(req)
	if err != nil {
		return 0, nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	reply, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	return resp.StatusCode, reply, err
}

// sleep waits d, or less if ctx ends first; it reports whether ctx is
// still going.
func sleep(ctx context.Context, d time.Duration) bool {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-t.C:
		return true
	}
}

var errNoJob = errors.New("no job waiting")
