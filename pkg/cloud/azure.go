package cloud

import (
	"context"
	"errors"
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
	NSGClient           *armnetwork.SecurityGroupsClient
}

type QueryResponse struct {
	TotalRecords *int64
	Data         map[string]interface {
	}
}

// ListPaused returns nothing: paused (deallocated) Azure VMs already appear in List.
func (p ProviderAzure) ListPaused() (VmList, error) {
	return VmList{}, nil
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
		return VmList{}, fmt.Errorf("failed to finish the request: %w", err)
	}
	if resp.TotalRecords == nil {
		return VmList{}, nil
	}
	// Print the obtained query results
	log.Printf("[DEBUG] Resources found: %d\n", *resp.TotalRecords)
	log.Printf("[DEBUG] Results: %v\n", resp.Data)
	if len(strconv.FormatInt(*resp.TotalRecords, 10)) == 0 {
		return VmList{}, nil
	}
	log.Println("[DEBUG] ", resp.Data)
	cloudList := make([]Vm, 0, len(strconv.FormatInt(*resp.TotalRecords, 10)))
	if m, ok := resp.Data.([]interface{}); ok {
		for _, r := range m {
			items, ok := r.(map[string]interface{})
			if !ok {
				return VmList{}, fmt.Errorf("unexpected resource graph row: %v", r)
			}
			// A column the query left out or nulled reads as "".
			str := func(key string) string {
				s, _ := items[key].(string)
				return s
			}
			createdAt, err := time.Parse("2006-01-02T15:04:05Z", str("timeCreated"))
			if err != nil {
				return VmList{}, fmt.Errorf("VM %q: bad creation time: %w", str("vmName"), err)
			}
			publicIP := str("publicIpAddress")
			if publicIP == "" {
				publicIP = "N/A"
			}

			cloudList = append(cloudList, Vm{
				Provider:  "azure",
				ID:        filepath.Base(str("vmId")),
				Name:      str("vmName"),
				IP:        publicIP,
				PrivateIP: str("privateIp"),
				Type:      str("vmSize"),
				Status:    str("status"),
				CreatedAt: createdAt,
				Location:  str("location"),
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
		return "", err
	}
	// Create the SSH Key
	sshKey, err := p.SSHKeyClient.Create(context.Background(), viper.GetString("azure.resourceGroup"), username, armcompute.SSHPublicKeyResource{
		Properties: &armcompute.SSHPublicKeyResourceProperties{
			PublicKey: to.Ptr(string(sshPublicKeyData[:])),
		},
		Location: to.Ptr(viper.GetString("azure.location")),
	}, nil)
	if err != nil {
		return "", err
	}
	if sshKey.ID == nil {
		return "", errors.New("Azure returned no ID for the SSH key")
	}
	return *sshKey.ID, nil
}

func (p ProviderAzure) getSSHKeyPublicData() (string, error) {
	userName := tools.GenerateUserName()
	sshKey, err := p.SSHKeyClient.Get(context.Background(), viper.GetString("azure.resourceGroup"), userName, nil)
	if err != nil {
		return "", err
	}
	log.Println("[DEBUG] ", sshKey)
	if sshKey.Properties == nil || sshKey.Properties.PublicKey == nil {
		return "", fmt.Errorf("SSH key %q has no public key", userName)
	}
	return *sshKey.Properties.PublicKey, nil
}

// Pause deallocates the VM. A deallocated Azure VM accrues no compute cost (only
// disk storage), so unlike Hetzner there is no need to snapshot and delete. Note
// that a plain power-off would still bill compute; deallocate is what stops it.
// The hot flag is accepted for interface symmetry but has no effect here.
func (p ProviderAzure) Pause(server Vm, hot bool) error {
	log.Println("[DEBUG] Deallocating VM: ", server.Name)
	poller, err := p.VmClient.BeginDeallocate(context.Background(), viper.GetString("azure.resourceGroup"), server.Name, nil)
	if err != nil {
		return err
	}
	_, err = poller.PollUntilDone(context.Background(), &runtime.PollUntilDoneOptions{
		Frequency: time.Duration(3) * time.Second,
	})
	return err
}

// Resume starts a previously paused (deallocated) VM and returns it once running.
func (p ProviderAzure) Resume(server Vm) (Vm, error) {
	log.Println("[DEBUG] Starting VM: ", server.Name)
	poller, err := p.VmClient.BeginStart(context.Background(), viper.GetString("azure.resourceGroup"), server.Name, nil)
	if err != nil {
		return Vm{}, err
	}
	if _, err = poller.PollUntilDone(context.Background(), &runtime.PollUntilDoneOptions{
		Frequency: time.Duration(3) * time.Second,
	}); err != nil {
		return Vm{}, err
	}
	return p.GetByName(server.Name)
}

func (p ProviderAzure) Deploy(server Vm) (Vm, error) {
	log.Println("[DEBUG] Deploy Server")

	var vnet *armnetwork.VirtualNetwork
	var err error
	// Create the Vnet
	if viper.GetString("azure.vm.vnet.create") == "true" {
		vnet, err = createVirtualNetwork(context.Background(), &p)
		if err != nil {
			return Vm{}, fmt.Errorf("creating virtual network: %w", err)
		}
		log.Println("[DEBUG] ", vnet)
	} else { // Get the Vnet
		vnetResp, err := p.VnetClient.Get(context.Background(), viper.GetString("azure.resourceGroup"), viper.GetString("azure.vm.vnet.name"), nil)
		if err != nil {
			return Vm{}, fmt.Errorf("getting virtual network: %w", err)
		}
		log.Println("[DEBUG] ", vnetResp)
		vnet = &vnetResp.VirtualNetwork
	}
	pip, err := createPublicIP(context.Background(), &p, server)
	if err != nil {
		return Vm{}, fmt.Errorf("creating public IP: %w", err)
	}
	log.Println("[DEBUG] ", pip)
	nsg, err := createNSG(context.Background(), &p, server)
	if err != nil {
		return Vm{}, fmt.Errorf("creating network security group: %w", err)
	}
	log.Println("[DEBUG] ", nsg)
	nic, err := createNic(context.Background(), &p, server, vnet, pip, nsg)
	if err != nil {
		return Vm{}, fmt.Errorf("creating network interface: %w", err)
	}
	log.Println("[DEBUG] ", nic)
	sshKeyData, err := p.getSSHKeyPublicData()
	if err != nil {
		return Vm{}, fmt.Errorf("reading the SSH public key: %w", err)
	}

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
					// Ephemeral OS disks (DiffDiskSettings) do not support Stop-Deallocate,
					// which is the only way to halt compute billing. Use a persistent managed
					// disk so that onctl pause/resume works correctly.
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
								KeyData: to.Ptr(sshKeyData),
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
		return Vm{}, fmt.Errorf("creating VM: %w", err)
	}
	resp, err := poller.PollUntilDone(context.Background(), &runtime.PollUntilDoneOptions{
		Frequency: time.Duration(3) * time.Second,
	})
	if err != nil {
		return Vm{}, fmt.Errorf("creating VM: %w", err)
	}
	if resp.Properties == nil || resp.Properties.HardwareProfile == nil || resp.Name == nil ||
		resp.Properties.VMID == nil || resp.Properties.ProvisioningState == nil || resp.Properties.TimeCreated == nil ||
		pip.Properties == nil || pip.Properties.IPAddress == nil || resp.Properties.HardwareProfile.VMSize == nil {
		return Vm{}, errors.New("Azure returned an incomplete VM or public IP")
	}
	return Vm{
		ID:        *resp.Properties.VMID,
		Name:      *resp.Name,
		IP:        *pip.Properties.IPAddress,
		Type:      string(*resp.Properties.HardwareProfile.VMSize),
		Status:    *resp.Properties.ProvisioningState,
		CreatedAt: *resp.Properties.TimeCreated,
	}, nil
}

func (p ProviderAzure) Destroy(server Vm) error {
	log.Println("[DEBUG] Destroy Server")
	resp, err := p.VmClient.BeginDelete(context.Background(), viper.GetString("azure.resourceGroup"), server.Name, nil)
	if err != nil {
		return fmt.Errorf("deleting VM: %w", err)
	}
	log.Println("[DEBUG] ", resp)

	respDone, err := resp.PollUntilDone(context.Background(), &runtime.PollUntilDoneOptions{
		Frequency: time.Duration(3) * time.Second,
	})
	if err != nil {
		return fmt.Errorf("deleting VM: %w", err)
	}
	log.Println("[DEBUG] ", respDone)
	if resp.Done() {
		log.Println("[DEBUG] DONE")
	}

	nic, err := p.NicClient.BeginDelete(context.Background(), viper.GetString("azure.resourceGroup"), server.Name+"-nic", nil)
	if err != nil {
		return fmt.Errorf("deleting network interface: %w", err)
	}
	nicDone, err := nic.PollUntilDone(context.Background(), &runtime.PollUntilDoneOptions{
		Frequency: time.Duration(3) * time.Second,
	})
	if err != nil {
		return fmt.Errorf("deleting network interface: %w", err)
	}
	log.Println("[DEBUG] ", nicDone)
	if nic.Done() {
		log.Println("[DEBUG] DONE")
	}
	pip, err := p.PublicIPClient.BeginDelete(context.Background(), viper.GetString("azure.resourceGroup"), server.Name+"-pip", nil)
	if err != nil {
		return fmt.Errorf("deleting public IP: %w", err)
	}
	log.Println("[DEBUG] ", pip)

	nsg, err := p.NSGClient.BeginDelete(context.Background(), viper.GetString("azure.resourceGroup"), server.Name+"-nsg", nil)
	if err != nil {
		return fmt.Errorf("deleting network security group: %w", err)
	}
	nsgDone, err := nsg.PollUntilDone(context.Background(), &runtime.PollUntilDoneOptions{
		Frequency: time.Duration(3) * time.Second,
	})
	if err != nil {
		return fmt.Errorf("deleting network security group: %w", err)
	}
	log.Println("[DEBUG] ", nsgDone)
	if nsg.Done() {
		log.Println("[DEBUG] NSG DONE")
	}

	return nil
}

func (p ProviderAzure) SSHInto(serverName string, port int, privateKey string, command []string, stdio Stdio) error {
	s, err := p.GetByName(serverName)
	if err != nil {
		return err
	}
	if s.ID == "" {
		return errors.New("No Server found with name: " + serverName)
	}
	log.Println("[DEBUG] " + s.String())

	if privateKey == "" {
		privateKey = viper.GetString("ssh.privateKey")
	}
	return tools.SSHIntoVM(tools.SSHIntoVMRequest{
		Stdin:          stdio.In,
		Stdout:         stdio.Out,
		Stderr:         stdio.Err,
		IPAddress:      s.IP,
		User:           viper.GetString("azure.vm.username"),
		Port:           port,
		PrivateKeyFile: privateKey,
		Command:        command,
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
		SKU: &armnetwork.PublicIPAddressSKU{
			Name: to.Ptr(armnetwork.PublicIPAddressSKUNameStandard),
		},
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

func createNSG(ctx context.Context, p *ProviderAzure, server Vm) (*armnetwork.SecurityGroup, error) {
	fmt.Print("Creating network security group...")
	nsgParameters := armnetwork.SecurityGroup{
		Location: to.Ptr(viper.GetString("azure.location")),
		Properties: &armnetwork.SecurityGroupPropertiesFormat{
			SecurityRules: []*armnetwork.SecurityRule{
				{
					Name: to.Ptr("SSH"),
					Properties: &armnetwork.SecurityRulePropertiesFormat{
						Description:              to.Ptr("Allow SSH"),
						Protocol:                 to.Ptr(armnetwork.SecurityRuleProtocolTCP),
						SourcePortRange:          to.Ptr("*"),
						DestinationPortRange:     to.Ptr("22"),
						SourceAddressPrefix:      to.Ptr("*"),
						DestinationAddressPrefix: to.Ptr("*"),
						Access:                   to.Ptr(armnetwork.SecurityRuleAccessAllow),
						Priority:                 to.Ptr[int32](100),
						Direction:                to.Ptr(armnetwork.SecurityRuleDirectionInbound),
					},
				},
			},
		},
	}

	pollerResponse, err := p.NSGClient.BeginCreateOrUpdate(ctx, viper.GetString("azure.resourceGroup"), server.Name+"-nsg", nsgParameters, nil)
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
		log.Println("[DEBUG] NSG DONE")
	}

	return &resp.SecurityGroup, nil
}

func createNic(ctx context.Context, p *ProviderAzure, server Vm, vnet *armnetwork.VirtualNetwork, pip *armnetwork.PublicIPAddress, nsg *armnetwork.SecurityGroup) (*armnetwork.Interface, error) {
	if vnet == nil || vnet.Properties == nil || len(vnet.Properties.Subnets) == 0 {
		return nil, errors.New("the virtual network has no subnet")
	}
	fmt.Print("Creating network interface...")
	nicResp, err := p.NicClient.BeginCreateOrUpdate(context.Background(), viper.GetString("azure.resourceGroup"), server.Name+"-nic", armnetwork.Interface{
		Location: to.Ptr(viper.GetString("azure.location")),
		Properties: &armnetwork.InterfacePropertiesFormat{
			NetworkSecurityGroup: &armnetwork.SecurityGroup{
				ID: nsg.ID,
			},
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
		return nil, err
	}
	nicRespDone, err := nicResp.PollUntilDone(ctx, &runtime.PollUntilDoneOptions{
		Frequency: time.Duration(3) * time.Second,
	})
	if err != nil {
		return nil, err
	}
	if nicResp.Done() {
		log.Println("[DEBUG] DONE")
	}

	log.Println("[DEBUG] ", nicRespDone)
	return &nicRespDone.Interface, nil
}
