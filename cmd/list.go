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
		log.Println("[DEBUG] VM List: ", serverList)
		if err != nil {
			log.Println(err)
		}
		tmpl := "ID\tNAME\tTYPE\tPUBLIC IP\tSTATE\tAGE\n{{range .List}}{{.ID}}\t{{.Name}}\t{{.Type}}\t{{.IP}}\t{{.Status}}\t{{durationFromCreatedAt .CreatedAt}}\n{{end}}"
		TabWriter(serverList, tmpl)
	},
}
