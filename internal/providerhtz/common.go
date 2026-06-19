package providerhtz

import (
	"log"
	"os"

	"github.com/hetznercloud/hcloud-go/v2/hcloud"
)

func GetClient() *hcloud.Client {
	token := os.Getenv("HCLOUD_TOKEN")
	if token != "" {
		client := hcloud.NewClient(hcloud.WithToken(token))
		return client
	}
	log.Printf("[DEBUG] HCLOUD_TOKEN is not set")
	return nil
}
