package cloud

import (
	"context"
	"fmt"
	"io"
	"net"
	"strings"
	"sync"
	"testing"
	"time"

	v1 "github.com/NVIDIA/OpenShell/sdk/go/openshell/v1"
)

// fakeSandboxes is a gateway's sandboxes in memory: just enough of the
// SDK's SandboxInterface for the provider.
type fakeSandboxes struct {
	mu        sync.Mutex
	sandboxes map[string]*v1.Sandbox
	created   []*v1.SandboxSpec
	calls     []string
	workspace string
}

func newFakeSandboxes() *fakeSandboxes {
	return &fakeSandboxes{sandboxes: map[string]*v1.Sandbox{}}
}

func (f *fakeSandboxes) add(name string, phase v1.SandboxPhase, created time.Time) {
	f.sandboxes[name] = &v1.Sandbox{ID: "id-" + name, Name: name, CreatedAt: created, Status: v1.SandboxStatus{Phase: phase}}
}

func (f *fakeSandboxes) record(call, workspace, name string) {
	f.calls = append(f.calls, call+" "+name)
	f.workspace = workspace
}

func notFound(name string) error {
	return &v1.StatusError{Code: v1.ErrorNotFound, Message: "sandbox " + name + " not found"}
}

func (f *fakeSandboxes) Create(_ context.Context, workspace, name string, spec *v1.SandboxSpec, _ ...v1.CreateOption) (*v1.Sandbox, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.record("create", workspace, name)
	f.created = append(f.created, spec)
	s := &v1.Sandbox{ID: "id-" + name, Name: name, Spec: *spec, Status: v1.SandboxStatus{Phase: v1.SandboxProvisioning}}
	f.sandboxes[name] = s
	return s, nil
}

func (f *fakeSandboxes) Get(_ context.Context, workspace, name string) (*v1.Sandbox, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.record("get", workspace, name)
	s, ok := f.sandboxes[name]
	if !ok {
		return nil, notFound(name)
	}
	return s, nil
}

func (f *fakeSandboxes) ListAll(_ context.Context, workspace string, _ ...v1.ListOptions) ([]*v1.Sandbox, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.record("list", workspace, "")
	var out []*v1.Sandbox
	for _, s := range f.sandboxes {
		out = append(out, s)
	}
	return out, nil
}

func (f *fakeSandboxes) setPhase(call, workspace, name string, phase v1.SandboxPhase) (*v1.Sandbox, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.record(call, workspace, name)
	s, ok := f.sandboxes[name]
	if !ok {
		return nil, notFound(name)
	}
	s.Status.Phase = phase
	return s, nil
}

func (f *fakeSandboxes) Stop(_ context.Context, workspace, name string) (*v1.Sandbox, error) {
	return f.setPhase("stop", workspace, name, v1.SandboxStopping)
}

func (f *fakeSandboxes) Start(_ context.Context, workspace, name string) (*v1.Sandbox, error) {
	return f.setPhase("start", workspace, name, v1.SandboxStarting)
}

func (f *fakeSandboxes) WaitReady(_ context.Context, workspace, name string, _ ...v1.WaitOptions) (*v1.Sandbox, error) {
	return f.setPhase("wait-ready", workspace, name, v1.SandboxReady)
}

func (f *fakeSandboxes) WaitStopped(_ context.Context, workspace, name string, _ ...v1.WaitOptions) (*v1.Sandbox, error) {
	return f.setPhase("wait-stopped", workspace, name, v1.SandboxStopped)
}

func (f *fakeSandboxes) Delete(_ context.Context, workspace, name string, _ ...v1.DeleteOptions) (*v1.DeletionResult, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.record("delete", workspace, name)
	if _, ok := f.sandboxes[name]; !ok {
		return nil, notFound(name)
	}
	delete(f.sandboxes, name)
	return &v1.DeletionResult{}, nil
}

// fakeStreams answers Tunnel and Forward with one end of a pipe whose
// other end echoes, and keeps the context each stream was opened with.
type fakeStreams struct {
	mu    sync.Mutex
	calls []string
	ctxs  []context.Context
}

func (f *fakeStreams) open(ctx context.Context, kind, name string, port uint32) (io.ReadWriteCloser, error) {
	f.mu.Lock()
	f.calls = append(f.calls, fmt.Sprintf("%s %s:%d", kind, name, port))
	f.ctxs = append(f.ctxs, ctx)
	f.mu.Unlock()
	client, server := net.Pipe()
	go func() {
		_, _ = io.Copy(server, server)
		_ = server.Close()
	}()
	return client, nil
}

func (f *fakeStreams) Tunnel(ctx context.Context, _, name string, port uint32, _ ...v1.TunnelOption) (io.ReadWriteCloser, error) {
	return f.open(ctx, "ssh", name, port)
}

func (f *fakeStreams) Forward(ctx context.Context, _, name string, port uint32, _ ...v1.ForwardOption) (io.ReadWriteCloser, error) {
	return f.open(ctx, "tcp", name, port)
}

func newTestOpenShell(cfg OpenShellConfig) (*ProviderOpenShell, *fakeSandboxes, *fakeStreams) {
	sb := newFakeSandboxes()
	st := &fakeStreams{}
	if cfg.Gateway == "" {
		cfg.Gateway = "gw"
	}
	return &ProviderOpenShell{Sandboxes: sb, SSH: st, TCP: st, Config: cfg}, sb, st
}

func TestOpenShellDeployImage(t *testing.T) {
	tests := []struct {
		name, vmImage, configImage, want string
	}{
		{"flag wins", "alpine:3.22", "ubuntu:24.04", "alpine:3.22"},
		{"config default", "", "ubuntu:24.04", "ubuntu:24.04"},
		{"gateway default", "", "", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p, sb, _ := newTestOpenShell(OpenShellConfig{Image: tt.configImage})
			vm, err := p.Deploy(Vm{Name: "s1", Image: tt.vmImage})
			if err != nil {
				t.Fatal(err)
			}
			spec := sb.created[0]
			got := ""
			if spec.Template != nil {
				got = spec.Template.Image
			}
			if got != tt.want {
				t.Errorf("image = %q, want %q", got, tt.want)
			}
			if tt.want == "" && spec.Template != nil {
				t.Errorf("template = %+v, want none so the gateway picks its default image", spec.Template)
			}
			if vm.Status != "running" || !vm.SSHReady || vm.IP != "N/A" || vm.Provider != "openshell" || vm.Location != "gw" {
				t.Errorf("vm = %+v", vm)
			}
			if sb.workspace != "default" {
				t.Errorf("workspace = %q, want default", sb.workspace)
			}
		})
	}
}

func TestOpenShellListSortedAndMapped(t *testing.T) {
	p, sb, _ := newTestOpenShell(OpenShellConfig{Workspace: "team"})
	now := time.Now()
	sb.add("newer", v1.SandboxStopped, now)
	sb.add("older", v1.SandboxReady, now.Add(-time.Hour))
	sb.add("broken", v1.SandboxError, now.Add(-2*time.Hour))
	list, err := p.List()
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, vm := range list.List {
		got = append(got, vm.Name+"="+vm.Status)
	}
	if want := "broken=error older=running newer=paused"; strings.Join(got, " ") != want {
		t.Errorf("list = %v, want %s", got, want)
	}
	if sb.workspace != "team" {
		t.Errorf("workspace = %q, want team", sb.workspace)
	}
}

func TestOpenShellGetByNameNotFound(t *testing.T) {
	p, _, _ := newTestOpenShell(OpenShellConfig{})
	_, err := p.GetByName("nope")
	if err == nil || !strings.Contains(err.Error(), "no sandbox named nope on OpenShell gateway gw") {
		t.Errorf("err = %v", err)
	}
}

func TestOpenShellPauseResume(t *testing.T) {
	p, sb, _ := newTestOpenShell(OpenShellConfig{})
	sb.add("s1", v1.SandboxReady, time.Now())
	if err := p.Pause(Vm{Name: "s1"}, true); err != nil {
		t.Fatal(err)
	}
	if got := sb.sandboxes["s1"].Status.Phase; got != v1.SandboxStopped {
		t.Errorf("after pause phase = %s", got)
	}
	vm, err := p.Resume(Vm{Name: "s1"})
	if err != nil {
		t.Fatal(err)
	}
	if vm.Status != "running" {
		t.Errorf("after resume status = %s", vm.Status)
	}
	if want := "stop s1,wait-stopped s1,start s1,wait-ready s1"; strings.Join(sb.calls, ",") != want {
		t.Errorf("calls = %v, want %s", sb.calls, want)
	}
}

func TestOpenShellDestroy(t *testing.T) {
	p, sb, _ := newTestOpenShell(OpenShellConfig{})
	sb.add("s1", v1.SandboxReady, time.Now())
	if err := p.Destroy(Vm{Name: "s1"}); err != nil {
		t.Fatal(err)
	}
	if _, ok := sb.sandboxes["s1"]; ok {
		t.Error("sandbox still there")
	}
}

func TestOpenShellDialVM(t *testing.T) {
	tests := []struct {
		port int
		want string
	}{
		{22, "ssh s1:22"},
		{8080, "tcp s1:8080"},
	}
	for _, tt := range tests {
		t.Run(fmt.Sprint(tt.port), func(t *testing.T) {
			p, sb, st := newTestOpenShell(OpenShellConfig{})
			sb.add("s1", v1.SandboxReady, time.Now())
			ctx, cancel := context.WithCancel(context.Background())
			conn, err := p.DialVM(ctx, Vm{Name: "s1"}, tt.port)
			if err != nil {
				t.Fatal(err)
			}
			defer func() { _ = conn.Close() }()
			// Callers (tools.Remote) cancel the dial's context as soon as
			// it returns; the stream has to outlive it.
			cancel()
			if st.ctxs[0].Err() != nil {
				t.Errorf("stream context cancelled with the dial's: %v", st.ctxs[0].Err())
			}
			if st.calls[0] != tt.want {
				t.Errorf("call = %s, want %s", st.calls[0], tt.want)
			}
			// The conn is a real net.Conn: deadlines work, bytes round-trip.
			if err := conn.SetDeadline(time.Now().Add(5 * time.Second)); err != nil {
				t.Fatal(err)
			}
			if _, err := conn.Write([]byte("ping")); err != nil {
				t.Fatal(err)
			}
			buf := make([]byte, 4)
			if _, err := io.ReadFull(conn, buf); err != nil || string(buf) != "ping" {
				t.Errorf("echo = %q, %v", buf, err)
			}
		})
	}
}

func TestOpenShellDialVMStartsStoppedSandbox(t *testing.T) {
	p, sb, _ := newTestOpenShell(OpenShellConfig{})
	sb.add("s1", v1.SandboxStopped, time.Now())
	conn, err := p.DialVM(context.Background(), Vm{Name: "s1"}, 22)
	if err != nil {
		t.Fatal(err)
	}
	_ = conn.Close()
	if want := "get s1,start s1,wait-ready s1"; strings.Join(sb.calls, ",") != want {
		t.Errorf("calls = %v, want %s", sb.calls, want)
	}
}

func TestOpenShellDialVMRefusesBrokenSandbox(t *testing.T) {
	p, sb, st := newTestOpenShell(OpenShellConfig{})
	sb.add("s1", v1.SandboxError, time.Now())
	if _, err := p.DialVM(context.Background(), Vm{Name: "s1"}, 22); err == nil || !strings.Contains(err.Error(), "sandbox s1 is error") {
		t.Errorf("err = %v", err)
	}
	if len(st.calls) != 0 {
		t.Errorf("opened a stream to a broken sandbox: %v", st.calls)
	}
}

func TestOpenShellProxyCommand(t *testing.T) {
	tests := []struct {
		name string
		cfg  OpenShellConfig
		want string
	}{
		{"gateway only", OpenShellConfig{Gateway: "gw", OnctlBin: "/usr/local/bin/onctl"},
			"/usr/local/bin/onctl ssh-proxy -p openshell --openshell-gateway gw s1 22"},
		{"workspace and quoting", OpenShellConfig{Gateway: "my gw", Workspace: "team", OnctlBin: "/Apps/my onctl"},
			"'/Apps/my onctl' ssh-proxy -p openshell --openshell-gateway 'my gw' --openshell-workspace team s1 22"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := &ProviderOpenShell{Config: tt.cfg}
			if got := p.proxyCommand("s1", 22); got != tt.want {
				t.Errorf("got  %s\nwant %s", got, tt.want)
			}
		})
	}
}

func TestOpenShellStatus(t *testing.T) {
	tests := map[v1.SandboxPhase]string{
		v1.SandboxReady:        "running",
		v1.SandboxStopped:      "paused",
		v1.SandboxProvisioning: "starting",
		v1.SandboxStarting:     "starting",
		v1.SandboxStopping:     "pausing",
		v1.SandboxError:        "error",
		v1.SandboxDeleting:     "deleting",
		"":                     "unknown",
	}
	for phase, want := range tests {
		if got := openShellStatus(phase); got != want {
			t.Errorf("openShellStatus(%q) = %q, want %q", phase, got, want)
		}
	}
}
