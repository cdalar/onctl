package providerhtz

import (
	"log"
	"os"

	"github.com/hetznercloud/hcloud-go/hcloud"
)

func GetClient() *hcloud.Client {
	token := os.Getenv("HCLOUD_TOKEN")
	if token != "" {
		client := hcloud.NewClient(hcloud.WithToken(token))
		return client
	} else {
		log.Println("HCLOUD_TOKEN is not set")
		os.Exit(1)
	}
	return nil
}
