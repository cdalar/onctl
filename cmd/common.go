package cmd

import (
	"encoding/json"
	"errors"
	"fmt"
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

	"github.com/aws/aws-sdk-go/service/ec2"
	"github.com/cdalar/onctl/internal/files"
	"github.com/cdalar/onctl/internal/tools"
	"github.com/gofrs/uuid/v5"
	"github.com/golang-jwt/jwt/v4"
	"github.com/manifoldco/promptui"
	homedir "github.com/mitchellh/go-homedir"
	"github.com/spf13/viper"
	"gopkg.in/yaml.v2"
	"k8s.io/apimachinery/pkg/util/duration"
)

// TODO decouple viper and use onctlConfig instead
// var onctlConfig map[string]interface{}

func GenerateIDToken() uuid.UUID {
	u1, err := uuid.NewV4()
	if err != nil {
		log.Fatalf("failed to generate ID Token: %v", err)
	}
	log.Printf("[DEBUG] ID Token generated %v", u1)
	return u1
}

func ReadConfig(cloudProvider string) error {
	// Check current working directory
	dir, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("failed to get working directory: %v", err)
	}

	localConfigPath := filepath.Join(dir, ".onctl")
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("failed to get home directory: %v", err)
	}
	homeConfigPath := filepath.Join(homeDir, ".onctl")

	log.Println("[DEBUG] Local Config Path:", localConfigPath)
	log.Println("[DEBUG] Home Config Path:", homeConfigPath)
	// Determine which directory to use
	var configDir string
	if _, err := os.Stat(localConfigPath); err == nil {
		configDir = localConfigPath
		log.Println("[DEBUG] Using local config directory")
	} else if _, err := os.Stat(homeConfigPath); err == nil {
		configDir = homeConfigPath
		log.Println("[DEBUG] Using home config directory")
	} else {
		return fmt.Errorf("no configuration directory found in current directory or home directory. Please run `onctl init` first")
	}

	// Set paths for general and cloud provider-specific config
	configFile := filepath.Join(configDir, cloudProvider+".yaml")
	log.Println("[DEBUG] Config File Path:", configFile)

	if _, err := os.Stat(configFile); os.IsNotExist(err) {
		return fmt.Errorf("no configuration file found for %s in %s", cloudProvider, configDir)
	}

	viper.SetConfigName("onctl") // General config
	viper.AddConfigPath(configDir)

	if err := viper.ReadInConfig(); err != nil {
		log.Printf("Failed to read general config: %v", err)
	}

	viper.SetConfigName(cloudProvider) // Specific config
	if err := viper.MergeInConfig(); err != nil {
		log.Printf("Failed to merge cloud provider config: %v", err)
	}

	log.Println("[DEBUG] Loaded Settings:", viper.AllSettings())
	return nil
}

func getNameFromTags(tags []*ec2.Tag) string {
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
	w.Flush()
}
func PrettyPrint(v interface{}) (err error) {
	b, err := json.MarshalIndent(v, "", "  ")
	if err == nil {
		fmt.Println(string(b))
	}
	return
}

//lint:ignore U1000 will use this function in the future
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

		defer resp.Body.Close()
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

func getSSHKeyFilePaths(filename string) (publicKeyFile, privateKeyFile string) {

	home, err := os.UserHomeDir()
	if err != nil {
		log.Println(err)
	}

	if filename == "" {
		publicKeyFile = viper.GetString("ssh.publicKey")
		privateKeyFile = viper.GetString("ssh.privateKey")
	} else {
		// check if filename has .pub extension
		if filename[len(filename)-4:] == ".pub" {
			publicKeyFile = filename
			privateKeyFile = filename[:len(filename)-4]
		} else {
			privateKeyFile = filename
			publicKeyFile = filename + ".pub"
		}
	}

	// change ~ char with home directory
	publicKeyFile = strings.Replace(publicKeyFile, "~", home, 1)
	privateKeyFile = strings.Replace(privateKeyFile, "~", home, 1)

	log.Println("[DEBUG] publicKeyFile:", publicKeyFile)
	log.Println("[DEBUG] privateKeyFile:", privateKeyFile)
	if _, err := os.Stat(publicKeyFile); err != nil {
		log.Println("[DEBUG]", publicKeyFile, "Public key file not found")
	}

	if _, err := os.Stat(privateKeyFile); err != nil {
		log.Println("[DEBUG]", privateKeyFile, "Private key file not found")
	}

	return publicKeyFile, privateKeyFile
}

func ProcessUploadSlice(uploadSlice []string, remote tools.Remote) {
	if len(uploadSlice) > 0 {
		var wg sync.WaitGroup
		for _, dfile := range uploadSlice {
			wg.Add(1)
			go func(dfile string) {
				defer wg.Done()

				var localFile, remoteFile string
				// Split by colon to determine if a rename is required
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

				fmt.Printf("Uploading file: %s -> %s\n", localFile, remoteFile)

				err := remote.SSHCopyFile(localFile, remoteFile)
				if err != nil {
					log.Printf("[ERROR] Failed to upload %s: %v", localFile, err)
				}
			}(dfile)
		}
		wg.Wait() // Wait for all goroutines to finish
	}
}

func ProcessDownloadSlice(downloadSlice []string, remote tools.Remote) {
	if len(downloadSlice) > 0 {
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

				err := remote.DownloadFile(remoteFile, localFile)
				if err != nil {
					log.Printf("[ERROR] Failed to download %s: %v", remoteFile, err)
				}
			}(dfile)
		}
		wg.Wait() // Wait for all goroutines to finish
	}
}

type CredentialsConfig struct {
	AccessToken string `yaml:"access_token"`
}

var ErrTokenExpired = errors.New("access token is expired")

// function to get access token from home directory and validate expiration
func getAccessToken() (string, error) {
	home, err := homedir.Dir()
	if err != nil {
		return "", fmt.Errorf("problem finding home directory: %v", err)
	}

	configFile := home + "/.onctl/credentials"
	content, err := os.ReadFile(configFile)
	if err != nil {
		return "", fmt.Errorf("problem reading credentials file: %v", err)
	}

	var creds CredentialsConfig
	err = yaml.Unmarshal(content, &creds)
	if err != nil {
		return "", fmt.Errorf("problem unmarshalling credentials file: %v", err)
	}

	token := creds.AccessToken

	// Parse the JWT token without verifying the signature
	parsedToken, _, err := jwt.NewParser().ParseUnverified(token, jwt.MapClaims{})
	if err != nil {
		return "", fmt.Errorf("problem parsing JWT token: %v", err)
	}

	// Extract claims and check expiration
	claims, ok := parsedToken.Claims.(jwt.MapClaims)
	if !ok {
		return "", errors.New("invalid JWT claims format")
	}

	exp, ok := claims["exp"].(float64)
	if !ok {
		return "", errors.New("expiration claim not found in JWT")
	}

	// Check if the token is expired
	if time.Unix(int64(exp), 0).Before(time.Now()) {
		return "", ErrTokenExpired
	}

	return token, nil
}
