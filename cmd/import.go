package cmd

import (
	"fmt"
	"log"
	"path/filepath"
	"time"

	"github.com/cdalar/onctl/pkg/cloud"
	"github.com/spf13/cobra"
)

const importedHostsFile = "imported.yaml"

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

// staticProvider builds a ProviderStatic backed by the inventory file in the
// resolved .onctl config dir, without going through initProvider/initState
// (import needs no cloud credentials).
func staticProvider() (cloud.ProviderStatic, error) {
	configDir, err := resolveConfigDir()
	if err != nil {
		return cloud.ProviderStatic{}, err
	}
	return cloud.ProviderStatic{InventoryPath: filepath.Join(configDir, importedHostsFile)}, nil
}

var importCmd = &cobra.Command{
	Use:   "import NAME",
	Short: "Import an existing server so it can be managed with ssh/ls",
	Long: `Import registers a server onctl did not create (e.g. a Hetzner auction/dedicated
box, or any other reachable host) so it shows up in 'onctl --provider static ls'
and can be reached with 'onctl --provider static ssh NAME'.

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
