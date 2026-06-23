package providergcp

import (
	"context"
	"log"
	"os/exec"
	"strings"
	"time"

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

// GCloudDefaultProject best-effort resolves the gcloud CLI's active project
// (the same project `gcloud` commands run against), so onctl can default
// gcp.project to it when the value in onctl.yaml is still the placeholder
// or otherwise unset. Returns "" if gcloud isn't installed, isn't configured,
// or errors out (with a short timeout).
func GCloudDefaultProject() string {
	if _, err := exec.LookPath("gcloud"); err != nil {
		return ""
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, "gcloud", "config", "get-value", "project").Output()
	if err != nil {
		return ""
	}
	project := strings.TrimSpace(string(out))
	if project == "" || project == "(unset)" {
		return ""
	}
	return project
}
