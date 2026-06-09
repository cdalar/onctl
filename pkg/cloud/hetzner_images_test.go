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

func fakeImageServer(t *testing.T, images []map[string]any) *httptest.Server {
	t.Helper()
	body, err := json.Marshal(map[string]any{
		"images": images,
		"meta": map[string]any{
			"pagination": map[string]any{
				"page":          1,
				"per_page":      50,
				"total_entries": len(images),
				"next_page":     nil,
				"last_page":     1,
			},
		},
	})
	require.NoError(t, err)
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(body)
	}))
}

func TestProviderHetzner_ListImages(t *testing.T) {
	ubuntuName := "ubuntu-22.04"
	fedoraName := "fedora-42"
	osVer22 := "22.04"
	osVer42 := "42"

	srv := fakeImageServer(t, []map[string]any{
		{
			"id": 1, "name": &ubuntuName, "type": "system", "status": "available",
			"description": "Ubuntu 22.04", "os_flavor": "ubuntu", "os_version": &osVer22,
			"architecture": "x86", "disk_size": 10.0, "rapid_deploy": false,
			"protection": map[string]any{"delete": false}, "labels": map[string]any{},
		},
		{
			"id": 2, "name": &fedoraName, "type": "system", "status": "available",
			"description": "Fedora 42", "os_flavor": "fedora", "os_version": &osVer42,
			"architecture": "x86", "disk_size": 10.0, "rapid_deploy": false,
			"protection": map[string]any{"delete": false}, "labels": map[string]any{},
		},
	})
	defer srv.Close()

	p := ProviderHetzner{
		Client: hcloud.NewClient(hcloud.WithToken("test"), hcloud.WithEndpoint(srv.URL)),
	}

	images, err := p.ListImages()
	require.NoError(t, err)
	assert.Len(t, images, 2)

	assert.Equal(t, "ubuntu-22.04", images[0].Name)
	assert.Equal(t, "system", images[0].Type)
	assert.Equal(t, "ubuntu", images[0].OSFlavor)
	assert.Equal(t, "22.04", images[0].OSVersion)
	assert.Equal(t, "Ubuntu 22.04", images[0].Description)

	assert.Equal(t, "fedora-42", images[1].Name)
	assert.Equal(t, "fedora", images[1].OSFlavor)
}

func TestProviderHetzner_ListImages_Empty(t *testing.T) {
	srv := fakeImageServer(t, []map[string]any{})
	defer srv.Close()

	p := ProviderHetzner{
		Client: hcloud.NewClient(hcloud.WithToken("test"), hcloud.WithEndpoint(srv.URL)),
	}

	images, err := p.ListImages()
	require.NoError(t, err)
	assert.Empty(t, images)
}

func TestProviderHetzner_ListImages_APIError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"error":{"code":"internal_error","message":"server error"}}`))
	}))
	defer srv.Close()

	p := ProviderHetzner{
		Client: hcloud.NewClient(hcloud.WithToken("test"), hcloud.WithEndpoint(srv.URL)),
	}

	images, err := p.ListImages()
	assert.Error(t, err)
	assert.Nil(t, images)
}

func TestCloudImage_Fields(t *testing.T) {
	img := CloudImage{
		Name:        "debian-12",
		Description: "Debian 12",
		Type:        "system",
		OSFlavor:    "debian",
		OSVersion:   "12",
	}
	assert.Equal(t, "debian-12", img.Name)
	assert.Equal(t, "Debian 12", img.Description)
	assert.Equal(t, "system", img.Type)
	assert.Equal(t, "debian", img.OSFlavor)
	assert.Equal(t, "12", img.OSVersion)
}

// compile-time check: ProviderHetzner implements ImageLister.
var _ ImageLister = ProviderHetzner{}
