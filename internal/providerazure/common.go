package providerazure

import (
	"log"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/Azure/azure-sdk-for-go/sdk/azidentity"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/compute/armcompute"
)

var subscriptionId string = "3c110410-a29d-4402-96c4-f82b0feaa895"

func GetClient() (vmClient *armcompute.VirtualMachinesClient) {
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

func connectionAzure() (azcore.TokenCredential, error) {
	cred, err := azidentity.NewDefaultAzureCredential(nil)
	if err != nil {
		return nil, err
	}
	return cred, nil
}
