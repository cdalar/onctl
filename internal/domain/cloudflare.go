package domain

import (
	"context"
	"errors"
	"log"
	"os"

	"github.com/cloudflare/cloudflare-go"
)

type CloudFlareService struct {
	CLOUDFLARE_API_TOKEN string
}

func NewCloudFlareService() *CloudFlareService {
	apiToken := os.Getenv("CLOUDFLARE_API_TOKEN")
	if apiToken == "" {
		log.Fatal("CLOUDFLARE_API_TOKEN is not set")
	}

	return &CloudFlareService{
		CLOUDFLARE_API_TOKEN: apiToken,
	}
}

func (c *CloudFlareService) CheckEnv() error {
	apiToken := os.Getenv("CLOUDFLARE_API_TOKEN")
	if apiToken == "" {
		// log.Println("CLOUDFLARE_API_TOKEN is not set")
		return errors.New("CLOUDFLARE_API_TOKEN is not set")
	}
	zoneID := os.Getenv("CLOUDFLARE_ZONE_ID")
	log.Println("[DEBUG] CLOUDFLARE_ZONE_ID:", zoneID)
	if zoneID == "" {
		// log.Println("CLOUDFLARE_ZONE_ID is not set")
		return errors.New("CLOUDFLARE_ZONE_ID is not set")
	}
	return nil
}

func (c *CloudFlareService) SetRecord(in *SetRecordRequest) (out *SetRecordResponse, err error) {
	// Call CloudFlare API to set domain
	// Construct a new API object using a global API key
	// api, err := cloudflare.New(os.Getenv("CLOUDFLARE_API_KEY"), os.Getenv("CLOUDFLARE_API_EMAIL"))
	// alternatively, you can use a scoped API token
	api, err := cloudflare.NewWithAPIToken(c.CLOUDFLARE_API_TOKEN)
	log.Println("[DEBUG] CLOUDFLARE_API_TOKEN:", c.CLOUDFLARE_API_TOKEN[:5])
	if err != nil {
		log.Fatal(err)
	}

	// Most API calls require a Context
	ctx := context.Background()

	zoneID := os.Getenv("CLOUDFLARE_ZONE_ID")
	log.Println("[DEBUG] CLOUDFLARE_ZONE_ID:", zoneID)
	if zoneID == "" {
		log.Fatal("CLOUDFLARE_ZONE_ID is not set")
	}
	dnsRecords, _, err := api.ListDNSRecords(ctx, cloudflare.ResourceIdentifier(zoneID), cloudflare.ListDNSRecordsParams{})
	if err != nil {
		log.Fatal(err)
	}
	log.Println("[DEBUG] dnsRecords:", dnsRecords)

	for _, record := range dnsRecords {
		log.Println("[DEBUG] record:", record.Name)
		if record.Name == in.Subdomain+"."+record.ZoneName {
			log.Println("[DEBUG] Deleting record:", record.Name)
			err := api.DeleteDNSRecord(ctx, cloudflare.ResourceIdentifier(zoneID), record.ID)
			if err != nil {
				log.Fatal(err)
			}
			log.Println("[DEBUG] Deleted record:", record.Name)
		}
	}

	dnsRecord, err := api.CreateDNSRecord(ctx, cloudflare.ResourceIdentifier(zoneID), cloudflare.CreateDNSRecordParams{
		Type: "A",
		// Name:    GenerateRandomSubDomain(),
		Name:    in.Subdomain,
		Proxied: cloudflare.BoolPtr(true),
		Content: in.Ipaddress,
	})
	if err != nil {
		log.Fatal(err)
	}
	log.Println("[DEBUG] dnsRecord:", dnsRecord)

	return &SetRecordResponse{}, nil
}
