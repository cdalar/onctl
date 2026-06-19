package cmd

import (
	"os"
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

// TestResolveProviderNames covers the provider-selection decision in
// isolation from provider construction (which, for gcp/azure, can log.Fatal
// without real credentials -- see resolveListProviders in list.go).
func TestResolveProviderNames(t *testing.T) {
	// Isolate detection from on-disk yamls (home + repo .onctl) so Detect only sees
	// the envs we set in subtests. Matches how the logic was tested originally.
	cleanHome := t.TempDir()
	t.Setenv("HOME", cleanHome)
	origDir, _ := os.Getwd()
	tmp := t.TempDir()
	os.Chdir(tmp)
	defer os.Chdir(origDir)

	envVars := []string{"AWS_ACCESS_KEY_ID", "AWS_PROFILE", "AZURE_CLIENT_ID", "GOOGLE_CREDENTIALS", "HCLOUD_TOKEN"}
	for _, v := range envVars {
		require.NoError(t, os.Unsetenv(v))
	}

	originalExplicit := providerExplicitlyChosen
	originalCloudProvider := cloudProvider
	defer func() {
		providerExplicitlyChosen = originalExplicit
		cloudProvider = originalCloudProvider
	}()

	t.Run("explicit provider, multiple credentials present, still single", func(t *testing.T) {
		t.Setenv("AWS_ACCESS_KEY_ID", "AKIAIOSFODNN7EXAMPLE")
		t.Setenv("HCLOUD_TOKEN", "some-hcloud-token")
		providerExplicitlyChosen = true
		cloudProvider = "aws"
		assert.Equal(t, []string{"aws"}, resolveProviderNames())
	})

	t.Run("not explicit, zero or one credential detected, falls back to resolved provider", func(t *testing.T) {
		providerExplicitlyChosen = false
		cloudProvider = "hetzner"
		t.Setenv("HCLOUD_TOKEN", "some-hcloud-token")
		assert.Equal(t, []string{"hetzner"}, resolveProviderNames())
	})

	t.Run("not explicit, multiple credentials detected, aggregates all", func(t *testing.T) {
		providerExplicitlyChosen = false
		cloudProvider = "aws"
		t.Setenv("AWS_ACCESS_KEY_ID", "AKIAIOSFODNN7EXAMPLE")
		t.Setenv("HCLOUD_TOKEN", "some-hcloud-token")
		assert.Equal(t, []string{"aws", "hetzner"}, resolveProviderNames())
	})
}

func TestBuildProvider_UnknownReturnsNil(t *testing.T) {
	p := buildProvider("not-a-real-provider")
	assert.Nil(t, p)
}

func TestResolveListProviders_IncludesNamed(t *testing.T) {
	// Save/restore global provider state touched by resolver
	origProvider := provider
	origCloud := cloudProvider
	origExplicit := providerExplicitlyChosen
	defer func() {
		provider = origProvider
		cloudProvider = origCloud
		providerExplicitlyChosen = origExplicit
	}()

	// Force single explicit provider path that re-uses the (possibly nil) global
	providerExplicitlyChosen = true
	cloudProvider = "hetzner"
	// No real client; resolver should still return one entry tagged by name
	names := resolveProviderNames()
	assert.Equal(t, []string{"hetzner"}, names)
	// resolveListProviders will use the global if match and not try building
	lps := resolveListProviders()
	assert.Len(t, lps, 1)
	assert.Equal(t, "hetzner", lps[0].name)
}
