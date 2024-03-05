package cmd

import (
	"log"
	"sort"
	"sync"

	"github.com/cdalar/onctl/internal/cloud"
	"github.com/cdalar/onctl/internal/tools"
	"github.com/spf13/cobra"
)

var pingCmd = &cobra.Command{
	Use:   "ping",
	Short: "latency tests on multi cloud providers",
	Run: func(cmd *cobra.Command, args []string) {
		log.Println("[DEBUG] Ping")
		messages := make(chan cloud.Location, 100)
		listOfLocations, err := provider.Locations()
		if err != nil {
			log.Fatalln(err)
		}
		var wg sync.WaitGroup
		for _, location := range listOfLocations {
			wg.Add(1)
			go func(location cloud.Location) {
				defer wg.Done()
				location.Latency, err = tools.Ping(location.Endpoint)
				if err != nil {
					log.Fatalln(err)
				}
				log.Println("[DEBUG]", location.Name, location.Latency)
				messages <- location

			}(location)
		}
		wg.Wait()
		close(messages)
		list := make([]cloud.Location, 0, 100)
		for message := range messages {
			list = append(list, message)
		}

		sort.Slice(list, func(i, j int) bool {
			return list[i].Latency < list[j].Latency
		})

		log.Println("[DEBUG] Location List: ", list)
		tmpl := "LOCATION\tLATENCY\n{{range .}}{{.Name}}\t{{.Latency}}\n{{end}}"
		TabWriter(list, tmpl)
	},
}
