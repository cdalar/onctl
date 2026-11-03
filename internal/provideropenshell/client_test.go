package provideropenshell

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// gatewayHome points the openshell CLI's config directory at a temporary
// one and returns its gateways directory.
func gatewayHome(t *testing.T) string {
	t.Helper()
	cfg := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", cfg)
	t.Setenv("OPENSHELL_SYSTEM_GATEWAY_DIR", t.TempDir())
	dir := filepath.Join(cfg, "openshell", "gateways")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	return dir
}

func addGateway(t *testing.T, gateways, name, metadata string) string {
	t.Helper()
	dir := filepath.Join(gateways, name)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "metadata.json"), []byte(metadata), 0o600); err != nil {
		t.Fatal(err)
	}
	return dir
}

func TestConnectNoActiveGateway(t *testing.T) {
	gatewayHome(t)
	_, err := Connect("")
	if err == nil || !strings.Contains(err.Error(), "no OpenShell gateway selected") {
		t.Errorf("err = %v", err)
	}
}

func TestConnectUnknownGateway(t *testing.T) {
	gatewayHome(t)
	_, err := Connect("nope")
	if err == nil || !strings.Contains(err.Error(), `no OpenShell gateway named "nope"`) {
		t.Errorf("err = %v", err)
	}
}

func TestConnectMTLSWithoutBundle(t *testing.T) {
	gateways := gatewayHome(t)
	addGateway(t, gateways, "gw", `{"name":"gw","gateway_endpoint":"https://203.0.113.10:8080","is_remote":true,"auth_mode":"mtls"}`)
	_, err := Connect("gw")
	if err == nil || !strings.Contains(err.Error(), "mTLS client bundle incomplete") {
		t.Errorf("err = %v", err)
	}
}

func TestMTLSBundlePaths(t *testing.T) {
	dir := t.TempDir()
	mtls := filepath.Join(dir, "mtls")
	if err := os.MkdirAll(mtls, 0o700); err != nil {
		t.Fatal(err)
	}
	for _, f := range []string{"ca.crt", "tls.crt", "tls.key"} {
		if err := os.WriteFile(filepath.Join(mtls, f), []byte("x"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	tls, err := mtlsBundle(dir)
	if err != nil {
		t.Fatal(err)
	}
	if tls.CAFile != filepath.Join(mtls, "ca.crt") || tls.CertFile != filepath.Join(mtls, "tls.crt") || tls.KeyFile != filepath.Join(mtls, "tls.key") {
		t.Errorf("tls = %+v", tls)
	}
}
