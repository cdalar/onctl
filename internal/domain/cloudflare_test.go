package domain

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// redirectTransport sends every request to the fake server, so the
// cloudflare client (which falls back to http.DefaultClient) never
// reaches the real API.
type redirectTransport struct{ target *url.URL }

func (rt redirectTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	req = req.Clone(req.Context())
	req.URL.Scheme = rt.target.Scheme
	req.URL.Host = rt.target.Host
	return http.DefaultTransport.RoundTrip(req)
}

// SetRecord must replace an existing record of the same name (delete then
// create) so the subdomain points at the new VM, and must leave other
// records in the zone untouched.
func TestSetRecord_ReplacesExistingRecord(t *testing.T) {
	var (
		mu      sync.Mutex
		deleted []string
		created map[string]any
	)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		assert.Equal(t, "Bearer test-token-12345", r.Header.Get("Authorization"))
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/client/v4/zones/zone-123/dns_records":
			_, _ = io.WriteString(w, `{"success":true,"errors":[],"messages":[],
				"result":[
					{"id":"rec-old","type":"A","name":"app.example.com","content":"9.9.9.9"},
					{"id":"rec-other","type":"A","name":"other.example.com","content":"8.8.8.8"}
				],
				"result_info":{"page":1,"per_page":100,"count":2,"total_count":2,"total_pages":1}}`)
		case r.Method == http.MethodDelete:
			deleted = append(deleted, r.URL.Path)
			_, _ = io.WriteString(w, `{"success":true,"errors":[],"messages":[],"result":{"id":"rec-old"}}`)
		case r.Method == http.MethodPost && r.URL.Path == "/client/v4/zones/zone-123/dns_records":
			body, _ := io.ReadAll(r.Body)
			require.NoError(t, json.Unmarshal(body, &created))
			_, _ = io.WriteString(w, `{"success":true,"errors":[],"messages":[],
				"result":{"id":"rec-new","type":"A","name":"app.example.com","content":"1.2.3.4","proxied":true}}`)
		default:
			t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()

	target, err := url.Parse(srv.URL)
	require.NoError(t, err)
	oldClientTransport := http.DefaultClient.Transport
	http.DefaultClient.Transport = redirectTransport{target: target}
	t.Cleanup(func() { http.DefaultClient.Transport = oldClientTransport })

	t.Setenv("CLOUDFLARE_ZONE_ID", "zone-123")
	svc := &CloudFlareService{CLOUDFLARE_API_TOKEN: "test-token-12345"}

	out, err := svc.SetRecord(&SetRecordRequest{Subdomain: "app.example.com", Ipaddress: "1.2.3.4"})
	require.NoError(t, err)
	assert.NotNil(t, out)

	mu.Lock()
	defer mu.Unlock()
	assert.Equal(t, []string{"/client/v4/zones/zone-123/dns_records/rec-old"}, deleted,
		"only the record matching the subdomain should be deleted")
	require.NotNil(t, created, "a new record must be created")
	assert.Equal(t, "A", created["type"])
	assert.Equal(t, "app.example.com", created["name"])
	assert.Equal(t, "1.2.3.4", created["content"])
	assert.Equal(t, true, created["proxied"])
}
