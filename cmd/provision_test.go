package cmd

import (
	"testing"

	"github.com/cdalar/onctl/pkg/box"
	"github.com/stretchr/testify/assert"
)

// -u is LOCAL:REMOTE and -d REMOTE:LOCAL; a bare path keeps its base name
// on the other side. Only the first colon splits, so a remote path may
// hold more.
func TestParseTransfers(t *testing.T) {
	assert.Equal(t, []box.Transfer{
		{Local: "dist/app", Remote: "/opt/app"},
		{Local: "/tmp/notes.txt", Remote: "notes.txt"},
		{Local: "a", Remote: "b:c"},
	}, parseUploads([]string{"dist/app:/opt/app", "/tmp/notes.txt", "a:b:c"}))
	assert.Equal(t, []box.Transfer{
		{Remote: "/var/log/app.log", Local: "app.log"},
		{Remote: "/etc/hosts", Local: "hosts.vm"},
	}, parseDownloads([]string{"/var/log/app.log", "/etc/hosts:hosts.vm"}))
}
