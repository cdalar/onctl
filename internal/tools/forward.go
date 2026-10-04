package tools

import (
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"strconv"
	"strings"
	"sync"
	"time"

	"golang.org/x/crypto/ssh"
)

// Forward is one port forward: connections to Local on this machine are
// carried over ssh to RemoteHost:RemotePort as seen from the VM (ssh -L).
type Forward struct {
	Local      string // host:port to listen on; port 0 picks a free one
	RemoteHost string // usually localhost: the VM itself
	RemotePort int
}

// ParseForward parses REMOTE, LOCAL:REMOTE or LOCAL:HOST:REMOTE (ssh -L's
// form, minus the bind address, which is bind). REMOTE alone listens on
// the same port number locally.
func ParseForward(spec, bind string) (Forward, error) {
	parts := strings.Split(spec, ":")
	port := func(s string, allowZero bool) (int, error) {
		n, err := strconv.Atoi(s)
		if err != nil || n < 0 || n > 65535 || (n == 0 && !allowZero) {
			return 0, fmt.Errorf("invalid port %q in %q", s, spec)
		}
		return n, nil
	}
	var local, remote int
	host := "localhost"
	var err error
	switch len(parts) {
	case 1:
		if remote, err = port(parts[0], false); err != nil {
			return Forward{}, err
		}
		local = remote
	case 2:
		if local, err = port(parts[0], true); err != nil {
			return Forward{}, err
		}
		if remote, err = port(parts[1], false); err != nil {
			return Forward{}, err
		}
	case 3:
		if local, err = port(parts[0], true); err != nil {
			return Forward{}, err
		}
		if host = parts[1]; host == "" {
			return Forward{}, fmt.Errorf("empty host in %q", spec)
		}
		if remote, err = port(parts[2], false); err != nil {
			return Forward{}, err
		}
	default:
		return Forward{}, fmt.Errorf("invalid forward %q: want REMOTE, LOCAL:REMOTE or LOCAL:HOST:REMOTE", spec)
	}
	return Forward{Local: net.JoinHostPort(bind, strconv.Itoa(local)), RemoteHost: host, RemotePort: remote}, nil
}

// Forwarder carries port forwards over one ssh connection to a VM,
// reconnecting once if that connection has dropped (a VM paused and
// resumed, a laptop that slept).
type Forwarder struct {
	Remote *Remote
	mu     sync.Mutex
}

// aliveTimeout bounds the keepalive check before each forwarded
// connection. A VM that was paused doesn't close its old ssh connection --
// it just stops answering -- so a missing reply is how a dead one shows.
var aliveTimeout = 5 * time.Second

// alive reports whether c still answers a keepalive.
func alive(c *ssh.Client) bool {
	done := make(chan error, 1)
	go func() {
		_, _, err := c.SendRequest("keepalive@openssh.com", true, nil)
		done <- err
	}()
	select {
	case err := <-done:
		return err == nil
	case <-time.After(aliveTimeout):
		return false
	}
}

// reconnect drops the current ssh connection and opens a new one.
func (f *Forwarder) reconnect(why string) error {
	log.Println("[DEBUG] forward: reconnecting ssh:", why)
	if f.Remote.Client != nil {
		_ = f.Remote.Client.Close()
		f.Remote.Client = nil
	}
	return f.Remote.NewSSHConnection()
}

// dial opens a channel to addr from the VM's side.
func (f *Forwarder) dial(addr string) (net.Conn, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.Remote.Client != nil && !alive(f.Remote.Client) {
		if err := f.reconnect("no answer to keepalive"); err != nil {
			return nil, err
		}
	}
	if err := f.Remote.NewSSHConnection(); err != nil {
		return nil, err
	}
	conn, err := f.Remote.Client.Dial("tcp", addr)
	var refused *ssh.OpenChannelError
	if err == nil || errors.As(err, &refused) {
		// Connected, or the VM answered that nothing listens there:
		// either way the ssh connection itself is fine.
		return conn, err
	}
	if err := f.reconnect(err.Error()); err != nil {
		return nil, err
	}
	return f.Remote.Client.Dial("tcp", addr)
}

// Serve accepts on ln and carries each connection to fw's remote end,
// until ln is closed. Failures are reported per connection through
// onErr; they don't stop the forward.
func (f *Forwarder) Serve(ln net.Listener, fw Forward, onErr func(error)) error {
	addr := net.JoinHostPort(fw.RemoteHost, strconv.Itoa(fw.RemotePort))
	for {
		local, err := ln.Accept()
		if err != nil {
			if errors.Is(err, net.ErrClosed) {
				return nil
			}
			return err
		}
		go func() {
			defer func() { _ = local.Close() }()
			remote, err := f.dial(addr)
			if err != nil {
				onErr(fmt.Errorf("%s: %w", addr, err))
				return
			}
			defer func() { _ = remote.Close() }()
			pipe(local, remote)
		}()
	}
}

// pipe copies both ways until either side is done.
func pipe(a, b net.Conn) {
	done := make(chan struct{}, 2)
	go func() { _, _ = io.Copy(a, b); done <- struct{}{} }()
	go func() { _, _ = io.Copy(b, a); done <- struct{}{} }()
	<-done
}
