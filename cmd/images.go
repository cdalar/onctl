package cmd

import (
	"fmt"
	"log"

	"github.com/cdalar/onctl/internal/providerhtz"
	"github.com/cdalar/onctl/pkg/cloud"
	"github.com/spf13/cobra"
)

func init() {
	rootCmd.AddCommand(imagesCmd)
}

var imagesCmd = &cobra.Command{
	Use:   "images",
	Short: "List available OS images for the current cloud provider",
	Run: func(cmd *cobra.Command, args []string) {
		if cloudProvider != "hetzner" {
			fmt.Println("The current cloud provider does not support listing images.")
			return
		}
		htzProvider := cloud.ProviderHetzner{
			Client: providerhtz.GetClient(),
		}
		images, err := htzProvider.ListImages()
		if err != nil {
			log.Fatalln(err)
		}
		TabWriter(images, "NAME\tTYPE\tOS FLAVOR\tOS VERSION\tDESCRIPTION\n{{range .}}{{.Name}}\t{{.Type}}\t{{.OSFlavor}}\t{{.OSVersion}}\t{{.Description}}\n{{end}}")
	},
}
