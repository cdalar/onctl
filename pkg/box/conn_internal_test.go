package box

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/pem"
	"os"
	"path/filepath"
	"testing"

	"github.com/cdalar/onctl/pkg/cloud"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/crypto/ssh"
)

// An imported host connects with what `onctl import` recorded (user,
// port, key), not the provider defaults; only an explicit port overrides
// the inventory's. A host missing from the inventory is an error, not a
// root@ guess.
func TestConnect_StaticUsesInventory(t *testing.T) {
	home := t.TempDir()
	key := filepath.Join(home, "imported_key")
	require.NoError(t, os.WriteFile(key, []byte("PRIVATE"), 0o600))
	require.NoError(t, os.WriteFile(key+".pub", []byte("ssh-ed25519 AAAA"), 0o644))
	sp := &cloud.ProviderStatic{InventoryPath: filepath.Join(home, "onctl_config")}
	require.NoError(t, sp.SaveInventory(cloud.StaticInventory{Hosts: []cloud.StaticHost{
		{Name: "legacy", IP: "10.0.0.5", Username: "ubuntu", SSHPort: 2222, PrivateKey: key},
	}}))
	c := Wrap("static", sp)
	vm := cloud.Vm{Name: "legacy", IP: "10.0.0.5"}

	conn, err := c.Connect(vm, ConnectOptions{})
	require.NoError(t, err)
	assert.Equal(t, "ubuntu", conn.r.Username)
	assert.Equal(t, 2222, conn.r.SSHPort, "the inventory's port when none is asked for")
	assert.Equal(t, "10.0.0.5", conn.r.IPAddress)
	assert.Equal(t, "PRIVATE", conn.r.PrivateKey)

	conn, err = c.Connect(vm, ConnectOptions{Port: 2200})
	require.NoError(t, err)
	assert.Equal(t, 2200, conn.r.SSHPort, "an explicit port wins over the inventory")

	_, err = c.Connect(cloud.Vm{Name: "ghost"}, ConnectOptions{})
	assert.Error(t, err)
}

// Elsewhere the user is the provider's configured one, else root, and
// the port 22.
func TestConnect_Defaults(t *testing.T) {
	key := filepath.Join(t.TempDir(), "id")
	require.NoError(t, os.WriteFile(key, []byte("PRIVATE"), 0o600))
	vm := cloud.Vm{Name: "web", IP: "10.0.0.2"}

	viper.Set("aws.vm.username", "ubuntu")
	t.Cleanup(func() { viper.Set("aws.vm.username", nil) })
	conn, err := Wrap("aws", cloud.ProviderAws{}).Connect(vm, ConnectOptions{KeyFile: key})
	require.NoError(t, err)
	assert.Equal(t, "ubuntu", conn.User())
	assert.Equal(t, 22, conn.r.SSHPort)

	conn, err = Wrap("fc", cloud.ProviderFC{}).Connect(vm, ConnectOptions{KeyFile: key})
	require.NoError(t, err)
	assert.Equal(t, "root", conn.User(), "no configured username is root")
}

// An encrypted key with no Passphrase callback fails with ErrEncryptedKey
// rather than prompting on a terminal a server doesn't have.
func TestConnect_EncryptedKeyWithoutPassphrase(t *testing.T) {
	conn, err := Wrap("fc", cloud.ProviderFC{}).Connect(cloud.Vm{Name: "web", IP: "127.0.0.1"}, ConnectOptions{KeyFile: writeEncryptedKey(t), Port: 1})
	require.NoError(t, err)
	assert.ErrorIs(t, conn.Open(), ErrEncryptedKey)
}

func writeEncryptedKey(t *testing.T) string {
	t.Helper()
	_, priv, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)
	block, err := ssh.MarshalPrivateKeyWithPassphrase(priv, "", []byte("secret"))
	require.NoError(t, err)
	path := filepath.Join(t.TempDir(), "id_ed25519")
	require.NoError(t, os.WriteFile(path, pem.EncodeToMemory(block), 0o600))
	return path
}
