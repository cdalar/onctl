package cmd

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/cdalar/onctl/internal/files"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
)

// TestGenericCreateFlagsExist verifies the YAML-replacing flags are registered.
func TestGenericCreateFlagsExist(t *testing.T) {
	for _, name := range []string{"type", "location", "username", "cloud-init-timeout", "image",
		"kernel-image", "rootfs-image", "fc-binary", "vcpu", "memory", "project"} {
		assert.NotNil(t, createCmd.Flags().Lookup(name), "create should have --%s flag", name)
	}
	assert.NotNil(t, rootCmd.PersistentFlags().Lookup("provider"), "root should have persistent --provider flag")
	assert.NotNil(t, actionCmd.Flags().Lookup("github-owner"), "action should have --github-owner flag")
}

// TestCreateFlagsBindToViper verifies that setting a create flag is reflected
// through its viper binding (the path the rest of the code reads), and that
// the default value matches the production onctl.yaml template written by
// `onctl init`. Deterministic: it sets values explicitly and restores them.
func TestCreateFlagsBindToViper(t *testing.T) {
	// Loading the real init template (rather than an empty config) clears any
	// values a sibling test (e.g. TestReadConfig_WithValidConfig) left behind
	// in the shared, global viper instance, so this test's outcome doesn't
	// depend on `go test`'s randomized run order, while also keeping this
	// test's expectations in sync with the file onctl init actually writes.
	tempDir, err := os.MkdirTemp("", "onctl-flags-test")
	assert.NoError(t, err)
	defer func() { _ = os.RemoveAll(tempDir) }()
	onctlDir := filepath.Join(tempDir, ".onctl")
	assert.NoError(t, os.Mkdir(onctlDir, 0755))
	template, err := files.EmbededFiles.ReadFile("init/onctl.yaml")
	assert.NoError(t, err)
	assert.NoError(t, os.WriteFile(filepath.Join(onctlDir, "onctl.yaml"), template, 0644))

	originalWd, err := os.Getwd()
	assert.NoError(t, err)
	defer func() { _ = os.Chdir(originalWd) }()
	assert.NoError(t, os.Chdir(tempDir))
	assert.NoError(t, ReadConfig())

	cases := []struct{ flag, key, def, override string }{
		{"type", "hetzner.vm.type", "cpx21", "cpx31"},
		{"location", "hetzner.location", "fsn1", "nbg1"},
		{"username", "hetzner.vm.username", "root", "admin"},
		{"cloud-init-timeout", "vm.cloud-init.timeout", "3m", "180s"},
		// Firecracker (replaces fc.yaml).
		{"kernel-image", "fc.kernelImage", "~/.onctl/firecracker/images/vmlinux", "/img/vmlinux"},
		{"rootfs-image", "fc.rootfsImage", "~/.onctl/firecracker/images/rootfs.ext4", "/img/rootfs.ext4"},
		{"fc-binary", "fc.binPath", "firecracker", "/usr/local/bin/firecracker"},
		{"vcpu", "fc.vcpuCount", "1", "4"},
		{"memory", "fc.memSizeMib", "512", "2048"},
		// AWS (replaces aws.yaml).
		{"type", "aws.vm.type", "t2.micro", "m5.large"},
		{"location", "aws.location", "eu-central-1", "us-east-1"},
		{"username", "aws.vm.username", "ubuntu", "ec2-user"},
		// GCP (from onctl.yaml gcp: section; project is account-specific placeholder
		// resolved via gcloud or --project; tested separately).
		{"type", "gcp.type", "n1-standard-1", "n2-standard-2"},
		{"location", "gcp.zone", "europe-west4-a", "us-central1-a"},
	}
	// Check every default before mutating any flag. Several keys share a
	// flag with a different per-provider default (e.g. --type backs both
	// hetzner.vm.type=cpx21 and aws.vm.type=t2.micro): once a flag is marked
	// Changed -- even by setting it back to its original value -- viper's
	// flag layer outranks SetDefault for every key bound to that flag, so
	// checking defaults and overrides in the same pass would make later
	// cases see the wrong "default".
	for _, c := range cases {
		assert.Equal(t, c.def, viper.GetString(c.key), "default for %s", c.key)
	}
	assert.Equal(t, "root", viper.GetString("fc.vm.username"))
	assert.Equal(t, "root", viper.GetString("gcp.vm.username"))

	for _, c := range cases {
		assert.NoError(t, createCmd.Flags().Set(c.flag, c.override))
		assert.Equal(t, c.override, viper.GetString(c.key), "override for %s", c.key)
		assert.NoError(t, createCmd.Flags().Set(c.flag, c.def))
	}

	// The generic --username flag drives the Firecracker and GCP users too.
	assert.NoError(t, createCmd.Flags().Set("username", "admin"))
	assert.Equal(t, "admin", viper.GetString("fc.vm.username"))
	assert.Equal(t, "admin", viper.GetString("gcp.vm.username"))

	// Restore the shared flags to their own intrinsic defaults (not
	// whichever per-provider case happened to run last) so sibling tests in
	// this package see the same state as before this test ran.
	assert.NoError(t, createCmd.Flags().Set("type", "cpx21"))
	assert.NoError(t, createCmd.Flags().Set("location", "fsn1"))
	assert.NoError(t, createCmd.Flags().Set("username", "root"))
}
