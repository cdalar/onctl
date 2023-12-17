package cmd

import (
	"fmt"
	"log"
	"os"

	"github.com/cdalar/onctl/internal/files"

	"github.com/spf13/cobra"
)

var initCmd = &cobra.Command{
	Use:   "init",
	Short: "init onctl environment",
	Run: func(cmd *cobra.Command, args []string) {
		if _, err := os.Stat(".onctl"); os.IsNotExist(err) {
			if err := os.Mkdir(".onctl", os.ModePerm); err != nil {
				log.Fatal(err)
			}
			embedDir, err := files.EmbededFiles.ReadDir("init")
			if err != nil {
				log.Fatal(err)
			}
			for _, configFile := range embedDir {
				log.Println("[DEBUG] initFile:" + configFile.Name())
				eFile, _ := files.EmbededFiles.ReadFile("init/" + configFile.Name())
				err = os.WriteFile(".onctl/"+configFile.Name(), eFile, 0644)
				if err != nil {
					log.Fatal(err)
				}
			}
			fmt.Println("onctl environment initialized")
		} else {
			fmt.Println("onctl environment already initialized")
		}
	},
}
