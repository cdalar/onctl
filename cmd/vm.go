package cmd

import (
	"github.com/spf13/cobra"
)

var vmCmd = &cobra.Command{
	Use:   "vm",
	Short: "Manage virtual machines",
	Long:  `Manage virtual machines across different cloud providers`,
}

func init() {
	vmCmd.AddCommand(listCmd)
	vmCmd.AddCommand(createCmd)
	vmCmd.AddCommand(destroyCmd)
	vmCmd.AddCommand(sshCmd)
	rootCmd.AddCommand(vmCmd)
}
