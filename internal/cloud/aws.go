package cloud

import (
	"crypto/md5"
	"fmt"
	"log"
	"os"
	"strconv"

	"github.com/cdalar/onctl/internal/tools"
	"github.com/spf13/viper"

	"github.com/cdalar/onctl/internal/provideraws"

	"github.com/aws/aws-sdk-go/aws"         //nolint:staticcheck // TODO: migrate to AWS SDK v2
	"github.com/aws/aws-sdk-go/aws/awserr"  //nolint:staticcheck // TODO: migrate to AWS SDK v2
	"github.com/aws/aws-sdk-go/service/ec2" //nolint:staticcheck // TODO: migrate to AWS SDK v2
	"golang.org/x/crypto/ssh"
)

type ProviderAws struct {
	Client *ec2.EC2
}

func (p ProviderAws) Deploy(server Vm) (Vm, error) {
	if server.Type == "" {
		server.Type = viper.GetString("aws.vm.type")
	}
	images, err := provideraws.GetImages()
	if err != nil {
		log.Println(err)
	}

	keyPairs, err := p.Client.DescribeKeyPairs(&ec2.DescribeKeyPairsInput{
		KeyPairIds: []*string{aws.String(server.SSHKeyID)},
	})
	if err != nil {
		log.Fatalln(err)
	}

	vpcId := provideraws.GetDefaultVpcId(p.Client)
	log.Println("[DEBUG] VPC ID: ", vpcId)

	// securityGroupIds := []*string{}
	// sgIdForSSH := provideraws.CreateSecurityGroupSSH(p.Client, vpcId)
	// securityGroupIds = append(securityGroupIds, sgIdForSSH)
	// for _, port := range server.ExposePorts {
	// 	sgId := provideraws.CreateSecurityGroupForPort(p.Client, vpcId, port)
	// 	log.Println("[DEBUG] Security Group ID: ", sgId)
	// 	securityGroupIds = append(securityGroupIds, sgId)
	// }
	// log.Println("[DEBUG] Security Group Ids: ", securityGroupIds)
	input := &ec2.RunInstancesInput{
		ImageId:      aws.String(*images[0].ImageId),
		InstanceType: aws.String(server.Type),
		// InstanceMarketOptions: &ec2.InstanceMarketOptionsRequest{
		// 	MarketType: aws.String("spot"),
		// 	SpotOptions: &ec2.SpotMarketOptions{
		// 		MaxPrice: aws.String("0.02"),
		// 	},
		// },
		MinCount: aws.Int64(1),
		MaxCount: aws.Int64(1),
		KeyName:  aws.String(*keyPairs.KeyPairs[0].KeyName),
		NetworkInterfaces: []*ec2.InstanceNetworkInterfaceSpecification{
			{
				DeviceIndex: aws.Int64(0),
				// SubnetId:                 aws.String(subnetIds[0]),
				AssociatePublicIpAddress: aws.Bool(true),
				DeleteOnTermination:      aws.Bool(true),
				// Groups:                   securityGroupIds,
			},
		},
		TagSpecifications: []*ec2.TagSpecification{
			{
				ResourceType: aws.String("instance"),
				Tags: []*ec2.Tag{
					{
						Key:   aws.String("Name"),
						Value: aws.String(server.Name),
					},
					{
						Key:   aws.String("Owner"),
						Value: aws.String("onctl"),
					},
				},
			},
		},
	}

	descOut, err := p.Client.DescribeInstances(&ec2.DescribeInstancesInput{
		Filters: []*ec2.Filter{
			{
				Name:   aws.String("tag:Name"),
				Values: []*string{aws.String(server.Name)},
			},
			{
				Name:   aws.String("tag:Owner"),
				Values: []*string{aws.String("onctl")},
			},
			{
				Name:   aws.String("instance-state-name"),
				Values: []*string{aws.String("running")},
			},
		},
	})
	if err != nil {
		log.Fatalln(err)
	}
	if len(descOut.Reservations) > 0 {
		log.Println("Instance already exists, skipping creation")
		return mapAwsServer(descOut.Reservations[0].Instances[0]), nil
	}

	result, err := p.Client.RunInstances(input)
	if err != nil {
		if aerr, ok := err.(awserr.Error); ok {
			switch aerr.Code() {
			default:
				fmt.Println(aerr.Error())
			}
		} else {
			fmt.Println(err.Error())
		}
		return Vm{}, err
	}
	log.Println("[DEBUG] " + result.String())
	err = p.Client.WaitUntilInstanceRunning(&ec2.DescribeInstancesInput{
		InstanceIds: []*string{result.Instances[0].InstanceId},
	})
	if err != nil {
		log.Fatalln(err)
	}
	instance := provideraws.DescribeInstance(*result.Instances[0].InstanceId)

	return mapAwsServer(instance), nil
}

func (p ProviderAws) Destroy(server Vm) error {
	if server.ID == "" {
		log.Println("[DEBUG] Server ID is empty")
		s, err := p.GetByName(server.Name)
		if err != nil || s.ID == "" {
			log.Fatalln(err)
		}
		server.ID = s.ID
	}
	log.Println("[DEBUG] Terminating Instance: " + server.ID)
	input := &ec2.TerminateInstancesInput{
		InstanceIds: []*string{
			aws.String(server.ID),
		},
	}
	result, err := p.Client.TerminateInstances(input)
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
		return err
	}
	log.Println("[DEBUG] " + result.String())
	return nil
}

func (p ProviderAws) List() (VmList, error) {

	input := &ec2.DescribeInstancesInput{
		Filters: []*ec2.Filter{
			{
				Name:   aws.String("tag:Owner"),
				Values: []*string{aws.String("onctl")},
			},
		},
	}
	instances, err := p.Client.DescribeInstances(input)
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
		return VmList{}, err
	}
	log.Println("[DEBUG] " + instances.String())

	if len(instances.Reservations) > 0 {
		log.Println("[DEBUG] # of Instances:" + strconv.Itoa(len(instances.Reservations[0].Instances)))
		log.Println("[DEBUG] # of Reservations:" + strconv.Itoa(len(instances.Reservations)))
		cloudList := make([]Vm, 0, len(instances.Reservations))
		for _, reserv := range instances.Reservations {
			cloudList = append(cloudList, mapAwsServer(reserv.Instances[0]))
		}
		output := VmList{
			List: cloudList,
		}
		return output, nil
	}
	return VmList{}, nil
}

func (p ProviderAws) CreateSSHKey(publicKeyFile string) (keyID string, err error) {
	publicKey, err := os.ReadFile(publicKeyFile)
	if err != nil {
		log.Fatalln(err)
	}

	SSHKeyMD5 := fmt.Sprintf("%x", md5.Sum(publicKey))
	pk, _, _, _, err := ssh.ParseAuthorizedKey(publicKey)
	if err != nil {
		panic(err)
	}

	// Get the fingerprint
	SSHKeyFingerPrint := ssh.FingerprintLegacyMD5(pk)

	// Print the fingerprint
	log.Println("[DEBUG] SSH Key Fingerpring: " + SSHKeyFingerPrint)
	log.Println("[DEBUG] SSH Key MD5: " + SSHKeyMD5)
	importKeyPairOutput, err := p.Client.ImportKeyPair(&ec2.ImportKeyPairInput{
		PublicKeyMaterial: publicKey,
		KeyName:           aws.String("onctl-" + SSHKeyMD5[:8]),
	})
	log.Println("[DEBUG] " + importKeyPairOutput.String())
	if err != nil {
		if aerr, ok := err.(awserr.Error); ok {
			log.Println("[DEBUG] AWS Error: " + aerr.Code())
			switch aerr.Code() {
			case "InvalidKeyPair.Duplicate":
				log.Println("[DEBUG] SSH Key already exists")
				keyPair, err := p.Client.DescribeKeyPairs(&ec2.DescribeKeyPairsInput{
					KeyNames: []*string{aws.String("onctl-" + SSHKeyMD5[:8])},
				})
				if err != nil {
					log.Fatalln(err)
				}
				log.Println("[DEBUG] SSH Key ID: " + *keyPair.KeyPairs[0].KeyPairId)
				return *keyPair.KeyPairs[0].KeyPairId, nil
			default:
				fmt.Println(aerr.Error())
			}
		} else {
			fmt.Println(err.Error())
		}
		log.Fatalln(err)
	}
	return *importKeyPairOutput.KeyPairId, nil
}

func mapAwsServer(server *ec2.Instance) Vm {
	var serverName = ""

	for _, tag := range server.Tags {
		if *tag.Key == "Name" {
			serverName = *tag.Value
		}
	}
	// log.Println("[DEBUG] " + server.String())
	if server.PublicIpAddress == nil {
		server.PublicIpAddress = aws.String("")
	}
	if server.PrivateIpAddress == nil {
		server.PrivateIpAddress = aws.String("")
	}
	return Vm{
		Provider:  "aws",
		ID:        *server.InstanceId,
		Name:      serverName,
		IP:        *server.PublicIpAddress,
		PrivateIP: *server.PrivateIpAddress,
		Type:      *server.InstanceType,
		Status:    *server.State.Name,
		CreatedAt: *server.LaunchTime,
		Location:  *server.Placement.AvailabilityZone,
		Cost: CostStruct{
			Currency:        "N/A",
			CostPerHour:     0,
			CostPerMonth:    0,
			AccumulatedCost: 0,
		},
	}
}

func (p ProviderAws) GetByName(serverName string) (Vm, error) {
	s, err := p.Client.DescribeInstances(&ec2.DescribeInstancesInput{
		Filters: []*ec2.Filter{
			{
				Name:   aws.String("tag:Name"),
				Values: []*string{aws.String(serverName)},
			},
			{
				Name:   aws.String("tag:Owner"),
				Values: []*string{aws.String("onctl")},
			},
			{
				Name:   aws.String("instance-state-name"),
				Values: []*string{aws.String("running")},
			},
		},
	})
	if err != nil {
		log.Fatalln(err)
	}
	if len(s.Reservations) == 0 {
		// fmt.Println("No server found with name: " + serverName)
		// os.Exit(1)
		return Vm{}, err
	}
	return mapAwsServer(s.Reservations[0].Instances[0]), nil
}

func (p ProviderAws) SSHInto(serverName string, port int, privateKey string) {

	s, err := p.GetByName(serverName)
	if err != nil || s.ID == "" {
		log.Fatalln(err)
	}
	log.Println("[DEBUG] " + s.String())

	if privateKey == "" {
		privateKey = viper.GetString("ssh.privateKey")
	}
	tools.SSHIntoVM(tools.SSHIntoVMRequest{
		IPAddress:      s.IP,
		User:           viper.GetString("aws.vm.username"),
		Port:           port,
		PrivateKeyFile: privateKey,
	})
}
