package cmd

import (
	"fmt"
	"log"
	"time"

	"github.com/briandowns/spinner"
	"github.com/cdalar/onctl/internal/cloud"
	"github.com/cdalar/onctl/internal/tools"

	"github.com/spf13/cobra"
)

var destroyCmd = &cobra.Command{
	Use:     "destroy",
	Aliases: []string{"down", "delete", "remove", "rm"},
	Short:   "Destroy VM(s)",
	Run: func(cmd *cobra.Command, args []string) {
		s := spinner.New(spinner.CharSets[9], 100*time.Millisecond) // Build our new spinner
		// s.Start()                                                   // Start the spinner
		log.Println("[DEBUG] args: ", args)
		if len(args) == 0 {
			fmt.Println("Please provide a VM id or 'all' to destroy all VMs")
			return
		}

		switch args[0] {
		// TODO: only works on the current directory
		case "self":
			serverName := tools.GenerateMachineUniqueName()
			log.Println("[DEBUG] Tear down self: " + serverName)
			if err := provider.Destroy(cloud.Vm{Name: serverName}); err != nil {
				log.Println(err)
			}
		case "all":
			// Tear down all servers
			log.Println("[DEBUG] Tear down all servers")
			servers, err := provider.List()
			if err != nil {
				log.Println(err)
			}
			log.Println("[DEBUG] Servers: ", servers.List)
			for _, server := range servers.List {
				if err := provider.Destroy(server); err != nil {
					log.Println(err)
				}
				log.Println(server.Name, "deleted.")
			}
		default:
			// Tear down specific server
			serverName := args[0]
			log.Println("[DEBUG] Tear down server: " + serverName)
			s.Start()
			s.Suffix = " Destroying VM..."
			if err := provider.Destroy(cloud.Vm{Name: serverName}); err != nil {
				s.Stop()
				fmt.Println("\033[31m\u2718\033[0m Could not destroy VM: " + serverName)
				log.Println(err)
			}
			s.Stop()
			fmt.Println("\033[32m\u2714\033[0m VM Destroyed: " + serverName)
		}
	},
}
