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

	// The saved file is read back, with its API URL.
	writeJSON(t, filepath.Join(dir, ".onctl", "boxes.json"), `{"api_url":"https://saved","token":"old"}`)
	c, err := LoadCredentials("")
	if err != nil || c.Token != "old" || c.APIURL != "https://saved" {
		t.Fatalf("saved file: %+v %v", c, err)
	}

	// Logging in replaces it.
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

	// Logging out leaves no token. boxctl's own ~/.boxctl/config.json is
	// never read -- onctl's credentials are its own -- so it doesn't stand
	// in for the removed one.
	t.Setenv(TokenEnv, "")
	writeJSON(t, filepath.Join(dir, ".boxctl", "config.json"), `{"token":"boxctl"}`)
	if err := ClearCredentials(); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadCredentials(""); !errors.Is(err, ErrNotLoggedIn) {
		t.Fatalf("after logout, with only boxctl's file present: got %v, want not logged in", err)
	}
}
