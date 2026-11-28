package cloud

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/cdalar/onctl/internal/fakeec2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// These paths used to log.Fatal or panic, ending the process; a library
// call has to hand the failure back so the command can report it and the
// TUI can stay up.

func TestAwsGetByNameReturnsTheAPIError(t *testing.T) {
	f, svc := fakeec2.New(t, nil)
	f.Fail("DescribeInstances", "UnauthorizedOperation")
	_, err := ProviderAws{Client: svc}.GetByName("web")
	assert.Error(t, err)
}

func TestAwsGetByNameEmptyReservation(t *testing.T) {
	_, svc := fakeec2.New(t, map[string]string{
		"DescribeInstances": `<reservationSet><item><instancesSet/></item></reservationSet>`,
	})
	vm, err := ProviderAws{Client: svc}.GetByName("web")
	assert.NoError(t, err)
	assert.Empty(t, vm.ID)

	list, err := ProviderAws{Client: svc}.List()
	assert.NoError(t, err)
	assert.Empty(t, list.List)
}

func TestAwsDestroyErrors(t *testing.T) {
	f, svc := fakeec2.New(t, map[string]string{"DescribeInstances": reservations()})
	p := ProviderAws{Client: svc}
	assert.ErrorContains(t, p.Destroy(Vm{Name: "ghost"}), "no instance found",
		"destroying a VM that isn't there is an error, not an exit")
	assert.NotContains(t, f.Called(), "TerminateInstances")

	f.Fail("DescribeInstances", "UnauthorizedOperation")
	assert.Error(t, p.Destroy(Vm{Name: "web"}))
}

func TestAwsCreateSSHKeyErrors(t *testing.T) {
	f, svc := fakeec2.New(t, nil)
	p := ProviderAws{Client: svc}
	dir := t.TempDir()

	_, err := p.CreateSSHKey(filepath.Join(dir, "missing.pub"))
	assert.Error(t, err, "an unreadable key file")

	bad := filepath.Join(dir, "bad.pub")
	require.NoError(t, os.WriteFile(bad, []byte("not a key"), 0o600))
	_, err = p.CreateSSHKey(bad)
	assert.Error(t, err, "a key that doesn't parse used to panic")
	assert.Empty(t, f.Called(), "nothing is sent to AWS for a bad key")

	good := filepath.Join(dir, "good.pub")
	require.NoError(t, os.WriteFile(good, []byte("ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIOMqqnkVzrm0SdG6UOoqKLsabgH5C9okWi0dh2l9GKJl test\n"), 0o600))
	f.Fail("ImportKeyPair", "UnauthorizedOperation")
	_, err = p.CreateSSHKey(good)
	assert.Error(t, err)

	// Duplicate, but the lookup of the existing key fails or comes back empty.
	f.Fail("ImportKeyPair", "InvalidKeyPair.Duplicate")
	f.Fail("DescribeKeyPairs", "UnauthorizedOperation")
	_, err = p.CreateSSHKey(good)
	assert.Error(t, err)
	f.Reply("DescribeKeyPairs", `<keySet/>`)
	_, err = p.CreateSSHKey(good)
	assert.Error(t, err)
}

func TestAwsDeployErrors(t *testing.T) {
	images := `<imagesSet><item><imageId>ami-1</imageId></item></imagesSet>`
	keys := `<keySet><item><keyName>onctl-k</keyName></item></keySet>`
	vpcs := `<vpcSet><item><vpcId>vpc-1</vpcId></item></vpcSet>`
	vm := Vm{Name: "web", Type: "t2.micro", SSHKeyID: "key-1"}

	t.Run("image lookup fails", func(t *testing.T) {
		f, svc := fakeec2.New(t, nil)
		f.Fail("DescribeImages", "UnauthorizedOperation")
		_, err := ProviderAws{Client: svc}.Deploy(vm)
		assert.ErrorContains(t, err, "listing images")
	})
	t.Run("no image matches", func(t *testing.T) {
		_, svc := fakeec2.New(t, map[string]string{"DescribeImages": `<imagesSet/>`})
		_, err := ProviderAws{Client: svc}.Deploy(vm)
		assert.ErrorContains(t, err, "no matching AMI", "used to index an empty list and panic")
	})
	t.Run("key lookup fails", func(t *testing.T) {
		f, svc := fakeec2.New(t, map[string]string{"DescribeImages": images})
		f.Fail("DescribeKeyPairs", "InvalidKeyPair.NotFound")
		_, err := ProviderAws{Client: svc}.Deploy(vm)
		assert.ErrorContains(t, err, "key-1")
	})
	t.Run("key not found", func(t *testing.T) {
		_, svc := fakeec2.New(t, map[string]string{"DescribeImages": images, "DescribeKeyPairs": `<keySet/>`})
		_, err := ProviderAws{Client: svc}.Deploy(vm)
		assert.ErrorContains(t, err, "not found")
	})
	t.Run("existing-instance check fails", func(t *testing.T) {
		f, svc := fakeec2.New(t, map[string]string{"DescribeImages": images, "DescribeKeyPairs": keys, "DescribeVpcs": vpcs})
		f.Fail("DescribeInstances", "RequestLimitExceeded")
		_, err := ProviderAws{Client: svc}.Deploy(vm)
		assert.Error(t, err)
	})
	t.Run("run fails", func(t *testing.T) {
		f, svc := fakeec2.New(t, map[string]string{
			"DescribeImages": images, "DescribeKeyPairs": keys, "DescribeVpcs": vpcs,
			"DescribeInstances": reservations(),
		})
		f.Fail("RunInstances", "InsufficientInstanceCapacity")
		_, err := ProviderAws{Client: svc}.Deploy(vm)
		assert.Error(t, err)
	})
	t.Run("run returns no instance", func(t *testing.T) {
		_, svc := fakeec2.New(t, map[string]string{
			"DescribeImages": images, "DescribeKeyPairs": keys, "DescribeVpcs": vpcs,
			"DescribeInstances": reservations(), "RunInstances": `<instancesSet/>`,
		})
		_, err := ProviderAws{Client: svc}.Deploy(vm)
		assert.ErrorContains(t, err, "no instance")
	})
}
