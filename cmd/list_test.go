package cmd

import (
	"errors"
	"github.com/cdalar/onctl/pkg/box"
	"io"
	"os"
	"strconv"
	"testing"
	"time"

	"github.com/cdalar/onctl/pkg/cloud"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestListCmd_CommandProperties(t *testing.T) {
	// Test that the command has the expected properties
	assert.Equal(t, "ls", listCmd.Use)
	assert.Contains(t, listCmd.Aliases, "list")
	assert.Equal(t, "List VMs", listCmd.Short)
	assert.NotNil(t, listCmd.RunE)
}

func TestListCmd_HasFlags(t *testing.T) {
	// Test that flags are properly registered
	flag := listCmd.Flags().Lookup("output")
	assert.NotNil(t, flag, "list command should have 'output' flag")
	assert.Equal(t, "o", flag.Shorthand, "output flag should have 'o' shorthand")
	assert.Equal(t, "tab", flag.DefValue, "output flag should have 'tab' default value")
	assert.Equal(t, "output format (tab, json, yaml, puppet, ansible)", flag.Usage)
}

func TestListCmd_FlagDefaults(t *testing.T) {
	// Test that default values are correct
	assert.Equal(t, "tab", output, "output flag should default to 'tab'")
}

// TestIsPausedStatus documents the cross-provider paused/resumable
// convention (AWS "stopped", Hetzner "off", GCP "TERMINATED", Azure
// "...deallocated") and
// guards against fc's "dead" status (a non-resumable, process-is-gone state
// with no cross-provider equivalent) ever being folded back into it, which
// would misreport a dead-after-host-reboot microVM as merely "paused" in
// `onctl ls`.
func TestIsPausedStatus(t *testing.T) {
	tests := []struct {
		status string
		want   bool
	}{
		{"stopped", true},
		{"off", true},
		{"TERMINATED", true},
		{"VM deallocated", true},
		{"SHELVED", true},
		{"SHELVED_OFFLOADED", true},
		{"running", false},
		{"paused", false},
		{"dead", false},
		{"ACTIVE", false},
	}
	for _, tt := range tests {
		assert.Equal(t, tt.want, isPausedStatus(tt.status), "status %q", tt.status)
	}
}

// TestListCmd_PausedRowRenders verifies a paused server row (as produced by
// ListPaused) renders through TabWriter without error — the separate PAUSED table.
func TestListCmd_PausedRowRenders(t *testing.T) {
	paused := cloud.VmList{List: []cloud.Vm{{
		Provider:  "hetzner",
		ID:        "392931438",
		Name:      "api",
		Location:  "fsn1",
		Type:      "ccx13",
		IP:        "178.105.251.103",
		PrivateIP: "N/A",
		Status:    "paused",
		CreatedAt: time.Now(),
	}}}
	tmpl := "CLOUD\tID\tNAME\tLOCATION\tTYPE\tPUBLIC IP\tPRIVATE IP\tSTATE\tAGE\n{{range .List}}{{.Provider}}\t{{.ID}}\t{{.Name}}\t{{.Location}}\t{{.Type}}\t{{.IP}}\t{{.PrivateIP}}\t{{.Status}}\t{{durationFromCreatedAt .CreatedAt}}\n{{end}}"
	assert.NotPanics(t, func() { TabWriter(paused, tmpl) })
}

// TestListVMs_SnapshotHiddenWhileServerExists: mid pause/resume Hetzner has
// both the server and its snapshot, so List and ListPaused both return the
// name; the server is the current record and must be the only row.
func TestListVMs_SnapshotHiddenWhileServerExists(t *testing.T) {
	p := &tuiFakeProvider{
		vms:    []cloud.Vm{{Name: "api", Status: "initializing"}, {Name: "db", Status: "stopped"}},
		paused: []cloud.Vm{{Name: "api", Status: "paused"}, {Name: "web", Status: "paused"}},
	}
	running, paused, err := box.Wrap("hetzner", p).List()
	assert.NoError(t, err)
	assert.Equal(t, []cloud.Vm{{Name: "api", Status: "initializing"}}, running.List)
	assert.Equal(t, []cloud.Vm{{Name: "db", Status: "stopped"}, {Name: "web", Status: "paused"}}, paused.List)
}

// TestListCmd_ProviderErrorIsTheResult: a provider that can't list (the
// container CLI missing, no credentials) used to be logged and followed
// by an empty table and exit 0, so a script read "no VMs". The error is
// now what ls returns, so main exits non-zero; what did come back is
// still printed first.
func TestListCmd_ProviderErrorIsTheResult(t *testing.T) {
	boom := errors.New("the ac provider needs Apple's container CLI")
	withProvider(t, &tuiFakeProvider{listErr: boom})
	err := listCmd.RunE(listCmd, nil)
	assert.ErrorIs(t, err, boom)

	withProvider(t, &tuiFakeProvider{vms: []cloud.Vm{{Name: "api"}}})
	assert.NoError(t, listCmd.RunE(listCmd, nil))
}

// TestListCmd_JSONContract pins `onctl ls -o json`, which the boxctl host
// service parses (`onctl ls -o json -p fc`): one flat array, running VMs
// then paused, every cloud.Vm field under its Go name. A change here
// breaks that service.
func TestListCmd_JSONContract(t *testing.T) {
	created := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	withProvider(t, &tuiFakeProvider{
		vms: []cloud.Vm{
			{ID: "1", Name: "web", IP: "10.0.0.2", Status: "running", Provider: "fc", SSHReady: true, SSHPort: 22, CreatedAt: created},
			{ID: "2", Name: "db", Status: "stopped", Provider: "fc", CreatedAt: created},
		},
		paused: []cloud.Vm{{Name: "api", Status: "paused", Provider: "fc", CreatedAt: created}},
	})
	orig := output
	output = "json"
	t.Cleanup(func() { output = orig })

	r, w, err := os.Pipe()
	require.NoError(t, err)
	stdout := os.Stdout
	os.Stdout = w
	runErr := listCmd.RunE(listCmd, nil)
	os.Stdout = stdout
	require.NoError(t, w.Close())
	got, err := io.ReadAll(r)
	require.NoError(t, err)
	require.NoError(t, runErr)

	vm := func(id, name, ip, status string, ready bool, port int) string {
		return `{"ID":"` + id + `","Name":"` + name + `","IP":"` + ip + `","PrivateIP":"","Type":"","Image":"","BaseImage":"","Status":"` + status +
			`","SSHReady":` + strconv.FormatBool(ready) + `,"Location":"","SSHKeyID":"","SSHPort":` + strconv.Itoa(port) +
			`,"CloudInitFile":"","CreatedAt":"2026-01-02T03:04:05Z","Provider":"fc","Cost":{"Currency":"","CostPerHour":0,"CostPerMonth":0,"AccumulatedCost":0}}`
	}
	want := "[" + vm("1", "web", "10.0.0.2", "running", true, 22) + "," + vm("2", "db", "", "stopped", false, 0) + "," + vm("", "api", "", "paused", false, 0) + "]\n"
	assert.Equal(t, want, string(got))
}
