package cmd

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"os/exec"
	"runtime"
	"text/tabwriter"
	"text/template"
	"time"

	"github.com/aws/aws-sdk-go/service/ec2"
	"github.com/manifoldco/promptui"
	"github.com/spf13/viper"
	"k8s.io/apimachinery/pkg/util/duration"
)

// TODO decomple viper and use onctlConfig instead
// var onctlConfig map[string]interface{}

func ReadConfig(filename string) {
	dir, err := os.Getwd()
	if err != nil {
		log.Println(err)
	}
	viper.SetConfigName("onctl")
	viper.AddConfigPath(dir + "/.onctl")
	err = viper.ReadInConfig()
	if err != nil {
		fmt.Println(err)
	}

	if _, err := os.Stat(dir + "/.onctl/" + filename + ".yaml"); err == nil {
		viper.SetConfigName(filename)
		err = viper.MergeInConfig()
		if err != nil {
			log.Println(err)
		}
	}

	log.Println("[DEBUG]", viper.AllSettings())
	// onctlConfig = viper.AllSettings()
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
