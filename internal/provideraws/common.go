package provideraws

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"sort"
	"strings"

	"github.com/cdalar/onctl/internal/tools"
	"github.com/spf13/viper"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	"github.com/aws/aws-sdk-go-v2/service/ec2/types"
	"github.com/aws/smithy-go"
)

// printAwsError prints an AWS API error, falling back to the raw error.
func printAwsError(err error) {
	var apiErr smithy.APIError
	if errors.As(err, &apiErr) {
		fmt.Println(apiErr.Error())
	} else {
		fmt.Println(err.Error())
	}
}

func SetDefaultRouteToMainRouteTable(svc *ec2.Client, routeTableId *string, internetGatewayId *string) {

	input := &ec2.CreateRouteInput{
		DestinationCidrBlock: aws.String("0.0.0.0/0"), // Required
		RouteTableId:         routeTableId,            // Required
		GatewayId:            internetGatewayId,
	}

	_, err := svc.CreateRoute(context.TODO(), input)
	if err != nil {
		printAwsError(err)
	}
}

func DefaultRouteTable(svc *ec2.Client, vpcId *string) *string {

	input := &ec2.DescribeRouteTablesInput{
		Filters: []types.Filter{
			{
				Name:   aws.String("vpc-id"),
				Values: []string{*vpcId},
			},
		},
	}
	result, err := svc.DescribeRouteTables(context.TODO(), input)
	if err != nil {
		printAwsError(err)
	}
	return result.RouteTables[0].RouteTableId
}

func CreateSecurityGroupSSH(svc *ec2.Client, vpcId *string) *string {

	sgs, err := svc.DescribeSecurityGroups(context.TODO(), &ec2.DescribeSecurityGroupsInput{
		Filters: []types.Filter{
			{Name: aws.String("tag:Name"), Values: []string{"onkube-sg-ssh"}}},
	})
	if err != nil {
		printAwsError(err)
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
			TagSpecifications: []types.TagSpecification{
				{ResourceType: types.ResourceTypeSecurityGroup, Tags: []types.Tag{{
					Key: aws.String("Name"), Value: aws.String("onkube-sg-ssh")}}},
			},
		}
		result, err := svc.CreateSecurityGroup(context.TODO(), input)
		if err != nil {
			printAwsError(err)
		}
		_, err = svc.AuthorizeSecurityGroupIngress(context.TODO(), &ec2.AuthorizeSecurityGroupIngressInput{
			GroupId:    result.GroupId,
			IpProtocol: aws.String("tcp"),
			FromPort:   aws.Int32(22),
			ToPort:     aws.Int32(22),
			CidrIp:     aws.String("0.0.0.0/0"),
		})
		if err != nil {
			log.Println(err)
		}
		// _, err = svc.AuthorizeSecurityGroupIngress(context.TODO(), &ec2.AuthorizeSecurityGroupIngressInput{
		// 	GroupId:    result.GroupId,
		// 	IpProtocol: aws.String("tcp"),
		// 	FromPort:   aws.Int32(80),
		// 	ToPort:     aws.Int32(80),
		// 	CidrIp:     aws.String("0.0.0.0/0"),
		// })
		// if err != nil {
		// 	log.Println(err)
		// }

		log.Println("Security Group created: ", *result.GroupId)
		return result.GroupId
	}
}

func getAvailabilityZones(svc *ec2.Client) []string {
	input := &ec2.DescribeAvailabilityZonesInput{}

	result, err := svc.DescribeAvailabilityZones(context.TODO(), input)
	if err != nil {
		printAwsError(err)
		return nil
	}
	var zones []string
	for _, zone := range result.AvailabilityZones {
		zones = append(zones, *zone.ZoneName)
	}
	return zones
}

func createSubnets(svc *ec2.Client, vpcId string) []string {

	log.Println("Creating subnets...")
	var subnets = []string{"10.174.0.0/20", "10.174.16.0/20", "10.174.32.0/20"}
	subnetsAz := getAvailabilityZones(svc)
	var subnetIds []string
	for k, v := range subnets {

		input := &ec2.CreateSubnetInput{
			CidrBlock:        aws.String(v),     // Required
			VpcId:            aws.String(vpcId), // Required
			AvailabilityZone: aws.String(subnetsAz[k]),
			TagSpecifications: []types.TagSpecification{
				{ResourceType: types.ResourceTypeSubnet, Tags: []types.Tag{{
					Key: aws.String("Name"), Value: aws.String("onkube-subnet-" + subnetsAz[k])}}},
			}}
		subnet, err := svc.CreateSubnet(context.TODO(), input)
		if err != nil {
			printAwsError(err)
		}
		subnetIds = append(subnetIds, *subnet.Subnet.SubnetId)
	}
	log.Println("Subnets created: ", subnetIds)
	return subnetIds
}

func AttachInternetGateway(svc *ec2.Client, vpcId *string, internetGatewayId *string) {

	igws, err := svc.DescribeInternetGateways(context.TODO(), &ec2.DescribeInternetGatewaysInput{
		Filters: []types.Filter{
			{Name: aws.String("tag:Name"), Values: []string{"onkube-internet-gateway"}}},
	})
	if err != nil {
		printAwsError(err)
	}
	if len(igws.InternetGateways) > 0 {
		if len(igws.InternetGateways[0].Attachments) > 0 {
			if string(igws.InternetGateways[0].Attachments[0].State) == "available" {
				log.Println("InternetGateway already attached")
				return
			}
		}
	}

	input := &ec2.AttachInternetGatewayInput{
		InternetGatewayId: internetGatewayId, // Required
		VpcId:             vpcId,             // Required
	}
	_, err = svc.AttachInternetGateway(context.TODO(), input)
	if err != nil {
		printAwsError(err)
	}
}

func CreateInternetGateway(svc *ec2.Client) *string {

	igws_input := &ec2.DescribeInternetGatewaysInput{
		Filters: []types.Filter{
			{Name: aws.String("tag:Name"), Values: []string{"onkube-internet-gateway"}}},
	}

	igws, err := svc.DescribeInternetGateways(context.TODO(), igws_input)
	if err != nil {
		printAwsError(err)
	}

	if len(igws.InternetGateways) > 0 {
		log.Println("InternetGateway found. using it...")
		return igws.InternetGateways[0].InternetGatewayId
	}

	log.Println("Creating InternetGateway...")
	input := &ec2.CreateInternetGatewayInput{
		TagSpecifications: []types.TagSpecification{
			{ResourceType: types.ResourceTypeInternetGateway, Tags: []types.Tag{{
				Key: aws.String("Name"), Value: aws.String("onkube-internet-gateway")}}},
		},
	}
	internetGateway, err := svc.CreateInternetGateway(context.TODO(), input)
	if err != nil {
		printAwsError(err)
	}
	log.Println("InternetGateway created: " + *internetGateway.InternetGateway.InternetGatewayId)
	return internetGateway.InternetGateway.InternetGatewayId
}

func createVpc(svc *ec2.Client) *string {
	input := &ec2.CreateVpcInput{
		CidrBlock: aws.String("10.174.0.0/16"), // Required
		TagSpecifications: []types.TagSpecification{
			{Tags: []types.Tag{
				{Key: aws.String("Name"), Value: aws.String("onkube-vpc")}},
				ResourceType: types.ResourceTypeVpc,
			},
		},
	}
	log.Println("Creating VPC...")
	vpc, err := svc.CreateVpc(context.TODO(), input)
	if err != nil {
		printAwsError(err)
	}
	log.Println("VPC created: ", *vpc.Vpc.VpcId)
	return vpc.Vpc.VpcId
}

// vpcId, subnetId
func CreateVpcAndSubnet(svc *ec2.Client) (*string, []string) {

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

func GetClient() *ec2.Client {
	cfg, err := config.LoadDefaultConfig(context.TODO(),
		config.WithRegion(viper.GetString("aws.location")))
	if err != nil {
		log.Println(err)
	}
	return ec2.NewFromConfig(cfg)
}

func GetImages() ([]types.Image, error) {
	svc := GetClient()

	// Use configured image name, or fallback to a pattern for latest Ubuntu
	imageName := viper.GetString("aws.vm.image")
	if imageName == "" {
		imageName = "ubuntu/images/hvm-ssd/ubuntu-jammy-22.04-amd64-server-*"
	}

	input := &ec2.DescribeImagesInput{
		Owners: []string{"amazon"},
		Filters: []types.Filter{
			{
				Name:   aws.String("name"),
				Values: []string{imageName},
			},
			{
				Name:   aws.String("state"),
				Values: []string{"available"},
			},
		},
	}

	result, err := svc.DescribeImages(context.TODO(), input)
	if err != nil {
		printAwsError(err)
		return nil, err
	}

	// Sort images by creation date (newest first) if using wildcard
	if strings.Contains(imageName, "*") && len(result.Images) > 0 {
		sort.Slice(result.Images, func(i, j int) bool {
			switch {
			case result.Images[i].CreationDate == nil && result.Images[j].CreationDate != nil:
				return false // i is older, push to end
			case result.Images[i].CreationDate != nil && result.Images[j].CreationDate == nil:
				return true // i is newer, keep at front
			case result.Images[i].CreationDate == nil && result.Images[j].CreationDate == nil:
				return false // equal, keep order
			default:
				return *result.Images[i].CreationDate > *result.Images[j].CreationDate
			}
		})
	}

	return result.Images, nil
}

func AddSecurityGroupToInstance(svc *ec2.Client, instanceId *string, securityGroupId *string) {
	instace := DescribeInstance(*instanceId)
	sgs := make([]string, 0, 5)
	sgs = append(sgs, *instace.SecurityGroups[0].GroupId)
	sgs = append(sgs, *securityGroupId)
	input := &ec2.ModifyInstanceAttributeInput{
		Groups:     sgs,
		InstanceId: instanceId,
	}
	_, err := svc.ModifyInstanceAttribute(context.TODO(), input)
	if err != nil {
		printAwsError(err)
	}
}

// CreateSecurityGroupForPort creates a security group for a given port
// and returns the security group id
func CreateSecurityGroupForPort(svc *ec2.Client, vpcId *string, port int64) (groupId *string) {
	securityGroups := GetSecurityGroups(svc, vpcId)
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
		TagSpecifications: []types.TagSpecification{
			{ResourceType: types.ResourceTypeSecurityGroup, Tags: []types.Tag{{
				Key: aws.String("Name"), Value: aws.String("onkube-sg-" + fmt.Sprint(port))}}},
		},
	}
	result, err := svc.CreateSecurityGroup(context.TODO(), input)
	if err != nil {
		printAwsError(err)
	}
	_, err = svc.AuthorizeSecurityGroupIngress(context.TODO(), &ec2.AuthorizeSecurityGroupIngressInput{
		GroupId:    result.GroupId,
		IpProtocol: aws.String("tcp"),
		FromPort:   aws.Int32(int32(port)),
		ToPort:     aws.Int32(int32(port)),
		CidrIp:     aws.String("0.0.0.0/0"),
	})
	if err != nil {
		log.Println(err)
	}
	return result.GroupId
}

func DescribeInstance(instanceId string) types.Instance {
	svc := GetClient()

	input := &ec2.DescribeInstancesInput{
		Filters: []types.Filter{
			{
				Name:   aws.String("instance-id"),
				Values: []string{instanceId},
			},
		},
	}
	instances, err := svc.DescribeInstances(context.TODO(), input)
	if err != nil {
		printAwsError(err)
		return types.Instance{}
	}
	// log.Println(instances)
	return instances.Reservations[0].Instances[0]
}

func checkIfKeyPairExists(svc *ec2.Client, keyName string) bool {
	input := &ec2.DescribeKeyPairsInput{
		KeyNames: []string{
			keyName,
		},
	}
	result, err := svc.DescribeKeyPairs(context.TODO(), input)
	if err != nil {
		printAwsError(err)
		return false
	}
	return len(result.KeyPairs) > 0
}

func ImportKeyPair(svc *ec2.Client, keyName string, publicKeyFile string) {
	if checkIfKeyPairExists(svc, keyName) {
		log.Println("Key pair already exists")
		return
	}
	publicKey, err := os.ReadFile(publicKeyFile)
	if err != nil {
		log.Println(err)
	}
	log.Println(string(publicKey))
	input := &ec2.ImportKeyPairInput{
		KeyName:           aws.String(keyName),
		PublicKeyMaterial: []byte(publicKey),
	}
	result, err := svc.ImportKeyPair(context.TODO(), input)
	if err != nil {
		printAwsError(err)
		return
	}
	log.Println(result)
}

func GetDefaultVpcId(svc *ec2.Client) (vpcId *string) {
	input := &ec2.DescribeVpcsInput{
		Filters: []types.Filter{
			{
				Name:   aws.String("is-default"),
				Values: []string{"true"},
			},
		},
	}
	result, err := svc.DescribeVpcs(context.TODO(), input)
	if err != nil {
		printAwsError(err)
		return nil
	}
	return result.Vpcs[0].VpcId
}

func GetSecurityGroups(svc *ec2.Client, vpcId *string) []types.SecurityGroup {
	sgs, err := svc.DescribeSecurityGroups(context.TODO(), &ec2.DescribeSecurityGroupsInput{
		Filters: []types.Filter{
			{
				Name:   aws.String("vpc-id"),
				Values: []string{*vpcId},
			},
		},
	})
	if err != nil {
		printAwsError(err)
	}
	return sgs.SecurityGroups
}

func GetSecurityGroupByName(svc *ec2.Client, name string) []types.SecurityGroup {
	sgs, err := svc.DescribeSecurityGroups(context.TODO(), &ec2.DescribeSecurityGroupsInput{
		Filters: []types.Filter{
			{
				Name:   aws.String("group-name"),
				Values: []string{name},
			},
		},
	})
	if err != nil {
		printAwsError(err)
	}
	return sgs.SecurityGroups
}
