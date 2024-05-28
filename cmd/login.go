package cmd

import (
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"time"

	"github.com/spf13/cobra"
)

var (
	loginURL = "https://login.onctl.com"
)
var loginCmd = &cobra.Command{
	Use:   "login",
	Short: "login",
	Run: func(cmd *cobra.Command, args []string) {
		// client_test()
		IDToken := GenerateIDToken().String()
		log.Println("[DEBUG] ID Token: ", IDToken)
		// viper.Set("accessKey", accessKey)
		// viper.SetConfigName("onctl")
		// viper.WriteConfig()
		login(IDToken)
	},
}

func login(IDToken string) {
	log.Println("login called")
	url := loginURL + "/login/" + IDToken
	fmt.Println("Please login through the browser to continue")
	openbrowser(url)
	_, err := keepChecking(loginURL, IDToken)
	if err != nil {
		log.Println(err)
	}

}

func checkToken(domain string, IDToken string) (bool, error) {
	resp, err := http.Get(domain + "/gettoken/" + IDToken)
	if err != nil {
		log.Fatalln(err)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		log.Fatalln(err)
	}
	//Convert the body to type string
	token := string(body)
	if token != "" {
		// err := saveAccessKeyToken(token)
		// if err != nil {
		// 	log.Println(err)
		// }
		return true, nil
	}
	return false, err
}

func keepChecking(domain string, IDToken string) (bool, error) {
	timeout := time.After(3 * time.Minute)
	ticker := time.Tick(5 * time.Second)
	// Keep trying until we're timed out or get a result/error
	for {
		select {
		// Got a timeout! fail with a timeout error
		case <-timeout:
			return false, errors.New("timed out")
		// Got a tick, we should check on checkSomething()
		case <-ticker:
			ok, err := checkToken(domain, IDToken)
			if err != nil {
				// We may return, or ignore the error
				return false, err
				// checkSomething() done! let's return
			} else if ok {
				fmt.Println("logged in")
				return true, nil
			}
			// checkSomething() isn't done yet, but it didn't fail either, let's try again
		}
	}
}
