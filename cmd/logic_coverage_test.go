package cmd

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/cdalar/onctl/internal/files"
	"github.com/cdalar/onctl/pkg/cloud"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// An embedded template must be materialised to a temp file with its exact
// content, so `onctl create -a docker.sh` runs what shipped in the binary.
func TestFindSingleFile_EmbeddedFile(t *testing.T) {
	t.Chdir(t.TempDir()) // make sure the name is not found on the filesystem

	want, err := files.EmbededFiles.ReadFile("docker.sh")
	require.NoError(t, err)

	got := findSingleFile("docker.sh")
	t.Cleanup(func() { _ = os.RemoveAll(filepath.Dir(got)) })

	assert.Equal(t, "docker.sh", filepath.Base(got))
	content, err := os.ReadFile(got)
	require.NoError(t, err)
	assert.Equal(t, want, content)
}

// A name present on the filesystem wins over an embedded file of the same
// name, so users can override shipped templates locally.
func TestFindSingleFile_FilesystemOverridesEmbedded(t *testing.T) {
	t.Chdir(t.TempDir())
	require.NoError(t, os.WriteFile("docker.sh", []byte("local"), 0644))

	assert.Equal(t, "docker.sh", findSingleFile("docker.sh"))
}

func TestFindFile_ResolvesEachEntryInOrder(t *testing.T) {
	t.Chdir(t.TempDir())
	require.NoError(t, os.WriteFile("a.sh", nil, 0644))
	require.NoError(t, os.WriteFile("b.sh", nil, 0644))

	assert.Equal(t, []string{"a.sh", "b.sh"}, findFile([]string{"a.sh", "b.sh"}))
	assert.Nil(t, findFile(nil))
}

// fakeAz puts an `az` executable first on PATH that answers the two queries
// resolveAzureIdentifiers makes, so the tests do not depend on a real az login.
func fakeAz(t *testing.T, subscription, group string) {
	t.Helper()
	dir := t.TempDir()
	script := "#!/bin/sh\n" +
		"case \"$1\" in\n" +
		"  account) echo '" + subscription + "' ;;\n" +
		"  config) echo '" + group + "' ;;\n" +
		"esac\n"
	require.NoError(t, os.WriteFile(filepath.Join(dir, "az"), []byte(script), 0755))
	t.Setenv("PATH", dir)
}

func resetAzureViper(t *testing.T, subscription, group string) {
	t.Helper()
	withViper(t, "azure.subscriptionId", subscription)
	withViper(t, "azure.resourceGroup", group)
}

// Explicit config values must never be overwritten by whatever az happens to
// have selected, otherwise onctl could act on the wrong subscription.
func TestResolveAzureIdentifiers_KeepsExplicitValues(t *testing.T) {
	fakeAz(t, "from-az", "rg-from-az")
	resetAzureViper(t, "my-sub", "my-rg")

	require.NoError(t, resolveAzureIdentifiers())

	assert.Equal(t, "my-sub", viper.GetString("azure.subscriptionId"))
	assert.Equal(t, "my-rg", viper.GetString("azure.resourceGroup"))
}

func TestResolveAzureIdentifiers_FillsPlaceholderAndEmptyFromAz(t *testing.T) {
	for _, sub := range []string{"", "<subscription-id>"} {
		t.Run("sub="+sub, func(t *testing.T) {
			fakeAz(t, "from-az", "rg-from-az")
			resetAzureViper(t, sub, "")

			require.NoError(t, resolveAzureIdentifiers())

			assert.Equal(t, "from-az", viper.GetString("azure.subscriptionId"))
			assert.Equal(t, "rg-from-az", viper.GetString("azure.resourceGroup"))
		})
	}
}

// Without a subscription there is nothing to talk to Azure with, so this
// must fail loudly instead of continuing with a placeholder.
func TestResolveAzureIdentifiers_ErrorsWithoutSubscription(t *testing.T) {
	t.Setenv("PATH", t.TempDir()) // no az available
	resetAzureViper(t, "<subscription-id>", "")

	err := resolveAzureIdentifiers()

	require.Error(t, err)
	assert.Contains(t, err.Error(), "azure.subscriptionId is required")
}

// A missing resource group is legitimate (user passes it per command), so it
// must not turn into an error when az has no default either.
func TestResolveAzureIdentifiers_EmptyResourceGroupIsNotAnError(t *testing.T) {
	fakeAz(t, "from-az", "")
	resetAzureViper(t, "my-sub", "")

	require.NoError(t, resolveAzureIdentifiers())

	assert.Equal(t, "", viper.GetString("azure.resourceGroup"))
}

// An already-built provider must be left alone: completion functions call
// ensureProvider on every keystroke and must not rebuild clients each time.
func TestEnsureProvider_NoopWhenProviderAlreadySet(t *testing.T) {
	withProvider(t, &cloud.ProviderAws{})
	before := provider

	ensureProvider()

	assert.Same(t, before, provider)
}

func TestCheckCloudProvider_ReturnsSupportedEnvValue(t *testing.T) {
	orig := cloudProvider
	t.Cleanup(func() { cloudProvider = orig })

	for _, name := range cloudProviderList {
		t.Setenv("ONCTL_CLOUD", name)
		assert.Equal(t, name, checkCloudProvider())
	}
}
