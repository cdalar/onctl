package hostagent

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/cdalar/onctl/pkg/box"
	"github.com/cdalar/onctl/pkg/cloud"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fakeServer is the control plane's side of the protocol: it hands out
// queued jobs, records results, facts and VM reports.
type fakeServer struct {
	t         *testing.T
	mu        sync.Mutex
	jobs      []Job
	results   map[string]Result
	registers int
	failFirst int // register attempts answered 503 before one is accepted
	facts     []Facts
	reports   [][]cloud.Vm
	abandon   map[string]bool
	resultCh  chan string
}

func newFakeServer(t *testing.T, jobs ...Job) (*fakeServer, *httptest.Server) {
	f := &fakeServer{t: t, jobs: jobs, results: map[string]Result{}, abandon: map[string]bool{}, resultCh: make(chan string, 16)}
	srv := httptest.NewServer(http.HandlerFunc(f.serve))
	t.Cleanup(srv.Close)
	return f, srv
}

func (f *fakeServer) serve(w http.ResponseWriter, r *http.Request) {
	if r.Header.Get("Authorization") != "Bearer tok" {
		w.WriteHeader(http.StatusUnauthorized)
		return
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	path := strings.TrimPrefix(r.URL.Path, "/hosts/h1")
	switch {
	case path == "/register":
		f.registers++
		var facts Facts
		require.NoError(f.t, json.NewDecoder(r.Body).Decode(&facts))
		f.facts = append(f.facts, facts)
		if f.registers <= f.failFirst {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	case path == "/heartbeat":
		var facts Facts
		_ = json.NewDecoder(r.Body).Decode(&facts)
		f.facts = append(f.facts, facts)
		w.WriteHeader(http.StatusNoContent)
	case path == "/vms-report":
		var vms []cloud.Vm
		require.NoError(f.t, json.NewDecoder(r.Body).Decode(&vms))
		f.reports = append(f.reports, vms)
		w.WriteHeader(http.StatusNoContent)
	case path == "/jobs/next":
		if len(f.jobs) == 0 {
			f.mu.Unlock()
			time.Sleep(20 * time.Millisecond) // a short long-poll
			f.mu.Lock()
			w.WriteHeader(http.StatusNoContent)
			return
		}
		j := f.jobs[0]
		f.jobs = f.jobs[1:]
		_ = json.NewEncoder(w).Encode(j)
	case strings.HasPrefix(path, "/jobs/") && strings.HasSuffix(path, "/result"):
		id := strings.TrimSuffix(strings.TrimPrefix(path, "/jobs/"), "/result")
		var res Result
		require.NoError(f.t, json.NewDecoder(r.Body).Decode(&res))
		f.results[id] = res
		_ = json.NewEncoder(w).Encode(map[string]bool{"abandoned": f.abandon[id]})
		f.resultCh <- id
	default:
		w.WriteHeader(http.StatusNotFound)
	}
}

func (f *fakeServer) waitResults(t *testing.T, n int) {
	t.Helper()
	for i := 0; i < n; i++ {
		select {
		case <-f.resultCh:
		case <-time.After(10 * time.Second):
			t.Fatalf("only %d of %d results arrived", i, n)
		}
	}
}

func testConfig(url string) Config {
	return Config{
		ServerURL: url, Host: "h1", Token: "tok", Version: "test",
		HeartbeatInterval: time.Hour, MaxJobs: 4, StateDir: os.TempDir(),
		NewClient: func(string) (*box.Client, error) { return nil, errors.New("no providers in this test") },
		Logf:      func(string, ...any) {},
	}
}

func runAgent(t *testing.T, cfg Config) (stop func()) {
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { _ = New(cfg).Run(ctx); close(done) }()
	return func() { cancel(); <-done }
}

// A job is the command it names, with its env, and its result is the
// command's combined output and "exit status N" -- what the server's
// side parses, exactly as the bash agents sent it.
func TestRunsJobsAndReportsTheirResult(t *testing.T) {
	f, srv := newFakeServer(t,
		Job{ID: "ok", Bin: "sh", Args: []string{"-c", "echo out; echo err >&2"}},
		Job{ID: "fails", Bin: "sh", Args: []string{"-c", "echo $GREETING; exit 3"}, Env: map[string]string{"GREETING": "hi"}},
	)
	stop := runAgent(t, testConfig(srv.URL))
	f.waitResults(t, 2)
	stop()

	assert.Equal(t, Result{Output: []byte("out\nerr\n")}, f.results["ok"])
	assert.Equal(t, Result{Output: []byte("hi\n"), Error: "exit status 3"}, f.results["fails"])
}

// Registration is retried until the server accepts it: a host and its
// server don't come up in a fixed order.
func TestRegisterRetriesUntilAccepted(t *testing.T) {
	f, srv := newFakeServer(t, Job{ID: "j", Bin: "true"})
	f.failFirst = 1
	cfg := testConfig(srv.URL)
	stop := runAgent(t, cfg)
	defer stop()
	// The retry waits 5s; the job only runs once registered.
	f.waitResults(t, 1)
	f.mu.Lock()
	defer f.mu.Unlock()
	assert.Equal(t, 2, f.registers)
	assert.Equal(t, "test", f.facts[0].Version)
}

// Two jobs for the same VM run one after the other, in the order the
// server sent them, even with room for both: otherwise the destroy sent
// after giving up on a slow create could run first and leave the
// create's VM untracked.
func TestJobsForOneVMRunInOrder(t *testing.T) {
	// The VM name comes from the args (create -n NAME, destroy NAME), so
	// the jobs run "create" and "destroy" scripts put on PATH.
	logFile := filepath.Join(t.TempDir(), "log")
	f, srv := newFakeServer(t,
		Job{ID: "create", Bin: "create", Args: []string{"create", "-n", "vm1"}, Env: map[string]string{"LOG": logFile}},
		Job{ID: "destroy", Bin: "destroy", Args: []string{"destroy", "vm1"}, Env: map[string]string{"LOG": logFile}},
	)
	dir := t.TempDir()
	writeScript(t, filepath.Join(dir, "create"), "sleep 0.3; echo create >> \"$LOG\"")
	writeScript(t, filepath.Join(dir, "destroy"), "echo destroy >> \"$LOG\"")
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))

	stop := runAgent(t, testConfig(srv.URL))
	f.waitResults(t, 2)
	stop()
	got, err := os.ReadFile(logFile)
	require.NoError(t, err)
	assert.Equal(t, "create\ndestroy\n", string(got))
}

// A create the server gave up on while it ran is destroyed by the agent
// once it succeeds: VM names are never reused, so nobody else would.
func TestAbandonedCreateIsDestroyed(t *testing.T) {
	dir := t.TempDir()
	logFile := filepath.Join(dir, "calls")
	bin := filepath.Join(dir, "fake-onctl")
	writeScript(t, bin, "echo \"$@\" >> "+logFile)
	f, srv := newFakeServer(t, Job{ID: "c1", Bin: bin, Args: []string{"create", "-n", "runner-7"}})
	f.abandon["c1"] = true

	stop := runAgent(t, testConfig(srv.URL))
	f.waitResults(t, 1)
	stop() // waits for the job, cleanup included
	got, err := os.ReadFile(logFile)
	require.NoError(t, err)
	assert.Equal(t, "create -n runner-7\ndestroy runner-7 -f\n", string(got))
}

// The VM report is `onctl ls -o json`'s shape for the providers that
// answered, running and paused together; a provider that can't list
// adds nothing and isn't reported among the host's working providers.
func TestReportsVMsOfProvidersThatAnswer(t *testing.T) {
	f, srv := newFakeServer(t)
	cfg := testConfig(srv.URL)
	cfg.ReportInterval = time.Hour
	cfg.ReportProviders = []string{"fc", "ch"}
	cfg.NewClient = func(p string) (*box.Client, error) {
		if p == "ch" {
			return nil, errors.New("ch isn't set up here")
		}
		return box.Wrap("fc", &listProvider{running: []cloud.Vm{{Name: "a", Status: "running"}}, paused: []cloud.Vm{{Name: "b", Status: "paused"}}}), nil
	}
	a := New(cfg)
	a.reportVMs(context.Background())

	require.Len(t, f.reports, 1)
	assert.Equal(t, []cloud.Vm{{Name: "a", Status: "running"}, {Name: "b", Status: "paused"}}, f.reports[0])
	assert.Equal(t, []string{"fc"}, a.facts(a.lastProviders()).Providers)
	assert.Nil(t, New(cfg).facts(nil).Providers, "before any report the providers aren't known: null, not []")
}

// A runner's create carries its GitHub credential as -e JIT_CONFIG=...;
// the line logged for it keeps the key and hides the value.
func TestLogLineMasksEnvValues(t *testing.T) {
	j := Job{Bin: "onctl", Args: []string{"create", "-n", "r1", "-e", "JIT_CONFIG=secret", "--env", "A=b=c"}}
	assert.Equal(t, "onctl create -n r1 -e JIT_CONFIG=<redacted> --env A=<redacted>", j.logLine())
}

// Images are reported by path with the sha256 their sidecar records,
// never hashed; a sidecar whose size no longer matches is stale and the
// image's sha unknown. Previous versions kept in .generations are
// reported by sha, only with a valid sidecar.
func TestImagesFromSidecars(t *testing.T) {
	dir := t.TempDir()
	write := func(path, content string) { require.NoError(t, os.WriteFile(path, []byte(content), 0o644)) }
	write(filepath.Join(dir, "ubuntu.ext4"), "12345")
	write(filepath.Join(dir, "ubuntu.ext4.sha256sum"), "aaa 5 ubuntu.ext4\n")
	write(filepath.Join(dir, "stale.ext4"), "123")
	write(filepath.Join(dir, "stale.ext4.sha256sum"), "bbb 99\n")
	write(filepath.Join(dir, "kernel.vmlinux"), "k")
	write(filepath.Join(dir, "notes.txt"), "not an image")
	require.NoError(t, os.MkdirAll(filepath.Join(dir, ".generations"), 0o755))
	write(filepath.Join(dir, ".generations", "old.ext4"), "1234")
	write(filepath.Join(dir, ".generations", "old.ext4.sha256sum"), "ccc 4\n")

	current, generations := images([]string{dir, filepath.Join(dir, "missing")})
	assert.Equal(t, map[string]string{
		filepath.Join(dir, "ubuntu.ext4"):    "aaa",
		filepath.Join(dir, "stale.ext4"):     "",
		filepath.Join(dir, "kernel.vmlinux"): "",
	}, current)
	assert.Equal(t, map[string]string{"ccc": filepath.Join(dir, ".generations", "old.ext4")}, generations)
}

func writeScript(t *testing.T, path, body string) {
	t.Helper()
	require.NoError(t, os.WriteFile(path, []byte("#!/bin/sh\n"+body+"\n"), 0o755))
}

type listProvider struct {
	cloud.CloudProviderInterface
	running, paused []cloud.Vm
}

func (p *listProvider) List() (cloud.VmList, error)       { return cloud.VmList{List: p.running}, nil }
func (p *listProvider) ListPaused() (cloud.VmList, error) { return cloud.VmList{List: p.paused}, nil }
