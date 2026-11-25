package cloud

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/arm"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/cloud"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/policy"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/compute/armcompute/v5"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/network/armnetwork/v5"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/resourcegraph/armresourcegraph"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fakeAzureCred hands out a token that never expires, so no sign-in is
// attempted.
type fakeAzureCred struct{}

func (fakeAzureCred) GetToken(context.Context, policy.TokenRequestOptions) (azcore.AccessToken, error) {
	return azcore.AccessToken{Token: "t", ExpiresOn: time.Now().Add(time.Hour)}, nil
}

// fakeARM serves Azure Resource Manager from a table of "METHOD path-part"
// to a JSON body. A path matches when it contains the part, and the first
// matching entry (in order) wins. Anything else is a 404.
type fakeARM struct {
	mu   sync.Mutex
	rows []armRow
	reqs []string
	body map[string]string // "METHOD path" -> request body
}

type armRow struct{ key, reply string }

func newFakeARM(t *testing.T, rows ...armRow) (*fakeARM, ProviderAzure) {
	t.Helper()
	f := &fakeARM{rows: rows, body: map[string]string{}}
	srv := httptest.NewTLSServer(f)
	t.Cleanup(srv.Close)

	for k, v := range map[string]string{
		"azure.resourceGroup":    "rg",
		"azure.subscriptionId":   "sub",
		"azure.location":         "westeurope",
		"azure.vm.type":          "Standard_B1s",
		"azure.vm.username":      "ubuntu",
		"azure.vm.vnet.name":     "vnet",
		"azure.vm.vnet.create":   "false",
		"azure.vm.image.sku":     "sku",
		"azure.vm.image.offer":   "offer",
		"azure.vm.image.version": "latest",
	} {
		viper.Set(k, v)
	}
	t.Cleanup(viper.Reset)

	opts := &arm.ClientOptions{ClientOptions: azcore.ClientOptions{
		Transport: srv.Client(),
		Retry:     policy.RetryOptions{MaxRetries: -1},
		Cloud: cloud.Configuration{Services: map[cloud.ServiceName]cloud.ServiceConfiguration{
			cloud.ResourceManager: {Endpoint: srv.URL, Audience: "https://management.core.windows.net/"},
		}},
	}}
	cred := fakeAzureCred{}
	var p ProviderAzure
	var err error
	p.VmClient, err = armcompute.NewVirtualMachinesClient("sub", cred, opts)
	require.NoError(t, err)
	p.SSHKeyClient, err = armcompute.NewSSHPublicKeysClient("sub", cred, opts)
	require.NoError(t, err)
	p.NicClient, err = armnetwork.NewInterfacesClient("sub", cred, opts)
	require.NoError(t, err)
	p.PublicIPClient, err = armnetwork.NewPublicIPAddressesClient("sub", cred, opts)
	require.NoError(t, err)
	p.VnetClient, err = armnetwork.NewVirtualNetworksClient("sub", cred, opts)
	require.NoError(t, err)
	p.NSGClient, err = armnetwork.NewSecurityGroupsClient("sub", cred, opts)
	require.NoError(t, err)
	p.ResourceGraphClient, err = armresourcegraph.NewClient(cred, opts)
	require.NoError(t, err)
	return f, p
}

func (f *fakeARM) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	b, _ := io.ReadAll(r.Body)
	f.mu.Lock()
	defer f.mu.Unlock()
	f.reqs = append(f.reqs, r.Method+" "+r.URL.Path)
	f.body[r.Method+" "+r.URL.Path] = string(b)

	w.Header().Set("Content-Type", "application/json")
	for _, row := range f.rows {
		method, part, _ := strings.Cut(row.key, " ")
		if r.Method == method && strings.Contains(r.URL.Path, part) {
			_, _ = io.WriteString(w, row.reply)
			return
		}
	}
	w.WriteHeader(http.StatusNotFound)
	_, _ = io.WriteString(w, `{"error":{"code":"ResourceNotFound","message":"not found"}}`)
}

func (f *fakeARM) requests() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.reqs...)
}

const azureGraphReply = `{"totalRecords":2,"count":2,"data":[
 {"vmId":"/subscriptions/sub/resourceGroups/rg/providers/Microsoft.Compute/virtualMachines/vm1","vmName":"vm1","vmSize":"Standard_B1s","timeCreated":"2026-01-02T03:04:05Z","status":"VM running","location":"westeurope","privateIp":"10.0.0.4","publicIpAddress":"1.2.3.4"},
 {"vmId":"/subscriptions/sub/resourceGroups/rg/providers/Microsoft.Compute/virtualMachines/vm2","vmName":"vm2","vmSize":"Standard_B2s","timeCreated":"2026-01-03T03:04:05Z","status":"VM deallocated","location":"westeurope","privateIp":"10.0.0.5"}
]}`

func TestAzureListAndGetByName(t *testing.T) {
	_, p := newFakeARM(t, armRow{"POST /providers/Microsoft.ResourceGraph/resources", azureGraphReply})

	list, err := p.List()
	require.NoError(t, err)
	require.Len(t, list.List, 2)
	assert.Equal(t, "vm1", list.List[0].ID, "the ID is the last segment of the resource ID")
	assert.Equal(t, "1.2.3.4", list.List[0].IP)
	assert.Equal(t, "N/A", list.List[1].IP, "a VM with no public IP, e.g. deallocated, says so")
	assert.Equal(t, "VM deallocated", list.List[1].Status)

	vm, err := p.GetByName("vm2")
	require.NoError(t, err)
	assert.Equal(t, "Standard_B2s", vm.Type)

	vm, err = p.GetByName("missing")
	assert.NoError(t, err)
	assert.Empty(t, vm.ID)

	paused, err := p.ListPaused()
	assert.NoError(t, err)
	assert.Empty(t, paused.List)
}

func TestAzurePauseAndResume(t *testing.T) {
	f, p := newFakeARM(t,
		armRow{"POST /virtualMachines/vm1/deallocate", ``},
		armRow{"POST /virtualMachines/vm1/start", ``},
		armRow{"POST /providers/Microsoft.ResourceGraph/resources", azureGraphReply},
	)
	require.NoError(t, p.Pause(Vm{Name: "vm1"}, false))
	vm, err := p.Resume(Vm{Name: "vm1"})
	require.NoError(t, err)
	assert.Equal(t, "vm1", vm.Name)

	reqs := strings.Join(f.requests(), "\n")
	// Deallocate, not power-off: only deallocation stops compute billing.
	assert.Contains(t, reqs, "/virtualMachines/vm1/deallocate")
	assert.Contains(t, reqs, "/virtualMachines/vm1/start")
}

func TestAzurePauseResumeErrors(t *testing.T) {
	_, p := newFakeARM(t) // everything 404s
	assert.Error(t, p.Pause(Vm{Name: "vm1"}, false))
	_, err := p.Resume(Vm{Name: "vm1"})
	assert.Error(t, err)
}

func TestAzureDestroyRemovesEverythingTheDeployCreated(t *testing.T) {
	f, p := newFakeARM(t,
		armRow{"DELETE /virtualMachines/vm1", ``},
		armRow{"DELETE /networkInterfaces/vm1-nic", ``},
		armRow{"DELETE /publicIPAddresses/vm1-pip", ``},
		armRow{"DELETE /networkSecurityGroups/vm1-nsg", ``},
	)
	require.NoError(t, p.Destroy(Vm{Name: "vm1"}))

	// The VM goes first: a NIC still attached to a VM can't be deleted.
	var order []string
	for _, r := range f.requests() {
		if strings.HasPrefix(r, "DELETE ") {
			order = append(order, r[strings.LastIndex(r, "/")+1:])
		}
	}
	assert.Equal(t, []string{"vm1", "vm1-nic", "vm1-pip", "vm1-nsg"}, order)
}

func TestAzureCreateSSHKey(t *testing.T) {
	_, p := newFakeARM(t, armRow{"PUT /sshPublicKeys/", `{"id":"/subscriptions/sub/resourceGroups/rg/providers/Microsoft.Compute/sshPublicKeys/me","name":"me"}`})
	pub := filepath.Join(t.TempDir(), "id.pub")
	require.NoError(t, os.WriteFile(pub, []byte("ssh-ed25519 AAAA test"), 0o600))

	id, err := p.CreateSSHKey(pub)
	require.NoError(t, err)
	assert.Contains(t, id, "/sshPublicKeys/")
}

func TestAzureGetSSHKeyPublicData(t *testing.T) {
	_, p := newFakeARM(t, armRow{"GET /sshPublicKeys/", `{"name":"me","properties":{"publicKey":"ssh-ed25519 AAAA test"}}`})
	key, err := p.getSSHKeyPublicData()
	require.NoError(t, err)
	assert.Equal(t, "ssh-ed25519 AAAA test", key)
}

func TestAzureDeploy(t *testing.T) {
	f, p := newFakeARM(t,
		armRow{"GET /virtualNetworks/vnet", `{"name":"vnet","id":"/vnet","properties":{"subnets":[{"id":"/vnet/subnets/default","name":"default"}]}}`},
		armRow{"PUT /publicIPAddresses/vm1-pip", `{"id":"/pip","name":"vm1-pip","properties":{"ipAddress":"4.3.2.1","provisioningState":"Succeeded"}}`},
		armRow{"PUT /networkSecurityGroups/vm1-nsg", `{"id":"/nsg","name":"vm1-nsg","properties":{"provisioningState":"Succeeded"}}`},
		armRow{"PUT /networkInterfaces/vm1-nic", `{"id":"/nic","name":"vm1-nic","properties":{"provisioningState":"Succeeded"}}`},
		armRow{"GET /sshPublicKeys/", `{"name":"me","properties":{"publicKey":"ssh-ed25519 AAAA test"}}`},
		armRow{"PUT /virtualMachines/vm1", `{"name":"vm1","properties":{"vmId":"abc-123","provisioningState":"Succeeded","timeCreated":"2026-01-02T03:04:05Z","hardwareProfile":{"vmSize":"Standard_B1s"}}}`},
	)
	viper.Set("azure.vm.priority", "Spot")

	vm, err := p.Deploy(Vm{Name: "vm1"})
	require.NoError(t, err)
	assert.Equal(t, "abc-123", vm.ID)
	assert.Equal(t, "4.3.2.1", vm.IP, "the address comes from the public IP created for it")
	assert.Equal(t, "Standard_B1s", vm.Type)
	assert.Equal(t, "Succeeded", vm.Status)

	f.mu.Lock()
	defer f.mu.Unlock()
	var vmBody string
	for k, b := range f.body {
		if strings.HasPrefix(k, "PUT ") && strings.HasSuffix(k, "/virtualMachines/vm1") {
			vmBody = b
		}
	}
	// Spot VMs are deleted on eviction rather than deallocated, so the
	// billing cap and policy must travel with the priority.
	assert.Contains(t, vmBody, `"priority":"Spot"`)
	assert.Contains(t, vmBody, `"evictionPolicy":"Delete"`)
	assert.Contains(t, vmBody, `"disablePasswordAuthentication":true`)
}
