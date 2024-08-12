package cloud

import (
	"context"
	"crypto/md5"
	"errors"
	"fmt"
	"log"
	"math"
	"net"
	"os"
	"strconv"
	"time"

	"github.com/cdalar/onctl/internal/tools"

	"github.com/hetznercloud/hcloud-go/v2/hcloud"
	"github.com/spf13/viper"
	"golang.org/x/crypto/ssh"
)

type ProviderHetzner struct {
	Client *hcloud.Client
}

type NetworkProviderHetzner struct {
	Client *hcloud.Client
}

func (n NetworkProviderHetzner) GetByName(networkName string) (Network, error) {
	s, _, err := n.Client.Network.GetByName(context.TODO(), networkName)
	if err != nil {
		return Network{}, err
	}
	if s == nil {
		return Network{}, errors.New("No Network found with name: " + networkName)
	}
	return mapHetznerNetwork(*s), nil
}

func (n NetworkProviderHetzner) List() ([]Network, error) {
	networkList, _, err := n.Client.Network.List(context.TODO(), hcloud.NetworkListOpts{
		ListOpts: hcloud.ListOpts{
			LabelSelector: "Owner=onctl",
		},
	})
	if err != nil {
		log.Println(err)
	}
	if len(networkList) == 0 {
		return nil, nil
	}
	cloudList := make([]Network, 0, len(networkList))
	for _, network := range networkList {
		cloudList = append(cloudList, mapHetznerNetwork(*network))
		log.Println("[DEBUG] network: ", network)
	}
	return cloudList, nil
}

func (n NetworkProviderHetzner) Delete(network Network) error {
	log.Println("[DEBUG] Deleting network: ", network)

	networkId, err := strconv.ParseInt(network.ID, 10, 64)
	if err != nil {
		log.Fatalln(err)
	}
	resp, err := n.Client.Network.Delete(context.TODO(), &hcloud.Network{
		ID: networkId,
	})
	if err != nil {
		log.Println(err)
		return err
	}
	log.Println("[DEBUG] ", resp)
	return nil
}

func (n NetworkProviderHetzner) Create(network Network) (Network, error) {
	_, ipNet, err := net.ParseCIDR(network.CIDR)
	log.Println("[DEBUG] ipNet.IP:", ipNet.IP.String())
	log.Println("[DEBUG] ipNet.Mask:", ipNet.Mask.String())
	if err != nil {
		log.Fatalln(err)
	}
	net, resp, err := n.Client.Network.Create(context.TODO(), hcloud.NetworkCreateOpts{
		Name:    network.Name,
		IPRange: ipNet,
		Labels: map[string]string{
			"Owner": "onctl",
		},
	})
	log.Println("[DEBUG] response:", resp)
	if err != nil {
		log.Println(err)
		return Network{}, err
	}
	log.Println("[DEBUG] ", net)

	subnet, resp, err := n.Client.Network.AddSubnet(context.TODO(), net, hcloud.NetworkAddSubnetOpts{
		Subnet: hcloud.NetworkSubnet{
			Type:        hcloud.NetworkSubnetTypeCloud,
			IPRange:     ipNet,
			NetworkZone: hcloud.NetworkZoneEUCentral, //TODO: make this configurable based on vm location ex. fsn1
		},
	})
	log.Println("[DEBUG] zone:", viper.GetString("hetzner.location"))
	if err != nil {
		log.Println("Add Subnet:", err)
		return Network{}, err
	}
	log.Println("[DEBUG] subnet:", subnet)
	log.Println("[DEBUG] subnet resp:", resp)

	return mapHetznerNetwork(*net), nil
}

func (p ProviderHetzner) DetachNetwork(vm Vm, network Network) error {
	log.Println("[DEBUG] Detaching network: ", network)
	vm, err := p.GetByName(vm.Name)
	if err != nil {
		log.Println(err)
		return err
	}

	networkId, err := strconv.ParseInt(network.ID, 10, 64)
	if err != nil {
		log.Fatalln(err)
	}
	serverId, err := strconv.ParseInt(vm.ID, 10, 64)
	if err != nil {
		log.Fatalln(err)
	}

	action, _, err := p.Client.Server.DetachFromNetwork(context.TODO(), &hcloud.Server{
		ID: serverId,
	}, hcloud.ServerDetachFromNetworkOpts{
		Network: &hcloud.Network{
			ID: networkId,
		},
	})
	if err != nil {
		log.Println(err)
		return err
	}
	log.Println("[DEBUG] ", action)
	return nil
}

func (p ProviderHetzner) AttachNetwork(vm Vm, network Network) error {
	log.Println("[DEBUG] Attaching network: ", network)
	vm, err := p.GetByName(vm.Name)
	if err != nil {
		log.Println(err)
		return err
	}

	networkId, err := strconv.ParseInt(network.ID, 10, 64)
	if err != nil {
		log.Fatalln(err)
	}
	serverId, err := strconv.ParseInt(vm.ID, 10, 64)
	if err != nil {
		log.Fatalln(err)
	}

	action, _, err := p.Client.Server.AttachToNetwork(context.TODO(), &hcloud.Server{
		ID: serverId,
	}, hcloud.ServerAttachToNetworkOpts{
		Network: &hcloud.Network{
			ID: networkId,
		},
	})
	if err != nil {
		log.Println(err)
		return err
	}
	log.Println("[DEBUG] ", action)
	return nil
}

func (p ProviderHetzner) Deploy(server Vm) (Vm, error) {

	log.Println("[DEBUG] Deploy server: ", server)
	sshKeyIDint, err := strconv.ParseInt(server.SSHKeyID, 10, 64)
	if err != nil {
		log.Fatalln(err)
	}
	result, _, err := p.Client.Server.Create(context.TODO(), hcloud.ServerCreateOpts{
		Name: server.Name,
		Location: &hcloud.Location{
			Name: viper.GetString("hetzner.location"),
		},
		Image: &hcloud.Image{
			Name: "ubuntu-22.04",
		},
		ServerType: &hcloud.ServerType{
			Name: viper.GetString("hetzner.vm.type"),
		},
		SSHKeys: []*hcloud.SSHKey{
			{
				ID: sshKeyIDint,
			},
		},
		Labels: map[string]string{
			"Owner": "onctl",
		},
		UserData: tools.FileToBase64(server.CloudInitFile),
	})
	if err != nil {
		if herr, ok := err.(hcloud.Error); ok {
			switch herr.Code {
			case hcloud.ErrorCodeUniquenessError:
				log.Println("Server already exists")
				s, _, err := p.Client.Server.GetByName(context.TODO(), server.Name)
				if err != nil {
					log.Fatalln(err)
				}
				return mapHetznerServer(*s), nil
			default:
				fmt.Println(herr.Error())
			}
		} else {
			fmt.Println(err.Error())
		}
		log.Fatalln(err)
	}
	return mapHetznerServer(*result.Server), nil
}

func (p ProviderHetzner) Destroy(server Vm) error {
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
	id, err := strconv.ParseInt(server.ID, 10, 64)
	if err != nil {
		log.Fatalln(err)
	}
	_, _, err = p.Client.Server.DeleteWithResult(context.TODO(), &hcloud.Server{
		ID: id,
	})
	if err != nil {
		log.Fatalln(err)
	}
	return nil
}

func (p ProviderHetzner) List() (VmList, error) {
	log.Println("[DEBUG] List Servers")
	list, _, err := p.Client.Server.List(context.TODO(), hcloud.ServerListOpts{
		ListOpts: hcloud.ListOpts{
			LabelSelector: "Owner=onctl",
		},
	})
	if err != nil {
		log.Println(err)
	}
	if len(list) == 0 {
		return VmList{}, nil
	}
	cloudList := make([]Vm, 0, len(list))
	for _, server := range list {
		cloudList = append(cloudList, mapHetznerServer(*server))
		log.Println("[DEBUG] server: ", server)
	}
	output := VmList{
		List: cloudList,
	}
	return output, nil
}

func (p ProviderHetzner) CreateSSHKey(publicKeyFile string) (keyID string, err error) {
	publicKey, err := os.ReadFile(publicKeyFile)
	if err != nil {
		log.Fatalln(err)
	}

	SSHKeyMD5 := fmt.Sprintf("%x", md5.Sum(publicKey))
	pk, _, _, _, err := ssh.ParseAuthorizedKey(publicKey)
	if err != nil {
		panic(err)
	}

	// Get the fingerprint
	SSHKeyFingerPrint := ssh.FingerprintLegacyMD5(pk)

	// Print the fingerprint
	log.Println("[DEBUG] SSH Key Fingerpring: " + SSHKeyFingerPrint)
	log.Println("[DEBUG] SSH Key MD5: " + SSHKeyMD5)
	// fmt.Println("Creating SSHKey: " + "onctl-" + SSHKeyMD5[:8] + "...")
	hkey, _, err := p.Client.SSHKey.Create(context.TODO(), hcloud.SSHKeyCreateOpts{
		Name:      "onctl-" + SSHKeyMD5[:8],
		PublicKey: string(publicKey),
	})
	if err != nil {
		if herr, ok := err.(hcloud.Error); ok {
			switch herr.Code {
			case hcloud.ErrorCodeUniquenessError:
				log.Println("[DEBUG] SSH Key already exists (onctl-" + SSHKeyMD5[:8] + ")")
				key, _, err := p.Client.SSHKey.GetByFingerprint(context.TODO(), SSHKeyFingerPrint)
				if err != nil {
					log.Fatalln(err)
				}
				log.Println("[DEBUG] SSH Key ID: " + strconv.FormatInt(key.ID, 10))
				return fmt.Sprint(key.ID), nil
			default:
				fmt.Println(herr.Error())
			}
		} else {
			fmt.Println(err.Error())
		}
		log.Fatalln(err)
	}
	// fmt.Println("DONE")
	return fmt.Sprint(hkey.ID), nil
}

func mapHetznerNetwork(network hcloud.Network) Network {
	return Network{
		Provider:  "hetzner",
		ID:        strconv.FormatInt(network.ID, 10),
		Name:      network.Name,
		CIDR:      network.IPRange.String(),
		CreatedAt: network.Created,
		Servers:   len(network.Servers),
	}
}

// mapHetznerServer gets a hcloud.Server and returns a Vm
func mapHetznerServer(server hcloud.Server) Vm {
	acculumatedCost := 0.0
	costPerHour := 0.0
	costPerMonth := 0.0
	currency := "EUR"
	for _, p := range server.ServerType.Pricings {
		if p.Location.Name == server.Datacenter.Location.Name {
			uptime := time.Since(server.Created)
			hourlyGross, _ := strconv.ParseFloat(p.Hourly.Gross, 64) // Convert p.Hourly.Gross to float64
			acculumatedCost = math.Round(hourlyGross*uptime.Hours()*10000) / 10000
			costPerHour, _ = strconv.ParseFloat(p.Hourly.Gross, 64)
			costPerMonth, _ = strconv.ParseFloat(p.Monthly.Gross, 64)
		}
	}
	var privateIP string
	if len(server.PrivateNet) == 0 {
		privateIP = "N/A"
	} else {
		privateIP = server.PrivateNet[0].IP.String()
	}

	return Vm{
		Provider:  "hetzner",
		ID:        strconv.FormatInt(server.ID, 10),
		Name:      server.Name,
		IP:        server.PublicNet.IPv4.IP.String(),
		PrivateIP: privateIP,
		Type:      server.ServerType.Name,
		Status:    string(server.Status),
		CreatedAt: server.Created,
		Location:  server.Datacenter.Location.Name,
		Cost: CostStruct{
			Currency:        currency,
			CostPerHour:     costPerHour,
			CostPerMonth:    costPerMonth,
			AccumulatedCost: acculumatedCost,
		},
	}
}

func (p ProviderHetzner) GetByName(serverName string) (Vm, error) {
	s, _, err := p.Client.Server.GetByName(context.TODO(), serverName)
	if err != nil {
		return Vm{}, err
	}
	if s == nil {
		return Vm{}, errors.New("No Server found with name: " + serverName)
	}
	return mapHetznerServer(*s), nil
}

func (p ProviderHetzner) SSHInto(serverName string, port int, privateKey string) {
	server, _, err := p.Client.Server.GetByName(context.TODO(), serverName)
	if server == nil {
		fmt.Println("No Server found with name: " + serverName)
		os.Exit(1)
	}

	if err != nil {
		if herr, ok := err.(hcloud.Error); ok {
			switch herr.Code {
			case hcloud.ErrorCodeNotFound:
				log.Fatalln("Server not found")
			default:
				log.Fatalln(herr.Error())
			}
		} else {
			log.Fatalln(err.Error())
		}
	}

	if privateKey == "" {
		privateKey = viper.GetString("ssh.privateKey")
	}
	tools.SSHIntoVM(tools.SSHIntoVMRequest{
		IPAddress:      server.PublicNet.IPv4.IP.String(),
		User:           viper.GetString("hetzner.vm.username"),
		Port:           port,
		PrivateKeyFile: privateKey,
	})
}
