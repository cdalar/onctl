package cloud

import (
	"context"
	"log"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore/arm"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/to"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/compute/armcompute"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/network/armnetwork"
)

const (
	location = "westeurope"
)

type ProviderAzure struct {
	VmClient       *armcompute.VirtualMachinesClient
	NicClient      *armnetwork.InterfacesClient
	PublicIPClient *armnetwork.PublicIPAddressesClient
}

func (p ProviderAzure) List() (VmList, error) {
	pager := p.VmClient.NewListAllPager(&armcompute.VirtualMachinesClientListAllOptions{})
	resp, err := pager.NextPage(context.Background())
	if err != nil {
		log.Fatalf("failed to advance page: %v", err)
		return VmList{}, err
	}
	if len(resp.Value) == 0 {
		return VmList{}, nil
	}
	cloudList := make([]Vm, 0, len(resp.Value))
	for _, server := range resp.Value {
		var publicIP armnetwork.PublicIPAddressesClientGetResponse
		for _, nicRef := range server.Properties.NetworkProfile.NetworkInterfaces {
			nicID, _ := arm.ParseResourceID(*nicRef.ID)
			nic, _ := p.NicClient.Get(context.Background(), nicID.ResourceGroupName, nicID.Name, nil)
			for _, ipCfg := range nic.Properties.IPConfigurations {
				if ipCfg.Properties.PublicIPAddress != nil {
					publicID, _ := arm.ParseResourceID(*ipCfg.Properties.PublicIPAddress.ID)
					publicIP, err = p.PublicIPClient.Get(context.Background(), publicID.ResourceGroupName, publicID.Name, &armnetwork.PublicIPAddressesClientGetOptions{Expand: nil})
					if err != nil {
						log.Println(err)
					}
					log.Println("[DEBUG] public IP: ", *publicIP.Properties.IPAddress)
					// do something with public IP
				} else if ipCfg.Properties.PrivateIPAddress != nil {
					// do something with the private IP
					log.Println("[DEBUG] public IP: " + *ipCfg.Properties.PrivateIPAddress)
				}
			}
		}

		cloudList = append(cloudList, mapAzureServer(server, publicIP))
		log.Println("[DEBUG] server name: " + *server.Name)
	}
	output := VmList{
		List: cloudList,
	}
	return output, nil

}

func (p ProviderAzure) Create() (Vm, error) {
	log.Println("[DEBUG] Create Server")
	return Vm{}, nil
}

func (p ProviderAzure) Delete(server Vm) error {
	// p.Client.BeginDelete(context.Background(), server.Name, )
	log.Println("[DEBUG] Delete Server")
	return nil
}

func (p ProviderAzure) CreateSSHKey(string) (string, error) {
	log.Println("[DEBUG] Create SSH Key")
	return "", nil
}

func (p ProviderAzure) Deploy(Vm) (Vm, error) {
	log.Println("[DEBUG] Deploy Server")

	// resp, err := p.NicClient.BeginCreateOrUpdate(context.Background(), "onkube", "testabc", armnetwork.Interface{
	// 	Location: to.Ptr("westeurope"),
	// 	Properties: &armnetwork.InterfacePropertiesFormat{
	// 		IPConfigurations: []*armnetwork.InterfaceIPConfiguration{
	// 			{
	// 				Name: to.Ptr("ipConfig"),
	// 				Properties: &armnetwork.InterfaceIPConfigurationPropertiesFormat{
	// 					PrivateIPAllocationMethod: to.Ptr(armnetwork.IPAllocationMethodDynamic),
	// 					Subnet: &armnetwork.Subnet{
	// 						ID: to.Ptr("/subscriptions/3c110410-a29d-4402-96c4-f82b0feaa895/resourceGroups/onkube/providers/Microsoft.Network/virtualNetworks/onkube-vnet/subnets/default"),
	// 					},
	// 				},

	// 			}
	// 		}
	// }, nil)
	// if err != nil {
	// 	log.Fatalln(err)
	// }
	// log.Println("[DEBUG] ", resp)
	_, err := p.VmClient.BeginCreateOrUpdate(context.Background(), "onkube", "test", armcompute.VirtualMachine{
		Location: to.Ptr(location),
		Properties: &armcompute.VirtualMachineProperties{
			HardwareProfile: &armcompute.HardwareProfile{
				VMSize: to.Ptr(armcompute.VirtualMachineSizeTypes("Standard_B1s")),
			},
			StorageProfile: &armcompute.StorageProfile{
				ImageReference: &armcompute.ImageReference{
					Publisher: to.Ptr("canonical"),
					Offer:     to.Ptr("0001-com-ubuntu-server-jammy"),
					Version:   to.Ptr("latest"),
					SKU:       to.Ptr("22_04-lts-gen2"),
				},
			},
			NetworkProfile: &armcompute.NetworkProfile{
				NetworkInterfaces: []*armcompute.NetworkInterfaceReference{
					{
						ID: to.Ptr("/subscriptions/3c110410-a29d-4402-96c4-f82b0feaa895/resourceGroups/onkube/providers/Microsoft.Network/networkInterfaces/testabc"),
						Properties: &armcompute.NetworkInterfaceReferenceProperties{
							Primary: to.Ptr(true),
						},
					},
				},
			},
			OSProfile: &armcompute.OSProfile{},
		},
	}, nil)
	if err != nil {
		log.Fatalln(err)
	}
	return Vm{}, nil
}

func (p ProviderAzure) Destroy(Vm) error {
	log.Println("[DEBUG] Start Server")
	return nil
}

func mapAzureServer(server *armcompute.VirtualMachine, publicIP armnetwork.PublicIPAddressesClientGetResponse) Vm {
	// var serverName = ""

	// for _, tag := range server.Tags {
	// 	if *tag.Key == "Name" {
	// 		serverName = *tag.Value
	// 	}
	// }
	// // log.Println("[DEBUG] " + server.String())
	// if server.PublicIpAddress == nil {
	// 	server.PublicIpAddress = aws.String("")
	// }
	vm := Vm{
		ID:        *server.Properties.VMID,
		Name:      *server.Name,
		IP:        string(*publicIP.Properties.IPAddress),
		Type:      string(*server.Properties.HardwareProfile.VMSize),
		Status:    *server.Properties.ProvisioningState,
		CreatedAt: *server.Properties.TimeCreated,
	}
	log.Println("[DEBUG] ", vm)
	return vm
}

func (p ProviderAzure) SSHInto(serverName string) {
	// TODO: implement SSH into Azure VM
	// s := p.VmClient.(serverName)
	// log.Println("[DEBUG] " + s.String())
	// if s.ID == "" {
	// 	fmt.Println("Server not found")
	// }

	// ipAddress := s.IP
	// tools.SSHIntoVM(ipAddress, "ubuntu")
}

// func (p ProviderAzure) createNetWorkInterface(ctx context.Context, subnetID string, publicIPID string, networkSecurityGroupID string) (*armnetwork.Interface, error) {

// 	parameters := armnetwork.Interface{
// 		Location: to.Ptr(location),
// 		Properties: &armnetwork.InterfacePropertiesFormat{
// 			//NetworkSecurityGroup:
// 			IPConfigurations: []*armnetwork.InterfaceIPConfiguration{
// 				{
// 					Name: to.Ptr("ipConfig"),
// 					Properties: &armnetwork.InterfaceIPConfigurationPropertiesFormat{
// 						PrivateIPAllocationMethod: to.Ptr(armnetwork.IPAllocationMethodDynamic),
// 						Subnet: &armnetwork.Subnet{
// 							ID: to.Ptr(subnetID),
// 						},
// 						PublicIPAddress: &armnetwork.PublicIPAddress{
// 							ID: to.Ptr(publicIPID),
// 						},
// 					},
// 				},
// 			},
// 			NetworkSecurityGroup: &armnetwork.SecurityGroup{
// 				ID: to.Ptr(networkSecurityGroupID),
// 			},
// 		},
// 	}

// 	pollerResponse, err := p.NicClient.BeginCreateOrUpdate(ctx, "onkube", "nicName", parameters, nil)
// 	if err != nil {
// 		return nil, err
// 	}

// 	resp, err := pollerResponse.PollUntilDone(ctx, nil)
// 	if err != nil {
// 		return nil, err
// 	}

// 	return &resp.Interface, err
// }

// func createPublicIP(ctx context.Context) (*armnetwork.PublicIPAddress, error) {

// 	parameters := armnetwork.PublicIPAddress{
// 		Location: to.Ptr(location),
// 		Properties: &armnetwork.PublicIPAddressPropertiesFormat{
// 			PublicIPAllocationMethod: to.Ptr(armnetwork.IPAllocationMethodStatic), // Static or Dynamic
// 		},
// 	}

// 	pollerResponse, err := publicIPAddressesClient.BeginCreateOrUpdate(ctx, resourceGroupName, publicIPName, parameters, nil)
// 	if err != nil {
// 		return nil, err
// 	}

// 	resp, err := pollerResponse.PollUntilDone(ctx, nil)
// 	if err != nil {
// 		return nil, err
// 	}
// 	return &resp.PublicIPAddress, err
// }
