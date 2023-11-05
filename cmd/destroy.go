package cmd

import (
	"cdalar/onctl/internal/cloud"
	"cdalar/onctl/internal/tools"
	"fmt"
	"log"

	"github.com/spf13/cobra"
)

var destroyCmd = &cobra.Command{
	Use:     "destroy",
	Aliases: []string{"teardown", "down", "delete", "remove"},
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
			serverID := args[0]
			log.Println("[DEBUG] Tear down server: " + serverID)
			if err := provider.Destroy(cloud.Vm{ID: serverID}); err != nil {
				log.Println(err)
			}
		}
	},
}
