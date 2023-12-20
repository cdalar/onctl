package tools

import (
	"encoding/base64"
	"log"
	"os"
	"os/user"
)

func GenerateMachineUniqueName() string {
	userCurrent, err := user.Current()
	if err != nil {
		log.Fatalf(err.Error())
	}
	workingDir, err := os.Getwd()
	if err != nil {
		log.Fatalf(err.Error())
	}

	stringToHash := "onctl-" + userCurrent.Username + workingDir

	return base64.StdEncoding.EncodeToString([]byte(stringToHash))[:7]
}
