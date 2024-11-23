package providervsphere

import (
	"context"
	"log"
	"net/url"
	"os"

	"github.com/vmware/govmomi"
)

func GetClient() *govmomi.Client {
	// Parse URL from string
	u, err := url.Parse("http://user:pass@127.0.0.1:8989/sdk")
	if err != nil {
		log.Println(err)
		os.Exit(1)
	}
	client, err := govmomi.NewClient(context.TODO(), u, true)
	if err != nil {
		log.Println(err)
		os.Exit(1)
	}
	return client
}
