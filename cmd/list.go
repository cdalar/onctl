package cmd

import (
	"fmt"
	"log"
	"onkube/onctl/internal/cloud"
	"onkube/onctl/internal/provideraws"
	"onkube/onctl/internal/providerhtz"
	"os"

	"github.com/spf13/cobra"
)

var listCmd = &cobra.Command{
	Use:     "ls",
	Aliases: []string{"list"},
	Short:   "List VMs",
	Run: func(cmd *cobra.Command, args []string) {

		var serverList cloud.VmList
		var err error
		log.Println("[DEBUG] Cloud Provider: ", os.Getenv("CLOUD_PROVIDER"))
		switch os.Getenv("CLOUD_PROVIDER") {
		case "hetzner":
			provider = &cloud.ProviderHetzner{
				Client: providerhtz.GetClient(),
			}
		case "aws":
			provider = &cloud.ProviderAws{
				Client: provideraws.GetClient(),
			}
		}
		serverList, err = provider.List()
		log.Println("[DEBUG] VM List: ", serverList)
		if err != nil {
			log.Println(err)
		}
		tmpl := "ID\tNAME\tTYPE\tPUBLIC IP\tSTATE\tAGE\n{{range .List}}{{.ID}}\t{{.Name}}\t{{.Type}}\t{{.IP}}\t{{.Status}}\t{{durationFromCreatedAt .CreatedAt}}\n{{end}}"
		if len(serverList.List) != 0 {
			TabWriter(serverList, tmpl)
		} else {
			fmt.Println("No VMs Found")
		}
	},
}
