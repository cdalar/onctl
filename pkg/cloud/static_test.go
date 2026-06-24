package cloud

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func newTestStaticProvider(t *testing.T) ProviderStatic {
	t.Helper()
	dir := t.TempDir()
	return ProviderStatic{InventoryPath: filepath.Join(dir, "imported.yaml")}
}

func TestProviderStatic_List_Empty(t *testing.T) {
	p := newTestStaticProvider(t)
	list, err := p.List()
	assert.NoError(t, err)
	assert.Empty(t, list.List)
}

func TestProviderStatic_SaveAndLoadInventory_RoundTrip(t *testing.T) {
	p := newTestStaticProvider(t)
	inv := StaticInventory{Hosts: []StaticHost{
		{Name: "box1", IP: "1.2.3.4", Username: "root", SSHPort: 22, ImportedAt: time.Now()},
		{Name: "box2", IP: "5.6.7.8", Username: "ubuntu", SSHPort: 2222, PrivateKey: "/tmp/key"},
	}}
	assert.NoError(t, p.SaveInventory(inv))

	loaded, err := p.LoadInventory()
	assert.NoError(t, err)
	assert.Len(t, loaded.Hosts, 2)
	assert.Equal(t, "box1", loaded.Hosts[0].Name)
	assert.Equal(t, "5.6.7.8", loaded.Hosts[1].IP)
	assert.Equal(t, "/tmp/key", loaded.Hosts[1].PrivateKey)

	list, err := p.List()
	assert.NoError(t, err)
	assert.Len(t, list.List, 2)
	assert.Equal(t, "imported", list.List[0].Status)
	assert.Equal(t, "static", list.List[0].Provider)
}

func TestProviderStatic_GetByName_Found(t *testing.T) {
	p := newTestStaticProvider(t)
	assert.NoError(t, p.SaveInventory(StaticInventory{Hosts: []StaticHost{
		{Name: "box1", IP: "1.2.3.4"},
	}}))

	vm, err := p.GetByName("box1")
	assert.NoError(t, err)
	assert.Equal(t, "1.2.3.4", vm.IP)
}

func TestProviderStatic_GetByName_NotFound(t *testing.T) {
	p := newTestStaticProvider(t)
	_, err := p.GetByName("missing")
	assert.Error(t, err)
}

func TestProviderStatic_Destroy_RemovesOnlyNamedEntry(t *testing.T) {
	p := newTestStaticProvider(t)
	assert.NoError(t, p.SaveInventory(StaticInventory{Hosts: []StaticHost{
		{Name: "box1", IP: "1.2.3.4"},
		{Name: "box2", IP: "5.6.7.8"},
	}}))

	assert.NoError(t, p.Destroy(Vm{Name: "box1"}))

	loaded, err := p.LoadInventory()
	assert.NoError(t, err)
	assert.Len(t, loaded.Hosts, 1)
	assert.Equal(t, "box2", loaded.Hosts[0].Name)
}

func TestProviderStatic_Destroy_NotFound(t *testing.T) {
	p := newTestStaticProvider(t)
	assert.NoError(t, p.SaveInventory(StaticInventory{Hosts: []StaticHost{
		{Name: "box1", IP: "1.2.3.4"},
	}}))

	err := p.Destroy(Vm{Name: "missing"})
	assert.Error(t, err)
}

func TestProviderStatic_UnsupportedOperations(t *testing.T) {
	p := newTestStaticProvider(t)

	_, err := p.Deploy(Vm{})
	assert.Error(t, err)

	err = p.Pause(Vm{}, false)
	assert.Error(t, err)

	_, err = p.Resume(Vm{})
	assert.Error(t, err)

	_, err = p.CreateSSHKey("")
	assert.Error(t, err)

	paused, err := p.ListPaused()
	assert.NoError(t, err)
	assert.Empty(t, paused.List)
}
