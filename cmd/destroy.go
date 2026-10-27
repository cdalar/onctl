package cmd

import (
	"fmt"
	"log"
	"os"
	"sync"

	"github.com/cdalar/onctl/internal/ui"
	"github.com/cdalar/onctl/pkg/cloud"

	"github.com/spf13/cobra"
)

var (
	force bool
)

func init() {
	destroyCmd.Flags().BoolVarP(&force, "force", "f", false, "force destroy VM(s) without confirmation")
	// Register destroy command at root level for convenience
	rootCmd.AddCommand(destroyCmd)
}

var destroyCmd = &cobra.Command{
	Use:     "destroy",
	Aliases: []string{"down", "delete", "remove", "rm"},
	Short:   "Destroy VM(s)",
	ValidArgsFunction: func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
		ensureProvider()
		if provider == nil {
			return nil, cobra.ShellCompDirectiveError
		}
		VMList, err := provider.List()
		list := []string{}
		for _, vm := range VMList.List {
			list = append(list, vm.Name)
		}

		if err != nil {
			return nil, cobra.ShellCompDirectiveError
		}

		return list, cobra.ShellCompDirectiveNoFileComp
	},
	Run: func(cmd *cobra.Command, args []string) {
		s := ui.New()               // Build our new spinner
		defer ensureCursorVisible() // Ensure cursor is visible when function exits
		log.Println("[DEBUG] args: ", args)
		if len(args) == 0 {
			fmt.Println("Please provide a VM id or 'all' to destroy all VMs")
			return
		}

		switch args[0] {
		case "all":
			// Tear down all servers
			if !force {
				if !yesNo() {
					os.Exit(0)
				}
			}
			log.Println("[DEBUG] Tear down all servers")
			servers, err := provider.List()
			if err != nil {
				log.Println(err)
			}
			log.Println("[DEBUG] Servers: ", servers.List)
			var wg sync.WaitGroup
			for _, server := range servers.List {
				wg.Add(1)
				go func(server cloud.Vm) {
					defer wg.Done()
					s.Suffix = " Destroying VM..."
					s.Start()
					if err := provider.Destroy(server); err != nil {
						fmt.Println("\033[31m✘\033[0m Could not destroy VM: " + server.Name)
						log.Println(err)
					}
					s.Stop()
					fmt.Println("\033[32m✔\033[0m VM Destroyed: " + server.Name)
				}(server)
			}
			wg.Wait()
			fmt.Println("\033[32m✔\033[0m ALL VM(s) are destroyed")
		default:
			// Tear down specific server
			serverName := args[0]
			log.Println("[DEBUG] Tear down server: " + serverName)
			if sd, ok := provider.(cloud.SnapshotDestroyer); ok && isPaused(serverName) {
				destroySnapshot(sd, serverName)
				return
			}
			if !force {
				fmt.Printf("This will permanently destroy VM %q and all its data. This cannot be undone.\n", serverName)
				if !yesNo() {
					os.Exit(0)
				}
			}
			s.Suffix = " Destroying VM..."
			s.Start()
			if err := provider.Destroy(cloud.Vm{Name: serverName}); err != nil {
				s.Stop()
				fmt.Println("\033[31m✘\033[0m Cannot destroy VM: " + serverName)
				fmt.Println(err)
				os.Exit(1)
			}
			s.Stop()
			fmt.Println("\033[32m✔\033[0m VM Destroyed: " + serverName)
		}
	},
}

// isPaused tells whether name is among the provider's paused VMs.
func isPaused(name string) bool {
	paused, err := provider.ListPaused()
	if err != nil {
		log.Println("[DEBUG] listing paused VMs: ", err)
		return false
	}
	for _, vm := range paused.List {
		if vm.Name == name {
			return true
		}
	}
	return false
}

// destroySnapshot deletes a paused VM that's only a snapshot (Hetzner's),
// saying that's what it is first: deleted, it can never be resumed.
func destroySnapshot(sd cloud.SnapshotDestroyer, name string) {
	if !force {
		fmt.Printf("%q is paused: all that's left of it is a snapshot (and the public IP kept for it).\n", name)
		fmt.Println("This will permanently delete the snapshot, so it can never be resumed. This cannot be undone.")
		if !yesNo() {
			os.Exit(0)
		}
	}
	if err := sd.DestroySnapshot(name); err != nil {
		fmt.Println("\033[31m✘\033[0m Cannot delete the snapshot of " + name)
		fmt.Println(err)
		os.Exit(1)
	}
	fmt.Println("\033[32m✔\033[0m Snapshot deleted: " + name)
}
