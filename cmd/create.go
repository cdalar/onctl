package cmd

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"time"

	"github.com/briandowns/spinner"
	"github.com/cdalar/onctl/internal/cloud"
	"github.com/cdalar/onctl/internal/domain"
	"github.com/cdalar/onctl/internal/tools"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

// TODO: ? Struct for options. cmdCreateOptions
// TODO: .env file support
// TODO: remove initFile and implement ssh apply structure
// TODO: ? Create Packages with cloud-init, apply Files, Variables. (cloud-init, apply, vars)

type cmdCreateOptions struct {
	PublicKeyFile string
	ApplyFile     []string
	DotEnvFile    string
	Variables     []string
	Vm            cloud.Vm
	Domain        string
}

var (
	opt cmdCreateOptions
)

func init() {
	createCmd.Flags().StringVarP(&opt.PublicKeyFile, "publicKey", "k", "", "Path to publicKey file (default: ~/.ssh/id_rsa))")
	createCmd.Flags().StringSliceVarP(&opt.ApplyFile, "apply-file", "a", []string{}, "bash script file(s) to run on remote")
	createCmd.Flags().StringSliceVarP(&downloadSlice, "download", "d", []string{}, "List of files to download")
	createCmd.Flags().StringVarP(&opt.Vm.Name, "name", "n", "", "vm name")
	createCmd.Flags().IntVarP(&opt.Vm.SSHPort, "ssh-port", "p", 22, "ssh port")
	createCmd.Flags().StringVarP(&opt.Vm.CloudInitFile, "cloud-init", "i", "", "cloud-init file")
	createCmd.Flags().StringVar(&opt.DotEnvFile, "dot-env", "", "dot-env (.env) file")
	createCmd.Flags().StringVar(&opt.Domain, "domain", "", "request a domain name for the VM")
	createCmd.Flags().StringSliceVarP(&opt.Variables, "vars", "e", []string{}, "Environment variables passed to the script")
	createCmd.SetUsageTemplate(createCmd.UsageTemplate() + `
Environment Variables:
  CLOUDFLARE_API_TOKEN  Cloudflare API Token (required for --domain)
  CLOUDFLARE_ZONE_ID    Cloudflare Zone ID (required for --domain)
`)
}

var createCmd = &cobra.Command{
	Use:     "create",
	Aliases: []string{"start", "up", "run"},
	Short:   "Create a VM",
	Long:    `Create a VM with the specified options and run the cloud-init file on the remote.`,
	Example: `  # Create a VM with docker installed and set ssh on port 443
  onctl create -n onctl-test -a docker/docker.sh -i cloud-init/ssh-443.config`,
	Run: func(cmd *cobra.Command, args []string) {
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

		applyFileFound := findFile(opt.ApplyFile)
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

		// BEGIN Apply File
		for i, applyFile := range applyFileFound {
			s.Restart()
			s.Suffix = " Running " + opt.ApplyFile[i] + " on Remote..."

			err = remote.CopyAndRunRemoteFile(&tools.CopyAndRunRemoteFileConfig{
				File: applyFile,
				Vars: opt.Variables,
			})
			if err != nil {
				log.Println(err)
			}
			s.Stop()
			fmt.Println("\033[32m\u2714\033[0m " + opt.ApplyFile[i] + " ran on Remote")

		}
		// TODO go routines
		if len(downloadSlice) > 0 {
			s.Start()
			s.Suffix = " Downloading " + fmt.Sprint(len(downloadSlice)) + " file(s)"
			for _, dfile := range downloadSlice {
				err = remote.DownloadFile(dfile, filepath.Base(dfile))
				if err != nil {
					s.Stop()
					fmt.Println("\033[32m\u2718\033[0m Could not download " + dfile + " from VM: " + vm.Name)
					log.Fatal(err)
				}
				s.Stop()
				fmt.Println("\033[32m\u2714\033[0m " + dfile + " downloaded from VM: " + vm.Name)
			}
			return
		}
		s.Stop()
		fmt.Println("\033[32m\u2714\033[0m VM Configured...")
	},
}
