package cloud

import (
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strconv"
	"time"

	compute "cloud.google.com/go/compute/apiv1"
	"cloud.google.com/go/compute/apiv1/computepb"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/to"
	"github.com/cdalar/onctl/internal/tools"
	"github.com/spf13/viper"
	"google.golang.org/api/iterator"
)

var (
	publicKey []byte
)

type ProviderGcp struct {
	Client      *compute.InstancesClient
	GroupClient *compute.InstanceGroupsClient
}

func (p ProviderGcp) AttachNetwork(vm Vm, network Network) error {
	log.Println("[DEBUG] Attaching network: ", network)
	return nil
}

func (p ProviderGcp) DetachNetwork(vm Vm, network Network) error {
	log.Println("[DEBUG] Detaching network: ", network)
	return nil
}

func (p ProviderGcp) List() (VmList, error) {
	log.Println("[DEBUG] List Servers")
	cloudList := make([]Vm, 0, 100)
	it := p.Client.AggregatedList(context.Background(), &computepb.AggregatedListInstancesRequest{
		Project: viper.GetString("gcp.project"),
	})
	for {
		resp, err := it.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			log.Fatalln(err)
		}
		for _, instance := range resp.Value.Instances {
			cloudList = append(cloudList, mapGcpServer(instance))
			log.Println("[DEBUG] server name: " + *instance.Name)
		}
		_ = resp
	}
	output := VmList{
		List: cloudList,
	}
	return output, nil
}

func (p ProviderGcp) CreateSSHKey(publicKeyFile string) (keyID string, err error) {
	publicKey, err = os.ReadFile(publicKeyFile)
	if err != nil {
		log.Fatalln(err)
	}
	return
}

func (p ProviderGcp) Destroy(server Vm) error {
	log.Println("[DEBUG] Destroy server: ", server)
	if server.ID == "" && server.Name != "" {
		log.Println("[DEBUG] Server ID is empty")
		log.Println("[DEBUG] Server name: " + server.Name)
		s, err := p.GetByName(server.Name)
		if err != nil || s.ID == "" {
			log.Println("[DEBUG] Server not found")
			return err
		}
		log.Println("[DEBUG] Server found ID: " + s.ID)
		server.ID = s.ID
	}
	opt, err := p.Client.Delete(context.Background(), &computepb.DeleteInstanceRequest{
		Project:  viper.GetString("gcp.project"),
		Zone:     viper.GetString("gcp.zone"),
		Instance: server.Name,
	})
	if err != nil {
		log.Fatalln(err)
	}
	log.Println("[DEBUG] Operation: ", opt)
	return nil
}

func (p ProviderGcp) GetByName(serverName string) (Vm, error) {
	log.Println("[DEBUG] Get Server by Name: ", serverName)
	server, err := p.Client.Get(context.Background(), &computepb.GetInstanceRequest{
		Project:  viper.GetString("gcp.project"),
		Zone:     viper.GetString("gcp.zone"),
		Instance: serverName,
	})
	if err != nil {
		log.Fatalln(err)
	}
	return mapGcpServer(server), nil

}

func (p ProviderGcp) Deploy(server Vm) (Vm, error) {

	machineType := fmt.Sprintf("zones/%s/machineTypes/%s", viper.GetString("gcp.zone"), viper.GetString("gcp.type"))
	op, err := p.Client.Insert(context.Background(), &computepb.InsertInstanceRequest{
		Project: viper.GetString("gcp.project"),
		Zone:    viper.GetString("gcp.zone"),
		InstanceResource: &computepb.Instance{
			Name:        &server.Name,
			MachineType: &machineType,
			Metadata: &computepb.Metadata{
				Items: []*computepb.Items{
					{
						Key:   to.Ptr("ssh-keys"),
						Value: to.Ptr(fmt.Sprintf("%s:%s", viper.GetString("gcp.vm.username"), string(publicKey))),
					},
					{
						Key:   to.Ptr("user-data"),
						Value: to.Ptr(tools.FileToBase64(server.CloudInitFile)),
					},
				},
			},
			Disks: []*computepb.AttachedDisk{
				{
					AutoDelete: to.Ptr(true),
					Boot:       to.Ptr(true),
					InitializeParams: &computepb.AttachedDiskInitializeParams{
						SourceImage: to.Ptr("projects/ubuntu-os-cloud/global/images/ubuntu-2204-jammy-v20240208"),
					},
				},
			},

			NetworkInterfaces: []*computepb.NetworkInterface{
				{
					AccessConfigs: []*computepb.AccessConfig{
						{
							Name: to.Ptr("External NAT"),
						},
					},
				},
			},
		},
	})
	if err != nil {
		log.Fatalln(err)
	}
	err = op.Wait(context.Background())
	if err != nil {
		log.Fatalln(err)
	}
	return p.GetByName(server.Name)
}

func (p ProviderGcp) SSHInto(serverName string, port int, privateKey string) {
	server, err := p.GetByName(serverName)
	if err != nil {
		log.Fatalln(err)
	}
	if privateKey == "" {
		privateKey = viper.GetString("ssh.privateKey")
	}
	tools.SSHIntoVM(tools.SSHIntoVMRequest{
		IPAddress:      server.IP,
		User:           viper.GetString("gcp.vm.username"),
		Port:           port,
		PrivateKeyFile: privateKey,
	})
}

// mapGcpServer maps a GCP server to a Vm struct
func mapGcpServer(server *computepb.Instance) Vm {
	createdAt, err := time.Parse(time.RFC3339, *server.CreationTimestamp)
	if err != nil {
		log.Fatalln(err)
	}

	return Vm{
		Provider: "gcp",
		ID:       strconv.FormatUint(server.GetId(), 10),
		Name:     server.GetName(),
		IP:       server.GetNetworkInterfaces()[0].GetAccessConfigs()[0].GetNatIP(),
		// PrivateIP:   server.GetNetworkInterfaces()[0].GetNetworkIP(),
		Type:      filepath.Base(server.GetMachineType()),
		Status:    server.GetStatus(),
		CreatedAt: createdAt,
		Location:  filepath.Base(server.GetZone()),
	}
}
