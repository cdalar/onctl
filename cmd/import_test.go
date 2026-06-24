package cmd

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestImportFlagsExist(t *testing.T) {
	for _, name := range []string{"ip", "user", "port", "key"} {
		assert.NotNil(t, importCmd.Flags().Lookup(name), "import should have --%s flag", name)
	}
}

// TestImportCommand_WritesInventory runs the import command's RunE directly
// against a temp .onctl dir and verifies the host lands in imported.yaml and
// is then visible through the static provider's List/GetByName, matching
// the only thing import.go's PersistentPreRunE skip-list lets it rely on:
// resolveConfigDir, not a configured cloud provider.
func TestImportCommand_WritesInventory(t *testing.T) {
	tempDir := t.TempDir()
	onctlDir := filepath.Join(tempDir, ".onctl")
	assert.NoError(t, os.Mkdir(onctlDir, 0755))

	originalWd, err := os.Getwd()
	assert.NoError(t, err)
	defer func() { _ = os.Chdir(originalWd) }()
	assert.NoError(t, os.Chdir(tempDir))

	importOpt = cmdImportOptions{
		IP:       "10.0.0.5",
		Username: "root",
		Port:     22,
	}
	assert.NoError(t, importCmd.RunE(importCmd, []string{"my-imported-box"}))

	p, err := staticProvider()
	assert.NoError(t, err)

	vm, err := p.GetByName("my-imported-box")
	assert.NoError(t, err)
	assert.Equal(t, "10.0.0.5", vm.IP)

	list, err := p.List()
	assert.NoError(t, err)
	assert.Len(t, list.List, 1)

	// Re-importing the same name updates rather than duplicates the entry.
	importOpt.IP = "10.0.0.6"
	assert.NoError(t, importCmd.RunE(importCmd, []string{"my-imported-box"}))
	list, err = p.List()
	assert.NoError(t, err)
	assert.Len(t, list.List, 1)
	assert.Equal(t, "10.0.0.6", list.List[0].IP)
}
