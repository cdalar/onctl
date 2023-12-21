package cmd

import (
	"fmt"
	"log"

	"github.com/spf13/cobra"
)

var (
	port string
)

func init() {
	createCmd.Flags().StringVarP(&port, "port", "p", "22", "ssh port")
}

var sshCmd = &cobra.Command{
	Use:                   "ssh VM_NAME",
	Short:                 "Spawn an SSH connection to a VM",
	Args:                  cobra.MinimumNArgs(1),
	TraverseChildren:      true,
	DisableFlagsInUseLine: true,
	Run: func(cmd *cobra.Command, args []string) {
		log.Println("[DEBUG] args: ", args)
		if len(args) == 0 {
			fmt.Println("Please provide a VM id")
			return
		}
		provider.SSHInto(args[0], port)
	},
}
