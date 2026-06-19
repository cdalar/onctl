package tools

import (
	"os"
	"path/filepath"
)

func Contains(slice []string, searchValue string) bool {
	for _, value := range slice {
		if value == searchValue {
			return true
		}
	}
	return false
}

var providerEnvChecks = []struct {
	name  string
	check func() bool
}{
	{"aws", func() bool {
		if os.Getenv("AWS_ACCESS_KEY_ID") != "" || os.Getenv("AWS_PROFILE") != "" {
			return true
		}
		// Common AWS SDK shared config locations. Even without explicit
		// AWS_* env vars or ONCTL_CLOUD, a populated ~/.aws/credentials
		// (or config) means the user can auth to AWS and ls should consider it.
		home, _ := os.UserHomeDir()
		candidates := []string{
			filepath.Join(home, ".aws", "credentials"),
			filepath.Join(home, ".aws", "config"),
			os.Getenv("AWS_SHARED_CREDENTIALS_FILE"),
			os.Getenv("AWS_CONFIG_FILE"),
		}
		for _, c := range candidates {
			if c != "" {
				if _, err := os.Stat(c); err == nil {
					return true
				}
			}
		}
		return false
	}},
	{"azure", func() bool { return os.Getenv("AZURE_CLIENT_ID") != "" }},
	{"gcp", func() bool { return os.Getenv("GOOGLE_CREDENTIALS") != "" }},
	{"hetzner", func() bool { return os.Getenv("HCLOUD_TOKEN") != "" }},
}

// DetectCloudProviders returns every provider that looks configured either
// through credential env vars or by the presence of its onctl provider
// yaml (<name>.yaml) in standard config locations. Used by `onctl ls` to
// decide whether to query one or all clouds when no explicit --provider or
// ONCTL_CLOUD is in effect. We deliberately keep env + fs heuristics rather
// than attempting to actually auth at detection time (clients are built only
// when we decide to list).
func DetectCloudProviders() []string {
	var found []string
	for _, c := range providerEnvChecks {
		if c.check() {
			found = append(found, c.name)
		}
	}
	// Also detect via onctl yaml files. This catches common flows where
	// users ran `onctl init` for several providers (yamls always written),
	// or azure/gcp where the yaml carries subscription/project required even
	// if the specific env marker is absent.
	for _, name := range []string{"aws", "hetzner", "azure", "gcp", "fc"} {
		if hasProviderYAML(name) && !Contains(found, name) {
			found = append(found, name)
		}
	}
	return found
}

// hasProviderYAML reports whether a <name>.yaml (e.g. aws.yaml) exists in
// any onctl config directory (cwd/.onctl or ~/.onctl).
func hasProviderYAML(name string) bool {
	for _, dir := range onctlConfigDirs() {
		if _, err := os.Stat(filepath.Join(dir, name+".yaml")); err == nil {
			return true
		}
	}
	return false
}

// onctlConfigDirs returns the list of directories where onctl stores provider
// yamls (searched in the same order as ReadConfig).
func onctlConfigDirs() []string {
	var dirs []string
	if cwd, err := os.Getwd(); err == nil {
		dirs = append(dirs, filepath.Join(cwd, ".onctl"))
	}
	if home, err := os.UserHomeDir(); err == nil {
		dirs = append(dirs, filepath.Join(home, ".onctl"))
	}
	return dirs
}

// WhichCloudProvider resolves the single provider for non-ls commands. It only
// looks at credential env vars (matching its pre-multi-provider behavior), not
// the yaml-presence heuristics in DetectCloudProviders: a placeholder
// <provider>.yaml from `onctl init` should not make create/destroy/ssh/etc.
// silently target an uncredentialed cloud instead of failing with "No Cloud
// Provider Set".
func WhichCloudProvider() string {
	for _, c := range providerEnvChecks {
		if c.check() {
			return c.name
		}
	}
	return "none"
}
