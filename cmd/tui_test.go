package cmd

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/cdalar/onctl/pkg/cloud"
	tea "github.com/charmbracelet/bubbletea"
)

// tuiFakeProvider is a CloudProviderInterface (with sizes and images)
// that records the calls the tui makes, so the tests check the tui
// drives the provider exactly as the subcommands would.
type tuiFakeProvider struct {
	vms    []cloud.Vm // what List returns (a stopped one among them)
	paused []cloud.Vm // what ListPaused returns
	calls  []string
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
func (f *tuiFakeProvider) List() (cloud.VmList, error) { return cloud.VmList{List: f.vms}, nil }
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
	// So is ssh.
	m, cmd = press(t, m, "s")
	if cmd == nil || !strings.Contains(m.flash, "resume it first") {
		t.Errorf("s on a paused VM: cmd=%v flash=%q", cmd, m.flash)
	}
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
	if m.dialog != nil || !strings.Contains(m.flash, "being resumed") {
		t.Errorf("ctrl+d on a pending VM: dialog=%v flash=%q", m.dialog, m.flash)
	}
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
	next, _ := m.Update(tuiListMsg{resource: "images", err: errors.New("no api token")})
	m = next.(tuiModel)
	if !m.flashErr || m.flash != "listing images: no api token" || m.loaded["images"] {
		t.Errorf("flash=%q loaded=%v", m.flash, m.loaded)
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
