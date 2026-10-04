package cmd

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"golang.org/x/term"
)

// GitHub credentials for Claude on a box -- docs/plans/claude-on-the-box.md
// in boxctl-vms. Three modes, by --github:
//
//   - forward (default): while you're attached, a server here answers
//     the box's requests for a token with `gh auth token`, over a Unix
//     socket the attach's ssh forwards to /root/.boxctl/gh.sock. The
//     claude-agent image's git credential helper and gh wrapper read it
//     per call (images/claude-agent in boxctl-vms), so the token is never
//     stored on the box and stops working when you detach.
//   - store: for work that has to go on while you're not attached. A
//     token you paste -- ideally a fine-grained one for just this
//     repository -- is logged in to the box's gh, which also becomes
//     git's credential helper there. It stays on the box until it's
//     destroyed.
//   - off: nothing.

const (
	githubForward = "forward"
	githubStore   = "store"
	githubOff     = "off"

	// boxGitHubSocket is where the image's helpers look for the socket.
	boxGitHubSocket = "/root/.boxctl/gh.sock"
)

// ghToken returns the token gh has for host. A variable so tests can
// stand in for gh.
var ghToken = func(ctx context.Context, host string) (string, error) {
	out, err := exec.CommandContext(ctx, "gh", "auth", "token", "--hostname", host).Output()
	if err != nil {
		if errors.Is(err, exec.ErrNotFound) {
			return "", errors.New("gh isn't installed on this machine")
		}
		return "", fmt.Errorf("gh has no token for %s here (gh auth login --hostname %s)", host, host)
	}
	token := strings.TrimSpace(string(out))
	if token == "" {
		return "", fmt.Errorf("gh has no token for %s here", host)
	}
	return token, nil
}

// tokenServer serves GitHub tokens to one box on a local Unix socket:
//
//	GET /token?host=<host>&for=<what>  ->  200 and the token, or 403 and why
//
// Every request is appended to log, not printed: the terminal belongs to
// Claude's interface while you're attached.
type tokenServer struct {
	box    string
	socket string
	dir    string
	srv    *http.Server
	log    *os.File

	mu      sync.Mutex
	given   int
	refused int
}

// startTokenServer listens in a fresh private directory: macOS caps a
// Unix socket path at 104 bytes, so it goes under the system temp dir
// rather than somewhere deeper.
func startTokenServer(box, logPath string) (*tokenServer, error) {
	dir, err := os.MkdirTemp("", "onctl-gh-")
	if err != nil {
		return nil, err
	}
	s := &tokenServer{box: box, dir: dir, socket: filepath.Join(dir, "gh.sock")}
	ln, err := net.Listen("unix", s.socket)
	if err != nil {
		_ = os.RemoveAll(dir)
		return nil, err
	}
	if err := os.Chmod(s.socket, 0o600); err != nil {
		_ = ln.Close()
		_ = os.RemoveAll(dir)
		return nil, err
	}
	if logPath != "" {
		if f, err := os.OpenFile(logPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600); err == nil {
			s.log = f
		}
	}
	s.srv = &http.Server{Handler: s, ReadHeaderTimeout: 10 * time.Second}
	go func() { _ = s.srv.Serve(ln) }()
	return s, nil
}

func (s *tokenServer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	host, what := r.URL.Query().Get("host"), r.URL.Query().Get("for")
	if what == "" {
		what = "?"
	}
	var token string
	var err error
	switch {
	case r.Method != http.MethodGet || r.URL.Path != "/token":
		err = errors.New("unknown request")
	case host == "" || strings.ContainsAny(host, "/ \t"):
		err = errors.New("no host")
	default:
		token, err = ghToken(r.Context(), host)
	}

	s.mu.Lock()
	if err == nil {
		s.given++
	} else {
		s.refused++
	}
	s.mu.Unlock()
	if s.log != nil {
		outcome := "given"
		if err != nil {
			outcome = "refused: " + err.Error()
		}
		_, _ = fmt.Fprintf(s.log, "%s %s asked for a %s token for %s: %s\n", time.Now().Format(time.RFC3339), s.box, host, what, outcome)
	}

	w.Header().Set("Content-Type", "text/plain")
	if err != nil {
		w.WriteHeader(http.StatusForbidden)
		_, _ = fmt.Fprintln(w, err)
		return
	}
	_, _ = fmt.Fprint(w, token)
}

// close stops serving and reports what the box asked for.
func (s *tokenServer) close() (given, refused int) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	_ = s.srv.Shutdown(ctx)
	_ = os.RemoveAll(s.dir)
	if s.log != nil {
		_ = s.log.Close()
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.given, s.refused
}

// storeGitHubToken logs a token in to gh on the box and makes gh git's
// credential helper there, for --github store. The token comes from
// ONCTL_GITHUB_TOKEN (or boxctl's BOXCTL_GITHUB_TOKEN), or is asked for -- never from `gh auth token`,
// which usually reaches every repository you can, and would then sit on
// the box for as long as it exists.
func storeGitHubToken(ctx context.Context, box *boxSSH, repo string) error {
	token := strings.TrimSpace(os.Getenv("ONCTL_GITHUB_TOKEN"))
	if token == "" {
		token = strings.TrimSpace(os.Getenv("BOXCTL_GITHUB_TOKEN")) // boxctl's name for it
	}
	if token == "" {
		if !term.IsTerminal(int(os.Stdin.Fd())) {
			return errors.New("--github store needs a token: set ONCTL_GITHUB_TOKEN")
		}
		fmt.Fprintf(os.Stderr, "--github store keeps a GitHub token on %s until it's destroyed, so give it one that can do little:\n", box.name)
		fmt.Fprintf(os.Stderr, "a fine-grained token for %s only, with Contents and Pull requests read/write and an expiry\n", repo)
		fmt.Fprintf(os.Stderr, "(https://github.com/settings/personal-access-tokens/new).\nToken: ")
		raw, err := term.ReadPassword(int(os.Stdin.Fd()))
		fmt.Fprintln(os.Stderr)
		if err != nil {
			return err
		}
		token = strings.TrimSpace(string(raw))
		if token == "" {
			return errors.New("no token given")
		}
	}
	// /usr/bin/gh, not the image's wrapper: with a forwarded token around,
	// the wrapper would set GH_TOKEN, and gh refuses to log in over it.
	err := box.run(ctx, `gh=/usr/bin/gh; [ -x "$gh" ] || gh=gh
"$gh" auth login --hostname github.com --with-token && "$gh" auth setup-git --hostname github.com`, strings.NewReader(token+"\n"))
	if err != nil {
		return fmt.Errorf("storing the GitHub token on %s: %w", box.name, err)
	}
	fmt.Fprintf(os.Stderr, "Stored the GitHub token on %s.\n", box.name)
	return nil
}

// githubRepo is the project's GitHub repository as owner/name, from its
// origin, or "" -- only for telling the user what to scope a token to.
func githubRepo(dir string) string {
	out, err := exec.Command("git", "-C", dir, "remote", "get-url", "origin").Output()
	if err != nil {
		return ""
	}
	url := strings.TrimSpace(string(out))
	for _, prefix := range []string{"git@github.com:", "ssh://git@github.com/", "https://github.com/"} {
		if rest, ok := strings.CutPrefix(url, prefix); ok {
			return strings.TrimSuffix(rest, ".git")
		}
	}
	return ""
}

// copyGitIdentity gives git on the box your user.name and user.email, so
// Claude's commits there are yours and git doesn't refuse to commit.
func copyGitIdentity(ctx context.Context, box *boxSSH, dir string) error {
	var script []string
	for _, key := range []string{"user.name", "user.email"} {
		out, err := exec.Command("git", "-C", dir, "config", "--get", key).Output()
		if err != nil {
			continue
		}
		if v := strings.TrimSpace(string(out)); v != "" {
			script = append(script, "git config --global "+key+" "+shellQuote(v))
		}
	}
	if len(script) == 0 {
		return nil
	}
	return box.run(ctx, strings.Join(script, " && "), nil)
}
