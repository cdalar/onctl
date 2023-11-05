package cloud

import (
	"cdalar/onctl/internal/tools"
	"context"
	"fmt"
	"log"
	"os"
	"os/user"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore/arm"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/to"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/compute/armcompute"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/network/armnetwork"
	"github.com/spf13/viper"
)

type ProviderAzure struct {
	VmClient       *armcompute.VirtualMachinesClient
	NicClient      *armnetwork.InterfacesClient
	PublicIPClient *armnetwork.PublicIPAddressesClient
	SSHKeyClient   *armcompute.SSHPublicKeysClient
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

func (p ProviderAzure) CreateSSHKey(publicKeyFileName string) (string, error) {
	log.Println("[DEBUG] Create SSH Key")
	currentUser, err := user.Current()
	if err != nil {
		log.Fatalf(err.Error())
	}

	username := currentUser.Username
	sshPublicKeyData, err := os.ReadFile(publicKeyFileName)
	if err != nil {
		log.Println(err)
	}
	sshKey, err := p.SSHKeyClient.Create(context.Background(), viper.GetString("azure.resourceGroup"), username, armcompute.SSHPublicKeyResource{
		Properties: &armcompute.SSHPublicKeyResourceProperties{
			PublicKey: to.Ptr(string(sshPublicKeyData[:])),
		},
		Location: to.Ptr(viper.GetString("azure.location")),
	}, nil)
	if err != nil {
		log.Println(err)
	}
	return *sshKey.ID, err
}

func (p ProviderAzure) getSSHKeyPublicData() string {
	currentUser, err := user.Current()
	if err != nil {
		log.Fatalf(err.Error())
	}

	sshKey, err := p.SSHKeyClient.Get(context.Background(), viper.GetString("azure.resourceGroup"), currentUser.Username, nil)
	if err != nil {
		log.Println(err)
	}
	return *sshKey.Properties.PublicKey
}

func (p ProviderAzure) Deploy(server Vm) (Vm, error) {
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
	poller, err := p.VmClient.BeginCreateOrUpdate(context.Background(), viper.GetString("azure.resourceGroup"), viper.GetString("vm.name"), armcompute.VirtualMachine{
		Location: to.Ptr(viper.GetString("azure.location")),
		Properties: &armcompute.VirtualMachineProperties{
			HardwareProfile: &armcompute.HardwareProfile{
				VMSize: to.Ptr(armcompute.VirtualMachineSizeTypes(viper.GetString("azure.vm.type"))),
			},
			StorageProfile: &armcompute.StorageProfile{
				ImageReference: &armcompute.ImageReference{
					Publisher: to.Ptr(viper.GetString("azure.vm.image.publisher")),
					Offer:     to.Ptr(viper.GetString("azure.vm.image.offer")),
					Version:   to.Ptr(viper.GetString("azure.vm.image.version")),
					SKU:       to.Ptr(viper.GetString("azure.vm.image.sku")),
				},
			},
			NetworkProfile: &armcompute.NetworkProfile{
				NetworkInterfaces: []*armcompute.NetworkInterfaceReference{
					{
						ID: to.Ptr("/subscriptions/3c110410-a29d-4402-96c4-f82b0feaa895/resourceGroups/onkube/providers/Microsoft.Network/networkInterfaces/myVMVMNic"),
						Properties: &armcompute.NetworkInterfaceReferenceProperties{
							Primary: to.Ptr(true),
						},
					},
				},
			},
			OSProfile: &armcompute.OSProfile{
				ComputerName:  to.Ptr(tools.GenerateMachineUniqueName()),
				AdminUsername: to.Ptr(viper.GetString("azure.vm.username")),
				LinuxConfiguration: &armcompute.LinuxConfiguration{
					DisablePasswordAuthentication: to.Ptr(true),
					SSH: &armcompute.SSHConfiguration{
						PublicKeys: []*armcompute.SSHPublicKey{
							{
								KeyData: to.Ptr(p.getSSHKeyPublicData()),
								Path:    to.Ptr("/home/" + viper.GetString("azure.vm.username") + "/.ssh/authorized_keys"),
							},
						},
					},
				},
			},
		},
	}, nil)
	if err != nil {
		log.Fatalln(err)
	}
	resp, err := poller.PollUntilDone(context.Background(), nil)
	return Vm{
		ID:   *resp.VirtualMachine.Properties.VMID,
		Name: *resp.VirtualMachine.Name,
		// IP:        string(*publicIP.Properties.IPAddress),
		Type:      string(*resp.VirtualMachine.Properties.HardwareProfile.VMSize),
		Status:    *resp.VirtualMachine.Properties.ProvisioningState,
		CreatedAt: *resp.VirtualMachine.Properties.TimeCreated,
	}, err
}

func (p ProviderAzure) Destroy(server Vm) error {
	log.Println("[DEBUG] Destroy Server")
	resp, err := p.VmClient.BeginDelete(context.Background(), viper.GetString("azure.resourceGroup"), server.ID, nil)
	if err != nil {
		log.Fatalln(err)
	}
	_, err = resp.PollUntilDone(context.Background(), nil)
	return err
}

func mapAzureServer(server *armcompute.VirtualMachine, publicIP armnetwork.PublicIPAddressesClientGetResponse) Vm {
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
	s := p.getServerByServerName(serverName)
	log.Println("[DEBUG] " + s.String())
	if s.ID == "" {
		fmt.Println("Server not found")
	}

	tools.SSHIntoVM(s.IP, "azureuser")
}

func (p ProviderAzure) getServerByServerName(serverName string) Vm {
	vmList, err := p.List()
	if err != nil {
		log.Println(err)
	}
	for _, vm := range vmList.List {
		if vm.Name == serverName {
			return vm
		}
	}
	return Vm{}
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
