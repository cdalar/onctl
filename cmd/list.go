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

		var serverList cloud.VmList
		var err error
		serverList, err = provider.List()
		if err != nil {
			log.Println(err)
		}
		log.Println("[DEBUG] VM List: ", serverList)
		tmpl := "ID\tNAME\tLOCATION\tTYPE\tPUBLIC IP\tSTATE\tAGE\tCOST/H\tUSAGE\n{{range .List}}{{.ID}}\t{{.Name}}\t{{.Location}}\t{{.Type}}\t{{.IP}}\t{{.Status}}\t{{durationFromCreatedAt .CreatedAt}}\t{{.Cost.CostPerHour}}{{.Cost.Currency}}\t{{.Cost.AccumulatedCost}}{{.Cost.Currency}}\n{{end}}"
		TabWriter(serverList, tmpl)
	},
}
