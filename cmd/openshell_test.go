package cmd

import (
	"errors"
	"testing"

	"github.com/cdalar/onctl/pkg/cloud"
)

func TestOpenShellCreateCheck(t *testing.T) {
	tests := []struct {
		name    string
		opt     cmdCreateOptions
		wantErr bool
	}{
		{"plain create", cmdCreateOptions{}, false},
		{"image only", cmdCreateOptions{Vm: cloud.Vm{Image: "alpine:3.22"}}, false},
		{"apply file", cmdCreateOptions{ApplyFiles: []string{"docker/docker.sh"}}, true},
		{"upload", cmdCreateOptions{UploadFiles: []string{"a:b"}}, true},
		{"download", cmdCreateOptions{DownloadFiles: []string{"a"}}, true},
		{"cloud-init", cmdCreateOptions{Vm: cloud.Vm{CloudInitFile: "x.config"}}, true},
		{"domain", cmdCreateOptions{Domain: "box"}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := openShellCreateCheck(tt.opt)
			if (err != nil) != tt.wantErr {
				t.Errorf("err = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

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
