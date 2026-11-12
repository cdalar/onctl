package providerfc

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// catalogServer serves files the way the public images bucket does: by
// key, with nothing listable.
func catalogServer(t *testing.T, files map[string]string) string {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, ok := files[r.URL.Path]
		if !ok {
			http.NotFound(w, r)
			return
		}
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)
	return srv.URL
}

// TestCatalogListerListImages: the list is for picking something to
// download and boot, so each row has to carry the exact object to fetch
// and how big it unpacks, a kernel has to be told apart from a rootfs,
// and an image retired from the catalog must not be offered.
func TestCatalogListerListImages(t *testing.T) {
	base := catalogServer(t, map[string]string{
		"/index.json":                 `{"schema":1,"images":["alpine","boxctl-kernel","old"]}`,
		"/catalog/alpine.json":        `{"name":"alpine","kind":"rootfs","os":"alpine","status":"hidden","current":{"size_bytes":536870912,"key":"images/alpine.ext4-a083df4.zst"}}`,
		"/catalog/boxctl-kernel.json": `{"name":"boxctl-kernel","kind":"kernel","status":"available","current":{"size_bytes":47016872,"key":"kernel/boxctl-kernel.vmlinux-18f2821.zst"}}`,
		"/catalog/old.json":           `{"name":"old","kind":"rootfs","status":"retired","current":{"size_bytes":1,"key":"images/old.zst"}}`,
	})

	images, err := CatalogLister{BaseURL: base + "/"}.ListImages()
	require.NoError(t, err)
	require.Len(t, images, 2, "the retired image is left out")

	assert.Equal(t, "alpine", images[0].Name)
	assert.Equal(t, "rootfs", images[0].Type)
	assert.Equal(t, "512 MiB", images[0].Size)
	assert.Equal(t, base+"/images/alpine.ext4-a083df4.zst", images[0].URL)

	assert.Equal(t, "kernel", images[1].Type)
	assert.Equal(t, "44 MiB", images[1].Size)
}

// TestCatalogListerFailsLoudly: a catalog that can't be read must be an
// error naming what failed, never an empty list that reads as "there are
// no images".
func TestCatalogListerFailsLoudly(t *testing.T) {
	_, err := CatalogLister{BaseURL: catalogServer(t, nil)}.ListImages()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "index.json")

	base := catalogServer(t, map[string]string{"/index.json": `{"images":["gone"]}`})
	_, err = CatalogLister{BaseURL: base}.ListImages()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "catalog/gone.json")
}

func TestFormatMiB(t *testing.T) {
	assert.Equal(t, "4.0 GiB", formatMiB(4<<30))
	assert.Equal(t, "512 MiB", formatMiB(512<<20))
}
