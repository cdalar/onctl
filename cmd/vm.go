package cmd

import (
	"log"

	"github.com/cdalar/onctl/internal/cloud"
	"github.com/spf13/cobra"
)

var (
	vmOpts cloud.Vm
	nOpts  cloud.Network
)

func init() {
	vmCmd.AddCommand(vmNetworkAttachCmd)
	vmNetworkAttachCmd.Flags().StringVar(&vmOpts.Name, "vm", "", "name of vm")
	vmNetworkAttachCmd.Flags().StringVarP(&nOpt.Name, "network", "n", "", "Name for the network")
	// networkCmd.AddCommand(vmNetworkDeattachCmd)

}

var vmCmd = &cobra.Command{
	Use:     "vm",
	Aliases: []string{"server"},
	Short:   "Manage vm resources",
}

var vmNetworkAttachCmd = &cobra.Command{
	Use:   "attach",
	Short: "Attach a network",
	Run: func(cmd *cobra.Command, args []string) {
		// Do network creation
		log.Println("[DEBUG] Attaching network")
		vm, err := provider.GetByName(args[0])
		if err != nil {
			log.Println(err)
		}
		log.Println("[DEBUG] VM: ", vm)
		net, err := networkManager.GetByName(args[1])
		if err != nil {
			log.Println(err)
		}
		log.Println("[DEBUG] Network: ", net)
		provider.AttachNetwork(vm, net)
	},
}
