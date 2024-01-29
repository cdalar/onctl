package tools

import (
	"log"
	"os"

	"github.com/pkg/sftp"
)

func (r *Remote) SSHCopyFile(srcPath, dstPath string) error {
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
	log.Println("[DEBUG] srcPath:" + srcPath)
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
