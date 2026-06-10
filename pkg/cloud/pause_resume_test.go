package cloud

import "testing"

// Compile-time guarantee that every cloud provider implements the full
// CloudProviderInterface, including Pause, Resume and ListPaused. The build
// breaks here if a provider drops any method or changes signatures.
var (
	_ CloudProviderInterface = ProviderHetzner{}
	_ CloudProviderInterface = ProviderAws{}
	_ CloudProviderInterface = ProviderAzure{}
	_ CloudProviderInterface = ProviderGcp{}
	_ CloudProviderInterface = ProviderFirecracker{}
)

// TestAllProvidersImplementPauseResume makes the cross-provider contract explicit
// in the suite: every provider must implement Pause/Resume/ListPaused (current
// stubs for non-Hetzner return "not supported yet").
func TestAllProvidersImplementPauseResume(t *testing.T) {
	providers := map[string]any{
		"hetzner":     ProviderHetzner{},
		"aws":         ProviderAws{},
		"azure":       ProviderAzure{},
		"gcp":         ProviderGcp{},
		"firecracker": ProviderFirecracker{},
	}
	for name, p := range providers {
		if _, ok := p.(CloudProviderInterface); !ok {
			t.Errorf("provider %q does not implement CloudProviderInterface (missing Pause/Resume?)", name)
		}
	}
}
