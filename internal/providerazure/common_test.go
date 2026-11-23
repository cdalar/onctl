package providerazure

import (
	"testing"
)

func TestConnectionAzure(t *testing.T) {
	cred, err := connectionAzure()
	if err != nil {
		t.Fatalf("unexpected error creating default azure credential: %v", err)
	}
	if cred == nil {
		t.Fatal("expected non-nil credential")
	}
}

func TestGetResourceGraphClient(t *testing.T) {
	client, err := GetResourceGraphClient()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if client == nil {
		t.Fatal("expected non-nil resource graph client")
	}
}
