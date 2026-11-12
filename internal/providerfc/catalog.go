package providerfc

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/cdalar/onctl/pkg/cloud"
)

// PublicCatalogURL is where boxctl publishes the images an fc microVM can
// boot (github.com/boxctl-io/boxctl-images): root filesystems and the
// kernel they boot with, each a zstd-compressed file anyone can download
// and hand to `onctl create -p fc --rootfs-image/--kernel-image`.
const PublicCatalogURL = "https://images.boxctl.io"

// catalogStatusRetired marks an image that is no longer meant to be
// booted; its manifest stays so old backups can still find their base.
const catalogStatusRetired = "retired"

// CatalogLister lists the images published at BaseURL. It is not a
// method on the fc provider on purpose: nothing here needs a Firecracker
// host, so `onctl images -p fc` works on any machine, including one that
// can't run the provider at all.
type CatalogLister struct {
	BaseURL string
	// HTTP defaults to a client with a timeout.
	HTTP *http.Client
}

// catalogManifest is the part of catalog/<name>.json this needs.
type catalogManifest struct {
	Name    string `json:"name"`
	Kind    string `json:"kind"`
	OS      string `json:"os"`
	Status  string `json:"status"`
	Current struct {
		SizeBytes int64  `json:"size_bytes"`
		Key       string `json:"key"`
	} `json:"current"`
}

// ListImages reads index.json for the names (the bucket can't be listed)
// and then each image's manifest.
func (l CatalogLister) ListImages() ([]cloud.CloudImage, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	base := strings.TrimRight(l.BaseURL, "/")

	var index struct {
		Images []string `json:"images"`
	}
	if err := l.getJSON(ctx, base+"/index.json", &index); err != nil {
		return nil, fmt.Errorf("reading the image catalog: %w", err)
	}

	images := make([]cloud.CloudImage, 0, len(index.Images))
	for _, name := range index.Images {
		var m catalogManifest
		if err := l.getJSON(ctx, base+"/catalog/"+name+".json", &m); err != nil {
			return nil, fmt.Errorf("reading the image catalog: %w", err)
		}
		if m.Status == catalogStatusRetired || m.Current.Key == "" {
			continue
		}
		images = append(images, cloud.CloudImage{
			Name:     m.Name,
			Type:     m.Kind,
			OSFlavor: m.OS,
			Size:     formatMiB(m.Current.SizeBytes),
			URL:      base + "/" + m.Current.Key,
		})
	}
	return images, nil
}

func (l CatalogLister) getJSON(ctx context.Context, url string, v any) error {
	client := l.HTTP
	if client == nil {
		client = &http.Client{Timeout: 15 * time.Second}
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("%s: %s", url, resp.Status)
	}
	// Manifests are a few hundred bytes; the cap is against a wrong URL
	// answering with something huge.
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(v); err != nil {
		return fmt.Errorf("%s: %w", url, err)
	}
	return nil
}

// formatMiB is an image's unpacked size the way its definition states
// it: whole MiB, or GiB once it is that big.
func formatMiB(bytes int64) string {
	const mib, gib = 1 << 20, 1 << 30
	if bytes >= gib {
		return fmt.Sprintf("%.1f GiB", float64(bytes)/gib)
	}
	return fmt.Sprintf("%d MiB", bytes/mib)
}
