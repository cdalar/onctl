package cmd

import (
	"encoding/json"
	"fmt"
	"log"

	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
)

func init() {
	networkCmd.AddCommand(networkCreateCmd)
	networkCmd.AddCommand(networkListCmd)
	networkCmd.AddCommand(networkDeleteCmd)
}

var networkCmd = &cobra.Command{
	Use:   "network",
	Short: "Manage network resources",
	Long:  `Manage network resources`,
}

var networkCreateCmd = &cobra.Command{
	Use:   "create",
	Short: "Create a network",
	Long:  `Create a network`,
	Run: func(cmd *cobra.Command, args []string) {
		// Do network creation
		log.Println("[DEBUG] Creating network")
	},
}
var networkListCmd = &cobra.Command{
	Use:   "list",
	Short: "List networks",
	Long:  `List networks`,
	Run: func(cmd *cobra.Command, args []string) {
		// Do network listing
		log.Println("[DEBUG] Listing networks")
		netlist, err := networkManager.List()
		if err != nil {
			log.Println(err)
		}
		switch output {
		case "json":
			jsonList, err := json.Marshal(netlist)
			if err != nil {
				log.Println(err)
			}
			fmt.Println(string(jsonList))
		case "yaml":
			yamlList, err := yaml.Marshal(netlist)
			if err != nil {
				log.Println(err)
			}
			fmt.Println(string(yamlList))
		default:
			tmpl := "CLOUD\tID\tNAME\tCIDR\tSERVERS\tAGE\n{{range .}}{{.Provider}}\t{{.ID}}\t{{.Name}}\t{{.CIDR}}\t{{.Servers}}\t{{durationFromCreatedAt .CreatedAt}}\n{{end}}"
			TabWriter(netlist, tmpl)
		}

	},
}
var networkDeleteCmd = &cobra.Command{
	Use:   "delete",
	Short: "Delete a network",
	Long:  `Delete a network`,
	Run: func(cmd *cobra.Command, args []string) {
		// Do network deletion
		log.Println("Deleting network")
	},
}
