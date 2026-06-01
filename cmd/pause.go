package cmd

import (
	"fmt"
	"os"

	"github.com/cdalar/onctl/internal/cloud"

	"github.com/spf13/cobra"
)

var (
	pauseForce bool
	pauseHot   bool
)

func init() {
	pauseCmd.Flags().BoolVarP(&pauseForce, "force", "f", false, "pause without confirmation")
	pauseCmd.Flags().BoolVar(&pauseHot, "hot", false, "snapshot without shutting down first (faster, crash-consistent)")
	rootCmd.AddCommand(pauseCmd)
	vmCmd.AddCommand(pauseCmd)
}

var pauseCmd = &cobra.Command{
	Use:     "pause <name>",
	Aliases: []string{"stop"},
	Short:   "Snapshot and delete a VM to stop compute cost (keeps its IP)",
	Long: `Pause takes a snapshot of the VM's disk, preserves its public IP, then
deletes the VM so it no longer accrues compute cost. All data is retained in the
snapshot. Use 'onctl resume <name>' to bring it back.`,
	Run: func(cmd *cobra.Command, args []string) {
		if len(args) == 0 {
			fmt.Println("Please provide a VM name to pause")
			return
		}
		serverName := args[0]

		if !pauseForce {
			fmt.Printf("This will pause VM %q to stop its compute cost. Data is preserved; resume with 'onctl resume %s'.\n", serverName, serverName)
			if !yesNo() {
				os.Exit(0)
			}
		}

		fmt.Println("\033[32m✔\033[0m Pausing VM " + serverName + "... (this can take a few minutes)")
		if err := provider.Pause(cloud.Vm{Name: serverName}, pauseHot); err != nil {
			fmt.Println("\033[31m✘\033[0m Could not pause VM: " + serverName)
			fmt.Println(err)
			os.Exit(1)
		}
		fmt.Println("\033[32m✔\033[0m VM Paused (snapshot saved, server deleted): " + serverName)
	},
}
