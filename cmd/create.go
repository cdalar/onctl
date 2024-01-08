package cmd

import (
	"fmt"
	"log"
	"os"

	"github.com/cdalar/onctl/internal/tools"

	"github.com/cdalar/onctl/internal/files"

	"github.com/cdalar/onctl/internal/cloud"

	homedir "github.com/mitchellh/go-homedir"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var (
	composeFile   string
	publicKeyFile string
	initFile      string
	exposePort    int64
	instanceType  string
	vmName        string
	vm            cloud.Vm
	cloudInitFile string
	SSHPort       string
)

func init() {
	createCmd.Flags().StringVarP(&composeFile, "composeFile", "c", "", "Path to docker-compose file")
	createCmd.Flags().StringVarP(&publicKeyFile, "publicKey", "k", "", "Path to publicKey file (default: ~/.ssh/id_rsa))")
	createCmd.Flags().StringVarP(&initFile, "initFile", "i", "", "init bash script file")
	// createCmd.Flags().Int64VarP(&exposePort, "port", "p", 80, "port you want to expose to internet")
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
			if viper.GetString(cloudProvider+".vm.name") != "" {
				vmName = viper.GetString(cloudProvider + ".vm.name")
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

// func runDocker(instanceId string) {
// 	instance := provideraws.DescribeInstance(instanceId)
// 	log.Println("Public IP: " + *instance.PublicIpAddress)
// 	err := os.WriteFile("ip.txt", []byte(*instance.PublicIpAddress), 0644)
// 	if err != nil {
// 		log.Print(err)
// 	}

// 	log.Println("[DEBUG] composeFile: " + composeFile)
// 	err = tools.SSHCopyFile("ubuntu", *instance.PublicIpAddress, string(privateKey), composeFile, "/home/ubuntu/docker-compose.yml")
// 	if err != nil {
// 		log.Println("Error on copy Compose")
// 		log.Println(err)
// 	}
// log.Println("[DEBUG] initFile: " + initFile)
// err = tools.SSHCopyFile("ubuntu", *instance.PublicIpAddress, string(privateKey), initFile, "/home/ubuntu/init.sh")
// if err != nil {
// 	log.Println("Error on copy Init")
// 	log.Println(err)
// }

// 	log.Println("[DEBUG] running init.sh...")
// 	runInitOutput, err := tools.RemoteRun("ubuntu", *instance.PublicIpAddress, string(privateKey), "chmod +x init.sh && sudo ./init.sh")
// 	if err != nil {
// 		log.Println("Error on init.sh")
// 		fmt.Println(runInitOutput)
// 		log.Fatal(err)
// 	}
// 	fmt.Println(runInitOutput)

// 	// cmdCompose := exec.Command("DOCKER_HOST=ssh://ubuntu@$(cat ip.txt)", "docker", "compose", "up", "-d", "--build")
// 	// log.Println(*instance.PublicIpAddress)
// 	// os.Setenv("DOCKER_HOST", "ssh://ubuntu@"+*instance.PublicIpAddress)
// 	// cmdCompose := exec.Command("docker", "compose", "up", "-d", "--build")
// 	// cmdCompose := exec.Command("ls", "-al")
// 	// // cmdCompose := exec.Command("echo", "$ASD")
// 	// err = cmdCompose.Run()
// 	// if err != nil {
// 	// 	log.Println("Run Compose")
// 	// 	log.Fatal(err)
// 	// }
// 	// out, err := cmdCompose.Output()
// 	// if err != nil {
// 	// 	log.Fatal(err)
// 	// }
// 	// fmt.Println(string(out))
// 	log.Println("Service configured on:", "http://"+*instance.PublicIpAddress+":"+strconv.FormatInt(exposePort, 10))

// 	tools.CreateDeployOutputFile(&tools.DeployOutput{
// 		PublicIP:  *instance.PublicIpAddress,
// 		PublicURL: "http://" + *instance.PublicIpAddress + ":" + strconv.FormatInt(exposePort, 10),
// 	})
// }
