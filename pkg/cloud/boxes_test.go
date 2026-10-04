package cloud

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/cdalar/onctl/internal/providerboxes"
	"github.com/gorilla/websocket"
)

// fakeBoxes is the boxes API in memory: just enough of /api/vms* for the
// provider.
type fakeBoxes struct {
	mu      sync.Mutex
	boxes   map[string]*providerboxes.VM
	created map[string]string // name -> "image size"
	execs   []string
	resumed []string
}

func newFakeBoxes(t *testing.T) (*fakeBoxes, *ProviderBoxes) {
	f := &fakeBoxes{boxes: map[string]*providerboxes.VM{}, created: map[string]string{}}
	up := websocket.Upgrader{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		defer f.mu.Unlock()
		parts := strings.Split(strings.TrimPrefix(r.URL.Path, "/api/vms"), "/")
		name, action := "", ""
		if len(parts) > 1 {
			name = parts[1]
		}
		if len(parts) > 2 {
			action = parts[2]
		}
		switch {
		case r.URL.Path == "/api/images":
			_ = json.NewEncoder(w).Encode([]providerboxes.Image{{Name: "claude-agent", Description: "Claude Code"}})
		case r.Method == http.MethodGet && name == "":
			out := []providerboxes.VM{}
			for _, b := range f.boxes {
				out = append(out, *b)
			}
			_ = json.NewEncoder(w).Encode(out)
		case r.Method == http.MethodPost && name == "":
			var body map[string]string
			_ = json.NewDecoder(r.Body).Decode(&body)
			f.created[body["name"]] = body["image"] + " " + body["size"]
			f.boxes[body["name"]] = &providerboxes.VM{Name: body["name"], State: "running", Ready: true, Image: body["image"], Size: body["size"]}
			_ = json.NewEncoder(w).Encode(f.boxes[body["name"]])
		case f.boxes[name] == nil:
			http.Error(w, "no such box", http.StatusNotFound)
		case action == "exec":
			var body struct{ Command string }
			_ = json.NewDecoder(r.Body).Decode(&body)
			f.execs = append(f.execs, name+": "+body.Command)
			_ = json.NewEncoder(w).Encode(providerboxes.ExecResult{})
		case action == "resume":
			f.resumed = append(f.resumed, name)
			f.boxes[name].State, f.boxes[name].Ready = "running", true
			_ = json.NewEncoder(w).Encode(f.boxes[name])
		case action == "pause":
			f.boxes[name].State, f.boxes[name].Ready = "paused", false
			_ = json.NewEncoder(w).Encode(f.boxes[name])
		case r.Method == http.MethodDelete:
			delete(f.boxes, name)
		case action == "port":
			ws, err := up.Upgrade(w, r, nil)
			if err != nil {
				return
			}
			go func() {
				defer func() { _ = ws.Close() }()
				for {
					mt, msg, err := ws.ReadMessage()
					if err != nil || ws.WriteMessage(mt, msg) != nil {
						return
					}
				}
			}()
		default:
			http.Error(w, "unexpected "+r.Method+" "+r.URL.Path, http.StatusTeapot)
		}
	}))
	t.Cleanup(srv.Close)
	return f, &ProviderBoxes{Client: providerboxes.New(srv.URL, "t"), Config: BoxesConfig{Image: "debian-slim"}}
}

func TestBoxesDeployAuthorizesKey(t *testing.T) {
	f, p := newFakeBoxes(t)
	vm, err := p.Deploy(Vm{Name: "dev", Type: "medium", SSHKeyID: "ssh-ed25519 AAAA me@laptop"})
	if err != nil {
		t.Fatal(err)
	}
	if vm.Name != "dev" || vm.Provider != "boxes" || vm.Type != "medium" || !vm.SSHReady {
		t.Fatalf("got %+v", vm)
	}
	if f.created["dev"] != "debian-slim medium" {
		t.Fatalf("created with %q, want the config's default image and the asked-for size", f.created["dev"])
	}
	if len(f.execs) != 1 || !strings.Contains(f.execs[0], "'ssh-ed25519 AAAA me@laptop'") || !strings.Contains(f.execs[0], "authorized_keys") {
		t.Fatalf("the key wasn't authorized: %q", f.execs)
	}
}

func TestBoxesRefusesMalformedKey(t *testing.T) {
	_, p := newFakeBoxes(t)
	if _, err := p.Deploy(Vm{Name: "dev", SSHKeyID: "ssh-ed25519 AAAA'; rm -rf / #"}); err == nil {
		t.Fatal("a key with a quote in it must not reach a shell")
	}
}

func TestBoxesLifecycle(t *testing.T) {
	_, p := newFakeBoxes(t)
	if _, err := p.Deploy(Vm{Name: "dev"}); err != nil {
		t.Fatal(err)
	}
	if err := p.Pause(Vm{Name: "dev"}, false); err != nil {
		t.Fatal(err)
	}
	vm, err := p.GetByName("dev")
	if err != nil || vm.Status != "paused" || vm.SSHReady {
		t.Fatalf("after pause: %+v %v", vm, err)
	}
	if vm, err = p.Resume(Vm{Name: "dev"}); err != nil || vm.Status != "running" {
		t.Fatalf("after resume: %+v %v", vm, err)
	}
	if err := p.Destroy(Vm{Name: "dev"}); err != nil {
		t.Fatal(err)
	}
	if _, err := p.GetByName("dev"); err == nil {
		t.Fatal("a destroyed box is still listed")
	}
	if paused, _ := p.ListPaused(); len(paused.List) != 0 {
		t.Fatal("ListPaused must be empty: List already includes paused boxes")
	}
	images, err := p.ListImages()
	if err != nil || len(images) != 1 || images[0].Name != "claude-agent" {
		t.Fatalf("images: %+v %v", images, err)
	}
}

func TestBoxesDialResumesPausedBox(t *testing.T) {
	f, p := newFakeBoxes(t)
	if _, err := p.Deploy(Vm{Name: "dev"}); err != nil {
		t.Fatal(err)
	}
	_ = p.Pause(Vm{Name: "dev"}, false)

	conn, err := p.DialVM(context.Background(), Vm{Name: "dev"}, 22)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = conn.Close() }()
	if len(f.resumed) != 1 {
		t.Fatalf("dialing a paused box should resume it first, resumed %v", f.resumed)
	}
	if _, err := conn.Write([]byte("ping")); err != nil {
		t.Fatal(err)
	}
	buf := make([]byte, 4)
	if _, err := io.ReadFull(conn, buf); err != nil || string(buf) != "ping" {
		t.Fatalf("got %q %v", buf, err)
	}

	if _, err := p.DialVM(context.Background(), Vm{Name: "ghost"}, 22); err == nil {
		t.Fatal("dialing a box that doesn't exist should fail")
	}
}

func TestBoxesProxyCommand(t *testing.T) {
	p := &ProviderBoxes{Config: BoxesConfig{OnctlBin: "/Applications/My Tools/onctl"}}
	got := p.proxyCommand("dev", 22)
	want := `'/Applications/My Tools/onctl' ssh-proxy -p boxes dev 22`
	if got != want {
		t.Fatalf("got %s, want %s", got, want)
	}
	if got := (&ProviderBoxes{}).proxyCommand("it's", 2222); got != `onctl ssh-proxy -p boxes 'it'\''s' 2222` {
		t.Fatalf("got %s", got)
	}
}

func newBoxesClient(url string) *providerboxes.Client { return providerboxes.New(url, "t") }
