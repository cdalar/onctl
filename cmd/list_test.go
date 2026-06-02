package cmd

import (
	"testing"
	"time"

	"github.com/cdalar/onctl/internal/cloud"
	"github.com/stretchr/testify/assert"
)

func TestListCmd_CommandProperties(t *testing.T) {
	// Test that the command has the expected properties
	assert.Equal(t, "ls", listCmd.Use)
	assert.Contains(t, listCmd.Aliases, "list")
	assert.Equal(t, "List VMs", listCmd.Short)
	assert.NotNil(t, listCmd.Run)
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
