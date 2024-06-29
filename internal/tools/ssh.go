package tools

import (
	"fmt"
	"log"
	"os"
	"os/exec"
	"syscall"
)

type SSHIntoVMRequest struct {
	IPAddress      string
	User           string
	Port           int
	PrivateKeyFile string
}

func SSHIntoVM(request SSHIntoVMRequest) {
	sshArgs := []string{
		"-o", "UserKnownHostsFile=/dev/null",
		"-o", "StrictHostKeyChecking=no",
		"-i", request.PrivateKeyFile,
		"-l", request.User,
		request.IPAddress,
		"-p", fmt.Sprint(request.Port),
	}
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
