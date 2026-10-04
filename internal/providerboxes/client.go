// Package providerboxes talks to the hosted boxes service's REST API
// (boxctl-vms's /api/vms*, /api/images), authenticating with a personal
// token minted from the dashboard. The server scopes every request to the
// token's owner, so names here are always the caller's own bare box names.
//
// Ported from boxctl's internal/client (docs/plans/boxes-provider.md).
package providerboxes

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/gorilla/websocket"
)

// DefaultAPIURL is the hosted service.
const DefaultAPIURL = "https://vms-backend.boxctl.io"

// errUnauthorized is what a 401 becomes: the only fix is a new token.
const errUnauthorized = "unauthorized -- your boxes token may be wrong or revoked; run `onctl login` again"

type Client struct {
	baseURL string
	token   string
	http    *http.Client
	// long has no fixed timeout, for calls bounded by their own context
	// instead: Create (the server's own limit is minutes) and Exec (as long
	// as the command runs).
	long *http.Client
}

func New(baseURL, token string) *Client {
	return &Client{
		baseURL: strings.TrimRight(baseURL, "/"),
		token:   token,
		http:    &http.Client{Timeout: 60 * time.Second},
		long:    &http.Client{},
	}
}

// VM mirrors the server's view of a box.
type VM struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	IP        string    `json:"ip"`
	State     string    `json:"state"`
	Ready     bool      `json:"ready"`
	CreatedAt time.Time `json:"created_at"`
	Image     string    `json:"image"`
	VCPU      int       `json:"vcpu"`
	MemMiB    int       `json:"mem_mib"`
	Size      string    `json:"size"`
}

// Image is a boot image the service offers; Name is what Create takes.
type Image struct {
	Name        string `json:"name"`
	Description string `json:"description"`
}

// ExecResult is POST /api/vms/{name}/exec's response.
type ExecResult struct {
	ExitCode int    `json:"exit_code"`
	Stdout   string `json:"stdout"`
	Stderr   string `json:"stderr"`
	// Error is a transport-level failure (the box couldn't be reached, or
	// the command timed out); a command's own nonzero exit is ExitCode.
	Error string `json:"error,omitempty"`
}

// APIError carries the server's status and message.
type APIError struct {
	Status int
	Body   string
}

func (e *APIError) Error() string {
	if body := strings.TrimSpace(e.Body); body != "" {
		return fmt.Sprintf("%s (HTTP %d)", body, e.Status)
	}
	return fmt.Sprintf("HTTP %d", e.Status)
}

func (c *Client) do(ctx context.Context, hc *http.Client, method, path string, body, out any) error {
	var reqBody io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			return err
		}
		reqBody = bytes.NewReader(data)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, reqBody)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+c.token)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	res, err := hc.Do(req)
	if err != nil {
		return fmt.Errorf("contacting %s: %w", c.baseURL, err)
	}
	defer func() { _ = res.Body.Close() }()
	if res.StatusCode == http.StatusUnauthorized {
		return fmt.Errorf("%s", errUnauthorized)
	}
	if res.StatusCode >= 300 {
		data, _ := io.ReadAll(io.LimitReader(res.Body, 4096))
		return &APIError{Status: res.StatusCode, Body: string(data)}
	}
	if out != nil {
		return json.NewDecoder(res.Body).Decode(out)
	}
	return nil
}

func vmPath(name string, rest ...string) string {
	return "/api/vms/" + url.PathEscape(name) + strings.Join(rest, "")
}

func (c *Client) List(ctx context.Context) ([]VM, error) {
	var vms []VM
	if err := c.do(ctx, c.http, http.MethodGet, "/api/vms", nil, &vms); err != nil {
		return nil, err
	}
	return vms, nil
}

func (c *Client) ListImages(ctx context.Context) ([]Image, error) {
	var images []Image
	if err := c.do(ctx, c.http, http.MethodGet, "/api/images", nil, &images); err != nil {
		return nil, err
	}
	return images, nil
}

// createTimeout is a minute past the server's own 5 minute limit on
// create, so the server's error arrives first.
const createTimeout = 6 * time.Minute

// Create creates a box. An empty size leaves it to the server's default.
func (c *Client) Create(ctx context.Context, name, image, size string) (*VM, error) {
	body := map[string]string{"name": name, "image": image}
	if size != "" {
		body["size"] = size
	}
	ctx, cancel := context.WithTimeout(ctx, createTimeout)
	defer cancel()
	var vm VM
	if err := c.do(ctx, c.long, http.MethodPost, "/api/vms", body, &vm); err != nil {
		return nil, err
	}
	return &vm, nil
}

func (c *Client) Destroy(ctx context.Context, name string) error {
	return c.do(ctx, c.http, http.MethodDelete, vmPath(name), nil, nil)
}

func (c *Client) Pause(ctx context.Context, name string) (*VM, error) {
	var vm VM
	if err := c.do(ctx, c.http, http.MethodPost, vmPath(name, "/pause"), nil, &vm); err != nil {
		return nil, err
	}
	return &vm, nil
}

func (c *Client) Resume(ctx context.Context, name string) (*VM, error) {
	var vm VM
	if err := c.do(ctx, c.http, http.MethodPost, vmPath(name, "/resume"), nil, &vm); err != nil {
		return nil, err
	}
	return &vm, nil
}

// Exec runs command on the box (over the server's own ssh, no key of
// ours needed) and returns its separated output and exit code.
func (c *Client) Exec(ctx context.Context, name, command string, timeout time.Duration) (*ExecResult, error) {
	body := map[string]any{"command": command, "timeout_seconds": int(timeout.Seconds())}
	ctx, cancel := context.WithTimeout(ctx, timeout+15*time.Second)
	defer cancel()
	var res ExecResult
	if err := c.do(ctx, c.long, http.MethodPost, vmPath(name, "/exec"), body, &res); err != nil {
		return nil, err
	}
	return &res, nil
}

// waitReadyPollInterval paces WaitReady.
var waitReadyPollInterval = 500 * time.Millisecond

// WaitReady polls until name is running and ready, or timeout elapses.
func (c *Client) WaitReady(ctx context.Context, name string, timeout time.Duration) (*VM, error) {
	deadline := time.Now().Add(timeout)
	for {
		vms, err := c.List(ctx)
		if err != nil {
			return nil, err
		}
		for i := range vms {
			if vms[i].Name == name && vms[i].State == "running" && vms[i].Ready {
				return &vms[i], nil
			}
		}
		if time.Now().After(deadline) {
			return nil, fmt.Errorf("timed out after %s waiting for %s to become ready", timeout, name)
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(waitReadyPollInterval):
		}
	}
}

// DialPort opens a byte stream to port inside the box, through the
// server's tunnel (GET /api/vms/{name}/port/{port}): a WebSocket whose
// binary messages carry the bytes, wrapped as a net.Conn.
func (c *Client) DialPort(ctx context.Context, name string, port int) (*WSConn, error) {
	wsURL := strings.Replace(c.baseURL, "http", "ws", 1) + vmPath(name, "/port/", strconv.Itoa(port))
	header := http.Header{"Authorization": []string{"Bearer " + c.token}}
	conn, res, err := websocket.DefaultDialer.DialContext(ctx, wsURL, header)
	if err != nil {
		// A refused upgrade (box not running, not found) is an ordinary
		// HTTP response with the reason in its body.
		if res != nil {
			defer func() { _ = res.Body.Close() }()
			if res.StatusCode == http.StatusUnauthorized {
				return nil, fmt.Errorf("%s", errUnauthorized)
			}
			data, _ := io.ReadAll(io.LimitReader(res.Body, 4096))
			return nil, &APIError{Status: res.StatusCode, Body: string(data)}
		}
		return nil, fmt.Errorf("contacting %s: %w", c.baseURL, err)
	}
	return NewWSConn(conn), nil
}

// Size is a box size the service offers; Name is what Create takes.
type Size struct {
	Name    string `json:"name"`
	VCPU    int    `json:"vcpu"`
	MemMiB  int    `json:"mem_mib"`
	DiskMiB int    `json:"disk_mib"`
	Default bool   `json:"default"`
}

func (c *Client) ListSizes(ctx context.Context) ([]Size, error) {
	var sizes []Size
	if err := c.do(ctx, c.http, http.MethodGet, "/api/sizes", nil, &sizes); err != nil {
		return nil, err
	}
	return sizes, nil
}

// SetIdleTTL sets how long name may go unused before the service pauses
// it (10 minutes to 30 days); zero means never.
func (c *Client) SetIdleTTL(ctx context.Context, name string, ttl time.Duration) (*VM, error) {
	body := map[string]int64{"idle_ttl_seconds": int64(ttl.Seconds())}
	var vm VM
	if err := c.do(ctx, c.http, http.MethodPut, vmPath(name, "/idle-ttl"), body, &vm); err != nil {
		return nil, err
	}
	return &vm, nil
}
