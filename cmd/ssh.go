package cmd

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"time"

	"github.com/briandowns/spinner"
	"github.com/cdalar/onctl/internal/tools"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var (
	port  int
	apply string
)

func init() {
	sshCmd.Flags().IntVarP(&port, "port", "p", 22, "ssh port")
	sshCmd.Flags().StringVarP(&apply, "apply", "a", "", "apply script")
	sshCmd.Flags().StringSliceVarP(&opt.Variables, "vars", "e", []string{}, "Environment variables passed to the script")
}

var sshCmd = &cobra.Command{
	Use:                   "ssh VM_NAME",
	Short:                 "Spawn an SSH connection to a VM",
	Args:                  cobra.MinimumNArgs(1),
	TraverseChildren:      true,
	DisableFlagsInUseLine: true,
	Run: func(cmd *cobra.Command, args []string) {
		s := spinner.New(spinner.CharSets[9], 100*time.Millisecond) // Build our new spinner
		apply = findFile(apply)
		log.Println("[DEBUG] args: ", args)

		if len(args) == 0 {
			fmt.Println("Please provide a VM id")
			return
		}
		log.Println("[DEBUG] port: ", port)
		log.Println("[DEBUG] filename: ", apply)
		_, privateKeyFile := getSSHKeyFilePaths("")
		privateKey, err := os.ReadFile(privateKeyFile)
		if err != nil {
			log.Fatal(err)
		}
		vm, err := provider.GetByName(args[0])
		if err != nil {
			log.Fatalln(err)
		}
		remote := tools.Remote{
			Username:   viper.GetString(cloudProvider + ".vm.username"),
			IPAddress:  vm.IP,
			SSHPort:    port,
			PrivateKey: string(privateKey),
		}

		if apply != "" {
			s.Start()
			s.Suffix = " Applying " + apply

			err = remote.CopyAndRunRemoteFile(&tools.CopyAndRunRemoteFileConfig{
				File: apply,
				Vars: opt.Variables,
			})
			if err != nil {
				s.Stop()
				fmt.Println("\033[32m\u2718\033[0m Could not apply " + apply + " to VM: " + vm.Name)
				log.Fatal(err)
			}
			s.Stop()
			fmt.Println("\033[32m\u2714\033[0m " + filepath.Base(apply) + " applied to VM: " + vm.Name)

		} else {
			provider.SSHInto(args[0], port)
		}

	},
}
