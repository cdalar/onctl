// cmd/run.go
package cmd

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/spf13/cobra"
)

// No default owner - user must always specify the full username/repo format

// GitHubRelease represents the structure of a GitHub release API response
type GitHubRelease struct {
	TagName string `json:"tag_name"`
}

var runCmd = &cobra.Command{
	Use:   "run [username/repo[:version]]",
	Short: "Run an onctl action",
	Long: `Run an onctl action by downloading the latest release binary that fits the system architecture.
You must specify the action in the format username/repo, and you can optionally specify a version using username/repo:version.
Example: onctl run username/repo:v1.1.1`,
	Run: func(cmd *cobra.Command, args []string) {
		if len(args) == 0 {
			fmt.Println("Please provide an action name")
			return
		}

		actionName := args[0]
		arch := getSystemArchitecture()
		operatingSystem := getSystemOS()

		// Validate and parse the repository path
		if actionName == "" {
			fmt.Println("Action name cannot be empty")
			return
		}

		// Parse the repository path, name, and version tag
		repoPath, repoName, versionTag := parseRepoPath(actionName)

		// Determine which tag to use
		var releaseTag string
		if versionTag != "" {
			// Use the specified version tag
			releaseTag = versionTag
			log.Printf("[DEBUG] Using specified version tag: %s", releaseTag)
		} else {
			// Get the latest release tag
			var err error
			releaseTag, err = getLatestReleaseTag(repoPath)
			if err != nil {
				log.Fatalf("Failed to get latest release tag: %v", err)
			}
			log.Printf("[DEBUG] Using latest release tag: %s", releaseTag)
		}

		// Format the binary name with hyphens as shown in the example URL
		binaryName := fmt.Sprintf("%s-%s-%s", repoName, operatingSystem, arch)
		downloadURL := fmt.Sprintf("https://github.com/%s/releases/download/%s/%s", repoPath, releaseTag, binaryName)

		// Create ~/.onctl/actions directory if it doesn't exist
		homeDir, err := os.UserHomeDir()
		if err != nil {
			log.Fatalf("Failed to get user home directory: %v", err)
		}

		actionsDir := filepath.Join(homeDir, ".onctl", "actions")
		err = os.MkdirAll(actionsDir, 0755)
		if err != nil {
			log.Fatalf("Failed to create actions directory: %v", err)
		}

		binaryPath := filepath.Join(actionsDir, binaryName)
		err = downloadBinary(downloadURL, binaryPath)
		if err != nil {
			log.Fatal(err)
		}

		// Make the binary executable
		err = os.Chmod(binaryPath, 0755)
		if err != nil {
			log.Fatalf("Failed to make binary executable: %v", err)
		}

		// Create the command - try both methods of passing arguments
		if len(args) > 1 {
			// Method 1: Pass arguments as command-line arguments
			execCmd := exec.Command(binaryPath, args[1:]...)
			execCmd.Stdout = os.Stdout
			execCmd.Stderr = os.Stderr

			log.Printf("[DEBUG] Running %s with command-line arguments: %v", binaryPath, args[1:])
			err = execCmd.Run()

			// If command-line arguments failed, try stdin method
			if err != nil {
				log.Printf("[DEBUG] Command-line arguments failed with: %v. Trying stdin method...", err)

				// Method 2: Pass arguments via stdin
				execCmd = exec.Command(binaryPath)
				execCmd.Stdout = os.Stdout
				execCmd.Stderr = os.Stderr

				// Join all additional arguments into a single string
				inputData := strings.Join(args[1:], " ")
				log.Printf("[DEBUG] Running %s with stdin input: %s", binaryPath, inputData)

				// Create a pipe to the command's stdin
				stdin, pipeErr := execCmd.StdinPipe()
				if pipeErr != nil {
					log.Fatalf("Failed to create stdin pipe: %v", pipeErr)
				}

				// Start the command
				startErr := execCmd.Start()
				if startErr != nil {
					log.Fatalf("Failed to start command: %v", startErr)
				}

				// Write the input data to stdin
				_, writeErr := io.WriteString(stdin, inputData)
				if writeErr != nil {
					log.Fatalf("Failed to write to stdin: %v", writeErr)
				}

				// Close stdin to signal EOF
				closeErr := stdin.Close()
				if closeErr != nil {
					log.Fatalf("Failed to close stdin: %v", closeErr)
				}

				// Wait for the command to complete
				waitErr := execCmd.Wait()
				if waitErr != nil {
					log.Fatalf("Command failed: %v", waitErr)
				}
			}
		} else {
			// If there are no additional arguments, just run the executable
			execCmd := exec.Command(binaryPath)
			execCmd.Stdout = os.Stdout
			execCmd.Stderr = os.Stderr

			log.Printf("[DEBUG] Running %s with no arguments", binaryPath)
			err = execCmd.Run()
			if err != nil {
				log.Fatal(err)
			}
		}
	},
}

func getSystemArchitecture() string {
	arch := runtime.GOARCH
	switch arch {
	case "amd64":
		return "amd64"
	case "arm64":
		return "arm64"
	default:
		log.Fatal("Unsupported architecture:", arch)
		return ""
	}
}

func getSystemOS() string {
	os := runtime.GOOS
	switch os {
	case "darwin":
		return "darwin"
	case "linux":
		return "linux"
	case "windows":
		return "windows"
	default:
		log.Fatal("Unsupported OS:", os)
		return ""
	}
}

func downloadBinary(url, path string) error {
	resp, err := http.Get(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	log.Println("[DEBUG] Downloading binary from", url)
	// Check if the response status is OK
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("failed to download binary: %s", resp.Status)
	}

	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()

	_, err = io.Copy(f, resp.Body)
	if err != nil {
		return err
	}

	return nil
}

// getLatestReleaseTag fetches the latest release tag from GitHub
func getLatestReleaseTag(repoPath string) (string, error) {
	url := fmt.Sprintf("https://api.github.com/repos/%s/releases/latest", repoPath)

	// Create a new request
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return "", err
	}

	// Add a user agent to avoid GitHub API limitations
	req.Header.Set("User-Agent", "onctl-client")

	// Make the request
	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	// Check if the response status is OK
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("failed to fetch latest release: %s", resp.Status)
	}

	// Parse the response
	var release GitHubRelease
	if err := json.NewDecoder(resp.Body).Decode(&release); err != nil {
		return "", err
	}

	if release.TagName == "" {
		return "", fmt.Errorf("no tag found in the latest release")
	}

	return release.TagName, nil
}

// parseRepoPath extracts the repository path, name, and version tag from an action name
// If the action name doesn't contain a slash, it assumes the default owner
// If the action name contains a colon, it extracts the version tag
// Parses the repository path in the format username/repo[:version]
// Returns: repoPath (without version), repoName, and versionTag (empty if not specified)
func parseRepoPath(actionName string) (repoPath, repoName, versionTag string) {
	// Check if a version tag is specified (e.g., username/repo:v1.1.1)
	actionParts := strings.Split(actionName, ":")
	actionNameWithoutVersion := actionParts[0]

	// Extract version tag if provided
	if len(actionParts) > 1 {
		versionTag = actionParts[1]
	}

	// Ensure the action name contains a username/repo format
	if !strings.Contains(actionNameWithoutVersion, "/") {
		log.Fatalf("Invalid action format: %s. Must be in the format username/repo[:version]", actionName)
	}

	// Extract the repo name from the path
	parts := strings.Split(actionNameWithoutVersion, "/")
	repoPath = actionNameWithoutVersion
	if len(parts) >= 2 {
		repoName = parts[1]
	} else {
		// This should not happen due to the check above, but just in case
		log.Fatalf("Invalid repository path: %s. Must be in the format username/repo[:version]", actionName)
	}

	return repoPath, repoName, versionTag
}
