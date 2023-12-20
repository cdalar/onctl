package providerazure

import (
	"log"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/Azure/azure-sdk-for-go/sdk/azidentity"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/compute/armcompute"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/network/armnetwork"
	"github.com/spf13/viper"
)

var subscriptionId string = viper.GetString("azure.subscription_id")

func GetVmClient() (vmClient *armcompute.VirtualMachinesClient) {
	cred, err := connectionAzure()
	if err != nil {
		log.Fatalf("cannot connect to Azure:%+v", err)
	}
	vmClient, err = armcompute.NewVirtualMachinesClient(subscriptionId, cred, nil)
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
	nicClient, err = armnetwork.NewInterfacesClient(subscriptionId, cred, nil)
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
	sshClient, err = armcompute.NewSSHPublicKeysClient(subscriptionId, cred, nil)
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
	ipClient, err := armnetwork.NewPublicIPAddressesClient(subscriptionId, cred, nil)
	if err != nil {
		return nil
	}

	return ipClient
}

func connectionAzure() (azcore.TokenCredential, error) {
	cred, err := azidentity.NewDefaultAzureCredential(nil)
	if err != nil {
		return nil, err
	}
	return cred, nil
}
