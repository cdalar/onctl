package cmd

import (
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/briandowns/spinner"
	"github.com/cdalar/onctl/internal/tools"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

type cmdDeployOptions struct {
	Image          string   `yaml:"image"`
	Env            []string `yaml:"env"`
	TransferMethod string   `yaml:"transferMethod"`
	Diagnostics    bool     `yaml:"diagnostics"`
	Name           string   `yaml:"name"`
}

var deployOpt cmdDeployOptions

// normalizeArch normalizes architecture names to a common format
// Maps Docker architecture names and uname -m output to consistent values
func normalizeArch(arch string) string {
	arch = strings.ToLower(strings.TrimSpace(arch))
	switch arch {
	case "x86_64", "amd64":
		return "amd64"
	case "aarch64", "arm64":
		return "arm64"
	case "armv7l", "arm":
		return "arm"
	case "i386", "i686", "386":
		return "386"
	default:
		return arch
	}
}

// runContainer runs a Docker container on the remote VM
func runContainer(remote tools.Remote, s *spinner.Spinner, image string, env []string, name string) {
	// Step 3: Run Docker container on remote VM
	s.Restart()
	s.Suffix = " Starting Docker container on remote VM..."

	// Prepare environment variables
	envVars := ""
	if len(env) > 0 {
		envVars = "-e " + strings.Join(env, " -e ")
	}

	// Prepare container name
	containerName := ""
	if name != "" {
		containerName = name
		// Stop and remove existing container with the same name if it exists
		stopCmd := fmt.Sprintf("docker stop %s 2>/dev/null || true", containerName)
		_, _ = remote.RemoteRun(&tools.RemoteRunConfig{
			Command: stopCmd,
		})
		removeCmd := fmt.Sprintf("docker rm %s 2>/dev/null || true", containerName)
		_, _ = remote.RemoteRun(&tools.RemoteRunConfig{
			Command: removeCmd,
		})
	}

	// Prepare name flag
	nameFlag := ""
	if containerName != "" {
		nameFlag = fmt.Sprintf("--name %s", containerName)
	}

	// Run the container
	runCmd := fmt.Sprintf("docker run -d %s %s %s", nameFlag, envVars, image)
	output, err := remote.RemoteRun(&tools.RemoteRunConfig{
		Command: runCmd,
	})
	if err != nil {
		s.Stop()
		fmt.Print("\033[?25h") // Ensure cursor is visible on error
		log.Fatalf("Failed to run Docker container: %v", err)
	}

	containerID := strings.TrimSpace(output)
	s.Suffix = " Docker container started on remote VM"
	s.Stop()

	// Check if container is actually running
	checkCmd := fmt.Sprintf("docker ps --filter id=%s --format '{{.Status}}'", containerID)
	statusOutput, statusErr := remote.RemoteRun(&tools.RemoteRunConfig{
		Command: checkCmd,
	})

	if statusErr != nil || strings.TrimSpace(statusOutput) == "" {
		// Container failed to start, get logs
		logsCmd := fmt.Sprintf("docker logs %s", containerID)
		logsOutput, logsErr := remote.RemoteRun(&tools.RemoteRunConfig{
			Command: logsCmd,
		})

		fmt.Printf("\033[31m\u2717\033[0m Docker container failed to start (ID: %s)\033[?25h\n", containerID)
		if logsErr == nil && logsOutput != "" {
			fmt.Println("Container logs:")
			fmt.Println(logsOutput)
		} else {
			fmt.Println("Unable to retrieve container logs")
		}
		log.Fatalf("Container deployment failed")
	}

	fmt.Printf("\033[32m\u2714\033[0m Docker container started successfully (ID: %s)\033[?25h\n", containerID)

	// Step 4: Clean up uploaded tar file (only if it exists)
	s.Restart()
	s.Suffix = " Cleaning up temporary files..."

	cleanupCmd := "rm image.tar.gz 2>/dev/null || true"
	_, err = remote.RemoteRun(&tools.RemoteRunConfig{
		Command: cleanupCmd,
	})
	if err != nil {
		log.Printf("Warning: Failed to clean up remote tar file: %v", err)
	}

	s.Suffix = " Cleanup completed"
	s.Stop()
	fmt.Println("\033[32m\u2714\033[0m Deployment completed successfully\033[?25h")
}

func init() {
	deployCmd.Flags().StringVarP(&deployOpt.Image, "image", "i", "", "Docker image to deploy (required)")
	deployCmd.Flags().StringSliceVarP(&deployOpt.Env, "env", "e", []string{}, "Environment variables for the container")
	deployCmd.Flags().StringVar(&deployOpt.TransferMethod, "transfer-method", "scp", "Transfer method to use (sftp or scp)")
	deployCmd.Flags().BoolVar(&deployOpt.Diagnostics, "diagnostics", false, "Run diagnostics to check for potential speed limitations")
	deployCmd.Flags().StringVarP(&deployOpt.Name, "name", "n", "", "Name for the Docker container")
	deployCmd.MarkFlagRequired("image")
	rootCmd.AddCommand(deployCmd)
}

var deployCmd = &cobra.Command{
	Use:   "deploy VM_NAME",
	Short: "Deploy a Docker image to a remote VM",
	Long: `Deploy a Docker image to a remote VM by saving it locally with gzip compression, uploading it via SSH, and running it on the remote host.

Note: Ensure the Docker image architecture matches the remote VM's architecture to avoid exec format errors.`,
	Args:                  cobra.MinimumNArgs(1),
	TraverseChildren:      true,
	DisableFlagsInUseLine: true,
	ValidArgsFunction: func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
		VMList, err := provider.List()
		list := []string{}
		for _, vm := range VMList.List {
			list = append(list, vm.Name)
		}

		if err != nil {
			return nil, cobra.ShellCompDirectiveError
		}

		return list, cobra.ShellCompDirectiveNoFileComp
	},

	Run: func(cmd *cobra.Command, args []string) {
		if len(args) == 0 {
			fmt.Println("Please provide a VM name")
			return
		}

		vmName := args[0]

		// Get VM details
		vm, err := provider.GetByName(vmName)
		if err != nil {
			log.Fatalln(err)
		}

		// Setup spinner
		s := spinner.New(spinner.CharSets[9], 100*time.Millisecond)

		// Get SSH key
		_, privateKeyFile := getSSHKeyFilePaths("")
		privateKey, err := os.ReadFile(privateKeyFile)
		if err != nil {
			log.Fatal(err)
		}

		remote := tools.Remote{
			Username:   viper.GetString(cloudProvider + ".vm.username"),
			IPAddress:  vm.IP,
			SSHPort:    22, // Default SSH port
			PrivateKey: string(privateKey),
			Spinner:    s,
		}

		// Run diagnostics if requested
		if deployOpt.Diagnostics {
			fmt.Println("🔍 Running diagnostics to check for potential speed limitations...")
			// TODO: Implement runDiagnostics function
			fmt.Println("Diagnostics not yet implemented")
		}

		// Check architecture compatibility
		s.Suffix = " Checking architecture compatibility..."
		s.Start()

		// Get local image architecture
		localArchCmd := exec.Command("docker", "image", "inspect", deployOpt.Image, "--format", "{{.Architecture}}")
		localArchOutput, err := localArchCmd.Output()
		if err != nil {
			s.Stop()
			fmt.Print("\033[?25h")
			log.Fatalf("Failed to inspect local Docker image: %v", err)
		}
		localArch := strings.TrimSpace(string(localArchOutput))

		// Get remote VM architecture
		remoteArchOutput, err := remote.RemoteRun(&tools.RemoteRunConfig{
			Command: "uname -m",
		})
		if err != nil {
			s.Stop()
			fmt.Print("\033[?25h")
			log.Fatalf("Failed to get remote VM architecture: %v", err)
		}
		remoteArch := strings.TrimSpace(remoteArchOutput)

		// Normalize architecture names
		normalizedLocalArch := normalizeArch(localArch)
		normalizedRemoteArch := normalizeArch(remoteArch)

		s.Stop()

		// Check if architectures match
		if normalizedLocalArch != normalizedRemoteArch {
			fmt.Print("\033[?25h")
			fmt.Printf("\033[31m\u2717\033[0m Architecture mismatch detected!\n")
			fmt.Printf("  Local image architecture:  %s (%s)\n", localArch, normalizedLocalArch)
			fmt.Printf("  Remote VM architecture:    %s (%s)\n", remoteArch, normalizedRemoteArch)
			fmt.Println("\nTo fix this issue:")
			fmt.Printf("  1. Pull the correct architecture: docker pull --platform linux/%s %s\n", normalizedRemoteArch, deployOpt.Image)
			fmt.Printf("  2. Then run the deploy command again\n")
			log.Fatalf("Cannot deploy %s image to %s VM", normalizedLocalArch, normalizedRemoteArch)
		}

		fmt.Printf("\033[32m\u2714\033[0m Architecture check passed: %s\033[?25h\n", normalizedLocalArch)

		// Step 1: Check if image already exists on remote VM
		s.Suffix = " Checking if Docker image already exists on remote VM..."
		s.Start()

		// Get local image layers (most reliable way to compare images)
		localLayersCmd := exec.Command("docker", "image", "inspect", deployOpt.Image, "--format", "{{.RootFS.Layers}}")
		localLayersOutput, err := localLayersCmd.Output()
		if err != nil {
			s.Stop()
			fmt.Print("\033[?25h")
			log.Fatalf("Failed to get local Docker image layers: %v", err)
		}
		localLayers := strings.TrimSpace(string(localLayersOutput))

		// Check if image exists on remote VM and get its layers
		checkRemoteCmd := fmt.Sprintf("docker image inspect %s --format '{{.RootFS.Layers}}'", deployOpt.Image)
		remoteLayersOutput, err := remote.RemoteRun(&tools.RemoteRunConfig{
			Command: checkRemoteCmd,
		})

		imageExists := false
		var remoteLayers string
		if err == nil {
			// Command succeeded, image exists
			remoteLayers = strings.TrimSpace(remoteLayersOutput)
			if remoteLayers != "" {
				imageExists = true
			}
		} else {
			// Command failed, image doesn't exist
			imageExists = false
			remoteLayers = ""
		}

		s.Stop()

		if imageExists && remoteLayers == localLayers {
			fmt.Printf("\033[32m\u2714\033[0m Docker image already exists on remote VM (same version)\033[?25h\n")
			// Skip upload and load, go directly to running container
			runContainer(remote, s, deployOpt.Image, deployOpt.Env, deployOpt.Name)
			return
		} else if imageExists {
			fmt.Printf("\033[33m\u26a0\033[0m Docker image exists but is different (will re-upload)\033[?25h\n")
		} else {
			fmt.Printf("\033[36m\u2139\033[0m Docker image not found on remote VM\033[?25h\n")
		}

		// Step 2: Save Docker image locally
		s.Suffix = " Saving Docker image locally..."
		s.Start()

		tempDir, err := os.MkdirTemp("", "onctl-deploy")
		if err != nil {
			log.Fatal(err)
		}
		defer os.RemoveAll(tempDir)

		imageTarPath := filepath.Join(tempDir, "image.tar.gz")
		dockerSaveCmd := exec.Command("sh", "-c", fmt.Sprintf("docker save %s | gzip > %s", deployOpt.Image, imageTarPath))
		if err := dockerSaveCmd.Run(); err != nil {
			s.Stop()
			fmt.Print("\033[?25h") // Ensure cursor is visible on error
			log.Fatalf("Failed to save and compress Docker image: %v", err)
		}

		// Get file size for display
		fileInfo, err := os.Stat(imageTarPath)
		if err != nil {
			log.Printf("Warning: Could not get file size: %v", err)
		}
		fileSize := fileInfo.Size()
		fileSizeMB := float64(fileSize) / (1024 * 1024)

		s.Suffix = " Docker image saved locally"
		s.Stop()
		fmt.Printf("\033[32m\u2714\033[0m Docker image saved locally (%.1f MB)\033[?25h\n", fileSizeMB)

		// Step 2: Upload image to remote VM with progress bar
		var totalBytes = fileSize
		startTime := time.Now()

		// Progress callback function
		progressCallback := func(current, total int64) {
			percentage := float64(current) / float64(total) * 100

			// Calculate speed in MBit/s
			elapsed := time.Since(startTime)
			if elapsed.Seconds() > 0 {
				bytesPerSecond := float64(current) / elapsed.Seconds()
				mbitsPerSecond := (bytesPerSecond * 8) / (1024 * 1024) // Convert to MBit/s
				if mbitsPerSecond < 1 {
					mbitsPerSecond = 0 // Don't show very low speeds
				}

				// Create progress bar (20 characters wide)
				progressBarWidth := 20
				filled := int(float64(current) / float64(total) * float64(progressBarWidth))
				bar := strings.Repeat("█", filled) + strings.Repeat("░", progressBarWidth-filled)

				s.Suffix = fmt.Sprintf(" Uploading... %.1f%% [%s] (%.1f/%.1f MB) %.1f MBit/s",
					percentage, bar, float64(current)/(1024*1024), fileSizeMB, mbitsPerSecond)
			} else {
				// First callback, speed not yet available
				progressBarWidth := 20
				filled := int(float64(current) / float64(total) * float64(progressBarWidth))
				bar := strings.Repeat("█", filled) + strings.Repeat("░", progressBarWidth-filled)

				s.Suffix = fmt.Sprintf(" Uploading... %.1f%% [%s] (%.1f/%.1f MB)",
					percentage, bar, float64(current)/(1024*1024), fileSizeMB)
			}
		}

		s.Suffix = fmt.Sprintf(" Uploading Docker image to remote VM (%.1f MB)...", fileSizeMB)
		s.Restart()

		remoteImagePath := "image.tar.gz"

		// Upload using SCP
		if deployOpt.TransferMethod == "sftp" {
			err = remote.SSHCopyFileWithProgress(imageTarPath, remoteImagePath, progressCallback)
		} else {
			// Default to SCP
			err = remote.SCPCopyFileWithProgress(imageTarPath, remoteImagePath, progressCallback)
		}

		if err != nil {
			s.Stop()
			fmt.Print("\033[?25h") // Ensure cursor is visible on error
			log.Fatalf("Failed to upload Docker image: %v", err)
		}

		// Final progress update
		progressCallback(totalBytes, totalBytes)
		time.Sleep(100 * time.Millisecond) // Brief pause to show 100%

		s.Suffix = " Docker image uploaded to remote VM"
		s.Stop()
		fmt.Println("\033[32m\u2714\033[0m Docker image uploaded to remote VM\033[?25h")

		// Step 3: Load Docker image on remote VM
		s.Restart()
		s.Suffix = " Loading Docker image on remote VM..."

		loadCmd := "gunzip -c image.tar.gz | docker load"
		_, err = remote.RemoteRun(&tools.RemoteRunConfig{
			Command: loadCmd,
		})
		if err != nil {
			s.Stop()
			fmt.Print("\033[?25h") // Ensure cursor is visible on error
			log.Fatalf("Failed to load Docker image on remote: %v", err)
		}

		s.Suffix = " Docker image loaded on remote VM"
		s.Stop()
		fmt.Println("\033[32m\u2714\033[0m Docker image loaded on remote VM\033[?25h")

		// Step 4: Run Docker container on remote VM
		runContainer(remote, s, deployOpt.Image, deployOpt.Env, deployOpt.Name)
	},
}
