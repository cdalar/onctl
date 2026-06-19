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
		log.Printf("[DEBUG] failed to create gcp instances client: %v", err)
		return nil
	}
	return client
}

func GetGroupClient() *compute.InstanceGroupsClient {
	ctx := context.Background()
	client, err := compute.NewInstanceGroupsRESTClient(ctx)
	if err != nil {
		log.Printf("[DEBUG] failed to create gcp instanceGroups client: %v", err)
		return nil
	}
	return client
}
