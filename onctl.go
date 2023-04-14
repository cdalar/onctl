package main

import (
	"cdalar/onctl/cmd"
	"log"
	"os"

	"github.com/hashicorp/logutils"
)

func main() {
	filter := &logutils.LevelFilter{
		Levels:   []logutils.LogLevel{"DEBUG", "WARN", "ERROR"},
		MinLevel: logutils.LogLevel("WARN"),
		Writer:   os.Stderr,
	}
	if os.Getenv("ONCTL_LOG") != "" {
		filter.MinLevel = logutils.LogLevel(os.Getenv("ONCTL_LOG"))
		log.SetFlags(log.Lshortfile)
	}
	log.SetOutput(filter)
	err := cmd.Execute()
	if err != nil {
		log.Println(err)
	}
}
