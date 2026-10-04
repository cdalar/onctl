package cmd

import (
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"

	"github.com/cdalar/onctl/pkg/cloud"
	"github.com/spf13/cobra"
)

var sshProxyCmd = &cobra.Command{
	Use:   "ssh-proxy <vm> [port]",
	Short: "Relay stdin/stdout to a VM's port through the provider (for ssh's ProxyCommand)",
	Long: `Connects stdin and stdout to a port inside the VM -- 22 unless another is
given -- through the provider, for providers whose VMs have no address of
their own (boxes). It's what onctl ssh uses as ssh's ProxyCommand, and it
lets anything that speaks ssh reach a box:

  ssh -o ProxyCommand='onctl ssh-proxy -p boxes %h' root@my-box

or once, in ~/.ssh/config, then plain ssh, scp, rsync, git or VS Code
Remote-SSH:

  Host *.box
    ProxyCommand onctl ssh-proxy -p boxes %n
    User root

A paused box is resumed first. The box needs your public key: onctl
create and onctl ssh add it.`,
	Args: cobra.RangeArgs(1, 2),
	RunE: func(cmd *cobra.Command, args []string) error {
		d, ok := provider.(cloud.Dialer)
		if !ok {
			return fmt.Errorf("ssh-proxy is for providers whose VMs aren't reachable directly (boxes); %s VMs take ssh at their IP", cloudProvider)
		}
		name := strings.TrimSuffix(args[0], ".box")
		port := 22
		if len(args) == 2 {
			p, err := strconv.Atoi(args[1])
			if err != nil || p < 1 || p > 65535 {
				return fmt.Errorf("invalid port %q", args[1])
			}
			port = p
		}
		conn, err := d.DialVM(cmd.Context(), cloud.Vm{Name: name}, port)
		if err != nil {
			return err
		}
		defer func() { _ = conn.Close() }()
		return relayStdio(conn, os.Stdin, os.Stdout)
	},
}

// relayStdio copies in to conn and conn to out until the VM side ends
// the stream. in reaching EOF doesn't end it: the VM may still be
// sending.
func relayStdio(conn io.ReadWriter, in io.Reader, out io.Writer) error {
	go func() { _, _ = io.Copy(conn, in) }()
	_, err := io.Copy(out, conn)
	return err
}

func init() {
	rootCmd.AddCommand(sshProxyCmd)
}
