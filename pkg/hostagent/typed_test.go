package hostagent

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/cdalar/onctl/pkg/box"
	"github.com/cdalar/onctl/pkg/cloud"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// typedFake is a provider recording what typed jobs asked of it.
type typedFake struct {
	mu       sync.Mutex
	vms      []cloud.Vm
	deployed []cloud.Vm
	calls    []string
}

func (f *typedFake) record(s string) { f.mu.Lock(); f.calls = append(f.calls, s); f.mu.Unlock() }

func (f *typedFake) Deploy(v cloud.Vm) (cloud.Vm, error) {
	f.record("deploy " + v.Name)
	v.IP, v.Status = "10.0.0.2", "running"
	f.mu.Lock()
	f.deployed = append(f.deployed, v)
	f.vms = append(f.vms, v)
	f.mu.Unlock()
	return v, nil
}
func (f *typedFake) Destroy(v cloud.Vm) error { f.record("destroy " + v.Name); return nil }
func (f *typedFake) Pause(v cloud.Vm, hot bool) error {
	f.record("pause " + v.Name + map[bool]string{true: " hot", false: ""}[hot])
	return nil
}
func (f *typedFake) Resume(v cloud.Vm) (cloud.Vm, error) {
	f.record("resume " + v.Name + " key=" + v.SSHKeyID)
	return cloud.Vm{Name: v.Name, IP: "10.0.0.9"}, nil
}
func (f *typedFake) List() (cloud.VmList, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return cloud.VmList{List: append([]cloud.Vm{}, f.vms...)}, nil
}
func (f *typedFake) ListPaused() (cloud.VmList, error) {
	return cloud.VmList{List: []cloud.Vm{{Name: "snap", Status: "paused"}}}, nil
}
func (f *typedFake) CreateSSHKey(pub string) (string, error) { return pub, nil }
func (f *typedFake) SSHInto(name string, port int, key string, command []string, stdio cloud.Stdio) error {
	if name == "missing" {
		return errors.New("no VM missing")
	}
	sh := exec.Command("sh", "-c", strings.Join(command, " "))
	sh.Stdout, sh.Stderr = stdio.Out, stdio.Err
	return sh.Run()
}
func (f *typedFake) GetByName(name string) (cloud.Vm, error) { return cloud.Vm{Name: name}, nil }
func (f *typedFake) Restart(v cloud.Vm, opts cloud.RestartOptions) (cloud.Vm, error) {
	f.record("restart " + v.Name + " kernel=" + opts.KernelImage)
	return cloud.Vm{Name: v.Name, IP: "10.0.0.3"}, nil
}

func typedConfig(t *testing.T, url string, p *typedFake) Config {
	t.Helper()
	dir := t.TempDir()
	pub := filepath.Join(dir, "id_ed25519.pub")
	require.NoError(t, os.WriteFile(pub, []byte("ssh-ed25519 AAAA test"), 0o644))
	require.NoError(t, os.WriteFile(strings.TrimSuffix(pub, ".pub"), []byte("key"), 0o600))
	viper.Set("ssh.publicKey", pub)
	viper.Set("ssh.privateKey", strings.TrimSuffix(pub, ".pub"))
	t.Cleanup(func() { viper.Set("ssh.publicKey", nil); viper.Set("ssh.privateKey", nil) })
	cfg := testConfig(url)
	cfg.NewClient = func(id string) (*box.Client, error) { return box.Wrap(id, p), nil }
	return cfg
}

// Typed jobs are done through pkg/box in the agent's own process, and
// come back as values (a VM, the VM list, an exit code) instead of CLI
// output the server has to parse.
func TestTypedJobs(t *testing.T) {
	p := &typedFake{vms: []cloud.Vm{{Name: "web", Status: "running"}}}
	f, srv := newFakeServer(t,
		Job{ID: "list", Op: OpList},
		Job{ID: "create", Op: OpCreate, VM: &cloud.Vm{Name: "box1", Type: "2vcpu-4096mb", Image: "/opt/fc/images/ubuntu.ext4", KernelImage: "/opt/fc/images/boxctl-kernel.vmlinux", DiskMiB: 20480}, CPUSet: "0-1"},
		Job{ID: "pause", Op: OpPause, Name: "box1", Hot: true},
		Job{ID: "resume", Op: OpResume, Name: "box1"},
		Job{ID: "restart", Op: OpRestart, Name: "box1", KernelImage: "/k2"},
		Job{ID: "exec", Op: OpExec, Name: "box1", Command: []string{"echo synced; exit 4"}},
		Job{ID: "exec-missing", Op: OpExec, Name: "missing", Command: []string{"sync"}},
		Job{ID: "destroy", Op: OpDestroy, Name: "box1"},
		Job{ID: "bad", Op: "reboot"},
	)
	cfg := typedConfig(t, srv.URL, p)
	cfg.MaxJobs = 1 // one at a time, so the calls below are in the jobs' order
	stop := runAgent(t, cfg)
	f.waitResults(t, 9)
	stop()

	r := f.results
	assert.Equal(t, []string{"web", "snap"}, names(r["list"].VMs), "running and paused")
	assert.Empty(t, r["list"].Error)

	require.NotNil(t, r["create"].VM)
	assert.Equal(t, "10.0.0.2", r["create"].VM.IP)
	require.Len(t, p.deployed, 1)
	d := p.deployed[0]
	assert.Equal(t, "/opt/fc/images/boxctl-kernel.vmlinux", d.KernelImage)
	assert.Equal(t, int64(20480), d.DiskMiB)
	assert.Equal(t, "0-1", d.CPUSet, "the job's cpuset pins the VM")
	assert.True(t, strings.HasSuffix(d.SSHKeyID, "id_ed25519.pub"), "the host's key goes on the VM: %s", d.SSHKeyID)

	assert.Equal(t, "10.0.0.9", r["resume"].VM.IP)
	assert.Equal(t, "10.0.0.3", r["restart"].VM.IP)

	assert.Equal(t, "synced\n", string(r["exec"].Output))
	assert.Equal(t, 4, r["exec"].ExitCode)
	assert.Equal(t, "exit status 4", r["exec"].Error)
	assert.Contains(t, r["exec-missing"].Error, "no VM missing")

	assert.Contains(t, r["bad"].Error, `unknown op "reboot"`)
	assert.Equal(t, []string{
		"deploy box1", "pause box1 hot", "resume box1 key=" + d.SSHKeyID, "restart box1 kernel=/k2", "destroy box1",
	}, p.calls)
}

// A typed create the server gave up on is destroyed through pkg/box, as
// a command create is with `onctl destroy`.
func TestAbandonedTypedCreateIsDestroyed(t *testing.T) {
	p := &typedFake{}
	f, srv := newFakeServer(t, Job{ID: "c", Op: OpCreate, VM: &cloud.Vm{Name: "box2"}})
	f.abandon["c"] = true
	stop := runAgent(t, typedConfig(t, srv.URL, p))
	f.waitResults(t, 1)
	stop()
	assert.Equal(t, []string{"deploy box2", "destroy box2"}, p.calls)
}

// The agent says it runs typed jobs, so a server knows it can send them.
func TestReportsTypedJobsCapability(t *testing.T) {
	assert.Contains(t, New(testConfig("http://x")).facts(nil).Capabilities, "typed-jobs")
}

func names(vms []cloud.Vm) []string {
	var out []string
	for _, v := range vms {
		out = append(out, v.Name)
	}
	return out
}
