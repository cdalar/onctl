package cmd

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"sort"
	"strings"

	"github.com/cdalar/onctl/internal/tools"
	"github.com/cdalar/onctl/pkg/cloud"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/spf13/pflag"
)

// The tui's keys and the actions behind them: the `:` command prompt,
// the `/` filter, the y/N dialog, the table and text-page keys, and the
// provider call (or `onctl` child process) each action makes.

func (m tuiModel) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if msg.Type == tea.KeyCtrlC {
		return m, tea.Quit
	}
	switch {
	case m.filtering:
		return m.handleFilterKey(msg)
	case m.commanding:
		return m.handleCommandKey(msg)
	case m.dialog != nil:
		return m.handleDialogKey(msg)
	case m.create != nil:
		return m.handleCreateKey(msg)
	}
	// Keys typed quickly (or held) can arrive as one message with
	// several runes; each is a keypress of its own here.
	if msg.Type == tea.KeyRunes && len(msg.Runes) > 1 && !msg.Paste {
		var cmds []tea.Cmd
		var next tea.Model = m
		for _, r := range msg.Runes {
			var cmd tea.Cmd
			next, cmd = next.(tuiModel).handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
			cmds = append(cmds, cmd)
		}
		return next, tea.Batch(cmds...)
	}
	if m.top().kind == pageText {
		return m.handleTextKey(msg)
	}
	return m.handleTableKey(msg)
}

// --- the : prompt ---------------------------------------------------------

func (m tuiModel) handleCommandKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.Type {
	case tea.KeyEsc:
		m.commanding = false
		m.command.Blur()
		return m, nil
	case tea.KeyEnter:
		m.commanding = false
		m.command.Blur()
		line := m.command.Value()
		m.command.SetValue("")
		return m.runCommand(line)
	case tea.KeyTab:
		return m.completeCommand()
	}
	var cmd tea.Cmd
	m.command, cmd = m.command.Update(msg)
	return m, cmd
}

// tuiCommands is what tab completes at the `:` prompt: the views' names
// and the other commands. Aliases (`:vm`, `:ctx`) are typed out.
func tuiCommands() []string {
	names := []string{"create", "ssh", "claude", "help", "q"}
	for _, v := range tuiViews {
		names = append(names, v.name)
	}
	sort.Strings(names)
	return names
}

// completeCommand is tab at the `:` prompt: the first word grows to the
// one command it starts (`:pro` -> `:providers `), or to what the several
// it starts share, flashing them to pick from (`:s` -> sizes, ssh). Once
// the command has an argument, tab does nothing.
func (m tuiModel) completeCommand() (tea.Model, tea.Cmd) {
	line := m.command.Value()
	if strings.ContainsAny(line, " \t") {
		return m, nil
	}
	word := strings.ToLower(line)
	var matches []string
	for _, c := range tuiCommands() {
		if strings.HasPrefix(c, word) {
			matches = append(matches, c)
		}
	}
	switch len(matches) {
	case 0:
		return m, nil
	case 1:
		m.command.SetValue(matches[0] + " ")
		m.command.CursorEnd()
		return m, nil
	}
	prefix := matches[0]
	for _, c := range matches[1:] {
		for !strings.HasPrefix(c, prefix) {
			prefix = prefix[:len(prefix)-1]
		}
	}
	m.command.SetValue(prefix)
	m.command.CursorEnd()
	return m, m.say("%s", strings.Join(matches, "  "))
}

// runCommand is a `:` command: a view's name, q, help, create, ssh,
// claude or provider.
func (m tuiModel) runCommand(line string) (tea.Model, tea.Cmd) {
	words := strings.Fields(strings.TrimPrefix(strings.TrimSpace(line), ":"))
	if len(words) == 0 {
		return m, nil
	}
	switch words[0] {
	case "q", "quit", "exit":
		return m, tea.Quit
	case "help", "h", "?":
		m.pushHelp()
		return m, nil
	case "create", "new":
		return m.openCreate("", "")
	case "ssh", "shell":
		if len(words) < 2 {
			return m, m.fail("usage: :ssh <vm>")
		}
		return m.ssh(words[1])
	case "claude":
		if len(words) > 1 {
			return m.claude(words[1])
		}
		return m.claude("")
	case "provider", "providers", "ctx", "cloud":
		if len(words) > 1 {
			return m.switchProvider(words[1])
		}
		return m, m.openView(providersView)
	}
	if v := findView(words[0]); v != nil {
		cmd := m.openView(v)
		if len(words) > 1 {
			// `:vms web` opens the view filtered, like k9s's `:pods nginx`.
			m.filter.SetValue(strings.Join(words[1:], " "))
		}
		return m, cmd
	}
	return m, m.fail("unknown command %q (try :vms, :images, :sizes, :repos, :provider, :claude, :help, :q)", words[0])
}

// --- the / filter ---------------------------------------------------------

func (m tuiModel) handleFilterKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.Type {
	case tea.KeyEsc:
		m.filter.SetValue("")
		m.filter.Blur()
		m.filtering = false
		return m, nil
	case tea.KeyEnter:
		m.filter.Blur()
		m.filtering = false
		return m, nil
	}
	var cmd tea.Cmd
	m.filter, cmd = m.filter.Update(msg)
	return m, cmd
}

// --- dialogs --------------------------------------------------------------

func (m tuiModel) handleDialogKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	d := m.dialog
	m.dialog = nil
	// y alone says yes, as in destroy's own [y/N]: enter is no.
	if s := msg.String(); s == "y" || s == "Y" {
		return d.yes(m)
	}
	return m, nil
}

// ask opens a y/N question.
func (m *tuiModel) ask(title string, body []string, yes func(m tuiModel) (tea.Model, tea.Cmd)) {
	m.dialog = &tuiDialog{title: title, body: body, yes: yes}
}

// --- table keys -----------------------------------------------------------

func (m tuiModel) handleTableKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	t := m.top()
	key := msg.String()
	switch key {
	case "up", "k":
		m.move(-1)
		return m, nil
	case "down", "j":
		m.move(1)
		return m, nil
	case "pgup", "ctrl+b":
		m.move(-m.tableHeight())
		return m, nil
	case "pgdown", "ctrl+f":
		m.move(m.tableHeight())
		return m, nil
	case "home", "g":
		m.moveTo(false)
		return m, nil
	case "end", "G":
		m.moveTo(true)
		return m, nil
	case ":":
		m.commanding = true
		m.command.SetValue("")
		return m, m.command.Focus()
	case "/":
		m.filtering = true
		return m, m.filter.Focus()
	case "?":
		m.pushHelp()
		return m, nil
	case "esc":
		switch {
		case m.filter.Value() != "":
			m.filter.SetValue("")
		case len(m.pages) > 1:
			m.pop()
		}
		return m, nil
	case "q":
		if len(m.pages) > 1 {
			m.pop()
			return m, nil
		}
		return m, tea.Quit
	case "R", "ctrl+r", "f5":
		if t.view == reposView {
			return m, tea.Batch(m.list("vms"), m.listRepos(true), m.spinIfBusy())
		}
		return m, m.refreshAll()
	case "0", "1", "2":
		if t.view == vmsView {
			m.stateFilter = stateFilter(key[0] - '0')
		}
		return m, nil
	case "c":
		switch t.view {
		case imagesView:
			if r := m.currentRow(); r != nil {
				return m.openCreate(r.key, "")
			}
		case sizesView:
			if r := m.currentRow(); r != nil {
				return m.openCreate("", r.key)
			}
		}
		return m.openCreate("", "")
	case "enter":
		if r := m.currentRow(); r != nil {
			switch t.view {
			case providersView:
				return m.switchProvider(r.key)
			case reposView:
				return m.claudeRepo(r.key)
			}
		}
		if t.view == providersView || t.view == reposView {
			return m, nil
		}
		return m.describe()
	case "d":
		return m.describe()
	case "y":
		return m.showJSON()
	case "ctrl+d":
		return m.askDestroy()
	}
	// shift+letter sorts by the column with that letter.
	if len(key) == 1 && key[0] >= 'A' && key[0] <= 'Z' {
		for i, col := range t.view.columns {
			if col.sort == rune(key[0]) {
				if t.sortCol == i {
					t.sortDesc = !t.sortDesc
				} else {
					t.sortCol, t.sortDesc = i, false
				}
				return m, nil
			}
		}
	}
	return m.handleVMKey(msg)
}

// handleVMKey is the keys that act on the page's VM: the row's in the
// vms view, or the one a describe page is about.
func (m tuiModel) handleVMKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	vm := m.currentVM()
	if vm == nil {
		return m, nil
	}
	p := m.p
	switch msg.String() {
	case "s":
		return m.ssh(vm.Name)
	case "a":
		return m.claude(vm.Name)
	case "p":
		// pause.go: the name is all the providers need.
		name := vm.Name
		return m.act(vm, "pause", 0, func() (string, error) {
			return "", p.Pause(cloud.Vm{Name: name}, false)
		})
	case "r":
		// resume.go: make sure the onctl SSH key exists and attach it.
		name := vm.Name
		return m.act(vm, "resume", 1, func() (string, error) {
			publicKeyFile, _ := getSSHKeyFilePaths("")
			keyID, err := p.CreateSSHKey(publicKeyFile)
			if err != nil {
				return "", err
			}
			resumed, err := p.Resume(cloud.Vm{Name: name, SSHKeyID: keyID})
			if err != nil {
				return "", err
			}
			return fmt.Sprintf("resumed %s (IP %s)", name, orDash(resumed.IP)), nil
		})
	}
	return m, nil
}

// --- text pages -----------------------------------------------------------

func (m tuiModel) handleTextKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	t := m.top()
	h := m.tableHeight()
	switch msg.String() {
	case "up", "k":
		t.scroll = max(t.scroll-1, 0)
	case "down", "j":
		t.scroll = min(t.scroll+1, max(len(t.lines)-h, 0))
	case "pgup", "ctrl+b", "b":
		t.scroll = max(t.scroll-h, 0)
	case "pgdown", "ctrl+f", "f", " ":
		t.scroll = min(t.scroll+h, max(len(t.lines)-h, 0))
	case "home", "g":
		t.scroll = 0
	case "end", "G":
		t.scroll = max(len(t.lines)-h, 0)
	case "esc", "q":
		m.pop()
	case ":":
		m.commanding = true
		m.command.SetValue("")
		return m, m.command.Focus()
	case "?":
		m.pushHelp()
	case "y":
		return m.showJSON()
	case "d", "enter":
		return m.describe()
	case "ctrl+d":
		return m.askDestroy()
	default:
		return m.handleVMKey(msg)
	}
	return m, nil
}

// pushHelp shows the keys: the view's hotkeys, the common ones, and the
// sort letters.
func (m *tuiModel) pushHelp() {
	v := vmsView
	for i := len(m.pages) - 1; i >= 0; i-- {
		if m.pages[i].kind == pageTable {
			v = m.pages[i].view
			break
		}
	}
	lines := []string{"General", ""}
	general := [][2]string{
		{":<view>", "switch view: vms, images, sizes, repos (`:vms web` filters too)"},
		{":create", "create a VM"}, {":ssh <vm>", "open a terminal on a VM"},
		{":claude [vm]", "run Claude Code on a VM (the project's box without one)"},
		{":provider [name]", "switch cloud provider (a list to pick from without a name)"},
		{":help", "this page"}, {":q", "quit (ctrl-c too)"},
		{"tab", "complete the command (`:pro` -> `:providers`)"},
		{"/text", "filter the rows; esc clears"},
		{"j/k ↑/↓", "move; ctrl-f/ctrl-b page; g/G top/bottom"},
		{"shift+letter", "sort by that column (again: the other way)"},
		{"esc", "back to the previous page"}, {"R", "refresh now (the VMs also every " + tuiRefreshEvery.String() + ")"},
	}
	for _, g := range general {
		lines = append(lines, fmt.Sprintf("  %-18s %s", g[0], g[1]))
	}
	lines = append(lines, "", v.title, "")
	for _, h := range v.hotkeys {
		lines = append(lines, fmt.Sprintf("  %-14s %s", h.key, h.label))
	}
	var sorts []string
	for _, col := range v.columns {
		if col.sort != 0 {
			sorts = append(sorts, fmt.Sprintf("shift-%c %s", col.sort+'a'-'A', col.title))
		}
	}
	if len(sorts) > 0 {
		lines = append(lines, "", "  sort:          "+strings.Join(sorts, ", "))
	}
	if v == reposView {
		lines = append(lines, "", "GitHub Repos",
			"  The repositories gh's account can reach (gh api user/repos), newest push first;",
			"  enter runs `onctl claude --repo owner/name` on this provider: a box (claude-<name>) with the repo",
			"  cloned on a new onctl/... branch and Claude attached; the same --repo later comes back to it.",
			"  The list is cached (~/.onctl/repos.json) and asked of GitHub again when an hour old, or with R.")
	}
	if v == vmsView {
		lines = append(lines, "", "VMs",
			"  s shell runs `onctl ssh <vm>`; a runs `onctl claude --box <vm>` (Claude Code on the VM, with this directory's project);",
			"  c runs `onctl create` in the foreground with what the form asks;",
			"  p is `onctl pause` (a snapshot, then the server is deleted, on Hetzner); r is `onctl resume`;",
			"  ctrl-d is `onctl destroy`, and asks first.")
	}
	m.push(page{kind: pageText, title: "help", lines: lines})
}

// --- actions --------------------------------------------------------------

// describe pushes the page's VM (or the repos view's repository) as a
// describe page.
func (m tuiModel) describe() (tea.Model, tea.Cmd) {
	t := m.top()
	if t.kind == pageTable && t.view == reposView {
		if r := m.currentRow(); r != nil {
			if repo := m.repo(r.key); repo != nil {
				m.push(page{kind: pageText, title: repo.FullName, lines: describeRepo(*repo)})
			}
		}
		return m, nil
	}
	vm := m.currentVM()
	if vm == nil {
		return m, nil
	}
	if t.kind == pageText && t.vm == vm.Name && t.title == vm.Name {
		return m, nil // already there
	}
	name := vm.Name
	m.push(page{kind: pageText, title: name, vm: name, lines: describeVM(*vm),
		refresh: func(m *tuiModel) []string {
			if vm := m.vm(name); vm != nil {
				return describeVM(*vm)
			}
			return nil
		}})
	return m, nil
}

// showJSON pushes the page's VM as the provider returns it (or the
// repos view's repository as GitHub does).
func (m tuiModel) showJSON() (tea.Model, tea.Cmd) {
	if t := m.top(); t.kind == pageTable && t.view == reposView {
		if r := m.currentRow(); r != nil {
			if repo := m.repo(r.key); repo != nil {
				m.push(page{kind: pageText, title: repo.FullName + ".json", lines: jsonLines(*repo)})
			}
		}
		return m, nil
	}
	vm := m.currentVM()
	if vm == nil {
		return m, nil
	}
	name := vm.Name
	m.push(page{kind: pageText, title: name + ".json", vm: name, lines: jsonLines(*vm),
		refresh: func(m *tuiModel) []string {
			if vm := m.vm(name); vm != nil {
				return jsonLines(*vm)
			}
			return nil
		}})
	return m, nil
}

func jsonLines(v any) []string {
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return []string{err.Error()}
	}
	return strings.Split(string(data), "\n")
}

// act runs one of the provider's per-VM calls on vm in the background,
// with the VM marked pending meanwhile. wantRank, when not -1, is the
// state group (vmStateRank) the VM must be in -- pause needs a live VM,
// resume a paused one -- so a sure-to-fail call isn't made. fn's
// string, if any, is what to flash on success.
func (m tuiModel) act(vm *cloud.Vm, verb string, wantRank int, fn func() (string, error)) (tea.Model, tea.Cmd) {
	if v, ok := m.pending[vm.Name]; ok {
		return m, m.fail("%s is being %s", vm.Name, pastTense(v))
	}
	if wantRank != -1 && vmStateRank(vm.Status) != wantRank {
		want := "running"
		if wantRank == 1 {
			want = "paused"
		}
		return m, m.fail("%s is %s, not %s; can't %s it", vm.Name, vm.Status, want, verb)
	}
	name := vm.Name
	m.pending[name] = verb
	run := func() tea.Msg {
		result, err := fn()
		return tuiActionMsg{name: name, verb: verb, err: err, result: result}
	}
	return m, tea.Batch(run, m.say("%s %s…", presentTense(verb), name), m.spinIfBusy())
}

// askDestroy is destroy's confirmation, then the call it makes.
func (m tuiModel) askDestroy() (tea.Model, tea.Cmd) {
	vm := m.currentVM()
	if vm == nil {
		return m, nil
	}
	if v, ok := m.pending[vm.Name]; ok {
		return m, m.fail("%s is being %s", vm.Name, pastTense(v))
	}
	name := vm.Name
	m.ask(fmt.Sprintf("Destroy %s?", name),
		[]string{fmt.Sprintf("This will permanently destroy VM %q and all its data.", name), "This cannot be undone."},
		func(m tuiModel) (tea.Model, tea.Cmd) {
			vm := m.vm(name)
			if vm == nil {
				return m, nil
			}
			p := m.p
			return m.act(vm, "destroy", -1, func() (string, error) {
				return "", p.Destroy(cloud.Vm{Name: name})
			})
		})
	return m, nil
}

// ssh hands the terminal to `onctl ssh <name>` -- a separate process,
// so its terminal handling (raw mode, its own stdin reader) can't
// interfere with this program's.
func (m tuiModel) ssh(name string) (tea.Model, tea.Cmd) {
	if vm := m.vm(name); vm != nil && vmStateRank(vm.Status) == 1 {
		return m, m.fail("%s is %s; press r to resume it first", name, vm.Status)
	}
	return m.runChild("ssh", name, tuiChildArgs("ssh", name))
}

// claude hands the terminal to `onctl claude`: on the VM named name
// (--box), or, with none, on the project's own box, created if need
// be. As on the command line, the project is the directory the tui
// was started in. A paused VM is resumed by onctl claude itself.
func (m tuiModel) claude(name string) (tea.Model, tea.Cmd) {
	if name == "" {
		dir, err := projectDir()
		if err != nil {
			return m, m.fail("claude: %v", err)
		}
		name = claudeBoxName(dir)
	}
	return m.runChild("claude", name, claudeArgs(name))
}

// claudeArgs is the `onctl claude` command line for the box named name.
func claudeArgs(name string) []string {
	return tuiChildArgs("claude", "--box", name)
}

// switchProvider is `:provider name`: the tui quits, and tuiCmd starts
// it again on that provider (see switchProvider in tui.go for why it
// isn't done in place).
func (m tuiModel) switchProvider(name string) (tea.Model, tea.Cmd) {
	name = strings.ToLower(strings.TrimSpace(name))
	if !tools.Contains(cloudProviderList, name) {
		return m, m.fail("unknown provider %q (one of %s)", name, strings.Join(cloudProviderList, ", "))
	}
	if name == cloudProvider {
		return m, m.say("already on %s", name)
	}
	m.switchTo = name
	return m, tea.Quit
}

// runChild runs this binary again with args in the foreground (the
// alternate screen is left meanwhile, so the command's output shows as
// usual) and reports back when it exits. Its stderr is also kept, to
// show why it failed once the screen is back.
func (m tuiModel) runChild(verb, name string, args []string) (tea.Model, tea.Cmd) {
	exe, err := os.Executable()
	if err != nil {
		return m, m.fail("finding onctl: %v", err)
	}
	c := exec.Command(exe, args...)
	var stderr bytes.Buffer
	c.Stderr = io.MultiWriter(os.Stderr, &stderr)
	return m, tea.ExecProcess(c, func(err error) tea.Msg {
		return tuiChildDoneMsg{verb: verb, name: name, err: err, stderr: stderr.String()}
	})
}

// tuiChildArgs is the argument list for an `onctl` child: the
// subcommand and its args, then the provider this tui resolved and
// every root flag given to it (--config, --project, ...), so the child
// lands on the same cloud and account.
func tuiChildArgs(args ...string) []string {
	args = append(args, "--provider", cloudProvider)
	rootCmd.PersistentFlags().VisitAll(func(f *pflag.Flag) {
		if f.Changed && f.Name != "provider" {
			args = append(args, "--"+f.Name, f.Value.String())
		}
	})
	return args
}
