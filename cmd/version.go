package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
)

// Version of CLI set by go build command
// go build -ldflags "-X main.Version=`git rev-parse HEAD`"
var Version = "Not Set"

var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Print the version number of onctl",
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Println("Version: " + Version)
	},
}
