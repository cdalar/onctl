package cmd

import (
	"fmt"
	"log"
	"os"

	"github.com/spf13/cobra"
)

var (
	envCreateTemplateFile  string
	envDestroyTemplateFile string
	envDestroyForce        bool
)

// envCmd is the parent command for environment management
var envCmd = &cobra.Command{
	Use:   "env",
	Short: "Manage environments",
	Long:  `Create or destroy environments using template files.`,
}

// envCreateCmd creates an environment from a template file
var envCreateCmd = &cobra.Command{
	Use:     "create",
	Aliases: []string{"start", "up"},
	Short:   "Create environment from a template file",
	Long:    `Create an entire environment defined in a template file.`,
	Run: func(cmd *cobra.Command, args []string) {
		if envCreateTemplateFile == "" {
			fmt.Println("Error: --template (-t) file must be specified")
			if err := cmd.Usage(); err != nil {
				log.Printf("Failed to display usage: %v", err)
			}
			os.Exit(1)
		}
		// Create the environment (alias for create with config file)
		// Set create command to use the template file
		if err := createCmd.Flags().Set("file", envCreateTemplateFile); err != nil {
			log.Fatalf("Error setting template file: %v", err)
		}
		createCmd.Run(createCmd, []string{})
	},
}

// envDestroyCmd destroys an environment defined in a template file
var envDestroyCmd = &cobra.Command{
	Use:     "destroy",
	Aliases: []string{"down", "delete", "remove", "rm"},
	Short:   "Destroy environment from a template file",
	Long:    `Destroy an entire environment defined in a template file.`,
	Run: func(cmd *cobra.Command, args []string) {
		if envDestroyTemplateFile == "" {
			fmt.Println("Error: --template (-t) file must be specified")
			if err := cmd.Usage(); err != nil {
				log.Printf("Failed to display usage: %v", err)
			}
			os.Exit(1)
		}
		// Parse template to get VM name
		config, err := parsePipelineConfigForCreate(envDestroyTemplateFile)
		if err != nil {
			log.Fatalf("Error parsing template file: %v", err)
		}
		vmName := config.Vm.Name
		if vmName == "" {
			log.Fatalf("No VM name specified in template file")
		}

		// Ask for confirmation unless force flag is set
		if !envDestroyForce {
			fmt.Printf("Are you sure you want to destroy the environment '%s'? This action cannot be undone.\n", vmName)
			if !yesNo() {
				fmt.Println("Environment destruction cancelled.")
				os.Exit(0)
			}
		}

		// Destroy the VM by name
		destroyCmd.Run(destroyCmd, []string{vmName})
	},
}

func init() {
	rootCmd.AddCommand(envCmd)
	envCmd.AddCommand(envCreateCmd)
	envCmd.AddCommand(envDestroyCmd)

	envCreateCmd.Flags().StringVarP(&envCreateTemplateFile, "config", "f", "", "Path to environment file")
	envDestroyCmd.Flags().StringVarP(&envDestroyTemplateFile, "config", "f", "", "Path to environment file")
	envDestroyCmd.Flags().BoolVarP(&envDestroyForce, "force", "F", false, "force destroy environment without confirmation")
}
