package cloud

import (
	"crypto/md5"
	"fmt"
	"log"
	"net"
	"os"
	"strconv"

	"github.com/cdalar/onctl/internal/tools"
	"github.com/spf13/viper"

	"github.com/cdalar/onctl/internal/provideraws"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/awserr"
	"github.com/aws/aws-sdk-go/service/ec2"
	"golang.org/x/crypto/ssh"
)

type ProviderAws struct {
	Client *ec2.EC2
}

type NetworkProviderAws struct {
	Client *ec2.EC2
}

func (n NetworkProviderAws) Create(netw Network) (Network, error) {
	_, ipNet, err := net.ParseCIDR(netw.CIDR)
	log.Println("[DEBUG] ipNet.IP:", ipNet.IP.String())
	log.Println("[DEBUG] ipNet.Mask:", ipNet.Mask.String())
	if err != nil {
		log.Fatalln(err)
	}

	network, err := n.Client.CreateVpc(&ec2.CreateVpcInput{
		CidrBlock: aws.String(netw.CIDR),
	})
	if err != nil {
		log.Println(err)
	}
	_, err = n.Client.CreateTags(&ec2.CreateTagsInput{
		Resources: []*string{network.Vpc.VpcId},
		Tags: []*ec2.Tag{
			{
				Key:   aws.String("Name"),
				Value: aws.String(netw.Name),
			},
		},
	})
	if err != nil {
		log.Println(err)
	}
	subnet, err := n.Client.CreateSubnet(&ec2.CreateSubnetInput{
		CidrBlock: aws.String(netw.CIDR),
		VpcId:     network.Vpc.VpcId,
	})
	if err != nil {
		log.Println(err)
	}
	log.Println("[DEBUG] Subnet: ", subnet)
	return mapAwsNetwork(network.Vpc), nil
}

func (n NetworkProviderAws) Delete(net Network) error {
	log.Println("[DEBUG] Deleting network.ID: ", net.ID)
	result, err := n.Client.DescribeSubnets(&ec2.DescribeSubnetsInput{
		Filters: []*ec2.Filter{
			{
				Name:   aws.String("vpc-id"),
				Values: []*string{aws.String(net.ID)},
			},
		},
	})
	if err != nil {
		log.Fatalf("Failed to describe subnets for VPC %s: %v", net.ID, err)
	}

	for _, subnet := range result.Subnets {
		_, err := n.Client.DeleteSubnet(&ec2.DeleteSubnetInput{
			SubnetId: subnet.SubnetId,
		})
		if err != nil {
			log.Fatalf("Failed to delete subnet %s: %v", *subnet.SubnetId, err)
		}
	}

	resp, err := n.Client.DeleteVpc(&ec2.DeleteVpcInput{
		VpcId: aws.String(net.ID),
	})

	if err != nil {
		log.Println(err)
	}
	log.Println("[DEBUG] " + resp.String())
	return nil
}

func (n NetworkProviderAws) GetByName(networkName string) (Network, error) {
	networkList, err := n.Client.DescribeVpcs(&ec2.DescribeVpcsInput{
		Filters: []*ec2.Filter{
			{
				Name:   aws.String("tag:Name"),
				Values: []*string{aws.String(networkName)},
			},
		},
	})
	if err != nil {
		log.Println(err)
	}
	if len(networkList.Vpcs) == 0 {
		return Network{}, nil
	} else if len(networkList.Vpcs) > 1 {
		log.Fatalln("Multiple networks found with the same name")
	}
	return mapAwsNetwork(networkList.Vpcs[0]), nil
}

func (n NetworkProviderAws) List() ([]Network, error) {
	networkList, err := n.Client.DescribeVpcs(&ec2.DescribeVpcsInput{})
	if err != nil {
		log.Println(err)
	}
	if len(networkList.Vpcs) == 0 {
		return nil, nil
	}
	cloudList := make([]Network, 0, len(networkList.Vpcs))
	for _, network := range networkList.Vpcs {
		cloudList = append(cloudList, mapAwsNetwork(network))
		log.Println("[DEBUG] network: ", network)
	}
	return cloudList, nil
}

func mapAwsNetwork(network *ec2.Vpc) Network {
	var networkName = ""

	for _, tag := range network.Tags {
		if *tag.Key == "Name" {
			networkName = *tag.Value
		}
	}
	return Network{
		Provider: "aws",
		ID:       *network.VpcId,
		Name:     networkName,
		CIDR:     *network.CidrBlock,
	}
}

func (p ProviderAws) AttachNetwork(vm Vm, network Network) error {
	log.Println("[DEBUG] Attaching network: ", network)
	return nil
}

func (p ProviderAws) DetachNetwork(vm Vm, network Network) error {
	log.Println("[DEBUG] Detaching network: ", network)
	return nil
}

func (p ProviderAws) Deploy(server Vm) (Vm, error) {
	if server.Type == "" {
		server.Type = viper.GetString("aws.vm.type")
	}
	// Get the latest Ubuntu 22.04 AMI for the current region
	latestAMI, err := provideraws.GetLatestUbuntu2204AMI()
	if err != nil {
		log.Fatalln("Failed to get latest Ubuntu 22.04 AMI:", err)
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
		ImageId:      aws.String(latestAMI),
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
				AssociatePublicIpAddress: aws.Bool(server.JumpHost == ""), // Only associate public IP if no jumphost
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

func (p ProviderAws) SSHInto(serverName string, port int, privateKey string, jumpHost string) {

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
		JumpHost:       jumpHost,
	})
}
