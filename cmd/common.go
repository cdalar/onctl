package cmd

import (
	"encoding/json"
	"fmt"
	"github.com/cdalar/onctl/internal/ui"
	"github.com/cdalar/onctl/pkg/box"
	"golang.org/x/term"
	"io"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"text/tabwriter"
	"text/template"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/ec2/types"
	"github.com/cdalar/onctl/internal/files"
	"github.com/cdalar/onctl/pkg/cloud"
	"github.com/gofrs/uuid/v5"
	"github.com/manifoldco/promptui"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"k8s.io/apimachinery/pkg/util/duration"
)

// TODO decouple viper and use onctlConfig instead
// var onctlConfig map[string]interface{}

// ensureCursorVisible ensures terminal cursor is visible.
// This should be called before log.Fatal* calls in command functions that use spinners.
// Spinners hide the cursor when started, and since log.Fatal* calls os.Exit() which
// does not run deferred functions, the cursor must be explicitly restored before fatal errors.
// It can also be used with defer for panic recovery, though defer does not run on os.Exit().
func ensureCursorVisible() {
	fmt.Print("\033[?25h") // ANSI escape code to show cursor
}

func GenerateIDToken() uuid.UUID {
	u1, err := uuid.NewV4()
	if err != nil {
		log.Fatalf("failed to generate ID Token: %v", err)
	}
	log.Printf("[DEBUG] ID Token generated %v", u1)
	return u1
}

// flagConfigFile is the optional --config/-c override pointing at a specific
// onctl.yaml (or similarly named) file, bypassing the .onctl directory
// lookup in resolveConfigDir. Set by rootCmd's persistent flag in root.go.
var flagConfigFile string

// resolveConfigDir finds the .onctl directory to use: an explicit
// --config/-c file's directory takes precedence, then the current working
// directory's .onctl, then the one in the user's home directory.
func resolveConfigDir() (string, error) {
	if flagConfigFile != "" {
		if _, err := os.Stat(flagConfigFile); err != nil {
			return "", fmt.Errorf("config file %s: %w", flagConfigFile, err)
		}
		return filepath.Dir(flagConfigFile), nil
	}

	dir, err := os.Getwd()
	if err != nil {
		return "", fmt.Errorf("failed to get working directory: %v", err)
	}

	localConfigPath := filepath.Join(dir, ".onctl")
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("failed to get home directory: %v", err)
	}
	homeConfigPath := filepath.Join(homeDir, ".onctl")

	log.Println("[DEBUG] Local Config Path:", localConfigPath)
	log.Println("[DEBUG] Home Config Path:", homeConfigPath)
	if _, err := os.Stat(localConfigPath); err == nil {
		log.Println("[DEBUG] Using local config directory")
		return localConfigPath, nil
	}
	if _, err := os.Stat(homeConfigPath); err == nil {
		log.Println("[DEBUG] Using home config directory")
		return homeConfigPath, nil
	}
	return "", fmt.Errorf("no configuration directory found in current directory or home directory. Please run `onctl init` first")
}

func ReadConfig() error {
	if flagConfigFile != "" {
		viper.SetConfigFile(flagConfigFile)
		if err := viper.ReadInConfig(); err != nil {
			return fmt.Errorf("failed to read config file %s: %w", flagConfigFile, err)
		}
		warnLegacyProviderConfigFiles(filepath.Dir(flagConfigFile))
		log.Println("[DEBUG] Loaded Settings:", viper.AllSettings())
		return nil
	}

	configDir, err := resolveConfigDir()
	if err != nil {
		return err
	}

	viper.SetConfigName("onctl") // single config file holds settings for every provider
	viper.AddConfigPath(configDir)

	if err := viper.ReadInConfig(); err != nil {
		return fmt.Errorf("failed to read onctl.yaml in %s: %w", configDir, err)
	}

	warnLegacyProviderConfigFiles(configDir)

	log.Println("[DEBUG] Loaded Settings:", viper.AllSettings())
	return nil
}

func getNameFromTags(tags []types.Tag) string {
	for _, v := range tags {
		if *v.Key == "Name" {
			return *v.Value
		}
	}
	return ""
}

func durationFromCreatedAt(createdAt time.Time) string {
	return duration.HumanDuration(time.Since(createdAt))
}

func TabWriter(res interface{}, tmpl string) { //nolint
	var funcs = template.FuncMap{"getNameFromTags": getNameFromTags}
	var funcs2 = template.FuncMap{"durationFromCreatedAt": durationFromCreatedAt}
	w := tabwriter.NewWriter(os.Stdout, 2, 2, 3, ' ', 0)
	tmp, err := template.New("test").Funcs(funcs).Funcs(funcs2).Parse(tmpl)
	if err != nil {
		log.Println(err)
	}
	err = tmp.Execute(w, res)
	if err != nil {
		log.Println(err)
	}
	if err := w.Flush(); err != nil {
		log.Println(err)
	}
}
func PrettyPrint(v interface{}) (err error) {
	b, err := json.MarshalIndent(v, "", "  ")
	if err == nil {
		fmt.Println(string(b))
	}
	return
}

func yesNo() bool {
	prompt := promptui.Select{
		Label:     "Please confirm [y/N]",
		Items:     []string{"Yes", "No"},
		CursorPos: 1,
	}
	_, result, err := prompt.Run()
	if err != nil {
		log.Fatalf("Prompt failed %v\n", err)
	}
	return result == "Yes"
}

//lint:ignore U1000 will use this function in the future
func openbrowser(url string) {
	var err error

	switch runtime.GOOS {
	case "linux":
		err = exec.Command("xdg-open", url).Start()
	case "windows":
		err = exec.Command("rundll32", "url.dll,FileProtocolHandler", url).Start()
	case "darwin":
		err = exec.Command("open", url).Start()
	default:
		err = fmt.Errorf("unsupported platform")
	}
	if err != nil {
		fmt.Println(err)
	}

}

func findFile(files []string) []string {
	var filePaths []string
	for _, file := range files {
		filePath := findSingleFile(file)
		filePaths = append(filePaths, filePath)
	}
	return filePaths
}

func findSingleFile(filename string) (filePath string) {
	if filename == "" {
		return ""
	}

	// Checking file in filesystem
	_, err := os.Stat(filename)
	if err == nil { // file found in filesystem
		return filename
	} else {
		log.Println("[DEBUG]", filename, "file not found in filesystem, trying to find in embeded files")
	}

	// file not found in filesystem, trying to find in embeded files
	fileContent, err := files.EmbededFiles.ReadFile(filename)
	if err == nil {
		log.Println("[DEBUG]", filename, "file found in embeded files")

		dir, err := os.MkdirTemp("", "onctl")
		if err != nil {
			log.Fatal(err)
		}

		file := filepath.Join(dir, filename)
		if err := os.WriteFile(file, fileContent, 0666); err != nil {
			log.Fatal(err)
		}

		return file

	} else {
		log.Println("[DEBUG]", filename, "not found in embeded files, trying to find in templates.onctl.com/")
	}

	// file not found in embeded files, trying to find in templates.onctl.com/
	if filename[0:4] != "http" {
		filename = "https://templates.onctl.com/" + filename
	}

	resp, err := http.Get(filename)
	if err == nil && resp.StatusCode == 200 {
		log.Println("[DEBUG]", filename, "file found in templates.onctl.com/")

		defer func() {
			if err := resp.Body.Close(); err != nil {
				log.Printf("Failed to close response body: %v", err)
			}
		}()
		dir, err := os.MkdirTemp("", "onctl")
		if err != nil {
			log.Fatal(err)
		}

		fileBaseName := filepath.Base(filename)
		filePath := filepath.Join(dir, fileBaseName)
		fileContent, err := io.ReadAll(resp.Body)
		if err != nil {
			log.Fatal(err)
		}
		if err := os.WriteFile(filePath, fileContent, 0666); err != nil {
			log.Fatal(err)
		}

		return filePath
	} else {
		log.Println("[DEBUG]", filename, "not found in templates.onctl.com/")
		fmt.Println("Error: " + filename + " not found in (filesystem, embeded files and templates.onctl.com/)")
		os.Exit(1)
	}
	return ""
}

// getSSHKeyFilePaths is box.KeyFiles: filename's key pair, or the
// configured default's (falling back to another key type on disk).
func getSSHKeyFilePaths(filename string) (publicKeyFile, privateKeyFile string) {
	return box.KeyFiles(filename)
}

func dirSize(path string) (int64, error) {
	var size int64
	err := filepath.WalkDir(path, func(_ string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() {
			info, err := d.Info()
			if err != nil {
				return err
			}
			size += info.Size()
		}
		return nil
	})
	return size, err
}

func ProcessUploadSlice(uploadSlice []string, conn *box.Conn, s *ui.Spinner) {
	if len(uploadSlice) == 0 {
		return
	}

	spinnerWasActive := s != nil && s.Active()
	if spinnerWasActive {
		s.Stop()
	}

	for _, dfile := range uploadSlice {
		var localFile, remoteFile string
		if strings.Contains(dfile, ":") {
			parts := strings.SplitN(dfile, ":", 2)
			localFile = parts[0]
			remoteFile = parts[1]
		} else {
			localFile = dfile
			remoteFile = filepath.Base(dfile)
		}

		log.Println("[DEBUG] localFile: " + localFile)
		log.Println("[DEBUG] remoteFile: " + remoteFile)

		fileInfo, err := os.Stat(localFile)
		if err != nil {
			log.Printf("[ERROR] Failed to stat %s: %v", localFile, err)
			continue
		}

		var totalBytes int64
		if fileInfo.IsDir() {
			totalBytes, err = dirSize(localFile)
			if err != nil {
				log.Printf("[ERROR] Failed to calculate size of %s: %v", localFile, err)
				continue
			}
		} else {
			totalBytes = fileInfo.Size()
		}

		if fileInfo.IsDir() {
			fmt.Printf("Uploading directory: %s -> %s (%.1f MB)\n", localFile, remoteFile, float64(totalBytes)/(1024*1024))
		} else {
			fmt.Printf("Uploading file: %s -> %s (%.1f MB)\n", localFile, remoteFile, float64(totalBytes)/(1024*1024))
		}

		startTime := time.Now()
		progressCallback := func(current, total int64) {
			if total == 0 {
				return
			}

			percentage := float64(current) / float64(total) * 100
			progressBarWidth := 20
			filled := int(float64(progressBarWidth) * float64(current) / float64(total))
			if filled > progressBarWidth {
				filled = progressBarWidth
			}
			bar := strings.Repeat("=", filled) + strings.Repeat(".", progressBarWidth-filled)

			var mbPerSecond float64
			elapsed := time.Since(startTime)
			if elapsed.Seconds() > 0 {
				mbPerSecond = float64(current) / elapsed.Seconds() / (1024 * 1024)
			}

			fmt.Printf("\r[%s] %5.1f%% (%.1f/%.1f MB) %.1f MB/s", bar, percentage, float64(current)/(1024*1024), float64(total)/(1024*1024), mbPerSecond)
		}

		if fileInfo.IsDir() {
			err = conn.Upload(localFile, remoteFile, progressCallback)
			if err != nil {
				fmt.Print("\n")
				log.Printf("[ERROR] Failed to upload directory %s: %v", localFile, err)
				continue
			}
			progressCallback(totalBytes, totalBytes)
			fmt.Printf("\n\033[32m\u2714\033[0m Uploaded directory %s\n", remoteFile)
		} else {
			err = conn.Upload(localFile, remoteFile, progressCallback)
			if err != nil {
				fmt.Print("\n")
				log.Printf("[ERROR] Failed to upload %s: %v", localFile, err)
				continue
			}
			progressCallback(totalBytes, totalBytes)
			fmt.Printf("\n\033[32m\u2714\033[0m Uploaded %s\n", remoteFile)
		}
	}

	if spinnerWasActive {
		s.Restart()
	}
}

func ProcessDownloadSlice(downloadSlice []string, conn *box.Conn) {
	if len(downloadSlice) > 0 {
		// Connect once up front: the downloads below share the connection.
		if err := conn.Open(); err != nil {
			log.Printf("[ERROR] Failed to connect for downloads: %v", err)
			return
		}
		var wg sync.WaitGroup
		for _, dfile := range downloadSlice {
			wg.Add(1)
			go func(dfile string) {
				defer wg.Done()

				var remoteFile, localFile string
				// Split by colon to determine if a rename is required
				if strings.Contains(dfile, ":") {
					parts := strings.SplitN(dfile, ":", 2)
					remoteFile = parts[0]
					localFile = parts[1]
				} else {
					remoteFile = dfile
					localFile = filepath.Base(dfile)
				}

				log.Printf("Downloading file: %s -> %s", remoteFile, localFile)

				err := conn.Download(remoteFile, localFile)
				if err != nil {
					log.Printf("[ERROR] Failed to download %s: %v", remoteFile, err)
				}
			}(dfile)
		}
		wg.Wait() // Wait for all goroutines to finish
	}
}

// MergeConfig merges the options from the configuration file into the command-line options.
// Command-line options take precedence over configuration file options.
func MergeConfig(opt *cmdCreateOptions, config *cmdCreateOptions) {
	if opt.PublicKeyFile == "" && config.PublicKeyFile != "" {
		opt.PublicKeyFile = config.PublicKeyFile
	}
	if len(opt.ApplyFiles) == 0 && len(config.ApplyFiles) > 0 {
		opt.ApplyFiles = append(opt.ApplyFiles, config.ApplyFiles...)
	}
	if opt.DotEnvFile == "" && config.DotEnvFile != "" {
		opt.DotEnvFile = config.DotEnvFile
	}
	if len(opt.Variables) == 0 && len(config.Variables) > 0 {
		opt.Variables = append(opt.Variables, config.Variables...)
	}
	if opt.Vm.Name == "" && config.Vm.Name != "" {
		opt.Vm.Name = config.Vm.Name
	}
	if opt.Vm.Type == "" && config.Vm.Type != "" {
		opt.Vm.Type = config.Vm.Type
	}
	if opt.Vm.Image == "" && config.Vm.Image != "" {
		opt.Vm.Image = config.Vm.Image
	}
	if opt.Vm.SSHPort == 22 && config.Vm.SSHPort != 0 { // Default SSH port is 22
		opt.Vm.SSHPort = config.Vm.SSHPort
	}
	if opt.Vm.CloudInitFile == "" && config.Vm.CloudInitFile != "" {
		opt.Vm.CloudInitFile = config.Vm.CloudInitFile
	}
	if opt.Domain == "" && config.Domain != "" {
		opt.Domain = config.Domain
	}
	if len(opt.DownloadFiles) == 0 && len(config.DownloadFiles) > 0 {
		opt.DownloadFiles = append(opt.DownloadFiles, config.DownloadFiles...)
	}
	if len(opt.UploadFiles) == 0 && len(config.UploadFiles) > 0 {
		opt.UploadFiles = append(opt.UploadFiles, config.UploadFiles...)
	}

	log.Println("[DEBUG] Merged options: ", opt)
}

// connect is box.Connect for the CLI: an encrypted key's passphrase is
// read from the terminal and WaitForSSH's hints printed, with s (when
// set) paused for each. keyFlag is -k's value (empty: an imported host's
// own key, else the configured one); port 0 is an imported host's own.
func connect(vm cloud.Vm, keyFlag string, port int, s *ui.Spinner) (*box.Conn, error) {
	paused := func(f func()) {
		active := s != nil && s.Active()
		if active {
			s.Stop()
		}
		f()
		if active {
			s.Restart()
		}
	}
	return boxClient().Connect(vm, box.ConnectOptions{
		KeyFile: keyFlag,
		Port:    port,
		Passphrase: func() (passphrase string, err error) {
			paused(func() {
				fmt.Print("Enter passphrase for private key:")
				var b []byte
				b, err = term.ReadPassword(int(os.Stdin.Fd()))
				fmt.Println()
				passphrase = string(b)
			})
			return passphrase, err
		},
		Hint: func(hint string) {
			paused(func() { fmt.Fprintln(os.Stderr, hint) })
		},
	})
}

// cloudInitTimeout is --cloud-init-timeout (vm.cloud-init.timeout).
func cloudInitTimeout() time.Duration {
	d, err := time.ParseDuration(viper.GetString("vm.cloud-init.timeout"))
	if err != nil {
		log.Fatalf("Invalid timeout value: %v", err)
	}
	return d
}

// completeVMName completes a command's first argument with the provider's
// VM names.
func completeVMName(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	if len(args) > 0 {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}
	ensureProvider()
	if provider == nil {
		return nil, cobra.ShellCompDirectiveError
	}
	vms, err := boxClient().Servers()
	if err != nil {
		return nil, cobra.ShellCompDirectiveError
	}
	var names []string
	for _, vm := range vms.List {
		names = append(names, vm.Name)
	}
	return names, cobra.ShellCompDirectiveNoFileComp
}
