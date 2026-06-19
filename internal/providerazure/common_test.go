package providerazure

import (
	"os/exec"
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestAzureCLISubscriptionID only checks that the helper doesn't panic and
// returns a string; the actual subscription ID depends on the test runner's
// local az CLI login state, so the value itself can't be asserted.
func TestAzureCLISubscriptionID(t *testing.T) {
	if _, err := exec.LookPath("az"); err != nil {
		t.Skip("az command not available")
	}
	assert.NotPanics(t, func() {
		AzureCLISubscriptionID()
	})
}

func TestAzureCLIDefaultResourceGroup(t *testing.T) {
	if _, err := exec.LookPath("az"); err != nil {
		t.Skip("az command not available")
	}
	assert.NotPanics(t, func() {
		AzureCLIDefaultResourceGroup()
	})
}

func TestAzureCLIHelpers_NoAzCLI(t *testing.T) {
	t.Setenv("PATH", "")
	assert.Equal(t, "", AzureCLISubscriptionID())
	assert.Equal(t, "", AzureCLIDefaultResourceGroup())
}
