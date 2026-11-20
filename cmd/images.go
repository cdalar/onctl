package cmd

import (
	"fmt"
	"log"
	"strings"

	"github.com/cdalar/onctl/internal/providerfc"
	"github.com/cdalar/onctl/internal/providerhtz"
	"github.com/cdalar/onctl/pkg/cloud"
	"github.com/spf13/cobra"
)

// hetznerImageLister returns the ImageLister to use. Overridable in tests.
var hetznerImageLister = func() (cloud.ImageLister, error) {
	client, err := providerhtz.GetClient()
	if err != nil {
		return nil, err
	}
	return cloud.ProviderHetzner{Client: client}, nil
}

// fcImageLister lists the images published for the fc provider.
// Overridable in tests.
var fcImageLister = func() cloud.ImageLister {
	return providerfc.CatalogLister{BaseURL: providerfc.PublicCatalogURL}
}

func init() {
	rootCmd.AddCommand(imagesCmd)
}

var imagesCmd = &cobra.Command{
	Use:   "images",
	Short: "List available OS images for the current cloud provider",
	Run: func(cmd *cobra.Command, args []string) {
		var lister cloud.ImageLister
		var err error
		switch cloudProvider {
		case "hetzner":
			lister, err = hetznerImageLister()
		case "fc":
			lister = fcImageLister()
		case "boxes":
			ensureProvider()
			lister, err = provider.(cloud.ImageLister), nil
			if err == nil && lister == nil {
				// ensureProvider might not set provider if it's not a lister
				err = fmt.Errorf("provider does not support listing images")
			}
		}
		if err != nil {
			log.Fatalln(err)
		}
		if lister == nil {
			fmt.Println("The current cloud provider does not support listing images.")
			return
		}
		images, err := lister.ListImages()
		if err != nil {
			log.Fatalln(err)
		}
		TabWriter(images, imagesTemplate(images))
	},
}

// imagesTemplate is the images table, without the columns no image fills
// in: providers know different things about an image (boxes: a name and
// a description; Hetzner: its OS flavor and version too; fc: a size and
// the URL to download it from).
func imagesTemplate(images []cloud.CloudImage) string {
	cols := []struct {
		header, field string
		has           func(cloud.CloudImage) bool
	}{
		{"NAME", "Name", func(i cloud.CloudImage) bool { return true }},
		{"TYPE", "Type", func(i cloud.CloudImage) bool { return i.Type != "" }},
		{"OS FLAVOR", "OSFlavor", func(i cloud.CloudImage) bool { return i.OSFlavor != "" }},
		{"OS VERSION", "OSVersion", func(i cloud.CloudImage) bool { return i.OSVersion != "" }},
		{"SIZE", "Size", func(i cloud.CloudImage) bool { return i.Size != "" }},
		{"DESCRIPTION", "Description", func(i cloud.CloudImage) bool { return i.Description != "" }},
		{"URL", "URL", func(i cloud.CloudImage) bool { return i.URL != "" }},
	}
	var headers, fields []string
	for _, c := range cols {
		for _, img := range images {
			if c.has(img) {
				headers = append(headers, c.header)
				fields = append(fields, "{{."+c.field+"}}")
				break
			}
		}
	}
	return strings.Join(headers, "\t") + "\n{{range .}}" + strings.Join(fields, "\t") + "\n{{end}}"
}
