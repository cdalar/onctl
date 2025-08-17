package cloud

import (
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"github.com/cdalar/onctl/internal/tools"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore/runtime"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/to"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/compute/armcompute/v5"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/network/armnetwork/v5"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/resourcegraph/armresourcegraph"
	"github.com/spf13/viper"
)

type ProviderAzure struct {
	ResourceGraphClient *armresourcegraph.Client
	VmClient            *armcompute.VirtualMachinesClient
	NicClient           *armnetwork.InterfacesClient
	PublicIPClient      *armnetwork.PublicIPAddressesClient
	SSHKeyClient        *armcompute.SSHPublicKeysClient
	VnetClient          *armnetwork.VirtualNetworksClient
}

type QueryResponse struct {
	TotalRecords *int64
	Data         map[string]interface {
	}
}

func (p ProviderAzure) AttachNetwork(vm Vm, network Network) error {
	log.Println("[DEBUG] Attaching network: ", network)
	return nil
}

func (p ProviderAzure) DetachNetwork(vm Vm, network Network) error {
	log.Println("[DEBUG] Detaching network: ", network)
	return nil
}

func (p ProviderAzure) List() (VmList, error) {
	log.Println("[DEBUG] List Servers")
	query := `
	resources
    | where type =~ 'microsoft.compute/virtualmachines' and resourceGroup =~ '` + viper.GetString("azure.resourceGroup") + `'	
    | extend nics=array_length(properties.networkProfile.networkInterfaces)
    | mv-expand nic=properties.networkProfile.networkInterfaces
    | where nics == 1 or nic.properties.primary =~ 'true' or isempty(nic)
    | project vmId = id, vmName = name, vmSize=tostring(properties.hardwareProfile.vmSize), nicId = tostring(nic.id), timeCreated = tostring(properties.timeCreated), status = tostring(properties.extended.instanceView.powerState.displayStatus), location = tostring(location)
    | join kind=leftouter (
        resources
        | where type =~ 'microsoft.network/networkinterfaces'
        | extend ipConfigsCount=array_length(properties.ipConfigurations)
        | mv-expand ipconfig=properties.ipConfigurations
        | where ipConfigsCount == 1 or ipconfig.properties.primary =~ 'true'
        | project nicId = id, publicIpId = tostring(ipconfig.properties.publicIPAddress.id), privateIp = tostring(ipconfig.properties.privateIPAddress))
    on nicId
    | project-away nicId1
    | summarize by vmId, vmName, vmSize, nicId, publicIpId, privateIp, timeCreated, status, location
    | join kind=leftouter (
        resources
        | where type =~ 'microsoft.network/publicipaddresses'
        | project publicIpId = id, publicIpAddress = properties.ipAddress)
    on publicIpId
    | project-away publicIpId1
    | order by timeCreated asc	
	`
	// Create the query request, Run the query and get the results. Update the VM and subscriptionID details below.
	resp, err := p.ResourceGraphClient.Resources(context.Background(),
		armresourcegraph.QueryRequest{
			Query: to.Ptr(query),
			Subscriptions: []*string{
				to.Ptr(viper.GetString("azure.subscriptionId"))},
			Options: &armresourcegraph.QueryRequestOptions{
				ResultFormat: to.Ptr(armresourcegraph.ResultFormatObjectArray),
			},
		},
		nil)
	if err != nil {
		log.Fatalf("failed to finish the request: %v", err)
	} else {
		// Print the obtained query results
		log.Printf("[DEBUG] Resources found: %d\n", *resp.TotalRecords)
		log.Printf("[DEBUG] Results: %v\n", resp.Data)
	}
	if len(strconv.FormatInt(*resp.TotalRecords, 10)) == 0 {
		return VmList{}, nil
	}
	log.Println("[DEBUG] ", resp.Data)
	cloudList := make([]Vm, 0, len(strconv.FormatInt(*resp.TotalRecords, 10)))
	if m, ok := resp.Data.([]interface{}); ok {
		for _, r := range m {
			items := r.(map[string]interface{})
			createdAt, err := time.Parse("2006-01-02T15:04:05Z", items["timeCreated"].(string))
			if err != nil {
				log.Fatalln(err)
			}
			if items["publicIpAddress"] == nil {
				items["publicIpAddress"] = "N/A"
			}

			cloudList = append(cloudList, Vm{
				Provider:  "azure",
				ID:        filepath.Base(items["vmId"].(string)),
				Name:      items["vmName"].(string),
				IP:        items["publicIpAddress"].(string),
				PrivateIP: items["privateIp"].(string),
				Type:      items["vmSize"].(string),
				Status:    items["status"].(string),
				CreatedAt: createdAt,
				Location:  items["location"].(string),
				Cost: CostStruct{
					Currency:        "N/A",
					CostPerHour:     0,
					CostPerMonth:    0,
					AccumulatedCost: 0,
				},
			})
		}
	}

	output := VmList{
		List: cloudList,
	}
	return output, nil
}

func (p ProviderAzure) CreateSSHKey(publicKeyFileName string) (string, error) {
	log.Println("[DEBUG] Create SSH Key")

	username := tools.GenerateUserName()
	sshPublicKeyData, err := os.ReadFile(publicKeyFileName)
	if err != nil {
		log.Println(err)
	}
	// Create the SSH Key
	sshKey, err := p.SSHKeyClient.Create(context.Background(), viper.GetString("azure.resourceGroup"), username, armcompute.SSHPublicKeyResource{
		Properties: &armcompute.SSHPublicKeyResourceProperties{
			PublicKey: to.Ptr(string(sshPublicKeyData[:])),
		},
		Location: to.Ptr(viper.GetString("azure.location")),
	}, nil)
	if err != nil {
		log.Fatalln(err)
	}
	return *sshKey.ID, err
}

func (p ProviderAzure) getSSHKeyPublicData() string {
	userName := tools.GenerateUserName()
	sshKey, err := p.SSHKeyClient.Get(context.Background(), viper.GetString("azure.resourceGroup"), userName, nil)
	if err != nil {
		log.Println(err)
	}
	log.Println("[DEBUG] ", sshKey)
	return *sshKey.Properties.PublicKey
}

func (p ProviderAzure) Deploy(server Vm) (Vm, error) {
	log.Println("[DEBUG] Deploy Server")

	var vnet *armnetwork.VirtualNetwork
	var err error
	// Create the Vnet
	if viper.GetString("azure.vm.vnet.create") == "true" {
		vnet, err = createVirtualNetwork(context.Background(), &p)
		if err != nil {
			log.Fatalln(err)
		}
		log.Println("[DEBUG] ", vnet)
	} else { // Get the Vnet
		vnetResp, err := p.VnetClient.Get(context.Background(), viper.GetString("azure.resourceGroup"), viper.GetString("azure.vm.vnet.name"), nil)
		if err != nil {
			log.Fatalln(err)
		}
		log.Println("[DEBUG] ", vnetResp)
		vnet = &vnetResp.VirtualNetwork
	}
	pip, err := createPublicIP(context.Background(), &p, server)
	if err != nil {
		log.Fatalln(err)
	}
	log.Println("[DEBUG] ", pip)
	nic, err := createNic(context.Background(), &p, server, vnet, pip)
	if err != nil {
		log.Fatalln(err)
	}
	log.Println("[DEBUG] ", nic)

	// Create the VM
	vmDefinition := armcompute.VirtualMachine{
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
				OSDisk: &armcompute.OSDisk{
					DiffDiskSettings: &armcompute.DiffDiskSettings{
						Option:    to.Ptr(armcompute.DiffDiskOptionsLocal),
						Placement: to.Ptr(armcompute.DiffDiskPlacementResourceDisk),
					},
					Caching:      to.Ptr(armcompute.CachingTypesReadOnly),
					CreateOption: to.Ptr(armcompute.DiskCreateOptionTypesFromImage),
				},
			},
			NetworkProfile: &armcompute.NetworkProfile{
				NetworkInterfaces: []*armcompute.NetworkInterfaceReference{
					{
						ID: nic.ID,
						Properties: &armcompute.NetworkInterfaceReferenceProperties{
							Primary: to.Ptr(true),
						},
					},
				},
			},
			OSProfile: &armcompute.OSProfile{
				CustomData:    to.Ptr(tools.FileToBase64(server.CloudInitFile)),
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
	}

	if viper.GetString("azure.vm.priority") == "Spot" {
		vmDefinition.Properties.Priority = to.Ptr(armcompute.VirtualMachinePriorityTypesSpot)
		vmDefinition.Properties.EvictionPolicy = to.Ptr(armcompute.VirtualMachineEvictionPolicyTypesDelete)
		vmDefinition.Properties.BillingProfile = &armcompute.BillingProfile{MaxPrice: to.Ptr(0.1)}
	}

	poller, err := p.VmClient.BeginCreateOrUpdate(context.Background(), viper.GetString("azure.resourceGroup"), server.Name, vmDefinition, nil)
	if err != nil {
		log.Fatalln(err)
	}
	resp, err := poller.PollUntilDone(context.Background(), &runtime.PollUntilDoneOptions{
		Frequency: time.Duration(3) * time.Second,
	})
	return Vm{
		ID:        *resp.Properties.VMID,
		Name:      *resp.Name,
		IP:        *pip.Properties.IPAddress,
		Type:      string(*resp.Properties.HardwareProfile.VMSize),
		Status:    *resp.Properties.ProvisioningState,
		CreatedAt: *resp.Properties.TimeCreated,
	}, err
}

func (p ProviderAzure) Destroy(server Vm) error {
	log.Println("[DEBUG] Destroy Server")
	resp, err := p.VmClient.BeginDelete(context.Background(), viper.GetString("azure.resourceGroup"), server.Name, nil)
	if err != nil {
		log.Fatalln(err)
	}
	log.Println("[DEBUG] ", resp)

	respDone, err := resp.PollUntilDone(context.Background(), &runtime.PollUntilDoneOptions{
		Frequency: time.Duration(3) * time.Second,
	})
	if err != nil {
		log.Fatalln(err)
	}
	log.Println("[DEBUG] ", respDone)
	if resp.Done() {
		log.Println("[DEBUG] DONE")
	}

	nic, err := p.NicClient.BeginDelete(context.Background(), viper.GetString("azure.resourceGroup"), server.Name+"-nic", nil)
	if err != nil {
		log.Fatalln(err)
	}
	nicDone, err := nic.PollUntilDone(context.Background(), &runtime.PollUntilDoneOptions{
		Frequency: time.Duration(3) * time.Second,
	})
	if err != nil {
		log.Fatalln(err)
	}
	log.Println("[DEBUG] ", nicDone)
	if nic.Done() {
		log.Println("[DEBUG] DONE")
	}
	pip, err := p.PublicIPClient.BeginDelete(context.Background(), viper.GetString("azure.resourceGroup"), server.Name+"-pip", nil)
	if err != nil {
		log.Fatalln(err)
	}
	if err != nil {
		log.Fatalln(err)
	}
	log.Println("[DEBUG] ", pip)

	return err

}

func (p ProviderAzure) SSHInto(serverName string, port int, privateKey string, jumpHost string) {
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
		User:           viper.GetString("azure.vm.username"),
		Port:           port,
		PrivateKeyFile: privateKey,
		JumpHost:       jumpHost,
	})

}

func (p ProviderAzure) GetByName(serverName string) (Vm, error) {
	vmList, err := p.List()
	if err != nil {
		return Vm{}, err
	}
	for _, vm := range vmList.List {
		if vm.Name == serverName {
			return vm, nil
		}
	}
	return Vm{}, nil
}

func createVirtualNetwork(ctx context.Context, p *ProviderAzure) (*armnetwork.VirtualNetwork, error) {
	parameters := armnetwork.VirtualNetwork{
		Location: to.Ptr(viper.GetString("azure.location")),
		Properties: &armnetwork.VirtualNetworkPropertiesFormat{
			AddressSpace: &armnetwork.AddressSpace{
				AddressPrefixes: []*string{
					to.Ptr(viper.GetString("azure.vm.vnet.cidr")),
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
	fmt.Print("Creating virtual network...")
	pollerResponse, err := p.VnetClient.BeginCreateOrUpdate(ctx, viper.GetString("azure.resourceGroup"), viper.GetString("azure.vm.vnet.name"), parameters, nil)
	if err != nil {
		return nil, err
	}

	resp, err := pollerResponse.PollUntilDone(ctx, &runtime.PollUntilDoneOptions{
		Frequency: time.Duration(3) * time.Second,
	})
	if err != nil {
		return nil, err
	}

	if pollerResponse.Done() {
		log.Println("[DEBUG] DONE")
	}

	return &resp.VirtualNetwork, nil
}

func createPublicIP(ctx context.Context, p *ProviderAzure, server Vm) (*armnetwork.PublicIPAddress, error) {

	parameters := armnetwork.PublicIPAddress{
		Location: to.Ptr(viper.GetString("azure.location")),
		Properties: &armnetwork.PublicIPAddressPropertiesFormat{
			PublicIPAllocationMethod: to.Ptr(armnetwork.IPAllocationMethodStatic), // Static or Dynamic
		},
	}
	fmt.Print("Creating public IP...")
	pollerResponse, err := p.PublicIPClient.BeginCreateOrUpdate(ctx, viper.GetString("azure.resourceGroup"), server.Name+"-pip", parameters, nil)
	if err != nil {
		return nil, err
	}

	resp, err := pollerResponse.PollUntilDone(ctx, &runtime.PollUntilDoneOptions{
		Frequency: time.Duration(3) * time.Second,
	})
	if err != nil {
		return nil, err
	}
	if pollerResponse.Done() {
		log.Println("[DEBUG] DONE")
	}

	return &resp.PublicIPAddress, err
}

func createNic(ctx context.Context, p *ProviderAzure, server Vm, vnet *armnetwork.VirtualNetwork, pip *armnetwork.PublicIPAddress) (*armnetwork.Interface, error) {
	fmt.Print("Creating network interface...")
	nicResp, err := p.NicClient.BeginCreateOrUpdate(context.Background(), viper.GetString("azure.resourceGroup"), server.Name+"-nic", armnetwork.Interface{
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
	nicRespDone, err := nicResp.PollUntilDone(ctx, &runtime.PollUntilDoneOptions{
		Frequency: time.Duration(3) * time.Second,
	})
	if err != nil {
		log.Fatalln(err)
	}
	if nicResp.Done() {
		log.Println("[DEBUG] DONE")
	}

	log.Println("[DEBUG] ", nicRespDone)
	return &nicRespDone.Interface, err

}
