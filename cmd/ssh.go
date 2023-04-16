package cmd

import (
	"cdalar/onctl/internal/cloud"
	"cdalar/onctl/internal/provideraws"
	"cdalar/onctl/internal/providerhtz"
	"fmt"
	"log"
	"os"

	"github.com/spf13/cobra"
)

var sshCmd = &cobra.Command{
	Use:                   "ssh [FLAGS] VM Name [COMMAND...]",
	Short:                 "Spawn an SSH connection to a VM",
	Args:                  cobra.MinimumNArgs(1),
	TraverseChildren:      true,
	DisableFlagsInUseLine: true,
	Run: func(cmd *cobra.Command, args []string) {
		log.Println("[DEBUG] args: ", args)
		if len(args) == 0 {
			fmt.Println("Please provide a VM id")
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

		provider.SSHInto(args[0])

	},
}
