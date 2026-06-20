package cmd

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"

	"github.com/cdalar/onctl/internal/files"
	"github.com/spf13/cobra"
	"golang.org/x/term"
)

const (
	onctlDirName  = ".onctl"
	initDir       = "init"
	onctlYamlFile = "onctl.yaml"
)

// legacyProviderConfigFiles lists the per-provider YAML files written by
// onctl releases prior to the single onctl.yaml config. They are no longer
// read by ReadConfig, so any settings in them (e.g. a custom gcp.project or
// azure subscription) are silently ignored once a .onctl directory has an
// onctl.yaml of the current format.
var legacyProviderConfigFiles = []string{"aws.yaml", "azure.yaml", "fc.yaml", "gcp.yaml", "hetzner.yaml"}

// skipInteractivePrompt is used to skip interactive prompts during testing
var skipInteractivePrompt = false

var initCmd = &cobra.Command{
	Use:   "init",
	Short: "init onctl environment",
	Run: func(cmd *cobra.Command, args []string) {
		if err := initializeOnctlEnv(); err != nil {
			log.Fatal(err)
		}
	},
}

func initializeOnctlEnv() error {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("failed to get home directory: %v", err)
	}
	homeOnctlPath := filepath.Join(homeDir, onctlDirName)

	localDir, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("failed to get working directory: %v", err)
	}
	localOnctlPath := filepath.Join(localDir, onctlDirName)

	// Always ensure home .onctl directory exists
	if _, err := os.Stat(homeOnctlPath); os.IsNotExist(err) {
		if err := os.Mkdir(homeOnctlPath, os.ModePerm); err != nil {
			return fmt.Errorf("failed to create %s directory: %w", homeOnctlPath, err)
		}
		if err := populateOnctlEnv(homeOnctlPath); err != nil {
			// Clean up empty directory if population fails
			_ = os.RemoveAll(homeOnctlPath)
			return err
		}
	} else if err != nil {
		// Handle other os.Stat errors (not IsNotExist)
		return fmt.Errorf("failed to check %s: %w", homeOnctlPath, err)
	} else {
		fmt.Printf("Global onctl environment already initialized in %s\n", homeOnctlPath)
		// A directory from before the single onctl.yaml config (e.g. one that
		// only had per-provider yaml files) has no onctl.yaml of its own;
		// populate it now instead of leaving ReadConfig to fail.
		if _, err := os.Stat(filepath.Join(homeOnctlPath, onctlYamlFile)); os.IsNotExist(err) {
			if err := populateOnctlEnv(homeOnctlPath); err != nil {
				return err
			}
		}
		warnLegacyProviderConfigFiles(homeOnctlPath)
	}

	// Check if local .onctl already exists
	if _, err := os.Stat(localOnctlPath); err == nil {
		fmt.Printf("Project-based onctl environment already initialized in %s\n", localOnctlPath)
		if _, err := os.Stat(filepath.Join(localOnctlPath, onctlYamlFile)); os.IsNotExist(err) {
			if err := populateOnctlEnv(localOnctlPath); err != nil {
				return err
			}
		}
		warnLegacyProviderConfigFiles(localOnctlPath)
		return nil
	} else if !os.IsNotExist(err) {
		// Handle other os.Stat errors (not IsNotExist)
		return fmt.Errorf("failed to check %s: %w", localOnctlPath, err)
	}

	// Ask user if they want to create a project-based .onctl folder
	// Only prompt if in interactive mode (TTY available) or skipInteractivePrompt is false
	if !skipInteractivePrompt && isInteractive() {
		fmt.Printf("\nDo you want to create a project-based .onctl folder in the current directory?\n")
		fmt.Printf("Project config will override global config settings.\n")
		if yesNo() {
			if err := os.Mkdir(localOnctlPath, os.ModePerm); err != nil {
				return fmt.Errorf("failed to create %s directory: %w", localOnctlPath, err)
			}
			if err := populateOnctlEnv(localOnctlPath); err != nil {
				// Clean up empty directory if population fails
				_ = os.RemoveAll(localOnctlPath)
				return err
			}
		}
	}

	return nil
}

// isInteractive checks if stdin is connected to a terminal (TTY)
func isInteractive() bool {
	return term.IsTerminal(int(os.Stdin.Fd()))
}

// warnLegacyProviderConfigFiles warns when configDir still has per-provider
// YAML files from before the single onctl.yaml config. ReadConfig only reads
// onctl.yaml, so settings left in these files (e.g. a custom gcp.project)
// are silently ignored; surface that instead of failing quietly.
func warnLegacyProviderConfigFiles(configDir string) {
	var found []string
	for _, name := range legacyProviderConfigFiles {
		if _, err := os.Stat(filepath.Join(configDir, name)); err == nil {
			found = append(found, name)
		}
	}
	if len(found) > 0 {
		fmt.Printf("Warning: %s no longer reads %s. Move any custom settings from these files into the matching provider section of %s.\n",
			configDir, strings.Join(found, ", "), filepath.Join(configDir, onctlYamlFile))
	}
}

func populateOnctlEnv(targetPath string) error {
	embedDir, err := files.EmbededFiles.ReadDir(initDir)
	if err != nil {
		return fmt.Errorf("failed to read embedded files: %w", err)
	}

	for _, configFile := range embedDir {
		log.Println("[DEBUG] initFile:", configFile.Name())
		eFile, err := files.EmbededFiles.ReadFile(filepath.Join(initDir, configFile.Name()))
		if err != nil {
			return fmt.Errorf("failed to read file %s: %w", configFile.Name(), err)
		}

		targetFilePath := filepath.Join(targetPath, configFile.Name())
		if err := os.WriteFile(targetFilePath, eFile, 0644); err != nil {
			return fmt.Errorf("failed to write file %s: %w", configFile.Name(), err)
		}
	}
	fmt.Printf("onctl environment initialized in %s\n", targetPath)
	return nil
}
