package cmd

import (
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/cdalar/onctl/pkg/cloud"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
)

// legacyImportedHostsFile is where `onctl import` wrote its inventory before
// this file moved to ~/.ssh/onctl_config (see migrateLegacyImportedYAML).
const legacyImportedHostsFile = "imported.yaml"

type cmdImportOptions struct {
	IP       string
	Username string
	Port     int
	Key      string
}

var importOpt cmdImportOptions

func init() {
	importCmd.Flags().StringVar(&importOpt.IP, "ip", "", "IP address of the server to import (required)")
	importCmd.Flags().StringVarP(&importOpt.Username, "user", "u", "root", "SSH username to connect as")
	importCmd.Flags().IntVarP(&importOpt.Port, "port", "P", 22, "ssh port")
	importCmd.Flags().StringVarP(&importOpt.Key, "key", "k", "", "Path to privateKey file for this host (default: ssh.privateKey from onctl.yaml)")
	_ = importCmd.MarkFlagRequired("ip")
	rootCmd.AddCommand(importCmd)
}

// onctlSSHConfigPath is the fixed location of onctl's own ssh_config-format
// inventory of imported hosts. It lives under ~/.ssh (not the .onctl config
// dir) so it can be `Include`d from ~/.ssh/config and imported hosts stay
// reachable with plain `ssh <name>`, independent of --config/-c.
func onctlSSHConfigPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("failed to get home directory: %w", err)
	}
	path := filepath.Join(home, ".ssh", "onctl_config")
	if err := migrateLegacyImportedYAML(path); err != nil {
		fmt.Printf("Warning: failed to migrate legacy imported-hosts inventory: %v\n", err)
	}
	return path, nil
}

// migrateLegacyImportedYAML copies hosts from the pre-ssh_config
// .onctl/imported.yaml inventory into newPath once, so upgrading onctl
// doesn't silently drop hosts registered with `onctl import` before this
// file moved under ~/.ssh. No-op once newPath exists, or if there is no
// .onctl dir (nothing to migrate) or no legacy file in it.
func migrateLegacyImportedYAML(newPath string) error {
	if _, err := os.Stat(newPath); err == nil {
		return nil
	}
	configDir, err := resolveConfigDir()
	if err != nil {
		return nil
	}
	legacyPath := filepath.Join(configDir, legacyImportedHostsFile)
	data, err := os.ReadFile(legacyPath)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("failed to read %s: %w", legacyPath, err)
	}

	// Matches the pre-ssh_config StaticHost yaml tags exactly (see git
	// history of pkg/cloud/static.go); cloud.StaticHost itself no longer
	// carries yaml tags now that it's serialized as ssh_config instead.
	var legacy struct {
		Hosts []struct {
			Name       string    `yaml:"name"`
			IP         string    `yaml:"ip"`
			Username   string    `yaml:"username"`
			SSHPort    int       `yaml:"sshPort"`
			PrivateKey string    `yaml:"privateKey"`
			ImportedAt time.Time `yaml:"importedAt"`
		} `yaml:"hosts"`
	}
	if err := yaml.Unmarshal(data, &legacy); err != nil {
		return fmt.Errorf("failed to parse %s: %w", legacyPath, err)
	}
	if len(legacy.Hosts) == 0 {
		return nil
	}

	inv := cloud.StaticInventory{}
	for _, h := range legacy.Hosts {
		inv.Hosts = append(inv.Hosts, cloud.StaticHost{
			Name:       h.Name,
			IP:         h.IP,
			Username:   h.Username,
			SSHPort:    h.SSHPort,
			PrivateKey: h.PrivateKey,
			ImportedAt: h.ImportedAt,
		})
	}

	p := cloud.ProviderStatic{InventoryPath: newPath}
	if err := p.SaveInventory(inv); err != nil {
		return err
	}
	if err := ensureSSHConfigInclude(newPath); err != nil {
		return err
	}
	fmt.Printf("Migrated %d imported host(s) from %s to %s\n", len(legacy.Hosts), legacyPath, newPath)
	return nil
}

// staticProvider builds a ProviderStatic backed by onctl's own ssh_config
// file, without going through initProvider/initState (import needs no cloud
// credentials).
func staticProvider() (cloud.ProviderStatic, error) {
	path, err := onctlSSHConfigPath()
	if err != nil {
		return cloud.ProviderStatic{}, err
	}
	return cloud.ProviderStatic{InventoryPath: path}, nil
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

var importCmd = &cobra.Command{
	Use:   "import NAME",
	Short: "Import an existing server so it can be managed with ssh/ls",
	Long: `Import registers a server onctl did not create (e.g. a Hetzner auction/dedicated
box, or any other reachable host) so it shows up in 'onctl --provider static ls'
and can be reached with 'onctl --provider static ssh NAME'.

The host is written to ~/.ssh/onctl_config, which is Included from
~/.ssh/config, so it's also reachable with plain 'ssh NAME'.

Imported hosts cannot be created/destroyed/paused through a cloud API since
onctl doesn't manage their lifecycle; 'destroy' on an imported host only
removes it from onctl's local record, it does not affect the real machine.`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		name := args[0]
		p, err := staticProvider()
		if err != nil {
			return err
		}

		inv, err := p.LoadInventory()
		if err != nil {
			return err
		}

		host := cloud.StaticHost{
			Name:       name,
			IP:         importOpt.IP,
			Username:   importOpt.Username,
			SSHPort:    importOpt.Port,
			PrivateKey: importOpt.Key,
			ImportedAt: time.Now(),
		}

		updated := false
		for i, h := range inv.Hosts {
			if h.Name == name {
				inv.Hosts[i] = host
				updated = true
				break
			}
		}
		if !updated {
			inv.Hosts = append(inv.Hosts, host)
		}

		if err := p.SaveInventory(inv); err != nil {
			return err
		}
		if err := ensureSSHConfigInclude(p.InventoryPath); err != nil {
			return err
		}

		log.Printf("[DEBUG] imported host %q -> %s", name, importOpt.IP)
		if updated {
			fmt.Printf("\033[32m✔\033[0m Updated imported host %q (%s)\n", name, importOpt.IP)
		} else {
			fmt.Printf("\033[32m✔\033[0m Imported %q (%s)\n", name, importOpt.IP)
		}
		fmt.Printf("Use it with: onctl --provider static ssh %s\n", name)
		return nil
	},
}
