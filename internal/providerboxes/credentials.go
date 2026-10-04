package providerboxes

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

// Credentials are what `onctl login` saves: the service's URL and a
// personal token from its dashboard.
type Credentials struct {
	APIURL string `json:"api_url"`
	Token  string `json:"token"`
}

// TokenEnv overrides the saved token (and needs no login), for CI.
const TokenEnv = "ONCTL_BOXES_TOKEN"

// ErrNotLoggedIn is returned when there's no token anywhere.
var ErrNotLoggedIn = errors.New("not logged in to boxes: run `onctl login` with a token from the dashboard, or set " + TokenEnv)

// home is os.UserHomeDir, swappable in tests.
var home = os.UserHomeDir

// CredentialsPath is ~/.onctl/boxes.json.
func CredentialsPath() (string, error) {
	h, err := home()
	if err != nil {
		return "", err
	}
	return filepath.Join(h, ".onctl", "boxes.json"), nil
}

// legacyPath is boxctl's own ~/.boxctl/config.json, the same shape.
func legacyPath() (string, error) {
	h, err := home()
	if err != nil {
		return "", err
	}
	return filepath.Join(h, ".boxctl", "config.json"), nil
}

func readCredentials(path string) (*Credentials, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var c Credentials
	if err := json.Unmarshal(data, &c); err != nil {
		return nil, fmt.Errorf("reading %s: %w", path, err)
	}
	return &c, nil
}

// LoadCredentials finds the token: $ONCTL_BOXES_TOKEN, then
// ~/.onctl/boxes.json, then boxctl's ~/.boxctl/config.json (so a boxctl
// user needn't log in again). apiURL, when set (boxes.apiURL in
// onctl.yaml), wins over the saved one; DefaultAPIURL is the fallback.
func LoadCredentials(apiURL string) (*Credentials, error) {
	creds := &Credentials{}
	if tok := os.Getenv(TokenEnv); tok != "" {
		creds.Token = tok
	} else {
		for _, pathFn := range []func() (string, error){CredentialsPath, legacyPath} {
			p, err := pathFn()
			if err != nil {
				return nil, err
			}
			c, err := readCredentials(p)
			if errors.Is(err, os.ErrNotExist) {
				continue
			}
			if err != nil {
				return nil, err
			}
			if c.Token != "" {
				creds = c
				break
			}
		}
	}
	if creds.Token == "" {
		return nil, ErrNotLoggedIn
	}
	if apiURL != "" {
		creds.APIURL = apiURL
	}
	if creds.APIURL == "" {
		creds.APIURL = DefaultAPIURL
	}
	return creds, nil
}

// SaveCredentials writes ~/.onctl/boxes.json, readable only by the user.
func SaveCredentials(c *Credentials) error {
	p, err := CredentialsPath()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(p), 0o700); err != nil {
		return err
	}
	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}
	tmp := p + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, p)
}

// ClearCredentials removes ~/.onctl/boxes.json. boxctl's own file is left
// alone: it belongs to boxctl.
func ClearCredentials() error {
	p, err := CredentialsPath()
	if err != nil {
		return err
	}
	if err := os.Remove(p); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return nil
}
