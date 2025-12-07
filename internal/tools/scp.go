package tools

import (
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"

	"github.com/pkg/sftp"
)

func (r *Remote) DownloadFile(srcPath, dstPath string) error {
	// Create a new SSH connection
	err := r.NewSSHConnection()
	if err != nil {
		return err
	}

	// open an SFTP session over an existing ssh connection with optimized settings.
	// UseConcurrentReads enables concurrent read requests for better throughput
	// MaxConcurrentRequestsPerFile controls how many requests can be in-flight (default 64)
	sftpClient, err := sftp.NewClient(r.Client,
		sftp.UseConcurrentReads(true),
		sftp.MaxConcurrentRequestsPerFile(64),
	)
	if err != nil {
		return err
	}
	defer func() {
		if err := sftpClient.Close(); err != nil {
			log.Printf("Failed to close SFTP client: %v", err)
		}
	}()

	// Open the source file
	srcFile, err := sftpClient.Open(srcPath)
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
	return r.SSHCopyFileWithProgress(srcPath, dstPath, nil)
}

func (r *Remote) SSHCopyFileWithProgress(srcPath, dstPath string, progressCallback func(current, total int64)) error {
	// Get file size for progress reporting
	srcStat, err := os.Stat(srcPath)
	if err != nil {
		return err
	}
	fileSize := srcStat.Size()

	// Create a new SSH connection
	err = r.NewSSHConnection()
	if err != nil {
		return err
	}

	// open an SFTP session over an existing ssh connection with optimized settings.
	// UseConcurrentWrites enables concurrent write requests for better throughput
	// MaxConcurrentRequestsPerFile controls how many requests can be in-flight
	sftpClient, err := sftp.NewClient(r.Client,
		sftp.UseConcurrentWrites(true),
		sftp.MaxConcurrentRequestsPerFile(64),
	)
	if err != nil {
		return err
	}
	defer func() {
		if err := sftpClient.Close(); err != nil {
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
	dstFile, err := sftpClient.Create(dstPath)
	if err != nil {
		return err
	}
	defer func() {
		if err := dstFile.Close(); err != nil {
			log.Printf("Failed to close destination file: %v", err)
		}
	}()

	// Use ReadFrom for optimized transfer with concurrent writes
	// The SFTP client will handle buffering and concurrent requests efficiently
	var totalWritten int64
	if progressCallback != nil {
		// Copy with progress reporting
		// Use 32KB buffer to match SFTP default packet size for compatibility
		const bufferSize = 32 * 1024
		buffer := make([]byte, bufferSize)
		var lastProgressUpdate int64

		for {
			n, readErr := srcFile.Read(buffer)
			if n > 0 {
				_, writeErr := dstFile.Write(buffer[:n])
				if writeErr != nil {
					return writeErr
				}
				totalWritten += int64(n)

				// Report progress at reasonable intervals (every 1MB or when complete)
				if totalWritten-lastProgressUpdate >= 1024*1024 || totalWritten == fileSize {
					progressCallback(totalWritten, fileSize)
					lastProgressUpdate = totalWritten
				}
			}

			if readErr != nil {
				if readErr == io.EOF {
					// Final progress update
					progressCallback(totalWritten, fileSize)
					break
				}
				return readErr
			}
		}
	} else {
		// No progress callback - use optimized ReadFrom for maximum throughput
		// ReadFrom uses concurrent writes internally when UseConcurrentWrites is enabled
		totalWritten, err = dstFile.ReadFrom(srcFile)
		if err != nil {
			return err
		}
	}

	_ = totalWritten // Suppress unused variable warning when no progress callback
	return nil
}

func (r *Remote) SCPCopyFileWithProgress(srcPath, dstPath string, progressCallback func(current, total int64)) error {
	// Get file size for progress reporting
	srcStat, err := os.Stat(srcPath)
	if err != nil {
		return err
	}
	fileSize := srcStat.Size()

	// Create temporary file for private key
	tmpKeyFile, err := os.CreateTemp("", "onctl-scp-key-")
	if err != nil {
		return err
	}
	defer func() {
		if removeErr := os.Remove(tmpKeyFile.Name()); removeErr != nil {
			log.Printf("Warning: failed to remove temporary key file: %v", removeErr)
		}
	}()

	// Write private key to temp file
	if _, err := tmpKeyFile.WriteString(r.PrivateKey); err != nil {
		return err
	}
	if err := tmpKeyFile.Close(); err != nil {
		return err
	}

	// Set restrictive permissions on key file
	if err := os.Chmod(tmpKeyFile.Name(), 0600); err != nil {
		return err
	}

	// Use scp command for faster transfer
	scpCmd := exec.Command("scp",
		"-i", tmpKeyFile.Name(),
		"-o", "StrictHostKeyChecking=no",
		"-o", "UserKnownHostsFile=/dev/null",
		"-o", "LogLevel=ERROR",
		srcPath,
		fmt.Sprintf("%s@%s:%s", r.Username, r.IPAddress, dstPath))

	// Execute scp command for file transfer
	// Note: scp doesn't provide built-in progress reporting, so we report completion after transfer
	err = scpCmd.Run()
	if err != nil {
		return err
	}

	// Report completion
	if progressCallback != nil {
		progressCallback(fileSize, fileSize)
	}

	return nil
}
