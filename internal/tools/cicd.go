package tools

import (
	"log"
	"os"
	"os/user"
	"strings"
)

func GenerateMachineUniqueName() string {
	userCurrent, err := user.Current()
	if err != nil {
		log.Fatalf(err.Error())
	}
	wd, err := os.Getwd()
	if err != nil {
		log.Fatalf(err.Error())
	}

	wd = strings.ReplaceAll(wd, "/", "-")
	wd = strings.ReplaceAll(wd, "\\", "-")
	wd = strings.ReplaceAll(wd, " ", "-")
	userName := strings.ReplaceAll(userCurrent.Username, "\\", "-")
	stringToHash := userName + wd[len(wd)-10:]

	return "onctl-" + stringToHash
}

func GenerateUserName() string {
	userCurrent, err := user.Current()
	if err != nil {
		log.Fatalf(err.Error())
	}
	userName := strings.ReplaceAll(userCurrent.Username, "\\", "-")
	userName = strings.ReplaceAll(userName, " ", "-")
	userName = strings.ReplaceAll(userName, "/", "-")
	userName = strings.ReplaceAll(userName, ".", "-")

	return userName

}
