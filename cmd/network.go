package cmd

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"sync"
	"time"

	"github.com/briandowns/spinner"
	"github.com/cdalar/onctl/internal/cloud"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
)

var (
	nOpt cloud.Network
)

func init() {
	networkCmd.AddCommand(networkCreateCmd)
	networkCmd.AddCommand(networkListCmd)
	networkCmd.AddCommand(networkDeleteCmd)
	networkCreateCmd.Flags().StringVar(&nOpt.CIDR, "cidr", "", "CIDR for the network ex. 10.0.0.0/16 ")
	networkCreateCmd.Flags().StringVarP(&nOpt.Name, "name", "n", "", "Name for the network")
}

var networkCmd = &cobra.Command{
	Use:     "network",
	Aliases: []string{"net"},
	Short:   "Manage network resources",
	Long:    `Manage network resources`,
}

var networkCreateCmd = &cobra.Command{
	Use:     "create",
	Aliases: []string{"new", "add", "up"},
	Short:   "Create a network",
	Long:    `Create a network`,
	Run: func(cmd *cobra.Command, args []string) {
		// Do network creation
		log.Println("[DEBUG] Creating network")
		_, err := networkManager.Create(cloud.Network{
			Name: nOpt.Name,
			CIDR: nOpt.CIDR,
		})
		if err != nil {
			log.Println(err)
		}
	},
}

var networkListCmd = &cobra.Command{
	Use:     "list",
	Aliases: []string{"ls"},
	Short:   "List networks",
	Long:    `List networks`,
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
	Use:     "delete",
	Aliases: []string{"rm", "remove", "destroy", "down", "del"},
	Short:   "Delete a network",
	Long:    `Delete a network`,
	ValidArgsFunction: func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
		netList, err := networkManager.List()
		list := []string{}
		for _, net := range netList {
			list = append(list, net.Name)
		}

		if err != nil {
			return nil, cobra.ShellCompDirectiveError
		}

		return list, cobra.ShellCompDirectiveNoFileComp
	},
	Run: func(cmd *cobra.Command, args []string) {
		// Do network deletion
		// log.Println("Deleting network")
		s := spinner.New(spinner.CharSets[9], 100*time.Millisecond) // Build our new spinner
		log.Println("[DEBUG] args: ", args)
		if len(args) == 0 {
			fmt.Println("Please provide a network id")
			return
		}
		switch args[0] {
		case "all":
			// Delete all networks
			if !force {
				if !yesNo() {
					os.Exit(0)
				}
			}
			log.Println("[DEBUG] Delete All Networks")
			networks, err := networkManager.List()
			if err != nil {
				log.Println(err)
			}
			log.Println("[DEBUG] Networks: ", networks)
			var wg sync.WaitGroup
			for _, network := range networks {
				wg.Add(1)
				go func(network cloud.Network) {
					defer wg.Done()
					s.Start()
					s.Suffix = " Destroying VM..."
					if err := networkManager.Delete(network); err != nil {
						fmt.Println("\033[31m\u2718\033[0m Could not delete Network: " + network.Name)
						log.Println(err)
					}
					s.Stop()
					fmt.Println("\033[32m\u2714\033[0m Network Deleted: " + network.Name)
				}(network)
			}
			wg.Wait()
			fmt.Println("\033[32m\u2714\033[0m ALL Network(s) are destroyed")
		default:
			// Tear down specific server
			networkName := args[0]
			netlist, err := networkManager.List()
			if err != nil {
				log.Println(err)
			}
			for _, network := range netlist {
				if network.Name == networkName {
					log.Println("[DEBUG] Delete network: " + networkName)
					s.Start()
					s.Suffix = " Destroying Network..."
					err := networkManager.Delete(cloud.Network{
						ID: network.ID,
					})
					if err != nil {
						s.Stop()
						fmt.Println("\033[31m\u2718\033[0m Cannot destroy Network: " + networkName)
						fmt.Println(err)
						os.Exit(1)
					}
					s.Stop()
					fmt.Println("\033[32m\u2714\033[0m Network Destroyed: " + networkName)
				}
			}
		}
	},
}
