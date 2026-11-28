package cloud

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Failures from Azure used to end the process with log.Fatal or panic on a
// nil field; they have to come back as errors the command can report.

func TestAzureAPIErrorsAreReturned(t *testing.T) {
	_, p := newFakeARM(t) // every call is a 404

	_, err := p.List()
	assert.Error(t, err)
	_, err = p.GetByName("vm1")
	assert.Error(t, err)
	_, err = p.getSSHKeyPublicData()
	assert.Error(t, err)
	assert.Error(t, p.Destroy(Vm{Name: "vm1"}))
	_, err = p.Deploy(Vm{Name: "vm1"})
	assert.ErrorContains(t, err, "virtual network")

	pub := filepath.Join(t.TempDir(), "id.pub")
	require.NoError(t, os.WriteFile(pub, []byte("ssh-ed25519 AAAA test"), 0o600))
	_, err = p.CreateSSHKey(pub)
	assert.Error(t, err)
	_, err = p.CreateSSHKey(filepath.Join(t.TempDir(), "missing.pub"))
	assert.Error(t, err)
}

func TestAzureListToleratesMissingColumnsAndRejectsBadTimes(t *testing.T) {
	_, p := newFakeARM(t, armRow{"POST /providers/Microsoft.ResourceGraph/resources",
		`{"totalRecords":1,"count":1,"data":[{"vmName":"vm1","timeCreated":"2026-01-02T03:04:05Z"}]}`})
	list, err := p.List()
	require.NoError(t, err, "a column the query left out used to panic on a type assertion")
	require.Len(t, list.List, 1)
	assert.Equal(t, "N/A", list.List[0].IP)

	_, p = newFakeARM(t, armRow{"POST /providers/Microsoft.ResourceGraph/resources",
		`{"totalRecords":1,"count":1,"data":[{"vmName":"vm1","timeCreated":"yesterday"}]}`})
	_, err = p.List()
	assert.ErrorContains(t, err, "vm1")

	_, p = newFakeARM(t, armRow{"POST /providers/Microsoft.ResourceGraph/resources",
		`{"totalRecords":1,"count":1,"data":["not an object"]}`})
	_, err = p.List()
	assert.Error(t, err)
}

func TestAzureListWithoutTotalRecords(t *testing.T) {
	_, p := newFakeARM(t, armRow{"POST /providers/Microsoft.ResourceGraph/resources", `{"data":[]}`})
	list, err := p.List()
	assert.NoError(t, err)
	assert.Empty(t, list.List)
}

func TestAzureSSHKeyWithoutPublicData(t *testing.T) {
	_, p := newFakeARM(t, armRow{"GET /sshPublicKeys/", `{"name":"me","properties":{}}`})
	_, err := p.getSSHKeyPublicData()
	assert.ErrorContains(t, err, "no public key")
}

func TestAzureDeployFailureSteps(t *testing.T) {
	vnet := armRow{"GET /virtualNetworks/vnet", `{"name":"vnet","id":"/vnet","properties":{"subnets":[{"id":"/vnet/subnets/default","name":"default"}]}}`}
	pip := armRow{"PUT /publicIPAddresses/vm1-pip", `{"id":"/pip","name":"vm1-pip","properties":{"ipAddress":"4.3.2.1"}}`}
	nsg := armRow{"PUT /networkSecurityGroups/vm1-nsg", `{"id":"/nsg","name":"vm1-nsg"}`}
	nic := armRow{"PUT /networkInterfaces/vm1-nic", `{"id":"/nic","name":"vm1-nic"}`}
	key := armRow{"GET /sshPublicKeys/", `{"name":"me","properties":{"publicKey":"ssh-ed25519 AAAA test"}}`}

	for name, tc := range map[string]struct {
		rows []armRow
		want string
	}{
		"public IP":       {[]armRow{vnet}, "public IP"},
		"security group":  {[]armRow{vnet, pip}, "security group"},
		"network card":    {[]armRow{vnet, pip, nsg}, "network interface"},
		"ssh key":         {[]armRow{vnet, pip, nsg, nic}, "SSH public key"},
		"vm create":       {[]armRow{vnet, pip, nsg, nic, key}, "creating VM"},
		"incomplete VM":   {[]armRow{vnet, pip, nsg, nic, key, {"PUT /virtualMachines/vm1", `{"name":"vm1"}`}}, "incomplete"},
		"vnet has no net": {[]armRow{{"GET /virtualNetworks/vnet", `{"name":"vnet","properties":{"subnets":[]}}`}, pip, nsg}, "no subnet"},
	} {
		t.Run(name, func(t *testing.T) {
			_, p := newFakeARM(t, tc.rows...)
			_, err := p.Deploy(Vm{Name: "vm1"})
			assert.ErrorContains(t, err, tc.want)
		})
	}
}

func TestAzureDeployCreatesTheVirtualNetwork(t *testing.T) {
	f, p := newFakeARM(t,
		armRow{"PUT /virtualNetworks/vnet", `{"name":"vnet","id":"/vnet","properties":{"subnets":[{"id":"/vnet/subnets/default","name":"default"}]}}`},
		armRow{"PUT /publicIPAddresses/vm1-pip", `{"id":"/pip","name":"vm1-pip","properties":{"ipAddress":"4.3.2.1"}}`},
		armRow{"PUT /networkSecurityGroups/vm1-nsg", `{"id":"/nsg","name":"vm1-nsg"}`},
		armRow{"PUT /networkInterfaces/vm1-nic", `{"id":"/nic","name":"vm1-nic"}`},
		armRow{"GET /sshPublicKeys/", `{"name":"me","properties":{"publicKey":"ssh-ed25519 AAAA test"}}`},
		armRow{"PUT /virtualMachines/vm1", `{"name":"vm1","properties":{"vmId":"abc","provisioningState":"Succeeded","timeCreated":"2026-01-02T03:04:05Z","hardwareProfile":{"vmSize":"Standard_B1s"}}}`},
	)
	viper.Set("azure.vm.vnet.create", "true")
	viper.Set("azure.vm.vnet.cidr", "10.0.0.0/16")
	viper.Set("azure.vm.vnet.subnet.name", "default")
	viper.Set("azure.vm.vnet.subnet.cidr", "10.0.0.0/24")

	vm, err := p.Deploy(Vm{Name: "vm1"})
	require.NoError(t, err)
	assert.Equal(t, "abc", vm.ID)
	assert.NotContains(t, f.requests(), "GET /subscriptions/sub/resourceGroups/rg/providers/Microsoft.Network/virtualNetworks/vnet",
		"a vnet that is created is not looked up first")
}

func TestAzureDestroyStopsAtTheFirstFailure(t *testing.T) {
	f, p := newFakeARM(t, armRow{"DELETE /virtualMachines/vm1", ``}) // the NIC delete 404s
	err := p.Destroy(Vm{Name: "vm1"})
	assert.ErrorContains(t, err, "network interface")
	for _, r := range f.requests() {
		assert.NotContains(t, r, "publicIPAddresses", "nothing after the failed step is attempted")
	}
}
