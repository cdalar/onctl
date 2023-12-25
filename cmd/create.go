package cmd

import (
	"fmt"
	"log"
	"os"

	"github.com/cdalar/onctl/internal/cloud"
	"github.com/cdalar/onctl/internal/files"
	"github.com/cdalar/onctl/internal/tools"

	homedir "github.com/mitchellh/go-homedir"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var (
	composeFile   string
	publicKeyFile string
	initFile      string
	instanceType  string
	vmName        string
	vm            cloud.Vm
	cloudInitFile string
)

func init() {
	createCmd.Flags().StringVarP(&composeFile, "composeFile", "c", "", "Path to docker-compose file")
	createCmd.Flags().StringVarP(&publicKeyFile, "publicKey", "k", "", "Path to publicKey file (default: ~/.ssh/id_rsa))")
	createCmd.Flags().StringVarP(&initFile, "initFile", "i", "", "init bash script file")
	createCmd.Flags().StringVarP(&instanceType, "type", "t", "", "instance type")
	createCmd.Flags().StringVarP(&vmName, "name", "n", "", "vm name")
	createCmd.Flags().StringVarP(&port, "port", "p", "22", "ssh port")
	createCmd.Flags().StringVar(&cloudInitFile, "cloud-init-file", "", "cloud-init file")

}

var createCmd = &cobra.Command{
	Use:     "create",
	Aliases: []string{"start", "up"},
	Short:   "Create a VM",
	Run: func(cmd *cobra.Command, args []string) {
		home, err := homedir.Dir()
		if err != nil {
			log.Fatal(err)
		}
		if initFile != "" {
			if _, err := os.Stat(initFile); err != nil {
				log.Println(initFile, "file not found in fileststem, trying to find in embeded files")
				if _, err = files.EmbededFiles.ReadFile(initFile); err != nil {
					log.Println(initFile, "file not found in embeded files")
					os.Exit(1)
				}
			}
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
			vmName = tools.GenerateMachineUniqueName()
		}
		log.Printf("[DEBUG] vmName: %s", vmName)
		s := cloud.Vm{
			Name:          vmName,
			Type:          instanceType,
			SSHKeyID:      keyID,
			SSHPort:       port,
			CloudInitFile: cloudInitFile,
		}
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
		tools.WaitForCloudInit(viper.GetString(cloudProvider+".vm.username"), vm.IP, s.SSHPort, string(privateKey))
		if initFile != "" {
			initFileLocal, err := os.Stat(initFile)
			if err != nil { // file not found in filesystem
				initFileEmbeded, _ := files.EmbededFiles.ReadFile(initFile)
				tmpfile, err := os.CreateTemp("", "onctl")
				if err != nil {
					log.Fatal(err)
				}
				log.Println("[DEBUG] initTmpfile:" + tmpfile.Name())
				_, err = tmpfile.Write(initFileEmbeded)
				if err != nil {
					log.Fatal(err)
				}
				defer tmpfile.Close()
				initFile = tmpfile.Name()
			} else { // file found in filesystem
				initFile = initFileLocal.Name()
			}

			_, err = tools.RunRemoteBashScript(viper.GetString(cloudProvider+".vm.username"), vm.IP, vm.SSHPort, string(privateKey), initFile)
			if err != nil {
				log.Fatal(err)
			}
		}
	},
}
