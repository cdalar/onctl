package cloud

import (
	"context"
	"crypto/md5"
	"errors"
	"fmt"
	"log"
	"os"
	"strconv"
	"time"

	"github.com/cdalar/onctl/internal/tools"
	"github.com/spf13/viper"

	"github.com/cdalar/onctl/internal/provideraws"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	"github.com/aws/aws-sdk-go-v2/service/ec2/types"
	"github.com/aws/smithy-go"
	"golang.org/x/crypto/ssh"
)

type ProviderAws struct {
	Client *ec2.Client
}

func (p ProviderAws) Deploy(server Vm) (Vm, error) {
	if server.Type == "" {
		server.Type = viper.GetString("aws.vm.type")
	}
	images, err := provideraws.GetImages()
	if err != nil {
		log.Println(err)
	}

	keyPairs, err := p.Client.DescribeKeyPairs(context.TODO(), &ec2.DescribeKeyPairsInput{
		KeyPairIds: []string{server.SSHKeyID},
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
		InstanceType: types.InstanceType(server.Type),
		// InstanceMarketOptions: &types.InstanceMarketOptionsRequest{
		// 	MarketType: types.MarketTypeSpot,
		// 	SpotOptions: &types.SpotMarketOptions{
		// 		MaxPrice: aws.String("0.02"),
		// 	},
		// },
		MinCount: aws.Int32(1),
		MaxCount: aws.Int32(1),
		KeyName:  aws.String(*keyPairs.KeyPairs[0].KeyName),
		NetworkInterfaces: []types.InstanceNetworkInterfaceSpecification{
			{
				DeviceIndex: aws.Int32(0),
				// SubnetId:                 aws.String(subnetIds[0]),
				AssociatePublicIpAddress: aws.Bool(true),
				DeleteOnTermination:      aws.Bool(true),
				// Groups:                   securityGroupIds,
			},
		},
		TagSpecifications: []types.TagSpecification{
			{
				ResourceType: types.ResourceTypeInstance,
				Tags: []types.Tag{
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

	descOut, err := p.Client.DescribeInstances(context.TODO(), &ec2.DescribeInstancesInput{
		Filters: []types.Filter{
			{
				Name:   aws.String("tag:Name"),
				Values: []string{server.Name},
			},
			{
				Name:   aws.String("tag:Owner"),
				Values: []string{"onctl"},
			},
			{
				Name:   aws.String("instance-state-name"),
				Values: []string{"running"},
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

	result, err := p.Client.RunInstances(context.TODO(), input)
	if err != nil {
		printAwsError(err)
		return Vm{}, err
	}
	log.Printf("[DEBUG] %+v", result)
	waiter := ec2.NewInstanceRunningWaiter(p.Client)
	err = waiter.Wait(context.TODO(), &ec2.DescribeInstancesInput{
		InstanceIds: []string{*result.Instances[0].InstanceId},
	}, 10*time.Minute)
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
		InstanceIds: []string{
			server.ID,
		},
	}
	result, err := p.Client.TerminateInstances(context.TODO(), input)
	if err != nil {
		printAwsError(err)
		return err
	}
	log.Printf("[DEBUG] %+v", result)
	return nil
}

func (p ProviderAws) List() (VmList, error) {

	input := &ec2.DescribeInstancesInput{
		Filters: []types.Filter{
			{
				Name:   aws.String("tag:Owner"),
				Values: []string{"onctl"},
			},
		},
	}
	instances, err := p.Client.DescribeInstances(context.TODO(), input)
	if err != nil {
		printAwsError(err)
		return VmList{}, err
	}
	log.Printf("[DEBUG] %+v", instances)

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
	importKeyPairOutput, err := p.Client.ImportKeyPair(context.TODO(), &ec2.ImportKeyPairInput{
		PublicKeyMaterial: publicKey,
		KeyName:           aws.String("onctl-" + SSHKeyMD5[:8]),
	})
	if err != nil {
		var apiErr smithy.APIError
		if errors.As(err, &apiErr) {
			log.Println("[DEBUG] AWS Error: " + apiErr.ErrorCode())
			switch apiErr.ErrorCode() {
			case "InvalidKeyPair.Duplicate":
				log.Println("[DEBUG] SSH Key already exists")
				keyPair, err := p.Client.DescribeKeyPairs(context.TODO(), &ec2.DescribeKeyPairsInput{
					KeyNames: []string{"onctl-" + SSHKeyMD5[:8]},
				})
				if err != nil {
					log.Fatalln(err)
				}
				log.Println("[DEBUG] SSH Key ID: " + *keyPair.KeyPairs[0].KeyPairId)
				return *keyPair.KeyPairs[0].KeyPairId, nil
			default:
				fmt.Println(apiErr.Error())
			}
		} else {
			fmt.Println(err.Error())
		}
		log.Fatalln(err)
	}
	log.Printf("[DEBUG] %+v", importKeyPairOutput)
	return *importKeyPairOutput.KeyPairId, nil
}

func mapAwsServer(server types.Instance) Vm {
	var serverName = ""

	for _, tag := range server.Tags {
		if *tag.Key == "Name" {
			serverName = *tag.Value
		}
	}
	// log.Printf("[DEBUG] %+v", server)
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
		Type:      string(server.InstanceType),
		Status:    string(server.State.Name),
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
	s, err := p.Client.DescribeInstances(context.TODO(), &ec2.DescribeInstancesInput{
		Filters: []types.Filter{
			{
				Name:   aws.String("tag:Name"),
				Values: []string{serverName},
			},
			{
				Name:   aws.String("tag:Owner"),
				Values: []string{"onctl"},
			},
			{
				Name:   aws.String("instance-state-name"),
				Values: []string{"running"},
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

func (p ProviderAws) SSHInto(serverName string, port int, privateKey string, command []string) {

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
		Command:        command,
	})
}

// printAwsError prints an AWS API error, falling back to the raw error.
func printAwsError(err error) {
	var apiErr smithy.APIError
	if errors.As(err, &apiErr) {
		fmt.Println(apiErr.Error())
	} else {
		fmt.Println(err.Error())
	}
}

// Pause is not yet supported for AWS.
func (p ProviderAws) Pause(server Vm, hot bool) error {
	return fmt.Errorf("pause not supported yet for AWS (Hetzner only for now)")
}

// Resume is not yet supported for AWS.
func (p ProviderAws) Resume(server Vm) (Vm, error) {
	return Vm{}, fmt.Errorf("resume not supported yet for AWS (Hetzner only for now)")
}

// ListPaused returns empty for AWS (stopped instances appear in List).
func (p ProviderAws) ListPaused() (VmList, error) {
	return VmList{}, nil
}
