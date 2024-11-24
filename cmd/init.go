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
	// Determine the target .onctl directory
	localDir, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("failed to get working directory: %v", err)
	}
	localOnctlPath := filepath.Join(localDir, onctlDirName)

	homeDir, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("failed to get home directory: %v", err)
	}
	homeOnctlPath := filepath.Join(homeDir, onctlDirName)

	var targetPath string
	if _, err := os.Stat(localOnctlPath); os.IsNotExist(err) {
		// If .onctl doesn't exist in current directory, use home directory
		targetPath = homeOnctlPath
	} else {
		targetPath = localOnctlPath
	}

	// Create the .onctl directory if it doesn't exist
	if _, err := os.Stat(targetPath); os.IsNotExist(err) {
		if err := os.Mkdir(targetPath, os.ModePerm); err != nil {
			return fmt.Errorf("failed to create %s directory: %w", targetPath, err)
		}
		return populateOnctlEnv(targetPath)
	}

	fmt.Printf("onctl environment already initialized in %s\n", targetPath)
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
