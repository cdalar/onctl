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

	// Use server.Type if provided, otherwise fall back to config
	serverType := server.Type
	if serverType == "" {
		serverType = viper.GetString("hetzner.vm.type")
	}

	result, _, err := p.Client.Server.Create(context.TODO(), hcloud.ServerCreateOpts{
		Name: server.Name,
		Location: &hcloud.Location{
			Name: viper.GetString("hetzner.location"),
		},
		Image: &hcloud.Image{
			Name: viper.GetString("hetzner.vm.image"),
		},
		ServerType: &hcloud.ServerType{
			Name: serverType,
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

func (p ProviderHetzner) SSHInto(serverName string, port int, privateKey string, command []string) {
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
		Command:        command,
	})
}

const (
	labelOwner      = "Owner"
	labelSnapshot   = "onctl-snapshot"
	labelServerType = "onctl-server-type"
	labelLocation   = "onctl-location"
)

// Pause snapshots the server's disk and then deletes the server so it stops
// accruing compute cost (Hetzner bills powered-off servers). The primary IP(s)
// are preserved (auto-delete disabled and labeled) so Resume can re-attach them
// and keep the same public address. Unless hot is true, the server is gracefully
// shut down before the snapshot so it is application-consistent.
func (p ProviderHetzner) Pause(server Vm, hot bool) error {
	log.Println("[DEBUG] Pause server: ", server.Name)
	s, _, err := p.Client.Server.GetByName(context.TODO(), server.Name)
	if err != nil {
		return err
	}
	if s == nil {
		return errors.New("No Server found with name: " + server.Name)
	}

	// Remove any previous pause snapshot for this name so they don't accumulate.
	if old, _ := p.findPauseSnapshot(server.Name); old != nil {
		log.Printf("[DEBUG] deleting previous pause snapshot %d\n", old.ID)
		if _, err := p.Client.Image.Delete(context.TODO(), old); err != nil {
			log.Println("[DEBUG] could not delete previous snapshot: ", err)
		}
	}

	// Gracefully shut down first (unless --hot) so the snapshot is consistent.
	if !hot {
		if err := p.shutdownServer(s); err != nil {
			return fmt.Errorf("shutting down server: %w", err)
		}
	}

	desc := "onctl pause: " + server.Name
	result, _, err := p.Client.Server.CreateImage(context.TODO(), s, &hcloud.ServerCreateImageOpts{
		Type:        hcloud.ImageTypeSnapshot,
		Description: &desc,
		Labels: map[string]string{
			labelOwner:      "onctl",
			labelSnapshot:   server.Name,
			labelServerType: s.ServerType.Name,
			labelLocation:   s.Datacenter.Location.Name,
		},
	})
	if err != nil {
		return fmt.Errorf("creating snapshot: %w", err)
	}
	log.Println("[DEBUG] waiting for snapshot to complete...")
	if err := p.Client.Action.WaitFor(context.TODO(), result.Action); err != nil {
		return fmt.Errorf("snapshot did not complete: %w", err)
	}

	// Preserve the primary IP(s): disable auto-delete and tag them so Resume
	// can find and re-attach them after the server is gone.
	for _, ipID := range []int64{s.PublicNet.IPv4.ID, s.PublicNet.IPv6.ID} {
		if ipID == 0 {
			continue
		}
		_, _, err := p.Client.PrimaryIP.Update(context.TODO(), &hcloud.PrimaryIP{ID: ipID}, hcloud.PrimaryIPUpdateOpts{
			AutoDelete: hcloud.Ptr(false),
			Labels: hcloud.Ptr(map[string]string{
				labelOwner:    "onctl",
				labelSnapshot: server.Name,
			}),
		})
		if err != nil {
			log.Printf("[DEBUG] could not preserve primary IP %d: %v\n", ipID, err)
		}
	}

	// Delete the server. Its primary IPs survive because auto-delete is now off.
	if _, _, err := p.Client.Server.DeleteWithResult(context.TODO(), s); err != nil {
		return fmt.Errorf("deleting server: %w", err)
	}
	return nil
}

// shutdownServer gracefully powers off the server (ACPI), waiting for it to
// reach the "off" state. If it does not stop within the timeout, it falls back
// to a hard power off so Pause never blocks indefinitely.
func (p ProviderHetzner) shutdownServer(s *hcloud.Server) error {
	log.Println("[DEBUG] gracefully shutting down server: ", s.Name)
	if _, _, err := p.Client.Server.Shutdown(context.TODO(), s); err != nil {
		return err
	}

	if p.waitForServerOff(s.ID, 60*time.Second) {
		return nil
	}

	log.Println("[DEBUG] graceful shutdown timed out; forcing power off")
	if _, _, err := p.Client.Server.Poweroff(context.TODO(), s); err != nil {
		return err
	}
	p.waitForServerOff(s.ID, 30*time.Second)
	return nil
}

// waitForServerOff polls the server status until it is off or the timeout
// elapses, returning true if it reached the off state.
func (p ProviderHetzner) waitForServerOff(id int64, timeout time.Duration) bool {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		time.Sleep(3 * time.Second)
		cur, _, err := p.Client.Server.GetByID(context.TODO(), id)
		if err != nil {
			log.Println("[DEBUG] polling server status: ", err)
			continue
		}
		if cur != nil && cur.Status == hcloud.ServerStatusOff {
			return true
		}
	}
	return false
}

// Resume recreates a server from the snapshot taken by Pause and re-attaches the
// preserved primary IP(s) when they are still reserved.
func (p ProviderHetzner) Resume(server Vm) (Vm, error) {
	log.Println("[DEBUG] Resume server: ", server.Name)
	img, err := p.findPauseSnapshot(server.Name)
	if err != nil {
		return Vm{}, err
	}
	if img == nil {
		return Vm{}, fmt.Errorf("no onctl pause snapshot found for %q", server.Name)
	}

	serverType := img.Labels[labelServerType]
	if serverType == "" {
		serverType = viper.GetString("hetzner.vm.type")
	}
	location := img.Labels[labelLocation]
	if location == "" {
		location = viper.GetString("hetzner.location")
	}

	opts := hcloud.ServerCreateOpts{
		Name:       server.Name,
		Location:   &hcloud.Location{Name: location},
		Image:      img,
		ServerType: &hcloud.ServerType{Name: serverType},
		Labels:     map[string]string{labelOwner: "onctl"},
	}
	if server.SSHKeyID != "" {
		if id, err := strconv.ParseInt(server.SSHKeyID, 10, 64); err == nil {
			opts.SSHKeys = []*hcloud.SSHKey{{ID: id}}
		}
	}

	// Re-attach any preserved primary IP(s) so the public address is unchanged.
	if reserved := p.findReservedPrimaryIPs(server.Name); len(reserved) > 0 {
		pubNet := &hcloud.ServerCreatePublicNet{}
		for _, ip := range reserved {
			switch ip.Type {
			case hcloud.PrimaryIPTypeIPv4:
				pubNet.EnableIPv4 = true
				pubNet.IPv4 = ip
			case hcloud.PrimaryIPTypeIPv6:
				pubNet.EnableIPv6 = true
				pubNet.IPv6 = ip
			}
			log.Printf("[DEBUG] re-attaching primary IP %s\n", ip.IP.String())
		}
		opts.PublicNet = pubNet
	}

	result, _, err := p.Client.Server.Create(context.TODO(), opts)
	if err != nil {
		if herr, ok := err.(hcloud.Error); ok && herr.Code == hcloud.ErrorCodeUniquenessError {
			log.Println("Server already exists")
			s, _, gerr := p.Client.Server.GetByName(context.TODO(), server.Name)
			if gerr != nil {
				return Vm{}, gerr
			}
			return mapHetznerServer(*s), nil
		}
		return Vm{}, fmt.Errorf("creating server from snapshot: %w", err)
	}

	// Wait for the server to be fully running before deleting the snapshot —
	// the server boots from the snapshot, so deleting it prematurely breaks startup.
	allActions := append([]*hcloud.Action{result.Action}, result.NextActions...)
	if err := p.Client.Action.WaitFor(context.TODO(), allActions...); err != nil {
		log.Println("[DEBUG] waiting for server actions after resume: ", err)
	}

	// Delete the pause snapshot now that the server is running — keeps
	// ListPaused clean and avoids stale storage costs.
	if _, err := p.Client.Image.Delete(context.TODO(), img); err != nil {
		log.Println("[DEBUG] could not delete pause snapshot after resume: ", err)
	}

	return mapHetznerServer(*result.Server), nil
}

// findReservedPrimaryIPs returns the unassigned primary IPs preserved for the
// given server name (those tagged by Pause and not currently attached).
func (p ProviderHetzner) findReservedPrimaryIPs(name string) []*hcloud.PrimaryIP {
	ips, err := p.Client.PrimaryIP.AllWithOpts(context.TODO(), hcloud.PrimaryIPListOpts{
		ListOpts: hcloud.ListOpts{LabelSelector: labelSnapshot + "=" + name},
	})
	if err != nil {
		log.Println("[DEBUG] listing primary IPs: ", err)
		return nil
	}
	var reserved []*hcloud.PrimaryIP
	for _, ip := range ips {
		if ip.AssigneeID == 0 { // unassigned -> available to re-attach
			reserved = append(reserved, ip)
		}
	}
	return reserved
}

// ListPaused returns the servers paused via Pause, reconstructed from their
// snapshots. The preserved primary IPv4 (tagged at pause time) is shown so the
// user sees the address that will return on Resume.
func (p ProviderHetzner) ListPaused() (VmList, error) {
	images, err := p.Client.Image.AllWithOpts(context.TODO(), hcloud.ImageListOpts{
		Type:     []hcloud.ImageType{hcloud.ImageTypeSnapshot},
		ListOpts: hcloud.ListOpts{LabelSelector: labelOwner + "=onctl"},
	})
	if err != nil {
		return VmList{}, err
	}
	if len(images) == 0 {
		return VmList{}, nil
	}

	// One-shot lookup of preserved primary IPs -> map[serverName]IPv4.
	ipByName := map[string]string{}
	if ips, err := p.Client.PrimaryIP.AllWithOpts(context.TODO(), hcloud.PrimaryIPListOpts{
		ListOpts: hcloud.ListOpts{LabelSelector: labelOwner + "=onctl"},
	}); err != nil {
		log.Println("[DEBUG] listing primary IPs for paused servers: ", err)
	} else {
		for _, ip := range ips {
			if ip.Type == hcloud.PrimaryIPTypeIPv4 {
				if name := ip.Labels[labelSnapshot]; name != "" {
					ipByName[name] = ip.IP.String()
				}
			}
		}
	}

	cloudList := make([]Vm, 0, len(images))
	for _, img := range images {
		name := img.Labels[labelSnapshot]
		if name == "" {
			continue // not an onctl pause snapshot
		}
		ip := ipByName[name]
		if ip == "" {
			ip = "N/A"
		}
		cloudList = append(cloudList, Vm{
			Provider:  "hetzner",
			ID:        strconv.FormatInt(img.ID, 10),
			Name:      name,
			IP:        ip,
			PrivateIP: "N/A",
			Type:      img.Labels[labelServerType],
			Status:    "paused",
			Location:  img.Labels[labelLocation],
			CreatedAt: img.Created,
		})
	}
	return VmList{List: cloudList}, nil
}

// findPauseSnapshot returns the most recent pause snapshot for the given server
// name, or nil if none exists.
func (p ProviderHetzner) findPauseSnapshot(name string) (*hcloud.Image, error) {
	images, err := p.Client.Image.AllWithOpts(context.TODO(), hcloud.ImageListOpts{
		Type:     []hcloud.ImageType{hcloud.ImageTypeSnapshot},
		ListOpts: hcloud.ListOpts{LabelSelector: labelSnapshot + "=" + name},
	})
	if err != nil {
		return nil, err
	}
	if len(images) == 0 {
		return nil, nil
	}
	latest := images[0]
	for _, img := range images[1:] {
		if img.Created.After(latest.Created) {
			latest = img
		}
	}
	return latest, nil
}
