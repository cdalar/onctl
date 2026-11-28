package box

import (
	"errors"
	"testing"

	"github.com/cdalar/onctl/pkg/cloud"
	"github.com/stretchr/testify/assert"
)

// fc and ch need KVM, which only Linux has: elsewhere they're refused
// before anything is made, and a Mac user is told what works there.
// Every other provider is a remote API (or ac, Apple containers) and runs
// anywhere.
func TestLocalVMMCheck(t *testing.T) {
	for _, id := range []string{"fc", "ch"} {
		assert.NoError(t, localVMMCheck(id, "linux"))
		err := localVMMCheck(id, "darwin")
		if !assert.Error(t, err) {
			continue
		}
		assert.Contains(t, err.Error(), "Linux")
		assert.Contains(t, err.Error(), "-p ac", "a Mac user should be told what does work there")
		assert.Error(t, localVMMCheck(id, "windows"))
	}
	for _, id := range []string{"aws", "hetzner", "ac", "boxes"} {
		assert.NoError(t, localVMMCheck(id, "darwin"), id)
	}
}

// An OpenShell sandbox has no root and no file transfer, and no public IP
// for a domain: CheckSetup refuses those before the sandbox is made.
func TestCheckSetup_OpenShell(t *testing.T) {
	c := Wrap("openshell", &cloud.ProviderOpenShell{})
	assert.NoError(t, c.CheckSetup(cloud.Vm{}, Setup{}))
	for name, s := range map[string]Setup{
		"apply":    {ApplyFiles: []string{"x.sh"}},
		"upload":   {Uploads: []Transfer{{Local: "a", Remote: "b"}}},
		"download": {Downloads: []Transfer{{Local: "a", Remote: "b"}}},
	} {
		assert.True(t, errors.Is(c.CheckSetup(cloud.Vm{}, s), cloud.ErrOpenShellApplyFiles), name)
	}
	assert.ErrorIs(t, c.CheckSetup(cloud.Vm{CloudInitFile: "ci.yaml"}, Setup{}), cloud.ErrOpenShellApplyFiles)
	assert.Error(t, c.CheckSetup(cloud.Vm{}, Setup{Domain: "box"}), "a sandbox has no public IP to point a domain at")
}

// An image can only be chosen where the provider's Create takes one.
func TestCheckSetup_Image(t *testing.T) {
	assert.Error(t, Wrap("gcp", cloud.ProviderGcp{}).CheckSetup(cloud.Vm{Image: "debian-12"}, Setup{}))
	assert.NoError(t, Wrap("hetzner", cloud.ProviderHetzner{}).CheckSetup(cloud.Vm{Image: "debian-12"}, Setup{}))
}

// A failed upload or apply doesn't stop the steps after it: each is
// reported as it starts and ends, in Setup's order, and every failure
// comes back joined. (The VM here refuses connections, so each step
// fails.)
func TestProvision_ReportsEveryStepAndKeepsGoing(t *testing.T) {
	key := writeEncryptedKey(t) // never parsed: the missing upload fails first, and apply fails to dial
	conn, err := Wrap("fc", cloud.ProviderFC{}).Connect(cloud.Vm{Name: "web", IP: "127.0.0.1"}, ConnectOptions{KeyFile: key, Port: 1})
	if err != nil {
		t.Fatal(err)
	}
	var seen []string
	err = Wrap("fc", cloud.ProviderFC{}).Provision(conn, Setup{
		Uploads:    []Transfer{{Local: "/nonexistent/file", Remote: "file"}},
		ApplyFiles: []string{"setup.sh"},
		Progress: func(s Step) {
			state := "start"
			if s.Done {
				state = "fail"
				if s.Err == nil {
					state = "ok"
				}
			}
			seen = append(seen, string(s.Kind)+" "+s.Item+" "+state)
		},
	})
	assert.Equal(t, []string{
		"upload /nonexistent/file start", "upload /nonexistent/file fail",
		"apply setup.sh start", "apply setup.sh fail",
	}, seen)
	assert.ErrorContains(t, err, "upload /nonexistent/file")
	assert.ErrorContains(t, err, "apply setup.sh")
}
