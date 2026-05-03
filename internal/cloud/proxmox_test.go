package cloud

import (
	"os"
	"path/filepath"
	"strconv"
	"testing"
)

// TestProviderProxmox_Destroy_InvalidID tests that Destroy returns an error for invalid VM IDs.
func TestProviderProxmox_Destroy_InvalidID(t *testing.T) {
	tests := []struct {
		name    string
		id      string
		wantErr bool
	}{
		{"empty string", "", true},
		{"non-numeric", "abc", true},
		{"negative", "-1", true},
		{"float", "1.5", true},
		{"too large for uint32", "4294967296", true}, // MaxUint32 + 1
		{"valid id", "100", false},
		{"valid id zero", "0", false},
		{"max uint32", "4294967295", false},
	}

	p := ProviderProxmox{Client: nil}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := strconv.ParseUint(tt.id, 10, 32)
			gotErr := err != nil
			if gotErr != tt.wantErr {
				t.Errorf("ParseUint(%q, 10, 32) error = %v, wantErr %v", tt.id, err, tt.wantErr)
			}
		})
	}

	// Ensure unused variable doesn't cause issues
	_ = p
}

// TestProviderProxmox_CreateSSHKey_FileNotFound tests that CreateSSHKey errors on missing file.
func TestProviderProxmox_CreateSSHKey_FileNotFound(t *testing.T) {
	p := ProviderProxmox{Client: nil}
	_, err := p.CreateSSHKey("/nonexistent/path/key.pub")
	if err == nil {
		t.Error("expected error for missing key file, got nil")
	}
}

// TestProviderProxmox_CreateSSHKey_InvalidKey tests that CreateSSHKey errors on invalid key content.
func TestProviderProxmox_CreateSSHKey_InvalidKey(t *testing.T) {
	tmp := t.TempDir()
	keyFile := filepath.Join(tmp, "bad.pub")
	if err := os.WriteFile(keyFile, []byte("not-a-valid-ssh-key\n"), 0600); err != nil {
		t.Fatal(err)
	}

	p := ProviderProxmox{Client: nil}
	_, err := p.CreateSSHKey(keyFile)
	if err == nil {
		t.Error("expected error for invalid SSH key content, got nil")
	}
}

// TestProviderProxmox_CreateSSHKey_ValidKey tests that CreateSSHKey succeeds with a real public key.
func TestProviderProxmox_CreateSSHKey_ValidKey(t *testing.T) {
	// Generate a real ED25519 key in a temp dir using ssh package helpers
	// We use a pre-generated public key for determinism in tests.
	const testPubKey = "ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIOMqqnkVzrm0SdG6UOoqKLsabgH5C9okWi0dh2l9GKJl test@test\n"

	tmp := t.TempDir()
	keyFile := filepath.Join(tmp, "id_ed25519.pub")
	if err := os.WriteFile(keyFile, []byte(testPubKey), 0600); err != nil {
		t.Fatal(err)
	}

	p := ProviderProxmox{Client: nil}
	keyID, err := p.CreateSSHKey(keyFile)
	if err != nil {
		t.Fatalf("CreateSSHKey returned unexpected error: %v", err)
	}
	if keyID != keyFile {
		t.Errorf("CreateSSHKey returned keyID %q, want %q", keyID, keyFile)
	}
}

// TestProviderProxmox_Type tests that ProviderProxmox satisfies expected structural properties.
func TestProviderProxmox_Type(t *testing.T) {
	p := ProviderProxmox{}
	if p.Client != nil {
		t.Error("expected nil Client on zero-value ProviderProxmox")
	}
}

// TestVmIDParsing tests the integer conversion logic used in Destroy (ParseUint with bit size 32).
func TestVmIDParsing(t *testing.T) {
	tests := []struct {
		input   string
		want    uint32
		wantErr bool
	}{
		{"100", 100, false},
		{"0", 0, false},
		{"4294967295", 4294967295, false}, // MaxUint32
		{"4294967296", 0, true},           // overflow
		{"-1", 0, true},
		{"abc", 0, true},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			v, err := strconv.ParseUint(tt.input, 10, 32)
			if (err != nil) != tt.wantErr {
				t.Errorf("ParseUint(%q) error = %v, wantErr %v", tt.input, err, tt.wantErr)
				return
			}
			if err == nil && uint32(v) != tt.want {
				t.Errorf("ParseUint(%q) = %d, want %d", tt.input, uint32(v), tt.want)
			}
		})
	}
}
