package cloud

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sort"
	"strings"
	"sync"
	"testing"

	"github.com/hetznercloud/hcloud-go/v2/hcloud"
	"github.com/stretchr/testify/assert"
)

// fakeHetznerForDestroy is a Hetzner API with one running server, web
// (id 3), and one paused VM, test: its pause snapshot (image 7) and the
// IPv4 kept for it (primary IP 9). Primary IP 10 carries test's label but
// is assigned to a server, so it isn't test's to delete. It returns the
// DELETE requests it got.
func fakeHetznerForDestroy(t *testing.T) (*ProviderHetzner, func() []string) {
	t.Helper()
	page := map[string]any{"pagination": map[string]any{"page": 1, "per_page": 50, "total_entries": 1, "last_page": 1}}
	var mu sync.Mutex
	var deleted []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		label := r.URL.Query().Get("label_selector")
		var out map[string]any
		switch {
		case r.Method == http.MethodDelete:
			mu.Lock()
			deleted = append(deleted, r.URL.Path)
			mu.Unlock()
			out = map[string]any{"action": map[string]any{"id": 1, "status": "success"}}
		case r.URL.Path == "/servers":
			servers := []any{}
			if r.URL.Query().Get("name") == "web" {
				servers = append(servers, map[string]any{"id": 3, "name": "web", "status": "running"})
			}
			out = map[string]any{"servers": servers, "meta": page}
		case r.URL.Path == "/images":
			images := []any{}
			if label == labelSnapshot+"=test" {
				images = append(images, map[string]any{"id": 7, "type": "snapshot", "status": "available", "labels": map[string]any{labelSnapshot: "test"}})
			}
			out = map[string]any{"images": images, "meta": page}
		case r.URL.Path == "/primary_ips":
			ips := []any{}
			if label == labelSnapshot+"=test" {
				ips = append(ips,
					map[string]any{"id": 9, "ip": "167.233.64.66", "type": "ipv4", "assignee_id": nil},
					map[string]any{"id": 10, "ip": "167.233.64.67", "type": "ipv4", "assignee_id": 3})
			}
			out = map[string]any{"primary_ips": ips, "meta": page}
		default:
			t.Errorf("unexpected %s %s", r.Method, r.URL)
			w.WriteHeader(http.StatusNotFound)
			return
		}
		_ = json.NewEncoder(w).Encode(out)
	}))
	t.Cleanup(srv.Close)
	p := &ProviderHetzner{Client: hcloud.NewClient(hcloud.WithToken("test"), hcloud.WithEndpoint(srv.URL))}
	return p, func() []string {
		mu.Lock()
		defer mu.Unlock()
		got := append([]string(nil), deleted...)
		sort.Strings(got)
		deleted = nil
		return got
	}
}

// A paused VM has no server -- Pause deleted it -- only a snapshot and
// the IP kept for it. Destroy deletes servers and nothing else, so it
// can't take a snapshot by surprise; DestroySnapshot, which callers ask
// about first, deletes both (they cost storage and the IP's monthly
// fee). An IP a server has is never touched.
func TestProviderHetzner_DestroyPaused(t *testing.T) {
	p, deletes := fakeHetznerForDestroy(t)

	err := p.Destroy(Vm{Name: "test"})
	if assert.Error(t, err) {
		assert.Equal(t, `no server named "test"`, err.Error())
	}
	assert.Empty(t, deletes(), "Destroy never deletes a snapshot")

	assert.NoError(t, p.Destroy(Vm{Name: "web"}))
	assert.Equal(t, []string{"/servers/3"}, deletes())

	assert.NoError(t, p.DestroySnapshot("test"))
	assert.Equal(t, []string{"/images/7", "/primary_ips/9"}, deletes())

	err = p.DestroySnapshot("gone")
	if assert.Error(t, err) {
		assert.True(t, strings.Contains(err.Error(), `no paused VM (snapshot) named "gone"`), err.Error())
	}
	assert.Empty(t, deletes())
}
