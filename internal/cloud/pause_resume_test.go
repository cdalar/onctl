package cloud

import "testing"

// Compile-time guarantee that every cloud provider implements the full
// CloudProviderInterface, including Pause and Resume. The build breaks here if a
// provider drops pause/resume or changes their signatures.
var (
	_ CloudProviderInterface = ProviderHetzner{}
	_ CloudProviderInterface = ProviderAws{}
	_ CloudProviderInterface = ProviderAzure{}
	_ CloudProviderInterface = ProviderGcp{}
)

// TestAllProvidersImplementPauseResume makes the cross-provider contract explicit
// in the suite: pause/resume is a capability every provider must offer, not a
// Hetzner-only feature.
func TestAllProvidersImplementPauseResume(t *testing.T) {
	providers := map[string]any{
		"hetzner": ProviderHetzner{},
		"aws":     ProviderAws{},
		"azure":   ProviderAzure{},
		"gcp":     ProviderGcp{},
	}
	for name, p := range providers {
		if _, ok := p.(CloudProviderInterface); !ok {
			t.Errorf("provider %q does not implement CloudProviderInterface (missing Pause/Resume?)", name)
		}
	}
}
