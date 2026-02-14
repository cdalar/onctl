package cmd

import (
	"fmt"
	"log"
	"os"
	"path/filepath"

	"github.com/cdalar/onctl/internal/files"
	"github.com/spf13/cobra"
)

const (
	onctlDirName = ".onctl"
	initDir      = "init"
)

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
	homeExists := false
	if _, err := os.Stat(homeOnctlPath); os.IsNotExist(err) {
		if err := os.Mkdir(homeOnctlPath, os.ModePerm); err != nil {
			return fmt.Errorf("failed to create %s directory: %w", homeOnctlPath, err)
		}
		if err := populateOnctlEnv(homeOnctlPath); err != nil {
			return err
		}
		homeExists = true
	} else {
		fmt.Printf("Global onctl environment already initialized in %s\n", homeOnctlPath)
		homeExists = true
	}

	// Check if local .onctl already exists
	if _, err := os.Stat(localOnctlPath); err == nil {
		fmt.Printf("Project-based onctl environment already initialized in %s\n", localOnctlPath)
		return nil
	}

	// Ask user if they want to create a project-based .onctl folder
	if homeExists && !skipInteractivePrompt {
		fmt.Printf("\nDo you want to create a project-based .onctl folder in the current directory?\n")
		fmt.Printf("Project config will override global config settings.\n")
		if yesNo() {
			if err := os.Mkdir(localOnctlPath, os.ModePerm); err != nil {
				return fmt.Errorf("failed to create %s directory: %w", localOnctlPath, err)
			}
			if err := populateOnctlEnv(localOnctlPath); err != nil {
				return err
			}
		}
	}

	return nil
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
