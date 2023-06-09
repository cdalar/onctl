package cmd

import (
	"cdalar/onctl/internal/tools"
	"fmt"
	"log"
	"os"
	"strings"

	"github.com/spf13/cobra"
)

var (
	rootCmd = &cobra.Command{
		Use:   "onctl",
		Short: "onctl is a tool to manage cross platform resources in cloud",
	}
)

func checkCloudProvider() {
	var cloudProviderList = []string{"aws", "hetzner"}
	var cloudProvider = os.Getenv("CLOUD_PROVIDER")
	if cloudProvider != "" {
		if !tools.Contains(cloudProviderList, cloudProvider) {
			fmt.Println("Cloud Platform (" + cloudProvider + ") is not Supported\nPlease use one of the following: " + strings.Join(cloudProviderList, ","))
			os.Exit(1)
		}
	} else {
		provider := tools.WhichCloudProvider()
		if provider != "none" {
			fmt.Println("Using: " + provider)
			err := os.Setenv("CLOUD_PROVIDER", provider)
			if err != nil {
				log.Println(err)
			}
			return
		} else {
			fmt.Println("No Cloud Provider Set.\nPlease set the CLOUD_PROVIDER environment variable to one of the following: " + strings.Join(cloudProviderList, ","))
			os.Exit(1)
		}
	}
}

// Execute executes the root command.
func Execute() error {
	log.Println("[DEBUG] Args: " + strings.Join(os.Args, ","))
	if len(os.Args) > 1 && os.Args[1] != "version" {
		checkCloudProvider()
	}
	return rootCmd.Execute()
}

func init() {
	rootCmd.AddCommand(versionCmd)
	rootCmd.AddCommand(listCmd)
	rootCmd.AddCommand(createCmd)
	rootCmd.AddCommand(destroyCmd)
	rootCmd.AddCommand(sshCmd)
}
