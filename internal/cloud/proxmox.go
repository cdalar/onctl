package cloud

import (
	"context"
	"crypto/md5"
	"errors"
	"fmt"
	"log"
	"os"
	"strconv"
	"time"

	"github.com/cdalar/onctl/internal/tools"
	pxapi "github.com/Telmate/proxmox-api-go/proxmox"
	"github.com/spf13/viper"
	"golang.org/x/crypto/ssh"
)

type ProviderProxmox struct {
	Client *pxapi.Client
}

func (p ProviderProxmox) Deploy(server Vm) (Vm, error) {
	log.Println("[DEBUG] Deploy server: ", server)
	ctx := context.Background()

	// Get configuration values
	node := viper.GetString("proxmox.node")
	vmID := viper.GetInt("proxmox.vm.id")
	template := viper.GetString("proxmox.vm.template")
	cores := viper.GetInt("proxmox.vm.cores")
	memory := viper.GetInt("proxmox.vm.memory")
	storage := viper.GetString("proxmox.vm.storage")
	networkBridge := viper.GetString("proxmox.vm.network_bridge")

	// Check if VM already exists
	vmRef := pxapi.NewVmRef(pxapi.GuestID(vmID))
	vmRef.SetNode(node)
	_, err := p.Client.GetVmInfo(ctx, vmRef)
	if err == nil {
		log.Println("VM already exists with ID:", vmID)
		return p.getVMInfo(vmRef, server.Name)
	}

	// Find template VM
	vmList, err := p.Client.GetVmList(ctx)
	if err != nil {
		return Vm{}, fmt.Errorf("failed to get VM list: %v", err)
	}

	var templateVmID int
	for _, vmInfo := range vmList["data"].([]interface{}) {
		vm := vmInfo.(map[string]interface{})
		if vm["name"] == template {
			templateVmID = int(vm["vmid"].(float64))
			break
		}
	}

	if templateVmID == 0 {
		return Vm{}, fmt.Errorf("template %s not found", template)
	}

	// Clone from template
	sourceVmr := pxapi.NewVmRef(pxapi.GuestID(templateVmID))
	sourceVmr.SetNode(node)

	// Configure clone parameters
	cloneParams := map[string]interface{}{
		"newid":   vmID,
		"name":    server.Name,
		"target":  node,
		"full":    1,
		"storage": storage,
	}

	// Clone the VM
	log.Println("[DEBUG] Cloning template", template, "to new VM", server.Name)
	_, err = p.Client.CloneQemuVm(ctx, sourceVmr, cloneParams)
	if err != nil {
		return Vm{}, fmt.Errorf("failed to clone VM: %v", err)
	}

	// Update VM configuration
	newVmRef := pxapi.NewVmRef(pxapi.GuestID(vmID))
	newVmRef.SetNode(node)

	// Set basic configuration via API call
	config := map[string]interface{}{
		"cores":       cores,
		"memory":      memory,
		"description": "Created by onctl",
		"tags":        "onctl",
	}

	// Add SSH key if provided
	if server.SSHKeyID != "" {
		publicKey, err := os.ReadFile(server.SSHKeyID)
		if err == nil {
			config["sshkeys"] = string(publicKey)
		}
	}

	// Add cloud-init if provided
	if server.CloudInitFile != "" {
		config["ciuser"] = viper.GetString("proxmox.vm.username")
	}

	// Configure network
	config["net0"] = fmt.Sprintf("virtio,bridge=%s", networkBridge)

	_, err = p.Client.SetVmConfig(newVmRef, config)
	if err != nil {
		return Vm{}, fmt.Errorf("failed to update VM config: %v", err)
	}

	// Start the VM
	_, err = p.Client.StartVm(ctx, newVmRef)
	if err != nil {
		return Vm{}, fmt.Errorf("failed to start VM: %v", err)
	}

	// Wait for VM to start
	time.Sleep(5 * time.Second)

	return p.getVMInfo(newVmRef, server.Name)
}

func (p ProviderProxmox) Destroy(server Vm) error {
	log.Println("[DEBUG] Destroy server: ", server)

	if server.ID == "" && server.Name != "" {
		log.Println("[DEBUG] Server ID is empty, finding by name")
		s, err := p.GetByName(server.Name)
		if err != nil || s.ID == "" {
			log.Println("[DEBUG] Server not found")
			return err
		}
		server.ID = s.ID
	}

	vmID, err := strconv.Atoi(server.ID)
	if err != nil {
		return fmt.Errorf("invalid VM ID: %v", err)
	}

	vmRef := pxapi.NewVmRef(pxapi.GuestID(vmID))
	vmRef.SetNode(viper.GetString("proxmox.node"))

	ctx := context.Background()

	// Stop VM first
	_, err = p.Client.StopVm(ctx, vmRef)
	if err != nil {
		log.Println("[DEBUG] Failed to stop VM (may already be stopped):", err)
	}

	// Wait for VM to stop
	time.Sleep(3 * time.Second)

	// Delete VM
	_, err = p.Client.DeleteVm(ctx, vmRef)
	if err != nil {
		return fmt.Errorf("failed to delete VM: %v", err)
	}

	return nil
}

func (p ProviderProxmox) List() (VmList, error) {
	log.Println("[DEBUG] List Servers")
	ctx := context.Background()

	vmList, err := p.Client.GetVmList(ctx)
	if err != nil {
		return VmList{}, err
	}

	cloudList := make([]Vm, 0)
	for _, vmInfo := range vmList["data"].([]interface{}) {
		vm := vmInfo.(map[string]interface{})

		// Filter by onctl tag
		tags, ok := vm["tags"].(string)
		if !ok || tags != "onctl" {
			continue
		}

		vmID := int(vm["vmid"].(float64))
		vmRef := pxapi.NewVmRef(pxapi.GuestID(vmID))
		vmRef.SetNode(vm["node"].(string))

		vmData, err := p.getVMInfo(vmRef, vm["name"].(string))
		if err != nil {
			log.Println("[DEBUG] Error getting VM info:", err)
			continue
		}

		cloudList = append(cloudList, vmData)
	}

	return VmList{List: cloudList}, nil
}

func (p ProviderProxmox) CreateSSHKey(publicKeyFile string) (keyID string, err error) {
	// Proxmox doesn't have a centralized SSH key store like cloud providers
	// We'll just return the public key file path to be used during VM creation
	publicKey, err := os.ReadFile(publicKeyFile)
	if err != nil {
		return "", err
	}

	SSHKeyMD5 := fmt.Sprintf("%x", md5.Sum(publicKey))
	pk, _, _, _, err := ssh.ParseAuthorizedKey(publicKey)
	if err != nil {
		return "", err
	}

	SSHKeyFingerPrint := ssh.FingerprintLegacyMD5(pk)
	log.Println("[DEBUG] SSH Key Fingerprint:", SSHKeyFingerPrint)
	log.Println("[DEBUG] SSH Key MD5:", SSHKeyMD5)

	// Return the public key file path
	return publicKeyFile, nil
}

func (p ProviderProxmox) GetByName(serverName string) (Vm, error) {
	ctx := context.Background()
	vmList, err := p.Client.GetVmList(ctx)
	if err != nil {
		return Vm{}, err
	}

	for _, vmInfo := range vmList["data"].([]interface{}) {
		vm := vmInfo.(map[string]interface{})
		if vm["name"] == serverName {
			// Check if it has the onctl tag
			tags, ok := vm["tags"].(string)
			if ok && tags == "onctl" {
				vmID := int(vm["vmid"].(float64))
				vmRef := pxapi.NewVmRef(pxapi.GuestID(vmID))
				vmRef.SetNode(vm["node"].(string))
				return p.getVMInfo(vmRef, serverName)
			}
		}
	}

	return Vm{}, errors.New("no server found with name: " + serverName)
}

func (p ProviderProxmox) SSHInto(serverName string, port int, privateKey string) {
	server, err := p.GetByName(serverName)
	if err != nil {
		fmt.Println("No server found with name:", serverName)
		os.Exit(1)
	}

	if privateKey == "" {
		privateKey = viper.GetString("ssh.privateKey")
	}

	tools.SSHIntoVM(tools.SSHIntoVMRequest{
		IPAddress:      server.IP,
		User:           viper.GetString("proxmox.vm.username"),
		Port:           port,
		PrivateKeyFile: privateKey,
	})
}

func (p ProviderProxmox) getVMInfo(vmRef *pxapi.VmRef, name string) (Vm, error) {
	ctx := context.Background()
	vmInfo, err := p.Client.GetVmInfo(ctx, vmRef)
	if err != nil {
		return Vm{}, err
	}

	// Extract IP address
	var ipAddress string
	var privateIP string

	// Get network interfaces
	if networks, ok := vmInfo["network"]; ok {
		netMap := networks.(map[string]interface{})
		for _, netInfo := range netMap {
			if netData, ok := netInfo.(map[string]interface{}); ok {
				if ip, ok := netData["ip-address"]; ok && ip != nil {
					ipStr := ip.(string)
					if ipStr != "" && ipAddress == "" {
						ipAddress = ipStr
						privateIP = ipStr
					}
				}
			}
		}
	}

	status := "unknown"
	if vmInfo["status"] != nil {
		status = vmInfo["status"].(string)
	}

	vmType := "N/A"
	if vmInfo["cores"] != nil && vmInfo["memory"] != nil {
		cores := int(vmInfo["cores"].(float64))
		memoryMB := int(vmInfo["memory"].(float64))
		vmType = fmt.Sprintf("%dC/%dG", cores, memoryMB/1024)
	}

	return Vm{
		Provider:  "proxmox",
		ID:        strconv.Itoa(int(vmRef.VmId())),
		Name:      name,
		IP:        ipAddress,
		PrivateIP: privateIP,
		Type:      vmType,
		Status:    status,
		CreatedAt: time.Now(), // Proxmox doesn't provide creation time easily
		Location:  string(vmRef.Node()),
		Cost: CostStruct{
			Currency:        "N/A",
			CostPerHour:     0,
			CostPerMonth:    0,
			AccumulatedCost: 0,
		},
	}, nil
}
