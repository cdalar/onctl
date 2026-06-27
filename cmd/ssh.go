package cmd

import (
	"fmt"
	"log"
	"os"

	"github.com/cdalar/onctl/internal/tools"
	"github.com/cdalar/onctl/internal/ui"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
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
		// Imported (static) hosts carry their own key/port from `onctl import`;
		// skip reading the global default key — we'll load credentials from inventory below.
		usesImportedKey := cloudProvider == "static" && sshOpt.Key == ""

		var privateKey []byte
		if !usesImportedKey {
			pk, err := os.ReadFile(privateKeyFile)
			if err != nil {
				log.Fatal(err)
			}
			privateKey = pk
		}
		vm, err := provider.GetByName(args[0])
		if err != nil {
			log.Fatalln(err)
		}
		remote := tools.Remote{
			Username:   viper.GetString(cloudProvider + ".vm.username"),
			IPAddress:  vm.IP,
			SSHPort:    sshOpt.Port,
			PrivateKey: string(privateKey),
			Spinner:    s,
		}
		// For imported (static) hosts, the username/port/key live in the
		// inventory, not in viper config or global SSH defaults.
		if cloudProvider == "static" {
			if sp, spErr := staticProvider(); spErr == nil {
				if h, hErr := sp.GetHost(args[0]); hErr == nil {
					remote.Username = h.Username
					if !cmd.Flags().Changed("port") {
						remote.SSHPort = h.SSHPort
					}
					if sshOpt.Key == "" && h.PrivateKey != "" {
						if pk, err := os.ReadFile(h.PrivateKey); err == nil {
							remote.PrivateKey = string(pk)
						}
					}
				}
			}
		}

		if sshOpt.DotEnvFile != "" {
			dotEnvVars, err := tools.ParseDotEnvFile(sshOpt.DotEnvFile)
			if err != nil {
				log.Println(err)
			}
			sshOpt.Variables = append(dotEnvVars, sshOpt.Variables...)
		}

		if len(sshOpt.UploadFiles) > 0 {
			ProcessUploadSlice(sshOpt.UploadFiles, remote)
		}

		// BEGIN Apply File
		for i, applyFile := range applyFileFound {
			s.Suffix = " Running " + sshOpt.ApplyFiles[i] + " on Remote..."
			s.Restart()

			err = remote.CopyAndRunRemoteFile(&tools.CopyAndRunRemoteFileConfig{
				File: applyFile,
				Vars: sshOpt.Variables,
			})
			if err != nil {
				log.Println(err)
			}
			s.Stop()
			fmt.Println("\033[32m\u2714\033[0m " + sshOpt.ApplyFiles[i] + " ran on Remote")

		}
		// END Apply File

		if len(sshOpt.DownloadFiles) > 0 {
			ProcessDownloadSlice(sshOpt.DownloadFiles, remote)
		}
		if isDirectSSH {
			sshKey := privateKeyFile
			sshPort := sshOpt.Port
			if cloudProvider == "static" {
				if usesImportedKey {
					sshKey = ""
				}
				if !cmd.Flags().Changed("port") {
					// let static.SSHInto fall back to the imported host's own port
					sshPort = 0
				}
			}
			provider.SSHInto(args[0], sshPort, sshKey, parseRemoteCmd(os.Args))
		}
	},
}
