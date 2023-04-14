package tools

import (
	"log"
	"os"

	"github.com/mitchellh/go-homedir"
)

// CreateConfigDirIfNotExist creates a directory on home directory
// if it does not exist and return the path.
func CreateConfigDirIfNotExist() (string, error) {
	// Find home directory.
	home, err := homedir.Dir()
	if err != nil {
		log.Fatal("Problem on home directory")
	}

	okDir := home + "/.onkube"
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
