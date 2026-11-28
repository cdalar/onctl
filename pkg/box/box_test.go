package box_test

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net"
	"os/exec"
	"strings"
	"testing"

	"github.com/cdalar/onctl/pkg/box"
	"github.com/cdalar/onctl/pkg/cloud"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fakeProvider is an in-memory provider: Deploy adds a VM, Destroy
// removes it, so a test drives the package the way a caller would and
// checks what the provider ends up with.
type fakeProvider struct {
	vms       []cloud.Vm
	paused    []cloud.Vm
	listErr   error
	deployed  []cloud.Vm
	destroyed []string
	calls     []string
}

func (f *fakeProvider) Deploy(v cloud.Vm) (cloud.Vm, error) {
	v.IP = "10.0.0.2"
	v.Status = "running"
	f.vms = append(f.vms, v)
	f.deployed = append(f.deployed, v)
	return v, nil
}

func (f *fakeProvider) Destroy(v cloud.Vm) error {
	f.destroyed = append(f.destroyed, v.Name)
	kept := f.vms[:0]
	for _, vm := range f.vms {
		if vm.Name != v.Name {
			kept = append(kept, vm)
		}
	}
	f.vms = kept
	return nil
}

func (f *fakeProvider) Pause(v cloud.Vm, hot bool) error {
	f.calls = append(f.calls, "pause "+v.Name)
	if hot {
		f.calls[len(f.calls)-1] += " hot"
	}
	return nil
}

func (f *fakeProvider) Resume(v cloud.Vm) (cloud.Vm, error) {
	f.calls = append(f.calls, "resume "+v.Name+" key="+v.SSHKeyID)
	return cloud.Vm{Name: v.Name, IP: "10.0.0.9"}, nil
}

func (f *fakeProvider) List() (cloud.VmList, error) {
	return cloud.VmList{List: append([]cloud.Vm{}, f.vms...)}, f.listErr
}

func (f *fakeProvider) ListPaused() (cloud.VmList, error) {
	return cloud.VmList{List: append([]cloud.Vm{}, f.paused...)}, nil
}

func (f *fakeProvider) CreateSSHKey(file string) (string, error) {
	f.calls = append(f.calls, "sshkey "+file)
	return "key-1", nil
}

// SSHInto runs command locally with sh in place of ssh, so tests see a
// real exit status and real output on stdio.
func (f *fakeProvider) SSHInto(name string, port int, key string, command []string, stdio cloud.Stdio) error {
	f.calls = append(f.calls, "ssh "+name)
	if name == "missing" {
		return errors.New("no VM found with name missing")
	}
	if len(command) == 0 {
		return nil
	}
	sh := exec.Command("sh", "-c", strings.Join(command, " "))
	sh.Stdout, sh.Stderr = stdio.Out, stdio.Err
	return sh.Run()
}

func (f *fakeProvider) GetByName(name string) (cloud.Vm, error) {
	for _, vm := range f.vms {
		if vm.Name == name {
			return vm, nil
		}
	}
	return cloud.Vm{}, errors.New("no VM " + name)
}

// snapshotProvider is a provider whose paused VMs are only snapshots
// (Hetzner's), which Destroy can't reach.
type snapshotProvider struct {
	*fakeProvider
	snapshotsDeleted []string
}

func (s *snapshotProvider) DestroySnapshot(name string) error {
	s.snapshotsDeleted = append(s.snapshotsDeleted, name)
	return nil
}

// A caller gets the whole lifecycle through the package: create, see it
// listed, destroy it, and it's gone.
func TestCreateListDestroy(t *testing.T) {
	p := &fakeProvider{}
	c := box.Wrap("fake", p)

	key, err := c.AddSSHKey("/keys/id.pub")
	require.NoError(t, err)
	vm, err := c.Create(cloud.Vm{Name: "web", SSHKeyID: key})
	require.NoError(t, err)
	assert.Equal(t, "10.0.0.2", vm.IP, "Create returns the VM as the provider made it")
	assert.Equal(t, "key-1", p.deployed[0].SSHKeyID, "the key reaches the provider")

	running, paused, err := c.List()
	require.NoError(t, err)
	assert.Equal(t, []string{"web"}, names(running))
	assert.Empty(t, paused.List)

	require.NoError(t, c.Destroy("web"))
	running, _, err = c.List()
	require.NoError(t, err)
	assert.Empty(t, running.List)
}

// Two boxes of one name would leave a caller unable to tell them apart
// (destroy, ssh and pause all go by name), so Create refuses the second.
func TestCreate_RefusesALiveName(t *testing.T) {
	p := &fakeProvider{vms: []cloud.Vm{{Name: "web", Status: "running"}}}
	_, err := box.Wrap("fake", p).Create(cloud.Vm{Name: "web"})
	assert.ErrorIs(t, err, box.ErrExists)
	assert.Empty(t, p.deployed, "nothing is deployed")
}

// A dead fc/ch microVM (its VMM gone after a host reboot) can't be
// started in place; Deploy recreates it, so its name isn't taken.
func TestCreate_RecreatesADeadMicroVM(t *testing.T) {
	for _, id := range []string{"fc", "ch"} {
		p := &fakeProvider{vms: []cloud.Vm{{Name: "web", Provider: id, Status: "dead"}}}
		_, err := box.Wrap(id, p).Create(cloud.Vm{Name: "web"})
		assert.NoError(t, err, id)
		assert.Len(t, p.deployed, 1, id)
	}
}

// A provider that can't list can still deploy; the list failure mustn't
// block a create, as it never did in the CLI.
func TestCreate_ListFailureDoesNotBlock(t *testing.T) {
	p := &fakeProvider{listErr: errors.New("no state dir yet")}
	_, err := box.Wrap("fake", p).Create(cloud.Vm{Name: "web"})
	assert.NoError(t, err)
	assert.Len(t, p.deployed, 1)
}

func TestCreate_NamesAnUnnamedVM(t *testing.T) {
	p := &fakeProvider{}
	vm, err := box.Wrap("fake", p).Create(cloud.Vm{})
	require.NoError(t, err)
	assert.Regexp(t, `^onctl-\w{5}$`, vm.Name)
}

// List splits running from paused for every provider: stopped VMs that
// List returns count as paused, and a snapshot whose server still exists
// (mid pause/resume) isn't shown twice.
func TestList_SplitsRunningAndPaused(t *testing.T) {
	p := &fakeProvider{
		vms:    []cloud.Vm{{Name: "api", Status: "initializing"}, {Name: "db", Status: "stopped"}},
		paused: []cloud.Vm{{Name: "api", Status: "paused"}, {Name: "web", Status: "paused"}},
	}
	running, paused, err := box.Wrap("fake", p).List()
	require.NoError(t, err)
	assert.Equal(t, []string{"api"}, names(running))
	assert.Equal(t, []string{"db", "web"}, names(paused))
}

// A failed List is the caller's to see, along with what did come back:
// "no VMs" and "couldn't list" must not look the same.
func TestList_ReportsFailure(t *testing.T) {
	boom := errors.New("container CLI missing")
	p := &fakeProvider{listErr: boom, paused: []cloud.Vm{{Name: "web", Status: "paused"}}}
	_, paused, err := box.Wrap("fake", p).List()
	assert.ErrorIs(t, err, boom)
	assert.Equal(t, []string{"web"}, names(paused))
}

// A Hetzner paused VM is only a snapshot: Destroy deletes the snapshot,
// not a server (there is none), and a live VM still goes through Destroy.
func TestDestroy_PausedSnapshot(t *testing.T) {
	p := &snapshotProvider{fakeProvider: &fakeProvider{
		vms:    []cloud.Vm{{Name: "api", Status: "running"}},
		paused: []cloud.Vm{{Name: "web", Status: "paused"}},
	}}
	c := box.Wrap("hetzner", p)

	require.NoError(t, c.Destroy("web"))
	assert.Equal(t, []string{"web"}, p.snapshotsDeleted)
	assert.Empty(t, p.destroyed)

	require.NoError(t, c.Destroy("api"))
	assert.Equal(t, []string{"api"}, p.destroyed)
	assert.Equal(t, []string{"web"}, p.snapshotsDeleted)
}

func TestPauseResumeSSH(t *testing.T) {
	p := &fakeProvider{}
	c := box.Wrap("fake", p)
	require.NoError(t, c.Pause("web", true))
	vm, err := c.Resume("web", "key-1")
	require.NoError(t, err)
	assert.Equal(t, "10.0.0.9", vm.IP)
	_, err = c.SSH("web", 22, "/keys/id", nil, cloud.Stdio{})
	require.NoError(t, err)
	assert.Equal(t, []string{"pause web hot", "resume web key=key-1", "ssh web"}, p.calls)
}

// The boxctl host service runs commands on boxes through SSH (e.g. sync)
// and has to report what they printed and whether they failed, without
// the process exiting: so output goes to the caller's writers, a non-zero
// exit is an exit code and not an error, and a VM that isn't there is an
// error.
func TestSSH_CapturesOutputAndExitCode(t *testing.T) {
	c := box.Wrap("fake", &fakeProvider{})
	var out, errOut bytes.Buffer
	code, err := c.SSH("web", 22, "", []string{"echo synced; echo oops >&2; exit 3"}, cloud.Stdio{Out: &out, Err: &errOut})
	require.NoError(t, err)
	assert.Equal(t, 3, code)
	assert.Equal(t, "synced\n", out.String())
	assert.Equal(t, "oops\n", errOut.String())

	code, err = c.SSH("web", 22, "", []string{"true"}, cloud.Stdio{Out: &out})
	require.NoError(t, err)
	assert.Equal(t, 0, code)
}

func TestSSH_MissingVMIsAnError(t *testing.T) {
	c := box.Wrap("fake", &fakeProvider{})
	_, err := c.SSH("missing", 22, "", []string{"true"}, cloud.Stdio{})
	assert.Error(t, err)
}

func TestPausedStatus(t *testing.T) {
	for _, s := range []string{"stopped", "off", "TERMINATED", "VM deallocated", "SHELVED", "SHELVED_OFFLOADED"} {
		assert.True(t, box.PausedStatus(s), s)
	}
	// fc's "dead" is gone for good, not resumable: never paused.
	for _, s := range []string{"running", "paused", "dead", "ACTIVE"} {
		assert.False(t, box.PausedStatus(s), s)
	}
}

func TestNew_UnknownProvider(t *testing.T) {
	_, err := box.New(box.Options{Provider: "nope"})
	assert.ErrorContains(t, err, `unsupported provider "nope"`)
}

// A caller with no cobra and no onctl.yaml configures the provider
// through Options alone; fc needs no credentials, so it can be built here.
func TestNew_SettingsConfigureTheProvider(t *testing.T) {
	t.Cleanup(func() { viper.Set("fc.binPath", nil) })
	c, err := box.New(box.Options{
		Provider: "fc",
		Settings: map[string]any{"fc.binPath": "/opt/bin/firecracker"},
	})
	require.NoError(t, err)
	assert.Equal(t, "fc", c.ID())
	fc, ok := c.Provider().(*cloud.ProviderFC)
	require.True(t, ok, "got %T", c.Provider())
	assert.Equal(t, "/opt/bin/firecracker", fc.Config.BinPath)
}

// The boxes provider's ssh ProxyCommand runs onctl; a program other
// than onctl must be able to say where onctl is.
func TestNew_OnctlBin(t *testing.T) {
	t.Setenv("ONCTL_BOXES_TOKEN", "tok")
	c, err := box.New(box.Options{Provider: "boxes", OnctlBin: "/usr/local/bin/onctl"})
	require.NoError(t, err)
	bx, ok := c.Provider().(*cloud.ProviderBoxes)
	require.True(t, ok, "got %T", c.Provider())
	assert.Equal(t, "/usr/local/bin/onctl", bx.Config.OnctlBin)
}

func names(l cloud.VmList) []string {
	out := []string{}
	for _, vm := range l.List {
		out = append(out, vm.Name)
	}
	return out
}

// A missing credential must come back from New as an error. The boxctl host
// service builds Clients per request; a provider that exits the process on
// a missing token would take the whole service down with it.
func TestNew_MissingCredentialsIsAnError(t *testing.T) {
	t.Setenv("HCLOUD_TOKEN", "")
	c, err := box.New(box.Options{Provider: "hetzner"})
	require.Error(t, err)
	assert.Nil(t, c)
	assert.Contains(t, err.Error(), "HCLOUD_TOKEN")
}

// Deploying with a key ID Hetzner can't take must fail the call, not exit.
func TestHetznerDeploy_BadKeyIDIsAnError(t *testing.T) {
	_, err := (cloud.ProviderHetzner{}).Deploy(cloud.Vm{Name: "x", SSHKeyID: "not-a-number"})
	assert.Error(t, err)
}

// A provider without a capability says so with ErrUnsupported, which the
// CLIs turn into their own "not supported" message rather than an error.
func TestCapabilities_UnsupportedIsErrUnsupported(t *testing.T) {
	c := box.Wrap("fake", &fakeProvider{})
	_, err := c.Images()
	assert.ErrorIs(t, err, box.ErrUnsupported)
	_, err = c.Sizes()
	assert.ErrorIs(t, err, box.ErrUnsupported)
	_, err = c.Restart("web", cloud.RestartOptions{})
	assert.ErrorIs(t, err, box.ErrUnsupported)
	assert.ErrorIs(t, c.AuthorizeKey(cloud.Vm{Name: "web"}, "ssh-ed25519 AAAA"), box.ErrUnsupported)
	assert.False(t, c.CanRestart())
	assert.False(t, c.Tunneled())
}

// Dial on a provider whose VMs have addresses goes straight to the VM's
// IP, looked up by name: that's what lets ssh-proxy and port-forward
// work on every provider, not only boxes.
func TestDial_DirectLooksUpTheIP(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	defer func() { _ = ln.Close() }()
	go func() {
		if conn, err := ln.Accept(); err == nil {
			_, _ = conn.Write([]byte("SSH-2.0-fake\r\n"))
			_ = conn.Close()
		}
	}()
	port := ln.Addr().(*net.TCPAddr).Port
	p := &fakeProvider{vms: []cloud.Vm{{Name: "web", IP: "127.0.0.1"}}}

	conn, err := box.Wrap("fake", p).Dial(context.Background(), cloud.Vm{Name: "web"}, port)
	require.NoError(t, err)
	defer func() { _ = conn.Close() }()
	got, err := io.ReadAll(conn)
	require.NoError(t, err)
	assert.Equal(t, "SSH-2.0-fake\r\n", string(got))
}
