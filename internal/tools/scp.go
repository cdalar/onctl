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
	defer sftp.Close()

	// Open the source file
	srcFile, err := sftp.Open(srcPath)
	if err != nil {
		return err
	}
	defer srcFile.Close()

	// Create the destination file
	dstFile, err := os.Create(dstPath)
	if err != nil {
		return err
	}
	defer dstFile.Close()

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
	defer sftp.Close()

	// Open the source file
	srcFile, err := os.Open(srcPath)
	if err != nil {
		log.Println(err)
		return err
	}
	defer srcFile.Close()

	// Create the destination file
	dstFile, err := sftp.Create(dstPath)
	if err != nil {
		return err
	}
	defer dstFile.Close()

	// write to file
	if _, err := dstFile.ReadFrom(srcFile); err != nil {
		return err
	}
	return nil
}
