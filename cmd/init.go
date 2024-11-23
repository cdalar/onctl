package cmd

import (
	"fmt"
	"log"
	"os"

	"github.com/cdalar/onctl/internal/files"
	"github.com/spf13/cobra"
)

const (
	onctlDir = ".onctl"
	initDir  = "init"
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
	if _, err := os.Stat(onctlDir); os.IsNotExist(err) {
		if err := os.Mkdir(onctlDir, os.ModePerm); err != nil {
			return fmt.Errorf("failed to create %s directory: %w", onctlDir, err)
		}
		return populateOnctlEnv()
	}
	fmt.Println("onctl environment already initialized")
	return nil
}

func populateOnctlEnv() error {
	embedDir, err := files.EmbededFiles.ReadDir(initDir)
	if err != nil {
		return fmt.Errorf("failed to read embedded files: %w", err)
	}

	for _, configFile := range embedDir {
		log.Println("[DEBUG] initFile:", configFile.Name())
		eFile, err := files.EmbededFiles.ReadFile(initDir + "/" + configFile.Name())
		if err != nil {
			return fmt.Errorf("failed to read file %s: %w", configFile.Name(), err)
		}

		if err := os.WriteFile(onctlDir+"/"+configFile.Name(), eFile, 0644); err != nil {
			return fmt.Errorf("failed to write file %s: %w", configFile.Name(), err)
		}
	}
	fmt.Println("onctl environment initialized")
	return nil
}
