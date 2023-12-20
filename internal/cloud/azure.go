package cloud

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/user"

	"github.com/cdalar/onctl/internal/tools"

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
	VnetClient     *armnetwork.VirtualNetworksClient
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
	log.Println("[DEBUG] ", currentUser)
	if err != nil {
		log.Fatalf(err.Error())
	}

	sshKey, err := p.SSHKeyClient.Get(context.Background(), viper.GetString("azure.resourceGroup"), currentUser.Username, nil)
	if err != nil {
		log.Println(err)
	}
	log.Println("[DEBUG] ", sshKey)
	return *sshKey.Properties.PublicKey
}

func (p ProviderAzure) Deploy(server Vm) (Vm, error) {
	log.Println("[DEBUG] Deploy Server")

	vnet, err := createVirtualNetwork(context.Background(), &p)
	if err != nil {
		log.Fatalln(err)
	}
	log.Println("[DEBUG] ", vnet)
	pip, err := createPublicIP(context.Background(), &p)
	if err != nil {
		log.Fatalln(err)
	}
	log.Println("[DEBUG] ", pip)
	nicResp, err := p.NicClient.BeginCreateOrUpdate(context.Background(), viper.GetString("azure.resourceGroup"), viper.GetString("azure.vm.nic.name"), armnetwork.Interface{
		Location: to.Ptr(viper.GetString("azure.location")),
		Properties: &armnetwork.InterfacePropertiesFormat{
			IPConfigurations: []*armnetwork.InterfaceIPConfiguration{
				{
					Name: to.Ptr("ipConfig"),
					Properties: &armnetwork.InterfaceIPConfigurationPropertiesFormat{
						PrivateIPAllocationMethod: to.Ptr(armnetwork.IPAllocationMethodDynamic),
						Subnet: &armnetwork.Subnet{
							ID: vnet.Properties.Subnets[0].ID,
						},
						PublicIPAddress: &armnetwork.PublicIPAddress{
							ID: pip.ID,
						},
					},
				},
			},
		},
	}, nil)
	if err != nil {
		log.Fatalln(err)
	}
	nicRespDone, err := nicResp.PollUntilDone(context.Background(), nil)
	if err != nil {
		log.Fatalln(err)
	}
	log.Println("[DEBUG] ", nicRespDone)
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
						ID: nicRespDone.Interface.ID,
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

func createVirtualNetwork(ctx context.Context, p *ProviderAzure) (*armnetwork.VirtualNetwork, error) {

	parameters := armnetwork.VirtualNetwork{
		Location: to.Ptr(viper.GetString("azure.location")),
		Properties: &armnetwork.VirtualNetworkPropertiesFormat{
			AddressSpace: &armnetwork.AddressSpace{
				AddressPrefixes: []*string{
					to.Ptr(viper.GetString("azure.vm.vnet.cidr")), // example 10.1.0.0/16
				},
			},
			Subnets: []*armnetwork.Subnet{
				{
					Name: to.Ptr(viper.GetString("azure.vm.vnet.subnet.name")),
					Properties: &armnetwork.SubnetPropertiesFormat{
						AddressPrefix: to.Ptr(viper.GetString("azure.vm.vnet.subnet.cidr")),
					},
				},
			},
		},
	}

	pollerResponse, err := p.VnetClient.BeginCreateOrUpdate(ctx, viper.GetString("azure.resourceGroup"), viper.GetString("azure.vm.vnet.name"), parameters, nil)
	if err != nil {
		return nil, err
	}

	resp, err := pollerResponse.PollUntilDone(ctx, nil)
	if err != nil {
		return nil, err
	}

	return &resp.VirtualNetwork, nil
}

func createPublicIP(ctx context.Context, p *ProviderAzure) (*armnetwork.PublicIPAddress, error) {

	parameters := armnetwork.PublicIPAddress{
		Location: to.Ptr(viper.GetString("azure.location")),
		Properties: &armnetwork.PublicIPAddressPropertiesFormat{
			PublicIPAllocationMethod: to.Ptr(armnetwork.IPAllocationMethodStatic), // Static or Dynamic
		},
	}

	pollerResponse, err := p.PublicIPClient.BeginCreateOrUpdate(ctx, viper.GetString("azure.resourceGroup"), viper.GetString("azure.vm.publicIP.name"), parameters, nil)
	if err != nil {
		return nil, err
	}

	resp, err := pollerResponse.PollUntilDone(ctx, nil)
	if err != nil {
		return nil, err
	}
	return &resp.PublicIPAddress, err
}
