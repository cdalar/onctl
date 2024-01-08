package cmd

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/exec"
	"runtime"
	"text/tabwriter"
	"text/template"
	"time"

	"github.com/aws/aws-sdk-go/service/ec2"
	"github.com/cdalar/onctl/internal/files"
	"github.com/manifoldco/promptui"
	"github.com/spf13/viper"
	"k8s.io/apimachinery/pkg/util/duration"
)

// TODO decomple viper and use onctlConfig instead
// var onctlConfig map[string]interface{}

func ReadConfig(cloudProvider string) {
	dir, err := os.Getwd()
	if err != nil {
		log.Println(err)
	}
	configFile := dir + "/.onctl/" + cloudProvider + ".yaml"
	log.Println("[DEBUG] Working Directory: " + configFile)
	configFileInfo, err := os.Stat(configFile)
	if err != nil {
		// log.Println(err)
		fmt.Println("No configuration found. Please run `onctl init` first")
		os.Exit(1)
	}

	viper.SetConfigName("onctl")
	viper.AddConfigPath(dir + "/.onctl")
	err = viper.ReadInConfig()
	if err != nil {
		log.Println(err)
	}

	if configFileInfo != nil {
		viper.SetConfigName(cloudProvider)
		err = viper.MergeInConfig()
		if err != nil {
			log.Println(err)
		}
	}

	log.Println("[DEBUG]", viper.AllSettings())
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
		Label: "Select[Yes/No]",
		Items: []string{"Yes", "No"},
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

func findFile(filename string) (filePath string) {
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
		outFile, err := os.CreateTemp("", "onctl")
		if err != nil {
			log.Fatal(err)
		}
		defer outFile.Close()
		_, err = io.Copy(outFile, bytes.NewReader(fileContent))
		if err != nil {
			log.Println(err)
		}
		return outFile.Name()

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
		outFile, err := os.CreateTemp("", "onctl")
		if err != nil {
			log.Fatal(err)
		}
		defer outFile.Close()
		_, err = io.Copy(outFile, resp.Body)
		if err != nil {
			log.Println(err)
		}
		filePath = outFile.Name()
		return filePath
	} else {
		log.Println("[DEBUG]", filename, "not found in templates.onctl.com/")
		fmt.Println("Error: " + filename + " not found in (filesystem, embeded files and templates.onctl.com/)")
		os.Exit(1)
	}
	return ""
}
