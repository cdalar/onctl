package cmd

import (
	"testing"

	"github.com/cdalar/onctl/pkg/cloud"
)

func TestImagesTemplateDropsEmptyColumns(t *testing.T) {
	boxes := []cloud.CloudImage{{Name: "debian-slim", Description: "Minimal Debian"}}
	if got, want := imagesTemplate(boxes), "NAME\tDESCRIPTION\n{{range .}}{{.Name}}\t{{.Description}}\n{{end}}"; got != want {
		t.Errorf("boxes: got %q, want %q", got, want)
	}
	hetzner := []cloud.CloudImage{
		{Name: "ubuntu-24.04", Type: "system", OSFlavor: "ubuntu", OSVersion: "24.04", Description: "Ubuntu 24.04"},
		{Name: "debian-13", Type: "system", OSFlavor: "debian"},
	}
	if got, want := imagesTemplate(hetzner), "NAME\tTYPE\tOS FLAVOR\tOS VERSION\tDESCRIPTION\n{{range .}}{{.Name}}\t{{.Type}}\t{{.OSFlavor}}\t{{.OSVersion}}\t{{.Description}}\n{{end}}"; got != want {
		t.Errorf("hetzner: a column any image fills stays: got %q", got)
	}
}
