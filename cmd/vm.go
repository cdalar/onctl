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
	vmNetworkAttachCmd.Flags().StringVarP(&nOpts.Name, "network", "n", "", "Name for the network")
	vmCmd.AddCommand(vmNetworkDetachCmd)
	vmNetworkDetachCmd.Flags().StringVar(&vmOpts.Name, "vm", "", "name of vm")
	vmNetworkDetachCmd.Flags().StringVarP(&nOpts.Name, "network", "n", "", "Name for the network")
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
		vm, err := provider.GetByName(vmOpts.Name)
		if err != nil {
			log.Println(err)
		}
		log.Println("[DEBUG] VM: ", vm)
		net, err := networkManager.GetByName(nOpts.Name)
		if err != nil {
			log.Println(err)
		}
		log.Println("[DEBUG] Network: ", net)
		err = provider.AttachNetwork(vm, net)
		if err != nil {
			log.Println(err)
		}
	},
}

var vmNetworkDetachCmd = &cobra.Command{
	Use:   "detach",
	Short: "Detach a network",
	Run: func(cmd *cobra.Command, args []string) {
		// Do network creation
		log.Println("[DEBUG] Detaching network")
		vm, err := provider.GetByName(vmOpts.Name)
		if err != nil {
			log.Println(err)
		}
		log.Println("[DEBUG] VM: ", vm)
		net, err := networkManager.GetByName(nOpts.Name)
		if err != nil {
			log.Println(err)
		}
		log.Println("[DEBUG] Network: ", net)
		err = provider.DetachNetwork(vm, net)
		if err != nil {
			log.Println(err)
		}
	},
}
