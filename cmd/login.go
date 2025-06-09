package cmd

import (
	"fmt"
	"log"
	"net/http"
	"os"

	homedir "github.com/mitchellh/go-homedir"
	"github.com/spf13/cobra"
)

var accessKeyToken string

func init() {
	loginCmd.PersistentFlags().StringVar(&accessKeyToken, "token", "", "access token")
}

func createConfigDirIfNotExist() (string, error) {
	// Find home directory.
	home, err := homedir.Dir()
	if err != nil {
		log.Fatal("Problem on home directory")
	}

	okDir := home + "/.onctl"
	_, err = os.Stat(okDir)
	if os.IsNotExist(err) {
		errDir := os.MkdirAll(okDir, 0700)
		if errDir != nil {
			log.Fatal(err)
			return "", err
		}
	}
	return okDir, nil
}

func saveAccessKeyToken(token string) error {

	okDir, err := createConfigDirIfNotExist()
	if err != nil {
		return err
	}
	configFile := okDir + "/credentials"

	// Prepare YAML content
	content := fmt.Sprintf("access_token: %q\n", token)

	// Write to file with 0600 permissions
	file, err := os.OpenFile(configFile, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0600)
	if err != nil {
		fmt.Println("Problem writing credentials")
		return err
	}
	defer file.Close()

	_, err = file.WriteString(content)
	if err != nil {
		log.Println(err)
		return err
	}
	return nil
}

// Listen for the token on localhost
func waitForTokenOnLocalhost(port string) (string, error) {
	var token string
	server := &http.Server{Addr: ":" + port}
	done := make(chan struct{})

	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		token = r.URL.Query().Get("token")
		if token != "" {
			w.Write([]byte("Login successful! You can close this window."))
			close(done)
		} else {
			w.Write([]byte("No token found."))
		}
	})

	go func() {
		_ = server.ListenAndServe()
	}()

	<-done
	server.Close()
	return token, nil
}

var loginCmd = &cobra.Command{
	Use:   "login",
	Short: "Login to onctl.io",
	Long:  `Command line tool to login onctl.io`,
	Run: func(cmd *cobra.Command, args []string) {
		port := "54123" // choose a free port
		// domain := "https://api.onctl.io"  // domain for login
		domain := "http://localhost:8081" // domain for login
		if accessKeyToken != "" {
			err := saveAccessKeyToken(accessKeyToken)
			if err != nil {
				log.Println(err)
			}
			fmt.Println("Token set")
			os.Exit(0)
		}
		// Add redirect_uri to the login URL
		url := domain + "/login"
		fmt.Println("Please login through the browser " + url)
		openbrowser(url)
		token, err := waitForTokenOnLocalhost(port)
		if err != nil {
			log.Println(err)
		}
		if token != "" {
			saveAccessKeyToken(token)
			fmt.Println("Logged in!")
		} else {
			fmt.Println("Failed to get token.")
		}
	},
}
