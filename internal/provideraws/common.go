package provideraws

import (
	"fmt"

	"cdalar/onctl/internal/rand"
	"cdalar/onctl/internal/tools"
	"log"
	"os"
	"path/filepath"
	"strings"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/awserr"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/ec2"
)

func SetDefaultRouteToMainRouteTable(svc *ec2.EC2, routeTableId *string, internetGatewayId *string) {

	input := &ec2.CreateRouteInput{
		DestinationCidrBlock: aws.String("0.0.0.0/0"), // Required
		RouteTableId:         routeTableId,            // Required
		GatewayId:            internetGatewayId,
	}

	_, err := svc.CreateRoute(input)
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
	}
}

func DefaultRouteTable(svc *ec2.EC2, vpcId *string) *string {

	input := &ec2.DescribeRouteTablesInput{
		Filters: []*ec2.Filter{
			{
				Name:   aws.String("vpc-id"),
				Values: []*string{vpcId},
			},
		},
	}
	result, err := svc.DescribeRouteTables(input)
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
	}
	return result.RouteTables[0].RouteTableId
}

// CreateKeyPair creates a keypair and saves it to a file
// Returns the name of the keypair and error if any
func CreateKeyPair(svc *ec2.EC2) (string, error) {

	var ret string
	home, err := tools.CreateConfigDirIfNotExist()
	if err != nil {
		log.Println(err)
	}
	files, err := os.ReadDir(home)
	if err != nil {
		log.Println(err)
	}

	//check if keypair file already exists
	var keyPairIndex int = -1
	for k, f := range files {
		// log.Println(f.Name())
		if len(f.Name()) > 10 && f.Name()[0:7] == "onkube-" {
			keyPairName := strings.TrimSuffix(filepath.Base(f.Name()), filepath.Ext(f.Name()))
			keyPairExists := checkIfKeyPairExists(svc, keyPairName)
			if keyPairExists {
				keyPairIndex = k
				break
			}
		}
	}
	log.Println("keyPairIndex: ", keyPairIndex)
	if keyPairIndex != -1 {
		ret = home + "/" + files[keyPairIndex].Name()
	}

	if keyPairIndex == -1 {
		// If keypair does not exist, create it
		// Create a key pair with the specified name.
		randomString := rand.String(6)
		keyPairName := "onkube-kp-" + randomString
		input := &ec2.CreateKeyPairInput{
			KeyName: aws.String(keyPairName), // Required
			TagSpecifications: []*ec2.TagSpecification{
				{ResourceType: aws.String("key-pair"),
					Tags: []*ec2.Tag{
						{
							Key:   aws.String("Name"),
							Value: aws.String(keyPairName),
						},
					},
				},
			},
		}

		result, err := svc.CreateKeyPair(input)
		if err != nil {
			log.Println(err)
		}

		file, err := os.Create(home + "/" + keyPairName + ".pem")
		if err != nil {
			log.Fatal("Cannot create file", err)
		}

		_, err = file.WriteString(*result.KeyMaterial)
		if err != nil {
			log.Println(err)
		}
		err = os.Chmod(home+"/"+keyPairName+".pem", 0400)
		if err != nil {
			log.Fatal("Cannot change file permissions", err)
		}
		log.Println("KeyPair created: ", *result.KeyName)
		ret = home + "/" + *result.KeyName + ".pem"
	}
	return ret, nil
}

func CreateSecurityGroupSSH(svc *ec2.EC2, vpcId *string) *string {

	sgs, err := svc.DescribeSecurityGroups(&ec2.DescribeSecurityGroupsInput{
		Filters: []*ec2.Filter{
			{Name: aws.String("tag:Name"), Values: []*string{aws.String("onkube-sg-ssh")}}},
	})
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
	}

	if len(sgs.SecurityGroups) > 0 {
		log.Println("Security Group already exists for SSH")
		return sgs.SecurityGroups[0].GroupId
	} else {
		log.Println("Creating Security Group...")
		input := &ec2.CreateSecurityGroupInput{
			Description: aws.String("onkube-sg-ssh"), // Required
			GroupName:   aws.String("onkube-sg-ssh"), // Required
			VpcId:       vpcId,                       // Required
			TagSpecifications: []*ec2.TagSpecification{
				{ResourceType: aws.String("security-group"), Tags: []*ec2.Tag{{
					Key: aws.String("Name"), Value: aws.String("onkube-sg-ssh")}}},
			},
		}
		result, err := svc.CreateSecurityGroup(input)
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
		}
		_, err = svc.AuthorizeSecurityGroupIngress(&ec2.AuthorizeSecurityGroupIngressInput{
			GroupId:    result.GroupId,
			IpProtocol: aws.String("tcp"),
			FromPort:   aws.Int64(22),
			ToPort:     aws.Int64(22),
			CidrIp:     aws.String("0.0.0.0/0"),
		})
		if err != nil {
			log.Println(err)
		}
		// _, err = svc.AuthorizeSecurityGroupIngress(&ec2.AuthorizeSecurityGroupIngressInput{
		// 	GroupId:    result.GroupId,
		// 	IpProtocol: aws.String("tcp"),
		// 	FromPort:   aws.Int64(80),
		// 	ToPort:     aws.Int64(80),
		// 	CidrIp:     aws.String("0.0.0.0/0"),
		// })
		// if err != nil {
		// 	log.Println(err)
		// }

		log.Println("Security Group created: ", *result.GroupId)
		return result.GroupId
	}
}

func getAvailabilityZones(svc *ec2.EC2) []string {
	input := &ec2.DescribeAvailabilityZonesInput{}

	result, err := svc.DescribeAvailabilityZones(input)
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
		return nil
	}
	var zones []string
	for _, zone := range result.AvailabilityZones {
		zones = append(zones, *zone.ZoneName)
	}
	return zones
}

func createSubnets(svc *ec2.EC2, vpcId string) []string {

	log.Println("Creating subnets...")
	var subnets = []string{"10.174.0.0/20", "10.174.16.0/20", "10.174.32.0/20"}
	subnetsAz := getAvailabilityZones(svc)
	var subnetIds []string
	for k, v := range subnets {

		input := &ec2.CreateSubnetInput{
			CidrBlock:        aws.String(v),     // Required
			VpcId:            aws.String(vpcId), // Required
			AvailabilityZone: aws.String(subnetsAz[k]),
			TagSpecifications: []*ec2.TagSpecification{
				{ResourceType: aws.String("subnet"), Tags: []*ec2.Tag{{
					Key: aws.String("Name"), Value: aws.String("onkube-subnet-" + subnetsAz[k])}}},
			}}
		subnet, err := svc.CreateSubnet(input)
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
		}
		subnetIds = append(subnetIds, *subnet.Subnet.SubnetId)
	}
	log.Println("Subnets created: ", subnetIds)
	return subnetIds
}

func AttachInternetGateway(svc *ec2.EC2, vpcId *string, internetGatewayId *string) {

	igws, err := svc.DescribeInternetGateways(&ec2.DescribeInternetGatewaysInput{
		Filters: []*ec2.Filter{
			{Name: aws.String("tag:Name"), Values: []*string{aws.String("onkube-internet-gateway")}}},
	})
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
	}
	if len(igws.InternetGateways) > 0 {
		if len(igws.InternetGateways[0].Attachments) > 0 {
			if *igws.InternetGateways[0].Attachments[0].State == "available" {
				log.Println("InternetGateway already attached")
				return
			}
		}
	}

	input := &ec2.AttachInternetGatewayInput{
		InternetGatewayId: internetGatewayId, // Required
		VpcId:             vpcId,             // Required
	}
	_, err = svc.AttachInternetGateway(input)
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
	}
}

func CreateInternetGateway(svc *ec2.EC2) *string {

	igws_input := &ec2.DescribeInternetGatewaysInput{
		Filters: []*ec2.Filter{
			{Name: aws.String("tag:Name"), Values: []*string{aws.String("onkube-internet-gateway")}}},
	}

	igws, err := svc.DescribeInternetGateways(igws_input)
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
	}

	if len(igws.InternetGateways) > 0 {
		log.Println("InternetGateway found. using it...")
		return igws.InternetGateways[0].InternetGatewayId
	}

	log.Println("Creating InternetGateway...")
	input := &ec2.CreateInternetGatewayInput{
		TagSpecifications: []*ec2.TagSpecification{
			{ResourceType: aws.String("internet-gateway"), Tags: []*ec2.Tag{{
				Key: aws.String("Name"), Value: aws.String("onkube-internet-gateway")}}},
		},
	}
	internetGateway, err := svc.CreateInternetGateway(input)
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
	}
	log.Println("InternetGateway created: " + *internetGateway.InternetGateway.InternetGatewayId)
	return internetGateway.InternetGateway.InternetGatewayId
}

func createVpc(svc *ec2.EC2) *string {
	input := &ec2.CreateVpcInput{
		CidrBlock: aws.String("10.174.0.0/16"), // Required
		TagSpecifications: []*ec2.TagSpecification{
			{Tags: []*ec2.Tag{
				{Key: aws.String("Name"), Value: aws.String("onkube-vpc")}},
				ResourceType: aws.String("vpc"),
			},
		},
	}
	log.Println("Creating VPC...")
	vpc, err := svc.CreateVpc(input)
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
	}
	log.Println("VPC created: ", *vpc.Vpc.VpcId)
	return vpc.Vpc.VpcId
}

// vpcId, subnetId
func CreateVpcAndSubnet(svc *ec2.EC2) (*string, []string) {

	var vpcId *string
	var subnetIds []string

	vpcs := tools.GetVpcs(svc)
	if len(vpcs.Vpcs) == 0 {
		log.Println("No VPC found")
		vpcId = createVpc(svc)
		subnetIds = createSubnets(svc, *vpcId)
	} else {
		log.Println("VPC found, using it...")
		vpcId = vpcs.Vpcs[0].VpcId
		subnets := tools.GetSubnets(svc, vpcId)
		for _, subnet := range subnets {
			subnetIds = append(subnetIds, *subnet.SubnetId)
		}
	}
	return vpcId, subnetIds
}

func GetClient() *ec2.EC2 {
	sess, err := session.NewSessionWithOptions(session.Options{
		SharedConfigState: session.SharedConfigEnable,
	})
	if err != nil {
		log.Println(err)
	}
	return ec2.New(sess)
}

func GetImages() ([]*ec2.Image, error) {
	svc := GetClient()
	input := &ec2.DescribeImagesInput{
		Filters: []*ec2.Filter{
			{
				Name: aws.String("owner-alias"),
				Values: []*string{
					aws.String("amazon"),
				},
			},
			{
				Name: aws.String("name"),
				Values: []*string{
					aws.String("ubuntu/images/hvm-ssd/ubuntu-jammy-22.04-amd64-server-20230208"),
				},
			},
		},
	}

	result, err := svc.DescribeImages(input)
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
		return nil, err
	}
	return result.Images, nil
}

func AddSecurityGroupToInstance(svc *ec2.EC2, instanceId *string, securityGroupId *string) {
	instace := DescribeInstance(*instanceId)
	sgs := make([]*string, 0, 5)
	sgs = append(sgs, instace.SecurityGroups[0].GroupId)
	sgs = append(sgs, securityGroupId)
	input := &ec2.ModifyInstanceAttributeInput{
		Groups:     sgs,
		InstanceId: instanceId,
	}
	_, err := svc.ModifyInstanceAttribute(input)
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
	}
}

// CreateSecurityGroupForPort creates a security group for a given port
// and returns the security group id
func CreateSecurityGroupForPort(svc *ec2.EC2, vpcId *string, port int64) (groupId *string) {
	securityGroups := tools.GetSecurityGroups(svc, vpcId)
	for _, v := range securityGroups {
		if *v.GroupName == "onkube-sg-"+fmt.Sprint(port) {
			log.Println("Security Group already exists for port:", port)
			return v.GroupId
		}
	}

	input := &ec2.CreateSecurityGroupInput{
		Description: aws.String("onkube security group for port " + fmt.Sprint(port)),
		GroupName:   aws.String("onkube-sg-" + fmt.Sprint(port)),
		VpcId:       vpcId,
		TagSpecifications: []*ec2.TagSpecification{
			{ResourceType: aws.String("security-group"), Tags: []*ec2.Tag{{
				Key: aws.String("Name"), Value: aws.String("onkube-sg-" + fmt.Sprint(port))}}},
		},
	}
	result, err := svc.CreateSecurityGroup(input)
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
	}
	_, err = svc.AuthorizeSecurityGroupIngress(&ec2.AuthorizeSecurityGroupIngressInput{
		GroupId:    result.GroupId,
		IpProtocol: aws.String("tcp"),
		FromPort:   aws.Int64(port),
		ToPort:     aws.Int64(port),
		CidrIp:     aws.String("0.0.0.0/0"),
	})
	if err != nil {
		log.Println(err)
	}
	return result.GroupId
}

func DescribeInstance(instanceId string) *ec2.Instance {
	svc := GetClient()

	input := &ec2.DescribeInstancesInput{
		Filters: []*ec2.Filter{
			{
				Name:   aws.String("instance-id"),
				Values: []*string{aws.String(instanceId)},
			},
		},
	}
	instances, err := svc.DescribeInstances(input)
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
		return nil
	}
	// log.Println(instances)
	return instances.Reservations[0].Instances[0]
}

func checkIfKeyPairExists(svc *ec2.EC2, keyName string) bool {
	input := &ec2.DescribeKeyPairsInput{
		KeyNames: []*string{
			aws.String(keyName),
		},
	}
	result, err := svc.DescribeKeyPairs(input)
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
		return false
	}
	return len(result.KeyPairs) > 0
}

func ImportKeyPair(svc *ec2.EC2, keyName string, publicKeyFile string) {
	if checkIfKeyPairExists(svc, keyName) {
		log.Println("Key pair already exists")
		return
	}
	publicKey, err := os.ReadFile(publicKeyFile)
	if err != nil {
		log.Println(err)
	}
	log.Println(publicKey)
	input := &ec2.ImportKeyPairInput{
		KeyName:           aws.String(keyName),
		PublicKeyMaterial: []byte(publicKey),
	}
	result, err := svc.ImportKeyPair(input)
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
		return
	}
	log.Println(result)
}

func GetDefaultVpcId(svc *ec2.EC2) (vpcId *string) {
	input := &ec2.DescribeVpcsInput{
		Filters: []*ec2.Filter{
			{
				Name:   aws.String("is-default"),
				Values: []*string{aws.String("true")},
			},
		},
	}
	result, err := svc.DescribeVpcs(input)
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
		return nil
	}
	return result.Vpcs[0].VpcId
}
