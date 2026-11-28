package cmd

import (
	"os"
	"runtime"
	"testing"
)

const testClaudeToken = "sk-ant-oat01-abcdefghijklmnopqrstuvwxyz"

// useTempHome points the token file at a throwaway home and clears the
// env override, so a test never reads or writes the developer's real token.
func useTempHome(t *testing.T) {
	t.Helper()
	if runtime.GOOS == "darwin" {
		t.Skip("saving on macOS goes to the Keychain")
	}
	t.Setenv("HOME", t.TempDir())
	t.Setenv("CLAUDE_CODE_OAUTH_TOKEN", "")
}

// TestClaudeTokenRoundTrip: a token saved by login is the one a box is
// started with, and logout makes the next run fall back to no token.
func TestClaudeTokenRoundTrip(t *testing.T) {
	useTempHome(t)

	if got, err := loadClaudeToken(); err != nil || got != "" {
		t.Fatalf("no saved token: got %q, %v; want \"\", nil", got, err)
	}
	where, err := saveClaudeToken(testClaudeToken)
	if err != nil {
		t.Fatal(err)
	}
	p, _ := claudeTokenPath()
	if where != "in "+p {
		t.Errorf("saved %q, want %q", where, "in "+p)
	}
	// The token is a credential: only its owner may read it.
	if fi, err := os.Stat(p); err != nil || fi.Mode().Perm() != 0o600 {
		t.Errorf("token file mode = %v, %v; want 0600", fi.Mode().Perm(), err)
	}
	if got, err := loadClaudeToken(); err != nil || got != testClaudeToken {
		t.Errorf("load = %q, %v; want the saved token", got, err)
	}

	removeClaudeToken()
	if got, _ := loadClaudeToken(); got != "" {
		t.Errorf("after logout load = %q, want \"\"", got)
	}
}

// TestSaveClaudeTokenRejectsJunk: whitespace or shell metacharacters must
// never reach the token file or a security command line.
func TestSaveClaudeTokenRejectsJunk(t *testing.T) {
	useTempHome(t)
	for _, tok := range []string{"", "short", "has space in it 0123456789", "semi;colon-0123456789abcdef"} {
		if _, err := saveClaudeToken(tok); err == nil {
			t.Errorf("saveClaudeToken(%q) succeeded, want error", tok)
		}
	}
}

// TestLoadClaudeTokenEnvWins: a token in the environment is used over a
// saved one, so CI can supply its own without touching ~/.onctl.
func TestLoadClaudeTokenEnvWins(t *testing.T) {
	useTempHome(t)
	if _, err := saveClaudeToken(testClaudeToken); err != nil {
		t.Fatal(err)
	}
	t.Setenv("CLAUDE_CODE_OAUTH_TOKEN", "  from-env-token  ")
	if got, _ := loadClaudeToken(); got != "from-env-token" {
		t.Errorf("load = %q, want the trimmed env token", got)
	}
}

// TestTokenAccount: the account name goes into a security -i command
// line, so it has to be plain characters.
func TestTokenAccount(t *testing.T) {
	if got := tokenAccount(); got == "" || !claudeTokenAccountOK(got) {
		t.Errorf("tokenAccount() = %q, want plain characters", got)
	}
}

func claudeTokenAccountOK(s string) bool {
	for _, r := range s {
		ok := r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '.' || r == '_' || r == '-'
		if !ok {
			return false
		}
	}
	return true
}

// TestProviderHint: a hint someone copies from an error has to name the
// provider the box is on, or it targets the wrong cloud.
func TestProviderHint(t *testing.T) {
	saved := cloudProvider
	t.Cleanup(func() { cloudProvider = saved })
	cloudProvider = "hetzner"

	if got, want := providerHint("onctl destroy x"), "onctl -p hetzner destroy x"; got != want {
		t.Errorf("providerHint = %q, want %q", got, want)
	}
	if got, want := destroyHint("box1"), "onctl -p hetzner destroy box1"; got != want {
		t.Errorf("destroyHint = %q, want %q", got, want)
	}
}
