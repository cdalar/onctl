package cmd

import (
	"fmt"
	"github.com/cdalar/onctl/pkg/box"
	"log"
	"os"

	"github.com/cdalar/onctl/internal/tools"
	"github.com/cdalar/onctl/internal/ui"
	"github.com/cdalar/onctl/pkg/cloud"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v2"
)

type cmdSSHOptions struct {
	Port          int      `yaml:"port"`
	ApplyFiles    []string `yaml:"applyFiles"`
	DownloadFiles []string `yaml:"downloadFiles"`
	UploadFiles   []string `yaml:"uploadFiles"`
	Key           string   `yaml:"key"`
	DotEnvFile    string   `yaml:"dotEnvFile"`
	Variables     []string `yaml:"variables"`
	ConfigFile    string   `yaml:"configFile"`
}

var sshOpt cmdSSHOptions

func parseRemoteCmd(osArgs []string) []string {
	for i, a := range osArgs {
		if a == "--" {
			return osArgs[i+1:]
		}
	}
	return nil
}

// shouldSSHInto reports whether provider.SSHInto should run: either an
// explicit trailing `-- command` was given (regardless of -u/-d/-a/-f), or
// this is a bare `ssh VM_NAME` invocation that should open an interactive
// session.
func shouldSSHInto(isDirectSSH bool, remoteCmd []string) bool {
	return len(remoteCmd) > 0 || isDirectSSH
}

func parseSSHConfigFile(configFile string) (*cmdSSHOptions, error) {
	file, err := os.Open(configFile)
	if err != nil {
		return nil, fmt.Errorf("failed to open config file %q: %w", configFile, err)
	}
	defer func() {
		if err := file.Close(); err != nil {
			log.Printf("Failed to close config file: %v", err)
		}
	}()

	var config cmdSSHOptions
	decoder := yaml.NewDecoder(file)
	if err := decoder.Decode(&config); err != nil {
		return nil, fmt.Errorf("failed to parse config file %q: %w", configFile, err)
	}

	return &config, nil
}

func init() {
	sshCmd.Flags().StringVarP(&sshOpt.Key, "key", "k", "", "Path to privateKey file (default: ~/.ssh/id_rsa))")
	sshCmd.Flags().IntVarP(&sshOpt.Port, "port", "P", 22, "ssh port")
	sshCmd.Flags().StringSliceVarP(&sshOpt.ApplyFiles, "apply-file", "a", []string{}, "bash script file(s) to run on remote")
	sshCmd.Flags().StringSliceVarP(&sshOpt.DownloadFiles, "download", "d", []string{}, "List of files to download")
	sshCmd.Flags().StringSliceVarP(&sshOpt.UploadFiles, "upload", "u", []string{}, "List of files to upload")
	sshCmd.Flags().StringVar(&sshOpt.DotEnvFile, "dot-env", "", "dot-env (.env) file")
	sshCmd.Flags().StringSliceVarP(&sshOpt.Variables, "vars", "e", []string{}, "Environment variables passed to the script")
	sshCmd.Flags().StringVarP(&sshOpt.ConfigFile, "file", "f", "", "Path to configuration YAML file")
	// Register ssh command at root level for convenience
	rootCmd.AddCommand(sshCmd)
}

var sshCmd = &cobra.Command{
	Use:                   "ssh VM_NAME [-- COMMAND [ARGS...]]",
	Short:                 "Spawn an SSH connection to a VM",
	Args:                  cobra.MinimumNArgs(1),
	TraverseChildren:      true,
	DisableFlagsInUseLine: true,
	ValidArgsFunction: func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
		ensureProvider()
		if provider == nil {
			return nil, cobra.ShellCompDirectiveError
		}
		VMList, err := boxClient().Servers()
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
		defer ensureCursorVisible()
		if sshOpt.ConfigFile != "" {
			config, err := parseSSHConfigFile(sshOpt.ConfigFile)
			if err != nil {
				log.Fatalf("Error parsing config file: %v", err)
			}
			log.Println("[DEBUG] config file: ", sshOpt.ConfigFile)
			log.Printf("[DEBUG] Parsed config: %+v\n", config)

			// Merge config file options into the command options
			if config.Key != "" {
				sshOpt.Key = config.Key
			}
			if config.Port != 0 {
				sshOpt.Port = config.Port
			}
			if len(config.ApplyFiles) > 0 {
				sshOpt.ApplyFiles = append(sshOpt.ApplyFiles, config.ApplyFiles...)
			}
			if len(config.DownloadFiles) > 0 {
				sshOpt.DownloadFiles = append(sshOpt.DownloadFiles, config.DownloadFiles...)
			}
			if len(config.UploadFiles) > 0 {
				sshOpt.UploadFiles = append(sshOpt.UploadFiles, config.UploadFiles...)
			}
			if config.DotEnvFile != "" {
				sshOpt.DotEnvFile = config.DotEnvFile
			}
			if len(config.Variables) > 0 {
				sshOpt.Variables = append(sshOpt.Variables, config.Variables...)
			}
		}

		s := ui.New() // Build our new spinner
		applyFileFound := findFile(sshOpt.ApplyFiles)
		log.Println("[DEBUG] args: ", args)

		if len(args) == 0 {
			fmt.Println("Please provide a VM id")
			return
		}
		log.Println("[DEBUG] port:", sshOpt.Port)
		log.Println("[DEBUG] filename:", applyFileFound)
		log.Println("[DEBUG] key:", sshOpt.Key)
		_, privateKeyFile := getSSHKeyFilePaths(sshOpt.Key)
		log.Println("[DEBUG] privateKeyFile:", privateKeyFile)

		isDirectSSH := sshOpt.ConfigFile == "" && len(applyFileFound) == 0 && len(sshOpt.DownloadFiles) == 0 && len(sshOpt.UploadFiles) == 0
		if cloudProvider == "openshell" {
			if err := openShellSSHCheck(applyFileFound, sshOpt.UploadFiles, sshOpt.DownloadFiles); err != nil {
				log.Fatalln(err)
			}
		}
		// Imported (static) hosts carry their own key/port from `onctl
		// import`; box.Connect uses them unless -k/--port say otherwise.
		usesImportedKey := cloudProvider == "static" && sshOpt.Key == ""
		sshPort := sshOpt.Port
		if cloudProvider == "static" && !cmd.Flags().Changed("port") {
			sshPort = 0
		}
		vm, err := boxClient().Get(args[0])
		if err != nil {
			log.Fatalln(err)
		}
		conn, err := connect(vm, sshOpt.Key, sshPort, s)
		if err != nil {
			log.Fatalln(err)
		}
		defer func() { _ = conn.Close() }()

		if sshOpt.DotEnvFile != "" {
			dotEnvVars, err := tools.ParseDotEnvFile(sshOpt.DotEnvFile)
			if err != nil {
				log.Println(err)
			}
			sshOpt.Variables = append(dotEnvVars, sshOpt.Variables...)
		}

		setup := box.Setup{
			Uploads:    parseUploads(sshOpt.UploadFiles),
			ApplyFiles: applyFileFound,
			Vars:       sshOpt.Variables,
			Downloads:  parseDownloads(sshOpt.DownloadFiles),
			Out:        os.Stdout,
		}
		printProgress(s, &setup)
		// A failed upload, apply file or download doesn't stop the
		// trailing command; it fails the command after.
		provisionErr := boxClient().Provision(conn, setup)
		s.Stop()
		if provisionErr != nil {
			log.Println(provisionErr)
		}
		remoteCmd := parseRemoteCmd(os.Args)
		// Run the trailing `-- command` whenever one is given, even alongside
		// -u/-d/-a/-f; previously it only ran on a bare `ssh VM` invocation, so
		// it was silently dropped whenever those flags were also present.
		if shouldSSHInto(isDirectSSH, remoteCmd) {
			sshKey := privateKeyFile
			if usesImportedKey {
				sshKey = ""
			}
			code, err := boxClient().SSH(args[0], sshPort, sshKey, remoteCmd, cloud.Stdio{})
			if err != nil {
				log.Fatalln(err)
			}
			if code != 0 {
				os.Exit(code)
			}
		}
		if provisionErr != nil {
			os.Exit(1)
		}
	},
}
