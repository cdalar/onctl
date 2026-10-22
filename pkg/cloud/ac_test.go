package cloud

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fakeACRunner is a test double for ACRunner that keeps an in-memory
// container table shaped like `container ls --format json` prints it, and
// records every CLI call so tests can check what onctl asked for.
type fakeACRunner struct {
	calls      [][]string
	containers []map[string]any
	// failOn maps a subcommand ("run", "stop", ...) to the error it fails with.
	failOn map[string]error
	// ipAfterLists withholds addresses until `ls` has been called that many
	// times, to exercise the IP poll.
	ipAfterLists int
	// stopOnList, when non-zero, marks every running container stopped on
	// that `ls` call, as if its init process had exited by then.
	stopOnList int
	lists      int
	// logs is what `container logs` prints for any container.
	logs string
}

func (f *fakeACRunner) Run(args ...string) ([]byte, error) {
	f.calls = append(f.calls, args)
	if err := f.failOn[args[0]]; err != nil {
		return nil, err
	}
	switch args[0] {
	case "ls":
		f.lists++
		if f.lists == f.stopOnList {
			for _, c := range f.containers {
				if c["status"] == "running" {
					c["status"] = "stopped"
					c["networks"] = []any{}
				}
			}
		}
		// Hand out addresses to running containers once the poll has gone
		// around enough times.
		for _, c := range f.containers {
			if c["status"] == "running" && f.lists > f.ipAfterLists {
				c["networks"] = []map[string]any{{"address": "192.168.65.2/24", "gateway": "192.168.65.1", "network": "default"}}
			}
		}
		return json.Marshal(f.containers)
	case "run":
		c := map[string]any{"status": "running", "networks": []any{}}
		cfg := map[string]any{"labels": map[string]string{}, "resources": map[string]any{}, "image": map[string]any{}}
		labels := cfg["labels"].(map[string]string)
		res := cfg["resources"].(map[string]any)
		for i := 1; i < len(args); i++ {
			switch args[i] {
			case "--name":
				cfg["id"] = args[i+1]
				i++
			case "--label":
				kv := strings.SplitN(args[i+1], "=", 2)
				labels[kv[0]] = kv[1]
				i++
			case "--cpus":
				n, _ := strconv.ParseInt(args[i+1], 10, 64)
				res["cpus"] = n
				i++
			case "--memory":
				n, _ := strconv.ParseInt(strings.TrimSuffix(args[i+1], "M"), 10, 64)
				res["memoryInBytes"] = n << 20
				i++
			case "--env", "--entrypoint", "--detach":
				if args[i] != "--detach" {
					i++
				}
			default:
				// First positional is the image; the rest are its args.
				cfg["image"].(map[string]any)["reference"] = args[i]
				i = len(args)
			}
		}
		c["configuration"] = cfg
		f.containers = append(f.containers, c)
		return nil, nil
	case "stop":
		return nil, f.setStatus(args[1], "stopped")
	case "start":
		return nil, f.setStatus(args[1], "running")
	case "logs":
		return []byte(f.logs), nil
	case "rm":
		for i, c := range f.containers {
			if c["configuration"].(map[string]any)["id"] == args[1] {
				f.containers = append(f.containers[:i], f.containers[i+1:]...)
				return nil, nil
			}
		}
		return nil, errors.New("not found")
	}
	return nil, errors.New("unexpected: " + strings.Join(args, " "))
}

func (f *fakeACRunner) setStatus(name, status string) error {
	for _, c := range f.containers {
		if c["configuration"].(map[string]any)["id"] == name {
			c["status"] = status
			if status == "stopped" {
				c["networks"] = []any{}
			}
			return nil
		}
	}
	return errors.New("not found")
}

// addForeign adds a container onctl didn't make, as the operator's own
// `container run` would.
func (f *fakeACRunner) addForeign(name string) {
	f.containers = append(f.containers, map[string]any{
		"status":        "running",
		"configuration": map[string]any{"id": name, "labels": map[string]string{}, "resources": map[string]any{}, "image": map[string]any{}},
		"networks":      []any{},
	})
}

func (f *fakeACRunner) callsTo(sub string) [][]string {
	var out [][]string
	for _, c := range f.calls {
		if c[0] == sub {
			out = append(out, c)
		}
	}
	return out
}

func newTestAC(t *testing.T, f *fakeACRunner) ProviderAC {
	t.Helper()
	return ProviderAC{
		Config:         ACConfig{Image: "docker.io/library/ubuntu:24.04", VCPUCount: 1, MemSizeMib: 2048, Username: "root"},
		CLI:            f,
		IPTimeout:      2 * time.Second,
		IPPollInterval: time.Millisecond,
	}
}

func argAfter(args []string, flag string) string {
	for i := range args {
		if args[i] == flag && i+1 < len(args) {
			return args[i+1]
		}
	}
	return ""
}

// Deploy must hand the container everything the guest needs to be ssh-able:
// the key and user for acBootstrap, the size from --type, and the labels List
// filters on -- otherwise the sandbox boots but `onctl ssh`/`ls` can't use it.
func TestAC_Deploy(t *testing.T) {
	f := &fakeACRunner{ipAfterLists: 2}
	p := newTestAC(t, f)
	keyPath := writeTestPublicKey(t)

	vm, err := p.Deploy(Vm{Name: "sb1", Type: "2vcpu-1024mb", SSHKeyID: keyPath})
	require.NoError(t, err)
	assert.Equal(t, "sb1", vm.Name)
	assert.Equal(t, "192.168.65.2", vm.IP, "IP must come from the poll, not be guessed")
	assert.Equal(t, "2vcpu-1024mb", vm.Type)
	assert.Equal(t, "running", vm.Status)
	assert.True(t, vm.SSHReady)
	assert.Equal(t, 22, vm.SSHPort)
	assert.Equal(t, "ac", vm.Provider)
	assert.Equal(t, "docker.io/library/ubuntu:24.04", vm.Image)
	assert.WithinDuration(t, time.Now(), vm.CreatedAt, time.Minute, "createdAt label must round-trip")

	runs := f.callsTo("run")
	require.Len(t, runs, 1)
	args := runs[0]
	assert.Equal(t, "2", argAfter(args, "--cpus"))
	assert.Equal(t, "1024M", argAfter(args, "--memory"))
	assert.Equal(t, "/bin/sh", argAfter(args, "--entrypoint"))
	assert.Contains(t, args, "--detach")
	assert.Contains(t, args, acLabelManaged+"=true")
	key, err := os.ReadFile(keyPath)
	require.NoError(t, err)
	assert.Contains(t, args, acEnvPublicKey+"="+strings.TrimSpace(string(key)))
	assert.Contains(t, args, acEnvUser+"=root")
	// The image, then sh's own arguments, must come last.
	n := len(args)
	assert.Equal(t, []string{"docker.io/library/ubuntu:24.04", "-c", acBootstrap}, args[n-3:])
}

// --image picks the OCI image for just this sandbox; the configured default
// applies otherwise.
func TestAC_Deploy_ImageOverride(t *testing.T) {
	f := &fakeACRunner{}
	p := newTestAC(t, f)
	vm, err := p.Deploy(Vm{Name: "sb1", Image: "docker.io/library/alpine:3.20"})
	require.NoError(t, err)
	assert.Equal(t, "docker.io/library/alpine:3.20", vm.Image)

	p.Config.Image = ""
	_, err = p.Deploy(Vm{Name: "sb2"})
	require.Error(t, err, "no image anywhere must be refused before anything runs")
	assert.Len(t, f.callsTo("run"), 1)
}

// A second Deploy of an existing name returns it rather than starting a
// duplicate (create's own duplicate check only sees List).
func TestAC_Deploy_Existing(t *testing.T) {
	f := &fakeACRunner{}
	p := newTestAC(t, f)
	_, err := p.Deploy(Vm{Name: "sb1"})
	require.NoError(t, err)
	vm, err := p.Deploy(Vm{Name: "sb1"})
	require.NoError(t, err)
	assert.Equal(t, "sb1", vm.Name)
	assert.Len(t, f.callsTo("run"), 1)
}

// A container whose init process dies (acBootstrap failed) must not be left
// behind as a stopped sandbox a retried create would mistake for an existing
// one.
func TestAC_Deploy_BootstrapExitCleansUp(t *testing.T) {
	// The first ls is Deploy's existence check; the init process exits
	// before the poll's third look.
	f := &fakeACRunner{ipAfterLists: 1 << 30, stopOnList: 3, logs: "apk: fetch failed\nonctl: image has no sshd\n"}
	p := newTestAC(t, f)
	_, err := p.Deploy(Vm{Name: "sb1"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "exited while starting")
	// The container is removed below, so the error itself must carry the
	// output the user would otherwise read with `container logs`.
	assert.Contains(t, err.Error(), "onctl: image has no sshd")
	assert.Len(t, f.callsTo("rm"), 1)
	assert.Empty(t, f.containers)
}

// The exit error stays readable when the container printed nothing.
func TestAC_Deploy_BootstrapExitNoLogs(t *testing.T) {
	f := &fakeACRunner{ipAfterLists: 1 << 30, stopOnList: 3}
	p := newTestAC(t, f)
	_, err := p.Deploy(Vm{Name: "sb1"})
	require.Error(t, err)
	assert.Equal(t, `container "sb1" exited while starting`, err.Error())
}

func TestAC_Deploy_IPTimeoutCleansUp(t *testing.T) {
	f := &fakeACRunner{ipAfterLists: 1 << 30}
	p := newTestAC(t, f)
	p.IPTimeout = 10 * time.Millisecond
	_, err := p.Deploy(Vm{Name: "sb1"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no IP address")
	assert.Len(t, f.callsTo("rm"), 1)
}

func TestAC_Deploy_RunFails(t *testing.T) {
	f := &fakeACRunner{failOn: map[string]error{"run": errors.New("boom")}}
	p := newTestAC(t, f)
	_, err := p.Deploy(Vm{Name: "sb1"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "boom")
}

func TestAC_Deploy_RequiresName(t *testing.T) {
	_, err := newTestAC(t, &fakeACRunner{}).Deploy(Vm{})
	require.Error(t, err)
}

// List reports only onctl's own containers: the operator's other `container
// run`s on the same host are not VMs to show, destroy or ssh into.
func TestAC_List_FiltersForeignContainers(t *testing.T) {
	f := &fakeACRunner{}
	p := newTestAC(t, f)
	f.addForeign("someone-elses")
	_, err := p.Deploy(Vm{Name: "sb1"})
	require.NoError(t, err)

	list, err := p.List()
	require.NoError(t, err)
	require.Len(t, list.List, 1)
	assert.Equal(t, "sb1", list.List[0].Name)

	vm, err := p.GetByName("someone-elses")
	require.NoError(t, err)
	assert.Empty(t, vm.Name, "a foreign container is not ours to find")

	assert.Error(t, p.Destroy(Vm{Name: "someone-elses"}), "and never ours to delete")
	assert.Empty(t, f.callsTo("rm"))
}

// Pause/Resume are `container stop`/`start`: a stopped sandbox stays in
// List (status "stopped", the cross-provider paused convention) with no
// address, and comes back with one.
func TestAC_PauseResume(t *testing.T) {
	f := &fakeACRunner{}
	p := newTestAC(t, f)
	_, err := p.Deploy(Vm{Name: "sb1"})
	require.NoError(t, err)

	require.NoError(t, p.Pause(Vm{Name: "sb1"}, false))
	list, err := p.List()
	require.NoError(t, err)
	require.Len(t, list.List, 1)
	assert.Equal(t, acStatusStopped, list.List[0].Status)
	assert.Empty(t, list.List[0].IP)
	assert.False(t, list.List[0].SSHReady)
	paused, err := p.ListPaused()
	require.NoError(t, err)
	assert.Empty(t, paused.List, "stopped sandboxes are already in List; ListPaused must not double-report them")

	// Pausing twice is a no-op, not a second stop.
	require.NoError(t, p.Pause(Vm{Name: "sb1"}, true))
	assert.Len(t, f.callsTo("stop"), 1)

	vm, err := p.Resume(Vm{Name: "sb1"})
	require.NoError(t, err)
	assert.Equal(t, acStatusRunning, vm.Status)
	assert.Equal(t, "192.168.65.2", vm.IP)
	assert.Len(t, f.callsTo("start"), 1)

	// Resuming a running sandbox is a no-op too.
	_, err = p.Resume(Vm{Name: "sb1"})
	require.NoError(t, err)
	assert.Len(t, f.callsTo("start"), 1)
}

func TestAC_PauseResume_Unknown(t *testing.T) {
	p := newTestAC(t, &fakeACRunner{})
	assert.Error(t, p.Pause(Vm{Name: "nope"}, false))
	_, err := p.Resume(Vm{Name: "nope"})
	assert.Error(t, err)
}

// Destroy stops a running sandbox before deleting it (`container rm`
// refuses a running one), and skips the stop for one already stopped.
func TestAC_Destroy(t *testing.T) {
	f := &fakeACRunner{}
	p := newTestAC(t, f)
	_, err := p.Deploy(Vm{Name: "running"})
	require.NoError(t, err)
	_, err = p.Deploy(Vm{Name: "stopped"})
	require.NoError(t, err)
	require.NoError(t, p.Pause(Vm{Name: "stopped"}, false))
	f.calls = nil

	require.NoError(t, p.Destroy(Vm{Name: "running"}))
	require.NoError(t, p.Destroy(Vm{Name: "stopped"}))
	assert.Equal(t, [][]string{{"stop", "running"}}, f.callsTo("stop"))
	assert.Equal(t, [][]string{{"rm", "running"}, {"rm", "stopped"}}, f.callsTo("rm"))
	assert.Empty(t, f.containers)

	assert.Error(t, p.Destroy(Vm{Name: "nope"}))
}

func TestAC_Destroy_StopFails(t *testing.T) {
	f := &fakeACRunner{}
	p := newTestAC(t, f)
	_, err := p.Deploy(Vm{Name: "sb1"})
	require.NoError(t, err)
	f.failOn = map[string]error{"stop": errors.New("busy")}
	err = p.Destroy(Vm{Name: "sb1"})
	require.Error(t, err)
	assert.Empty(t, f.callsTo("rm"), "must not try to delete a container it couldn't stop")
}

// `container stop` can exit non-zero ("internalError: stop failed for one or
// more containers") for a container it did stop. Destroy must then go on
// and delete it rather than leave a stopped sandbox behind.
func TestAC_Destroy_StopFailsButStopped(t *testing.T) {
	f := &fakeACRunner{}
	p := newTestAC(t, f)
	_, err := p.Deploy(Vm{Name: "sb1"})
	require.NoError(t, err)
	f.calls, f.lists = nil, 0
	f.failOn = map[string]error{"stop": errors.New("internalError")}
	// Destroy lists once before the stop and once after; the container
	// reads stopped on that second look.
	f.stopOnList = 2
	require.NoError(t, p.Destroy(Vm{Name: "sb1"}))
	assert.Equal(t, [][]string{{"stop", "sb1"}}, f.callsTo("stop"))
	assert.Equal(t, [][]string{{"rm", "sb1"}}, f.callsTo("rm"))
	assert.Empty(t, f.containers)
}

// The CLI's JSON has carried the address under two names across releases;
// both must work, and a container with no network yet has no IP.
func TestAC_ParseContainers_Address(t *testing.T) {
	data := []byte(`[
	  {"status":"running","configuration":{"id":"a","labels":{"onctl.managed":"true","onctl.createdAt":"2026-10-20T10:00:00Z"},"resources":{"cpus":4,"memoryInBytes":1073741824},"image":{"reference":"img:1"}},"networks":[{"address":"192.168.65.3/24"}]},
	  {"status":"running","configuration":{"id":"b","labels":{},"resources":{"cpus":1,"memoryInBytes":536870912},"image":{"reference":"img:2"}},"networks":[{"ipv4Address":"192.168.64.9/24"}]},
	  {"status":"stopped","configuration":{"id":"c","labels":{},"resources":{},"image":{}},"networks":[]}
	]`)
	list, err := parseACContainers(data)
	require.NoError(t, err)
	require.Len(t, list, 3)

	a := mapACContainer(list[0])
	assert.Equal(t, "192.168.65.3", a.IP)
	assert.Equal(t, "4vcpu-1024mb", a.Type)
	assert.Equal(t, "img:1", a.Image)
	assert.Equal(t, time.Date(2026, 10, 20, 10, 0, 0, 0, time.UTC), a.CreatedAt)
	assert.Equal(t, "192.168.64.9", mapACContainer(list[1]).IP)
	assert.Equal(t, "1vcpu-512mb", mapACContainer(list[1]).Type)
	c := mapACContainer(list[2])
	assert.Empty(t, c.IP)
	assert.True(t, c.CreatedAt.IsZero(), "no label, no guess")
	assert.False(t, c.SSHReady)

	_, err = parseACContainers([]byte("not json"))
	assert.Error(t, err)
}

func TestAC_List_CLIError(t *testing.T) {
	p := newTestAC(t, &fakeACRunner{failOn: map[string]error{"ls": errors.New("XPC connection error")}})
	_, err := p.List()
	require.Error(t, err)
	_, err = p.GetByName("x")
	require.Error(t, err)
}

func TestAC_CreateSSHKey(t *testing.T) {
	p := newTestAC(t, &fakeACRunner{})
	keyPath := writeTestPublicKey(t)
	id, err := p.CreateSSHKey(keyPath)
	require.NoError(t, err)
	assert.True(t, filepath.IsAbs(id))

	bad := filepath.Join(t.TempDir(), "bad.pub")
	require.NoError(t, os.WriteFile(bad, []byte("garbage"), 0600))
	_, err = p.CreateSSHKey(bad)
	assert.Error(t, err)
	_, err = p.CreateSSHKey(filepath.Join(t.TempDir(), "missing.pub"))
	assert.Error(t, err)
}

// acBootstrap is shell handed to `sh -c`; keep it to what busybox sh (alpine)
// and dash (debian/ubuntu) both run, and make sure the contract with Deploy
// (the env var names) holds.
func TestAC_Bootstrap(t *testing.T) {
	assert.Contains(t, acBootstrap, "$"+acEnvPublicKey)
	assert.Contains(t, acBootstrap, "${"+acEnvUser+":-root}")
	assert.True(t, strings.HasSuffix(strings.TrimSpace(acBootstrap), "exec /usr/sbin/sshd -D -e -o PermitRootLogin=prohibit-password"), "sshd must be the container's init process so the sandbox lives as long as it does")
	for _, bashism := range []string{"[[", "function ", "$(( ", "pipefail"} {
		assert.NotContains(t, acBootstrap, bashism)
	}
	// The apply step runs scripts through `sudo -E` and tails their log with
	// GNU `tail --pid`; stock images have neither, so every package manager
	// branch must install sudo, and alpine (busybox) bash and coreutils too.
	assert.Contains(t, acBootstrap, "apk add --no-cache openssh sudo bash coreutils")
	assert.Contains(t, acBootstrap, "apt-get install -y -qq openssh-server sudo")
	assert.Contains(t, acBootstrap, "dnf install -y -q openssh-server sudo")
	assert.Contains(t, acBootstrap, "|| ! command -v sudo", "an image that ships sshd but not sudo must still get sudo")
}

// TestAC_ParseContainers_StatusObject: container CLI 1.6 moved the state
// and the networks under a "status" object; onctl must read either schema.
func TestAC_ParseContainers_StatusObject(t *testing.T) {
	list, err := parseACContainers([]byte(`[
	  {"configuration":{"id":"a","labels":{"onctl.managed":"true"},"resources":{"cpus":4,"memoryInBytes":1073741824},"image":{"reference":"img:1"}},"status":{"networks":[{"hostname":"a","ipv4Address":"192.168.64.3/24","network":"default"}],"startedDate":"2026-10-21T13:13:32Z","state":"running"}},
	  {"configuration":{"id":"b","labels":{},"resources":{},"image":{}},"status":{"networks":[],"state":"stopped"}}
	]`))
	assert.NoError(t, err)
	if assert.Len(t, list, 2) {
		assert.Equal(t, "running", list[0].Status)
		assert.Equal(t, "192.168.64.3", list[0].ip())
		assert.Equal(t, "stopped", list[1].Status)
		assert.Equal(t, "", list[1].ip())
	}
}
