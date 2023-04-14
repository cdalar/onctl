package cmd

import (
	"cdalar/onctl/internal/cloud"
	"cdalar/onctl/internal/files"
	"cdalar/onctl/internal/provideraws"
	"cdalar/onctl/internal/providerhtz"
	"cdalar/onctl/internal/tools"
	"fmt"
	"log"
	"os"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/awserr"
	"github.com/aws/aws-sdk-go/service/ec2"
	homedir "github.com/mitchellh/go-homedir"
	"github.com/spf13/cobra"
)

var (
	composeFile   string
	publicKeyFile string
	initFile      string
	exposePort    int64
	instanceType  string
	username      string
	vm            cloud.Vm
	provider      cloud.CloudProviderInterface
)

func init() {
	createCmd.Flags().StringVarP(&composeFile, "composeFile", "c", "##", "Path to docker-compose file")
	createCmd.Flags().StringVarP(&publicKeyFile, "publicKey", "k", "##", "Path to publicKey file (default: ~/.ssh/id_rsa))")
	createCmd.Flags().StringVarP(&initFile, "initFile", "i", "##", "init bash script file")
	createCmd.Flags().Int64VarP(&exposePort, "port", "p", 80, "port you want to expose to internet")
	createCmd.Flags().StringVarP(&instanceType, "type", "t", "##", "instance type")
}

var createCmd = &cobra.Command{
	Use:   "create",
	Short: "Create Vm",
	Run: func(cmd *cobra.Command, args []string) {
		home, err := homedir.Dir()
		if err != nil {
			log.Fatal(err)
		}
		if publicKeyFile == "##" {
			publicKeyFile = home + "/.ssh/id_rsa.pub"
		}

		if err != nil {
			log.Fatal(err)
		}
		switch os.Getenv("CLOUD_PROVIDER") {
		case "hetzner":
			provider = cloud.ProviderHetzner{
				Client: providerhtz.GetClient(),
			}
			//TODO username should be part of the image
			username = "root"
		case "aws":
			provider = &cloud.ProviderAws{
				Client: provideraws.GetClient(),
			}
			username = "ubuntu"
		}
		keyID, err := provider.CreateSSHKey(publicKeyFile)
		if err != nil {
			log.Fatalln(err)
		}
		log.Printf("[DEBUG] keyID: %s", keyID)
		s := cloud.Vm{
			Name:        tools.GenerateMachineUniqueName(),
			Type:        instanceType,
			SSHKeyID:    keyID,
			ExposePorts: []int64{exposePort},
		}
		log.Println("Starting server...")
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
		if initFile == "##" {
			initFileEmbeded, _ := files.EmbededFiles.ReadFile("init.sh")
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
		}

		if _, err := os.Stat(initFile); err != nil {
			fmt.Println(initFile + " Init file not found")
			os.Exit(1)
		}
		if _, err := os.Stat(publicKeyFile); err != nil {
			fmt.Println(publicKeyFile + " Public key file not found")
			os.Exit(1)
		}

		tools.WaitForCloudInit(username, vm.IP, string(privateKey))
		tools.PrepareDocker(username, vm.IP, string(privateKey), initFile)

		tools.CreateDeployOutputFile(&tools.DeployOutput{
			Username:   username,
			PublicIP:   vm.IP,
			PublicURL:  "http://" + vm.IP,
			DockerHost: "ssh://" + username + "@" + vm.IP,
		})

		// tools.RunDockerCompose(username, cloudServer.IP, string(privateKey), composeFile)
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

// CheckIfInstanceExists checks if an instance with the given name exists
// and returns the instance ID if it does
func CheckIfInstanceExists(svc *ec2.EC2, instanceName string) (string, error) {
	input := &ec2.DescribeInstancesInput{
		Filters: []*ec2.Filter{
			{
				Name:   aws.String("tag:Name"),
				Values: []*string{aws.String(instanceName)},
			},
			{
				Name:   aws.String("instance-state-name"),
				Values: []*string{aws.String("running")},
			},
		},
	}
	result, err := svc.DescribeInstances(input)
	if err != nil {
		if aerr, ok := err.(awserr.Error); ok {
			switch aerr.Code() {
			default:
				fmt.Println(aerr.Error())
			}
		} else {
			// Print the error, cast err to awserr.Error to get the Code and
			// Message from an error.
			fmt.Println(err.Error())
		}
		return "", err
	}
	if len(result.Reservations) > 0 {
		log.Println("[DEBUG] Instance Id: " + *result.Reservations[0].Instances[0].InstanceId)
		return *result.Reservations[0].Instances[0].InstanceId, err
	}
	return "", err
}
