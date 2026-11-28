package hostagent

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/cdalar/onctl/pkg/box"
	"github.com/cdalar/onctl/pkg/cloud"
)

// The typed ops a Job may carry instead of Bin and Args, run through
// pkg/box in this process rather than as an onctl command whose output
// the server parses. An agent that runs them reports the "typed-jobs"
// capability; a server sends them only to agents that do.
const (
	OpCreate  = "create"  // VM (+ CPUSet); then Template with Vars, once ssh answers
	OpList    = "list"    // Result.VMs: running and paused
	OpDestroy = "destroy" // Name
	OpPause   = "pause"   // Name, Hot
	OpResume  = "resume"  // Name; Result.VM
	OpRestart = "restart" // Name, KernelImage; Result.VM
	OpExec    = "exec"    // Name, Command; Result.Output and ExitCode
)

// typedReadyTimeout bounds how long a create waits for ssh before running
// its template.
var typedReadyTimeout = 5 * time.Minute

// runTyped runs j's Op through pkg/box.
func (a *Agent) runTyped(ctx context.Context, j Job) Result {
	c, err := a.cfg.NewClient(j.provider())
	if err != nil {
		return Result{Error: err.Error()}
	}
	var res Result
	switch j.Op {
	case OpCreate:
		res, err = a.create(c, j)
	case OpList:
		var running, paused cloud.VmList
		running, paused, err = c.List()
		res.VMs = append(append([]cloud.Vm{}, running.List...), paused.List...)
	case OpDestroy:
		err = c.Destroy(j.Name)
	case OpPause:
		err = c.Pause(j.Name, j.Hot)
	case OpResume:
		var keyID string
		if keyID, err = addKey(c); err == nil {
			var vm cloud.Vm
			vm, err = c.Resume(j.Name, keyID)
			res.VM = &vm
		}
	case OpRestart:
		var vm cloud.Vm
		vm, err = c.Restart(j.Name, cloud.RestartOptions{KernelImage: j.KernelImage})
		res.VM = &vm
	case OpExec:
		var out bytes.Buffer
		res.ExitCode, err = c.SSH(j.Name, 0, "", j.Command, cloud.Stdio{In: &bytes.Buffer{}, Out: &out, Err: &out})
		res.Output = out.Bytes()
		if err == nil && res.ExitCode != 0 {
			res.Error = fmt.Sprintf("exit status %d", res.ExitCode)
		}
	default:
		err = fmt.Errorf("unknown op %q", j.Op)
	}
	if err != nil {
		res.Error = err.Error()
	}
	return res
}

// create makes j.VM with this host's ssh key on it and, when j has a
// Template, runs it once the VM answers ssh, its output the Result's.
// A template that fails leaves the VM: the server decides what to do
// with a box whose setup didn't finish.
func (a *Agent) create(c *box.Client, j Job) (Result, error) {
	if j.VM == nil {
		return Result{}, errors.New("create needs a vm")
	}
	vm := *j.VM
	if j.CPUSet != "" {
		vm.CPUSet = j.CPUSet
	}
	keyID, err := addKey(c)
	if err != nil {
		return Result{}, err
	}
	vm.SSHKeyID = keyID
	created, err := c.Create(vm)
	if err != nil {
		return Result{}, err
	}
	res := Result{VM: &created}
	if j.Template == "" {
		return res, nil
	}
	file, err := box.ResolveFile(j.Template)
	if err != nil {
		return res, err
	}
	conn, err := c.Connect(created, box.ConnectOptions{})
	if err != nil {
		return res, err
	}
	defer func() { _ = conn.Close() }()
	var out bytes.Buffer
	err = c.Provision(conn, box.Setup{ReadyTimeout: typedReadyTimeout, ApplyFiles: []string{file}, Vars: j.Vars, Out: &out})
	res.Output = out.Bytes()
	return res, err
}

// addKey puts this host's configured ssh key on the provider (for fc,
// the public key file Create writes into the rootfs).
func addKey(c *box.Client) (string, error) {
	pub, _ := box.KeyFiles("")
	return c.AddSSHKey(pub)
}

func (j Job) provider() string {
	if j.Provider != "" {
		return j.Provider
	}
	return "fc"
}
