package cmd

import (
	"fmt"
	"log"
	"os"

	"github.com/spf13/cobra"
)

var resumePublicKeyFile string

func init() {
	resumeCmd.Flags().StringVarP(&resumePublicKeyFile, "publicKey", "k", "", "Path to publicKey file (default: ~/.ssh/id_rsa)")
	rootCmd.AddCommand(resumeCmd)
}

var resumeCmd = &cobra.Command{
	Use:   "resume <name>",
	Short: "Recreate a paused VM from its snapshot",
	Long: `Resume recreates a VM previously paused with 'onctl pause' from its
snapshot and re-attaches the preserved public IP when available.`,
	Run: func(cmd *cobra.Command, args []string) {
		if len(args) == 0 {
			fmt.Println("Please provide a VM name to resume")
			return
		}
		serverName := args[0]

		// Ensure the onctl SSH key exists and attach it (mirrors create).
		publicKeyFile, _ := getSSHKeyFilePaths(resumePublicKeyFile)
		c := boxClient()
		keyID, err := c.AddSSHKey(publicKeyFile)
		if err != nil {
			log.Fatalln(err)
		}

		fmt.Println("\033[32m✔\033[0m Resuming VM from snapshot...")
		vm, err := c.Resume(serverName, keyID)
		if err != nil {
			fmt.Println("\033[31m✘\033[0m Could not resume VM: " + serverName)
			fmt.Println(err)
			os.Exit(1)
		}
		fmt.Println("\033[32m✔\033[0m VM Resumed: " + serverName + " (IP: " + vm.IP + ")")
	},
}
