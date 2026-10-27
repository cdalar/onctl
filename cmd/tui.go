package cmd

import (
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/cdalar/onctl/internal/tools"
	"github.com/cdalar/onctl/pkg/cloud"
	bspinner "github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/manifoldco/promptui"
	"github.com/spf13/cobra"
	"golang.org/x/term"
)

// `onctl tui`: your VMs the way k9s shows a cluster (Bubble Tea), after
// boxctl's tui. A header with an info block, the current view's hotkeys
// and a logo; a bordered table per resource view (tuiviews.go), switched
// with a `:` command and filtered with `/`; describe/json pages you drill
// into with esc to come back; breadcrumbs and flash messages at the
// bottom. It is a front end to the provider calls the subcommands make
// -- List/ListPaused, Pause, Resume, Destroy, ListImages, ListSizes --
// and runs `onctl ssh`, `onctl create` and `onctl claude` as child
// processes for a terminal, a new VM and Claude Code on a VM (or, from
// `:repos`, on a box with a GitHub repository cloned -- tuirepos.go), so
// nothing here changes how those commands behave. `:provider` switches clouds
// like k9s's `:ctx`: the tui restarts on the other provider.

var tuiCmd = &cobra.Command{
	Use:   "tui",
	Short: "Browse and manage your VMs in a k9s-style terminal UI",
	Long: `Opens a live, full-screen view of your VMs, laid out like k9s.

  :vms :agents :images :sizes :repos   switch view (also :q, :help, :create)
  :provider [name]      switch cloud provider (a list to pick from without a name)
  :claude [vm]          run Claude Code on a VM (onctl claude --box vm; the project's box without one)
  /text        filter the rows; esc clears
  j/k, arrows  move; g/G top/bottom; shift+letter sorts by that column
  enter, d     describe the VM; y its JSON
  s            open a terminal on the VM (onctl ssh)
  a            run Claude Code on the VM (onctl claude --box vm)
  enter        in :repos (your GitHub repos, newest push first, from gh), start Claude
               on a box of this provider with the repo cloned (onctl claude --repo):
               a new box, or the repo's own box, is asked
  :agents      Claude sessions on boxes, with herdr's status and their PRs; n starts
               one (pick a repo, say the task), enter attaches, P pushes and opens
               a PR, o opens it. You get a desktop notification when Claude waits
  c            create a VM (name, image and type are asked; runs onctl create)
  p / r        pause / resume
  ctrl-d       destroy the VM (asks first)
  0 / 1 / 2    all VMs / running only / paused only
  esc          back; ctrl-c or :q quits`,
	Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		if !term.IsTerminal(int(os.Stdout.Fd())) || !term.IsTerminal(int(os.Stdin.Fd())) {
			return fmt.Errorf("onctl tui needs a terminal; use `onctl ls` on a pipe")
		}
		var flash string
		var flashErr bool
		for {
			m := newTUI(provider)
			m.startFlash, m.startFlashErr = flash, flashErr
			restore := quietLogs()
			final, err := tea.NewProgram(m, tea.WithAltScreen()).Run()
			restore()
			if err != nil {
				return err
			}
			name := final.(tuiModel).switchTo
			if name == "" {
				return nil
			}
			// `:provider name` quit the tui to switch: it starts again
			// on the other provider, or on this one with the reason it
			// couldn't.
			flash, flashErr = "switched to "+name, false
			if err := switchProvider(name); err != nil {
				flash, flashErr = err.Error(), true
			}
		}
	},
}

// switchProvider makes name the provider the tui and its children use,
// as `onctl -p name tui` would have: ONCTL_CLOUD, the config and the
// client are redone. A config that can't be loaded leaves the current
// provider in place. It runs between two runs of the tui, with the
// screen and the logs back to normal, because a provider's client
// exits the process when its credentials are missing (hetzner without
// HCLOUD_TOKEN, boxes without onctl login) -- as `onctl -p name tui`
// does, with the same message.
func switchProvider(name string) error {
	if !tools.Contains(cloudProviderList, name) {
		return fmt.Errorf("unsupported provider %q; use one of: %s", name, strings.Join(cloudProviderList, ", "))
	}
	prev := cloudProvider
	prevEnv, hadEnv := os.LookupEnv("ONCTL_CLOUD")
	if err := os.Setenv("ONCTL_CLOUD", name); err != nil {
		return err
	}
	if err := initState(); err != nil {
		cloudProvider = prev
		if hadEnv {
			_ = os.Setenv("ONCTL_CLOUD", prevEnv)
		} else {
			_ = os.Unsetenv("ONCTL_CLOUD")
		}
		return fmt.Errorf("switching to %s: %w", name, err)
	}
	initProvider(name)
	return nil
}

// askProvider is `onctl tui` with neither -p nor ONCTL_CLOUD: the
// provider is picked from a list, each with its full name and where it
// runs ("ac  Apple Container  local"), instead of guessed from the
// credentials in the environment.
func askProvider() (string, error) {
	prompt := promptui.Select{
		Label: "Which provider do you want to work on",
		Items: providerChoices(),
		Size:  len(cloudProviderList),
	}
	i, _, err := prompt.Run()
	if err != nil {
		return "", err
	}
	return cloudProviderList[i], nil
}

// providerChoices are askProvider's lines, in cloudProviderList's order.
func providerChoices() []string {
	var items []string
	for _, name := range cloudProviderList {
		p := providerPlaces[name]
		items = append(items, fmt.Sprintf("%-8s %-25s %s", name, p.title, p.kind.offering()))
	}
	return items
}

func init() {
	rootCmd.AddCommand(tuiCmd)
}

// quietLogs keeps the provider code's log lines (main.go sends them to
// stderr) off the alternate screen while the tui runs: dropped, or, with
// ONCTL_LOG set, written to a file whose name is printed on exit. The
// returned func puts the previous writer back.
func quietLogs() func() {
	prev := log.Writer()
	if os.Getenv("ONCTL_LOG") == "" {
		log.SetOutput(io.Discard)
		return func() { log.SetOutput(prev) }
	}
	f, err := os.CreateTemp("", "onctl-tui-*.log")
	if err != nil {
		log.SetOutput(io.Discard)
		return func() { log.SetOutput(prev) }
	}
	log.SetOutput(f)
	return func() {
		log.SetOutput(prev)
		_ = f.Close()
		fmt.Fprintln(os.Stderr, "onctl tui log:", f.Name())
	}
}

// tuiRefreshEvery paces the list's own refreshes while nothing else is
// going on; an action's completion refreshes at once.
const tuiRefreshEvery = 15 * time.Second

// tuiFlashFor is how long a flash message stays on the crumbs line (a
// variable so tests, which run every command, needn't wait for it).
var tuiFlashFor = 5 * time.Second

// Messages from the commands the model runs.
type (
	tuiListMsg struct {
		resource string // vms, images, sizes, repos, agents
		vms      []cloud.Vm
		images   []cloud.CloudImage
		sizes    []cloud.CloudSize
		repos    []ghRepo
		agents   []agentSession
		err      error
	}
	tuiActionMsg struct {
		name, verb string
		err        error
		result     string // what to flash on success, "" for the default
	}
	tuiChildDoneMsg struct { // `onctl ssh`/`onctl create`/`onctl claude` exited
		verb   string // ssh, create, claude
		name   string
		err    error
		stderr string
	}
	tuiClaudeDoneMsg struct { // `onctl claude --repo` from :repos ended
		repo   string
		box    string // the box it ran on; "" for the repo's own
		err    error
		stderr string
	}
	tuiFlashGoneMsg int
	tuiFlashMsg     struct { // something to flash (the start flash)
		text  string
		isErr bool
	}
	tuiTickMsg time.Time
)

// pageKind is what a page of the navigation stack shows.
type pageKind int

const (
	pageTable pageKind = iota
	pageText
)

// page is one level of the navigation stack: a table view with its own
// cursor and sort, or a text page (describe, json, help).
type page struct {
	kind     pageKind
	view     *tableView // pageTable
	cursor   int
	selected string // the row key under the cursor, to find it again after a refresh
	sortCol  int    // -1: the view's own order
	sortDesc bool

	title  string   // pageText: the crumb
	vm     string   // pageText: the VM it describes, "" when not about one
	lines  []string // pageText
	scroll int
	// refresh regenerates lines from the model's data after a refresh
	// (describe/json pages); nil for fixed text.
	refresh func(m *tuiModel) []string
}

// tuiDialog is a modal: a y/N question (yes), a choice between a few
// keys (choices), or an error to acknowledge (alert). Any other key
// closes it.
type tuiDialog struct {
	title   string
	body    []string
	yes     func(m tuiModel) (tea.Model, tea.Cmd)
	choices []tuiChoice
	alert   bool
}

// tuiChoice is one key of a choice dialog and what it does.
type tuiChoice struct {
	key, label string
	do         func(m tuiModel) (tea.Model, tea.Cmd)
}

type tuiModel struct {
	p cloud.CloudProviderInterface

	vms     []cloud.Vm // every VM, running then paused, as sortVMs orders them
	images  []cloud.CloudImage
	sizes   []cloud.CloudSize
	repos   []ghRepo         // GitHub's, newest push first (tuirepos.go)
	reposAt time.Time        // when repos came from GitHub, or the cache did
	loaded  map[string]bool  // resources listed at least once
	loading map[string]bool  // listings in flight
	listErr map[string]error // why a resource's last listing failed

	agents        []agentSession    // :agents (tuiagents.go)
	agentStatuses map[string]string // each box's status at the last poll; nil before the first
	tasking       bool              // typing a new session's task
	task          textinput.Model
	taskRepo      string

	pages       []page // the navigation stack; the last is shown
	stateFilter stateFilter
	filter      textinput.Model
	filtering   bool // typing in the filter prompt
	command     textinput.Model
	commanding  bool // typing in the : prompt
	dialog      *tuiDialog
	create      *tuiCreateForm
	pending     map[string]string // VM -> verb in flight
	spin        bspinner.Model
	spinning    bool

	flash    string
	flashErr bool
	flashID  int
	// startFlash is flashed when the tui starts: how a provider switch
	// went, since it runs between two runs of the tui.
	startFlash    string
	startFlashErr bool
	// switchTo is the provider `:provider` asked for; set, the tui
	// quits and tuiCmd starts it again on that provider.
	switchTo string

	width, height int
	refreshedAt   time.Time
}

func newTUI(p cloud.CloudProviderInterface) tuiModel {
	f := textinput.New()
	f.Prompt = ""
	f.Placeholder = "filter"
	f.CharLimit = 80
	cmd := textinput.New()
	cmd.Prompt = ""
	cmd.Placeholder = "vms, agents, images, sizes, repos, create, claude, provider, help, q"
	cmd.CharLimit = 80
	task := textinput.New()
	task.Prompt = ""
	task.CharLimit = 2000
	sp := bspinner.New(bspinner.WithSpinner(bspinner.Spinner{Frames: tuiSpinnerFrames, FPS: 80 * time.Millisecond}))
	sp.Style = k9s.logo
	return tuiModel{
		p:           p,
		loaded:      map[string]bool{},
		loading:     map[string]bool{},
		listErr:     map[string]error{},
		pages:       []page{{kind: pageTable, view: vmsView, sortCol: -1}},
		stateFilter: showAllVMs,
		filter:      f,
		command:     cmd,
		task:        task,
		pending:     map[string]string{},
		spin:        sp,
	}
}

var tuiSpinnerFrames = []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}

func (m tuiModel) Init() tea.Cmd {
	cmds := []tea.Cmd{m.list("vms"), m.tick(), m.spin.Tick}
	if m.startFlash != "" {
		flash := tuiFlashMsg{text: m.startFlash, isErr: m.startFlashErr}
		cmds = append(cmds, func() tea.Msg { return flash })
	}
	return tea.Batch(cmds...)
}

// top is the page shown.
func (m *tuiModel) top() *page { return &m.pages[len(m.pages)-1] }

// push shows p on top of the current page.
func (m *tuiModel) push(p page) { m.pages = append(m.pages, p) }

// pop goes back one page; the root stays.
func (m *tuiModel) pop() {
	if len(m.pages) > 1 {
		m.pages = m.pages[:len(m.pages)-1]
	}
}

// list fetches one resource; "" (a view made of what's known) is
// nothing to fetch.
func (m *tuiModel) list(resource string) tea.Cmd {
	if resource == "repos" {
		return m.listRepos(false)
	}
	if resource == "" || m.loading[resource] {
		return nil
	}
	m.loading[resource] = true
	p := m.p
	return func() tea.Msg {
		msg := tuiListMsg{resource: resource}
		switch resource {
		case "vms":
			running, paused, err := listVMs(p)
			msg.vms = append(append([]cloud.Vm{}, running.List...), paused.List...)
			msg.err = err
		case "images":
			msg.images, msg.err = tuiListImages(p)
		case "sizes":
			msg.sizes, msg.err = tuiListSizes(p)
		case "agents":
			msg.agents, msg.err = pollAgents()
		}
		return msg
	}
}

// tuiListImages is `onctl images`: the provider's images, when it lists
// them.
func tuiListImages(p cloud.CloudProviderInterface) ([]cloud.CloudImage, error) {
	lister, ok := p.(cloud.ImageLister)
	if !ok {
		return nil, fmt.Errorf("the %s provider does not list images", cloudProvider)
	}
	return lister.ListImages()
}

// tuiListSizes is `onctl sizes`: the provider's sizes, when it lists them.
func tuiListSizes(p cloud.CloudProviderInterface) ([]cloud.CloudSize, error) {
	lister, ok := p.(cloud.SizeLister)
	if !ok {
		first, _, _ := strings.Cut(sizesUnsupported(cloudProvider), "\n")
		return nil, errors.New(first)
	}
	return lister.ListSizes()
}

// refresh lists the VMs (the header and most pages need them). The
// catalogs (images, sizes) are listed when their view opens and on R,
// not on every tick: they hardly change, and some providers list them
// through several API calls.
func (m *tuiModel) refresh() tea.Cmd {
	if m.loaded["agents"] {
		// Once :agents has been opened its boxes are polled in every
		// view, so a Claude waiting for you is noticed wherever you are.
		return tea.Batch(m.list("vms"), m.list("agents"), m.spinIfBusy())
	}
	return tea.Batch(m.list("vms"), m.spinIfBusy())
}

// refreshAll is R: the VMs and the top page's catalog too.
func (m *tuiModel) refreshAll() tea.Cmd {
	cmds := []tea.Cmd{m.list("vms")}
	if t := m.top(); t.kind == pageTable && t.view.resource != "vms" {
		cmds = append(cmds, m.list(t.view.resource))
	}
	cmds = append(cmds, m.spinIfBusy())
	return tea.Batch(cmds...)
}

func (m tuiModel) tick() tea.Cmd {
	return tea.Tick(tuiRefreshEvery, func(t time.Time) tea.Msg { return tuiTickMsg(t) })
}

// busy reports whether something is in flight, i.e. the spinner should
// turn.
func (m tuiModel) busy() bool {
	return len(m.loading) > 0 || len(m.pending) > 0
}

// spinIfBusy starts the spinner when work begins and it isn't turning.
func (m *tuiModel) spinIfBusy() tea.Cmd {
	if m.busy() && !m.spinning {
		m.spinning = true
		return m.spin.Tick
	}
	return nil
}

// say flashes a message on the crumbs line for a while.
func (m *tuiModel) say(format string, a ...any) tea.Cmd {
	return m.setFlash(fmt.Sprintf(format, a...), false)
}

// fail flashes an error.
// fail shows an error in a popup until it's acknowledged -- the crumbs
// line cuts a long one off, and a flash goes before it can be read --
// and flashes it too.
func (m *tuiModel) fail(format string, a ...any) tea.Cmd {
	s := fmt.Sprintf(format, a...)
	m.dialog = &tuiDialog{title: "Error", body: wrapLines(s, m.alertWidth()), alert: true}
	return m.setFlash(s, true)
}

// alertWidth is how wide an error popup's text is: at most 76 columns,
// less on a narrow screen.
func (m *tuiModel) alertWidth() int {
	if m.width == 0 {
		return 76
	}
	return max(min(m.width-12, 76), 20)
}

// wrapLines breaks s into lines of at most width columns, at spaces.
func wrapLines(s string, width int) []string {
	lines := strings.Split(lipgloss.NewStyle().Width(width).Render(s), "\n")
	for i, l := range lines {
		lines[i] = strings.TrimRight(l, " ")
	}
	return lines
}

func (m *tuiModel) setFlash(s string, isErr bool) tea.Cmd {
	m.flash, m.flashErr = s, isErr
	m.flashID++
	id := m.flashID
	return tea.Tick(tuiFlashFor, func(time.Time) tea.Msg { return tuiFlashGoneMsg(id) })
}

func (m tuiModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		return m, nil

	case tuiTickMsg:
		return m, tea.Batch(m.refresh(), m.tick())

	case bspinner.TickMsg:
		if !m.busy() {
			m.spinning = false
			return m, nil
		}
		var cmd tea.Cmd
		m.spin, cmd = m.spin.Update(msg)
		return m, cmd

	case tuiFlashGoneMsg:
		if int(msg) == m.flashID {
			m.flash = ""
		}
		return m, nil

	case tuiFlashMsg:
		if msg.isErr {
			// Why a provider switch failed: as any error, in a popup.
			return m, m.fail("%s", msg.text)
		}
		return m, m.setFlash(msg.text, false)

	case tuiListMsg:
		delete(m.loading, msg.resource)
		// Listed, even when it failed: the table then says why it is
		// empty (the flash goes away) instead of "Loading…" for ever.
		m.loaded[msg.resource] = true
		delete(m.listErr, msg.resource)
		var cmd tea.Cmd
		if msg.err != nil {
			// Only flashed: a listing fails again on every refresh, and the
			// table says why it's empty.
			cmd = m.setFlash(fmt.Sprintf("listing %s: %v", msg.resource, msg.err), true)
			m.listErr[msg.resource] = msg.err
			// One of List/ListPaused failing still leaves the other's
			// VMs worth showing; the catalogs are all or nothing.
			if msg.resource != "vms" || len(msg.vms) == 0 {
				return m, cmd
			}
		}
		switch msg.resource {
		case "vms":
			m.vms = msg.vms
			sortVMs(m.vms)
			m.refreshedAt = time.Now()
		case "images":
			m.images = msg.images
		case "sizes":
			m.sizes = msg.sizes
		case "repos":
			m.repos = msg.repos
			m.reposAt = time.Now()
		case "agents":
			m.gotAgents(msg.agents)
		}
		// A describe/json page follows its VM.
		for i := range m.pages {
			if p := &m.pages[i]; p.kind == pageText && p.refresh != nil {
				if lines := p.refresh(&m); lines != nil {
					p.lines = lines
				}
			}
		}
		return m, cmd

	case tuiActionMsg:
		delete(m.pending, msg.name)
		if msg.err != nil {
			return m, tea.Batch(m.fail("%s %s: %v", presentTense(msg.verb), msg.name, msg.err), m.refresh())
		}
		result := msg.result
		if result == "" {
			result = fmt.Sprintf("%s %s", pastTense(msg.verb), msg.name)
		}
		return m, tea.Batch(m.say("%s", result), m.refresh())

	case tuiChildDoneMsg:
		what := msg.name
		if what == "" {
			what = "a VM"
		}
		verb := msg.verb
		if verb == "claude" {
			verb = "claude on"
		}
		if msg.err != nil {
			// The command's own last word (the alternate screen hides
			// what it printed), not exec's "exit status 1".
			reason := lastLine(msg.stderr)
			reason = strings.TrimPrefix(reason, "Error: ")
			if reason == "" && (msg.verb == "ssh" || msg.verb == "claude") {
				// `onctl ssh` exits with the remote shell's status, and
				// `onctl claude` with the attach's; a non-zero one with
				// nothing said is the remote's, not a connection that
				// failed.
				return m, m.refresh()
			}
			if reason == "" {
				reason = msg.err.Error()
			}
			return m, tea.Batch(m.fail("%s %s: %s", verb, what, reason), m.refresh())
		}
		if msg.verb == "create" {
			return m, tea.Batch(m.say("created %s", what), m.refresh())
		}
		if msg.verb == "start" {
			return m, tea.Batch(m.say("Claude is working on %s; you're notified when it waits for you", what), m.refresh(), m.list("agents"))
		}
		return m, m.refresh()

	case tuiClaudeDoneMsg:
		if msg.err != nil {
			return m, tea.Batch(m.fail("claude on %s: %s", msg.repo, claudeFailure(msg.stderr, msg.err)), m.refresh())
		}
		if msg.box != "" {
			return m, tea.Batch(m.say("claude on %s ended (%s is kept)", msg.repo, msg.box), m.refresh())
		}
		return m, tea.Batch(m.say("claude on %s ended", msg.repo), m.refresh())

	case tuiCatalogMsg:
		if m.create != nil {
			m.create.setCatalog(msg)
		}
		return m, nil

	case tea.KeyMsg:
		return m.handleKey(msg)
	}

	// Anything else (cursor blinks and the like) belongs to whichever
	// input has focus.
	return m, m.updateInputs(msg)
}

// lastLine is the last non-empty line of s, trimmed.
func lastLine(s string) string {
	lines := strings.Split(strings.TrimSpace(s), "\n")
	for i := len(lines) - 1; i >= 0; i-- {
		if l := strings.TrimSpace(lines[i]); l != "" {
			return l
		}
	}
	return ""
}

// updateInputs passes msg to the focused text input, if any.
func (m *tuiModel) updateInputs(msg tea.Msg) tea.Cmd {
	var cmd tea.Cmd
	switch {
	case m.filtering:
		m.filter, cmd = m.filter.Update(msg)
	case m.commanding:
		m.command, cmd = m.command.Update(msg)
	case m.tasking:
		m.task, cmd = m.task.Update(msg)
	case m.create != nil:
		cmd = m.create.update(msg)
	}
	return cmd
}

// rows is the top table's rows as shown: the view's, filtered and
// sorted. The cursor is clamped and put back on the selected row here,
// since that is the one place the rows are known.
func (m *tuiModel) rows() []tableRow {
	t := m.top()
	if t.kind != pageTable {
		return nil
	}
	rows := filterRows(t.view.rows(m), m.filter.Value())
	sortRows(rows, t.sortCol, t.sortDesc)
	t.cursor = 0
	for i, r := range rows {
		if r.key == t.selected {
			t.cursor = i
			break
		}
	}
	if len(rows) > 0 {
		t.selected = rows[t.cursor].key
	}
	return rows
}

// currentRow is the row under the cursor, nil with none.
func (m *tuiModel) currentRow() *tableRow {
	rows := m.rows()
	t := m.top()
	if t.cursor < 0 || t.cursor >= len(rows) {
		return nil
	}
	return &rows[t.cursor]
}

// currentVM is the VM the top page is about: the row's in the vms view,
// or a text page's VM.
func (m *tuiModel) currentVM() *cloud.Vm {
	t := m.top()
	var name string
	switch {
	case t.kind == pageText:
		name = t.vm
	case t.view == vmsView || t.view == agentsView:
		if r := m.currentRow(); r != nil {
			name = r.key
		}
	}
	return m.vm(name)
}

// vm finds name among the VMs, nil when it isn't there.
func (m *tuiModel) vm(name string) *cloud.Vm {
	for i := range m.vms {
		if m.vms[i].Name == name {
			return &m.vms[i]
		}
	}
	return nil
}

// move shifts the cursor by n rows, within the list.
func (m *tuiModel) move(n int) {
	rows := m.rows()
	t := m.top()
	if len(rows) == 0 {
		return
	}
	t.cursor = min(max(t.cursor+n, 0), len(rows)-1)
	t.selected = rows[t.cursor].key
}

// moveTo puts the cursor on the first or the last row.
func (m *tuiModel) moveTo(last bool) {
	rows := m.rows()
	t := m.top()
	if len(rows) == 0 {
		return
	}
	t.cursor = 0
	if last {
		t.cursor = len(rows) - 1
	}
	t.selected = rows[t.cursor].key
}

// openView makes v the only page (k9s's `:resource`), listing what it
// needs.
func (m *tuiModel) openView(v *tableView) tea.Cmd {
	m.pages = []page{{kind: pageTable, view: v, sortCol: -1}}
	m.filter.SetValue("")
	return tea.Batch(m.list(v.resource), m.spinIfBusy())
}

// vmStateRank groups statuses the way `onctl ls` does: 0 live, 1 paused
// (ls's PAUSED section: "paused", or a stopped status from a provider
// whose List includes stopped VMs), 2 gone for good (fc/ch "dead").
func vmStateRank(status string) int {
	switch {
	case status == "paused" || isPausedStatus(status):
		return 1
	case status == "dead" || status == "failed" || status == "error":
		return 2
	}
	return 0
}

// sortVMs orders vms live, paused, other; newest first within a group;
// name as the tiebreak. It is the vms view's order before any
// shift+letter sort.
func sortVMs(vms []cloud.Vm) {
	sort.SliceStable(vms, func(i, j int) bool {
		ri, rj := vmStateRank(vms[i].Status), vmStateRank(vms[j].Status)
		if ri != rj {
			return ri < rj
		}
		if !vms[i].CreatedAt.Equal(vms[j].CreatedAt) {
			return vms[i].CreatedAt.After(vms[j].CreatedAt)
		}
		return vms[i].Name < vms[j].Name
	})
}

// presentTense/pastTense word the flash: "pausing my-vm", "paused
// my-vm".
func presentTense(verb string) string {
	switch verb {
	case "pause":
		return "pausing"
	case "resume":
		return "resuming"
	case "destroy":
		return "destroying"
	case "create":
		return "creating"
	}
	return verb + "ing"
}

func pastTense(verb string) string {
	switch verb {
	case "pause", "resume", "create":
		return verb + "d"
	case "destroy":
		return "destroyed"
	}
	return verb + "ed"
}
