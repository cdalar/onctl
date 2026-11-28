package cmd

import (
	"errors"
	"fmt"
	"log"
	"strings"

	"github.com/cdalar/onctl/pkg/box"
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
		ensureProvider()
		if provider == nil {
			log.Fatalln("unknown provider " + cloudProvider)
		}
		images, err := boxClient().Images()
		if errors.Is(err, box.ErrUnsupported) {
			fmt.Println("The current cloud provider does not support listing images.")
			return
		}
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
