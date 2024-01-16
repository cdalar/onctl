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

var (
	// composeFile   string
	publicKeyFile string
	filename      string
	instanceType  string
	vmName        string
	vm            cloud.Vm
	cloudInitFile string
	SSHPort       string
)

func init() {
	createCmd.Flags().StringVarP(&publicKeyFile, "publicKey", "k", "", "Path to publicKey file (default: ~/.ssh/id_rsa))")
	createCmd.Flags().StringVarP(&filename, "init", "i", "", "init bash script file")
	createCmd.Flags().StringVarP(&instanceType, "type", "t", "", "instance type")
	createCmd.Flags().StringVarP(&vmName, "name", "n", "", "vm name")
	createCmd.Flags().StringVarP(&SSHPort, "ssh-port", "p", "22", "ssh port")
	createCmd.Flags().StringVar(&cloudInitFile, "cloud-init", "", "cloud-init file")

}

var createCmd = &cobra.Command{
	Use:     "create",
	Aliases: []string{"start", "up"},
	Short:   "Create a VM",
	Run: func(cmd *cobra.Command, args []string) {
		s := spinner.New(spinner.CharSets[9], 100*time.Millisecond) // Build our new spinner
		filename = findFile(filename)
		cloudInitFile = findFile(cloudInitFile)
		log.Println("[DEBUG]", "filename: ", filename)
		log.Println("[DEBUG]", "cloudInitFile: ", cloudInitFile)

		publicKeyFile, privateKeyFile := getSSHKeyFilePaths(publicKeyFile)
		s.Start()
		s.Suffix = " Checking SSH Keys..."
		keyID, err := provider.CreateSSHKey(publicKeyFile)
		if err != nil {
			s.Stop()
			fmt.Println("\033[32m\u2718\033[0m Checking SSH Keys...")
			log.Fatalln(err)
		}
		s.Stop()
		fmt.Println("\033[32m\u2714\033[0m Checking SSH Keys...")

		log.Printf("[DEBUG] keyID: %s", keyID)
		if vmName == "" {
			if viper.GetString("vm.name") != "" {
				vmName = viper.GetString("vm.name")
			} else {
				vmName = tools.GenerateMachineUniqueName()
			}
		}
		log.Printf("[DEBUG] vmName: %s", vmName)
		server := cloud.Vm{
			Name:          vmName,
			Type:          instanceType,
			SSHKeyID:      keyID,
			SSHPort:       SSHPort,
			CloudInitFile: cloudInitFile,
		}
		log.Println("[DEBUG] s: ", server)
		// fmt.Println("Starting server...")
		s.Restart()
		s.Suffix = " VM Starting..."

		vm, err = provider.Deploy(server)
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

		log.Println("[DEBUG] waiting for cloud-init")
		log.Println("[DEBUG] ssh port: ", server.SSHPort)
		s.Stop()
		fmt.Println("\033[32m\u2714\033[0m VM Started...")
		s.Restart()
		s.Suffix = " Waiting for provider cloud-init..."
		tools.WaitForCloudInit(viper.GetString(cloudProvider+".vm.username"), vm.IP, server.SSHPort, string(privateKey))
		s.Stop()
		fmt.Println("\033[32m\u2714\033[0m Cloud-init finished...")
		log.Println("[DEBUG] cloud-init finished")
		if filename != "" {
			log.Println("[DEBUG] filename: ", filename)
			log.Println("[DEBUG] ssh port: ", server.SSHPort)
			s.Restart()
			s.Suffix = " Running " + filename + " on Remote..."

			_, err := tools.RunRemoteBashScript(&tools.RunRemoteBashScriptConfig{
				Username:   viper.GetString(cloudProvider + ".vm.username"),
				IPAddress:  vm.IP,
				SSHPort:    server.SSHPort,
				PrivateKey: string(privateKey),
				Script:     filename,
			})
			if err != nil {
				log.Fatal(err)
			}
			// fmt.Println(runInitOutput)
			s.Stop()
			fmt.Println("\033[32m\u2714\033[0m Remote Run Completed...")

		}
		s.Suffix = " Vm is Ready..."
		s.Stop()
		fmt.Println("\033[32m\u2714\033[0m Vm is Ready...")
	},
}
