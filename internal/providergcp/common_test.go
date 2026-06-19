package providergcp

import (
	"os/exec"
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestGCloudDefaultProject only checks that the helper doesn't panic and
// returns a string; the actual project ID depends on the test runner's local
// gcloud configuration, so the value itself can't be asserted.
func TestGCloudDefaultProject(t *testing.T) {
	if _, err := exec.LookPath("gcloud"); err != nil {
		t.Skip("gcloud command not available")
	}
	assert.NotPanics(t, func() {
		GCloudDefaultProject()
	})
}

func TestGCloudDefaultProject_NoGcloud(t *testing.T) {
	t.Setenv("PATH", "")
	assert.Equal(t, "", GCloudDefaultProject())
}
