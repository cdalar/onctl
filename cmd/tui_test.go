package cmd

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/cdalar/onctl/pkg/cloud"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// tuiFakeProvider is a CloudProviderInterface (with sizes and images)
// that records the calls the tui makes, so the tests check the tui
// drives the provider exactly as the subcommands would.
type tuiFakeProvider struct {
	vms     []cloud.Vm // what List returns (a stopped one among them)
	paused  []cloud.Vm // what ListPaused returns
	listErr error      // what List fails with, when set
	calls   []string
}

func (f *tuiFakeProvider) Deploy(v cloud.Vm) (cloud.Vm, error) {
	f.calls = append(f.calls, "deploy "+v.Name)
	return v, nil
}
func (f *tuiFakeProvider) Destroy(v cloud.Vm) error {
	f.calls = append(f.calls, "destroy "+v.Name)
	return nil
}
func (f *tuiFakeProvider) Pause(v cloud.Vm, hot bool) error {
	f.calls = append(f.calls, "pause "+v.Name)
	return nil
}
func (f *tuiFakeProvider) Resume(v cloud.Vm) (cloud.Vm, error) {
	f.calls = append(f.calls, "resume "+v.Name+" key="+v.SSHKeyID)
	return cloud.Vm{Name: v.Name, IP: "10.0.0.9"}, nil
}
func (f *tuiFakeProvider) List() (cloud.VmList, error) {
	return cloud.VmList{List: f.vms}, f.listErr
}
func (f *tuiFakeProvider) ListPaused() (cloud.VmList, error) {
	return cloud.VmList{List: f.paused}, nil
}
func (f *tuiFakeProvider) CreateSSHKey(string) (string, error) {
	f.calls = append(f.calls, "sshkey")
	return "key-1", nil
}
func (f *tuiFakeProvider) SSHInto(string, int, string, []string) {}
func (f *tuiFakeProvider) GetByName(name string) (cloud.Vm, error) {
	for _, v := range append(f.vms, f.paused...) {
		if v.Name == name {
			return v, nil
		}
	}
	return cloud.Vm{}, errors.New("not found")
}
func (f *tuiFakeProvider) ListImages() ([]cloud.CloudImage, error) {
	return []cloud.CloudImage{{Name: "debian-12", Description: "Debian 12"}, {Name: "ubuntu-24.04", Description: "Ubuntu 24.04"}}, nil
}
func (f *tuiFakeProvider) ListSizes() ([]cloud.CloudSize, error) {
	return []cloud.CloudSize{{Name: "small", VCPU: 2, MemoryMiB: 2048, Default: true}, {Name: "large", VCPU: 8, MemoryMiB: 16384}}, nil
}

func tuiFixture() *tuiFakeProvider {
	now := time.Now()
	return &tuiFakeProvider{
		vms: []cloud.Vm{
			{ID: "2", Name: "web", Status: "running", IP: "1.2.3.4", Type: "cpx21", Image: "ubuntu-24.04", Location: "fsn1", Provider: "hetzner",
				CreatedAt: now.Add(-2 * time.Hour), Cost: cloud.CostStruct{Currency: "EUR", CostPerHour: 0.0084, CostPerMonth: 5.49}},
			{ID: "3", Name: "db", Status: "running", IP: "1.2.3.5", Type: "cpx31", Image: "debian-12", Location: "nbg1", Provider: "hetzner", CreatedAt: now.Add(-1 * time.Hour)},
			{ID: "4", Name: "booting", Status: "BUILD", Provider: "hetzner", CreatedAt: now},
			// AWS-style: List includes stopped VMs; ls files them under PAUSED.
			{ID: "5", Name: "stopped-aws", Status: "stopped", Provider: "aws", CreatedAt: now.Add(-3 * time.Hour)},
		},
		paused: []cloud.Vm{
			{ID: "1", Name: "old-paused", Status: "paused", Provider: "hetzner", CreatedAt: now.Add(-48 * time.Hour)},
		},
	}
}

// withCloudProvider sets the provider name global for one test.
func withCloudProvider(t *testing.T, name string) {
	t.Helper()
	orig := cloudProvider
	cloudProvider = name
	t.Cleanup(func() { cloudProvider = orig })
}

func keys(rows []tableRow) string {
	var out []string
	for _, r := range rows {
		out = append(out, r.key)
	}
	return strings.Join(out, " ")
}

// listed is a model with the fixture's VMs in, as the real listing
// (through listVMs) delivers them.
func listed(t *testing.T, f *tuiFakeProvider, w, h int) tuiModel {
	t.Helper()
	m := newTUI(f)
	next, _ := m.Update(tea.WindowSizeMsg{Width: w, Height: h})
	m = next.(tuiModel)
	next, _ = m.Update(m.list("vms")())
	m = next.(tuiModel)
	if m.loading["vms"] {
		t.Fatal("still loading after the listing arrived")
	}
	return m
}

// The vms view is ls's two sections in one table: live VMs first (the
// stopped one List returned filed as paused), newest first, and 0/1/2,
// the filter and shift+letter narrow or reorder it without touching
// the data.
func TestTUIRowsSortAndFilter(t *testing.T) {
	m := listed(t, tuiFixture(), 120, 40)
	if got, want := keys(m.rows()), "booting db web stopped-aws old-paused"; got != want {
		t.Fatalf("rows = %q, want %q", got, want)
	}
	m.stateFilter = showLiveVMs
	if got, want := keys(m.rows()), "booting db web"; got != want {
		t.Errorf("running = %q, want %q", got, want)
	}
	m.stateFilter = showPausedVMs
	if got, want := keys(m.rows()), "stopped-aws old-paused"; got != want {
		t.Errorf("paused = %q, want %q", got, want)
	}
	m.stateFilter = showAllVMs
	for filter, want := range map[string]string{
		"WEB":     "web", // name, case-insensitively
		"nbg1":    "db",  // location
		"debian":  "db",  // image
		"1.2.3.":  "db web",
		"nothing": "",
	} {
		m.filter.SetValue(filter)
		if got := keys(m.rows()); got != want {
			t.Errorf("filter %q = %q, want %q", filter, got, want)
		}
	}
	m.filter.SetValue("")
	// shift-n sorts by name, again the other way; shift-a by age keeps
	// newest first.
	m.top().sortCol, m.top().sortDesc = 0, false
	if got, want := keys(m.rows()), "booting db old-paused stopped-aws web"; got != want {
		t.Errorf("by name = %q, want %q", got, want)
	}
	m.top().sortDesc = true
	if got, want := keys(m.rows()), "web stopped-aws old-paused db booting"; got != want {
		t.Errorf("by name desc = %q, want %q", got, want)
	}
	m.top().sortCol, m.top().sortDesc = 6, false
	if got, want := keys(m.rows()), "booting db web stopped-aws old-paused"; got != want {
		t.Errorf("by age = %q, want %q", got, want)
	}
	// shift-c: the VM with a price sorts after the ones without.
	m.top().sortCol = 7
	if got := keys(m.rows()); !strings.HasSuffix(got, " web") {
		t.Errorf("by cost = %q, want web last", got)
	}
}

// runCmd runs cmd and returns the messages it produces, a batch
// flattened.
func runCmd(cmd tea.Cmd) []tea.Msg {
	if cmd == nil {
		return nil
	}
	msg := cmd()
	batch, ok := msg.(tea.BatchMsg)
	if !ok {
		return []tea.Msg{msg}
	}
	var out []tea.Msg
	for _, c := range batch {
		out = append(out, runCmd(c)...)
	}
	return out
}

// press sends one key to the model.
func press(t *testing.T, m tuiModel, key string) (tuiModel, tea.Cmd) {
	t.Helper()
	var msg tea.KeyMsg
	switch key {
	case "enter":
		msg = tea.KeyMsg{Type: tea.KeyEnter}
	case "esc":
		msg = tea.KeyMsg{Type: tea.KeyEsc}
	case "ctrl+d":
		msg = tea.KeyMsg{Type: tea.KeyCtrlD}
	case "tab":
		msg = tea.KeyMsg{Type: tea.KeyTab}
	case "left":
		msg = tea.KeyMsg{Type: tea.KeyLeft}
	case "right":
		msg = tea.KeyMsg{Type: tea.KeyRight}
	default:
		msg = tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(key)}
	}
	next, cmd := m.Update(msg)
	return next.(tuiModel), cmd
}

// ack is OK on the error popup a refusal opens -- the error stays, in
// full, until it's acknowledged -- after which keys act again.
func ack(t *testing.T, m tuiModel) tuiModel {
	t.Helper()
	if m.dialog == nil || !m.dialog.alert || !strings.Contains(m.View(), "<enter> OK") {
		t.Fatalf("no error popup (flash %q)", m.flash)
	}
	m, _ = press(t, m, "enter")
	if m.dialog != nil {
		t.Fatal("OK didn't close the error popup")
	}
	return m
}

// typeIn sends s one key at a time.
func typeIn(t *testing.T, m tuiModel, s string) tuiModel {
	t.Helper()
	for _, r := range s {
		m, _ = press(t, m, string(r))
	}
	return m
}

// hasAction finds a tuiActionMsg among a command's messages, running
// the provider call behind it.
func hasAction(cmd tea.Cmd) *tuiActionMsg {
	for _, msg := range runCmd(cmd) {
		if am, ok := msg.(tuiActionMsg); ok {
			return &am
		}
	}
	return nil
}

// Every key is the subcommand's provider call, made only when it can
// succeed (pause needs a live VM, resume a paused one, nothing twice
// while one is in flight) and destroy asks first -- the tui must never
// be a faster way to make a mistake than the CLI.
func TestTUIListAndKeys(t *testing.T) {
	tuiFlashFor = time.Millisecond
	withCloudProvider(t, "hetzner")
	f := tuiFixture()
	m := listed(t, f, 140, 40)

	view := m.View()
	for _, want := range []string{"web", "db", "booting", "stopped-aws", "old-paused", "1.2.3.4", "cpx21", "0.0084EUR",
		"VMs(all)[5]", "<vms>", "3 running, 2 paused", "hetzner", "<s>", "shell", `\___/`} {
		if !strings.Contains(view, want) {
			t.Errorf("view lacks %q:\n%s", want, view)
		}
	}

	// 1 shows the running ones; the cursor stays on the first VM.
	m, _ = press(t, m, "1")
	if strings.Contains(m.View(), "old-paused") || !strings.Contains(m.View(), "VMs(running)[3]") {
		t.Errorf("1 didn't narrow to running VMs:\n%s", m.View())
	}
	m, _ = press(t, m, "0")
	if m.currentRow().key != "booting" {
		t.Errorf("cursor on %s, want booting", m.currentRow().key)
	}
	m, _ = press(t, m, "j")
	m, _ = press(t, m, "j")
	if m.currentRow().key != "web" {
		t.Errorf("cursor on %s after j j, want web", m.currentRow().key)
	}
	// Two keys that arrived in one message are two keypresses.
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("kk")})
	m = next.(tuiModel)
	if m.currentRow().key != "booting" {
		t.Errorf("cursor on %s after kk, want booting", m.currentRow().key)
	}
	m, _ = press(t, m, "G")
	if m.currentRow().key != "old-paused" {
		t.Errorf("cursor on %s after G, want old-paused", m.currentRow().key)
	}

	// Pausing a paused VM is refused here, not sent.
	m, cmd := press(t, m, "p")
	if hasAction(cmd) != nil || !m.flashErr || !strings.Contains(m.flash, "not running") {
		t.Errorf("p on a paused VM: flash=%q", m.flash)
	}
	m = ack(t, m)
	// So is ssh.
	m, cmd = press(t, m, "s")
	if cmd == nil || !strings.Contains(m.flash, "resume it first") {
		t.Errorf("s on a paused VM: cmd=%v flash=%q", cmd, m.flash)
	}
	m = ack(t, m)
	// Resuming is sent, the way resume.go does it (the SSH key first),
	// and the VM is pending meanwhile.
	m, cmd = press(t, m, "r")
	if m.pending["old-paused"] != "resume" {
		t.Fatalf("r on a paused VM: pending=%v", m.pending)
	}
	if !strings.Contains(m.View(), "resuming") {
		t.Error("pending VM's row doesn't say resuming")
	}
	am := hasAction(cmd)
	if am == nil || am.err != nil || am.result != "resumed old-paused (IP 10.0.0.9)" {
		t.Fatalf("resume's result: %#v", am)
	}
	if got := strings.Join(f.calls, ", "); got != "sshkey, resume old-paused key=key-1" {
		t.Errorf("provider saw %q", got)
	}
	f.calls = nil
	// Keys on a pending VM are refused until it's done.
	m, _ = press(t, m, "ctrl+d")
	if !strings.Contains(m.flash, "being resumed") {
		t.Errorf("ctrl+d on a pending VM: flash=%q", m.flash)
	}
	m = ack(t, m) // the refusal, not destroy's question
	next, _ = m.Update(*am)
	m = next.(tuiModel)
	if _, ok := m.pending["old-paused"]; ok || m.flash != am.result {
		t.Errorf("after the action: pending=%v flash=%q", m.pending, m.flash)
	}

	// Destroy asks first, in destroy's words; n cancels.
	m, _ = press(t, m, "g")
	m, _ = press(t, m, "ctrl+d")
	if m.dialog == nil || !strings.Contains(m.View(), "Destroy booting?") || !strings.Contains(m.View(), "cannot be undone") {
		t.Errorf("ctrl+d: view:\n%s", m.View())
	}
	m, cmd = press(t, m, "n")
	if m.dialog != nil || hasAction(cmd) != nil {
		t.Errorf("n: dialog=%v", m.dialog)
	}
	// So does enter: destroy's own prompt is [y/N], and a VM is not
	// something to lose to a reflex.
	m, _ = press(t, m, "ctrl+d")
	m, cmd = press(t, m, "enter")
	if m.dialog != nil || hasAction(cmd) != nil {
		t.Errorf("enter: dialog=%v", m.dialog)
	}
	// y sends it.
	m, _ = press(t, m, "ctrl+d")
	m, cmd = press(t, m, "y")
	if m.pending["booting"] != "destroy" {
		t.Fatalf("y: pending=%v", m.pending)
	}
	if am := hasAction(cmd); am == nil || am.err != nil {
		t.Fatalf("destroy's result: %#v", am)
	}
	if got := strings.Join(f.calls, ", "); got != "destroy booting" {
		t.Errorf("provider saw %q", got)
	}
	f.calls = nil
	next, _ = m.Update(tuiActionMsg{name: "booting", verb: "destroy"})
	m = next.(tuiModel)
	if m.flash != "destroyed booting" {
		t.Errorf("flash = %q", m.flash)
	}

	// The filter narrows the rows as it's typed; esc clears it.
	m, _ = press(t, m, "/")
	m = typeIn(t, m, "web")
	if got := keys(m.rows()); got != "web" {
		t.Errorf("filtered rows = %q", got)
	}
	m, _ = press(t, m, "enter")
	if m.filtering || !strings.Contains(m.View(), "</web>") {
		t.Errorf("enter keeps the filter: view:\n%s", m.View())
	}
	m, cmd = press(t, m, "esc")
	if cmd != nil || keys(m.rows()) != "booting db web stopped-aws old-paused" {
		t.Errorf("esc with a filter: rows=%q", keys(m.rows()))
	}

	// Describe and json drill in; esc comes back.
	m, _ = press(t, m, "g")
	m, _ = press(t, m, "j")
	m, _ = press(t, m, "j") // web
	m, _ = press(t, m, "enter")
	if len(m.pages) != 2 || m.top().kind != pageText || !strings.Contains(m.View(), "Public IP:      1.2.3.4") || !strings.Contains(m.View(), "<web>") {
		t.Errorf("enter didn't describe web:\n%s", m.View())
	}
	m, _ = press(t, m, "y")
	if len(m.pages) != 3 || !strings.Contains(m.View(), `"Name": "web"`) || !strings.Contains(m.View(), "<web.json>") {
		t.Errorf("y didn't show web's json:\n%s", m.View())
	}
	// The page's VM takes the VM keys too: pause.go's call, by name.
	m, cmd = press(t, m, "p")
	if hasAction(cmd) == nil || m.pending["web"] != "pause" {
		t.Errorf("p on web's json page: pending=%v", m.pending)
	}
	if got := strings.Join(f.calls, ", "); got != "pause web" {
		t.Errorf("provider saw %q", got)
	}
	next, _ = m.Update(tuiActionMsg{name: "web", verb: "pause"})
	m = next.(tuiModel)
	m, _ = press(t, m, "esc")
	m, _ = press(t, m, "esc")
	if len(m.pages) != 1 || m.top().view != vmsView {
		t.Errorf("esc esc didn't come back: %d pages", len(m.pages))
	}

	// :images and :sizes are the other listings; `:vms web` filters.
	m, _ = press(t, m, ":")
	m = typeIn(t, m, "images")
	m, cmd = press(t, m, "enter")
	for _, msg := range runCmd(cmd) {
		next, _ = m.Update(msg)
		m = next.(tuiModel)
	}
	if m.top().view != imagesView || !strings.Contains(m.View(), "Images[2]") || !strings.Contains(m.View(), "Debian 12") {
		t.Errorf(":images: view:\n%s", m.View())
	}
	m, _ = press(t, m, ":")
	m = typeIn(t, m, "sizes")
	m, cmd = press(t, m, "enter")
	for _, msg := range runCmd(cmd) {
		next, _ = m.Update(msg)
		m = next.(tuiModel)
	}
	if m.top().view != sizesView || !strings.Contains(m.View(), "Sizes[2]") || !strings.Contains(m.View(), "16 GiB") {
		t.Errorf(":sizes: view:\n%s", m.View())
	}
	// A sorts by ARCH (R is taken by refresh). A tick refreshes the VMs
	// only -- the catalog was listed when the view opened and is a
	// handful of API calls on some providers -- and R lists it again.
	m, _ = press(t, m, "A")
	if m.top().sortCol != 4 {
		t.Errorf("A: sortCol=%d, want ARCH (4)", m.top().sortCol)
	}
	next, _ = m.Update(tuiTickMsg(time.Now()))
	m = next.(tuiModel)
	if m.loading["sizes"] || !m.loading["vms"] {
		t.Errorf("tick: loading=%v, want vms only", m.loading)
	}
	next, _ = m.Update(tuiListMsg{resource: "vms", vms: m.vms})
	m = next.(tuiModel)
	m, _ = press(t, m, "R")
	if !m.loading["sizes"] || !m.loading["vms"] {
		t.Errorf("R: loading=%v, want sizes and vms", m.loading)
	}
	next, _ = m.Update(tuiListMsg{resource: "vms", vms: m.vms})
	m = next.(tuiModel)
	next, _ = m.Update(tuiListMsg{resource: "sizes", sizes: m.sizes})
	m = next.(tuiModel)
	m, _ = press(t, m, ":")
	m = typeIn(t, m, "vms web")
	m, _ = press(t, m, "enter")
	if m.top().view != vmsView || keys(m.rows()) != "web" {
		t.Errorf(":vms web: rows=%q", keys(m.rows()))
	}

	// An unknown command is refused; :q quits.
	m, _ = press(t, m, ":")
	m = typeIn(t, m, "pods")
	m, _ = press(t, m, "enter")
	if !m.flashErr || !strings.Contains(m.flash, "unknown command") {
		t.Errorf(":pods: flash=%q", m.flash)
	}
	m = ack(t, m)
	m, _ = press(t, m, ":")
	m = typeIn(t, m, "q")
	_, cmd = press(t, m, "enter")
	if cmd == nil {
		t.Fatal(":q returned no command")
	}
	if _, ok := cmd().(tea.QuitMsg); !ok {
		t.Error(":q didn't quit")
	}
}

// A failed listing is shown, not fatal, and partial VM data (one of
// List/ListPaused failing) is still worth showing.
func TestTUIListError(t *testing.T) {
	tuiFlashFor = time.Millisecond
	m := newTUI(nil)
	next, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	m = next.(tuiModel)
	m.pages = []page{{kind: pageTable, view: imagesView, sortCol: -1}}
	next, _ = m.Update(tuiListMsg{resource: "images", err: errors.New("no api token")})
	m = next.(tuiModel)
	// The flash says so now, and the table says so after the flash is
	// gone: a failed listing is not "Loading…" for ever.
	if !m.flashErr || m.flash != "listing images: no api token" || !m.loaded["images"] ||
		!strings.Contains(m.View(), "Listing images failed: no api token") {
		t.Errorf("flash=%q loaded=%v view:\n%s", m.flash, m.loaded, m.View())
	}
	// A listing that works clears it.
	next, _ = m.Update(tuiListMsg{resource: "images", images: []cloud.CloudImage{{Name: "debian-12"}}})
	m = next.(tuiModel)
	if m.listErr["images"] != nil || strings.Contains(m.View(), "failed") || !strings.Contains(m.View(), "debian-12") {
		t.Errorf("after a good listing: listErr=%v view:\n%s", m.listErr, m.View())
	}
	next, _ = m.Update(tuiListMsg{resource: "vms", vms: []cloud.Vm{{Name: "web", Status: "running"}}, err: errors.New("snapshots: 403")})
	m = next.(tuiModel)
	if len(m.vms) != 1 || !m.loaded["vms"] || !strings.Contains(m.flash, "snapshots: 403") {
		t.Errorf("partial listing: vms=%d loaded=%v flash=%q", len(m.vms), m.loaded, m.flash)
	}
}

// The child's own last line is the reason shown, not exec's "exit
// status 1".
func TestTUIChildDone(t *testing.T) {
	tuiFlashFor = time.Millisecond
	m := newTUI(nil)
	next, _ := m.Update(tuiChildDoneMsg{verb: "ssh", name: "web", err: errors.New("exit status 1"),
		stderr: "2026/10/21 [DEBUG] dialing\nError: vm web not found\n\n"})
	m = next.(tuiModel)
	if !m.flashErr || m.flash != "ssh web: vm web not found" {
		t.Errorf("flash = %q", m.flash)
	}
	// A non-zero status with nothing on stderr is the remote shell's
	// (`onctl ssh` exits with it), not a connection that failed.
	m.flash = ""
	next, _ = m.Update(tuiChildDoneMsg{verb: "ssh", name: "web", err: errors.New("exit status 1")})
	m = next.(tuiModel)
	if m.flash != "" {
		t.Errorf("flash = %q, want none", m.flash)
	}
	next, _ = m.Update(tuiChildDoneMsg{verb: "create", name: "", err: errors.New("exit status 1")})
	m = next.(tuiModel)
	if m.flash != "create a VM: exit status 1" {
		t.Errorf("flash = %q", m.flash)
	}
	next, _ = m.Update(tuiChildDoneMsg{verb: "create", name: "my-vm"})
	m = next.(tuiModel)
	if m.flashErr || m.flash != "created my-vm" {
		t.Errorf("flash = %q", m.flash)
	}
	// `onctl claude` exits with the attach's status, like ssh: quiet
	// when nothing was said, its own last word otherwise.
	m.flash = ""
	next, _ = m.Update(tuiChildDoneMsg{verb: "claude", name: "claude-app", err: errors.New("exit status 1")})
	m = next.(tuiModel)
	if m.flash != "" {
		t.Errorf("flash = %q, want none", m.flash)
	}
	next, _ = m.Update(tuiChildDoneMsg{verb: "claude", name: "claude-app", err: errors.New("exit status 1"),
		stderr: "Error: claude-app holds /x, not /y -- run onctl claude from there, or pick another box with --box\n"})
	m = next.(tuiModel)
	if !m.flashErr || !strings.HasPrefix(m.flash, "claude on claude-app: claude-app holds /x") {
		t.Errorf("flash = %q", m.flash)
	}
}

// The children land on the same cloud and account as the tui: the
// resolved provider always, and any root flag that was given.
func TestTUIChildArgs(t *testing.T) {
	withCloudProvider(t, "gcp")
	flag := rootCmd.PersistentFlags().Lookup("project")
	if err := flag.Value.Set("my-proj"); err != nil {
		t.Fatal(err)
	}
	flag.Changed = true
	t.Cleanup(func() { flag.Changed = false; _ = flag.Value.Set("") })
	got := strings.Join(tuiChildArgs("ssh", "web"), " ")
	if got != "ssh web --provider gcp --project my-proj" {
		t.Errorf("args = %q", got)
	}
	got = strings.Join(createArgs("", "debian-12", ""), " ")
	if got != "create --image debian-12 --provider gcp --project my-proj" {
		t.Errorf("create args = %q", got)
	}
	// a / :claude <vm> is `onctl claude --box <vm>` on the same cloud.
	got = strings.Join(claudeArgs("web"), " ")
	if got != "claude --box web --provider gcp --project my-proj" {
		t.Errorf("claude args = %q", got)
	}
}

// a and :claude hand the terminal to `onctl claude --box <vm>`; with
// no VM named, the box is the project's own (claude-<dir>), as the CLI
// would pick -- a child process, never a provider call from here.
func TestTUIClaudeKeys(t *testing.T) {
	tuiFlashFor = time.Millisecond
	withCloudProvider(t, "hetzner")
	f := tuiFixture()
	m := listed(t, f, 140, 40)
	if !strings.Contains(m.View(), "<a>") || !strings.Contains(m.View(), "claude") {
		t.Errorf("header lacks the a/claude hotkey:\n%s", m.View())
	}
	m, _ = press(t, m, "G") // old-paused: onctl claude resumes it itself
	m, cmd := press(t, m, "a")
	if cmd == nil || m.flash != "" {
		t.Errorf("a: cmd=%v flash=%q", cmd, m.flash)
	}
	m, _ = press(t, m, ":")
	m = typeIn(t, m, "claude")
	m, cmd = press(t, m, "enter")
	if cmd == nil || m.flash != "" {
		t.Errorf(":claude: cmd=%v flash=%q", cmd, m.flash)
	}
	if len(f.calls) != 0 {
		t.Errorf("provider saw %v, want nothing", f.calls)
	}
}

// :provider switches clouds the way k9s's :ctx switches clusters: a
// name is checked here, the current one is a no-op, and a switch quits
// the tui for tuiCmd to start it again on the other provider; without
// a name, the providers view lists them and enter picks one.
func TestTUIProviderSwitch(t *testing.T) {
	tuiFlashFor = time.Millisecond
	withCloudProvider(t, "hetzner")
	m := listed(t, tuiFixture(), 120, 40)
	run := func(m tuiModel, line string) (tuiModel, tea.Cmd) {
		m, _ = press(t, m, ":")
		m = typeIn(t, m, line)
		return press(t, m, "enter")
	}
	m, _ = run(m, "provider nope")
	if !m.flashErr || !strings.Contains(m.flash, "unknown provider") || m.switchTo != "" {
		t.Errorf(":provider nope: flash=%q switchTo=%q", m.flash, m.switchTo)
	}
	m = ack(t, m)
	m, _ = run(m, "provider hetzner")
	if m.flashErr || m.flash != "already on hetzner" || m.switchTo != "" {
		t.Errorf(":provider hetzner: flash=%q switchTo=%q", m.flash, m.switchTo)
	}
	m, cmd := run(m, "provider aws")
	if m.switchTo != "aws" || cmd == nil {
		t.Fatalf(":provider aws: switchTo=%q cmd=%v", m.switchTo, cmd)
	}
	if _, ok := cmd().(tea.QuitMsg); !ok {
		t.Error(":provider aws didn't quit")
	}

	m = listed(t, tuiFixture(), 120, 40)
	m, cmd = run(m, "provider")
	if cmd != nil && len(runCmd(cmd)) != 0 {
		t.Errorf(":provider listed something: %v", runCmd(cmd))
	}
	if m.top().view != providersView || keys(m.rows()) != strings.Join(cloudProviderList, " ") {
		t.Errorf(":provider: view=%v rows=%q", m.top().view, keys(m.rows()))
	}
	// Each provider by its full name and where it runs -- "ac" or "ch"
	// alone tells you nothing -- the current one marked.
	if view := regexp.MustCompile(`\x1b\[[0-9;]*m`).ReplaceAllString(m.View(), ""); !strings.Contains(view, "Providers["+strconv.Itoa(len(cloudProviderList))+"]") ||
		!regexp.MustCompile(`hetzner +Hetzner Cloud +cloud, BYO account +\*`).MatchString(view) ||
		!regexp.MustCompile(`ac +Apple Container +local `).MatchString(view) {
		t.Errorf(":provider view:\n%s", view)
	}
	// A provider added without a full name would show as "-" here.
	for _, r := range m.rows() {
		if r.cells[1] == "-" {
			t.Errorf("provider %s has no full name in providerPlaces", r.key)
		}
	}
	m, _ = press(t, m, "j")
	m, _ = press(t, m, "j") // azure
	m, cmd = press(t, m, "enter")
	if m.switchTo != "azure" || cmd == nil {
		t.Errorf("enter on azure: switchTo=%q cmd=%v", m.switchTo, cmd)
	}
	// The tui's next run says how it went.
	m = newTUI(tuiFixture())
	m.startFlash, m.startFlashErr = "switching to azure: no config", true
	var flashed bool
	for _, msg := range runCmd(m.Init()) {
		if fm, ok := msg.(tuiFlashMsg); ok {
			next, _ := m.Update(fm)
			m = next.(tuiModel)
			flashed = true
		}
	}
	if !flashed || !m.flashErr || m.flash != "switching to azure: no config" {
		t.Errorf("start flash: flashed=%v flash=%q", flashed, m.flash)
	}
}

// A switch whose config can't be loaded leaves the provider the tui
// had, in the global and in ONCTL_CLOUD (which the children read), so
// the tui comes back on it with the reason.
// Tab at the `:` prompt completes the command: a unique prefix fills in
// (the point: `:pro` means providers, not a typo to retype), several
// candidates grow to their shared prefix and are flashed, and an argument
// is left alone.
func TestTUICommandComplete(t *testing.T) {
	tuiFlashFor = time.Millisecond
	m := listed(t, tuiFixture(), 120, 40)
	m, _ = press(t, m, ":")
	m = typeIn(t, m, "pro")
	m, _ = press(t, m, "tab")
	if got := m.command.Value(); got != "providers " {
		t.Errorf(":pro<tab> = %q, want %q", got, "providers ")
	}
	m, _ = press(t, m, "esc")

	m, _ = press(t, m, ":")
	m = typeIn(t, m, "S")
	m, _ = press(t, m, "tab")
	if got := m.command.Value(); got != "s" {
		t.Errorf(":S<tab> = %q, want the shared prefix %q", got, "s")
	}
	if m.flashErr || !strings.Contains(m.flash, "sizes") || !strings.Contains(m.flash, "ssh") {
		t.Errorf(":S<tab> flash=%q, want the candidates", m.flash)
	}
	m, _ = press(t, m, "esc")

	m, _ = press(t, m, ":")
	m = typeIn(t, m, "ssh we")
	m, _ = press(t, m, "tab")
	if got := m.command.Value(); got != "ssh we" {
		t.Errorf(":ssh we<tab> = %q, want it unchanged", got)
	}
	m, _ = press(t, m, "esc")

	// The completed word runs as typed: `:providers hetzner` switches
	// like `:provider hetzner`, so completion needs one spelling only.
	withCloudProvider(t, "hetzner")
	m, _ = press(t, m, ":")
	m = typeIn(t, m, "providers nope")
	m, _ = press(t, m, "enter")
	if !m.flashErr || !strings.Contains(m.flash, "unknown provider") {
		t.Errorf(":providers nope: flash=%q", m.flash)
	}
}

func TestTUISwitchProviderRestores(t *testing.T) {
	withCloudProvider(t, "hetzner")
	origConfig := flagConfigFile
	flagConfigFile = t.TempDir() + "/missing.yaml"
	t.Cleanup(func() { flagConfigFile = origConfig })

	if err := switchProvider("nope"); err == nil || !strings.Contains(err.Error(), "unsupported provider") {
		t.Errorf("switchProvider(nope) = %v", err)
	}
	t.Setenv("ONCTL_CLOUD", "hetzner")
	err := switchProvider("aws")
	if err == nil || !strings.Contains(err.Error(), "switching to aws") || !strings.Contains(err.Error(), "missing.yaml") {
		t.Errorf("switchProvider(aws) = %v", err)
	}
	if cloudProvider != "hetzner" || os.Getenv("ONCTL_CLOUD") != "hetzner" {
		t.Errorf("after a failed switch: cloudProvider=%q ONCTL_CLOUD=%q", cloudProvider, os.Getenv("ONCTL_CLOUD"))
	}
	if err := os.Unsetenv("ONCTL_CLOUD"); err != nil {
		t.Fatal(err)
	}
	if err := switchProvider("aws"); err == nil {
		t.Error("switchProvider(aws) with no config succeeded")
	}
	if _, set := os.LookupEnv("ONCTL_CLOUD"); set || cloudProvider != "hetzner" {
		t.Errorf("after a failed switch from no ONCTL_CLOUD: set=%v cloudProvider=%q", set, cloudProvider)
	}
}

func TestTUIDescribe(t *testing.T) {
	lines := strings.Join(describeVM(tuiFixture().vms[0]), "\n")
	for _, want := range []string{"Name:           web", "State:          running", "Public IP:      1.2.3.4", "Type:           cpx21",
		"Per Hour:       0.0084 EUR", "Per Month:      5.49 EUR"} {
		if !strings.Contains(lines, want) {
			t.Errorf("describe lacks %q:\n%s", want, lines)
		}
	}
	// No cost block without a price; fc/ch say whether sshd is up.
	lines = strings.Join(describeVM(cloud.Vm{Name: "mv", Provider: "fc", Status: "running", SSHReady: true}), "\n")
	if strings.Contains(lines, "Cost:") || !strings.Contains(lines, "SSH Ready:      yes") {
		t.Errorf("fc describe:\n%s", lines)
	}
}

// The form is `onctl create`'s flags: typed, or walked through the
// provider's catalog, and left off when empty so create's own defaults
// apply. A name that exists is refused before anything runs.
func TestTUICreateForm(t *testing.T) {
	tuiFlashFor = time.Millisecond
	withCloudProvider(t, "test")
	f := tuiFixture()
	m := listed(t, f, 100, 24)
	m, cmd := press(t, m, "c")
	if m.create == nil {
		t.Fatal("c opened no form")
	}
	// Enter before the catalog is in is refused.
	m, _ = press(t, m, "enter")
	if m.create == nil || !strings.Contains(m.flash, "still listing") {
		t.Errorf("early enter: flash=%q", m.flash)
	}
	m = ack(t, m)
	for _, msg := range runCmd(cmd) {
		if c, ok := msg.(tuiCatalogMsg); ok {
			next, _ := m.Update(c)
			m = next.(tuiModel)
		}
	}
	form := m.create
	if !form.loaded || len(form.images) != 2 || len(form.sizes) != 2 {
		t.Fatalf("catalog: loaded=%v images=%d sizes=%d", form.loaded, len(form.images), len(form.sizes))
	}
	// Type a name, then tab to type and walk it: right from empty is the
	// first size, left from the first wraps to the last.
	m = typeIn(t, m, "my-vm")
	m, _ = press(t, m, "tab")
	m, _ = press(t, m, "tab")
	m, _ = press(t, m, "right")
	if form.field != createFieldType || form.typ.Value() != "small" {
		t.Errorf("after tab tab right: field=%d type=%q", form.field, form.typ.Value())
	}
	m, _ = press(t, m, "left")
	if form.typ.Value() != "large" {
		t.Errorf("after left: type=%q", form.typ.Value())
	}
	view := m.View()
	for _, want := range []string{"my-vm", "large", "8 vCPU, 16 GiB", "Create a VM", "<create>", "2 images listed"} {
		if !strings.Contains(view, want) {
			t.Errorf("form lacks %q:\n%s", want, view)
		}
	}
	// A name that exists is refused.
	form.name.SetValue("web")
	m, cmd = press(t, m, "enter")
	if m.create == nil || cmd == nil || !strings.Contains(m.flash, "already exists") {
		t.Errorf("enter with an existing name: flash=%q", m.flash)
	}
	m = ack(t, m)
	if m.create == nil {
		t.Fatal("OK on the refusal closed the form too")
	}
	form.name.SetValue("my-vm")
	m, cmd = press(t, m, "enter")
	if m.create != nil || cmd == nil {
		t.Fatalf("enter: form=%v cmd=%v", m.create, cmd)
	}
	if got := strings.Join(createArgs("my-vm", "", "large"), " "); got != "create -n my-vm --type large --provider test" {
		t.Errorf("create args = %q", got)
	}

	// From the sizes view, c fills the row's size in.
	m.sizes, _ = f.ListSizes()
	m.loaded["sizes"] = true
	m.pages = []page{{kind: pageTable, view: sizesView, sortCol: -1}}
	m, _ = press(t, m, "G")
	m, _ = press(t, m, "c")
	if m.create == nil || m.create.typ.Value() != "large" {
		t.Errorf("c from sizes: form=%v", m.create)
	}
	m, _ = press(t, m, "esc")
	if m.create != nil {
		t.Error("esc didn't close the form")
	}
}

// :repos is gh's listing, not the provider's: the rows come from the
// cache when there is one and from GitHub otherwise (or when stale, or
// on R), never on the tick; enter hands the terminal to `onctl claude
// --repo` on the tui's provider, and d says so.
func TestTUIRepos(t *testing.T) {
	tuiFlashFor = time.Millisecond
	withCloudProvider(t, "hetzner")
	cache := filepath.Join(t.TempDir(), "repos.json")
	origPath, origList := repoCachePath, listGitHubRepos
	t.Cleanup(func() { repoCachePath, listGitHubRepos = origPath, origList })
	repoCachePath = func() (string, error) { return cache, nil }
	now := time.Now()
	fetched := 0
	listGitHubRepos = func(context.Context) ([]ghRepo, error) {
		fetched++
		return []ghRepo{
			{FullName: "cdalar/onctl", Description: "the CLI", Language: "Go", PushedAt: now.Add(-time.Hour)},
			{FullName: "acme/web", Private: true, Fork: true, Language: "TypeScript", PushedAt: now.Add(-48 * time.Hour)},
			{FullName: "acme/old", Archived: true, PushedAt: now.Add(-24 * 30 * time.Hour)},
		}, nil
	}

	f := tuiFixture()
	m := listed(t, f, 140, 40)
	// :repos with no cache asks GitHub.
	m, _ = press(t, m, ":")
	m = typeIn(t, m, "repos")
	m, cmd := press(t, m, "enter")
	if m.top().view != reposView || cmd == nil || !m.loading["repos"] {
		t.Fatalf("view=%v loading=%v cmd=%v", m.top().view, m.loading, cmd != nil)
	}
	var listedMsg *tuiListMsg
	for _, msg := range runCmd(cmd) {
		if lm, ok := msg.(tuiListMsg); ok && lm.resource == "repos" {
			listedMsg = &lm
		}
	}
	if listedMsg == nil || listedMsg.err != nil || fetched != 1 {
		t.Fatalf("listing: %#v fetched=%d", listedMsg, fetched)
	}
	next, _ := m.Update(*listedMsg)
	m = next.(tuiModel)
	if got, want := keys(m.rows()), "cdalar/onctl acme/web acme/old"; got != want {
		t.Fatalf("rows = %q, want %q", got, want)
	}
	view := m.View()
	for _, want := range []string{"GitHub Repos[3]", "<repos>", "private fork", "archived public", "TypeScript", "the CLI", "<enter>", "claude on a box"} {
		if !strings.Contains(view, want) {
			t.Errorf("view lacks %q:\n%s", want, view)
		}
	}
	// The tick refreshes the VMs only; GitHub isn't asked again.
	next, _ = m.Update(tuiTickMsg(now))
	m = next.(tuiModel)
	if m.loading["repos"] || fetched != 1 {
		t.Errorf("the tick asked GitHub: loading=%v fetches=%d", m.loading, fetched)
	}
	// shift-p by PUSHED keeps newest first; the other way round reverses it.
	m, _ = press(t, m, "P")
	if got, want := keys(m.rows()), "cdalar/onctl acme/web acme/old"; got != want {
		t.Errorf("by pushed = %q, want %q", got, want)
	}
	m, _ = press(t, m, "P")
	if got, want := keys(m.rows()), "acme/old acme/web cdalar/onctl"; got != want {
		t.Errorf("by pushed desc = %q, want %q", got, want)
	}
	m, _ = press(t, m, "P")
	m.filter.SetValue("acme")
	if got, want := keys(m.rows()), "acme/web acme/old"; got != want {
		t.Errorf("filtered = %q, want %q", got, want)
	}
	m.filter.SetValue("")
	m, _ = press(t, m, "g")

	// d describes the row; the page names the command enter runs, on
	// this provider.
	m, _ = press(t, m, "d")
	if m.top().kind != pageText || !strings.Contains(m.View(), "onctl claude --repo cdalar/onctl --provider hetzner") || !strings.Contains(m.View(), "claude-onctl") {
		t.Errorf("describe page:\n%s", m.View())
	}
	m, _ = press(t, m, "esc")
	// y is GitHub's record of it.
	m, _ = press(t, m, "y")
	if m.top().kind != pageText || m.top().title != "cdalar/onctl.json" || !strings.Contains(m.View(), `"full_name": "cdalar/onctl"`) {
		t.Errorf("json page: title=%q\n%s", m.top().title, m.View())
	}
	m, _ = press(t, m, "esc")
	// enter asks which box -- a new one, or the repo's own, claude-onctl,
	// not made yet here -- and esc cancels; n or r hands the terminal to
	// onctl claude --repo. The VM keys do nothing here (there's no VM
	// under the cursor).
	m, cmd = press(t, m, "enter")
	if cmd != nil || m.dialog == nil || len(m.dialog.choices) != 2 {
		t.Fatalf("enter: cmd=%v dialog=%v", cmd != nil, m.dialog)
	}
	for _, want := range []string{"Claude on cdalar/onctl", "<n> new box", "<r> repo's box", "<esc> cancel", "claude-onctl (not made yet", "onctl-<word>-<word>-<id>"} {
		if !strings.Contains(m.View(), want) {
			t.Errorf("dialog lacks %q:\n%s", want, m.View())
		}
	}
	m, cmd = press(t, m, "esc")
	if cmd != nil || m.dialog != nil || m.flash != "" {
		t.Errorf("esc: cmd=%v dialog=%v flash=%q", cmd != nil, m.dialog != nil, m.flash)
	}
	for _, k := range []string{"n", "r"} {
		m, _ = press(t, m, "enter")
		m, cmd = press(t, m, k)
		if cmd == nil || m.dialog != nil || m.flash != "" {
			t.Errorf("enter, %s: cmd=%v dialog=%v flash=%q", k, cmd != nil, m.dialog != nil, m.flash)
		}
	}
	// With the repo's box there, the dialog says so.
	m.vms = []cloud.Vm{{Name: "claude-onctl", Status: "running"}}
	m, _ = press(t, m, "enter")
	if !strings.Contains(m.View(), "claude-onctl (running, with what was done on it)") {
		t.Errorf("dialog with the box there:\n%s", m.View())
	}
	m, _ = press(t, m, "esc")
	m.vms = nil
	for _, k := range []string{"p", "r", "s", "ctrl+d"} {
		m, cmd = press(t, m, k)
		if cmd != nil || m.dialog != nil {
			t.Errorf("%s in :repos: cmd=%v dialog=%v", k, cmd != nil, m.dialog != nil)
		}
	}
	if len(f.calls) != 0 {
		t.Errorf("provider saw %v, want nothing", f.calls)
	}

	// The list was cached: a fresh model shows it at once and doesn't
	// ask GitHub again...
	m2 := newTUI(f)
	cmd = m2.openView(reposView)
	if !m2.loaded["repos"] || m2.loading["repos"] || len(m2.repos) != 3 || cmd != nil && fetched != 1 {
		t.Fatalf("from the cache: loaded=%v loading=%v repos=%d fetched=%d", m2.loaded, m2.loading, len(m2.repos), fetched)
	}
	// ...R does...
	m2, cmd = press(t, m2, "R")
	runCmd(cmd)
	if fetched != 2 {
		t.Errorf("R: fetched %d times in all, want 2", fetched)
	}
	// ...and so does opening it once the cache is an hour old, with the
	// cache shown meanwhile.
	stale, _ := json.Marshal(repoCache{FetchedAt: now.Add(-2 * time.Hour), Repos: []ghRepo{{FullName: "cdalar/stale"}}})
	if err := os.WriteFile(cache, stale, 0o600); err != nil {
		t.Fatal(err)
	}
	m3 := newTUI(f)
	cmd = m3.openView(reposView)
	if keys(m3.rows()) != "cdalar/stale" || !m3.loading["repos"] {
		t.Errorf("stale cache: rows=%q loading=%v", keys(m3.rows()), m3.loading)
	}
	// applyRepos feeds a listing's outcome back to the model.
	applyRepos := func(m tuiModel, cmd tea.Cmd) tuiModel {
		for _, msg := range runCmd(cmd) {
			if lm, ok := msg.(tuiListMsg); ok && lm.resource == "repos" {
				next, _ := m.Update(lm)
				m = next.(tuiModel)
			}
		}
		return m
	}
	m3 = applyRepos(m3, cmd)
	if fetched != 3 || keys(m3.rows()) != "cdalar/onctl acme/web acme/old" {
		t.Errorf("stale cache: fetched %d times in all, want 3; rows=%q", fetched, keys(m3.rows()))
	}
	if c := readRepoCache(); c == nil || len(c.Repos) != 3 || time.Since(c.FetchedAt) > time.Minute {
		t.Errorf("cache after the fetch: %#v", c)
	}

	// A listing that fails keeps what was shown and says why.
	listGitHubRepos = func(context.Context) ([]ghRepo, error) { return nil, errors.New("gh isn't installed") }
	m3, cmd = press(t, m3, "R")
	m3 = applyRepos(m3, cmd)
	if keys(m3.rows()) != "cdalar/onctl acme/web acme/old" || !m3.flashErr || !strings.Contains(m3.flash, "gh isn't installed") {
		t.Errorf("failed listing: rows=%q flash=%q", keys(m3.rows()), m3.flash)
	}
}

// The flash after `onctl claude --repo` ends is cobra's Error line --
// usage and main's log line come after it on stderr -- or, failing
// that, whatever was said last.
func TestTUIClaudeDone(t *testing.T) {
	tuiFlashFor = time.Millisecond
	m := newTUI(nil)
	next, _ := m.Update(tuiClaudeDoneMsg{repo: "cdalar/onctl", err: errors.New("exit status 1"),
		stderr: "Cloning cdalar/onctl on claude-onctl...\nfatal: repository not found\nError: cloning cdalar/onctl on claude-onctl: exit status 128\nUsage:\n  onctl claude [-- claude-args...] [flags]\n\n2026/10/23 10:00:00 cloning cdalar/onctl on claude-onctl: exit status 128\n"})
	m = next.(tuiModel)
	if !m.flashErr || m.flash != "claude on cdalar/onctl: cloning cdalar/onctl on claude-onctl: exit status 128" {
		t.Errorf("flash = %q", m.flash)
	}
	next, _ = m.Update(tuiClaudeDoneMsg{repo: "cdalar/onctl", err: errors.New("signal: killed"), stderr: "ssh: connection closed\n"})
	m = next.(tuiModel)
	if m.flash != "claude on cdalar/onctl: ssh: connection closed" {
		t.Errorf("flash = %q", m.flash)
	}
	next, _ = m.Update(tuiClaudeDoneMsg{repo: "cdalar/onctl", err: errors.New("exit status 1")})
	m = next.(tuiModel)
	if m.flash != "claude on cdalar/onctl: exit status 1" {
		t.Errorf("flash = %q", m.flash)
	}
	next, _ = m.Update(tuiClaudeDoneMsg{repo: "cdalar/onctl", box: "onctl-quiet-otter-1a2b"})
	m = next.(tuiModel)
	if m.flashErr || m.flash != "claude on cdalar/onctl ended (onctl-quiet-otter-1a2b is kept)" {
		t.Errorf("flash = %q", m.flash)
	}
	// On the repo's own box there's no box to name: it's the usual one.
	next, _ = m.Update(tuiClaudeDoneMsg{repo: "cdalar/onctl"})
	m = next.(tuiModel)
	if m.flashErr || m.flash != "claude on cdalar/onctl ended" {
		t.Errorf("flash = %q", m.flash)
	}
}

// enter in :repos names a new box each time -- the repository's name,
// two words and an id, never one of the VMs listed -- so that two
// pieces of work on a repository never share a box (--repo alone would
// come back to claude-<name>).
func TestTUIRepoBoxName(t *testing.T) {
	withCloudProvider(t, "hetzner")
	m := newTUI(nil)
	m.vms = []cloud.Vm{{Name: "claude-onctl"}, {Name: "onctl-brave-otter-1a2b"}}
	seen := map[string]bool{}
	for range 5 {
		name, err := m.repoBoxName("cdalar/Onctl")
		if err != nil {
			t.Fatal(err)
		}
		if !regexp.MustCompile(`^onctl-[a-z]+-[a-z]+-[0-9a-f]{4}$`).MatchString(name) || name == "onctl-brave-otter-1a2b" {
			t.Errorf("box name %q", name)
		}
		if seen[name] {
			t.Errorf("box name %q twice", name)
		}
		seen[name] = true
	}
	if got := claudeRepoArgs("cdalar/onctl", "onctl-quiet-otter-1a2b"); strings.Join(got, " ") != "claude --repo cdalar/onctl --box onctl-quiet-otter-1a2b --provider hetzner" {
		t.Errorf("args = %q", got)
	}
	// "" is the repo's own box: --repo alone comes back to claude-<name>.
	if got := claudeRepoArgs("cdalar/onctl", ""); strings.Join(got, " ") != "claude --repo cdalar/onctl --provider hetzner" {
		t.Errorf("args = %q", got)
	}
}

func TestParseRepoPage(t *testing.T) {
	repos, err := parseRepoPage([]byte(`[{"full_name":"cdalar/onctl","description":null,"private":false,"fork":false,"archived":false,
		"language":"Go","default_branch":"main","pushed_at":"2026-10-23T14:17:26Z","html_url":"https://github.com/cdalar/onctl","permissions":{"push":true}},
		{"full_name":"acme/empty","pushed_at":null,"permissions":{"push":false}}]`))
	if err != nil || len(repos) != 2 {
		t.Fatalf("parse: %v, %d repos", err, len(repos))
	}
	if r := repos[0]; r.FullName != "cdalar/onctl" || r.Language != "Go" || !r.Permissions.Push || r.PushedAt.Year() != 2026 {
		t.Errorf("repo = %#v", r)
	}
	if r := repos[1]; !r.PushedAt.IsZero() || r.Permissions.Push {
		t.Errorf("empty repo = %#v", r)
	}
	if _, err := parseRepoPage([]byte(`{"message":"Bad credentials"}`)); err == nil {
		t.Error("an error page parsed")
	}
}

// TestTUIActionRefreshes: the periodic tick is slow, so an action's
// completion, succeeded or failed, must list the VMs at once; the row
// would otherwise show a stale state for up to tuiRefreshEvery.
func TestTUIActionRefreshes(t *testing.T) {
	tuiFlashFor = time.Millisecond
	for _, msg := range []tea.Msg{
		tuiActionMsg{name: "web", verb: "pause"},
		tuiActionMsg{name: "web", verb: "pause", err: errors.New("boom")},
		tuiChildDoneMsg{verb: "create", name: "my-vm"},
		tuiChildDoneMsg{verb: "ssh", name: "web", err: errors.New("exit status 1")},
		tuiClaudeDoneMsg{repo: "app"},
		tuiClaudeDoneMsg{repo: "app", err: errors.New("boom")},
	} {
		m := newTUI(nil)
		next, cmd := m.Update(msg)
		m = next.(tuiModel)
		if !m.loading["vms"] || cmd == nil {
			t.Errorf("%T did not refresh the VMs", msg)
		}
	}
}

// The startup pick shows each provider's full name next to the short one,
// so a reader who doesn't know "ac" sees "Apple Container"; a provider
// added to cloudProviderList without a title would show up blank.
func TestTUIProviderChoices(t *testing.T) {
	items := providerChoices()
	if len(items) != len(cloudProviderList) {
		t.Fatalf("got %d choices for %d providers", len(items), len(cloudProviderList))
	}
	for i, name := range cloudProviderList {
		title := providerPlaces[name].title
		if title == "" {
			t.Errorf("provider %q has no full name", name)
		}
		if !strings.HasPrefix(items[i], name+" ") || !strings.Contains(items[i], title) {
			t.Errorf("choice %d = %q, want %q and its full name", i, items[i], name)
		}
	}
	// Each line says where the machines run, so the pick tells a
	// container on this laptop from a cloud bill or a hosted service.
	want := map[string]string{
		"ac":     "Apple Container",
		"fc":     "local",
		"aws":    "cloud, BYO account",
		"boxes":  "cloud, SaaS",
		"static": "remote, BYO host",
	}
	for i, name := range cloudProviderList {
		if w, ok := want[name]; ok && !strings.Contains(items[i], w) {
			t.Errorf("%s choice = %q, want it to say %q", name, items[i], w)
		}
	}
}

// An error opens a popup with the whole message, wrapped -- the crumbs
// line cut the end off one like a provider's setup hint, where the part
// you need is -- and stays until OK. A listing that fails isn't one: it
// fails again on every refresh, so it's only flashed, and the table says
// why it's empty.
func TestTUIErrorPopup(t *testing.T) {
	tuiFlashFor = time.Millisecond
	m := listed(t, tuiFixture(), 100, 30)
	long := `switching to gcp: gcp.project is required: set --project, edit .onctl/onctl.yaml, or run "gcloud config set project <id>"`
	m.fail("%s", long)
	view := regexp.MustCompile(`\x1b\[[0-9;]*m`).ReplaceAllString(m.View(), "")
	var text []string
	for _, l := range strings.Split(view, "\n") {
		if i := strings.Index(l, "│  "); i >= 0 && strings.Count(l, "│") >= 3 {
			text = append(text, strings.TrimSpace(strings.Trim(l[i:], "│ ")))
		}
	}
	joined := strings.Join(text, " ")
	if !strings.Contains(joined, "Error") || !strings.Contains(joined, `"gcloud config set project <id>"`) {
		t.Errorf("the popup doesn't hold the whole error:\n%s", view)
	}
	for _, l := range m.dialog.body {
		if lipgloss.Width(l) > m.alertWidth() {
			t.Errorf("line wider than the popup: %q", l)
		}
	}
	m = ack(t, m)

	// Why a :provider switch failed comes in when the tui starts again
	// (the screenshot that asked for this): a popup too.
	next, _ := m.Update(tuiFlashMsg{text: long, isErr: true})
	m = next.(tuiModel)
	m = ack(t, m)
	next, _ = m.Update(tuiFlashMsg{text: "switched to ac"})
	m = next.(tuiModel)
	if m.dialog != nil || m.flash != "switched to ac" {
		t.Errorf("a switch that worked: dialog=%v flash=%q", m.dialog, m.flash)
	}

	next, _ = m.Update(tuiListMsg{resource: "vms", err: errors.New("token expired")})
	m = next.(tuiModel)
	if m.dialog != nil || !m.flashErr || !strings.Contains(m.flash, "token expired") {
		t.Errorf("a failed listing: dialog=%v flash=%q", m.dialog, m.flash)
	}
}
