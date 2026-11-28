package cloud

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	compute "cloud.google.com/go/compute/apiv1"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/api/option"
)

const gcpInstanceJSON = `{"id":"123","name":"vm1","creationTimestamp":"2026-01-02T03:04:05Z",
"machineType":"https://x/zones/z1/machineTypes/e2-micro","status":"RUNNING","zone":"https://x/zones/z1",
"networkInterfaces":[{"accessConfigs":[{"natIP":"1.2.3.4"}]}]}`

// gcpDone is a finished zone operation: what Insert, Start, Stop and
// Delete answer, and what waiting on one answers.
const gcpDone = `{"name":"op-1","status":"DONE","zone":"https://x/zones/z1"}`

// gcpRequest is one request the fake received.
type gcpRequest struct {
	Method, Path, Body string
}

// fakeGCP serves the Compute REST API from a table of "METHOD path-suffix"
// to JSON. Anything not in the table is a 404, so a call the test didn't
// expect shows up as an error rather than a hang.
type fakeGCP struct {
	mu    sync.Mutex
	table map[string]string
	reqs  []gcpRequest
}

func newFakeGCP(t *testing.T, table map[string]string) (*fakeGCP, ProviderGcp) {
	t.Helper()
	f := &fakeGCP{table: table}
	srv := httptest.NewServer(f)
	t.Cleanup(srv.Close)

	viper.Set("gcp.project", "proj")
	viper.Set("gcp.zone", "z1")
	t.Cleanup(func() {
		viper.Set("gcp.project", nil)
		viper.Set("gcp.zone", nil)
		viper.Set("gcp.type", nil)
		viper.Set("gcp.vm.username", nil)
		viper.Set("gcp.vm.nestedVirtualization", nil)
	})

	opts := []option.ClientOption{option.WithEndpoint(srv.URL), option.WithoutAuthentication()}
	c, err := compute.NewInstancesRESTClient(context.Background(), opts...)
	require.NoError(t, err)
	t.Cleanup(func() { _ = c.Close() })
	return f, ProviderGcp{Client: c}
}

func (f *fakeGCP) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	body, _ := io.ReadAll(r.Body)
	f.mu.Lock()
	f.reqs = append(f.reqs, gcpRequest{r.Method, r.URL.Path, string(body)})
	defer f.mu.Unlock()

	w.Header().Set("Content-Type", "application/json")
	for key, reply := range f.table {
		method, suffix, _ := strings.Cut(key, " ")
		if r.Method == method && strings.HasSuffix(r.URL.Path, suffix) {
			_, _ = io.WriteString(w, reply)
			return
		}
	}
	w.WriteHeader(http.StatusNotFound)
	_, _ = io.WriteString(w, `{"error":{"code":404,"message":"not found"}}`)
}

func (f *fakeGCP) requests() []gcpRequest {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]gcpRequest(nil), f.reqs...)
}

const gcpInstancePath = "/projects/proj/zones/z1/instances/vm1"

func TestGcpListAndGetByName(t *testing.T) {
	_, p := newFakeGCP(t, map[string]string{
		"GET /projects/proj/aggregated/instances": `{"items":{"zones/z1":{"instances":[` + gcpInstanceJSON + `]},"zones/z2":{}}}`,
		"GET " + gcpInstancePath:                  gcpInstanceJSON,
	})

	list, err := p.List()
	require.NoError(t, err)
	require.Len(t, list.List, 1)
	vm := list.List[0]
	assert.Equal(t, "gcp", vm.Provider)
	assert.Equal(t, "123", vm.ID)
	assert.Equal(t, "vm1", vm.Name)
	assert.Equal(t, "1.2.3.4", vm.IP)
	assert.Equal(t, "e2-micro", vm.Type)
	assert.Equal(t, "z1", vm.Location)

	got, err := p.GetByName("vm1")
	require.NoError(t, err)
	assert.Equal(t, vm.ID, got.ID)

	paused, err := p.ListPaused()
	assert.NoError(t, err)
	assert.Empty(t, paused.List, "stopped instances already show up in List")
}

func TestGcpPauseAndResume(t *testing.T) {
	f, p := newFakeGCP(t, map[string]string{
		"POST " + gcpInstancePath + "/stop":           gcpDone,
		"POST " + gcpInstancePath + "/start":          gcpDone,
		"GET /projects/proj/zones/z1/operations/op-1": gcpDone,
		"GET " + gcpInstancePath:                      gcpInstanceJSON,
	})

	require.NoError(t, p.Pause(Vm{Name: "vm1"}, false))
	vm, err := p.Resume(Vm{Name: "vm1"})
	require.NoError(t, err)
	assert.Equal(t, "RUNNING", vm.Status)

	var paths []string
	for _, r := range f.requests() {
		paths = append(paths, r.Method+" "+r.Path)
	}
	assert.Contains(t, paths, "POST /compute/v1"+gcpInstancePath+"/stop")
	assert.Contains(t, paths, "POST /compute/v1"+gcpInstancePath+"/start")
}

func TestGcpPauseResumeErrors(t *testing.T) {
	_, p := newFakeGCP(t, map[string]string{}) // every call is a 404
	assert.Error(t, p.Pause(Vm{Name: "vm1"}, false))
	_, err := p.Resume(Vm{Name: "vm1"})
	assert.Error(t, err)
}

func TestGcpDestroy(t *testing.T) {
	f, p := newFakeGCP(t, map[string]string{
		"DELETE " + gcpInstancePath: gcpDone,
		"GET " + gcpInstancePath:    gcpInstanceJSON,
	})
	require.NoError(t, p.Destroy(Vm{ID: "123", Name: "vm1"}))
	last := f.requests()[len(f.requests())-1]
	assert.Equal(t, "DELETE", last.Method)

	// Name only: it is resolved first, so a missing instance is reported
	// before anything is deleted.
	require.NoError(t, p.Destroy(Vm{Name: "vm1"}))
}

func TestGcpDeploy(t *testing.T) {
	f, p := newFakeGCP(t, map[string]string{
		"POST /projects/proj/zones/z1/instances":      gcpDone,
		"GET /projects/proj/zones/z1/operations/op-1": gcpDone,
		"GET " + gcpInstancePath:                      gcpInstanceJSON,
	})
	viper.Set("gcp.type", "e2-micro")
	viper.Set("gcp.vm.username", "ubuntu")
	viper.Set("gcp.vm.nestedVirtualization", true)

	cloudInit := filepath.Join(t.TempDir(), "ci.yaml")
	require.NoError(t, os.WriteFile(cloudInit, []byte("#cloud-config\n"), 0o600))

	vm, err := p.Deploy(Vm{Name: "vm1", CloudInitFile: cloudInit})
	require.NoError(t, err)
	assert.Equal(t, "vm1", vm.Name)

	var insert gcpRequest
	for _, r := range f.requests() {
		if r.Method == "POST" && strings.HasSuffix(r.Path, "/instances") {
			insert = r
		}
	}
	require.NotEmpty(t, insert.Body, "no insert request was sent")
	var sent struct {
		Name                    string `json:"name"`
		MachineType             string `json:"machineType"`
		MinCPUPlatform          string `json:"minCpuPlatform"`
		AdvancedMachineFeatures struct {
			EnableNestedVirtualization bool `json:"enableNestedVirtualization"`
		} `json:"advancedMachineFeatures"`
	}
	require.NoError(t, json.Unmarshal([]byte(insert.Body), &sent))
	assert.Equal(t, "vm1", sent.Name)
	assert.Equal(t, "zones/z1/machineTypes/e2-micro", sent.MachineType)
	// KVM-based guests (Firecracker) only boot with nested virtualization.
	assert.True(t, sent.AdvancedMachineFeatures.EnableNestedVirtualization)
	assert.Equal(t, "Intel Haswell", sent.MinCPUPlatform)
}

func TestGcpDeployWithoutNestedVirtualization(t *testing.T) {
	f, p := newFakeGCP(t, map[string]string{
		"POST /projects/proj/zones/z1/instances":      gcpDone,
		"GET /projects/proj/zones/z1/operations/op-1": gcpDone,
		"GET " + gcpInstancePath:                      gcpInstanceJSON,
	})
	cloudInit := filepath.Join(t.TempDir(), "ci.yaml")
	require.NoError(t, os.WriteFile(cloudInit, []byte("x"), 0o600))
	_, err := p.Deploy(Vm{Name: "vm1", CloudInitFile: cloudInit})
	require.NoError(t, err)
	for _, r := range f.requests() {
		if r.Method == "POST" && strings.HasSuffix(r.Path, "/instances") {
			assert.NotContains(t, r.Body, "enableNestedVirtualization", "opt-in only: most machine types reject it")
		}
	}
}

func TestGcpCreateSSHKey(t *testing.T) {
	_, p := newFakeGCP(t, nil)
	pub := filepath.Join(t.TempDir(), "id.pub")
	require.NoError(t, os.WriteFile(pub, []byte("ssh-ed25519 AAAA test"), 0o600))
	_, err := p.CreateSSHKey(pub)
	require.NoError(t, err)
	assert.Equal(t, "ssh-ed25519 AAAA test", string(publicKey))
}
