package cmd

import (
	"fmt"
	"log"
	"os"
	"time"

	"github.com/briandowns/spinner"
	"github.com/cdalar/onctl/internal/cloud"
	"github.com/cdalar/onctl/internal/domain"
	"github.com/cdalar/onctl/internal/tools"
	"gopkg.in/yaml.v2"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

// TODO: ? Struct for options. cmdCreateOptions
// TODO: .env file support
// TODO: remove initFile and implement ssh apply structure
// TODO: ? Create Packages with cloud-init, apply Files, Variables. (cloud-init, apply, vars)

type cmdCreateOptions struct {
	PublicKeyFile string   `yaml:"publicKeyFile"`
	ApplyFiles    []string `yaml:"applyFiles"`
	DotEnvFile    string   `yaml:"dotEnvFile"`
	Variables     []string `yaml:"variables"`
	Vm            cloud.Vm `yaml:"vm"`
	Domain        string   `yaml:"domain"`
	DownloadFiles []string `yaml:"downloadFiles"`
	UploadFiles   []string `yaml:"uploadFiles"`
	ConfigFile    string   `yaml:"configFile"`
}

var (
	opt cmdCreateOptions
)

func parseConfigFile(configFile string) (*cmdCreateOptions, error) {
	file, err := os.Open(configFile)
	if err != nil {
		return nil, fmt.Errorf("failed to open config file %q: %w", configFile, err)
	}
	defer func() {
		if err := file.Close(); err != nil {
			log.Printf("Failed to close config file: %v", err)
		}
	}()

	var config cmdCreateOptions
	decoder := yaml.NewDecoder(file)
	if err := decoder.Decode(&config); err != nil {
		return nil, fmt.Errorf("failed to parse config file %q: %w", configFile, err)
	}

	return &config, nil
}

func init() {
	createCmd.Flags().StringVarP(&opt.PublicKeyFile, "publicKey", "k", "", "Path to publicKey file (default: ~/.ssh/id_rsa))")
	createCmd.Flags().StringSliceVarP(&opt.ApplyFiles, "apply-file", "a", []string{}, "bash script file(s) to run on remote")
	createCmd.Flags().StringSliceVarP(&opt.DownloadFiles, "download", "d", []string{}, "List of files to download")
	createCmd.Flags().StringSliceVarP(&opt.UploadFiles, "upload", "u", []string{}, "List of files to upload")
	createCmd.Flags().StringVarP(&opt.Vm.Name, "name", "n", "", "vm name")
	createCmd.Flags().IntVarP(&opt.Vm.SSHPort, "ssh-port", "p", 22, "ssh port")
	createCmd.Flags().StringVarP(&opt.Vm.CloudInitFile, "cloud-init", "i", "", "cloud-init file")
	createCmd.Flags().StringVar(&opt.DotEnvFile, "dot-env", "", "dot-env (.env) file")
	createCmd.Flags().StringVar(&opt.Domain, "domain", "", "request a domain name for the VM")
	createCmd.Flags().StringSliceVarP(&opt.Variables, "vars", "e", []string{}, "Environment variables passed to the script")
	createCmd.Flags().StringVarP(&opt.ConfigFile, "file", "f", "", "Path to configuration YAML file")
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
		if opt.ConfigFile != "" {
			config, err := parseConfigFile(opt.ConfigFile)
			if err != nil {
				log.Fatalf("Error parsing config file: %v", err)
			}
			log.Println("[DEBUG] config file: ", opt.ConfigFile)
			log.Printf("[DEBUG] Parsed config: %+v\n", config)

			// Use the new MergeConfig function
			MergeConfig(&opt, config)
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
			if vm.Name == opt.Vm.Name {
				s.Stop()
				fmt.Println("\033[31m\u2718\033[0m VM " + opt.Vm.Name + " exists. Aborting...")
				os.Exit(1)
			}
		}

		s.Stop()
		fmt.Println("\033[32m\u2714\033[0m Creating VM...")

		// Check Domain Env
		if opt.Domain != "" {
			s.Start()
			s.Suffix = " --domain flag is set... Checking Domain Env..."
			err := domain.NewCloudFlareService().CheckEnv()
			if err != nil {
				s.Stop()
				fmt.Println("\033[31m\u2718\033[0m Error on Domain: ", err)
				os.Exit(1)
			}
		}

		applyFileFound := findFile(opt.ApplyFiles)
		log.Println("[DEBUG] applyFileFound: ", applyFileFound)
		opt.Vm.CloudInitFile = findSingleFile(opt.Vm.CloudInitFile)

		// BEGIN SSH Key
		publicKeyFile, privateKeyFile := getSSHKeyFilePaths(opt.PublicKeyFile)
		log.Println("[DEBUG] publicKeyFile: ", publicKeyFile)
		log.Println("[DEBUG] privateKeyFile: ", privateKeyFile)
		fmt.Println("\033[32m\u2714\033[0m Using Public Key:", publicKeyFile)
		s.Start()
		s.Suffix = " Checking SSH Keys..."
		opt.Vm.SSHKeyID, err = provider.CreateSSHKey(publicKeyFile)
		if err != nil {
			s.Stop()
			fmt.Println("\033[32m\u2718\033[0m Checking SSH Keys...")
			log.Fatalln(err)
		}
		s.Stop()
		fmt.Println("\033[32m\u2714\033[0m Checking SSH Keys... ")
		// END SSH Key

		// BEGIN Set VM Name
		log.Printf("[DEBUG] keyID: %s", opt.Vm.SSHKeyID)
		if opt.Vm.Name == "" {
			if viper.GetString("vm.name") != "" {
				opt.Vm.Name = viper.GetString("vm.name")
			} else {
				opt.Vm.Name = tools.GenerateMachineUniqueName()
			}
		}
		s.Restart()
		s.Suffix = " VM Starting..."
		// END Set VM Name

		vm, err := provider.Deploy(opt.Vm)
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
		log.Println("[DEBUG] ssh port: ", opt.Vm.SSHPort)
		s.Stop()
		// fmt.Println("\033[32m\u2714\033[0m VM Starting...")
		remote := tools.Remote{
			Username:   viper.GetString(cloudProvider + ".vm.username"),
			IPAddress:  vm.IP,
			SSHPort:    opt.Vm.SSHPort,
			PrivateKey: string(privateKey),
			Spinner:    s,
		}

		// BEGIN Domain
		if opt.Domain != "" {
			s.Restart()
			s.Suffix = " Requesting Domain..."
			_, err := domain.NewCloudFlareService().SetRecord(&domain.SetRecordRequest{
				Subdomain: opt.Domain,
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
		if opt.DotEnvFile != "" {
			dotEnvVars, err := tools.ParseDotEnvFile(opt.DotEnvFile)
			if err != nil {
				log.Println(err)
			}
			opt.Variables = append(dotEnvVars, opt.Variables...)
		}

		// Upload Files
		if len(opt.UploadFiles) > 0 {
			ProcessUploadSlice(opt.UploadFiles, remote)
		}

		// BEGIN Apply File
		for i, applyFile := range applyFileFound {
			s.Restart()
			s.Suffix = " Running " + opt.ApplyFiles[i] + " on Remote..."

			err = remote.CopyAndRunRemoteFile(&tools.CopyAndRunRemoteFileConfig{
				File: applyFile,
				Vars: opt.Variables,
			})
			if err != nil {
				log.Println(err)
			}
			s.Stop()
			fmt.Println("\033[32m\u2714\033[0m " + opt.ApplyFiles[i] + " ran on Remote")

		}
		if len(opt.DownloadFiles) > 0 {
			ProcessDownloadSlice(opt.DownloadFiles, remote)
		}
		s.Stop()
		fmt.Println("\033[32m\u2714\033[0m VM Configured...")
	},
}
