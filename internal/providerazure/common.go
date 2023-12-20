package providerazure

import (
	"log"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/Azure/azure-sdk-for-go/sdk/azidentity"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/compute/armcompute"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/network/armnetwork"
	"github.com/spf13/viper"
)

func GetVmClient() (vmClient *armcompute.VirtualMachinesClient) {
	cred, err := connectionAzure()
	if err != nil {
		log.Fatalf("cannot connect to Azure:%+v", err)
	}
	vmClient, err = armcompute.NewVirtualMachinesClient(viper.GetString("azure.subscriptionId"), cred, nil)
	// armCompute, err = armcompute.
	if err != nil {
		return nil
	}

	return vmClient
}

func GetNicClient() (nicClient *armnetwork.InterfacesClient) {
	cred, err := connectionAzure()
	if err != nil {
		log.Fatalf("cannot connect to Azure:%+v", err)
	}
	nicClient, err = armnetwork.NewInterfacesClient(viper.GetString("azure.subscriptionId"), cred, nil)
	if err != nil {
		return nil
	}

	return nicClient
}

func GetSSHKeyClient() (sshClient *armcompute.SSHPublicKeysClient) {
	cred, err := connectionAzure()
	if err != nil {
		log.Fatalf("cannot connect to Azure:%+v", err)
	}
	sshClient, err = armcompute.NewSSHPublicKeysClient(viper.GetString("azure.subscriptionId"), cred, nil)
	if err != nil {
		return nil
	}

	return sshClient
}

func GetIPClient() (publicIpClient *armnetwork.PublicIPAddressesClient) {
	cred, err := connectionAzure()
	if err != nil {
		log.Fatalf("cannot connect to Azure:%+v", err)
	}
	ipClient, err := armnetwork.NewPublicIPAddressesClient(viper.GetString("azure.subscriptionId"), cred, nil)
	if err != nil {
		return nil
	}

	return ipClient
}

func GetVnetClient() (vnetClient *armnetwork.VirtualNetworksClient) {
	cred, err := connectionAzure()
	if err != nil {
		log.Fatalf("cannot connect to Azure:%+v", err)
	}
	vnetClient, err = armnetwork.NewVirtualNetworksClient(viper.GetString("azure.subscriptionId"), cred, nil)
	if err != nil {
		return nil
	}

	return vnetClient
}

func connectionAzure() (azcore.TokenCredential, error) {
	cred, err := azidentity.NewDefaultAzureCredential(nil)
	if err != nil {
		return nil, err
	}
	return cred, nil
}
