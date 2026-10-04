package providerboxes

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func withHome(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	orig := home
	home = func() (string, error) { return dir, nil }
	t.Cleanup(func() { home = orig })
	t.Setenv(TokenEnv, "")
	return dir
}

func writeJSON(t *testing.T, path, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestLoadCredentials(t *testing.T) {
	dir := withHome(t)

	if _, err := LoadCredentials(""); !errors.Is(err, ErrNotLoggedIn) {
		t.Fatalf("no token anywhere: got %v", err)
	}

	// boxctl's saved token works without logging in again.
	writeJSON(t, filepath.Join(dir, ".boxctl", "config.json"), `{"api_url":"https://legacy","token":"old"}`)
	c, err := LoadCredentials("")
	if err != nil || c.Token != "old" || c.APIURL != "https://legacy" {
		t.Fatalf("legacy: %+v %v", c, err)
	}

	// onctl's own file wins over boxctl's.
	if err := SaveCredentials(&Credentials{Token: "new"}); err != nil {
		t.Fatal(err)
	}
	c, err = LoadCredentials("")
	if err != nil || c.Token != "new" || c.APIURL != DefaultAPIURL {
		t.Fatalf("saved: %+v %v", c, err)
	}
	info, err := os.Stat(filepath.Join(dir, ".onctl", "boxes.json"))
	if err != nil || info.Mode().Perm() != 0o600 {
		t.Fatalf("the token file must be private: %v %v", info.Mode(), err)
	}

	// boxes.apiURL from onctl.yaml wins over the saved URL.
	if c, _ := LoadCredentials("https://staging"); c.APIURL != "https://staging" {
		t.Fatalf("apiURL override: %+v", c)
	}

	// The environment wins over everything, for CI.
	t.Setenv(TokenEnv, "ci")
	if c, _ := LoadCredentials(""); c.Token != "ci" {
		t.Fatalf("env: %+v", c)
	}

	// Logging out removes onctl's file only.
	t.Setenv(TokenEnv, "")
	if err := ClearCredentials(); err != nil {
		t.Fatal(err)
	}
	if c, _ := LoadCredentials(""); c.Token != "old" {
		t.Fatalf("after logout boxctl's own token should remain usable: %+v", c)
	}
}
