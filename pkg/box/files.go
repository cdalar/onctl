package box

import (
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/cdalar/onctl/internal/files"
)

// TemplatesURL is where ResolveFile looks for a name it finds neither on
// disk nor among onctl's own files (onctl create -a docker/docker.sh).
const TemplatesURL = "https://templates.onctl.com/"

// ResolveFile is a local path to the apply file, cloud-init file or
// template name: name itself when it's on disk, else a copy of onctl's
// embedded file of that name, else a download of name (an http(s) URL,
// or a path under TemplatesURL).
func ResolveFile(name string) (string, error) {
	if name == "" {
		return "", nil
	}
	if _, err := os.Stat(name); err == nil {
		return name, nil
	}
	if content, err := files.EmbededFiles.ReadFile(name); err == nil {
		log.Println("[DEBUG]", name, "found in onctl's embedded files")
		return writeTemp(name, content)
	}
	url := name
	if !strings.HasPrefix(url, "http") {
		url = TemplatesURL + name
	}
	client := http.Client{Timeout: 60 * time.Second}
	resp, err := client.Get(url)
	if err != nil {
		return "", fmt.Errorf("%s isn't on disk, among onctl's files, or at %s: %w", name, url, err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("%s isn't on disk, among onctl's files, or at %s (http %d)", name, url, resp.StatusCode)
	}
	content, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("downloading %s: %w", url, err)
	}
	log.Println("[DEBUG]", name, "downloaded from", url)
	return writeTemp(filepath.Base(url), content)
}

// writeTemp writes content to name (which may hold directories) in a new
// temporary directory, so the file keeps its name on the VM.
func writeTemp(name string, content []byte) (string, error) {
	dir, err := os.MkdirTemp("", "onctl")
	if err != nil {
		return "", err
	}
	path := filepath.Join(dir, name)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return "", err
	}
	if err := os.WriteFile(path, content, 0o644); err != nil {
		return "", err
	}
	return path, nil
}
