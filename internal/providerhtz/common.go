package providerhtz

import (
	"fmt"
	"os"

	"github.com/hetznercloud/hcloud-go/v2/hcloud"
)

func GetClient() (*hcloud.Client, error) {
	token := os.Getenv("HCLOUD_TOKEN")
	if token == "" {
		return nil, fmt.Errorf("HCLOUD_TOKEN is not set")
	}
	client := hcloud.NewClient(hcloud.WithToken(token))
	return client, nil
}
