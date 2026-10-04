package cmd

import (
	"fmt"
	"log"
	"net"
	"os"
	"os/signal"
	"syscall"

	"github.com/cdalar/onctl/internal/tools"
	"github.com/cdalar/onctl/pkg/cloud"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var (
	pfKey     string
	pfSSHPort int
	pfAddress string
)

var portForwardCmd = &cobra.Command{
	Use:     "port-forward VM_NAME [LOCAL:]REMOTE...",
	Aliases: []string{"pf"},
	Short:   "Forward local ports to ports on a VM, over ssh",
	Long: `Forwards each local port to a port on the VM, over the same ssh
connection onctl ssh uses -- like ssh -L, for any provider. Runs until
interrupted (Ctrl-C).

Each forward is one of:

  REMOTE             the VM's port, on the same port locally
  LOCAL:REMOTE       the VM's port REMOTE on local port LOCAL (0: any free port)
  LOCAL:HOST:REMOTE  HOST:REMOTE as reached from the VM (another machine on
                     its network, say)

Local ports listen on 127.0.0.1 unless --address says otherwise.`,
	Example: `  # A dev server on the VM's port 3000, at http://localhost:3000
  onctl port-forward my-vm 3000

  # Postgres on local 15432, and the VM's 8080 on any free port
  onctl pf my-vm 15432:5432 0:8080

  # Through the VM, to a database only it can reach
  onctl pf my-vm 5432:db.internal:5432`,
	Args:              cobra.MinimumNArgs(2),
	ValidArgsFunction: completeVMName,
	RunE: func(cmd *cobra.Command, args []string) error {
		var forwards []tools.Forward
		for _, spec := range args[1:] {
			fw, err := tools.ParseForward(spec, pfAddress)
			if err != nil {
				return err
			}
			forwards = append(forwards, fw)
		}

		vm, err := provider.GetByName(args[0])
		if err != nil {
			return err
		}
		remote, err := remoteForVM(vm, pfKey, pfSSHPort, cmd.Flags().Changed("ssh-port"))
		if err != nil {
			return err
		}
		f := &tools.Forwarder{Remote: &remote}
		// Connect now, so a wrong key or an unreachable VM fails here
		// rather than on the first forwarded connection.
		if err := remote.NewSSHConnection(); err != nil {
			return fmt.Errorf("connecting to %s: %w", vm.Name, err)
		}

		var listeners []net.Listener
		defer func() {
			for _, ln := range listeners {
				_ = ln.Close()
			}
		}()
		for _, fw := range forwards {
			ln, err := net.Listen("tcp", fw.Local)
			if err != nil {
				return fmt.Errorf("listening on %s: %w", fw.Local, err)
			}
			listeners = append(listeners, ln)
			fmt.Printf("Forwarding %s -> %s:%d on %s\n", ln.Addr(), fw.RemoteHost, fw.RemotePort, vm.Name)
			go func(ln net.Listener, fw tools.Forward) {
				if err := f.Serve(ln, fw, func(err error) {
					fmt.Fprintln(os.Stderr, "forward:", err)
				}); err != nil {
					log.Println("[ERROR] forward:", err)
				}
			}(ln, fw)
		}

		stop := make(chan os.Signal, 1)
		signal.Notify(stop, os.Interrupt, syscall.SIGTERM)
		<-stop
		return nil
	},
}

// remoteForVM builds the ssh connection settings onctl uses for vm:
// the provider's username, key (keyFlag, or the default), and port --
// an imported (static) host's own from the inventory -- through the
// provider's tunnel when it has one. A provider that can put the key on
// the VM itself (boxes) is asked to first.
func remoteForVM(vm cloud.Vm, keyFlag string, port int, portChanged bool) (tools.Remote, error) {
	publicKeyFile, privateKeyFile := getSSHKeyFilePaths(keyFlag)
	remote := tools.Remote{
		Username:  viper.GetString(cloudProvider + ".vm.username"),
		IPAddress: vm.IP,
		SSHPort:   port,
	}
	if cloudProvider == "static" {
		sp, err := staticProvider()
		if err != nil {
			return remote, err
		}
		h, err := sp.GetHost(vm.Name)
		if err != nil {
			return remote, err
		}
		remote.Username = h.Username
		if !portChanged {
			remote.SSHPort = h.SSHPort
		}
		if keyFlag == "" && h.PrivateKey != "" {
			publicKeyFile, privateKeyFile = getSSHKeyFilePaths(h.PrivateKey)
		}
	}
	if remote.Username == "" {
		remote.Username = "root"
	}
	privateKey, err := os.ReadFile(privateKeyFile)
	if err != nil {
		return remote, err
	}
	remote.PrivateKey = string(privateKey)
	if ka, ok := provider.(cloud.KeyAuthorizer); ok {
		pub, err := os.ReadFile(publicKeyFile)
		if err != nil {
			return remote, err
		}
		if err := ka.AuthorizeKey(vm, string(pub)); err != nil {
			return remote, err
		}
	}
	attachDialer(&remote, vm)
	return remote, nil
}

func init() {
	portForwardCmd.Flags().StringVarP(&pfKey, "key", "k", "", "Path to the private key (default: ~/.ssh/id_rsa)")
	portForwardCmd.Flags().IntVarP(&pfSSHPort, "ssh-port", "P", 22, "ssh port on the VM")
	portForwardCmd.Flags().StringVar(&pfAddress, "address", "127.0.0.1", "local address to listen on (0.0.0.0 shares the forwards with your network)")
	rootCmd.AddCommand(portForwardCmd)
}
