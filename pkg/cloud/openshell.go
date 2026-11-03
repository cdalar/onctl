package cloud

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"os"
	"sort"
	"strings"
	"time"

	v1 "github.com/NVIDIA/OpenShell/sdk/go/openshell/v1"
	"github.com/cdalar/onctl/internal/tools"
)

// OpenShellConfig is the openshell provider's settings (onctl.yaml's
// openshell:).
type OpenShellConfig struct {
	// Gateway is the openshell CLI's name for the gateway; set by
	// whoever builds the provider to the gateway it connected to.
	Gateway string
	// Workspace is the OpenShell workspace sandboxes go in.
	Workspace string
	// Image is the default sandbox image; empty leaves it to the
	// gateway's default_image.
	Image string
	// Username is the ssh user. The gateway's ssh relay logs every
	// connection in as the sandbox's own user whatever is asked for, so
	// it only has to be non-empty.
	Username string
	// OnctlBin is this onctl's own path, for the ssh ProxyCommand.
	OnctlBin string
}

// OpenShellSandboxes is the part of the SDK's v1.SandboxInterface this
// provider uses.
type OpenShellSandboxes interface {
	Create(ctx context.Context, workspace, name string, spec *v1.SandboxSpec, opts ...v1.CreateOption) (*v1.Sandbox, error)
	Get(ctx context.Context, workspace, name string) (*v1.Sandbox, error)
	ListAll(ctx context.Context, workspace string, opts ...v1.ListOptions) ([]*v1.Sandbox, error)
	Stop(ctx context.Context, workspace, name string) (*v1.Sandbox, error)
	Start(ctx context.Context, workspace, name string) (*v1.Sandbox, error)
	Delete(ctx context.Context, workspace, name string, opts ...v1.DeleteOptions) (*v1.DeletionResult, error)
	WaitReady(ctx context.Context, workspace, name string, opts ...v1.WaitOptions) (*v1.Sandbox, error)
	WaitStopped(ctx context.Context, workspace, name string, opts ...v1.WaitOptions) (*v1.Sandbox, error)
}

// OpenShellSSH opens a stream to a sandbox's ssh server through the
// gateway's relay (the SDK's v1.SSHInterface.Tunnel).
type OpenShellSSH interface {
	Tunnel(ctx context.Context, workspace, sandboxName string, port uint32, opts ...v1.TunnelOption) (io.ReadWriteCloser, error)
}

// OpenShellTCP opens a stream to any other port of a sandbox (the SDK's
// v1.TCPInterface.Forward).
type OpenShellTCP interface {
	Forward(ctx context.Context, workspace, sandboxName string, port uint32, opts ...v1.ForwardOption) (io.ReadWriteCloser, error)
}

// ProviderOpenShell runs VMs as sandboxes on an NVIDIA OpenShell gateway
// (https://github.com/NVIDIA/OpenShell): the gateway decides where a
// sandbox runs (a container, a microVM) and enforces its policy, with
// outbound connections denied by default. A sandbox has no address of
// its own, so ssh goes through the gateway's relay (DialVM, and `onctl
// ssh-proxy` for the system ssh).
//
// A sandbox is unprivileged by design -- no root, no sudo -- so the
// software it needs comes from its image, not from apply files.
type ProviderOpenShell struct {
	Sandboxes OpenShellSandboxes
	SSH       OpenShellSSH
	TCP       OpenShellTCP
	Config    OpenShellConfig
}

var (
	_ CloudProviderInterface = (*ProviderOpenShell)(nil)
	_ Dialer                 = (*ProviderOpenShell)(nil)
)

const (
	// openShellReadyTimeout bounds waiting for a created or started
	// sandbox, which includes the gateway pulling its image.
	openShellReadyTimeout = 5 * time.Minute
	// openShellStopTimeout bounds waiting for a sandbox to stop.
	openShellStopTimeout = 2 * time.Minute
	openShellSSHPort     = 22
)

func (p *ProviderOpenShell) workspace() string {
	if p.Config.Workspace == "" {
		return "default"
	}
	return p.Config.Workspace
}

// openShellStatus maps a sandbox phase to the status words the rest of
// onctl uses: a stopped sandbox is "paused", since Pause stops it.
func openShellStatus(phase v1.SandboxPhase) string {
	switch phase {
	case v1.SandboxReady:
		return "running"
	case v1.SandboxStopped:
		return "paused"
	case v1.SandboxProvisioning, v1.SandboxStarting:
		return "starting"
	case v1.SandboxStopping:
		return "pausing"
	case "":
		return "unknown"
	}
	return strings.ToLower(string(phase))
}

func (p *ProviderOpenShell) mapSandbox(s *v1.Sandbox) Vm {
	image := ""
	if s.Spec.Template != nil {
		image = s.Spec.Template.Image
	}
	return Vm{
		ID:   s.ID,
		Name: s.Name,
		// A sandbox has no address: it's reached through the gateway
		// (DialVM, ssh-proxy).
		IP:        "N/A",
		Image:     image,
		Status:    openShellStatus(s.Status.Phase),
		SSHReady:  s.Status.Phase == v1.SandboxReady,
		Location:  p.Config.Gateway,
		SSHPort:   openShellSSHPort,
		CreatedAt: s.CreatedAt,
		Provider:  "openshell",
	}
}

// Deploy creates a sandbox and waits until it's ready. The ssh key isn't
// needed: the gateway authenticates the connection (mTLS or a token)
// before its relay reaches the sandbox.
func (p *ProviderOpenShell) Deploy(server Vm) (Vm, error) {
	ctx, cancel := context.WithTimeout(context.Background(), openShellReadyTimeout)
	defer cancel()
	image := server.Image
	if image == "" {
		image = p.Config.Image
	}
	spec := &v1.SandboxSpec{}
	if image != "" {
		spec.Template = &v1.SandboxTemplate{Image: image}
	}
	if _, err := p.Sandboxes.Create(ctx, p.workspace(), server.Name, spec); err != nil {
		return Vm{}, p.gatewayError(fmt.Errorf("creating sandbox %s: %w", server.Name, err))
	}
	s, err := p.Sandboxes.WaitReady(ctx, p.workspace(), server.Name)
	if err != nil {
		return Vm{}, fmt.Errorf("waiting for sandbox %s: %w", server.Name, err)
	}
	return p.mapSandbox(s), nil
}

func (p *ProviderOpenShell) Destroy(server Vm) error {
	_, err := p.Sandboxes.Delete(context.Background(), p.workspace(), server.Name)
	return err
}

// Pause stops the sandbox. The gateway keeps its workspace, so Resume
// brings back its files but not its processes; hot doesn't apply.
func (p *ProviderOpenShell) Pause(server Vm, hot bool) error {
	ctx, cancel := context.WithTimeout(context.Background(), openShellStopTimeout)
	defer cancel()
	if _, err := p.Sandboxes.Stop(ctx, p.workspace(), server.Name); err != nil {
		return err
	}
	_, err := p.Sandboxes.WaitStopped(ctx, p.workspace(), server.Name)
	return err
}

func (p *ProviderOpenShell) Resume(server Vm) (Vm, error) {
	ctx, cancel := context.WithTimeout(context.Background(), openShellReadyTimeout)
	defer cancel()
	if _, err := p.Sandboxes.Start(ctx, p.workspace(), server.Name); err != nil {
		return Vm{}, err
	}
	s, err := p.Sandboxes.WaitReady(ctx, p.workspace(), server.Name)
	if err != nil {
		return Vm{}, err
	}
	return p.mapSandbox(s), nil
}

// List returns the workspace's sandboxes, stopped ones included, oldest
// first.
func (p *ProviderOpenShell) List() (VmList, error) {
	sandboxes, err := p.Sandboxes.ListAll(context.Background(), p.workspace())
	if err != nil {
		return VmList{}, p.gatewayError(err)
	}
	list := VmList{}
	for _, s := range sandboxes {
		list.List = append(list.List, p.mapSandbox(s))
	}
	sort.SliceStable(list.List, func(i, j int) bool {
		a, b := list.List[i], list.List[j]
		if !a.CreatedAt.Equal(b.CreatedAt) {
			return a.CreatedAt.Before(b.CreatedAt)
		}
		return a.Name < b.Name
	})
	return list, nil
}

// ListPaused is empty: List already includes stopped sandboxes.
func (p *ProviderOpenShell) ListPaused() (VmList, error) { return VmList{}, nil }

func (p *ProviderOpenShell) GetByName(serverName string) (Vm, error) {
	s, err := p.Sandboxes.Get(context.Background(), p.workspace(), serverName)
	if err != nil {
		if v1.IsNotFound(err) {
			return Vm{}, fmt.Errorf("no sandbox named %s on OpenShell gateway %s", serverName, p.Config.Gateway)
		}
		return Vm{}, p.gatewayError(err)
	}
	return p.mapSandbox(s), nil
}

// gatewayError says which gateway a failed call went to: the SDK's
// errors name only the address it dialed.
func (p *ProviderOpenShell) gatewayError(err error) error {
	return fmt.Errorf("OpenShell gateway %s: %w", p.Config.Gateway, err)
}

// CreateSSHKey has no registry to talk to, and the relay doesn't check
// the key: the "key ID" is the public key itself, unused.
func (p *ProviderOpenShell) CreateSSHKey(publicKeyFile string) (string, error) {
	data, err := os.ReadFile(publicKeyFile)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(data)), nil
}

// SSHInto runs the system ssh with `onctl ssh-proxy` as its
// ProxyCommand, starting the sandbox first if it's stopped.
func (p *ProviderOpenShell) SSHInto(serverName string, port int, privateKey string, command []string) {
	if port == 0 {
		port = openShellSSHPort
	}
	if err := p.ensureRunning(context.Background(), serverName); err != nil {
		log.Fatalln(err)
	}
	username := p.Config.Username
	if username == "" {
		username = "sandbox"
	}
	tools.SSHIntoVM(tools.SSHIntoVMRequest{
		IPAddress:      serverName,
		User:           username,
		Port:           port,
		PrivateKeyFile: privateKey,
		Command:        command,
		ProxyCommand:   p.proxyCommand(serverName, port),
	})
}

// proxyCommand is the ssh ProxyCommand reaching name's port, on this
// provider's gateway and workspace.
func (p *ProviderOpenShell) proxyCommand(name string, port int) string {
	bin := p.Config.OnctlBin
	if bin == "" {
		bin = "onctl"
	}
	cmd := fmt.Sprintf("%s ssh-proxy -p openshell", shellQuote(bin))
	if p.Config.Gateway != "" {
		cmd += " --openshell-gateway " + shellQuote(p.Config.Gateway)
	}
	if p.Config.Workspace != "" {
		cmd += " --openshell-workspace " + shellQuote(p.Config.Workspace)
	}
	return fmt.Sprintf("%s %s %d", cmd, shellQuote(name), port)
}

// DialVM opens a connection to port inside the sandbox through the
// gateway, starting the sandbox first if it's stopped. Port 22 is the
// sandbox's ssh server, through the gateway's ssh relay; any other port
// is a plain TCP forward.
//
// ctx bounds only opening the connection: the SDK ties the stream's
// lifetime to the context it's opened with, and callers cancel theirs
// once the dial returns.
func (p *ProviderOpenShell) DialVM(ctx context.Context, vm Vm, port int) (net.Conn, error) {
	if port < 1 || port > 65535 {
		return nil, fmt.Errorf("invalid port %d", port)
	}
	if err := p.ensureRunning(ctx, vm.Name); err != nil {
		return nil, err
	}
	type result struct {
		rwc io.ReadWriteCloser
		err error
	}
	done := make(chan result, 1)
	stream := context.WithoutCancel(ctx)
	go func() {
		var r result
		if port == openShellSSHPort {
			r.rwc, r.err = p.SSH.Tunnel(stream, p.workspace(), vm.Name, uint32(port))
		} else {
			r.rwc, r.err = p.TCP.Forward(stream, p.workspace(), vm.Name, uint32(port))
		}
		done <- r
	}()
	select {
	case r := <-done:
		if r.err != nil {
			return nil, fmt.Errorf("connecting to %s:%d: %w", vm.Name, port, r.err)
		}
		return streamConn(r.rwc), nil
	case <-ctx.Done():
		// Close whatever the dial still produces.
		go func() {
			if r := <-done; r.rwc != nil {
				_ = r.rwc.Close()
			}
		}()
		return nil, fmt.Errorf("connecting to %s:%d: %w", vm.Name, port, ctx.Err())
	}
}

// streamConn presents rwc as a net.Conn, deadlines included -- the ssh
// handshake bounds itself with one, and the SDK's streams have none --
// by relaying it through one end of a net.Pipe.
func streamConn(rwc io.ReadWriteCloser) net.Conn {
	local, remote := net.Pipe()
	go func() {
		_, _ = io.Copy(rwc, remote)
		_ = rwc.Close()
	}()
	go func() {
		_, _ = io.Copy(remote, rwc)
		_ = remote.Close()
	}()
	return local
}

// ensureRunning starts name if it's stopped and waits until it's ready.
func (p *ProviderOpenShell) ensureRunning(ctx context.Context, name string) error {
	s, err := p.Sandboxes.Get(ctx, p.workspace(), name)
	if err != nil {
		if v1.IsNotFound(err) {
			return fmt.Errorf("no sandbox named %s on OpenShell gateway %s", name, p.Config.Gateway)
		}
		return p.gatewayError(err)
	}
	switch s.Status.Phase {
	case v1.SandboxReady:
		return nil
	case v1.SandboxStopped:
		fmt.Fprintf(os.Stderr, "Starting %s...\n", name)
		if _, err := p.Sandboxes.Start(ctx, p.workspace(), name); err != nil {
			return fmt.Errorf("starting %s: %w", name, err)
		}
	case v1.SandboxProvisioning, v1.SandboxStarting:
	default:
		return fmt.Errorf("sandbox %s is %s", name, openShellStatus(s.Status.Phase))
	}
	wctx, cancel := context.WithTimeout(ctx, openShellReadyTimeout)
	defer cancel()
	if _, err := p.Sandboxes.WaitReady(wctx, p.workspace(), name); err != nil {
		return fmt.Errorf("waiting for %s: %w", name, err)
	}
	return nil
}

// ErrOpenShellApplyFiles explains why apply files, uploads and
// cloud-init don't run on an OpenShell sandbox.
var ErrOpenShellApplyFiles = errors.New("OpenShell sandboxes run unprivileged, with no root or sudo, and the gateway's ssh relay has no file transfer: " +
	"apply files (-a), uploads, downloads and cloud-init don't work there. Put what the sandbox needs in its image (--image)")
