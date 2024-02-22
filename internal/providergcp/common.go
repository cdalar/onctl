package providergcp

import (
	"context"
	"log"

	compute "cloud.google.com/go/compute/apiv1"
)

func GetClient() *compute.InstancesClient {
	ctx := context.Background()
	client, err := compute.NewInstancesRESTClient(ctx)
	if err != nil {
		log.Fatalln(err)
	}
	return client
}

func GetGroupClient() *compute.InstanceGroupsClient {
	ctx := context.Background()
	client, err := compute.NewInstanceGroupsRESTClient(ctx)
	if err != nil {
		log.Fatalln(err)
	}
	return client
}
