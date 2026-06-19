package cmd

import (
	"fmt"
	"log"
	"os"
	"strings"

	"github.com/cdalar/onctl/internal/provideraws"
	"github.com/cdalar/onctl/internal/providerazure"
	"github.com/cdalar/onctl/internal/providerfirecracker"
	"github.com/cdalar/onctl/internal/providergcp"
	"github.com/cdalar/onctl/internal/providerhtz"
	"github.com/cdalar/onctl/internal/tools"
	"github.com/cdalar/onctl/pkg/cloud"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var (
	rootCmd = &cobra.Command{
		Use:   "onctl",
		Short: "onctl is a tool to manage cross platform resources in cloud",
		Long:  `onctl is a tool to manage cross platform resources in cloud`,
		Example: `  # List all VMs
  onctl ls

  # Create a VM with docker installed
  onctl create -n test -a docker/docker.sh

  # SSH into a VM
  onctl ssh test

  # Destroy a VM
  onctl destroy test`,
		// PersistentPreRunE runs after cobra parses flags, so flag→viper
		// bindings (set in each command's init) are in effect before we build
		// the provider config. init/version/help and the bare root need no
		// provider, so they are skipped.
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			// Bare root (no subcommand) and commands that need no provider.
			if cmd.Parent() == nil {
				return nil
			}
			switch cmd.Name() {
			case "init", "version", "help":
				return nil
			}
			setDefaults()
			if providerFlag != "" {
				if !tools.Contains(cloudProviderList, providerFlag) {
					return fmt.Errorf("unsupported provider %q; use one of: %s", providerFlag, strings.Join(cloudProviderList, ", "))
				}
				if err := os.Setenv("ONCTL_CLOUD", providerFlag); err != nil {
					return err
				}
			}
			cloudProvider = checkCloudProvider()
			log.Println("[DEBUG] Cloud: " + cloudProvider)
			// Best-effort: a missing .onctl is fine now that defaults exist.
			if err := ReadConfig(cloudProvider); err != nil {
				log.Println("[DEBUG] no config file loaded, using defaults:", err)
			}
			// The images command initializes its own provider lazily.
			if cmd.Name() != "images" {
				initProvider(cloudProvider)
			}
			return nil
		},
	}
	cloudProvider     string
	cloudProviderList = []string{"aws", "hetzner", "azure", "gcp", "firecracker"}
	provider          cloud.CloudProviderInterface
	providerFlag      string
)

func checkCloudProvider() string {
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
			return cloudProvider
		} else {
			fmt.Println("No Cloud Provider Set.\nPlease set the ONCTL_CLOUD environment variable to one of the following: " + strings.Join(cloudProviderList, ","))
			os.Exit(1)
		}
	}
	return cloudProvider
}

// Execute executes the root command. Provider selection and config loading now
// happen in rootCmd.PersistentPreRunE (after flag parsing) so CLI flags can
// override config values.
func Execute() error {
	log.Println("[DEBUG] Args: " + strings.Join(os.Args, ","))
	return rootCmd.Execute()
}

// initProvider builds the global provider client for the selected cloud.
func initProvider(cloudProvider string) {
	switch cloudProvider {
	case "hetzner":
		provider = &cloud.ProviderHetzner{
			Client: providerhtz.GetClient(),
			Config: cloud.HetznerConfig{
				Location:      viper.GetString("hetzner.location"),
				VMType:        viper.GetString("hetzner.vm.type"),
				Image:         viper.GetString("hetzner.vm.image"),
				Username:      viper.GetString("hetzner.vm.username"),
				SSHPrivateKey: viper.GetString("ssh.privateKey"),
			},
		}
	case "gcp":
		provider = &cloud.ProviderGcp{
			Client:      providergcp.GetClient(),
			GroupClient: providergcp.GetGroupClient(),
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
			NSGClient:           providerazure.GetNSGClient(),
		}
	case "firecracker":
		fcConfig := providerfirecracker.GetConfig()
		provider = &cloud.ProviderFirecracker{
			Config:  fcConfig,
			Process: providerfirecracker.NewProcessManager(fcConfig.BinPath),
			API:     providerfirecracker.NewAPIClient(),
			Net:     providerfirecracker.NewNetworkManager(),
			Rootfs:  providerfirecracker.NewRootfsPreparer(),
		}
	}
}

func init() {
	rootCmd.PersistentFlags().StringVar(&providerFlag, "provider", "", "cloud provider: "+strings.Join(cloudProviderList, ", ")+" (overrides ONCTL_CLOUD)")
	rootCmd.AddCommand(versionCmd)
	rootCmd.AddCommand(initCmd)
}
