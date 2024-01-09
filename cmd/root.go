package cmd

import (
	"fmt"
	"log"
	"os"
	"strings"

	"github.com/cdalar/onctl/internal/cloud"
	"github.com/cdalar/onctl/internal/provideraws"
	"github.com/cdalar/onctl/internal/providerazure"
	"github.com/cdalar/onctl/internal/providerhtz"
	"github.com/cdalar/onctl/internal/provideroracle"
	"github.com/cdalar/onctl/internal/tools"

	"github.com/spf13/cobra"
)

var (
	rootCmd = &cobra.Command{
		Use:   "onctl",
		Short: "onctl is a tool to manage cross platform resources in cloud",
	}
	cloudProvider     string
	cloudProviderList = []string{"aws", "hetzner", "azure", "oracle"}
	provider          cloud.CloudProviderInterface
)

func checkCloudProvider() {
	cloudProvider = os.Getenv("ONCTL_CLOUD")
	// ONCTL_CLOUD is set
	if cloudProvider != "" {
		if !tools.Contains(cloudProviderList, cloudProvider) {
			log.Println("Cloud Platform (" + cloudProvider + ") is not Supported\nPlease use one of the following: " + strings.Join(cloudProviderList, ","))
			os.Exit(1)
		}
	} else { // ONCTL_CLOUD is not set
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
	if len(os.Args) > 1 && os.Args[1] != "init" && os.Args[1] != "version" {
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
			ResourceGraphClient: providerazure.GetResourceGraphClient(),
			VmClient:            providerazure.GetVmClient(),
			NicClient:           providerazure.GetNicClient(),
			PublicIPClient:      providerazure.GetIPClient(),
			SSHKeyClient:        providerazure.GetSSHKeyClient(),
			VnetClient:          providerazure.GetVnetClient(),
		}
	case "oracle":
		provider = &cloud.ProviderOracle{
			Client: provideroracle.GetComputeClient(),
			Base:   provideroracle.GetBaseClient(),
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
