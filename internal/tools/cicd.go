package tools

import (
	"log"
	"os/user"
	"strings"

	"github.com/cdalar/onctl/internal/rand"
)

func GenerateMachineUniqueName() string {
	return "onctl-" + rand.String(5)
}

func GenerateUserName() string {
	userCurrent, err := user.Current()
	if err != nil {
		log.Fatalf("%s", err.Error())
	}
	userName := strings.ReplaceAll(userCurrent.Username, "\\", "-")
	userName = strings.ReplaceAll(userName, " ", "-")
	userName = strings.ReplaceAll(userName, "/", "-")
	userName = strings.ReplaceAll(userName, ".", "-")

	return userName

}
