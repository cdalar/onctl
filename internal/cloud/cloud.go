package cloud

import (
	"fmt"
	"reflect"
	"time"
)

type VmList struct {
	List []Vm
}

type Price struct {
	// Currency is the currency of the price
	Currency string
	// Hourly is the hourly price
	Hourly string
	// Monthly is the monthly price
	Monthly string
}

type Vm struct {
	// ID is the ID of the instance
	ID string
	// Name is the name of the instance
	Name string
	// IP is the public IP of the instance
	IP string
	//LocalIP is the local IP of the instance
	PrivateIP string
	// Type is the type of the instance
	Type string
	// Status is the status of the instance
	Status string
	// Location is the location of the instance
	Location string
	// SSHKeyID is the ID of the SSH key
	SSHKeyID string
	// SSHPort is the port to connect to the instance
	SSHPort int
	// CloudInit is the cloud-init file
	CloudInitFile string
	// CreatedAt is the creation date of the instance
	CreatedAt time.Time
	// Provider is the cloud provider
	Provider string
	// Cost is the cost of the vm
	Cost CostStruct
}

type CostStruct struct {
	Currency        string
	CostPerHour     float64
	CostPerMonth    float64
	AccumulatedCost float64
}

func (v Vm) String() string {
	value := reflect.ValueOf(v)
	typeOfS := value.Type()
	var ret string = "\n"
	for i := 0; i < value.NumField(); i++ {
		ret = ret + fmt.Sprintf("%s:\t %v\n", typeOfS.Field(i).Name, value.Field(i).Interface())
	}
	return ret
}

type CloudProviderInterface interface {
	// Deploy deploys a new instance
	Deploy(Vm) (Vm, error)
	// Destroy destroys an instance
	Destroy(Vm) error
	// List lists all instances
	List() (VmList, error)
	// CreateSSHKey creates a new SSH key
	CreateSSHKey(publicKeyFile string) (keyID string, err error)
	// SSHInto connects to a VM
	SSHInto(serverName string, port int, privateKey string)
	// GetByName gets a VM by name
	GetByName(serverName string) (Vm, error)
	// AttachNetwork attaches a network to a VM
	AttachNetwork(vm Vm, network Network) error
	// DetachNetwork detaches a network from a VM
	// DetachNetwork(vm Vm, network Network) error
}

type NetworkManager interface {
	// Create creates a network
	Create(Network) (Network, error)
	// Delete deletes a network
	Delete(Network) error
	// List lists all networks
	List() ([]Network, error)
	// GetByName gets a network by name
	GetByName(networkName string) (Network, error)
}

type Network struct {
	// ID is the ID of the network
	ID string
	// Name is the name of the network
	Name string
	// CIDR is the CIDR of the network
	CIDR string
	// Type is the type of the network
	Type string
	// Status is the status of the network
	Status string
	// Location is the location of the network
	Location string
	// CreatedAt is the creation date of the network
	CreatedAt time.Time
	// Provider is the cloud provider
	Provider string
	// Servers is the number of servers in the network
	Servers int
}

type StorageManager interface {
	// StorageCreate creates a storage
	StorageCreate()
	// StorageDelete deletes a storage
	StorageDelete()
	// StorageList lists all storages
	StorageList()
	// StorageAttach attaches a storage to a VM
	StorageAttach()
	// StorageDetach detaches a storage from a VM
	StorageDetach()
}
