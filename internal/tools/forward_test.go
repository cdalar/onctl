package tools

import (
	"bufio"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/pem"
	"fmt"
	"io"
	"net"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"golang.org/x/crypto/ssh"
)

func TestParseForward(t *testing.T) {
	tests := []struct {
		spec, bind string
		want       Forward
		wantErr    bool
	}{
		{spec: "3000", bind: "127.0.0.1", want: Forward{"127.0.0.1:3000", "localhost", 3000}},
		{spec: "15432:5432", bind: "127.0.0.1", want: Forward{"127.0.0.1:15432", "localhost", 5432}},
		{spec: "0:8080", bind: "0.0.0.0", want: Forward{"0.0.0.0:0", "localhost", 8080}},
		{spec: "5432:db.internal:5432", bind: "127.0.0.1", want: Forward{"127.0.0.1:5432", "db.internal", 5432}},
		{spec: "::1", bind: "::1", want: Forward{}, wantErr: true},
		{spec: "0", wantErr: true},       // the VM's port can't be 0
		{spec: "70000", wantErr: true},   // out of range
		{spec: "a:80", wantErr: true},    // not a number
		{spec: "80::80", wantErr: true},  // empty host
		{spec: "1:2:3:4", wantErr: true}, // too many parts
	}
	for _, tt := range tests {
		got, err := ParseForward(tt.spec, tt.bind)
		if tt.wantErr {
			if err == nil {
				t.Errorf("%q: expected an error, got %+v", tt.spec, got)
			}
			continue
		}
		if err != nil || got != tt.want {
			t.Errorf("%q: got %+v %v, want %+v", tt.spec, got, err, tt.want)
		}
	}
}

// sshForwardServer is an ssh server that serves direct-tcpip (ssh -L)
// channels by dialing the target itself. Each accepted ssh connection is
// recorded so a test can cut it.
type sshForwardServer struct {
	ln    net.Listener
	cfg   *ssh.ServerConfig
	mu    sync.Mutex
	conns []net.Conn
}

func newSSHForwardServer(t *testing.T, key ssh.PublicKey) *sshForwardServer {
	t.Helper()
	_, hostPriv, _ := ed25519.GenerateKey(rand.Reader)
	hostSigner, _ := ssh.NewSignerFromKey(hostPriv)
	cfg := &ssh.ServerConfig{
		PublicKeyCallback: func(_ ssh.ConnMetadata, k ssh.PublicKey) (*ssh.Permissions, error) {
			if string(k.Marshal()) == string(key.Marshal()) {
				return nil, nil
			}
			return nil, ssh.ErrNoAuth
		},
	}
	cfg.AddHostKey(hostSigner)
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	s := &sshForwardServer{ln: ln, cfg: cfg}
	t.Cleanup(func() { _ = ln.Close() })
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			s.mu.Lock()
			s.conns = append(s.conns, c)
			s.mu.Unlock()
			go s.serve(c)
		}
	}()
	return s
}

func (s *sshForwardServer) serve(c net.Conn) {
	_, chans, reqs, err := ssh.NewServerConn(c, s.cfg)
	if err != nil {
		return
	}
	go ssh.DiscardRequests(reqs)
	for nc := range chans {
		if nc.ChannelType() != "direct-tcpip" {
			_ = nc.Reject(ssh.UnknownChannelType, "")
			continue
		}
		var target struct {
			Host       string
			Port       uint32
			OriginHost string
			OriginPort uint32
		}
		if err := ssh.Unmarshal(nc.ExtraData(), &target); err != nil {
			_ = nc.Reject(ssh.ConnectionFailed, err.Error())
			continue
		}
		dst, err := net.Dial("tcp", net.JoinHostPort(target.Host, strconv.Itoa(int(target.Port))))
		if err != nil {
			_ = nc.Reject(ssh.ConnectionFailed, "connection refused")
			continue
		}
		ch, chReqs, err := nc.Accept()
		if err != nil {
			_ = dst.Close()
			continue
		}
		go ssh.DiscardRequests(chReqs)
		go func() {
			defer func() { _ = ch.Close() }()
			defer func() { _ = dst.Close() }()
			done := make(chan struct{}, 2)
			go func() { _, _ = io.Copy(ch, dst); done <- struct{}{} }()
			go func() { _, _ = io.Copy(dst, ch); done <- struct{}{} }()
			<-done
		}()
	}
}

// dropAll cuts every ssh connection, as a VM pause or a sleeping laptop
// would.
func (s *sshForwardServer) dropAll() {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, c := range s.conns {
		_ = c.Close()
	}
	s.conns = nil
}

// lineEcho answers each line with "echo: <line>".
func lineEcho(t *testing.T) int {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = ln.Close() })
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			go func() {
				defer func() { _ = c.Close() }()
				sc := bufio.NewScanner(c)
				for sc.Scan() {
					_, _ = fmt.Fprintf(c, "echo: %s\n", sc.Text())
				}
			}()
		}
	}()
	return ln.Addr().(*net.TCPAddr).Port
}

func roundTrip(t *testing.T, addr, line string) (string, error) {
	t.Helper()
	c, err := net.Dial("tcp", addr)
	if err != nil {
		return "", err
	}
	defer func() { _ = c.Close() }()
	if _, err := fmt.Fprintln(c, line); err != nil {
		return "", err
	}
	return bufio.NewReader(c).ReadString('\n')
}

func TestForwarder(t *testing.T) {
	pub, priv, _ := ed25519.GenerateKey(rand.Reader)
	block, _ := ssh.MarshalPrivateKey(priv, "")
	sshPub, _ := ssh.NewPublicKey(pub)
	srv := newSSHForwardServer(t, sshPub)

	dials := 0
	remote := &Remote{
		Username:   "root",
		IPAddress:  "vm",
		SSHPort:    22,
		PrivateKey: string(pem.EncodeToMemory(block)),
		// Through Dial, as for boxes; a plain TCP Remote works the same.
		Dial: func(ctx context.Context, port int) (net.Conn, error) {
			dials++
			return net.Dial("tcp", srv.ln.Addr().String())
		},
	}
	f := &Forwarder{Remote: remote}

	echoPort := lineEcho(t)
	fw, err := ParseForward("0:"+strconv.Itoa(echoPort), "127.0.0.1")
	if err != nil {
		t.Fatal(err)
	}
	ln, err := net.Listen("tcp", fw.Local)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = ln.Close() }()
	var errMu sync.Mutex
	var errs []string
	go func() {
		_ = f.Serve(ln, fw, func(err error) {
			errMu.Lock()
			errs = append(errs, err.Error())
			errMu.Unlock()
		})
	}()

	got, err := roundTrip(t, ln.Addr().String(), "hello")
	if err != nil || got != "echo: hello\n" {
		t.Fatalf("got %q %v", got, err)
	}
	// Two connections, one ssh connection.
	if got, _ := roundTrip(t, ln.Addr().String(), "again"); got != "echo: again\n" || dials != 1 {
		t.Fatalf("got %q after %d ssh dials", got, dials)
	}

	// The ssh connection drops; the next forwarded connection reconnects.
	srv.dropAll()
	if got, err := roundTrip(t, ln.Addr().String(), "back"); err != nil || got != "echo: back\n" || dials != 2 {
		t.Fatalf("after a drop: got %q %v after %d ssh dials", got, err, dials)
	}

	// Nothing listening on the VM's side: reported, and no reconnect for it.
	refused := listenAndClose(t)
	fw2, _ := ParseForward("0:"+strconv.Itoa(refused), "127.0.0.1")
	ln2, _ := net.Listen("tcp", fw2.Local)
	defer func() { _ = ln2.Close() }()
	go func() {
		_ = f.Serve(ln2, fw2, func(err error) {
			errMu.Lock()
			errs = append(errs, err.Error())
			errMu.Unlock()
		})
	}()
	if got, _ := roundTrip(t, ln2.Addr().String(), "x"); got != "" {
		t.Fatalf("a refused port should close the local connection, got %q", got)
	}
	errMu.Lock()
	defer errMu.Unlock()
	if len(errs) != 1 || !strings.Contains(errs[0], "connection refused") || dials != 2 {
		t.Fatalf("errs %q after %d ssh dials", errs, dials)
	}
}

// listenAndClose returns a port nothing listens on.
func listenAndClose(t *testing.T) int {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := ln.Addr().(*net.TCPAddr).Port
	_ = ln.Close()
	return port
}

// freezableRelay forwards a TCP connection to target until frozen, after
// which it stops passing bytes but keeps both sockets open -- what a
// paused VM's tunnel looks like from this end.
type freezableRelay struct {
	target string
	mu     sync.Mutex
	frozen bool
}

func (r *freezableRelay) dial() (net.Conn, error) {
	ours, theirs := net.Pipe()
	up, err := net.Dial("tcp", r.target)
	if err != nil {
		return nil, err
	}
	copyUnlessFrozen := func(dst, src net.Conn) {
		buf := make([]byte, 32*1024)
		for {
			n, err := src.Read(buf)
			r.mu.Lock()
			frozen := r.frozen
			r.mu.Unlock()
			if frozen {
				return // swallow: never deliver, never close
			}
			if n > 0 {
				if _, werr := dst.Write(buf[:n]); werr != nil {
					return
				}
			}
			if err != nil {
				return
			}
		}
	}
	go copyUnlessFrozen(up, theirs)
	go copyUnlessFrozen(theirs, up)
	return ours, nil
}

func (r *freezableRelay) freeze() {
	r.mu.Lock()
	r.frozen = true
	r.mu.Unlock()
}

func (r *freezableRelay) thaw() {
	r.mu.Lock()
	r.frozen = false
	r.mu.Unlock()
}

// TestForwarderReconnectsSilentConnection: a connection that stops
// answering without closing (a paused VM) is replaced, not waited on.
func TestForwarderReconnectsSilentConnection(t *testing.T) {
	orig := aliveTimeout
	aliveTimeout = 200 * time.Millisecond
	t.Cleanup(func() { aliveTimeout = orig })

	pub, priv, _ := ed25519.GenerateKey(rand.Reader)
	block, _ := ssh.MarshalPrivateKey(priv, "")
	sshPub, _ := ssh.NewPublicKey(pub)
	srv := newSSHForwardServer(t, sshPub)
	relay := &freezableRelay{target: srv.ln.Addr().String()}

	dials := 0
	f := &Forwarder{Remote: &Remote{
		Username:   "root",
		IPAddress:  "vm",
		SSHPort:    22,
		PrivateKey: string(pem.EncodeToMemory(block)),
		Dial: func(ctx context.Context, port int) (net.Conn, error) {
			dials++
			relay.thaw() // a fresh connection (the VM resumed) flows again
			return relay.dial()
		},
	}}
	fw, _ := ParseForward("0:"+strconv.Itoa(lineEcho(t)), "127.0.0.1")
	ln, _ := net.Listen("tcp", fw.Local)
	defer func() { _ = ln.Close() }()
	go func() { _ = f.Serve(ln, fw, func(err error) { t.Log(err) }) }()

	if got, err := roundTrip(t, ln.Addr().String(), "one"); err != nil || got != "echo: one\n" {
		t.Fatalf("got %q %v", got, err)
	}
	relay.freeze()
	start := time.Now()
	got, err := roundTrip(t, ln.Addr().String(), "two")
	if err != nil || got != "echo: two\n" || dials != 2 {
		t.Fatalf("after the connection went silent: got %q %v after %d ssh dials", got, err, dials)
	}
	if waited := time.Since(start); waited > 5*time.Second {
		t.Fatalf("took %s to notice the silent connection", waited)
	}
}
