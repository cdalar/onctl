package tools

import (
	"fmt"
	"log"
	"os"
	"os/exec"
	"strings"
	"syscall"
)

type SSHIntoVMRequest struct {
	IPAddress      string
	User           string
	Port           int
	PrivateKeyFile string
	JumpHost       string
}

func SSHIntoVM(request SSHIntoVMRequest) {
	sshArgs := []string{
		"-o", "UserKnownHostsFile=/dev/null",
		"-o", "StrictHostKeyChecking=no",
		"-i", request.PrivateKeyFile,
		"-l", request.User,
		"-p", fmt.Sprint(request.Port),
	}

	// Add jumphost support using SSH's ProxyJump option
	if request.JumpHost != "" {
		// Format jumphost as user@host if user is not already specified
		jumpHostSpec := request.JumpHost
		if !strings.Contains(jumpHostSpec, "@") {
			jumpHostSpec = request.User + "@" + jumpHostSpec
		}
		sshArgs = append(sshArgs, "-J", jumpHostSpec)
	}

	// Add the target IP address
	sshArgs = append(sshArgs, request.IPAddress)

	log.Println("[DEBUG] sshArgs: ", sshArgs)
	// sshCommand := exec.Command("ssh", append(sshArgs, args[1:]...)...)
	sshCommand := exec.Command("ssh", sshArgs...)
	sshCommand.Stdin = os.Stdin
	sshCommand.Stdout = os.Stdout
	sshCommand.Stderr = os.Stderr

	if err := sshCommand.Run(); err != nil {
		if exitError, ok := err.(*exec.ExitError); ok {
			waitStatus := exitError.Sys().(syscall.WaitStatus)
			os.Exit(waitStatus.ExitStatus())
		} else {
			log.Panic(err)
		}
	}
}
