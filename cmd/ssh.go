package cmd

import (
	"fmt"
	"log"
	"os"

	"github.com/cdalar/onctl/internal/tools"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var (
	port  string
	apply string
)

func init() {
	sshCmd.Flags().StringVarP(&port, "port", "p", "22", "ssh port")
	sshCmd.Flags().StringVarP(&apply, "apply", "a", "", "apply script")
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
		log.Println("[DEBUG] port: ", port)
		log.Println("[DEBUG] filename: ", apply)
		_, privateKeyFile := getSSHKeyFilePaths("")
		privateKey, err := os.ReadFile(privateKeyFile)
		if err != nil {
			log.Fatal(err)
		}
		vm := provider.GetByName(args[0])
		if apply != "" {
			_, err := tools.RunRemoteBashScript(&tools.RunRemoteBashScriptConfig{
				Username:   viper.GetString(cloudProvider + ".vm.username"),
				IPAddress:  vm.IP,
				SSHPort:    port,
				PrivateKey: string(privateKey),
				Script:     apply,
				IsApply:    true,
			})
			if err != nil {
				log.Fatal(err)
			}
		} else {
			provider.SSHInto(args[0], port)
		}

	},
}
