package box

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/cdalar/onctl/pkg/cloud"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fc's images are the published catalog, not something the fc provider
// lists: Images must read the catalog even though ProviderFC has no
// ListImages, so `onctl images -p fc` works on a machine without
// Firecracker.
func TestImages_FCReadsTheCatalog(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/index.json":
			_, _ = w.Write([]byte(`{"images":["alpine"]}`))
		case "/catalog/alpine.json":
			_, _ = w.Write([]byte(`{"name":"alpine","kind":"rootfs","os":"alpine","current":{"size_bytes":536870912,"key":"rootfs/alpine.zst"}}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()
	prev := fcCatalogURL
	fcCatalogURL = srv.URL
	t.Cleanup(func() { fcCatalogURL = prev })

	images, err := Wrap("fc", cloud.ProviderFC{}).Images()
	require.NoError(t, err)
	require.Len(t, images, 1)
	assert.Equal(t, "alpine", images[0].Name)
	assert.Equal(t, srv.URL+"/rootfs/alpine.zst", images[0].URL)
}
