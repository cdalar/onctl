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
	Name string `yaml:"name"`
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
	SSHPort int `yaml:"sshPort"`
	// CloudInit is the cloud-init file
	CloudInitFile string `yaml:"cloudInitFile"`
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
}
