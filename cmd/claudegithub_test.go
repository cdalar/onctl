package cmd

import (
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// unixClient talks HTTP to a Unix socket, as curl --unix-socket does on
// the box.
func unixClient(socket string) *http.Client {
	return &http.Client{Transport: &http.Transport{
		DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
			return (&net.Dialer{}).DialContext(ctx, "unix", socket)
		},
	}}
}

func get(t *testing.T, c *http.Client, path string) (int, string) {
	t.Helper()
	res, err := c.Get("http://boxctl" + path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = res.Body.Close() }()
	body, _ := io.ReadAll(res.Body)
	return res.StatusCode, string(body)
}

func TestTokenServer(t *testing.T) {
	prev := ghToken
	t.Cleanup(func() { ghToken = prev })
	ghToken = func(_ context.Context, host string) (string, error) {
		if host == "github.com" {
			return "tok123", nil
		}
		return "", errors.New("gh has no token for " + host)
	}

	logPath := filepath.Join(t.TempDir(), "github.log")
	srv, err := startTokenServer("claude-x", logPath)
	if err != nil {
		t.Fatal(err)
	}
	if st, err := os.Stat(srv.socket); err != nil || st.Mode().Perm() != 0o600 {
		t.Fatalf("socket mode: %v %v", st, err)
	}
	if len(srv.socket) > 100 {
		t.Errorf("socket path is %d bytes; macOS allows 104", len(srv.socket))
	}
	c := unixClient(srv.socket)

	if code, body := get(t, c, "/token?host=github.com&for=gh-pr-create"); code != 200 || body != "tok123" {
		t.Errorf("github.com: %d %q", code, body)
	}
	if code, body := get(t, c, "/token?host=gitlab.com&for=git"); code != 403 || strings.Contains(body, "tok123") {
		t.Errorf("gitlab.com: %d %q", code, body)
	}
	for _, bad := range []string{"/token", "/other?host=github.com", "/token?host=a%2Fb"} {
		if code, _ := get(t, c, bad); code != 403 {
			t.Errorf("%s: %d, want 403", bad, code)
		}
	}

	dir := srv.dir
	given, refused := srv.close()
	if given != 1 || refused != 4 {
		t.Errorf("given %d refused %d, want 1 and 4", given, refused)
	}
	if _, err := os.Stat(dir); !os.IsNotExist(err) {
		t.Error("the socket's directory outlived the server")
	}
	log, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(log), "claude-x asked for a github.com token for gh-pr-create: given") ||
		!strings.Contains(string(log), "gitlab.com token for git: refused") {
		t.Errorf("log:\n%s", log)
	}
	if strings.Contains(string(log), "tok123") {
		t.Error("the token was written to the log")
	}
}

func TestAttachArgsGetTheirOwnConnection(t *testing.T) {
	b := &boxSSH{name: "x", args: []string{"-o", "ControlMaster=auto", "-o", "ControlPath=/tmp/cm", "root@x"}}
	got := b.attachArgs("tmux", []string{"-R", "/root/.boxctl/gh.sock:/tmp/s"})
	none, shared := slices.Index(got, "ControlPath=none"), slices.Index(got, "ControlPath=/tmp/cm")
	if none < 0 || shared < 0 || none > shared {
		t.Fatalf("ssh takes an option's first value; ControlPath=none must come before the shared one: %v", got)
	}
	if got[len(got)-2] != "root@x" || got[len(got)-1] != "tmux" || !slices.Contains(got, "-R") {
		t.Fatalf("got %v", got)
	}
}

func TestGitHubRepo(t *testing.T) {
	dir := t.TempDir()
	run(t, dir, "git", "init", "-q")
	if githubRepo(dir) != "" {
		t.Error("no origin should give no repo")
	}
	run(t, dir, "git", "remote", "add", "origin", "https://example.com/placeholder")
	for url, want := range map[string]string{
		"git@github.com:cdalar/boxctl.git":       "cdalar/boxctl",
		"https://github.com/cdalar/boxctl":       "cdalar/boxctl",
		"ssh://git@github.com/cdalar/boxctl.git": "cdalar/boxctl",
		"https://gitlab.com/x/y.git":             "",
	} {
		run(t, dir, "git", "remote", "remove", "origin")
		run(t, dir, "git", "remote", "add", "origin", url)
		if got := githubRepo(dir); got != want {
			t.Errorf("%s: got %q, want %q", url, got, want)
		}
	}
}
