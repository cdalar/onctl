package cmd

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestImportFlagsExist(t *testing.T) {
	for _, name := range []string{"ip", "user", "port", "key"} {
		assert.NotNil(t, importCmd.Flags().Lookup(name), "import should have --%s flag", name)
	}
}

// TestImportCommand_WritesInventory runs the import command's RunE directly
// against a temp $HOME and verifies the host lands in
// ~/.ssh/onctl_config and is then visible through the static provider's
// List/GetByName, matching the only thing import.go's PersistentPreRunE
// skip-list lets it rely on: no configured cloud provider needed.
func TestImportCommand_WritesInventory(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

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
