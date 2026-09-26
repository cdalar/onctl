package cmd

import (
	"fmt"
	"os"

	"github.com/cdalar/onctl/pkg/cloud"

	"github.com/spf13/cobra"
)

var (
	restartForce       bool
	restartKernelImage string
)

func init() {
	restartCmd.Flags().BoolVarP(&restartForce, "force", "f", false, "restart without confirmation")
	restartCmd.Flags().StringVar(&restartKernelImage, "kernel-image", "", "Firecracker: boot this kernel (vmlinux) from now on instead of the VM's current one")
	rootCmd.AddCommand(restartCmd)
	vmCmd.AddCommand(restartCmd)
}

var restartCmd = &cobra.Command{
	Use:   "restart <name>",
	Short: "Cold-boot a VM again from its own disk, optionally on a new kernel (fc)",
	Long: `Restart stops a VM and boots it again from its existing disk -- a reboot that
keeps all its data, unlike destroy + create. Guest memory is not kept.

With --kernel-image the VM boots that kernel from now on, which is the only way
to move an existing Firecracker VM to a new kernel without losing its disk.
Also brings back a VM whose process died (e.g. after a host reboot), and a
paused VM (its snapshot is discarded).

Only the Firecracker (fc) provider supports restart.`,
	Args: cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		serverName := args[0]
		restarter, ok := provider.(cloud.Restarter)
		if !ok {
			fmt.Println("\033[31m✘\033[0m The current provider does not support restart (only fc does).")
			os.Exit(1)
		}

		if !restartForce {
			fmt.Printf("This will reboot VM %q from its disk; anything only in its memory is lost.\n", serverName)
			if !yesNo() {
				os.Exit(0)
			}
		}

		fmt.Println("\033[32m✔\033[0m Restarting VM " + serverName + "...")
		vm, err := restarter.Restart(cloud.Vm{Name: serverName}, cloud.RestartOptions{KernelImage: restartKernelImage})
		if err != nil {
			fmt.Println("\033[31m✘\033[0m Could not restart VM: " + serverName)
			fmt.Println(err)
			os.Exit(1)
		}
		fmt.Println("\033[32m✔\033[0m VM Restarted: " + vm.Name + " (" + vm.IP + ")")
	},
}
