package providerhtz

import (
	"errors"
	"os"

	"github.com/hetznercloud/hcloud-go/v2/hcloud"
)

// GetClient builds a Hetzner client from HCLOUD_TOKEN, or fails when it
// isn't set.
func GetClient() (*hcloud.Client, error) {
	token := os.Getenv("HCLOUD_TOKEN")
	if token == "" {
		return nil, errors.New("HCLOUD_TOKEN is not set")
	}
	return hcloud.NewClient(hcloud.WithToken(token)), nil
}
