package providerpxmx

import (
	"crypto/tls"
	"fmt"
	"os"

	pxapi "github.com/Telmate/proxmox-api-go/proxmox"
	"github.com/spf13/viper"
)

func GetClient() (*pxapi.Client, error) {
	apiURL := os.Getenv("PROXMOX_API_URL")
	tokenID := os.Getenv("PROXMOX_TOKEN_ID")
	secret := os.Getenv("PROXMOX_SECRET")

	if apiURL == "" || tokenID == "" || secret == "" {
		return nil, fmt.Errorf("PROXMOX_API_URL, PROXMOX_TOKEN_ID, and PROXMOX_SECRET must be set")
	}

	insecureSkipVerify := viper.GetBool("proxmox.insecure")
	tlsConfig := &tls.Config{
		InsecureSkipVerify: insecureSkipVerify,
	}

	client, err := pxapi.NewClient(apiURL, nil, "", tlsConfig, "", 300)
	if err != nil {
		return nil, fmt.Errorf("failed to create Proxmox client: %w", err)
	}

	client.SetAPIToken(tokenID, secret)

	return client, nil
}
