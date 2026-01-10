package cmd

import (
	"fmt"
	"log"
	"os"
	"time"

	"github.com/briandowns/spinner"
	"github.com/cdalar/onctl/internal/cloud"
	"github.com/cdalar/onctl/internal/domain"
	"github.com/cdalar/onctl/internal/pipeline"
	"github.com/cdalar/onctl/internal/tools"
	"gopkg.in/yaml.v2"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

// CreateConfig holds configuration for VM creation
type CreateConfig struct {
	ConfigFile    string   `yaml:"configFile"`
	PublicKeyFile string   `yaml:"publicKeyFile"`
	ApplyFiles    []string `yaml:"applyFiles"`
	DotEnvFile    string   `yaml:"dotEnvFile"`
	Variables     []string `yaml:"variables"`
	Vm            cloud.Vm `yaml:"vm"`
	Domain        string   `yaml:"domain"`
	DownloadFiles []string `yaml:"downloadFiles"`
	UploadFiles   []string `yaml:"uploadFiles"`
}

var createConfig CreateConfig

func parseConfigFile(configFile string) (*CreateConfig, error) {
	file, err := os.Open(configFile)
	if err != nil {
		return nil, fmt.Errorf("failed to open config file %q: %w", configFile, err)
	}
	defer func() {
		if err := file.Close(); err != nil {
			log.Printf("Failed to close config file: %v", err)
		}
	}()

	var config CreateConfig
	decoder := yaml.NewDecoder(file)
	if err := decoder.Decode(&config); err != nil {
		return nil, fmt.Errorf("failed to parse config file %q: %w", configFile, err)
	}

	return &config, nil
}

func parsePipelineConfigForCreate(configFile string) (*CreateConfig, error) {
	pipelineConfig, err := pipeline.LoadConfig(configFile)
	if err != nil {
		return nil, fmt.Errorf("failed to parse pipeline config file %q: %w", configFile, err)
	}

	// For single VM create, use the first target if there's only one, or require target specification
	var targetName string
	if len(pipelineConfig.Targets) == 1 {
		targetName = pipelineConfig.Targets[0].Name
	} else {
		// If multiple targets, look for create steps and use their targets
		for _, step := range pipelineConfig.Steps {
			if step.Type == "create" {
				targetName = step.Target
				break
			}
		}
		if targetName == "" {
			targetName = pipelineConfig.Targets[0].Name // fallback to first target
		}
	}

	// Find target config
	var target *pipeline.Target
	var targetConfig *pipeline.TargetConfig
	for _, t := range pipelineConfig.Targets {
		if t.Name == targetName {
			target = &t
			targetConfig = &t.Config
			break
		}
	}
	if target == nil {
		return nil, fmt.Errorf("target %q not found in pipeline configuration", targetName)
	}

	// Initialize create config from target
	config := &CreateConfig{
		PublicKeyFile: targetConfig.PublicKeyFile,
		Vm:            targetConfig.Vm,
	}

	// Find apply/create steps for this target and merge their configs
	for _, step := range pipelineConfig.Steps {
		if step.Type == "apply" && step.Target == targetName {
			if step.Config.DotEnvFile != "" {
				config.DotEnvFile = step.Config.DotEnvFile
			}
			if len(step.Config.Variables) > 0 {
				config.Variables = append(config.Variables, step.Config.Variables...)
			}
			// Apply steps can have files to run
			if len(step.Config.Files) > 0 {
				config.ApplyFiles = append(config.ApplyFiles, step.Config.Files...)
			}
		}
		if step.Type == "upload" && step.Target == targetName {
			if len(step.Config.Files) > 0 {
				config.UploadFiles = append(config.UploadFiles, step.Config.Files...)
			}
		}
		if step.Type == "download" && step.Target == targetName {
			if len(step.Config.Files) > 0 {
				config.DownloadFiles = append(config.DownloadFiles, step.Config.Files...)
			}
		}
	}

	return config, nil
}

func init() {
	createCmd.Flags().StringVarP(&createConfig.PublicKeyFile, "publicKey", "k", "", "Path to publicKey file (default: ~/.ssh/id_rsa))")
	createCmd.Flags().StringSliceVarP(&createConfig.ApplyFiles, "apply-file", "a", []string{}, "bash script file(s) to run on remote")
	createCmd.Flags().StringSliceVarP(&createConfig.DownloadFiles, "download", "d", []string{}, "List of files to download")
	createCmd.Flags().StringSliceVarP(&createConfig.UploadFiles, "upload", "u", []string{}, "List of files to upload")
	createCmd.Flags().StringVarP(&createConfig.Vm.Name, "name", "n", "", "vm name")
	createCmd.Flags().IntVarP(&createConfig.Vm.SSHPort, "ssh-port", "p", 22, "ssh port")
	createCmd.Flags().StringVarP(&createConfig.Vm.CloudInitFile, "cloud-init", "i", "", "cloud-init file")
	createCmd.Flags().StringVar(&createConfig.DotEnvFile, "dot-env", "", "dot-env (.env) file")
	createCmd.Flags().StringVar(&createConfig.Domain, "domain", "", "request a domain name for the VM")
	createCmd.Flags().StringSliceVarP(&createConfig.Variables, "vars", "e", []string{}, "Environment variables passed to the script")
	createCmd.Flags().StringVarP(&createConfig.ConfigFile, "file", "f", "", "Path to configuration YAML file")
	// Register create command at root level for convenience
	rootCmd.AddCommand(createCmd)
	createCmd.SetUsageTemplate(createCmd.UsageTemplate() + `
Environment Variables:
  CLOUDFLARE_API_TOKEN  Cloudflare API Token (required for --domain)
  CLOUDFLARE_ZONE_ID    Cloudflare Zone ID (required for --domain)
`)
}

var createCmd = &cobra.Command{
	Use:     "create",
	Aliases: []string{"start", "up"},
	Short:   "Create a VM",
	Long:    `Create a VM with the specified options and run the cloud-init file on the remote.`,
	Example: `  # Create a VM with docker installed and set ssh on port 443
  onctl create -n onctl-test -a docker/docker.sh -i cloud-init/ssh-443.config`,
	Run: func(cmd *cobra.Command, args []string) {
		if createConfig.ConfigFile != "" {
			config, err := parsePipelineConfigForCreate(createConfig.ConfigFile)
			if err != nil {
				log.Fatalf("Error parsing config file: %v", err)
			}
			log.Println("[DEBUG] config file: ", createConfig.ConfigFile)
			log.Printf("[DEBUG] Parsed config: %+v\n", config)

			// Merge parsed config with CLI flags (CLI flags take precedence)
			if createConfig.PublicKeyFile == "" && config.PublicKeyFile != "" {
				createConfig.PublicKeyFile = config.PublicKeyFile
			}
			if len(createConfig.ApplyFiles) == 0 && len(config.ApplyFiles) > 0 {
				createConfig.ApplyFiles = append(createConfig.ApplyFiles, config.ApplyFiles...)
			}
			if createConfig.DotEnvFile == "" && config.DotEnvFile != "" {
				createConfig.DotEnvFile = config.DotEnvFile
			}
			if len(createConfig.Variables) == 0 && len(config.Variables) > 0 {
				createConfig.Variables = append(createConfig.Variables, config.Variables...)
			}
			if createConfig.Vm.Name == "" && config.Vm.Name != "" {
				createConfig.Vm.Name = config.Vm.Name
			}
			if createConfig.Vm.SSHPort == 22 && config.Vm.SSHPort != 0 {
				createConfig.Vm.SSHPort = config.Vm.SSHPort
			}
			if createConfig.Vm.CloudInitFile == "" && config.Vm.CloudInitFile != "" {
				createConfig.Vm.CloudInitFile = config.Vm.CloudInitFile
			}
			if createConfig.Domain == "" && config.Domain != "" {
				createConfig.Domain = config.Domain
			}
			if len(createConfig.DownloadFiles) == 0 && len(config.DownloadFiles) > 0 {
				createConfig.DownloadFiles = append(createConfig.DownloadFiles, config.DownloadFiles...)
			}
			if len(createConfig.UploadFiles) == 0 && len(config.UploadFiles) > 0 {
				createConfig.UploadFiles = append(createConfig.UploadFiles, config.UploadFiles...)
			}
		}
		s := spinner.New(spinner.CharSets[9], 100*time.Millisecond) // Build our new spinner
		s.Start()
		s.Suffix = " Checking vm..."
		list, err := provider.List()
		if err != nil {
			s.Stop()
			log.Println(err)
		}

		for _, vm := range list.List {
			if vm.Name == createConfig.Vm.Name {
				s.Stop()
				fmt.Println("\033[31m\u2718\033[0m VM " + createConfig.Vm.Name + " exists. Aborting...")
				os.Exit(1)
			}
		}

		s.Stop()
		fmt.Println("\033[32m\u2714\033[0m Creating VM...")

		// Check Domain Env
		if createConfig.Domain != "" {
			s.Start()
			s.Suffix = " --domain flag is set... Checking Domain Env..."
			err := domain.NewCloudFlareService().CheckEnv()
			if err != nil {
				s.Stop()
				fmt.Println("\033[31m\u2718\033[0m Error on Domain: ", err)
				os.Exit(1)
			}
		}

		applyFileFound := findFile(createConfig.ApplyFiles)
		log.Println("[DEBUG] applyFileFound: ", applyFileFound)
		createConfig.Vm.CloudInitFile = findSingleFile(createConfig.Vm.CloudInitFile)

		// BEGIN SSH Key
		publicKeyFile, privateKeyFile := getSSHKeyFilePaths(createConfig.PublicKeyFile)
		log.Println("[DEBUG] publicKeyFile: ", publicKeyFile)
		log.Println("[DEBUG] privateKeyFile: ", privateKeyFile)
		fmt.Println("\033[32m\u2714\033[0m Using Public Key:", publicKeyFile)
		s.Start()
		s.Suffix = " Checking SSH Keys..."
		createConfig.Vm.SSHKeyID, err = provider.CreateSSHKey(publicKeyFile)
		if err != nil {
			s.Stop()
			fmt.Println("\033[32m\u2718\033[0m Checking SSH Keys...")
			log.Fatalln(err)
		}
		s.Stop()
		fmt.Println("\033[32m\u2714\033[0m Checking SSH Keys... ")
		// END SSH Key

		// BEGIN Set VM Name
		log.Printf("[DEBUG] keyID: %s", createConfig.Vm.SSHKeyID)
		if createConfig.Vm.Name == "" {
			if viper.GetString("vm.name") != "" {
				createConfig.Vm.Name = viper.GetString("vm.name")
			} else {
				createConfig.Vm.Name = tools.GenerateMachineUniqueName()
			}
		}
		s.Restart()
		s.Suffix = " VM Starting..."
		// END Set VM Name

		vm, err := provider.Deploy(createConfig.Vm)
		if err != nil {
			log.Println(err)
		}
		s.Restart()
		s.Suffix = " VM IP: " + vm.IP
		s.Stop()
		fmt.Println("\033[32m\u2714\033[0m" + s.Suffix)

		log.Println("[DEBUG] Vm:" + vm.String())
		privateKey, err := os.ReadFile(privateKeyFile)
		if err != nil {
			log.Println(err)
		}

		// BEGIN Cloud-init
		log.Println("[DEBUG] waiting for cloud-init")
		log.Println("[DEBUG] ssh port: ", createConfig.Vm.SSHPort)
		s.Stop()
		// fmt.Println("\033[32m\u2714\033[0m VM Starting...")
		remote := tools.Remote{
			Username:   viper.GetString(cloudProvider + ".vm.username"),
			IPAddress:  vm.IP,
			SSHPort:    createConfig.Vm.SSHPort,
			PrivateKey: string(privateKey),
			Spinner:    s,
		}

		// BEGIN Domain
		if createConfig.Domain != "" {
			s.Restart()
			s.Suffix = " Requesting Domain..."
			_, err := domain.NewCloudFlareService().SetRecord(&domain.SetRecordRequest{
				Subdomain: createConfig.Domain,
				Ipaddress: vm.IP,
			})
			s.Stop()
			if err != nil {
				fmt.Println("\033[31m\u2718\033[0m Error on Domain: ")
				log.Println(err)
			} else {
				fmt.Println("\033[32m\u2714\033[0m Domain is ready: ")
			}
		}

		s.Restart()
		s.Suffix = " Waiting for VM to be ready..."
		remote.WaitForCloudInit(viper.GetString("vm.cloud-init.timeout"))
		s.Stop()
		fmt.Println("\033[32m\u2714\033[0m VM is Ready")
		log.Println("[DEBUG] cloud-init finished")
		// END Cloud-init

		s.Restart()
		s.Suffix = " Configuring VM..."
		if createConfig.DotEnvFile != "" {
			dotEnvVars, err := tools.ParseDotEnvFile(createConfig.DotEnvFile)
			if err != nil {
				log.Println(err)
			}
			createConfig.Variables = append(dotEnvVars, createConfig.Variables...)
		}

		// Upload Files
		if len(createConfig.UploadFiles) > 0 {
			ProcessUploadSlice(createConfig.UploadFiles, remote)
		}

		// BEGIN Apply File
		for i, applyFile := range applyFileFound {
			s.Restart()
			s.Suffix = " Running " + createConfig.ApplyFiles[i] + " on Remote..."

			err = remote.CopyAndRunRemoteFile(&tools.CopyAndRunRemoteFileConfig{
				File: applyFile,
				Vars: createConfig.Variables,
			})
			if err != nil {
				log.Println(err)
			}
			s.Stop()
			fmt.Println("\033[32m\u2714\033[0m " + createConfig.ApplyFiles[i] + " ran on Remote")

		}
		if len(createConfig.DownloadFiles) > 0 {
			ProcessDownloadSlice(createConfig.DownloadFiles, remote)
		}
		s.Stop()
		fmt.Println("\033[32m\u2714\033[0m VM Configured...")
	},
}
