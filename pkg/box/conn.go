package box

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/cdalar/onctl/internal/tools"
	"github.com/cdalar/onctl/pkg/cloud"
	"github.com/spf13/viper"
)

// keyTypeFallbackOrder is the key basenames KeyFiles tries, in order, when
// the configured default key (ssh.publicKey/ssh.privateKey in onctl.yaml)
// isn't on disk. Newer, more secure key types first.
var keyTypeFallbackOrder = []string{"id_ed25519", "id_ecdsa", "id_rsa"}

// KeyFiles is the ssh key pair for name: a private key path or its
// ".pub", with ~ for the home directory. An empty name is the configured
// default (ssh.publicKey/ssh.privateKey), or when that pair isn't on disk,
// the first of id_ed25519, id_ecdsa and id_rsa beside it that is. An
// explicit name is never swapped for another.
func KeyFiles(name string) (publicKeyFile, privateKeyFile string) {
	home, err := os.UserHomeDir()
	if err != nil {
		log.Println(err)
	}
	usingConfigDefault := name == ""
	switch {
	case usingConfigDefault:
		publicKeyFile = viper.GetString("ssh.publicKey")
		privateKeyFile = viper.GetString("ssh.privateKey")
	case strings.HasSuffix(name, ".pub"):
		publicKeyFile, privateKeyFile = name, strings.TrimSuffix(name, ".pub")
	default:
		publicKeyFile, privateKeyFile = name+".pub", name
	}
	publicKeyFile = strings.Replace(publicKeyFile, "~", home, 1)
	privateKeyFile = strings.Replace(privateKeyFile, "~", home, 1)

	if usingConfigDefault && !(exists(publicKeyFile) && exists(privateKeyFile)) {
		keyDir := filepath.Dir(publicKeyFile)
		for _, candidate := range keyTypeFallbackOrder {
			pub, priv := filepath.Join(keyDir, candidate+".pub"), filepath.Join(keyDir, candidate)
			if exists(pub) && exists(priv) {
				log.Println("[DEBUG] configured SSH key", publicKeyFile, "not found, falling back to", pub)
				publicKeyFile, privateKeyFile = pub, priv
				break
			}
		}
	}
	log.Println("[DEBUG] publicKeyFile:", publicKeyFile)
	log.Println("[DEBUG] privateKeyFile:", privateKeyFile)
	return publicKeyFile, privateKeyFile
}

func exists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// ErrEncryptedKey is what Connect's ssh fails with when the key needs a
// passphrase and ConnectOptions.Passphrase is nil.
var ErrEncryptedKey = errors.New("the ssh key is encrypted and no passphrase was given")

// ConnectOptions says how Connect reaches a VM over ssh.
type ConnectOptions struct {
	// KeyFile is the private key, or its ".pub" (see KeyFiles). Empty is
	// an imported (static) host's own key, else the configured default.
	KeyFile string
	// Port is the VM's ssh port. 0 is an imported host's own, else 22.
	Port int
	// User is who to log in as. Empty is an imported host's own, else the
	// provider's <id>.vm.username, else root.
	User string
	// Passphrase is called when the key is encrypted. nil fails such a
	// key with ErrEncryptedKey.
	Passphrase func() (string, error)
	// Hint, when set, gets advice while WaitForSSH waits on a VM that
	// doesn't answer (a firewall, a VPN in the way); nil drops it.
	Hint func(string)
}

// Conn is an ssh connection to one VM, opened on first use: run commands,
// copy files, apply scripts, wait for it to be ready, forward ports.
type Conn struct {
	r   *tools.Remote
	vm  cloud.Vm
	fwd *tools.Forwarder
}

// Connect resolves how to ssh to vm and returns a Conn; it doesn't dial
// yet. On providers that put keys on existing VMs themselves (boxes) the
// public key beside the private one is authorized first.
func (c *Client) Connect(vm cloud.Vm, opts ConnectOptions) (*Conn, error) {
	r := &tools.Remote{IPAddress: vm.IP, SSHPort: opts.Port, Username: opts.User}
	keyName := opts.KeyFile
	if sp, ok := c.static(); ok {
		h, err := sp.GetHost(vm.Name)
		if err != nil {
			return nil, err
		}
		if r.Username == "" {
			r.Username = h.Username
		}
		if r.SSHPort == 0 {
			r.SSHPort = h.SSHPort
		}
		if keyName == "" {
			keyName = h.PrivateKey
		}
		if r.IPAddress == "" {
			r.IPAddress = h.IP
		}
	}
	if r.Username == "" {
		r.Username = viper.GetString(c.id + ".vm.username")
	}
	if r.Username == "" {
		r.Username = "root"
	}
	if r.SSHPort == 0 {
		r.SSHPort = 22
	}
	publicKeyFile, privateKeyFile := KeyFiles(keyName)
	key, err := os.ReadFile(privateKeyFile)
	if err != nil {
		return nil, err
	}
	r.PrivateKey = string(key)
	r.Hint = opts.Hint
	if r.Hint == nil {
		r.Hint = func(string) {}
	}
	r.AskPassphrase = opts.Passphrase
	if r.AskPassphrase == nil {
		r.AskPassphrase = func() (string, error) { return "", ErrEncryptedKey }
	}
	if pub, err := os.ReadFile(publicKeyFile); err == nil {
		if err := c.AuthorizeKey(vm, string(pub)); err != nil && !errors.Is(err, ErrUnsupported) {
			return nil, err
		}
	}
	if c.Tunneled() {
		r.Dial = func(ctx context.Context, port int) (net.Conn, error) {
			return c.Dial(ctx, vm, port)
		}
	}
	return &Conn{r: r, vm: vm, fwd: &tools.Forwarder{Remote: r}}, nil
}

func (c *Client) static() (interface {
	GetHost(string) (cloud.StaticHost, error)
}, bool) {
	switch p := c.p.(type) {
	case *cloud.ProviderStatic:
		return p, true
	case cloud.ProviderStatic:
		return p, true
	}
	return nil, false
}

// VM is the VM this Conn reaches.
func (c *Conn) VM() cloud.Vm { return c.vm }

// User is who Conn logs in as.
func (c *Conn) User() string { return c.r.Username }

// Open connects now, rather than on first use.
func (c *Conn) Open() error { return c.r.NewSSHConnection() }

// Close ends the ssh connection, if one is open.
func (c *Conn) Close() error {
	if c.r.Client == nil {
		return nil
	}
	err := c.r.Client.Close()
	c.r.Client = nil
	return err
}

// Run runs command with vars (NAME=value) in its environment and returns
// its stdout. A non-zero exit is an error (*ssh.ExitError).
func (c *Conn) Run(command string, vars ...string) (string, error) {
	return c.r.RemoteRun(&tools.RemoteRunConfig{Command: command, Vars: vars})
}

// Apply copies file to a new apply dir on the VM (~/.onctl/applyNN) and
// runs it there as root with vars, its output streamed to out. It keeps
// running if the connection drops; its log stays in the apply dir.
func (c *Conn) Apply(file string, vars []string, out io.Writer) error {
	return c.r.CopyAndRunRemoteFile(&tools.CopyAndRunRemoteFileConfig{File: file, Vars: vars, Out: out})
}

// Upload copies the local file or directory src to dst on the VM.
// progress, when set, is called with bytes copied so far and the total.
func (c *Conn) Upload(src, dst string, progress func(current, total int64)) error {
	info, err := os.Stat(src)
	if err != nil {
		return err
	}
	if !info.IsDir() {
		return c.r.SSHCopyFileWithProgress(src, dst, progress)
	}
	total, err := dirSize(src)
	if err != nil {
		return err
	}
	return c.r.SSHUploadDir(src, dst, total, progress)
}

// UploadSCP copies the local file src to dst over scp rather than sftp,
// for VMs without an sftp server.
func (c *Conn) UploadSCP(src, dst string, progress func(current, total int64)) error {
	return c.r.SCPCopyFileWithProgress(src, dst, progress)
}

// Download copies the file src on the VM to the local dst.
func (c *Conn) Download(src, dst string) error {
	return c.r.DownloadFile(src, dst)
}

// WaitForSSH waits until ssh to the VM answers, up to timeout.
func (c *Conn) WaitForSSH(timeout time.Duration) error {
	return c.r.WaitForSSH(timeout.String())
}

// WaitForCloudInit waits until cloud-init has finished on the VM, up to
// timeout.
func (c *Conn) WaitForCloudInit(timeout time.Duration) error {
	return c.r.WaitForCloudInit(timeout.String())
}

// Forward serves ln, forwarding each connection to remoteHost:remotePort
// as seen from the VM, until ln is closed. A dropped ssh connection is
// redialled on the next forwarded connection; onErr gets each forwarded
// connection's failure.
func (c *Conn) Forward(ln net.Listener, remoteHost string, remotePort int, onErr func(error)) error {
	return c.fwd.Serve(ln, tools.Forward{Local: ln.Addr().String(), RemoteHost: remoteHost, RemotePort: remotePort}, onErr)
}

func dirSize(path string) (int64, error) {
	var size int64
	err := filepath.Walk(path, func(_ string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.IsDir() {
			size += info.Size()
		}
		return nil
	})
	if err != nil {
		return 0, fmt.Errorf("sizing %s: %w", path, err)
	}
	return size, nil
}
