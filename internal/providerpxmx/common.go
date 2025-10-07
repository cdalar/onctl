package providerpxmx

import (
	"crypto/tls"
	"log"
	"os"

	pxapi "github.com/Telmate/proxmox-api-go/proxmox"
)

func GetClient() *pxapi.Client {
	apiURL := os.Getenv("PROXMOX_API_URL")
	tokenID := os.Getenv("PROXMOX_TOKEN_ID")
	secret := os.Getenv("PROXMOX_SECRET")

	if apiURL == "" || tokenID == "" || secret == "" {
		log.Println("PROXMOX_API_URL, PROXMOX_TOKEN_ID, and PROXMOX_SECRET must be set")
		os.Exit(1)
	}

	tlsConfig := &tls.Config{
		InsecureSkipVerify: true, // For self-signed certificates
	}

	client, err := pxapi.NewClient(apiURL, nil, "", tlsConfig, "", 300)
	if err != nil {
		log.Fatalln("Failed to create Proxmox client:", err)
	}

	client.SetAPIToken(tokenID, secret)

	return client
}
