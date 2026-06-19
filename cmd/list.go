package cmd

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/cdalar/onctl/internal/tools"
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

// namedProvider pairs a provider client with the name it was built for, so
// results can be tagged and per-provider settings (e.g. vm.username) looked
// up correctly when aggregating across providers.
type namedProvider struct {
	name     string
	provider cloud.CloudProviderInterface
}

// resolveProviderNames decides which provider name(s) `ls` queries. An
// explicit --provider/ONCTL_CLOUD always means exactly one, matching every
// other command. Otherwise, if more than one provider's credentials are
// detected, `ls` aggregates across all of them; with zero or one detected,
// it falls back to the already-resolved single provider (unchanged
// behavior). Kept separate from resolveListProviders so the decision can be
// tested without constructing real provider clients (which, for gcp/azure,
// can log.Fatal without credentials).
func resolveProviderNames() []string {
	if !providerExplicitlyChosen {
		if names := tools.DetectCloudProviders(); len(names) > 1 {
			return names
		}
	}
	return []string{cloudProvider}
}

// resolveListProviders builds a provider client for each name returned by
// resolveProviderNames, reusing the already-built global provider rather
// than constructing a second client when there's only one.
func resolveListProviders() []namedProvider {
	names := resolveProviderNames()
	log.Printf("[DEBUG] ls multi names=%v explicit=%v primary=%s", names, providerExplicitlyChosen, cloudProvider)
	out := make([]namedProvider, 0, len(names))
	for _, n := range names {
		if len(names) == 1 && n == cloudProvider {
			out = append(out, namedProvider{n, provider})
			continue
		}
		// Best-effort load this provider's config into viper (hetzner/aws/fc
		// may not have a yaml; azure/gcp may need subscription/project etc).
		_ = ReadConfig(n)
		p := buildProvider(n)
		if p == nil {
			log.Printf("[DEBUG] skipping provider %s: no client", n)
			continue
		}
		out = append(out, namedProvider{n, p})
	}
	return out
}

var listCmd = &cobra.Command{
	Use:     "ls",
	Aliases: []string{"list"},
	Short:   "List VMs",
	Run: func(cmd *cobra.Command, args []string) {

		var (
			tmpl       string
			serverList cloud.VmList
		)
		providers := resolveListProviders()
		for _, np := range providers {
			l, err := np.provider.List()
			if err != nil {
				log.Printf("[DEBUG] ls provider %s: %v", np.name, err)
			}
			serverList.List = append(serverList.List, l.List...)
		}
		log.Println("[DEBUG] VM List: ", serverList)

		var pausedList cloud.VmList
		if output != "puppet" && output != "ansible" {
			for _, np := range providers {
				pl, err := np.provider.ListPaused()
				if err != nil {
					log.Printf("[DEBUG] ls provider %s ListPaused: %v", np.name, err)
				}
				pausedList.List = append(pausedList.List, pl.List...)
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
			if err := encoder.Encode(puppetInventory); err != nil {
				log.Println(err)
			}
		case "ansible":
			log.Println("[DEBUG] Ansible output")
			fmt.Println("[onctl]")
			for _, server := range serverList.List {
				username := viper.GetString(server.Provider + ".vm.username")
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
			// The cost column is Hetzner-specific, so it only applies when
			// ls resolved to exactly that one provider, not when aggregating.
			if len(providers) == 1 && providers[0].name == "hetzner" {
				tmpl = "CLOUD\tID\tNAME\tLOCATION\tTYPE\tPUBLIC IP\tPRIVATE IP\tSTATE\tAGE\tCOST/H\tUSAGE\n{{range .List}}{{.Provider}}\t{{.ID}}\t{{.Name}}\t{{.Location}}\t{{.Type}}\t{{.IP}}\t{{.PrivateIP}}\t{{.Status}}\t{{durationFromCreatedAt .CreatedAt}}\t{{.Cost.CostPerHour}}{{.Cost.Currency}}\t{{.Cost.AccumulatedCost}}{{.Cost.Currency}}\n{{end}}"
			} else {
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
