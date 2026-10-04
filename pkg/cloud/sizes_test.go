package cloud

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/hetznercloud/hcloud-go/v2/hcloud"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func hetznerServerType(id int, name string, cores int, mem float64, arch string, price string, locs map[string]bool, deprecated bool) map[string]any {
	var locations []map[string]any
	for name, available := range locs {
		locations = append(locations, map[string]any{
			"id": 1, "name": name, "available": available, "recommended": false, "deprecation": nil,
		})
	}
	st := map[string]any{
		"id": id, "name": name, "description": name, "category": "shared", "cores": cores, "memory": mem, "disk": 40,
		"storage_type": "local", "cpu_type": "shared", "architecture": arch, "deprecated": deprecated,
		"prices": []map[string]any{
			{"location": "fsn1", "price_hourly": map[string]any{"net": price, "gross": price}, "price_monthly": map[string]any{"net": "4.5", "gross": "4.5"}},
			{"location": "ash", "price_hourly": map[string]any{"net": "9", "gross": "9"}, "price_monthly": map[string]any{"net": "99", "gross": "99"}},
		},
		"locations": locations,
	}
	if deprecated {
		st["deprecation"] = map[string]any{"announced": "2024-01-01T00:00:00Z", "unavailable_after": "2024-06-01T00:00:00Z"}
	}
	return st
}

func TestProviderHetzner_ListSizes(t *testing.T) {
	types := []map[string]any{
		hetznerServerType(3, "cpx31", 4, 8, "x86", "0.0250000000", map[string]bool{"fsn1": true}, false),
		hetznerServerType(1, "cpx21", 3, 4, "x86", "0.0080000000", map[string]bool{"fsn1": true}, false),
		hetznerServerType(2, "cax11", 2, 4, "arm", "0.0060000000", map[string]bool{"fsn1": true}, false),
		hetznerServerType(4, "ccx99", 48, 192, "x86", "1", map[string]bool{"fsn1": false}, false), // not offered in fsn1
		hetznerServerType(5, "cx11", 1, 2, "x86", "0.005", map[string]bool{"fsn1": true}, true),   // deprecated
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"server_types": types,
			"meta":         map[string]any{"pagination": map[string]any{"page": 1, "per_page": 50, "total_entries": len(types), "last_page": 1}},
		})
	}))
	defer srv.Close()

	p := ProviderHetzner{
		Client: hcloud.NewClient(hcloud.WithToken("test"), hcloud.WithEndpoint(srv.URL)),
		Config: HetznerConfig{Location: "fsn1", VMType: "cpx21"},
	}
	sizes, err := p.ListSizes()
	require.NoError(t, err)

	var names []string
	for _, s := range sizes {
		names = append(names, s.Name)
	}
	// arm first, then by size; unavailable-here and deprecated types gone.
	assert.Equal(t, []string{"cax11", "cpx21", "cpx31"}, names)

	cpx21 := sizes[1]
	assert.Equal(t, 3, cpx21.VCPU)
	assert.Equal(t, 4096, cpx21.MemoryMiB)
	assert.Equal(t, 40, cpx21.DiskGiB)
	assert.Equal(t, "x86", cpx21.Arch)
	assert.True(t, cpx21.Default, "the configured hetzner.vm.type is the default")
	assert.False(t, sizes[0].Default)
	// fsn1's price, not ash's, in euros (server-type prices carry no
	// currency of their own).
	assert.Equal(t, "0.0080 EUR", cpx21.HourlyPrice)
	assert.Equal(t, "4.50 EUR", cpx21.MonthlyPrice)
}

func TestProviderOvh_ListSizes(t *testing.T) {
	_, client := newOvhFakeServer(t, func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "/cloud/project/svc123/flavor", r.URL.Path)
		_ = json.NewEncoder(w).Encode([]ovhFlavor{
			{ID: "1", Name: "b2-7", Region: "GRA11", VCPUs: 2, RAM: 7000, Disk: 50, Available: true},
			{ID: "2", Name: "d2-4", Region: "GRA11", VCPUs: 2, RAM: 4000, Disk: 50, Available: true},
			{ID: "3", Name: "d2-4", Region: "BHS5", VCPUs: 2, RAM: 4000, Disk: 50, Available: true}, // other region
			{ID: "4", Name: "t1-45", Region: "GRA11", VCPUs: 8, RAM: 45000, Disk: 400, Available: false},
		})
	})
	p := ProviderOvh{Client: client, Config: testConfig()}
	sizes, err := p.ListSizes()
	require.NoError(t, err)
	require.Len(t, sizes, 2)
	assert.Equal(t, "d2-4", sizes[0].Name, "smallest first")
	assert.True(t, sizes[0].Default, "the configured ovh.vm.type is the default")
	assert.Equal(t, 4000, sizes[0].MemoryMiB)
	assert.Empty(t, sizes[0].HourlyPrice, "OVH's API publishes no prices")
}

func TestProviderBoxes_ListSizes(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "/api/sizes", r.URL.Path)
		_, _ = w.Write([]byte(`[{"name":"small","vcpu":1,"mem_mib":1024,"disk_mib":10240},{"name":"medium","vcpu":2,"mem_mib":4096,"disk_mib":20480,"default":true}]`))
	}))
	defer srv.Close()
	_, p := newFakeBoxes(t)
	p.Client = newBoxesClient(srv.URL)
	sizes, err := p.ListSizes()
	require.NoError(t, err)
	assert.Equal(t, []CloudSize{
		{Name: "small", VCPU: 1, MemoryMiB: 1024, DiskGiB: 10},
		{Name: "medium", VCPU: 2, MemoryMiB: 4096, DiskGiB: 20, Default: true},
	}, sizes)
}
