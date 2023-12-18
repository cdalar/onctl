package cloud

import (
	"context"
	"fmt"
	"log"

	"github.com/cdalar/onctl/internal/tools"

	"github.com/oracle/oci-go-sdk/common"
	"github.com/oracle/oci-go-sdk/core"
	"github.com/oracle/oci-go-sdk/example/helpers"
)

type ProviderOracle struct {
	Client core.ComputeClient
}

const (
	instanceShape      = "VM.Standard2.1"
	subnetDisplayName1 = "subnet1"
	vcnDisplayName     = "vcn1"
)

func (p ProviderOracle) Deploy(server Vm) (Vm, error) {
	c := p.Client
	ctx := context.Background()

	// create the launch instance request
	request := core.LaunchInstanceRequest{}
	request.CompartmentId = helpers.CompartmentID()
	request.DisplayName = common.String("OCI-Sample-Instance")
	request.AvailabilityDomain = helpers.AvailabilityDomain()

	// create a subnet or get the one already created
	subnet := CreateOrGetSubnet()
	fmt.Println("subnet created")
	request.CreateVnicDetails = &core.CreateVnicDetails{SubnetId: subnet.Id}

	// get a image
	image := listImages(ctx, c)[0]
	fmt.Println("list images")
	request.SourceDetails = core.InstanceSourceViaImageDetails{ImageId: image.Id}

	// use VM.Standard2.1 to create instance
	request.Shape = common.String(instanceShape)

	// default retry policy will retry on non-200 response
	request.RequestMetadata = helpers.GetRequestMetadataWithDefaultRetryPolicy()

	createResp, err := c.LaunchInstance(ctx, request)
	helpers.FatalIfError(err)
	log.Println(createResp.RawResponse)
	fmt.Println("launching instance")
	return Vm{}, nil
}
func (p ProviderOracle) Destroy(server Vm) error {
	return nil
}

func (p ProviderOracle) List() (VmList, error) {
	log.Println("[DEBUG] List Servers")
	return VmList{}, nil
}

func (p ProviderOracle) CreateSSHKey(publicKeyFile string) (keyID string, err error) {
	return
}

func (p ProviderOracle) SSHInto(serverName string) {

	ipAddress := "1.1.1.1"
	tools.SSHIntoVM(ipAddress, "root")
}

func CreateOrGetSubnet() core.Subnet {
	return CreateOrGetSubnetWithDetails(
		common.String(subnetDisplayName1),
		common.String("10.0.0.0/24"),
		common.String("subnetdns1"),
		helpers.AvailabilityDomain())
}

// CreateOrGetSubnetWithDetails either creates a new Virtual Cloud Network (VCN) or get the one already exist
// with detail info
func CreateOrGetSubnetWithDetails(displayName *string, cidrBlock *string, dnsLabel *string, availableDomain *string) core.Subnet {
	c, clerr := core.NewVirtualNetworkClientWithConfigurationProvider(common.DefaultConfigProvider())
	helpers.FatalIfError(clerr)
	ctx := context.Background()

	subnets := listSubnets(ctx, c)

	if displayName == nil {
		displayName = common.String(subnetDisplayName1)
	}

	// check if the subnet has already been created
	for _, element := range subnets {
		if *element.DisplayName == *displayName {
			// find the subnet, return it
			return element
		}
	}

	// create a new subnet
	request := core.CreateSubnetRequest{}
	request.AvailabilityDomain = availableDomain
	request.CompartmentId = helpers.CompartmentID()
	request.CidrBlock = cidrBlock
	request.DisplayName = displayName
	request.DnsLabel = dnsLabel
	request.RequestMetadata = helpers.GetRequestMetadataWithDefaultRetryPolicy()

	vcn := CreateOrGetVcn()
	request.VcnId = vcn.Id

	r, err := c.CreateSubnet(ctx, request)
	helpers.FatalIfError(err)

	// retry condition check, stop until return true
	pollUntilAvailable := func(r common.OCIOperationResponse) bool {
		if converted, ok := r.Response.(core.GetSubnetResponse); ok {
			return converted.LifecycleState != core.SubnetLifecycleStateAvailable
		}
		return true
	}

	pollGetRequest := core.GetSubnetRequest{
		SubnetId:        r.Id,
		RequestMetadata: helpers.GetRequestMetadataWithCustomizedRetryPolicy(pollUntilAvailable),
	}

	// wait for lifecyle become running
	_, pollErr := c.GetSubnet(ctx, pollGetRequest)
	helpers.FatalIfError(pollErr)

	// update the security rules
	getReq := core.GetSecurityListRequest{
		SecurityListId: common.String(r.SecurityListIds[0]),
	}

	getResp, err := c.GetSecurityList(ctx, getReq)
	helpers.FatalIfError(err)

	// this security rule allows remote control the instance
	portRange := core.PortRange{
		Max: common.Int(1521),
		Min: common.Int(1521),
	}

	newRules := append(getResp.IngressSecurityRules, core.IngressSecurityRule{
		Protocol: common.String("6"), // TCP
		Source:   common.String("0.0.0.0/0"),
		TcpOptions: &core.TcpOptions{
			DestinationPortRange: &portRange,
		},
	})

	updateReq := core.UpdateSecurityListRequest{
		SecurityListId: common.String(r.SecurityListIds[0]),
	}

	updateReq.IngressSecurityRules = newRules

	_, err = c.UpdateSecurityList(ctx, updateReq)
	helpers.FatalIfError(err)

	return r.Subnet
}

func listSubnets(ctx context.Context, c core.VirtualNetworkClient) []core.Subnet {
	vcn := CreateOrGetVcn()

	request := core.ListSubnetsRequest{
		CompartmentId: helpers.CompartmentID(),
		VcnId:         vcn.Id,
	}

	r, err := c.ListSubnets(ctx, request)
	helpers.FatalIfError(err)
	return r.Items
}

// ListImages lists the available images in the specified compartment.
func listImages(ctx context.Context, c core.ComputeClient) []core.Image {
	request := core.ListImagesRequest{
		CompartmentId:   helpers.CompartmentID(),
		OperatingSystem: common.String("Oracle Linux"),
		Shape:           common.String(instanceShape),
	}

	r, err := c.ListImages(ctx, request)
	helpers.FatalIfError(err)

	return r.Items
}

// CreateOrGetVcn either creates a new Virtual Cloud Network (VCN) or get the one already exist
func CreateOrGetVcn() core.Vcn {
	c, clerr := core.NewVirtualNetworkClientWithConfigurationProvider(common.DefaultConfigProvider())
	helpers.FatalIfError(clerr)
	ctx := context.Background()

	vcnItems := listVcns(ctx, c)

	for _, element := range vcnItems {
		if *element.DisplayName == vcnDisplayName {
			// VCN already created, return it
			return element
		}
	}

	// create a new VCN
	request := core.CreateVcnRequest{}
	request.CidrBlock = common.String("10.0.0.0/16")
	request.CompartmentId = helpers.CompartmentID()
	request.DisplayName = common.String(vcnDisplayName)
	request.DnsLabel = common.String("vcndns")

	r, err := c.CreateVcn(ctx, request)
	helpers.FatalIfError(err)
	return r.Vcn
}

func listVcns(ctx context.Context, c core.VirtualNetworkClient) []core.Vcn {
	request := core.ListVcnsRequest{
		CompartmentId: helpers.CompartmentID(),
	}

	r, err := c.ListVcns(ctx, request)
	helpers.FatalIfError(err)
	return r.Items
}
