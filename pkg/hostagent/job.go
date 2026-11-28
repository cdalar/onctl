package hostagent

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"

	"github.com/cdalar/onctl/pkg/cloud"
)

// Job is one thing the server wants done on this host. Either a command,
// Bin with Args, Env added to this process's environment and pinned to
// CPUSet (taskset -c) when set; or a typed Op (see OpCreate) run through
// pkg/box, with the fields that op takes.
type Job struct {
	ID     string            `json:"id"`
	Bin    string            `json:"bin,omitempty"`
	Args   []string          `json:"args,omitempty"`
	Env    map[string]string `json:"env,omitempty"`
	CPUSet string            `json:"cpuset,omitempty"`

	Op          string    `json:"op,omitempty"`
	Provider    string    `json:"provider,omitempty"` // default fc
	Name        string    `json:"name,omitempty"`
	VM          *cloud.Vm `json:"vm,omitempty"`
	Template    string    `json:"template,omitempty"`
	Vars        []string  `json:"vars,omitempty"`
	Command     []string  `json:"command,omitempty"`
	Hot         bool      `json:"hot,omitempty"`
	KernelImage string    `json:"kernel_image,omitempty"`
}

// Result is what a Job did: a command's combined stdout and stderr, and
// "exit status N" when it failed; a typed op's failure, and the VM or
// VMs it returns. Output is base64 on the wire ([]byte in JSON), as the
// server's side expects.
type Result struct {
	Output   []byte     `json:"output"`
	Error    string     `json:"error"`
	VM       *cloud.Vm  `json:"vm,omitempty"`
	VMs      []cloud.Vm `json:"vms,omitempty"`
	ExitCode int        `json:"exit_code,omitempty"`
}

// vmName is the VM a job acts on (`create -n NAME`, `destroy NAME`, ...),
// or "" when it acts on none (ls).
func (j Job) vmName() string {
	if j.Op != "" {
		if j.VM != nil {
			return j.VM.Name
		}
		return j.Name
	}
	if len(j.Args) == 0 {
		return ""
	}
	switch j.Args[0] {
	case "create":
		for i, a := range j.Args {
			if (a == "-n" || a == "--name") && i+1 < len(j.Args) {
				return j.Args[i+1]
			}
		}
	case "destroy", "pause", "resume", "restart", "ssh":
		if len(j.Args) > 1 {
			return j.Args[1]
		}
	}
	return ""
}

// logLine is the job's command with every -e/--env KEY=VALUE value
// masked: for a runner's create that's its GitHub credential, and this
// line lands in the host's journal.
func (j Job) logLine() string {
	if j.Op != "" {
		return j.Op + " " + j.vmName()
	}
	parts := []string{j.Bin}
	for i, a := range j.Args {
		if i > 0 && (j.Args[i-1] == "-e" || j.Args[i-1] == "--env") && strings.Contains(a, "=") {
			a = a[:strings.Index(a, "=")] + "=<redacted>"
		}
		parts = append(parts, a)
	}
	return strings.Join(parts, " ")
}

// jobLoop claims jobs while ctx lasts, at most MaxJobs running at once
// (one isn't claimed without room for it: an unclaimed job stays in the
// server's queue, where a cancelled one is still skipped). Jobs for the
// same VM run one after another, in the order claimed, so the destroy
// the server sends after giving up on a slow create can't finish first
// and leave the create's VM untracked. Returns once ctx is done and
// every running job has reported.
func (a *Agent) jobLoop(ctx context.Context) {
	slots := make(chan struct{}, a.cfg.MaxJobs)
	var wg sync.WaitGroup
	defer wg.Wait()
	last := map[string]chan struct{}{} // VM name -> done of the last job claimed for it
	a.cfg.Logf("polling for work (up to %d jobs at once)", a.cfg.MaxJobs)
	for {
		select {
		case slots <- struct{}{}:
		case <-ctx.Done():
			return
		}
		job, err := a.next(ctx)
		if err != nil {
			<-slots
			if ctx.Err() != nil {
				return
			}
			if err != errNoJob {
				a.cfg.Logf("jobs/next: %v, retrying in 5s", err)
				sleep(ctx, 5*time.Second)
			}
			continue
		}
		done := make(chan struct{})
		var after chan struct{}
		if name := job.vmName(); name != "" {
			after = last[name]
			last[name] = done
		}
		wg.Add(1)
		go func() {
			defer wg.Done()
			defer func() { <-slots }()
			defer close(done)
			if after != nil {
				a.cfg.Logf("job %s: waiting for an earlier job on the same VM", job.ID)
				<-after
			}
			// Not ctx: a job that started is let finish and report,
			// as it was when the bash agent's jobs outlived it.
			a.runJob(context.WithoutCancel(ctx), job)
		}()
		for name, ch := range last {
			select {
			case <-ch:
				delete(last, name)
			default:
			}
		}
	}
}

// next long-polls for a job: errNoJob on 204.
func (a *Agent) next(ctx context.Context) (Job, error) {
	status, body, err := a.do(ctx, http.MethodGet, "/jobs/next", nil, 40*time.Second)
	if err != nil {
		return Job{}, err
	}
	switch status {
	case http.StatusOK:
		var j Job
		if err := json.Unmarshal(body, &j); err != nil {
			return Job{}, fmt.Errorf("decoding job: %w", err)
		}
		return j, nil
	case http.StatusNoContent:
		return Job{}, errNoJob
	}
	return Job{}, fmt.Errorf("http %d", status)
}

// runJob runs j, posts its Result, destroys what a create made if the
// server had already given up on it, and pushes a fresh VM list.
func (a *Agent) runJob(ctx context.Context, j Job) {
	a.cfg.Logf("running job %s: %s", j.ID, j.logLine())
	var res Result
	if j.Op != "" {
		res = a.runTyped(ctx, j)
	} else {
		out, err := j.command(ctx).CombinedOutput()
		res = Result{Output: out}
		if err != nil {
			res.Error = "exit status " + exitStatus(err)
		}
	}
	if res.Error != "" {
		a.cfg.Logf("job %s failed: %s", j.ID, res.Error)
	} else {
		a.cfg.Logf("job %s: finished", j.ID)
	}
	status, reply, postErr := a.post(ctx, "/jobs/"+j.ID+"/result", res, 15*time.Second)
	a.cfg.Logf("job %s: result posted, http=%d err=%v", j.ID, status, postErr)

	var answer struct {
		Abandoned bool `json:"abandoned"`
	}
	_ = json.Unmarshal(reply, &answer)
	// A successful create the server gave up on (its ctx ran out, it
	// restarted) is a VM nobody tracks: names are never reused, so
	// destroy it here.
	if res.Error == "" && answer.Abandoned && j.Op == OpCreate && j.VM != nil {
		a.cfg.Logf("job %s: the server abandoned the create of %s; destroying it", j.ID, j.VM.Name)
		if c, err := a.cfg.NewClient(j.provider()); err == nil {
			if err := c.Destroy(j.VM.Name); err != nil {
				a.cfg.Logf("job %s: destroying %s failed: %v", j.ID, j.VM.Name, err)
			}
		}
	}
	if res.Error == "" && answer.Abandoned && len(j.Args) > 0 && j.Args[0] == "create" {
		if name := j.vmName(); name != "" {
			a.cfg.Logf("job %s: the server abandoned the create of %s; destroying it", j.ID, name)
			cleanup := Job{Bin: j.Bin, Args: []string{"destroy", name, "-f"}, Env: j.Env}
			if out, err := cleanup.command(ctx).CombinedOutput(); err != nil {
				a.cfg.Logf("job %s: destroying %s failed: %v: %s", j.ID, name, err, out)
			}
		}
	}
	a.reportVMs(ctx)
}

func (j Job) command(ctx context.Context) *exec.Cmd {
	name, args := j.Bin, j.Args
	if j.CPUSet != "" {
		name, args = "taskset", append([]string{"-c", j.CPUSet, j.Bin}, j.Args...)
	}
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Env = os.Environ()
	for k, v := range j.Env {
		cmd.Env = append(cmd.Env, k+"="+v)
	}
	return cmd
}

func exitStatus(err error) string {
	if ee, ok := err.(*exec.ExitError); ok {
		return fmt.Sprint(ee.ExitCode())
	}
	return "127: " + err.Error()
}
