package cmd

import (
	"fmt"
	"log"
	"os"
	"sync"
	"time"

	"github.com/briandowns/spinner"
	"github.com/cdalar/onctl/internal/cloud"

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
		s := spinner.New(spinner.CharSets[9], 100*time.Millisecond) // Build our new spinner
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
					s.Start()
					s.Suffix = " Destroying VM..."
					if err := provider.Destroy(server); err != nil {
						fmt.Println("\033[31m\u2718\033[0m Could not destroy VM: " + server.Name)
						log.Println(err)
					}
					s.Stop()
					fmt.Println("\033[32m\u2714\033[0m VM Destroyed: " + server.Name)
				}(server)
			}
			wg.Wait()
			fmt.Println("\033[32m\u2714\033[0m ALL VM(s) are destroyed")
		default:
			// Tear down specific server
			serverName := args[0]
			log.Println("[DEBUG] Tear down server: " + serverName)
			s.Start()
			s.Suffix = " Destroying VM..."
			if err := provider.Destroy(cloud.Vm{Name: serverName}); err != nil {
				s.Stop()
				fmt.Println("\033[31m\u2718\033[0m Cannot destroy VM: " + serverName)
				fmt.Println(err)
				os.Exit(1)
			}
			s.Stop()
			fmt.Println("\033[32m\u2714\033[0m VM Destroyed: " + serverName)
		}
	},
}
