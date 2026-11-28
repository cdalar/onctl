package tools

import (
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"path/filepath"
)

// OnctlSSHConfigPath is the fixed location of onctl's own ssh_config-format
// inventory of imported hosts (the static provider's), ~/.ssh/onctl_config.
func OnctlSSHConfigPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("failed to get home directory: %w", err)
	}
	return filepath.Join(home, ".ssh", "onctl_config"), nil
}

type SSHIntoVMRequest struct {
	IPAddress      string
	User           string
	Port           int
	PrivateKeyFile string
	Command        []string
	// ProxyCommand, when set, is how ssh reaches the VM (for VMs with no
	// address of their own, like boxes); IPAddress is then just a name.
	ProxyCommand string
	// Stdin, Stdout and Stderr are the session's streams; nil means this
	// process's own.
	Stdin          io.Reader
	Stdout, Stderr io.Writer
}

// SSHIntoVM runs ssh to the VM. A remote command that exits non-zero comes
// back as an *exec.ExitError carrying its exit code.
func SSHIntoVM(request SSHIntoVMRequest) error {
	sshArgs := []string{
		"-o", "UserKnownHostsFile=/dev/null",
		"-o", "StrictHostKeyChecking=no",
		"-i", request.PrivateKeyFile,
	}
	if request.ProxyCommand != "" {
		sshArgs = append(sshArgs, "-o", "ProxyCommand="+request.ProxyCommand)
	}
	sshArgs = append(sshArgs,
		"-l", request.User,
		request.IPAddress,
		"-p", fmt.Sprint(request.Port),
	)
	if len(request.Command) > 0 {
		sshArgs = append(sshArgs, "--")
		sshArgs = append(sshArgs, request.Command...)
	}
	log.Println("[DEBUG] sshArgs: ", sshArgs)
	sshCommand := exec.Command("ssh", sshArgs...)
	sshCommand.Stdin, sshCommand.Stdout, sshCommand.Stderr = os.Stdin, os.Stdout, os.Stderr
	if request.Stdin != nil {
		sshCommand.Stdin = request.Stdin
	}
	if request.Stdout != nil {
		sshCommand.Stdout = request.Stdout
	}
	if request.Stderr != nil {
		sshCommand.Stderr = request.Stderr
	}
	// The VM has terminfo for the common terminals, not for every one
	// (xterm-ghostty, xterm-kitty), and tmux won't start without it.
	sshCommand.Env = append(os.Environ(), "TERM=xterm-256color")

	return sshCommand.Run()
}
