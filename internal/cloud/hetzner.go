package cloud

import (
	"context"
	"crypto/md5"
	"errors"
	"fmt"
	"log"
	"math"
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

func (p ProviderHetzner) Locations() ([]Location, error) {
	log.Println("[DEBUG] Get Locations")
	list, err := p.Client.Location.All(context.TODO())
	if err != nil {
		log.Println(err)
	}
	if len(list) == 0 {
		return []Location{}, nil
	}
	locationList := make([]Location, 0, len(list))
	for _, location := range list {
		locationList = append(locationList, Location{
			Name:     location.Name,
			Endpoint: location.Name + "-speed.hetzner.com:80",
		})
	}
	return locationList, nil
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
	_, _, err = p.Client.SSHKey.Create(context.TODO(), hcloud.SSHKeyCreateOpts{
		Name:      "onctl-" + SSHKeyMD5[:8],
		PublicKey: string(publicKey),
	})
	if err != nil {
		if herr, ok := err.(hcloud.Error); ok {
			switch herr.Code {
			case hcloud.ErrorCodeUniquenessError:
				// fmt.Println("SSH Key already exists (onctl-" + SSHKeyMD5[:8] + ")")
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
	return
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

func (p ProviderHetzner) SSHInto(serverName string, port int) {
	// server, _, err := p.Client.Server().Get(ctx, idOrName)
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

	ipAddress := server.PublicNet.IPv4.IP
	tools.SSHIntoVM(ipAddress.String(), "root", port)
}
