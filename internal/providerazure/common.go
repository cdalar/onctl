package providerazure

import (
	"log"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/Azure/azure-sdk-for-go/sdk/azidentity"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/compute/armcompute/v5"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/network/armnetwork/v5"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/resourcegraph/armresourcegraph"
	"github.com/spf13/viper"
)

func GetResourceGraphClient() (resourceGraphClient *armresourcegraph.Client) {
	cred, err := connectionAzure()
	if err != nil {
		log.Printf("[DEBUG] cannot connect to Azure: %+v", err)
		return nil
	}
	resourceGraphClient, err = armresourcegraph.NewClient(cred, nil)
	if err != nil {
		return nil
	}

	return resourceGraphClient
}

func GetVmClient() (vmClient *armcompute.VirtualMachinesClient) {
	sub := viper.GetString("azure.subscriptionId")
	if sub == "" {
		log.Printf("[DEBUG] azure.subscriptionId not configured")
		return nil
	}
	cred, err := connectionAzure()
	if err != nil {
		log.Printf("[DEBUG] cannot connect to Azure: %+v", err)
		return nil
	}
	vmClient, err = armcompute.NewVirtualMachinesClient(sub, cred, nil)
	// armCompute, err = armcompute.
	if err != nil {
		return nil
	}

	return vmClient
}

func GetNicClient() (nicClient *armnetwork.InterfacesClient) {
	sub := viper.GetString("azure.subscriptionId")
	if sub == "" {
		log.Printf("[DEBUG] azure.subscriptionId not configured")
		return nil
	}
	cred, err := connectionAzure()
	if err != nil {
		log.Printf("[DEBUG] cannot connect to Azure: %+v", err)
		return nil
	}
	nicClient, err = armnetwork.NewInterfacesClient(sub, cred, nil)
	if err != nil {
		return nil
	}

	return nicClient
}

func GetSSHKeyClient() (sshClient *armcompute.SSHPublicKeysClient) {
	sub := viper.GetString("azure.subscriptionId")
	if sub == "" {
		log.Printf("[DEBUG] azure.subscriptionId not configured")
		return nil
	}
	cred, err := connectionAzure()
	if err != nil {
		log.Printf("[DEBUG] cannot connect to Azure: %+v", err)
		return nil
	}
	sshClient, err = armcompute.NewSSHPublicKeysClient(sub, cred, nil)
	if err != nil {
		return nil
	}

	return sshClient
}

func GetIPClient() (publicIpClient *armnetwork.PublicIPAddressesClient) {
	sub := viper.GetString("azure.subscriptionId")
	if sub == "" {
		log.Printf("[DEBUG] azure.subscriptionId not configured")
		return nil
	}
	cred, err := connectionAzure()
	if err != nil {
		log.Printf("[DEBUG] cannot connect to Azure: %+v", err)
		return nil
	}
	ipClient, err := armnetwork.NewPublicIPAddressesClient(sub, cred, nil)
	if err != nil {
		return nil
	}

	return ipClient
}

func GetVnetClient() (vnetClient *armnetwork.VirtualNetworksClient) {
	sub := viper.GetString("azure.subscriptionId")
	if sub == "" {
		log.Printf("[DEBUG] azure.subscriptionId not configured")
		return nil
	}
	cred, err := connectionAzure()
	if err != nil {
		log.Printf("[DEBUG] cannot connect to Azure: %+v", err)
		return nil
	}
	vnetClient, err = armnetwork.NewVirtualNetworksClient(sub, cred, nil)
	if err != nil {
		return nil
	}

	return vnetClient
}

func GetNSGClient() (nsgClient *armnetwork.SecurityGroupsClient) {
	sub := viper.GetString("azure.subscriptionId")
	if sub == "" {
		log.Printf("[DEBUG] azure.subscriptionId not configured")
		return nil
	}
	cred, err := connectionAzure()
	if err != nil {
		log.Printf("[DEBUG] cannot connect to Azure: %+v", err)
		return nil
	}
	nsgClient, err = armnetwork.NewSecurityGroupsClient(sub, cred, nil)
	if err != nil {
		return nil
	}

	return nsgClient
}

func connectionAzure() (azcore.TokenCredential, error) {
	cred, err := azidentity.NewDefaultAzureCredential(nil)
	if err != nil {
		return nil, err
	}
	return cred, nil
}
