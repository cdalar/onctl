package cmd

import (
	"errors"
	"testing"

	"github.com/cdalar/onctl/pkg/cloud"
)


func TestOpenShellSSHCheck(t *testing.T) {
	if err := openShellSSHCheck(nil, nil, nil); err != nil {
		t.Errorf("plain ssh: %v", err)
	}
	if err := openShellSSHCheck([]string{"x.sh"}, nil, nil); !errors.Is(err, cloud.ErrOpenShellApplyFiles) {
		t.Errorf("apply file: %v", err)
	}
}

func TestConfigOptional(t *testing.T) {
	for p, want := range map[string]bool{"boxes": true, "openshell": true, "hetzner": false, "ac": false} {
		if got := configOptional(p); got != want {
			t.Errorf("configOptional(%s) = %v, want %v", p, got, want)
		}
	}
}
