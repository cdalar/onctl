package tools

import (
	"fmt"
	"log"
	"os"
	"os/exec"
	"strings"

	"github.com/pkg/sftp"
)

func (r *Remote) DownloadFile(srcPath, dstPath string) error {
	// If jumphost is specified, use system scp command with ProxyJump
	if r.JumpHost != "" {
		return r.downloadFileWithJumpHost(srcPath, dstPath)
	}

	// Create a new SSH connection
	err := r.NewSSHConnection()
	if err != nil {
		return err
	}

	// open an SFTP session over an existing ssh connection.
	sftp, err := sftp.NewClient(r.Client)
	if err != nil {
		return err
	}
	defer func() {
		if err := sftp.Close(); err != nil {
			// Only log non-EOF errors as EOF is expected when connection is already closed
			if err.Error() != "EOF" {
				log.Printf("Failed to close SFTP client: %v", err)
			}
		}
	}()

	// Open the source file
	srcFile, err := sftp.Open(srcPath)
	if err != nil {
		return err
	}
	defer func() {
		if err := srcFile.Close(); err != nil {
			// Only log non-EOF errors as EOF is expected when file is already closed
			if err.Error() != "EOF" {
				log.Printf("Failed to close source file: %v", err)
			}
		}
	}()

	// Create the destination file
	dstFile, err := os.Create(dstPath)
	if err != nil {
		return err
	}
	defer func() {
		if err := dstFile.Close(); err != nil {
			// Only log non-EOF errors as EOF is expected when file is already closed
			if err.Error() != "EOF" {
				log.Printf("Failed to close destination file: %v", err)
			}
		}
	}()

	// write to file
	if _, err := srcFile.WriteTo(dstFile); err != nil {
		return err
	}
	return nil
}

func (r *Remote) downloadFileWithJumpHost(srcPath, dstPath string) error {
	// Create a temporary file for the private key
	tempKeyFile, err := os.CreateTemp("", "onctl_ssh_key_*")
	if err != nil {
		return fmt.Errorf("failed to create temp key file: %v", err)
	}
	defer func() {
		if err := os.Remove(tempKeyFile.Name()); err != nil {
			log.Printf("Failed to remove temp key file: %v", err)
		}
	}()

	// Write the private key to the temp file
	if _, err := tempKeyFile.WriteString(r.PrivateKey); err != nil {
		return fmt.Errorf("failed to write private key to temp file: %v", err)
	}
	if err := tempKeyFile.Close(); err != nil {
		return fmt.Errorf("failed to close temp key file: %v", err)
	}

	// Use system scp command with ProxyJump
	scpArgs := []string{
		"-o", "UserKnownHostsFile=/dev/null",
		"-o", "StrictHostKeyChecking=no",
		"-i", tempKeyFile.Name(),
		"-P", fmt.Sprint(r.SSHPort),
	}

	// Add jumphost support using SSH's ProxyJump option
	if r.JumpHost != "" {
		// Format jumphost as user@host if user is not already specified
		jumpHostSpec := r.JumpHost
		if !strings.Contains(jumpHostSpec, "@") {
			jumpHostSpec = r.Username + "@" + jumpHostSpec
		}
		scpArgs = append(scpArgs, "-J", jumpHostSpec)
	}

	// Add the source and destination
	scpArgs = append(scpArgs, fmt.Sprintf("%s@%s:%s", r.Username, r.IPAddress, srcPath), dstPath)

	log.Printf("[DEBUG] scp download args: %v", scpArgs)

	scpCommand := exec.Command("scp", scpArgs...)
	scpCommand.Stdout = os.Stdout
	scpCommand.Stderr = os.Stderr

	return scpCommand.Run()
}

func (r *Remote) SSHCopyFile(srcPath, dstPath string) error {
	log.Println("[DEBUG] srcPath:" + srcPath)
	log.Println("[DEBUG] dstPath:" + dstPath)

	// If jumphost is specified, use system scp command with ProxyJump
	if r.JumpHost != "" {
		return r.uploadFileWithJumpHost(srcPath, dstPath)
	}

	// Create a new SSH connection
	err := r.NewSSHConnection()
	if err != nil {
		return err
	}

	// open an SFTP session over an existing ssh connection.
	sftp, err := sftp.NewClient(r.Client)
	if err != nil {
		return err
	}
	defer func() {
		if err := sftp.Close(); err != nil {
			// Only log non-EOF errors as EOF is expected when connection is already closed
			if err.Error() != "EOF" {
				log.Printf("Failed to close SFTP client: %v", err)
			}
		}
	}()

	// Open the source file
	srcFile, err := os.Open(srcPath)
	if err != nil {
		log.Println(err)
		return err
	}
	defer func() {
		if err := srcFile.Close(); err != nil {
			// Only log non-EOF errors as EOF is expected when file is already closed
			if err.Error() != "EOF" {
				log.Printf("Failed to close source file: %v", err)
			}
		}
	}()

	// Create the destination file
	dstFile, err := sftp.Create(dstPath)
	if err != nil {
		return err
	}
	defer func() {
		if err := dstFile.Close(); err != nil {
			// Only log non-EOF errors as EOF is expected when file is already closed
			if err.Error() != "EOF" {
				log.Printf("Failed to close destination file: %v", err)
			}
		}
	}()

	// write to file
	if _, err := dstFile.ReadFrom(srcFile); err != nil {
		return err
	}
	return nil
}

func (r *Remote) uploadFileWithJumpHost(srcPath, dstPath string) error {
	// Create a temporary file for the private key
	tempKeyFile, err := os.CreateTemp("", "onctl_ssh_key_*")
	if err != nil {
		return fmt.Errorf("failed to create temp key file: %v", err)
	}
	defer func() {
		if err := os.Remove(tempKeyFile.Name()); err != nil {
			log.Printf("Failed to remove temp key file: %v", err)
		}
	}()

	// Write the private key to the temp file
	if _, err := tempKeyFile.WriteString(r.PrivateKey); err != nil {
		return fmt.Errorf("failed to write private key to temp file: %v", err)
	}
	if err := tempKeyFile.Close(); err != nil {
		return fmt.Errorf("failed to close temp key file: %v", err)
	}

	// Use system scp command with ProxyJump
	scpArgs := []string{
		"-o", "UserKnownHostsFile=/dev/null",
		"-o", "StrictHostKeyChecking=no",
		"-i", tempKeyFile.Name(),
		"-P", fmt.Sprint(r.SSHPort),
	}

	// Add jumphost support using SSH's ProxyJump option
	if r.JumpHost != "" {
		// Format jumphost as user@host if user is not already specified
		jumpHostSpec := r.JumpHost
		if !strings.Contains(jumpHostSpec, "@") {
			jumpHostSpec = r.Username + "@" + jumpHostSpec
		}
		scpArgs = append(scpArgs, "-J", jumpHostSpec)
	}

	// Add the source and destination
	scpArgs = append(scpArgs, srcPath, fmt.Sprintf("%s@%s:%s", r.Username, r.IPAddress, dstPath))

	log.Printf("[DEBUG] scp upload args: %v", scpArgs)

	scpCommand := exec.Command("scp", scpArgs...)
	scpCommand.Stdout = os.Stdout
	scpCommand.Stderr = os.Stderr

	return scpCommand.Run()
}
