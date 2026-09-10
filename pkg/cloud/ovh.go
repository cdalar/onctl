package cloud

import (
	"crypto/md5"
	"errors"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	"github.com/cdalar/onctl/internal/tools"

	"github.com/ovh/go-ovh/ovh"
	"golang.org/x/crypto/ssh"
)

type OvhConfig struct {
	ServiceName   string
	Region        string
	VMType        string
	Image         string
	Username      string
	SSHPrivateKey string
}

type ProviderOvh struct {
	Client *ovh.Client
	Config OvhConfig
}

// -- OVH Public Cloud API request/response shapes --
// Field names verified against the live OVH API schema
// (https://eu.api.ovh.com/1.0/cloud.json), since go-ovh is a generic REST
// wrapper with no typed resource models of its own (unlike hcloud-go).

type ovhFlavor struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Region    string `json:"region"`
	VCPUs     int    `json:"vcpus"`
	RAM       int    `json:"ram"`
	Disk      int    `json:"disk"`
	Available bool   `json:"available"`
}

type ovhImage struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	Region string `json:"region"`
	Type   string `json:"type"`
}

type ovhSSHKey struct {
	ID        string   `json:"id"`
	Name      string   `json:"name"`
	PublicKey string   `json:"publicKey"`
	Regions   []string `json:"regions"`
}

type ovhSSHKeyCreate struct {
	Name      string `json:"name"`
	PublicKey string `json:"publicKey"`
}

type ovhInstanceCreate struct {
	Name           string `json:"name"`
	FlavorID       string `json:"flavorId"`
	ImageID        string `json:"imageId,omitempty"`
	Region         string `json:"region"`
	SSHKeyID       string `json:"sshKeyId,omitempty"`
	MonthlyBilling bool   `json:"monthlyBilling"`
	UserData       string `json:"userData,omitempty"`
}

type ovhIPAddress struct {
	IP      string `json:"ip"`
	Type    string `json:"type"`
	Version int    `json:"version"`
}

type ovhInstance struct {
	ID          string         `json:"id"`
	Name        string         `json:"name"`
	Status      string         `json:"status"`
	Region      string         `json:"region"`
	FlavorID    string         `json:"flavorId"`
	ImageID     string         `json:"imageId"`
	IPAddresses []ovhIPAddress `json:"ipAddresses"`
	Created     time.Time      `json:"created"`
}

// ovhStatusActive/ovhStatusError/ovhStatusShelved* mirror the
// cloud.instance.InstanceStatusEnum values from the OVH API schema that this
// provider cares about.
const (
	ovhStatusActive           = "ACTIVE"
	ovhStatusError            = "ERROR"
	ovhStatusShelved          = "SHELVED"
	ovhStatusShelvedOffloaded = "SHELVED_OFFLOADED"
)

func (p ProviderOvh) projectPath(suffix string) string {
	return fmt.Sprintf("/cloud/project/%s%s", p.Config.ServiceName, suffix)
}

func (p ProviderOvh) Deploy(server Vm) (Vm, error) {
	log.Println("[DEBUG] Deploy server: ", server)

	region := p.Config.Region
	vmType := server.Type
	if vmType == "" {
		vmType = p.Config.VMType
	}
	image := server.Image
	if image == "" {
		image = p.Config.Image
	}

	flavorID, err := p.resolveFlavorID(region, vmType)
	if err != nil {
		return Vm{}, err
	}
	var imageID string
	if image != "" {
		imageID, err = p.resolveImageID(region, image)
		if err != nil {
			return Vm{}, err
		}
	}

	// OVH's userData field is documented as free-form "text"; every other
	// provider in this codebase sends cloud-init as base64 (tools.FileToBase64),
	// so this follows that convention until verified against a live account
	// (see docs/plans/ovh-provider.md's Verification section).
	req := ovhInstanceCreate{
		Name:           server.Name,
		FlavorID:       flavorID,
		ImageID:        imageID,
		Region:         region,
		SSHKeyID:       server.SSHKeyID,
		MonthlyBilling: false,
		UserData:       tools.FileToBase64(server.CloudInitFile),
	}

	var created ovhInstance
	if err := p.Client.Post(p.projectPath("/instance"), req, &created); err != nil {
		log.Fatalln(err)
	}

	final, err := p.waitForStatus(created.ID, ovhStatusActive, 10*time.Minute)
	if err != nil {
		return Vm{}, err
	}
	return mapOvhInstance(final), nil
}

func (p ProviderOvh) Destroy(server Vm) error {
	log.Println("[DEBUG] Destroy server: ", server)
	id := server.ID
	if id == "" && server.Name != "" {
		s, err := p.GetByName(server.Name)
		if err != nil {
			return err
		}
		id = s.ID
	}
	if err := p.Client.Delete(p.projectPath("/instance/"+id), nil); err != nil {
		return fmt.Errorf("deleting instance: %w", err)
	}
	return nil
}

// Pause shelves the instance. OVH's shelve/unshelve already frees the
// compute resource server-side (stopping billing) the way Hetzner's manual
// snapshot+delete dance does by hand, so there is no separate "hot" shutdown
// path to implement here -- hot is accepted only to satisfy
// CloudProviderInterface.
func (p ProviderOvh) Pause(server Vm, hot bool) error {
	log.Println("[DEBUG] Pause server: ", server.Name)
	id := server.ID
	if id == "" && server.Name != "" {
		s, err := p.GetByName(server.Name)
		if err != nil {
			return err
		}
		id = s.ID
	}
	if err := p.Client.Post(p.projectPath("/instance/"+id+"/shelve"), nil, nil); err != nil {
		return fmt.Errorf("shelving instance: %w", err)
	}
	return nil
}

func (p ProviderOvh) Resume(server Vm) (Vm, error) {
	log.Println("[DEBUG] Resume server: ", server.Name)
	id := server.ID
	if id == "" && server.Name != "" {
		s, err := p.GetByName(server.Name)
		if err != nil {
			return Vm{}, err
		}
		id = s.ID
	}
	if err := p.Client.Post(p.projectPath("/instance/"+id+"/unshelve"), nil, nil); err != nil {
		return Vm{}, fmt.Errorf("unshelving instance: %w", err)
	}
	final, err := p.waitForStatus(id, ovhStatusActive, 10*time.Minute)
	if err != nil {
		return Vm{}, err
	}
	return mapOvhInstance(final), nil
}

// ListPaused returns the instances OVH reports as shelved. Unlike Hetzner,
// OVH keeps shelved instances listed (no separate snapshot bookkeeping
// needed): List and ListPaused both read from the same instance listing.
func (p ProviderOvh) ListPaused() (VmList, error) {
	instances, err := p.listInstances()
	if err != nil {
		return VmList{}, err
	}
	cloudList := make([]Vm, 0)
	for _, inst := range instances {
		if inst.Status == ovhStatusShelved || inst.Status == ovhStatusShelvedOffloaded {
			cloudList = append(cloudList, mapOvhInstance(inst))
		}
	}
	return VmList{List: cloudList}, nil
}

// List returns every instance in the configured OVH project. OVH's Public
// Cloud instance API has no tags/labels (confirmed against the API schema),
// so unlike hetzner/aws/gcp/azure this provider cannot scope to an
// "Owner=onctl" selector -- the whole configured project is treated as
// onctl's (see the "OVHcloud" section of onctl.yaml and README).
func (p ProviderOvh) List() (VmList, error) {
	instances, err := p.listInstances()
	if err != nil {
		return VmList{}, err
	}
	cloudList := make([]Vm, 0, len(instances))
	for _, inst := range instances {
		cloudList = append(cloudList, mapOvhInstance(inst))
	}
	return VmList{List: cloudList}, nil
}

func (p ProviderOvh) listInstances() ([]ovhInstance, error) {
	var instances []ovhInstance
	if err := p.Client.Get(p.projectPath("/instance"), &instances); err != nil {
		return nil, fmt.Errorf("listing instances: %w", err)
	}
	return instances, nil
}

func (p ProviderOvh) GetByName(serverName string) (Vm, error) {
	instances, err := p.listInstances()
	if err != nil {
		return Vm{}, err
	}
	for _, inst := range instances {
		if inst.Name == serverName {
			return mapOvhInstance(inst), nil
		}
	}
	return Vm{}, errors.New("No Server found with name: " + serverName)
}

// CreateSSHKey registers publicKeyFile with OVH, reusing an existing key
// (matched by key material, not just name) instead of relying on a specific
// OVH conflict error code we haven't confirmed against a live account.
func (p ProviderOvh) CreateSSHKey(publicKeyFile string) (keyID string, err error) {
	publicKey, err := os.ReadFile(publicKeyFile)
	if err != nil {
		log.Fatalln(err)
	}
	pk, _, _, _, err := ssh.ParseAuthorizedKey(publicKey)
	if err != nil {
		panic(err)
	}
	fingerprint := ssh.FingerprintSHA256(pk)

	var keys []ovhSSHKey
	if err := p.Client.Get(p.projectPath("/sshkey"), &keys); err != nil {
		return "", fmt.Errorf("listing ssh keys: %w", err)
	}
	for _, k := range keys {
		existingPk, _, _, _, err := ssh.ParseAuthorizedKey([]byte(k.PublicKey))
		if err != nil {
			continue
		}
		if ssh.FingerprintSHA256(existingPk) == fingerprint {
			log.Println("[DEBUG] SSH Key already exists: " + k.ID)
			return k.ID, nil
		}
	}

	SSHKeyMD5 := fmt.Sprintf("%x", md5.Sum(publicKey))
	var created ovhSSHKey
	if err := p.Client.Post(p.projectPath("/sshkey"), ovhSSHKeyCreate{
		Name:      "onctl-" + SSHKeyMD5[:8],
		PublicKey: strings.TrimSpace(string(publicKey)),
	}, &created); err != nil {
		return "", fmt.Errorf("creating ssh key: %w", err)
	}
	return created.ID, nil
}

func (p ProviderOvh) SSHInto(serverName string, port int, privateKey string, command []string) {
	server, err := p.GetByName(serverName)
	if err != nil || server.ID == "" {
		fmt.Println("No Server found with name: " + serverName)
		os.Exit(1)
	}
	if privateKey == "" {
		privateKey = p.Config.SSHPrivateKey
	}
	tools.SSHIntoVM(tools.SSHIntoVMRequest{
		IPAddress:      server.IP,
		User:           p.Config.Username,
		Port:           port,
		PrivateKeyFile: privateKey,
		Command:        command,
	})
}

// resolveFlavorID looks up a flavor's opaque id by name+region: OVH selects
// flavors/images by id, not name, unlike Hetzner's ServerType.Name.
func (p ProviderOvh) resolveFlavorID(region, name string) (string, error) {
	var flavors []ovhFlavor
	if err := p.Client.Get(p.projectPath("/flavor"), &flavors); err != nil {
		return "", fmt.Errorf("listing flavors: %w", err)
	}
	for _, f := range flavors {
		if f.Region == region && f.Name == name {
			return f.ID, nil
		}
	}
	return "", fmt.Errorf("no flavor named %q found in region %q", name, region)
}

func (p ProviderOvh) resolveImageID(region, name string) (string, error) {
	var images []ovhImage
	if err := p.Client.Get(p.projectPath("/image"), &images); err != nil {
		return "", fmt.Errorf("listing images: %w", err)
	}
	for _, img := range images {
		if img.Region == region && img.Name == name {
			return img.ID, nil
		}
	}
	return "", fmt.Errorf("no image named %q found in region %q", name, region)
}

// waitForStatus polls GET /instance/{id} until it reaches want or hits a
// terminal ERROR status, bounded by timeout. OVH's instance create/shelve/
// unshelve calls are async (the instance comes back in BUILD/SHELVING/...),
// unlike Hetzner's synchronous Server.Create.
func (p ProviderOvh) waitForStatus(id string, want string, timeout time.Duration) (ovhInstance, error) {
	deadline := time.Now().Add(timeout)
	for {
		var inst ovhInstance
		if err := p.Client.Get(p.projectPath("/instance/"+id), &inst); err != nil {
			return ovhInstance{}, fmt.Errorf("getting instance %s: %w", id, err)
		}
		if inst.Status == want {
			return inst, nil
		}
		if inst.Status == ovhStatusError {
			return ovhInstance{}, fmt.Errorf("instance %s entered ERROR status", id)
		}
		if time.Now().After(deadline) {
			return ovhInstance{}, fmt.Errorf("timed out waiting for instance %s to reach %s (last status: %s)", id, want, inst.Status)
		}
		time.Sleep(3 * time.Second)
	}
}

func mapOvhInstance(inst ovhInstance) Vm {
	var ip, privateIP string
	for _, addr := range inst.IPAddresses {
		switch addr.Type {
		case "public":
			ip = addr.IP
		case "private":
			privateIP = addr.IP
		}
	}
	if privateIP == "" {
		privateIP = "N/A"
	}
	return Vm{
		Provider:  "ovh",
		ID:        inst.ID,
		Name:      inst.Name,
		IP:        ip,
		PrivateIP: privateIP,
		Type:      inst.FlavorID,
		Status:    inst.Status,
		Location:  inst.Region,
		CreatedAt: inst.Created,
		// No inline pricing in this API (unlike Hetzner's flavor.Pricings) --
		// left zero rather than fabricated, matching fc/ch's local-VM convention.
	}
}
