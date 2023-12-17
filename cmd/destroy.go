package cmd

import (
	"fmt"
	"log"

	"github.com/cdalar/onctl/internal/cloud"
	"github.com/cdalar/onctl/internal/tools"

	"github.com/spf13/cobra"
)

var destroyCmd = &cobra.Command{
	Use:     "destroy",
	Aliases: []string{"down", "delete", "remove", "rm"},
	Short:   "Destroy VM(s)",
	Run: func(cmd *cobra.Command, args []string) {
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
			}
		default:
			// Tear down specific server
			serverName := args[0]
			log.Println("[DEBUG] Tear down server: " + serverName)
			if err := provider.Destroy(cloud.Vm{Name: serverName}); err != nil {
				log.Println(err)
			}
		}
	},
}
