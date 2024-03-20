package cmd

import (
	"encoding/json"
	"fmt"
	"log"
	"os"

	"gopkg.in/yaml.v3"

	"github.com/cdalar/onctl/internal/cloud"
	"github.com/cdalar/onctl/internal/tools/puppet"

	"github.com/spf13/cobra"
)

var output string

func init() {
	listCmd.Flags().StringVarP(&output, "output", "o", "tab", "output format (tab, json, yaml, puppet, ansiable)")
}

var listCmd = &cobra.Command{
	Use:     "ls",
	Aliases: []string{"list"},
	Short:   "List VMs",
	Run: func(cmd *cobra.Command, args []string) {

		var (
			tmpl       string
			serverList cloud.VmList
			err        error
		)
		serverList, err = provider.List()
		if err != nil {
			log.Println(err)
		}
		log.Println("[DEBUG] VM List: ", serverList)

		switch output {
		case "puppet":
			var puppetInventory puppet.Inventory
			puppetInventory.Groups = make([]puppet.Group, 1)
			puppetInventory.Groups[0].Name = "servers"
			puppetInventory.Config.SSH = puppet.SSH{
				User:         "root",
				HostKeyCheck: false,
				NativeSSH:    true,
				SSHCommand:   "ssh",
			}
			puppetInventory.Config.Transport = "ssh"
			_, privateKey := getSSHKeyFilePaths("")
			puppetInventory.Config.SSH.PrivateKey = privateKey
			for _, server := range serverList.List {
				puppetInventory.Groups[0].Targets = append(puppetInventory.Groups[0].Targets, server.IP)
			}

			encoder := yaml.NewEncoder(os.Stdout)
			encoder.SetIndent(2) // Set YAML indentation
			err = encoder.Encode(puppetInventory)
			if err != nil {
				log.Println(err)
			}
		case "ansible":
			for _, server := range serverList.List {
				fmt.Println(server.IP)
			}
		case "json":
			jsonList, err := json.Marshal(serverList.List)
			if err != nil {
				log.Println(err)
			}
			fmt.Println(string(jsonList))
		case "yaml":
			yamlList, err := yaml.Marshal(serverList.List)
			if err != nil {
				log.Println(err)
			}
			fmt.Println(string(yamlList))
		default:
			switch cloudProvider {
			case "hetzner":
				tmpl = "CLOUD\tID\tNAME\tLOCATION\tTYPE\tPUBLIC IP\tPRIVATE IP\tSTATE\tAGE\tCOST/H\tUSAGE\n{{range .List}}{{.Provider}}\t{{.ID}}\t{{.Name}}\t{{.Location}}\t{{.Type}}\t{{.IP}}\t{{.PrivateIP}}\t{{.Status}}\t{{durationFromCreatedAt .CreatedAt}}\t{{.Cost.CostPerHour}}{{.Cost.Currency}}\t{{.Cost.AccumulatedCost}}{{.Cost.Currency}}\n{{end}}"
			default:
				tmpl = "CLOUD\tID\tNAME\tLOCATION\tTYPE\tPUBLIC IP\tPRIVATE IP\tSTATE\tAGE\n{{range .List}}{{.Provider}}\t{{.ID}}\t{{.Name}}\t{{.Location}}\t{{.Type}}\t{{.IP}}\t{{.PrivateIP}}\t{{.Status}}\t{{durationFromCreatedAt .CreatedAt}}\n{{end}}"
			}
			TabWriter(serverList, tmpl)
		}

	},
}
