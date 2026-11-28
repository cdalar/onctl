package box

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"

	"github.com/cdalar/onctl/internal/tools"
	"github.com/cdalar/onctl/pkg/cloud"
)

// Import registers a host onctl didn't create (a dedicated server, any
// reachable machine) as an imported host of the static provider, in
// ~/.ssh/onctl_config, and makes sure ~/.ssh/config Includes that file so
// plain `ssh NAME` reaches it too. A host of the same name is replaced;
// updated says so.
func Import(h cloud.StaticHost) (updated bool, err error) {
	path, err := tools.OnctlSSHConfigPath()
	if err != nil {
		return false, err
	}
	p := cloud.ProviderStatic{InventoryPath: path}
	inv, err := p.LoadInventory()
	if err != nil {
		return false, err
	}
	for i, existing := range inv.Hosts {
		if existing.Name == h.Name {
			inv.Hosts[i] = h
			updated = true
			break
		}
	}
	if !updated {
		inv.Hosts = append(inv.Hosts, h)
	}
	if err := p.SaveInventory(inv); err != nil {
		return false, err
	}
	return updated, ensureSSHConfigInclude(path)
}

// hasActiveInclude reports whether data already has an active (uncommented)
// "Include <target>" directive pointing at target, so a commented-out line
// or an unrelated Include whose path merely starts with target doesn't
// produce a false "already included" positive.
func hasActiveInclude(data []byte, target string) bool {
	for _, line := range strings.Split(string(data), "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		fields := cloud.SplitSSHConfigFields(trimmed)
		if len(fields) < 2 || !strings.EqualFold(fields[0], "include") {
			continue
		}
		for _, f := range fields[1:] {
			if f == target {
				return true
			}
		}
	}
	return false
}

// ensureSSHConfigInclude makes sure ~/.ssh/config includes onctlConfigPath,
// so imported hosts are reachable with plain `ssh <name>` too, not just
// through onctl. It's idempotent: a no-op once the Include line is present.
// The Include is inserted first so imported hosts aren't shadowed by a
// later catch-all "Host *" block.
func ensureSSHConfigInclude(onctlConfigPath string) error {
	sshDir := filepath.Dir(onctlConfigPath)
	if err := os.MkdirAll(sshDir, 0700); err != nil {
		return fmt.Errorf("failed to create %s: %w", sshDir, err)
	}

	configPath := filepath.Join(sshDir, "config")

	data, err := os.ReadFile(configPath)
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to read %s: %w", configPath, err)
	}
	if hasActiveInclude(data, onctlConfigPath) {
		return nil
	}

	includeLine := "Include " + cloud.QuoteSSHConfigValue(onctlConfigPath)
	newContent := includeLine + "\n\n" + string(data)
	if err := os.WriteFile(configPath, []byte(newContent), 0600); err != nil {
		return fmt.Errorf("failed to update %s: %w", configPath, err)
	}
	log.Printf("[DEBUG] added %q to %s", includeLine, configPath)
	return nil
}
