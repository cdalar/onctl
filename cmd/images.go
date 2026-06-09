package cmd

import (
	"fmt"
	"log"

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
		lister, ok := provider.(cloud.ImageLister)
		if !ok {
			fmt.Println("The current cloud provider does not support listing images.")
			return
		}
		images, err := lister.ListImages()
		if err != nil {
			log.Fatalln(err)
		}
		TabWriter(images, "NAME\tTYPE\tOS FLAVOR\tOS VERSION\tDESCRIPTION\n{{range .}}{{.Name}}\t{{.Type}}\t{{.OSFlavor}}\t{{.OSVersion}}\t{{.Description}}\n{{end}}")
	},
}
