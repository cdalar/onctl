package cmd

import (
	"cdalar/onctl/internal/cloud"
	"cdalar/onctl/internal/provideraws"
	"cdalar/onctl/internal/providerhtz"
	"cdalar/onctl/internal/tools"
	"fmt"
	"log"
	"os"

	"github.com/spf13/cobra"
)

var destroyCmd = &cobra.Command{
	Use:   "destroy",
	Short: "Destroy VM ex. `onctl destroy i-04a38e5063a838483`",
	Run: func(cmd *cobra.Command, args []string) {
		log.Println("[DEBUG] args: ", args)
		if len(args) == 0 {
			fmt.Println("Please provide a VM id or 'all' to destroy all VMs")
			return
		}

		// Set up cloud provider and client
		var provider cloud.CloudProviderInterface
		switch os.Getenv("CLOUD_PROVIDER") {
		case "hetzner":
			provider = &cloud.ProviderHetzner{
				Client: providerhtz.GetClient(),
			}
		case "aws":
			provider = &cloud.ProviderAws{
				Client: provideraws.GetClient(),
			}
		default:
			log.Fatal("Unknown cloud provider")
		}

		switch args[0] {
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
