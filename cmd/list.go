package cmd

import (
	"log"

	"github.com/cdalar/onctl/internal/cloud"

	"github.com/spf13/cobra"
)

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
		switch cloudProvider {
		case "hetzner":
			tmpl = "ID\tNAME\tLOCATION\tTYPE\tPUBLIC IP\tSTATE\tAGE\tCOST/H\tUSAGE\n{{range .List}}{{.ID}}\t{{.Name}}\t{{.Location}}\t{{.Type}}\t{{.IP}}\t{{.Status}}\t{{durationFromCreatedAt .CreatedAt}}\t{{.Cost.CostPerHour}}{{.Cost.Currency}}\t{{.Cost.AccumulatedCost}}{{.Cost.Currency}}\n{{end}}"
		default:
			tmpl = "ID\tNAME\tLOCATION\tTYPE\tPUBLIC IP\tSTATE\tAGE\n{{range .List}}{{.ID}}\t{{.Name}}\t{{.Location}}\t{{.Type}}\t{{.IP}}\t{{.Status}}\t{{durationFromCreatedAt .CreatedAt}}\n{{end}}"
		}
		TabWriter(serverList, tmpl)
	},
}
