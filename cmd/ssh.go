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
)

var (
	port          int
	apply         []string
	downloadSlice []string
	uploadSlice   []string
	key           string
)

func init() {
	sshCmd.Flags().StringVarP(&key, "key", "k", "", "Path to privateKey file (default: ~/.ssh/id_rsa))")
	sshCmd.Flags().IntVarP(&port, "port", "p", 22, "ssh port")
	sshCmd.Flags().StringSliceVarP(&apply, "apply-file", "a", []string{}, "bash script file(s) to run on remote")
	sshCmd.Flags().StringSliceVarP(&downloadSlice, "download", "d", []string{}, "List of files to download")
	sshCmd.Flags().StringSliceVarP(&uploadSlice, "upload", "u", []string{}, "List of files to upload")
	sshCmd.Flags().StringVar(&opt.DotEnvFile, "dot-env", "", "dot-env (.env) file")
	sshCmd.Flags().StringSliceVarP(&opt.Variables, "vars", "e", []string{}, "Environment variables passed to the script")
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
		s := spinner.New(spinner.CharSets[9], 100*time.Millisecond) // Build our new spinner
		applyFileFound := findFile(apply)
		log.Println("[DEBUG] args: ", args)

		if len(args) == 0 {
			fmt.Println("Please provide a VM id")
			return
		}
		log.Println("[DEBUG] port: ", port)
		log.Println("[DEBUG] filename: ", applyFileFound)
		log.Println("[DEBUG] key: ", key)
		_, privateKeyFile := getSSHKeyFilePaths(key)
		log.Println("[DEBUG] privateKeyFile: ", privateKeyFile)

		privateKey, err := os.ReadFile(privateKeyFile)
		if err != nil {
			log.Fatal(err)
		}
		vm, err := provider.GetByName(args[0])
		if err != nil {
			log.Fatalln(err)
		}
		remote := tools.Remote{
			Username:   viper.GetString(cloudProvider + ".vm.username"),
			IPAddress:  vm.IP,
			SSHPort:    port,
			PrivateKey: string(privateKey),
			Spinner:    s,
		}

		if opt.DotEnvFile != "" {
			dotEnvVars, err := tools.ParseDotEnvFile(opt.DotEnvFile)
			if err != nil {
				log.Println(err)
			}
			opt.Variables = append(dotEnvVars, opt.Variables...)
		}

		if len(uploadSlice) > 0 {
			ProcessUploadSlice(uploadSlice, remote)
		}

		// BEGIN Apply File
		for i, applyFile := range applyFileFound {
			s.Restart()
			s.Suffix = " Running " + apply[i] + " on Remote..."

			err = remote.CopyAndRunRemoteFile(&tools.CopyAndRunRemoteFileConfig{
				File: applyFile,
				Vars: opt.Variables,
			})
			if err != nil {
				log.Println(err)
			}
			s.Stop()
			fmt.Println("\033[32m\u2714\033[0m " + apply[i] + " ran on Remote")

		}
		// END Apply File

		if len(downloadSlice) > 0 {
			ProcessDownloadSlice(downloadSlice, remote)
		}
		if len(applyFileFound) == 0 && len(downloadSlice) == 0 && len(uploadSlice) == 0 {
			provider.SSHInto(args[0], port, privateKeyFile)
		}
	},
}
