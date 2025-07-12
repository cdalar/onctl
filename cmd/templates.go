package cmd

import (
	"fmt"
	"io"
	"log"
	"net/http"
	"os"

	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
)

type TemplateIndex struct {
	Templates []Template `yaml:"templates"`
}

type Template struct {
	Name        string `yaml:"name"`
	Description string `yaml:"description"`
	Config      string `yaml:"config"`
	Type        string `yaml:"type"`
}

var (
	indexFile string
)

func init() {
	rootCmd.AddCommand(templatesCmd)
	templatesCmd.AddCommand(templatesListCmd)
	templatesListCmd.Flags().StringVarP(&indexFile, "file", "f", "", "local index.yaml file path")
}

var templatesCmd = &cobra.Command{
	Use:     "templates",
	Aliases: []string{"tmpl"},
	Short:   "Manage onctl templates",
	Long:    `List and manage onctl templates from templates.onctl.com`,
}

var templatesListCmd = &cobra.Command{
	Use:     "list",
	Aliases: []string{"ls"},
	Short:   "List available templates",
	Run: func(cmd *cobra.Command, args []string) {
		var body []byte
		var err error

		if indexFile != "" {
			// Read from local file
			body, err = os.ReadFile(indexFile)
			if err != nil {
				fmt.Printf("Error reading local file %s: %v\n", indexFile, err)
				os.Exit(1)
			}
		} else {
			// Fetch from remote
			resp, err := http.Get("https://templates.onctl.com/index.yaml")
			if err != nil {
				fmt.Println("Error fetching templates:", err)
				os.Exit(1)
			}
			defer func() {
				if err := resp.Body.Close(); err != nil {
					log.Printf("Failed to close response body: %v", err)
				}
			}()

			// Check for 404 status code
			if resp.StatusCode == http.StatusNotFound {
				fmt.Println("Error: Template index file not found (404)")
				fmt.Println("The remote file https://templates.onctl.com/index.yaml does not exist")
				os.Exit(1)
			}

			// Check for other non-200 status codes
			if resp.StatusCode != http.StatusOK {
				fmt.Printf("Error: Unexpected status code %d when fetching templates\n", resp.StatusCode)
				os.Exit(1)
			}

			body, err = io.ReadAll(resp.Body)
			if err != nil {
				fmt.Println("Error reading response:", err)
				os.Exit(1)
			}
		}

		// Parse the YAML
		var index TemplateIndex
		err = yaml.Unmarshal(body, &index)
		if err != nil {
			fmt.Println("Error parsing YAML:", err)
			os.Exit(1)
		}

		// Create template for tabwriter
		tmpl := "NAME\tTYPE\tCONFIG\tDESCRIPTION\n{{range .Templates}}{{.Name}}\t{{.Type}}\t{{.Config}}\t{{.Description}}\n{{end}}"

		log.Println("[DEBUG] Templates:", index)
		TabWriter(index, tmpl)
	},
}
