package cmd

import (
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var actionParamsFile string

var actionCmd = &cobra.Command{
	Use:   "action <name>",
	Short: "Execute a custom action from GitHub",
	Long:  `Download and execute custom actions from GitHub repositories. Actions are automatically downloaded from cdalar/onctl-action-<name> repositories.`,
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		actionName := args[0]
		log.Printf("[DEBUG] Executing action: %s", actionName)

		// Get home directory
		home, err := os.UserHomeDir()
		if err != nil {
			log.Fatalf("Failed to get home directory: %v", err)
		}

		actionsDir := filepath.Join(home, ".onctl", "actions")
		if err := os.MkdirAll(actionsDir, 0755); err != nil {
			log.Fatalf("Failed to create actions directory: %v", err)
		}

		// Determine platform-specific binary name
		var arch string
		switch runtime.GOARCH {
		case "amd64":
			arch = "amd64"
		case "arm64":
			arch = "arm64"
		default:
			log.Fatalf("Unsupported architecture: %s", runtime.GOARCH)
		}

		var osName string
		switch runtime.GOOS {
		case "darwin":
			osName = "darwin"
		case "linux":
			osName = "linux"
		case "windows":
			osName = "windows"
		default:
			log.Fatalf("Unsupported OS: %s", runtime.GOOS)
		}

		repoName := fmt.Sprintf("onctl-action-%s", actionName)
		binaryName := fmt.Sprintf("%s-%s-%s", repoName, osName, arch)
		binaryPath := filepath.Join(actionsDir, binaryName)

		// Check if binary already exists
		if _, err := os.Stat(binaryPath); os.IsNotExist(err) {
			log.Printf("[DEBUG] Downloading action binary: %s", binaryName)

			// Try to read config to get GitHub owner, default to "cdalar"
			githubOwner := "cdalar"
			if viper.IsSet("actions.githubOwner") {
				githubOwner = viper.GetString("actions.githubOwner")
			}

			// Download from GitHub releases
			downloadURL := fmt.Sprintf("https://github.com/%s/%s/releases/latest/download/%s", githubOwner, repoName, binaryName)
			if err := downloadFile(downloadURL, binaryPath); err != nil {
				log.Fatalf("Failed to download action binary: %v", err)
			}

			// Make executable on Unix-like systems
			if runtime.GOOS != "windows" {
				if err := os.Chmod(binaryPath, 0755); err != nil {
					log.Fatalf("Failed to make binary executable: %v", err)
				}
			}

			log.Printf("[DEBUG] Successfully downloaded: %s", binaryName)
		} else {
			log.Printf("[DEBUG] Action binary already exists: %s", binaryName)
		}

		// Execute the binary with remaining arguments
		command := exec.Command(binaryPath, args[1:]...)
		command.Stdout = os.Stdout
		command.Stderr = os.Stderr

		// Set up stdin based on parameter file flag
		if actionParamsFile != "" {
			log.Printf("[DEBUG] Using parameter file: %s", actionParamsFile)
			// Check if file exists
			paramsFile, err := os.Open(actionParamsFile)
			if err != nil {
				log.Fatalf("Failed to open parameter file %s: %v", actionParamsFile, err)
			}
			defer paramsFile.Close()
			command.Stdin = paramsFile
		} else {
			command.Stdin = os.Stdin
		}

		log.Printf("[DEBUG] Running: %s", binaryPath)
		if err := command.Run(); err != nil {
			log.Fatalf("Failed to execute action %s: %v", actionName, err)
		}
	},
}

func downloadFile(url, filepath string) error {
	resp, err := http.Get(url)
	if err != nil {
		return fmt.Errorf("failed to download: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("download failed with status: %s", resp.Status)
	}

	out, err := os.Create(filepath)
	if err != nil {
		return fmt.Errorf("failed to create file: %v", err)
	}
	defer out.Close()

	_, err = io.Copy(out, resp.Body)
	if err != nil {
		return fmt.Errorf("failed to write file: %v", err)
	}

	return nil
}

func init() {
	actionCmd.Flags().StringVarP(&actionParamsFile, "params", "p", "", "JSON parameter file to pass as stdin")
	rootCmd.AddCommand(actionCmd)
}
