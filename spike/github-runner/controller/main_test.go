package main

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

const testSecret = "test-secret"

func testServer(t *testing.T) (*httptest.Server, string) {
	t.Helper()
	stubLog := filepath.Join(t.TempDir(), "stub")

	t.Setenv("GH_REPO", "owner/repo")
	t.Setenv("WEBHOOK_SECRET", testSecret)
	t.Setenv("RUNNER_LABELS", "self-hosted,onctl")
	t.Setenv("BOOTSTRAP_SCRIPT", "../github-runner-jit.sh")
	t.Setenv("ONCTL_BIN", "testdata/stub-onctl.sh")
	t.Setenv("GH_BIN", "testdata/stub-gh.sh")
	t.Setenv("STUB_LOG", stubLog)

	s := newServer(loadConfig())
	ts := httptest.NewServer(s.routes())
	t.Cleanup(ts.Close)
	return ts, stubLog
}

func workflowJobPayload(t *testing.T, action string, jobID int64, labels []string, runnerName string) []byte {
	t.Helper()
	body := map[string]any{
		"action": action,
		"workflow_job": map[string]any{
			"id":          jobID,
			"labels":      labels,
			"runner_name": runnerName,
		},
	}
	b, err := json.Marshal(body)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func postWebhook(t *testing.T, url, event string, body []byte, secret string) *http.Response {
	t.Helper()
	req, err := http.NewRequest(http.MethodPost, url+"/webhook", bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("X-GitHub-Event", event)
	if secret != "" {
		mac := hmac.New(sha256.New, []byte(secret))
		mac.Write(body)
		req.Header.Set("X-Hub-Signature-256", "sha256="+hex.EncodeToString(mac.Sum(nil)))
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	return resp
}

func waitForLogContains(t *testing.T, path, substr string, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if data, err := os.ReadFile(path); err == nil && strings.Contains(string(data), substr) {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %q in %s", substr, path)
}

func TestQueuedWithMatchingLabelsProvisionsVM(t *testing.T) {
	ts, stubLog := testServer(t)

	body := workflowJobPayload(t, "queued", 1001, []string{"self-hosted", "onctl"}, "")
	resp := postWebhook(t, ts.URL, "workflow_job", body, testSecret)
	if resp.StatusCode != http.StatusAccepted {
		t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusAccepted)
	}

	waitForLogContains(t, stubLog+".gh", "name=gh-runner-1001", time.Second)
	waitForLogContains(t, stubLog+".onctl",
		"create -n gh-runner-1001 -a ../github-runner-jit.sh -e JIT_CONFIG=fake-jit-config-blob",
		time.Second)
}

func TestCompletedDestroysVM(t *testing.T) {
	ts, stubLog := testServer(t)

	// The completed job's own ID (9999) deliberately differs from the
	// runner's name: a runner picks up whatever queued job matches its
	// labels, not necessarily the one that triggered its creation. Teardown
	// must key off runner_name (the VM name), not the completed job's ID.
	body := workflowJobPayload(t, "completed", 9999, []string{"self-hosted", "onctl"}, "gh-runner-1002")
	resp := postWebhook(t, ts.URL, "workflow_job", body, testSecret)
	if resp.StatusCode != http.StatusAccepted {
		t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusAccepted)
	}

	waitForLogContains(t, stubLog+".onctl", "destroy gh-runner-1002 -f", time.Second)
}

func TestCompletedWithoutRunnerNameIsSkipped(t *testing.T) {
	ts, stubLog := testServer(t)

	// A job can complete (e.g. cancelled) without ever being assigned a
	// runner, in which case runner_name is empty and there's no VM to
	// destroy.
	body := workflowJobPayload(t, "completed", 1003, []string{"self-hosted", "onctl"}, "")
	resp := postWebhook(t, ts.URL, "workflow_job", body, testSecret)
	if resp.StatusCode != http.StatusAccepted {
		t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusAccepted)
	}

	if _, err := os.Stat(stubLog + ".onctl"); !os.IsNotExist(err) {
		t.Fatalf("expected no .onctl log, got err=%v", err)
	}
}

func TestQueuedWithNonMatchingLabelsIsIgnored(t *testing.T) {
	ts, stubLog := testServer(t)

	body := workflowJobPayload(t, "queued", 2002, []string{"ubuntu-latest"}, "")
	resp := postWebhook(t, ts.URL, "workflow_job", body, testSecret)
	if resp.StatusCode != http.StatusAccepted {
		t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusAccepted)
	}

	// No goroutine is spawned for a label mismatch, so the absence of the
	// log files is immediate, not a race.
	for _, suffix := range []string{".gh", ".onctl"} {
		if _, err := os.Stat(stubLog + suffix); !os.IsNotExist(err) {
			t.Fatalf("expected no %s log, got err=%v", suffix, err)
		}
	}
}

func TestBadSignatureIsRejected(t *testing.T) {
	ts, stubLog := testServer(t)

	body := workflowJobPayload(t, "queued", 3003, []string{"self-hosted", "onctl"}, "")
	resp := postWebhook(t, ts.URL, "workflow_job", body, "wrong-secret")
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusUnauthorized)
	}

	for _, suffix := range []string{".gh", ".onctl"} {
		if _, err := os.Stat(stubLog + suffix); !os.IsNotExist(err) {
			t.Fatalf("expected no %s log, got err=%v", suffix, err)
		}
	}
}

func TestPingIsAcknowledged(t *testing.T) {
	ts, stubLog := testServer(t)

	resp := postWebhook(t, ts.URL, "ping", []byte(`{"zen":"hi"}`), testSecret)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusOK)
	}

	for _, suffix := range []string{".gh", ".onctl"} {
		if _, err := os.Stat(stubLog + suffix); !os.IsNotExist(err) {
			t.Fatalf("expected no %s log, got err=%v", suffix, err)
		}
	}
}

// Sanity check that validSignature matches GitHub's documented scheme
// independent of the HTTP plumbing above.
func TestValidSignature(t *testing.T) {
	body := []byte(`{"hello":"world"}`)
	mac := hmac.New(sha256.New, []byte(testSecret))
	mac.Write(body)
	sig := "sha256=" + hex.EncodeToString(mac.Sum(nil))

	if !validSignature(body, sig, testSecret) {
		t.Error("expected valid signature to pass")
	}
	if validSignature(body, sig, "other-secret") {
		t.Error("expected wrong secret to fail")
	}
	if validSignature(body, "sha1=deadbeef", testSecret) {
		t.Error("expected non-sha256 prefix to fail")
	}
	if validSignature(body, "", testSecret) {
		t.Error("expected missing header to fail")
	}
}
