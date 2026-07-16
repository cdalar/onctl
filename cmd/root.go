package cmd

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
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
			case "init", "version", "help", "import", "__complete", "__completeNoDesc":
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
	cloudProviderList = []string{"aws", "hetzner", "azure", "gcp", "fc", "static"}
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

// initState resolves the cloud provider and loads its config. The onctl.yaml
// written by `onctl init` is the source of truth (see internal/files/init/onctl.yaml).
// A missing file is fatal. For gcp (and later azure) we additionally resolve
// account-specific placeholders using the cloud CLIs after loading.
func initState() error {
	cloudProvider = checkCloudProvider()
	log.Println("[DEBUG] Cloud: " + cloudProvider)
	if err := ReadConfig(); err != nil {
		return err
	}
	if cloudProvider == "gcp" {
		if err := resolveGCPProject(); err != nil {
			return err
		}
	}
	if cloudProvider == "azure" {
		if err := resolveAzureIdentifiers(); err != nil {
			return err
		}
	}
	return nil
}

// resolveGCPProject fills gcp.project from the gcloud CLI's active project
// when the value loaded from onctl.yaml is still the placeholder
// ("<project-id>") or empty. This keeps the single onctl.yaml usable out of
// the box for users who have gcloud configured. If still unset it returns a
// clear actionable error.
func resolveGCPProject() error {
	proj := viper.GetString("gcp.project")
	if proj != "" && proj != "<project-id>" {
		return nil
	}
	if project := providergcp.GCloudDefaultProject(); project != "" {
		viper.Set("gcp.project", project)
		log.Printf("[DEBUG] resolved gcp.project from gcloud: %s", project)
		return nil
	}
	return fmt.Errorf(`gcp.project is required: set --project, edit .onctl/onctl.yaml, or run "gcloud config set project <id>"`)
}

// resolveAzureIdentifiers fills azure.subscriptionId and azure.resourceGroup
// from az CLI when the onctl.yaml still has the placeholders. Both are
// required to talk to Azure; the resource group one may legitimately stay
// empty for users who always pass it or set it in az defaults.
func resolveAzureIdentifiers() error {
	sub := viper.GetString("azure.subscriptionId")
	if sub == "" || sub == "<subscription-id>" {
		if id := providerazure.AzureCLISubscriptionID(); id != "" {
			viper.Set("azure.subscriptionId", id)
			log.Printf("[DEBUG] resolved azure.subscriptionId from az: %s", id)
		} else {
			return fmt.Errorf(`azure.subscriptionId is required: set --subscription-id, edit .onctl/onctl.yaml, or run "az login"`)
		}
	}

	rg := viper.GetString("azure.resourceGroup")
	if rg == "" {
		// Only fill from az default if no value was provided in onctl.yaml
		// (respect an explicit "test" or other name the user may have set).
		if group := providerazure.AzureCLIDefaultResourceGroup(); group != "" {
			viper.Set("azure.resourceGroup", group)
			log.Printf("[DEBUG] resolved azure.resourceGroup from az: %s", group)
		}
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
			Cache:   providerfc.NewCacheDiskPreparer(),
		}
	case "static":
		configDir, err := resolveConfigDir()
		if err != nil {
			log.Fatalln(err)
		}
		provider = &cloud.ProviderStatic{InventoryPath: filepath.Join(configDir, importedHostsFile)}
	}
}

func init() {
	rootCmd.PersistentFlags().StringVarP(&providerFlag, "provider", "p", "", "cloud provider: "+strings.Join(cloudProviderList, ", ")+" (overrides ONCTL_CLOUD)")
	// GCP project and Azure account-specific settings are needed for many
	// commands (ls, ssh, destroy...), not just create. Register as persistent
	// so resolve runs and flags are available everywhere.
	rootCmd.PersistentFlags().StringVar(&flagGCPProject, "project", "", "GCP: project ID (falls back to `gcloud config get-value project` when the onctl.yaml placeholder is present)")
	rootCmd.PersistentFlags().StringVar(&flagAzureSubscriptionID, "subscription-id", "", "Azure: subscription ID (required for the azure provider; falls back to `az account show`)")
	rootCmd.PersistentFlags().StringVar(&flagAzureResourceGroup, "resource-group", "", "Azure: resource group (required for the azure provider; falls back to the az CLI's configured default group, if any)")

	// Bind the account-specific global flags early (persistent on root) so
	// viper sees the CLI values in initState/resolve for all commands.
	_ = viper.BindPFlag("gcp.project", rootCmd.PersistentFlags().Lookup("project"))
	_ = viper.BindPFlag("azure.subscriptionId", rootCmd.PersistentFlags().Lookup("subscription-id"))
	_ = viper.BindPFlag("azure.resourceGroup", rootCmd.PersistentFlags().Lookup("resource-group"))

	rootCmd.AddCommand(versionCmd)
	rootCmd.AddCommand(initCmd)
}
