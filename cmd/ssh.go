package cmd

import (
	"fmt"
	"log"
	"os"
	"time"

	"github.com/briandowns/spinner"
	"github.com/cdalar/onctl/internal/tools"
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
	JumpHost      string   `yaml:"jumpHost"`
}

var sshOpt cmdSSHOptions

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
	sshCmd.Flags().IntVarP(&sshOpt.Port, "port", "p", 22, "ssh port")
	sshCmd.Flags().StringSliceVarP(&sshOpt.ApplyFiles, "apply-file", "a", []string{}, "bash script file(s) to run on remote")
	sshCmd.Flags().StringSliceVarP(&sshOpt.DownloadFiles, "download", "d", []string{}, "List of files to download")
	sshCmd.Flags().StringSliceVarP(&sshOpt.UploadFiles, "upload", "u", []string{}, "List of files to upload")
	sshCmd.Flags().StringVar(&sshOpt.DotEnvFile, "dot-env", "", "dot-env (.env) file")
	sshCmd.Flags().StringSliceVarP(&sshOpt.Variables, "vars", "e", []string{}, "Environment variables passed to the script")
	sshCmd.Flags().StringVarP(&sshOpt.ConfigFile, "file", "f", "", "Path to configuration YAML file")
	sshCmd.Flags().StringVarP(&sshOpt.JumpHost, "jump-host", "j", "", "Jump host")
}

var sshCmd = &cobra.Command{
	Use:                   "ssh VM_NAME",
	Short:                 "Spawn an SSH connection to a VM",
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
			if config.JumpHost != "" {
				sshOpt.JumpHost = config.JumpHost
			}
		}

		s := spinner.New(spinner.CharSets[9], 100*time.Millisecond) // Build our new spinner
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

		privateKey, err := os.ReadFile(privateKeyFile)
		if err != nil {
			log.Fatal(err)
		}
		vm, err := provider.GetByName(args[0])
		if err != nil {
			log.Fatalln(err)
		}

		// Resolve jumphost name to IP address if it's not already an IP
		resolvedJumpHost := sshOpt.JumpHost
		if sshOpt.JumpHost != "" {
			jumpHostVM, err := provider.GetByName(sshOpt.JumpHost)
			if err != nil {
				log.Printf("[WARNING] Could not resolve jumphost '%s': %v", sshOpt.JumpHost, err)
			} else {
				resolvedJumpHost = jumpHostVM.IP
				log.Printf("[DEBUG] Resolved jumphost '%s' to IP '%s'", sshOpt.JumpHost, resolvedJumpHost)
			}
		}

		// Determine which IP to use for the target VM
		var targetIP string
		if resolvedJumpHost != "" && (vm.IP == "" || vm.IP == "<nil>") {
			// If using jumphost and no public IP, use private IP
			if vm.PrivateIP != "" && vm.PrivateIP != "N/A" {
				targetIP = vm.PrivateIP
				log.Printf("[DEBUG] Using private IP '%s' for target VM", targetIP)
			} else {
				log.Fatalln("No private IP available for VM")
			}
		} else {
			// Use public IP if available
			if vm.IP != "" && vm.IP != "<nil>" {
				targetIP = vm.IP
				log.Printf("[DEBUG] Using public IP '%s' for target VM", targetIP)
			} else {
				log.Fatalln("No public IP available for VM and no jumphost specified")
			}
		}

		remote := tools.Remote{
			Username:   viper.GetString(cloudProvider + ".vm.username"),
			IPAddress:  targetIP,
			SSHPort:    sshOpt.Port,
			PrivateKey: string(privateKey),
			Spinner:    s,
			JumpHost:   resolvedJumpHost,
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
			s.Restart()
			s.Suffix = " Running " + sshOpt.ApplyFiles[i] + " on Remote..."

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
		if sshOpt.ConfigFile == "" && len(applyFileFound) == 0 && len(sshOpt.DownloadFiles) == 0 && len(sshOpt.UploadFiles) == 0 {
			// Call SSH directly with the calculated target IP and resolved jump host
			tools.SSHIntoVM(tools.SSHIntoVMRequest{
				IPAddress:      targetIP,
				User:           viper.GetString(cloudProvider + ".vm.username"),
				Port:           sshOpt.Port,
				PrivateKeyFile: privateKeyFile,
				JumpHost:       resolvedJumpHost,
			})
		}
	},
}
