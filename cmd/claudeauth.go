package cmd

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"

	"github.com/spf13/cobra"
	"golang.org/x/term"
)

// The Claude token every box's Claude is started with, so nobody logs in
// on a box: a one-year subscription token from `claude setup-token`
// (Claude Code reads it as CLAUDE_CODE_OAUTH_TOKEN), saved here once by
// `onctl claude login` -- in the macOS Keychain, or a 0600 file
// elsewhere. Not your normal Claude login copied around: that one's
// refresh token rotates, so several machines sharing it would log each
// other (and this one) out.

const (
	claudeTokenService = "onctl-claude-oauth-token"
	claudeTokenFile    = "oauth-token" // in ~/.onctl/claude, off macOS
	// boxctl saved the token under this name; it's still read, so nobody
	// has to log in again.
	legacyClaudeTokenService = "boxctl-claude-oauth-token"
)

var claudeTokenPattern = regexp.MustCompile(`^[A-Za-z0-9._-]{20,512}$`)

var claudeLoginCmd = &cobra.Command{
	Use:   "login",
	Short: "Save a Claude token so Claude on every box is logged in",
	Long: `Saves a long-lived Claude token here, so Claude on every box onctl
claude starts is logged in without /login. It runs claude setup-token (a
browser approval, for a Pro, Max, Team or Enterprise plan) and asks you
to paste the token it prints; or pipe a token in.

The token goes in the macOS Keychain (elsewhere ~/.onctl/claude/` + claudeTokenFile + `,
mode 0600). Each box's Claude gets it in its environment when it starts,
never on its disk. It can only make model requests: Remote Control and
claude.ai connectors don't work with it -- use --claude-auth login on a
box that needs them.`,
	Args: cobra.NoArgs,
	RunE: func(_ *cobra.Command, _ []string) error {
		token, err := readClaudeToken()
		if err != nil {
			return err
		}
		where, err := saveClaudeToken(token)
		if err != nil {
			return err
		}
		fmt.Fprintf(os.Stderr, "Saved (%s). Claude on every box onctl claude starts is now logged in.\n", where)
		return nil
	},
}

var claudeLogoutCmd = &cobra.Command{
	Use:   "logout",
	Short: "Forget the saved Claude token",
	Args:  cobra.NoArgs,
	RunE: func(_ *cobra.Command, _ []string) error {
		removeClaudeToken()
		fmt.Fprintln(os.Stderr, "Forgot the saved Claude token. Boxes already running Claude keep it until that Claude exits.")
		return nil
	},
}

func init() {
	claudeCmd.AddCommand(claudeLoginCmd, claudeLogoutCmd)
}

// readClaudeToken gets a token to save: piped in, or made with claude
// setup-token and pasted back.
func readClaudeToken() (string, error) {
	var raw string
	if !term.IsTerminal(int(os.Stdin.Fd())) {
		data, err := io.ReadAll(bufio.NewReader(os.Stdin))
		if err != nil {
			return "", err
		}
		raw = string(data)
	} else {
		if _, err := exec.LookPath("claude"); err == nil {
			fmt.Fprintln(os.Stderr, "Running claude setup-token -- approve it in the browser, then come back here.")
			c := exec.Command("claude", "setup-token")
			c.Stdin, c.Stdout, c.Stderr = os.Stdin, os.Stdout, os.Stderr
			if err := c.Run(); err != nil {
				return "", fmt.Errorf("claude setup-token: %w", err)
			}
		} else {
			fmt.Fprintln(os.Stderr, "Make a token with claude setup-token (on any machine with Claude Code).")
		}
		fmt.Fprint(os.Stderr, "Paste the token (input hidden): ")
		b, err := term.ReadPassword(int(os.Stdin.Fd()))
		fmt.Fprintln(os.Stderr)
		if err != nil {
			return "", err
		}
		raw = string(b)
	}
	token := strings.TrimSpace(raw)
	if !claudeTokenPattern.MatchString(token) {
		return "", errors.New("that doesn't look like a token")
	}
	if !strings.HasPrefix(token, "sk-ant-") {
		fmt.Fprintln(os.Stderr, "warning: Claude tokens start with sk-ant-; saving it anyway.")
	}
	return token, nil
}

// loadClaudeToken is the token Claude on a box is started with:
// CLAUDE_CODE_OAUTH_TOKEN if it's set here, else the saved one, else "".
func loadClaudeToken() (string, error) {
	if t := strings.TrimSpace(os.Getenv("CLAUDE_CODE_OAUTH_TOKEN")); t != "" {
		return t, nil
	}
	if runtime.GOOS == "darwin" {
		for _, service := range []string{claudeTokenService, legacyClaudeTokenService} {
			out, err := exec.Command("security", "find-generic-password", "-s", service, "-a", tokenAccount(), "-w").Output()
			if err == nil {
				return strings.TrimSpace(string(out)), nil
			}
		}
		// Not in the Keychain: fall through to the file, which is where a
		// Keychain that refused the write sent it.
	}
	paths, err := claudeTokenPaths()
	if err != nil {
		return "", err
	}
	for _, p := range paths {
		data, err := os.ReadFile(p)
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		return strings.TrimSpace(string(data)), err
	}
	return "", nil
}

// saveClaudeToken stores token and says where.
func saveClaudeToken(token string) (string, error) {
	if !claudeTokenPattern.MatchString(token) {
		return "", errors.New("invalid token")
	}
	if runtime.GOOS == "darwin" {
		// security -i reads the command from stdin, so the token never
		// appears on a command line.
		c := exec.Command("security", "-i")
		c.Stdin = strings.NewReader(fmt.Sprintf("add-generic-password -U -a %s -s %s -w %s\n", tokenAccount(), claudeTokenService, token))
		if out, err := c.CombinedOutput(); err == nil && !strings.Contains(string(out), "error") {
			return "in the macOS Keychain", nil
		}
	}
	p, err := claudeTokenPath()
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(filepath.Dir(p), 0o700); err != nil {
		return "", err
	}
	if err := os.WriteFile(p, []byte(token+"\n"), 0o600); err != nil {
		return "", err
	}
	return "in " + p, nil
}

// removeClaudeToken forgets the token everywhere it's read from --
// boxctl's places included, or logging out wouldn't.
func removeClaudeToken() {
	if runtime.GOOS == "darwin" {
		for _, service := range []string{claudeTokenService, legacyClaudeTokenService} {
			_ = exec.Command("security", "delete-generic-password", "-s", service, "-a", tokenAccount()).Run()
		}
	}
	if paths, err := claudeTokenPaths(); err == nil {
		for _, p := range paths {
			_ = os.Remove(p)
		}
	}
}

// claudeTokenPath is where the token file is written.
func claudeTokenPath() (string, error) {
	paths, err := claudeTokenPaths()
	if err != nil {
		return "", err
	}
	return paths[0], nil
}

// claudeTokenPaths are where the token file is read from: onctl's, then
// boxctl's.
func claudeTokenPaths() ([]string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, err
	}
	return []string{
		filepath.Join(home, ".onctl", "claude", claudeTokenFile),
		filepath.Join(home, ".boxctl", "claude", claudeTokenFile),
	}, nil
}

// tokenAccount is the Keychain account the token is filed under: this
// user, so the entry reads as theirs in Keychain Access. Plain
// characters only -- it's part of a security -i command line.
func tokenAccount() string {
	if u, err := user.Current(); err == nil && regexp.MustCompile(`^[A-Za-z0-9._-]+$`).MatchString(u.Username) {
		return u.Username
	}
	return "onctl"
}
