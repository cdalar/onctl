package cloud

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/ovh/go-ovh/ovh"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ovhFakeServer builds an httptest.Server standing in for the OVH API,
// driven by a caller-supplied handler, plus a real *ovh.Client pointed at
// it. go-ovh's endpoint resolution treats any string containing "/" as a
// literal URL (see loadConfig in the ovh module), so the real signing/
// request-marshaling code path is exercised end-to-end, the same way
// hetzner_images_test.go exercises the real hcloud-go client against a fake
// server rather than a hand-rolled interface. Every authenticated call
// starts with a GET /auth/time, which the handler below serves generically.
func newOvhFakeServer(t *testing.T, handler http.HandlerFunc) (*httptest.Server, *ovh.Client) {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/auth/time", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(time.Now().Unix())
	})
	mux.HandleFunc("/", handler)
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	client, err := ovh.NewClient(srv.URL, "test-app-key", "test-app-secret", "test-consumer-key")
	require.NoError(t, err)
	return srv, client
}

func testConfig() OvhConfig {
	return OvhConfig{
		ServiceName: "svc123",
		Region:      "GRA11",
		VMType:      "d2-4",
		Image:       "Ubuntu 22.04",
		Username:    "ubuntu",
	}
}

func TestProviderOvh_Deploy(t *testing.T) {
	const flavorID = "flavor-abc"
	const imageID = "image-def"
	const instanceID = "inst-123"

	_, client := newOvhFakeServer(t, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == "GET" && r.URL.Path == "/cloud/project/svc123/flavor":
			_ = json.NewEncoder(w).Encode([]ovhFlavor{
				{ID: flavorID, Name: "d2-4", Region: "GRA11"},
				{ID: "other", Name: "d2-4", Region: "BHS5"}, // wrong region, must not match
			})
		case r.Method == "GET" && r.URL.Path == "/cloud/project/svc123/image":
			_ = json.NewEncoder(w).Encode([]ovhImage{
				{ID: imageID, Name: "Ubuntu 22.04", Region: "GRA11"},
			})
		case r.Method == "POST" && r.URL.Path == "/cloud/project/svc123/instance":
			var req ovhInstanceCreate
			_ = json.NewDecoder(r.Body).Decode(&req)
			assert.Equal(t, flavorID, req.FlavorID, "must resolve the flavor name to its id before creating")
			assert.Equal(t, imageID, req.ImageID, "must resolve the image name to its id before creating")
			assert.Equal(t, "GRA11", req.Region)
			_ = json.NewEncoder(w).Encode(ovhInstance{ID: instanceID, Name: "my-vm", Status: "BUILD", Region: "GRA11"})
		case r.Method == "GET" && r.URL.Path == "/cloud/project/svc123/instance/"+instanceID:
			// First poll already reports ACTIVE, so Deploy doesn't block on
			// the real 3s inter-poll sleep in waitForStatus.
			_ = json.NewEncoder(w).Encode(ovhInstance{
				ID: instanceID, Name: "my-vm", Status: ovhStatusActive, Region: "GRA11",
				IPAddresses: []ovhIPAddress{
					{IP: "203.0.113.10", Type: "public"},
					{IP: "10.0.0.5", Type: "private"},
				},
			})
		default:
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
	})

	p := ProviderOvh{Client: client, Config: testConfig()}
	vm, err := p.Deploy(Vm{Name: "my-vm"})
	require.NoError(t, err)
	assert.Equal(t, "ovh", vm.Provider)
	assert.Equal(t, instanceID, vm.ID)
	assert.Equal(t, "203.0.113.10", vm.IP)
	assert.Equal(t, "10.0.0.5", vm.PrivateIP)
	assert.Equal(t, ovhStatusActive, vm.Status)
}

func TestProviderOvh_Deploy_FlavorNotFound(t *testing.T) {
	_, client := newOvhFakeServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "GET" && r.URL.Path == "/cloud/project/svc123/flavor" {
			_ = json.NewEncoder(w).Encode([]ovhFlavor{})
			return
		}
		t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
	})

	p := ProviderOvh{Client: client, Config: testConfig()}
	_, err := p.Deploy(Vm{Name: "my-vm"})
	require.Error(t, err, "an unresolvable flavor name must fail loudly, not silently fall back to some default")
}

func TestProviderOvh_Pause_Resume(t *testing.T) {
	const instanceID = "inst-123"
	var shelved, unshelved bool

	_, client := newOvhFakeServer(t, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == "GET" && r.URL.Path == "/cloud/project/svc123/instance":
			_ = json.NewEncoder(w).Encode([]ovhInstance{{ID: instanceID, Name: "my-vm", Status: ovhStatusActive}})
		case r.Method == "POST" && r.URL.Path == "/cloud/project/svc123/instance/"+instanceID+"/shelve":
			shelved = true
			w.WriteHeader(http.StatusOK)
		case r.Method == "POST" && r.URL.Path == "/cloud/project/svc123/instance/"+instanceID+"/unshelve":
			unshelved = true
			w.WriteHeader(http.StatusOK)
		case r.Method == "GET" && r.URL.Path == "/cloud/project/svc123/instance/"+instanceID:
			_ = json.NewEncoder(w).Encode(ovhInstance{ID: instanceID, Name: "my-vm", Status: ovhStatusActive})
		default:
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
	})

	p := ProviderOvh{Client: client, Config: testConfig()}

	err := p.Pause(Vm{Name: "my-vm"}, false)
	require.NoError(t, err)
	assert.True(t, shelved, "Pause must call the shelve endpoint, OVH's built-in stop-billing action")

	vm, err := p.Resume(Vm{Name: "my-vm"})
	require.NoError(t, err)
	assert.True(t, unshelved, "Resume must call the unshelve endpoint")
	assert.Equal(t, ovhStatusActive, vm.Status)
}

func TestProviderOvh_List_ListPaused(t *testing.T) {
	_, client := newOvhFakeServer(t, func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "/cloud/project/svc123/instance", r.URL.Path)
		_ = json.NewEncoder(w).Encode([]ovhInstance{
			{ID: "1", Name: "running-vm", Status: ovhStatusActive},
			{ID: "2", Name: "shelved-vm", Status: ovhStatusShelved},
			{ID: "3", Name: "offloaded-vm", Status: ovhStatusShelvedOffloaded},
		})
	})

	p := ProviderOvh{Client: client, Config: testConfig()}

	all, err := p.List()
	require.NoError(t, err)
	assert.Len(t, all.List, 3, "List returns every instance in the project -- OVH has no tags to scope by")

	paused, err := p.ListPaused()
	require.NoError(t, err)
	require.Len(t, paused.List, 2)
	names := []string{paused.List[0].Name, paused.List[1].Name}
	assert.ElementsMatch(t, []string{"shelved-vm", "offloaded-vm"}, names)
}

func TestProviderOvh_GetByName_NotFound(t *testing.T) {
	_, client := newOvhFakeServer(t, func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode([]ovhInstance{{ID: "1", Name: "other-vm"}})
	})

	p := ProviderOvh{Client: client, Config: testConfig()}
	_, err := p.GetByName("missing-vm")
	assert.Error(t, err)
}

func TestProviderOvh_Destroy_ResolvesIDByName(t *testing.T) {
	const instanceID = "inst-123"
	var deleted bool

	_, client := newOvhFakeServer(t, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == "GET" && r.URL.Path == "/cloud/project/svc123/instance":
			_ = json.NewEncoder(w).Encode([]ovhInstance{{ID: instanceID, Name: "my-vm"}})
		case r.Method == "DELETE" && r.URL.Path == "/cloud/project/svc123/instance/"+instanceID:
			deleted = true
			w.WriteHeader(http.StatusOK)
		default:
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
	})

	p := ProviderOvh{Client: client, Config: testConfig()}
	err := p.Destroy(Vm{Name: "my-vm"}) // no ID given, must resolve via GetByName first
	require.NoError(t, err)
	assert.True(t, deleted)
}

func TestProviderOvh_CreateSSHKey(t *testing.T) {
	pubKeyPath := writeTempSSHKey(t)
	pubKeyBytes, err := os.ReadFile(pubKeyPath)
	require.NoError(t, err)

	t.Run("reuses an existing key matched by fingerprint, not name", func(t *testing.T) {
		var posted bool
		_, client := newOvhFakeServer(t, func(w http.ResponseWriter, r *http.Request) {
			switch {
			case r.Method == "GET" && r.URL.Path == "/cloud/project/svc123/sshkey":
				_ = json.NewEncoder(w).Encode([]ovhSSHKey{
					{ID: "existing-id", Name: "some-other-name", PublicKey: string(pubKeyBytes)},
				})
			case r.Method == "POST" && r.URL.Path == "/cloud/project/svc123/sshkey":
				posted = true
				t.Fatal("must not create a duplicate key when one with matching key material already exists")
			default:
				t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
			}
		})

		p := ProviderOvh{Client: client, Config: testConfig()}
		id, err := p.CreateSSHKey(pubKeyPath)
		require.NoError(t, err)
		assert.Equal(t, "existing-id", id)
		assert.False(t, posted)
	})

	t.Run("creates a new key when none matches", func(t *testing.T) {
		_, client := newOvhFakeServer(t, func(w http.ResponseWriter, r *http.Request) {
			switch {
			case r.Method == "GET" && r.URL.Path == "/cloud/project/svc123/sshkey":
				_ = json.NewEncoder(w).Encode([]ovhSSHKey{})
			case r.Method == "POST" && r.URL.Path == "/cloud/project/svc123/sshkey":
				var req ovhSSHKeyCreate
				_ = json.NewDecoder(r.Body).Decode(&req)
				assert.NotEmpty(t, req.Name)
				_ = json.NewEncoder(w).Encode(ovhSSHKey{ID: "new-id", Name: req.Name, PublicKey: req.PublicKey})
			default:
				t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
			}
		})

		p := ProviderOvh{Client: client, Config: testConfig()}
		id, err := p.CreateSSHKey(pubKeyPath)
		require.NoError(t, err)
		assert.Equal(t, "new-id", id)
	})
}

// writeTempSSHKey writes a syntactically valid ed25519 authorized_keys line
// to a temp file and returns its path.
func writeTempSSHKey(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	path := dir + "/id_ed25519.pub"
	const pubKey = "ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIJ+2GJ7fkgD8/BSCyH8UYbqCWk9j6dcxE5oZ8YQ3P8bV test@onctl\n"
	require.NoError(t, os.WriteFile(path, []byte(pubKey), 0600))
	return path
}

func TestMapOvhInstance(t *testing.T) {
	t.Run("splits public and private addresses", func(t *testing.T) {
		vm := mapOvhInstance(ovhInstance{
			ID: "1", Name: "vm1", Status: "ACTIVE", Region: "GRA11", FlavorID: "flavor-1",
			IPAddresses: []ovhIPAddress{
				{IP: "203.0.113.1", Type: "public"},
				{IP: "10.0.0.1", Type: "private"},
			},
		})
		assert.Equal(t, "ovh", vm.Provider)
		assert.Equal(t, "203.0.113.1", vm.IP)
		assert.Equal(t, "10.0.0.1", vm.PrivateIP)
	})

	t.Run("private IP defaults to N/A when absent", func(t *testing.T) {
		vm := mapOvhInstance(ovhInstance{
			ID: "1", Name: "vm1",
			IPAddresses: []ovhIPAddress{{IP: "203.0.113.1", Type: "public"}},
		})
		assert.Equal(t, "N/A", vm.PrivateIP)
	})
}
