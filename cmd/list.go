package cmd

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/cdalar/onctl/internal/cloud"
	"github.com/cdalar/onctl/internal/tools/puppet"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var output string

func init() {
	listCmd.Flags().StringVarP(&output, "output", "o", "tab", "output format (tab, json, yaml, puppet, ansible)")
	// Register list command at root level for convenience
	rootCmd.AddCommand(listCmd)
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

		var pausedList cloud.VmList
		if output != "puppet" && output != "ansible" {
			pausedList, err = provider.ListPaused()
			if err != nil {
				log.Println(err)
			}
			log.Println("[DEBUG] Paused List: ", pausedList)

			// Partition any stopped VMs out of serverList into pausedList so all
			// providers show the split running/paused view, not just Hetzner.
			var running cloud.VmList
			for _, vm := range serverList.List {
				if isPausedStatus(vm.Status) {
					pausedList.List = append(pausedList.List, vm)
				} else {
					running.List = append(running.List, vm)
				}
			}
			serverList = running
		}

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
			log.Println("[DEBUG] Ansible output")
			username := viper.GetString(cloudProvider + ".vm.username")
			if err != nil {
				log.Println(err)
			}
			fmt.Println("[onctl]")
			for _, server := range serverList.List {
				fmt.Println(server.IP, "ansible_user="+username)
			}
		case "json":
			// Flat array of running + paused (each paused row carries Status "paused").
			combined := append(append([]cloud.Vm{}, serverList.List...), pausedList.List...)
			jsonList, err := json.Marshal(combined)
			if err != nil {
				log.Println(err)
			}
			fmt.Println(string(jsonList))
		case "yaml":
			combined := append(append([]cloud.Vm{}, serverList.List...), pausedList.List...)
			yamlList, err := yaml.Marshal(combined)
			if err != nil {
				log.Println(err)
			}
			fmt.Println(string(yamlList))
		default:
			noCostTmpl := "CLOUD\tID\tNAME\tLOCATION\tTYPE\tPUBLIC IP\tPRIVATE IP\tSTATE\tAGE\n{{range .List}}{{.Provider}}\t{{.ID}}\t{{.Name}}\t{{.Location}}\t{{.Type}}\t{{.IP}}\t{{.PrivateIP}}\t{{.Status}}\t{{durationFromCreatedAt .CreatedAt}}\n{{end}}"
			switch cloudProvider {
			case "hetzner":
				tmpl = "CLOUD\tID\tNAME\tLOCATION\tTYPE\tPUBLIC IP\tPRIVATE IP\tSTATE\tAGE\tCOST/H\tUSAGE\n{{range .List}}{{.Provider}}\t{{.ID}}\t{{.Name}}\t{{.Location}}\t{{.Type}}\t{{.IP}}\t{{.PrivateIP}}\t{{.Status}}\t{{durationFromCreatedAt .CreatedAt}}\t{{.Cost.CostPerHour}}{{.Cost.Currency}}\t{{.Cost.AccumulatedCost}}{{.Cost.Currency}}\n{{end}}"
			default:
				tmpl = noCostTmpl
			}
			// When there are paused servers, label both groups so they read as
			// distinct sections; otherwise keep the plain single-table output.
			if len(pausedList.List) > 0 {
				fmt.Printf("\033[1;32m● RUNNING (%d)\033[0m\n", len(serverList.List))
			}
			TabWriter(serverList, tmpl)

			if len(pausedList.List) > 0 {
				fmt.Printf("\n\033[1;33m● PAUSED (%d)\033[0m\n", len(pausedList.List))
				TabWriter(pausedList, noCostTmpl)
			}
		}

	},
}

// isPausedStatus reports whether a VM status means stopped/paused rather than
// running. AWS uses "stopped", GCP uses "TERMINATED" (stopped, not deleted),
// Azure uses "VM deallocated".
func isPausedStatus(status string) bool {
	return status == "stopped" ||
		status == "TERMINATED" ||
		strings.Contains(strings.ToLower(status), "deallocated")
}
