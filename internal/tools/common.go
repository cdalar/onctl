package tools

import "os"

func Contains(slice []string, searchValue string) bool {
	for _, value := range slice {
		if value == searchValue {
			return true
		}
	}
	return false
}

func WhichCloudProvider() string {
	if os.Getenv("AWS_ACCESS_KEY_ID") != "" || os.Getenv("AWS_PROFILE") != "" {
		return "aws"
	}
	if os.Getenv("AZURE_CLIENT_ID") != "" {
		return "azure"
	}
	if os.Getenv("GOOGLE_CREDENTIALS") != "" {
		return "gcp"
	}
	if os.Getenv("HCLOUD_TOKEN") != "" {
		return "hetzner"
	}
	return "none"
}
