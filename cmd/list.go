package cmd

import (
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"os"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/cdalar/onctl/internal/tools/puppet"
	"github.com/cdalar/onctl/pkg/cloud"

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
		var pausedList cloud.VmList
		if output == "puppet" || output == "ansible" {
			serverList, err = provider.List()
			if err != nil {
				log.Println(err)
			}
			log.Println("[DEBUG] VM List: ", serverList)
		} else {
			serverList, pausedList, err = listVMs(provider)
			if err != nil {
				log.Println(err)
			}
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

// listVMs is what `onctl ls` and `onctl tui` show: the provider's VMs split
// into running and paused -- ListPaused's, plus any stopped VMs List
// returned, so all providers show the split, not just Hetzner. A failed
// call is reported through err; what the other call returned is still
// handed back.
func listVMs(p cloud.CloudProviderInterface) (running, paused cloud.VmList, err error) {
	all, listErr := p.List()
	log.Println("[DEBUG] VM List: ", all)
	snapshots, pausedErr := p.ListPaused()
	log.Println("[DEBUG] Paused List: ", snapshots)
	live := map[string]bool{}
	for _, vm := range all.List {
		live[vm.Name] = true
		if isPausedStatus(vm.Status) {
			paused.List = append(paused.List, vm)
		} else {
			running.List = append(running.List, vm)
		}
	}
	// Mid pause/resume the server and its snapshot coexist (Hetzner
	// snapshots before deleting, and creates before deleting the
	// snapshot); the server is the current record, so skip its snapshot.
	for _, vm := range snapshots.List {
		if !live[vm.Name] {
			paused.List = append(paused.List, vm)
		}
	}
	return running, paused, errors.Join(listErr, pausedErr)
}

// isPausedStatus reports whether a VM status means stopped/paused rather than
// running. AWS uses "stopped", GCP uses "TERMINATED" (stopped, not deleted),
// Azure uses "VM deallocated", OVH uses "SHELVED"/"SHELVED_OFFLOADED".
func isPausedStatus(status string) bool {
	return status == "stopped" ||
		status == "TERMINATED" ||
		status == "SHELVED" ||
		status == "SHELVED_OFFLOADED" ||
		strings.Contains(strings.ToLower(status), "deallocated")
}
