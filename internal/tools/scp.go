package tools

import (
	"log"
	"os"

	"github.com/pkg/sftp"
)

func (r *Remote) DownloadFile(srcPath, dstPath string) error {
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
			log.Printf("Failed to close SFTP client: %v", err)
		}
	}()

	// Open the source file
	srcFile, err := sftp.Open(srcPath)
	if err != nil {
		return err
	}
	defer func() {
		if err := srcFile.Close(); err != nil {
			log.Printf("Failed to close source file: %v", err)
		}
	}()

	// Create the destination file
	dstFile, err := os.Create(dstPath)
	if err != nil {
		return err
	}
	defer func() {
		if err := dstFile.Close(); err != nil {
			log.Printf("Failed to close destination file: %v", err)
		}
	}()

	// write to file
	if _, err := srcFile.WriteTo(dstFile); err != nil {
		return err
	}
	return nil
}

func (r *Remote) SSHCopyFile(srcPath, dstPath string) error {
	log.Println("[DEBUG] srcPath:" + srcPath)
	log.Println("[DEBUG] dstPath:" + dstPath)

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
			log.Printf("Failed to close SFTP client: %v", err)
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
			log.Printf("Failed to close source file: %v", err)
		}
	}()

	// Create the destination file
	dstFile, err := sftp.Create(dstPath)
	if err != nil {
		return err
	}
	defer func() {
		if err := dstFile.Close(); err != nil {
			log.Printf("Failed to close destination file: %v", err)
		}
	}()

	// write to file
	if _, err := dstFile.ReadFrom(srcFile); err != nil {
		return err
	}
	return nil
}
