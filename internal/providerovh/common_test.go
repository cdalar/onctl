package providerovh

import (
	"testing"
)

func TestGetClient_MissingCredentials(t *testing.T) {
	t.Setenv("OVH_ENDPOINT", "")
	t.Setenv("OVH_APPLICATION_KEY", "")
	t.Setenv("OVH_APPLICATION_SECRET", "")
	t.Setenv("OVH_CONSUMER_KEY", "")
	client, err := GetClient()
	if err == nil {
		t.Fatal("expected error without OVH credentials, got nil")
	}
	if client != nil {
		t.Fatalf("expected nil client on error, got %v", client)
	}
}
