package cmd

import (
	"fmt"
	"log"
	"os"

	"github.com/cdalar/onctl/internal/cloud"
	"github.com/cdalar/onctl/internal/tools"

	homedir "github.com/mitchellh/go-homedir"
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
	initFile      string
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
		filename = findFile(filename)
		cloudInitFile = findFile(cloudInitFile)
		log.Println("[DEBUG]", "filename: ", filename)
		log.Println("[DEBUG]", "cloudInitFile: ", cloudInitFile)
		home, err := homedir.Dir()
		if err != nil {
			log.Fatal(err)
		}

		if publicKeyFile == "" {
			publicKeyFile = home + "/.ssh/id_rsa.pub"
		}

		if err != nil {
			log.Fatal(err)
		}
		keyID, err := provider.CreateSSHKey(publicKeyFile)
		if err != nil {
			log.Fatalln(err)
		}
		log.Printf("[DEBUG] keyID: %s", keyID)
		if vmName == "" {
			if viper.GetString("vm.name") != "" {
				vmName = viper.GetString("vm.name")
			} else {
				vmName = tools.GenerateMachineUniqueName()
			}
		}
		log.Printf("[DEBUG] vmName: %s", vmName)
		s := cloud.Vm{
			Name:          vmName,
			Type:          instanceType,
			SSHKeyID:      keyID,
			SSHPort:       SSHPort,
			CloudInitFile: cloudInitFile,
		}
		log.Println("[DEBUG] s: ", s)
		fmt.Println("Starting server...")
		vm, err = provider.Deploy(s)
		if err != nil {
			log.Println(err)
		}
		fmt.Println("Server IP: " + vm.IP)
		log.Println("[DEBUG] Vm:" + vm.String())
		privateKey, err := os.ReadFile(publicKeyFile[:len(publicKeyFile)-4])
		if err != nil {
			log.Println(err)
		}

		if _, err := os.Stat(publicKeyFile); err != nil {
			log.Fatalln(publicKeyFile + " Public key file not found")
		}
		log.Println("[DEBUG] waiting for cloud-init")
		log.Println("[DEBUG] ssh port: ", s.SSHPort)
		tools.WaitForCloudInit(viper.GetString(cloudProvider+".vm.username"), vm.IP, s.SSHPort, string(privateKey))
		log.Println("[DEBUG] cloud-init finished")
		if filename != "" {
			log.Println("[DEBUG] filename: ", filename)
			log.Println("[DEBUG] ssh port: ", s.SSHPort)
			_, err = tools.RunRemoteBashScript(viper.GetString(cloudProvider+".vm.username"), vm.IP, s.SSHPort, string(privateKey), filename)
			if err != nil {
				log.Fatal(err)
			}

		}
	},
}
