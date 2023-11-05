package cmd

import (
	"cdalar/onctl/internal/cloud"
	"cdalar/onctl/internal/provideraws"
	"cdalar/onctl/internal/providerazure"
	"cdalar/onctl/internal/providerhtz"
	"cdalar/onctl/internal/tools"
	"fmt"
	"log"
	"os"
	"slices"
	"strings"

	"github.com/spf13/cobra"
)

var (
	rootCmd = &cobra.Command{
		Use:   "onctl",
		Short: "onctl is a tool to manage cross platform resources in cloud",
	}
	cloudProvider     string
	cloudProviderList = []string{"aws", "hetzner", "azure"}
	provider          cloud.CloudProviderInterface
)

func checkCloudProvider() {
	cloudProvider = os.Getenv("ONCTL_CLOUD")
	fmt.Println("Using: " + cloudProvider)
	if cloudProvider != "" {
		if !tools.Contains(cloudProviderList, cloudProvider) {
			log.Println("Cloud Platform (" + cloudProvider + ") is not Supported\nPlease use one of the following: " + strings.Join(cloudProviderList, ","))
			os.Exit(1)
		}
	} else {
		cloudProvider = tools.WhichCloudProvider()
		if cloudProvider != "none" {
			err := os.Setenv("ONCTL_CLOUD", cloudProvider)
			if err != nil {
				log.Println(err)
			}
			return
		} else {
			fmt.Println("No Cloud Provider Set.\nPlease set the ONCTL_CLOUD environment variable to one of the following: " + strings.Join(cloudProviderList, ","))
			os.Exit(1)
		}
	}
}

// Execute executes the root command.
func Execute() error {
	log.Println("[DEBUG] Args: " + strings.Join(os.Args, ","))
	if len(os.Args) > 1 && !slices.Contains([]string{"version", "init"}, os.Args[1]) {
		checkCloudProvider()
		ReadConfig(cloudProvider)
	}

	switch cloudProvider {
	case "hetzner":
		provider = &cloud.ProviderHetzner{
			Client: providerhtz.GetClient(),
		}
	case "aws":
		provider = &cloud.ProviderAws{
			Client: provideraws.GetClient(),
		}
	case "azure":
		provider = &cloud.ProviderAzure{
			VmClient:       providerazure.GetVmClient(),
			NicClient:      providerazure.GetNicClient(),
			PublicIPClient: providerazure.GetIPClient(),
			SSHKeyClient:   providerazure.GetSSHKeyClient(),
		}
	}
	return rootCmd.Execute()
}

func init() {
	rootCmd.AddCommand(versionCmd)
	rootCmd.AddCommand(listCmd)
	rootCmd.AddCommand(createCmd)
	rootCmd.AddCommand(destroyCmd)
	rootCmd.AddCommand(sshCmd)
	rootCmd.AddCommand(initCmd)
}
