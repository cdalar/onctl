package cmd

import (
	"fmt"
	"log"
	"os"
	"time"

	"github.com/briandowns/spinner"
	"github.com/cdalar/onctl/internal/cloud"
	"github.com/cdalar/onctl/internal/tools"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

// TODO: ? Struct for options. cmdCreateOptions
// TODO: .env file support
// TODO: remove initFile and implement ssh apply structure
// TODO: ? Create Packages with cloud-init, apply Files, Variables. (cloud-init, apply, vars)

type cmdCreateOptions struct {
	PublicKeyFile string
	ApplyFile     string
	CloudInitFile string
	DotEnvFile    string
	Variables     []string
	Vm            cloud.Vm
}

var (
	opt cmdCreateOptions
	err error
)

func init() {
	createCmd.Flags().StringVarP(&opt.PublicKeyFile, "publicKey", "k", "", "Path to publicKey file (default: ~/.ssh/id_rsa))")
	createCmd.Flags().StringVarP(&opt.ApplyFile, "apply", "a", "", "apply bash script file")
	createCmd.Flags().StringVarP(&opt.Vm.Type, "type", "t", "", "instance type")
	createCmd.Flags().StringVarP(&opt.Vm.Name, "name", "n", "", "vm name")
	createCmd.Flags().IntVarP(&opt.Vm.SSHPort, "ssh-port", "p", 22, "ssh port")
	createCmd.Flags().StringVar(&opt.CloudInitFile, "cloud-init", "", "cloud-init file")
	createCmd.Flags().StringVar(&opt.DotEnvFile, "dot-env", "", "dot-env (.env) file")
	createCmd.Flags().StringSliceVarP(&opt.Variables, "vars", "e", []string{}, "Environment variables passed to the script")
}

var createCmd = &cobra.Command{
	Use:     "create",
	Aliases: []string{"start", "up"},
	Short:   "Create a VM",
	Run: func(cmd *cobra.Command, args []string) {
		s := spinner.New(spinner.CharSets[9], 100*time.Millisecond) // Build our new spinner
		opt.ApplyFile = findFile(opt.ApplyFile)
		opt.CloudInitFile = findFile(opt.CloudInitFile)

		// BEGIN SSH Key
		publicKeyFile, privateKeyFile := getSSHKeyFilePaths(opt.PublicKeyFile)
		s.Start()
		s.Suffix = " Checking SSH Keys..."
		opt.Vm.SSHKeyID, err = provider.CreateSSHKey(publicKeyFile)
		if err != nil {
			s.Stop()
			fmt.Println("\033[32m\u2718\033[0m Checking SSH Keys...")
			log.Fatalln(err)
		}
		s.Stop()
		fmt.Println("\033[32m\u2714\033[0m Checking SSH Keys...")
		// END SSH Key

		// BEGIN Set VM Name
		log.Printf("[DEBUG] keyID: %s", opt.Vm.SSHKeyID)
		if opt.Vm.Name == "" {
			if viper.GetString("vm.name") != "" {
				opt.Vm.Name = viper.GetString("vm.name")
			} else {
				opt.Vm.Name = tools.GenerateMachineUniqueName()
			}
		}
		s.Restart()
		s.Suffix = " VM Starting..."
		// END Set VM Name

		vm, err := provider.Deploy(opt.Vm)
		if err != nil {
			log.Println(err)
		}
		// fmt.Println("Server IP: " + vm.IP)
		s.Restart()
		s.Suffix = " VM IP: " + vm.IP
		s.Stop()
		fmt.Println("\033[32m\u2714\033[0m" + s.Suffix)

		log.Println("[DEBUG] Vm:" + vm.String())
		privateKey, err := os.ReadFile(privateKeyFile)
		if err != nil {
			log.Println(err)
		}

		// BEGIN Cloud-init
		log.Println("[DEBUG] waiting for cloud-init")
		log.Println("[DEBUG] ssh port: ", opt.Vm.SSHPort)
		s.Stop()
		fmt.Println("\033[32m\u2714\033[0m VM Started...")
		s.Restart()
		s.Suffix = " Waiting for provider cloud-init..."
		remote := tools.Remote{
			Username:   viper.GetString(cloudProvider + ".vm.username"),
			IPAddress:  vm.IP,
			SSHPort:    opt.Vm.SSHPort,
			PrivateKey: string(privateKey),
		}

		remote.WaitForCloudInit()

		s.Stop()
		fmt.Println("\033[32m\u2714\033[0m Cloud-init finished...")
		log.Println("[DEBUG] cloud-init finished")
		// END Cloud-init

		if opt.ApplyFile != "" {
			s.Restart()
			s.Suffix = " Running " + opt.ApplyFile + " on Remote..."

			err = remote.CopyAndRunRemoteFile(&tools.CopyAndRunRemoteFileConfig{
				File: opt.ApplyFile,
				Vars: opt.Variables,
			})
			if err != nil {
				log.Println(err)
			}
			s.Stop()
			fmt.Println("\033[32m\u2714\033[0m Remote Run Completed...")

		}
		s.Suffix = " Vm is Ready..."
		s.Stop()
		fmt.Println("\033[32m\u2714\033[0m Vm is Ready...")
	},
}
