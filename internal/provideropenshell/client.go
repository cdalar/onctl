// Package provideropenshell connects onctl to an NVIDIA OpenShell gateway
// (https://github.com/NVIDIA/OpenShell) through its Go SDK.
//
// onctl keeps no gateway settings of its own: it uses the gateways the
// openshell CLI has registered (`openshell gateway add`), from
// ~/.config/openshell/gateways/<name>/, so one registration serves both.
package provideropenshell

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	v1 "github.com/NVIDIA/OpenShell/sdk/go/openshell/v1"
	"github.com/NVIDIA/OpenShell/sdk/go/openshell/v1/gateway"
)

// Gateway is a registered gateway and a client connected to it.
type Gateway struct {
	// Name is the gateway's name in the openshell CLI: the one asked
	// for, or the active gateway's when none was.
	Name     string
	Endpoint string
	Client   *v1.Client
}

// Connect returns a client for the gateway the openshell CLI registered
// as name, or for its active gateway when name is empty.
func Connect(name string) (*Gateway, error) {
	cfg, err := gateway.LoadConfig(name)
	if err != nil {
		return nil, explainLoadError(name, err)
	}
	client, err := newClient(cfg)
	if err != nil {
		return nil, fmt.Errorf("connecting to OpenShell gateway %s (%s): %w", cfg.Name, cfg.Endpoint, err)
	}
	return &Gateway{Name: cfg.Name, Endpoint: cfg.Endpoint, Client: client}, nil
}

// newClient builds the SDK client for cfg. The SDK's own loader
// (gateway.NewClient) doesn't do mTLS, the mode a self-hosted gateway
// uses by default, so that mode is built here from the bundle the CLI
// keeps beside metadata.json; the others go through the SDK.
func newClient(cfg *gateway.Config) (*v1.Client, error) {
	if cfg.AuthMode != gateway.AuthModeMTLS {
		return gateway.NewClient(cfg.Name)
	}
	tls, err := mtlsBundle(cfg.Dir)
	if err != nil {
		return nil, err
	}
	return v1.NewClient(v1.Config{Address: cfg.Endpoint, TLS: tls})
}

// mtlsBundle is the client certificate, its key and the gateway's CA in
// dir/mtls, where `openshell gateway add` expects them.
func mtlsBundle(dir string) (*v1.TLSConfig, error) {
	mtls := filepath.Join(dir, "mtls")
	tls := &v1.TLSConfig{
		CAFile:   filepath.Join(mtls, "ca.crt"),
		CertFile: filepath.Join(mtls, "tls.crt"),
		KeyFile:  filepath.Join(mtls, "tls.key"),
	}
	for _, f := range []string{tls.CAFile, tls.CertFile, tls.KeyFile} {
		if _, err := os.Stat(f); err != nil {
			return nil, fmt.Errorf("mTLS client bundle incomplete: %w", err)
		}
	}
	return tls, nil
}

func explainLoadError(name string, err error) error {
	switch {
	case errors.Is(err, gateway.ErrNoActiveGateway):
		return errors.New("no OpenShell gateway selected: register one with `openshell gateway add` " +
			"(the onctl-templates openshell template prints how) or set openshell.gateway in onctl.yaml")
	case errors.Is(err, gateway.ErrGatewayNotFound):
		return fmt.Errorf("no OpenShell gateway named %q is registered with the openshell CLI (see `openshell gateway list`)", name)
	}
	return fmt.Errorf("reading OpenShell gateway %q: %w", name, err)
}
