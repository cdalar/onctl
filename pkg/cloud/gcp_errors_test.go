package cloud

import (
	"path/filepath"
	"testing"

	"cloud.google.com/go/compute/apiv1/computepb"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// A failing GCP call used to end the process with log.Fatal; it has to
// come back as an error instead.

func TestGcpAPIErrorsAreReturned(t *testing.T) {
	_, p := newFakeGCP(t, map[string]string{}) // every call is a 404

	_, err := p.List()
	assert.Error(t, err)
	_, err = p.GetByName("vm1")
	assert.Error(t, err)
	assert.Error(t, p.Destroy(Vm{ID: "1", Name: "vm1"}))
	_, err = p.Deploy(Vm{Name: "vm1"})
	assert.Error(t, err)
}

func TestGcpDeployFailsWhenTheOperationFails(t *testing.T) {
	_, p := newFakeGCP(t, map[string]string{
		"POST /projects/proj/zones/z1/instances": gcpDone,
		// the operation can't be polled: no GET entry, so Wait gets a 404
	})
	_, err := p.Deploy(Vm{Name: "vm1"})
	assert.Error(t, err)
}

func TestGcpDestroyUnknownInstance(t *testing.T) {
	_, p := newFakeGCP(t, map[string]string{})
	assert.Error(t, p.Destroy(Vm{Name: "ghost"}), "the lookup error is reported, nothing is deleted")
}

func TestGcpCreateSSHKeyMissingFile(t *testing.T) {
	_, p := newFakeGCP(t, nil)
	_, err := p.CreateSSHKey(filepath.Join(t.TempDir(), "nope.pub"))
	assert.Error(t, err)
}

func TestMapGcpServerOddInstances(t *testing.T) {
	ts := "2026-01-02T03:04:05Z"

	// No external address: no access config to read the IP from.
	vm, err := mapGcpServer(&computepb.Instance{
		Name:              strPtr("vm1"),
		CreationTimestamp: &ts,
		NetworkInterfaces: []*computepb.NetworkInterface{{}},
	})
	require.NoError(t, err)
	assert.Empty(t, vm.IP)

	// No network interface at all.
	vm, err = mapGcpServer(&computepb.Instance{Name: strPtr("vm2"), CreationTimestamp: &ts})
	require.NoError(t, err)
	assert.Empty(t, vm.IP)

	// A missing or malformed timestamp is an error naming the instance.
	_, err = mapGcpServer(&computepb.Instance{Name: strPtr("vm3")})
	assert.ErrorContains(t, err, "vm3")
	bad := "yesterday"
	_, err = mapGcpServer(&computepb.Instance{Name: strPtr("vm4"), CreationTimestamp: &bad})
	assert.Error(t, err)
}

func strPtr(s string) *string { return &s }
