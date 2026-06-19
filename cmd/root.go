package cmd

import (
	"fmt"
	"log"
	"os"
	"strings"

	"github.com/cdalar/onctl/internal/provideraws"
	"github.com/cdalar/onctl/internal/providerazure"
	"github.com/cdalar/onctl/internal/providerfc"
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
			case "init", "version", "help", "__complete", "__completeNoDesc":
				return nil
			}
			if providerFlag != "" {
				if !tools.Contains(cloudProviderList, providerFlag) {
					return fmt.Errorf("unsupported provider %q; use one of: %s", providerFlag, strings.Join(cloudProviderList, ", "))
				}
				if err := os.Setenv("ONCTL_CLOUD", providerFlag); err != nil {
					return err
				}
			}
			if err := initState(); err != nil {
				return err
			}
			// The images command initializes its own provider lazily.
			if cmd.Name() != "images" {
				initProvider(cloudProvider)
			}
			return nil
		},
	}
	cloudProvider     string
	cloudProviderList = []string{"aws", "hetzner", "azure", "gcp", "fc"}
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

// initState resolves the cloud provider and loads its config. Hetzner has
// built-in defaults (see setDefaults), so a missing .onctl is best-effort
// there; every other provider still requires its YAML config, so a missing
// or unreadable config stays a fatal error, matching the pre-flags behavior.
func initState() error {
	setDefaults()
	cloudProvider = checkCloudProvider()
	log.Println("[DEBUG] Cloud: " + cloudProvider)
	if err := ReadConfig(cloudProvider); err != nil {
		if cloudProvider != "hetzner" {
			return err
		}
		log.Println("[DEBUG] no config file loaded, using defaults:", err)
	}
	return nil
}

// ensureProvider builds the provider client if it hasn't been already.
// Cobra's shell-completion machinery (`__complete`) resolves
// ValidArgsFunction without running PersistentPreRunE
// (https://github.com/spf13/cobra/issues/1291), so completion functions that
// call provider.List() (destroy, ssh) must invoke this themselves, or they'd
// dereference a nil provider.
func ensureProvider() {
	if provider != nil {
		return
	}
	if err := initState(); err != nil {
		log.Println("[DEBUG] completion: " + err.Error())
		return
	}
	initProvider(cloudProvider)
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
	case "fc":
		fcConfig := providerfc.GetConfig()
		provider = &cloud.ProviderFC{
			Config:  fcConfig,
			Process: providerfc.NewProcessManager(fcConfig.BinPath),
			API:     providerfc.NewAPIClient(),
			Net:     providerfc.NewNetworkManager(),
			Rootfs:  providerfc.NewRootfsPreparer(),
		}
	}
}

func init() {
	rootCmd.PersistentFlags().StringVar(&providerFlag, "provider", "", "cloud provider: "+strings.Join(cloudProviderList, ", ")+" (overrides ONCTL_CLOUD)")
	rootCmd.AddCommand(versionCmd)
	rootCmd.AddCommand(initCmd)
}
