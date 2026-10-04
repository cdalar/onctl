package tools

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/binary"
	"encoding/pem"
	"net"
	"testing"

	"golang.org/x/crypto/ssh"
)

// serveSSH runs a minimal ssh server on conn that accepts key, and
// answers every exec request with "ran: <command>" and exit status 0.
func serveSSH(t *testing.T, conn net.Conn, key ssh.PublicKey) {
	t.Helper()
	_, hostPriv, _ := ed25519.GenerateKey(rand.Reader)
	hostSigner, err := ssh.NewSignerFromKey(hostPriv)
	if err != nil {
		t.Fatal(err)
	}
	cfg := &ssh.ServerConfig{
		PublicKeyCallback: func(_ ssh.ConnMetadata, k ssh.PublicKey) (*ssh.Permissions, error) {
			if string(k.Marshal()) == string(key.Marshal()) {
				return nil, nil
			}
			return nil, ssh.ErrNoAuth
		},
	}
	cfg.AddHostKey(hostSigner)
	go func() {
		_, chans, reqs, err := ssh.NewServerConn(conn, cfg)
		if err != nil {
			return
		}
		go ssh.DiscardRequests(reqs)
		for nc := range chans {
			ch, chReqs, err := nc.Accept()
			if err != nil {
				return
			}
			go func() {
				for req := range chReqs {
					if req.Type != "exec" {
						_ = req.Reply(false, nil)
						continue
					}
					cmd := string(req.Payload[4:])
					_ = req.Reply(true, nil)
					_, _ = ch.Write([]byte("ran: " + cmd))
					status := make([]byte, 4)
					binary.BigEndian.PutUint32(status, 0)
					_, _ = ch.SendRequest("exit-status", false, status)
					_ = ch.Close()
				}
			}()
		}
	}()
}

// TestRemoteDial runs a command over a Remote whose connection comes from
// Dial instead of a TCP dial to IPAddress -- the path every cloud.Dialer
// provider (boxes) takes. IPAddress is a name nothing could dial.
func TestRemoteDial(t *testing.T) {
	pub, priv, _ := ed25519.GenerateKey(rand.Reader)
	block, err := ssh.MarshalPrivateKey(priv, "")
	if err != nil {
		t.Fatal(err)
	}
	sshPub, _ := ssh.NewPublicKey(pub)

	var dialedPort int
	r := Remote{
		Username:   "root",
		IPAddress:  "dev", // only a name here: nothing dials it
		SSHPort:    22,
		PrivateKey: string(pem.EncodeToMemory(block)),
		Dial: func(ctx context.Context, port int) (net.Conn, error) {
			dialedPort = port
			// A loopback socket pair, not net.Pipe: both ssh ends write
			// their banner before reading, which deadlocks on an
			// unbuffered pipe (real tunnels buffer).
			ln, err := net.Listen("tcp", "127.0.0.1:0")
			if err != nil {
				return nil, err
			}
			defer func() { _ = ln.Close() }()
			client, err := net.Dial("tcp", ln.Addr().String())
			if err != nil {
				return nil, err
			}
			server, err := ln.Accept()
			if err != nil {
				return nil, err
			}
			serveSSH(t, server, sshPub)
			return client, nil
		},
	}
	out, err := r.RemoteRun(&RemoteRunConfig{Command: "uname -a"})
	if err != nil {
		t.Fatal(err)
	}
	if out != "ran: uname -a" {
		t.Fatalf("got %q", out)
	}
	if dialedPort != 22 {
		t.Fatalf("dialed port %d", dialedPort)
	}
}
