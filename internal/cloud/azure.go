package cloud

import (
	"context"
	"log"

	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/compute/armcompute"
)

type ProviderAzure struct {
	Client *armcompute.VirtualMachinesClient
}

func (p ProviderAzure) List() (VmList, error) {
	pager := p.Client.NewListAllPager(&armcompute.VirtualMachinesClientListAllOptions{})
	for pager.More() {
		nextResult, err := pager.NextPage(context.Background())
		if err != nil {
			log.Fatalf("failed to advance page: %v", err)
			return VmList{}, err
		}
		for _, v := range nextResult.Value {
			_ = v
			// vm := mapAzureServer(v)
		}
	}
	// for pager.NextPage() {
	// 	page := pager.PageResponse()
	// 	log.Println("[DEBUG] Page: ", page)
	// }

	log.Println("[DEBUG] List Servers")
	return VmList{}, nil
}

func (p ProviderAzure) Create() (Vm, error) {
	log.Println("[DEBUG] Create Server")
	return Vm{}, nil
}

func (p ProviderAzure) Delete() error {
	log.Println("[DEBUG] Delete Server")
	return nil
}

func (p ProviderAzure) CreateSSHKey(string) (string, error) {
	log.Println("[DEBUG] Start Server")
	return "nil", nil
}

func (p ProviderAzure) Deploy(Vm) (Vm, error) {
	log.Println("[DEBUG] Start Server")
	return Vm{}, nil
}

func (p ProviderAzure) Destroy(Vm) error {
	log.Println("[DEBUG] Start Server")
	return nil
}

func mapAzureServer(server *armcompute.VirtualMachine) Vm {
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
	return Vm{
		// ID:        *server.ID,
		// Name:      serverName,
		// IP:        *server.Properties.NetworkProfile.NetworkInterfaces[0].Properties.,
		// Type:      *server.InstanceType,
		// Status:    *server.State.Name,
		// CreatedAt: *server.LaunchTime,
	}
}
