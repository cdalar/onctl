package cloud

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/cdalar/onctl/internal/fakeec2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// instanceXML is one EC2 instance as DescribeInstances reports it.
func instanceXML(id, name, state string) string {
	return `<item><instanceId>` + id + `</instanceId><instanceType>t2.micro</instanceType>` +
		`<ipAddress>1.2.3.4</ipAddress><privateIpAddress>10.0.0.5</privateIpAddress>` +
		`<launchTime>2026-01-02T03:04:05.000Z</launchTime>` +
		`<instanceState><name>` + state + `</name></instanceState>` +
		`<placement><availabilityZone>us-east-1a</availabilityZone></placement>` +
		`<tagSet><item><key>Name</key><value>` + name + `</value></item></tagSet></item>`
}

func reservations(items ...string) string {
	s := `<reservationSet>`
	for _, it := range items {
		s += `<item><instancesSet>` + it + `</instancesSet></item>`
	}
	return s + `</reservationSet>`
}

func TestAwsListAndGetByName(t *testing.T) {
	f, svc := fakeec2.New(t, map[string]string{
		"DescribeInstances": reservations(instanceXML("i-1", "web", "running"), instanceXML("i-2", "db", "stopped")),
	})
	p := ProviderAws{Client: svc}

	list, err := p.List()
	require.NoError(t, err)
	require.Len(t, list.List, 2)
	assert.Equal(t, "web", list.List[0].Name)
	assert.Equal(t, "stopped", list.List[1].Status, "paused instances stay in the list")
	// Only onctl's own instances are listed, never everything in the account.
	assert.Equal(t, "tag:Owner", f.Field(0, "Filter.1.Name"))
	assert.Equal(t, "onctl", f.Field(0, "Filter.1.Value.1"))

	vm, err := p.GetByName("web")
	require.NoError(t, err)
	assert.Equal(t, "i-1", vm.ID)
	assert.Equal(t, "1.2.3.4", vm.IP)

	f.Reply("DescribeInstances", reservations())
	list, err = p.List()
	require.NoError(t, err)
	assert.Empty(t, list.List)
	vm, err = p.GetByName("nope")
	assert.NoError(t, err)
	assert.Empty(t, vm.ID)

	f.Fail("DescribeInstances", "UnauthorizedOperation")
	_, err = p.List()
	assert.Error(t, err)

	paused, err := p.ListPaused()
	assert.NoError(t, err)
	assert.Empty(t, paused.List)
}

func TestAwsFindInstanceSkipsTerminated(t *testing.T) {
	_, svc := fakeec2.New(t, map[string]string{
		"DescribeInstances": reservations(instanceXML("i-dead", "web", "terminated"), instanceXML("i-live", "web", "stopped")),
	})
	id, err := ProviderAws{Client: svc}.findAwsInstance("web")
	require.NoError(t, err)
	assert.Equal(t, "i-live", id, "a terminated instance with the same name must not be picked")
}

func TestAwsPause(t *testing.T) {
	f, svc := fakeec2.New(t, map[string]string{
		"DescribeInstances": reservations(instanceXML("i-1", "web", "stopped")),
		"StopInstances":     `<instancesSet/>`,
	})
	p := ProviderAws{Client: svc}

	require.NoError(t, p.Pause(Vm{ID: "i-1"}, false))
	assert.Equal(t, "i-1", f.Field(0, "InstanceId.1"))

	// By name: looks the instance up first.
	require.NoError(t, p.Pause(Vm{Name: "web"}, false))
	assert.Equal(t, []string{"StopInstances", "DescribeInstances", "DescribeInstances", "StopInstances", "DescribeInstances"}, f.Called(), "by name: find, stop, wait")

	f.Reply("DescribeInstances", reservations())
	assert.ErrorContains(t, p.Pause(Vm{Name: "ghost"}, false), "no instance found")

	f.Fail("StopInstances", "IncorrectInstanceState")
	assert.Error(t, p.Pause(Vm{ID: "i-1"}, false))
}

func TestAwsResume(t *testing.T) {
	f, svc := fakeec2.New(t, map[string]string{
		"StartInstances": `<instancesSet/>`,
	})
	p := ProviderAws{Client: svc}
	// find (stopped) -> running waiter -> GetByName (running)
	f.ReplySeq("DescribeInstances",
		reservations(instanceXML("i-1", "web", "stopped")),
		reservations(instanceXML("i-1", "web", "running")),
	)
	vm, err := p.Resume(Vm{Name: "web"})
	require.NoError(t, err)
	assert.Equal(t, "i-1", vm.ID)
	assert.Equal(t, "running", vm.Status)

	f.Reply("DescribeInstances", reservations())
	_, err = p.Resume(Vm{Name: "ghost"})
	assert.ErrorContains(t, err, "no paused instance found")
}

func TestAwsAssociateElasticIP(t *testing.T) {
	f, svc := fakeec2.New(t, map[string]string{
		"AllocateAddress":  `<publicIp>9.9.9.9</publicIp><allocationId>eipalloc-1</allocationId>`,
		"AssociateAddress": `<associationId>eipassoc-1</associationId>`,
	})
	p := ProviderAws{Client: svc}
	ip, err := p.associateElasticIP("i-1", "web")
	require.NoError(t, err)
	assert.Equal(t, "9.9.9.9", ip)
	assert.Equal(t, "eipalloc-1", f.Field(1, "AllocationId"))

	// A failed associate must give the address back, or it bills forever.
	f2, svc2 := fakeec2.New(t, map[string]string{
		"AllocateAddress": `<publicIp>9.9.9.9</publicIp><allocationId>eipalloc-2</allocationId>`,
		"ReleaseAddress":  `<return>true</return>`,
	})
	f2.Fail("AssociateAddress", "InvalidInstanceID")
	_, err = ProviderAws{Client: svc2}.associateElasticIP("i-1", "web")
	assert.Error(t, err)
	assert.Equal(t, []string{"AllocateAddress", "AssociateAddress", "ReleaseAddress"}, f2.Called())

	f3, svc3 := fakeec2.New(t, nil)
	f3.Fail("AllocateAddress", "AddressLimitExceeded")
	_, err = ProviderAws{Client: svc3}.associateElasticIP("i-1", "web")
	assert.Error(t, err)
}

func TestAwsReleaseElasticIP(t *testing.T) {
	f, svc := fakeec2.New(t, map[string]string{
		"DescribeAddresses":   `<addressesSet><item><allocationId>eipalloc-1</allocationId><associationId>eipassoc-1</associationId></item><item><allocationId>eipalloc-2</allocationId></item></addressesSet>`,
		"DisassociateAddress": `<return>true</return>`,
		"ReleaseAddress":      `<return>true</return>`,
	})
	ProviderAws{Client: svc}.releaseElasticIP("web")
	// Only the associated one is disassociated; both are released.
	assert.Equal(t, []string{"DescribeAddresses", "DisassociateAddress", "ReleaseAddress", "ReleaseAddress"}, f.Called())

	f.Fail("DescribeAddresses", "UnauthorizedOperation")
	ProviderAws{Client: svc}.releaseElasticIP("web") // logged, not fatal
}

func TestAwsDestroy(t *testing.T) {
	f, svc := fakeec2.New(t, map[string]string{
		"TerminateInstances": `<instancesSet/>`,
		"DescribeAddresses":  `<addressesSet/>`,
	})
	p := ProviderAws{Client: svc}
	require.NoError(t, p.Destroy(Vm{ID: "i-1", Name: "web"}))
	assert.Equal(t, []string{"TerminateInstances", "DescribeAddresses"}, f.Called(), "the pinned Elastic IP is released with the instance")

	f.Fail("TerminateInstances", "UnauthorizedOperation")
	assert.Error(t, p.Destroy(Vm{ID: "i-1", Name: "web"}))
}

func TestAwsDestroyFindsByName(t *testing.T) {
	f, svc := fakeec2.New(t, map[string]string{
		"DescribeInstances":  reservations(instanceXML("i-7", "web", "stopped")),
		"TerminateInstances": `<instancesSet/>`,
		"DescribeAddresses":  `<addressesSet/>`,
	})
	require.NoError(t, ProviderAws{Client: svc}.Destroy(Vm{Name: "web"}))
	assert.Equal(t, "i-7", f.Field(1, "InstanceId.1"), "a paused instance is still destroyable by name")
}

func TestAwsCreateSSHKey(t *testing.T) {
	pub := filepath.Join(t.TempDir(), "id.pub")
	require.NoError(t, os.WriteFile(pub, []byte("ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIOMqqnkVzrm0SdG6UOoqKLsabgH5C9okWi0dh2l9GKJl test\n"), 0o600))

	f, svc := fakeec2.New(t, map[string]string{
		"ImportKeyPair": `<keyName>onctl-x</keyName><keyPairId>key-1</keyPairId>`,
	})
	id, err := ProviderAws{Client: svc}.CreateSSHKey(pub)
	require.NoError(t, err)
	assert.Equal(t, "key-1", id)
	assert.Regexp(t, `^onctl-[0-9a-f]{8}$`, f.Field(0, "KeyName"))

	// Importing the same key twice returns the existing one.
	f2, svc2 := fakeec2.New(t, map[string]string{
		"DescribeKeyPairs": `<keySet><item><keyPairId>key-old</keyPairId></item></keySet>`,
	})
	f2.Fail("ImportKeyPair", "InvalidKeyPair.Duplicate")
	id, err = ProviderAws{Client: svc2}.CreateSSHKey(pub)
	require.NoError(t, err)
	assert.Equal(t, "key-old", id)
}

func TestAwsDeployReusesRunningInstance(t *testing.T) {
	f, svc := fakeec2.New(t, map[string]string{
		"DescribeImages":    `<imagesSet><item><imageId>ami-1</imageId></item></imagesSet>`,
		"DescribeKeyPairs":  `<keySet><item><keyName>onctl-k</keyName></item></keySet>`,
		"DescribeVpcs":      `<vpcSet><item><vpcId>vpc-1</vpcId></item></vpcSet>`,
		"DescribeInstances": reservations(instanceXML("i-1", "web", "running")),
	})
	vm, err := ProviderAws{Client: svc}.Deploy(Vm{Name: "web", Type: "t2.micro", SSHKeyID: "key-1"})
	require.NoError(t, err)
	assert.Equal(t, "i-1", vm.ID)
	assert.NotContains(t, f.Called(), "RunInstances", "deploying twice must not start a second instance")
}

func TestAwsDeployCreatesInstance(t *testing.T) {
	f, svc := fakeec2.New(t, map[string]string{
		"DescribeImages":   `<imagesSet><item><imageId>ami-1</imageId></item></imagesSet>`,
		"DescribeKeyPairs": `<keySet><item><keyName>onctl-k</keyName></item></keySet>`,
		"DescribeVpcs":     `<vpcSet><item><vpcId>vpc-1</vpcId></item></vpcSet>`,
		"RunInstances":     `<instancesSet>` + instanceXML("i-new", "web", "pending") + `</instancesSet>`,
		"AllocateAddress":  `<publicIp>9.9.9.9</publicIp><allocationId>eipalloc-1</allocationId>`,
		"AssociateAddress": `<associationId>eipassoc-1</associationId>`,
	})
	// existing-instance check (none) -> running waiter -> DescribeInstance x2
	f.ReplySeq("DescribeInstances",
		reservations(),
		reservations(instanceXML("i-new", "web", "running")),
	)
	vm, err := ProviderAws{Client: svc}.Deploy(Vm{Name: "web", Type: "t2.micro", SSHKeyID: "key-1"})
	require.NoError(t, err)
	assert.Equal(t, "i-new", vm.ID)
	assert.Contains(t, f.Called(), "RunInstances")
	assert.Contains(t, f.Called(), "AssociateAddress", "a static IP is pinned by default")
}
