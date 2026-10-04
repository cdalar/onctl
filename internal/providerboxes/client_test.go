package providerboxes

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

func TestClientSendsTokenAndDecodes(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bearer tok" {
			t.Errorf("Authorization = %q", got)
		}
		_ = json.NewEncoder(w).Encode([]VM{{Name: "a", State: "running", Ready: true, Size: "small"}})
	}))
	defer srv.Close()

	vms, err := New(srv.URL+"/", "tok").List(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(vms) != 1 || vms[0].Name != "a" || vms[0].Size != "small" {
		t.Fatalf("got %+v", vms)
	}
}

func TestClientErrors(t *testing.T) {
	status := http.StatusUnauthorized
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(status)
		_, _ = io.WriteString(w, "no such image\n")
	}))
	defer srv.Close()
	c := New(srv.URL, "tok")

	_, err := c.List(context.Background())
	if err == nil || !strings.Contains(err.Error(), "onctl login") {
		t.Fatalf("a 401 should say to log in again, got %v", err)
	}
	status = http.StatusBadRequest
	_, err = c.Create(context.Background(), "a", "nope", "")
	if err == nil || err.Error() != "no such image (HTTP 400)" {
		t.Fatalf("got %v", err)
	}
}

func TestCreateOmitsEmptySize(t *testing.T) {
	var body map[string]string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&body)
		_ = json.NewEncoder(w).Encode(VM{Name: body["name"]})
	}))
	defer srv.Close()

	if _, err := New(srv.URL, "t").Create(context.Background(), "a", "debian-slim", ""); err != nil {
		t.Fatal(err)
	}
	if _, has := body["size"]; has || body["image"] != "debian-slim" {
		t.Fatalf("body %v: an empty size must be left to the server", body)
	}
}

func TestWaitReady(t *testing.T) {
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		_ = json.NewEncoder(w).Encode([]VM{{Name: "a", State: "running", Ready: calls >= 3}})
	}))
	defer srv.Close()
	waitReadyPollInterval = time.Millisecond

	vm, err := New(srv.URL, "t").WaitReady(context.Background(), "a", 5*time.Second)
	if err != nil || !vm.Ready || calls != 3 {
		t.Fatalf("vm %+v err %v after %d calls", vm, err, calls)
	}
	if _, err := New(srv.URL, "t").WaitReady(context.Background(), "missing", 10*time.Millisecond); err == nil {
		t.Fatal("expected a timeout for a box that never appears")
	}
}

// echoTunnel is the server's port tunnel, echoing whatever arrives.
func echoTunnel(t *testing.T) *httptest.Server {
	up := websocket.Upgrader{}
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/vms/a/port/22" || r.Header.Get("Authorization") != "Bearer t" {
			http.Error(w, "box a is paused", http.StatusConflict)
			return
		}
		ws, err := up.Upgrade(w, r, nil)
		if err != nil {
			t.Error(err)
			return
		}
		defer func() { _ = ws.Close() }()
		for {
			mt, msg, err := ws.ReadMessage()
			if err != nil {
				return
			}
			if err := ws.WriteMessage(mt, msg); err != nil {
				return
			}
		}
	}))
}

func TestDialPortIsAByteStream(t *testing.T) {
	srv := echoTunnel(t)
	defer srv.Close()

	conn, err := New(srv.URL, "t").DialPort(context.Background(), "a", 22)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = conn.Close() }()
	if _, err := conn.Write([]byte("hello, ")); err != nil {
		t.Fatal(err)
	}
	if _, err := conn.Write([]byte("box")); err != nil {
		t.Fatal(err)
	}
	// Two messages, read back through small reads: message boundaries
	// must not show.
	got := make([]byte, 0, 10)
	buf := make([]byte, 3)
	for len(got) < len("hello, box") {
		n, err := conn.Read(buf)
		if err != nil {
			t.Fatal(err)
		}
		got = append(got, buf[:n]...)
	}
	if string(got) != "hello, box" {
		t.Fatalf("got %q", got)
	}
}

func TestDialPortRefused(t *testing.T) {
	srv := echoTunnel(t)
	defer srv.Close()

	_, err := New(srv.URL, "t").DialPort(context.Background(), "b", 22)
	if err == nil || !strings.Contains(err.Error(), "box a is paused") {
		t.Fatalf("a refused upgrade should carry the server's reason, got %v", err)
	}
}
