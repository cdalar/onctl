package cloud

import (
	"cdalar/onctl/internal/tools"
	"context"
	"crypto/md5"
	"fmt"
	"log"
	"os"
	"strconv"

	"github.com/hetznercloud/hcloud-go/hcloud"
	"golang.org/x/crypto/ssh"
)

type ProviderHetzner struct {
	Client *hcloud.Client
}

func (p ProviderHetzner) Deploy(server Vm) (Vm, error) {
	if server.Type == "##" {
		server.Type = "cpx21"
	}
	sshKeyIDint, err := strconv.Atoi(server.SSHKeyID)
	if err != nil {
		log.Fatalln(err)
	}
	result, _, err := p.Client.Server.Create(context.TODO(), hcloud.ServerCreateOpts{
		Name: server.Name,
		Image: &hcloud.Image{
			Name: "ubuntu-22.04",
		},
		ServerType: &hcloud.ServerType{
			Name: server.Type,
		},
		SSHKeys: []*hcloud.SSHKey{
			{
				ID: sshKeyIDint,
			},
		},
		Labels: map[string]string{
			"Owner": "onctl",
		},
	})
	if err != nil {
		if herr, ok := err.(hcloud.Error); ok {
			switch herr.Code {
			case hcloud.ErrorCodeUniquenessError:
				log.Println("[DEBUG] Server already exists")
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
	if server.ID == "" {
		log.Println("[DEBUG] Server ID is empty")
		log.Println("[DEBUG] Server name: " + server.Name)
		s := p.getServerByServerName(server.Name)
		if s.ID == "" {
			log.Println("[DEBUG] Server not found")
			return nil
		}
		log.Println("[DEBUG] Server found ID: " + s.ID)
		server.ID = s.ID
	}
	id, err := strconv.Atoi(server.ID)
	if err != nil {
		log.Fatalln(err)
	}
	_, _, err = p.Client.Server.DeleteWithResult(context.TODO(), &hcloud.Server{
		ID: id,
	})
	if err != nil {
		log.Fatalln(err)
	}
	log.Println("Server deleted")
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
		log.Println("[DEBUG] server name: " + server.Name)
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
	fmt.Println("Creating SSHKey: " + "onctl-" + SSHKeyMD5[:8] + "...")
	_, _, err = p.Client.SSHKey.Create(context.TODO(), hcloud.SSHKeyCreateOpts{
		Name:      "onctl-" + SSHKeyMD5[:8],
		PublicKey: string(publicKey),
	})
	if err != nil {
		if herr, ok := err.(hcloud.Error); ok {
			switch herr.Code {
			case hcloud.ErrorCodeUniquenessError:
				fmt.Println("SSH Key already exists (onctl-" + SSHKeyMD5[:8] + ")")
				key, _, err := p.Client.SSHKey.GetByFingerprint(context.TODO(), SSHKeyFingerPrint)
				if err != nil {
					log.Fatalln(err)
				}
				log.Println("[DEBUG] SSH Key ID: " + strconv.Itoa(key.ID))
				return fmt.Sprint(key.ID), nil
			default:
				fmt.Println(herr.Error())
			}
		} else {
			fmt.Println(err.Error())
		}
		log.Fatalln(err)
	}
	fmt.Println("DONE")
	return
}

// mapHetznerServer gets a hcloud.Server and returns a Vm
func mapHetznerServer(server hcloud.Server) Vm {
	return Vm{
		ID:        strconv.Itoa(server.ID),
		Name:      server.Name,
		IP:        server.PublicNet.IPv4.IP.String(),
		Type:      server.ServerType.Name,
		Status:    string(server.Status),
		CreatedAt: server.Created,
	}
}

func (p ProviderHetzner) getServerByServerName(serverName string) Vm {
	s, _, err := p.Client.Server.GetByName(context.TODO(), serverName)
	if err != nil {
		log.Fatalln(err)
	}
	if s == nil {
		fmt.Println("No Server found with name: " + serverName)
		return Vm{}
	}
	return mapHetznerServer(*s)
}

func (p ProviderHetzner) SSHInto(serverName string) {
	// server, _, err := p.Client.Server().Get(ctx, idOrName)
	server, _, err := p.Client.Server.GetByName(context.TODO(), serverName)
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
	tools.SSHIntoVM(ipAddress.String(), "root")
}
