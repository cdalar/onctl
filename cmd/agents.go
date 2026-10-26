package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/spf13/cobra"
	"golang.org/x/term"
)

// onctl agents (proof of concept): Claude sessions as a list, the way
// herdr shows agents, except each one is a box of its own. n picks a
// GitHub repository and a task, and runs `onctl claude --repo` detached
// on a new box, in herdr (whose agent status says when Claude is
// waiting) with a GitHub token stored there (so Claude can open the PR
// it's asked to open when it's done). The list polls every box over
// ssh, and a desktop notification goes out when a Claude starts waiting
// for you. On a provider other than boxes only the VMs started here are
// polled: connecting to a VM gives root ssh on it (connectClaudeVM),
// which isn't for VMs onctl claude never touched.

const agentsPollEvery = 10 * time.Second

// agentsPRInstruction is added to every task: making the PR is the
// point of the session, and writing its title and body is Claude's to do.
const agentsPRInstruction = "\n\nWhen the task is done: commit your work on the current branch, push it to origin, and open a pull request with `gh pr create`, with a clear title and a body that says what changed and why. Then reply with the pull request's URL."

var agentsCmd = &cobra.Command{
	Use:   "agents",
	Short: "Claude sessions on your boxes: start one on a repo, get told when one waits for you (proof of concept)",
	Long: `Lists the boxes running Claude, with what Claude is doing (herdr's
status: working, blocked, idle, done) and its pull request, and sends a
desktop notification when one starts waiting for you.

  n        new session: pick a repo, say the task; a new box gets it, and
           Claude is asked to open a pull request when it's done
  enter    attach to the session (herdr: Ctrl-b q detaches)
  o        open the session's pull request in the browser
  P        push the branch and open a pull request now (gh pr create --fill)
  x        destroy the box (onctl destroy asks first)
  r        refresh; q quits

The GitHub token stored on a new box comes from ONCTL_GITHUB_TOKEN, or is
asked for: give it a fine-grained one for the repo, with Contents and
Pull requests read/write.`,
	Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		if !term.IsTerminal(int(os.Stdout.Fd())) || !term.IsTerminal(int(os.Stdin.Fd())) {
			return fmt.Errorf("onctl agents needs a terminal")
		}
		restore := quietLogs()
		defer restore()
		_, err := tea.NewProgram(newAgentsModel(), tea.WithAltScreen()).Run()
		return err
	},
}

func init() {
	rootCmd.AddCommand(agentsCmd)
}

// agentSession is a box with a Claude project on it, as the list shows it.
type agentSession struct {
	box, repo, dir, branch, mux, status, pr, err string
}

// waiting reports whether Claude needs you: herdr's blocked (asking for
// a permission, a question) or done (finished its turn).
func (s agentSession) waiting() bool { return s.status == "blocked" || s.status == "done" }

// newlyWaiting is the sessions that started waiting since the last poll,
// whose statuses were prev (nil on the first poll: nothing is new then,
// or every waiting session would notify each time the view opens).
func newlyWaiting(prev map[string]string, now []agentSession) []agentSession {
	if prev == nil {
		return nil
	}
	var out []agentSession
	for _, s := range now {
		if was, seen := prev[s.box]; s.waiting() && (!seen || was != s.status) {
			out = append(out, s)
		}
	}
	return out
}

// agentStatusScript prints what the list shows of the box's project as
// key=value lines, or nothing on a box without one.
func agentStatusScript() string {
	return `d=$(cat ` + projectMarker + ` 2>/dev/null); [ -n "$d" ] || exit 0
gh=/usr/bin/gh; [ -x "$gh" ] || gh=gh
echo "dir=$d"
echo "mux=$(cat ` + muxMarker + ` 2>/dev/null)"
echo "origin=$(git -C "$d" remote get-url origin 2>/dev/null)"
echo "branch=$(git -C "$d" branch --show-current 2>/dev/null)"
echo "pr=$(cd "$d" && timeout 5 "$gh" pr view --json url -q .url 2>/dev/null)"
echo "herdr=$(herdr workspace list 2>/dev/null | tr -d '\n')"`
}

// parseAgentStatus reads agentStatusScript's output for the box named
// box; ok is false when the box holds no project.
func parseAgentStatus(box, out string) (s agentSession, ok bool) {
	kv := map[string]string{}
	for _, line := range strings.Split(out, "\n") {
		if k, v, found := strings.Cut(line, "="); found {
			kv[k] = strings.TrimSpace(v)
		}
	}
	if kv["dir"] == "" {
		return s, false
	}
	s = agentSession{box: box, dir: kv["dir"], branch: kv["branch"], mux: kv["mux"], pr: kv["pr"], repo: originRepo(kv["origin"])}
	switch s.mux {
	case "herdr":
		s.status = herdrMux{}.parseList(kv["herdr"])[claudeSession]
		if s.status == "" {
			s.status = "stopped"
		}
	case "":
		s.status = "not started"
	default:
		s.status = s.mux // tmux tells nothing of what Claude is doing
	}
	return s, true
}

// originRepo is a GitHub remote URL as owner/name, or the URL itself.
func originRepo(url string) string {
	for _, prefix := range []string{"git@github.com:", "ssh://git@github.com/", "https://github.com/"} {
		if rest, ok := strings.CutPrefix(url, prefix); ok {
			return strings.TrimSuffix(rest, ".git")
		}
	}
	return url
}

// --- boxes started here, and connections -------------------------------------

// agentsRegistryPath, ~/.onctl/agents.json, lists the boxes started from
// this view (box name -> repo). A variable for tests.
var agentsRegistryPath = func() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".onctl", "agents.json"), nil
}

func readAgentsRegistry() map[string]string {
	reg := map[string]string{}
	if p, err := agentsRegistryPath(); err == nil {
		if data, err := os.ReadFile(p); err == nil {
			_ = json.Unmarshal(data, &reg)
		}
	}
	return reg
}

func addToAgentsRegistry(box, repo string) error {
	p, err := agentsRegistryPath()
	if err != nil {
		return err
	}
	reg := readAgentsRegistry()
	reg[box] = repo
	data, err := json.Marshal(reg)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(p), 0o700); err != nil {
		return err
	}
	return os.WriteFile(p, data, 0o600)
}

// agentConns keeps one ssh connection per box for the view's life: the
// provider and key setup that makes it is a few API calls, and the
// connection is a control master that makes each poll cheap. A box that
// couldn't be reached is remembered (nil) and not tried again.
var agentConns = struct {
	sync.Mutex
	m map[string]*boxSSH
}{m: map[string]*boxSSH{}}

func agentConn(ctx context.Context, name string) *boxSSH {
	agentConns.Lock()
	b, tried := agentConns.m[name]
	agentConns.Unlock()
	if tried {
		return b
	}
	var err error
	if onBoxes() {
		b, err = connectClaudeBox(ctx, name, false, false)
	} else {
		b, _, err = connectClaudeVM(ctx, name, false)
	}
	if err != nil {
		b = nil
	}
	agentConns.Lock()
	agentConns.m[name] = b
	agentConns.Unlock()
	return b
}

// pollAgents lists the sessions: the provider's running VMs (on another
// provider than boxes, those started here) that hold a project.
func pollAgents() agentsPolledMsg {
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	vms, err := listClaudeVMs(ctx)
	if err != nil {
		return agentsPolledMsg{err: err}
	}
	reg := readAgentsRegistry()
	var (
		mu  sync.Mutex
		wg  sync.WaitGroup
		out []agentSession
	)
	for _, vm := range vms {
		if _, ours := reg[vm.name]; vm.state != "running" || (!onBoxes() && !ours) {
			continue
		}
		wg.Add(1)
		go func(name string) {
			defer wg.Done()
			b := agentConn(ctx, name)
			if b == nil {
				return
			}
			raw, err := b.output(ctx, agentStatusScript())
			s, ok := parseAgentStatus(name, string(raw))
			if err != nil {
				s, ok = agentSession{box: name, repo: reg[name], err: lastLine(err.Error())}, true
			}
			if !ok {
				return
			}
			mu.Lock()
			out = append(out, s)
			mu.Unlock()
		}(vm.name)
	}
	wg.Wait()
	sort.Slice(out, func(i, j int) bool { return out[i].box < out[j].box })
	return agentsPolledMsg{sessions: out}
}

// notifyDesktop shows a desktop notification. A variable for tests.
var notifyDesktop = func(title, body string) {
	switch runtime.GOOS {
	case "darwin":
		_ = exec.Command("osascript", "-e", fmt.Sprintf("display notification %q with title %q sound name \"Glass\"", body, title)).Run()
	case "linux":
		_ = exec.Command("notify-send", title, body).Run()
	}
}

// --- the view -------------------------------------------------------------------

type agentsMode int

const (
	agentsList agentsMode = iota
	agentsPickRepo
	agentsAskTask
)

type (
	agentsTickMsg   struct{}
	agentsPolledMsg struct {
		sessions []agentSession
		err      error
	}
	agentsReposMsg struct {
		repos []ghRepo
		err   error
	}
	agentsFlashMsg struct {
		text  string
		isErr bool
	}
)

type agentsModel struct {
	mode     agentsMode
	sessions []agentSession
	cursor   int
	statuses map[string]string // each box's status at the last poll
	polling  bool
	polled   bool
	flash    string
	flashErr bool

	repos      []ghRepo
	repoCursor int
	filter     textinput.Model
	task       textinput.Model
	repo       string

	width, height int
}

func newAgentsModel() agentsModel {
	f := textinput.New()
	f.Prompt = "repo> "
	t := textinput.New()
	t.Prompt = "task> "
	t.CharLimit = 2000
	return agentsModel{filter: f, task: t}
}

func (m agentsModel) Init() tea.Cmd { return m.poll() }

func (m *agentsModel) poll() tea.Cmd {
	if m.polling {
		return nil
	}
	m.polling = true
	return func() tea.Msg { return pollAgents() }
}

func agentsTick() tea.Cmd {
	return tea.Tick(agentsPollEvery, func(time.Time) tea.Msg { return agentsTickMsg{} })
}

func (m agentsModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		return m, nil
	case agentsTickMsg:
		return m, m.poll()
	case agentsPolledMsg:
		m.polling, m.polled = false, true
		if msg.err != nil {
			m.flash, m.flashErr = "listing boxes: "+msg.err.Error(), true
			return m, agentsTick()
		}
		for _, s := range newlyWaiting(m.statuses, msg.sessions) {
			go notifyDesktop("Claude is waiting: "+s.box, orDash(s.repo)+" -- "+s.status)
		}
		m.statuses = map[string]string{}
		for _, s := range msg.sessions {
			m.statuses[s.box] = s.status
		}
		m.sessions = msg.sessions
		m.cursor = min(m.cursor, max(len(m.sessions)-1, 0))
		return m, agentsTick()
	case agentsReposMsg:
		if msg.err != nil {
			m.mode, m.flash, m.flashErr = agentsList, "listing repos: "+msg.err.Error(), true
			return m, nil
		}
		m.repos = msg.repos
		return m, nil
	case agentsFlashMsg:
		m.flash, m.flashErr = msg.text, msg.isErr
		return m, m.poll()
	case tea.KeyMsg:
		switch m.mode {
		case agentsPickRepo:
			return m.pickRepoKey(msg)
		case agentsAskTask:
			return m.askTaskKey(msg)
		}
		return m.listKey(msg)
	}
	return m, nil
}

func (m agentsModel) current() *agentSession {
	if m.cursor < len(m.sessions) {
		return &m.sessions[m.cursor]
	}
	return nil
}

func (m agentsModel) listKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	s := m.current()
	switch msg.String() {
	case "q", "ctrl+c":
		return m, tea.Quit
	case "j", "down":
		m.cursor = min(m.cursor+1, max(len(m.sessions)-1, 0))
	case "k", "up":
		m.cursor = max(m.cursor-1, 0)
	case "r":
		return m, m.poll()
	case "n":
		return m.openRepoPicker()
	case "enter":
		if s != nil {
			return m, attachAgent(*s)
		}
	case "o":
		if s != nil && s.pr != "" {
			openbrowser(s.pr)
		} else if s != nil {
			m.flash, m.flashErr = s.box+" has no pull request yet", true
		}
	case "P":
		if s != nil {
			m.flash, m.flashErr = "opening a pull request from "+s.box+"...", false
			return m, openPR(*s)
		}
	case "x":
		if s != nil {
			return m, runAgentsChild("destroying "+s.box, tuiChildArgs("destroy", s.box))
		}
	}
	return m, nil
}

func (m agentsModel) openRepoPicker() (tea.Model, tea.Cmd) {
	m.mode, m.repoCursor, m.flash = agentsPickRepo, 0, ""
	m.filter.SetValue("")
	m.filter.Focus()
	if c := readRepoCache(); c != nil {
		m.repos = c.Repos
		if time.Since(c.FetchedAt) < repoCacheMaxAge {
			return m, textinput.Blink
		}
	}
	return m, func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
		defer cancel()
		repos, err := listGitHubRepos(ctx)
		if err == nil {
			writeRepoCache(repos)
		}
		return agentsReposMsg{repos: repos, err: err}
	}
}

// matchingRepos is the repos whose name has the filter in it.
func (m agentsModel) matchingRepos() []ghRepo {
	f := strings.ToLower(strings.TrimSpace(m.filter.Value()))
	var out []ghRepo
	for _, r := range m.repos {
		if !r.Archived && strings.Contains(strings.ToLower(r.FullName), f) {
			out = append(out, r)
		}
	}
	return out
}

func (m agentsModel) pickRepoKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	repos := m.matchingRepos()
	switch msg.String() {
	case "esc", "ctrl+c":
		m.mode = agentsList
		return m, nil
	case "down", "ctrl+n":
		m.repoCursor = min(m.repoCursor+1, max(len(repos)-1, 0))
		return m, nil
	case "up", "ctrl+p":
		m.repoCursor = max(m.repoCursor-1, 0)
		return m, nil
	case "enter":
		if m.repoCursor >= len(repos) {
			return m, nil
		}
		m.repo, m.mode = repos[m.repoCursor].FullName, agentsAskTask
		m.task.SetValue("")
		m.task.Focus()
		return m, textinput.Blink
	}
	var cmd tea.Cmd
	m.filter, cmd = m.filter.Update(msg)
	m.repoCursor = 0
	return m, cmd
}

func (m agentsModel) askTaskKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc", "ctrl+c":
		m.mode = agentsPickRepo
		return m, nil
	case "enter":
		task := strings.TrimSpace(m.task.Value())
		if task == "" {
			return m, nil
		}
		m.mode = agentsList
		taken := map[string]bool{}
		for _, s := range m.sessions {
			taken[s.box] = true
		}
		n, err := generateName(repoBoxPrefix(m.repo), task, func(s string) bool { return taken[s] })
		if err != nil {
			m.flash, m.flashErr = err.Error(), true
			return m, nil
		}
		box := n.box()
		if err := addToAgentsRegistry(box, m.repo); err != nil {
			m.flash, m.flashErr = "remembering "+box+": "+err.Error(), true
			return m, nil
		}
		m.flash, m.flashErr = "starting Claude on "+box+"...", false
		return m, runAgentsChild("starting "+box, agentsStartArgs(m.repo, box, task))
	}
	var cmd tea.Cmd
	m.task, cmd = m.task.Update(msg)
	return m, cmd
}

// agentsStartArgs is the `onctl claude` command line that starts a
// session: repo cloned on the new box, Claude in herdr (for its status),
// the GitHub token stored there (a detached Claude has no forwarding to
// open its PR with), and the task as the first message.
func agentsStartArgs(repo, box, task string) []string {
	return tuiChildArgs("claude", "--repo", repo, "--box", box, "--mux", "herdr",
		"--github", githubStore, "--detach", "--prompt", task+agentsPRInstruction)
}

// runAgentsChild runs onctl again in the foreground (so its progress and
// any token prompt show), and flashes how it went.
func runAgentsChild(what string, args []string) tea.Cmd {
	exe, err := os.Executable()
	if err != nil {
		return func() tea.Msg { return agentsFlashMsg{"finding onctl: " + err.Error(), true} }
	}
	c := exec.Command(exe, args...)
	tail := &tailBuffer{max: 4096}
	c.Stderr = io.MultiWriter(os.Stderr, tail)
	return tea.ExecProcess(c, func(err error) tea.Msg {
		if err != nil {
			return agentsFlashMsg{what + ": " + claudeFailure(tail.String(), err), true}
		}
		return agentsFlashMsg{what + ": done", false}
	})
}

// attachAgent hands the terminal to the session: herdr focused on its
// workspace, or its tmux session.
func attachAgent(s agentSession) tea.Cmd {
	b := agentConn(context.Background(), s.box)
	if b == nil {
		return func() tea.Msg { return agentsFlashMsg{"can't reach " + s.box, true} }
	}
	script := "exec tmux -u attach -t =" + claudeSession
	if s.mux == "herdr" {
		script = `ws=$(herdr workspace list 2>/dev/null | grep -o ` + shellQuote(`"label":"`+claudeSession+`"[^}]*"workspace_id":"[^"]*"`) + ` | sed 's/.*"workspace_id":"//; s/"$//')
[ -z "$ws" ] || herdr workspace focus "$ws" >/dev/null
exec herdr`
	}
	c := exec.Command("ssh", b.attachArgs(script, nil)...)
	c.Env = append(os.Environ(), "TERM=xterm-256color")
	return tea.ExecProcess(c, func(error) tea.Msg { return agentsFlashMsg{"back from " + s.box, false} })
}

// openPRScript pushes the box's branch and opens a pull request from
// what's committed, printing its URL (the existing one's, if there is).
func openPRScript(dir string) string {
	return `cd ` + shellQuote(dir) + ` || exit 1
gh=/usr/bin/gh; [ -x "$gh" ] || gh=gh
git push -u origin HEAD >&2 || exit 1
url=$("$gh" pr view --json url -q .url 2>/dev/null) || { "$gh" pr create --fill >&2 && url=$("$gh" pr view --json url -q .url); } && echo "$url"`
}

func openPR(s agentSession) tea.Cmd {
	return func() tea.Msg {
		b := agentConn(context.Background(), s.box)
		if b == nil {
			return agentsFlashMsg{"can't reach " + s.box, true}
		}
		ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
		defer cancel()
		out, err := b.output(ctx, openPRScript(s.dir))
		if err != nil {
			return agentsFlashMsg{"pull request from " + s.box + ": " + lastLine(err.Error()), true}
		}
		return agentsFlashMsg{"pull request: " + strings.TrimSpace(string(out)), false}
	}
}

// --- rendering ----------------------------------------------------------------

var (
	agentsTitle   = lipgloss.NewStyle().Bold(true)
	agentsDim     = lipgloss.NewStyle().Faint(true)
	agentsSel     = lipgloss.NewStyle().Reverse(true)
	agentsErr     = lipgloss.NewStyle().Foreground(lipgloss.Color("9"))
	agentsWaiting = lipgloss.NewStyle().Foreground(lipgloss.Color("11")).Bold(true)
	agentsWorking = lipgloss.NewStyle().Foreground(lipgloss.Color("10"))
)

func (m agentsModel) View() string {
	var b strings.Builder
	b.WriteString(agentsTitle.Render("onctl agents") + agentsDim.Render("  on "+cloudProvider) + "\n\n")
	switch m.mode {
	case agentsPickRepo:
		b.WriteString("New session: which repo?\n" + m.filter.View() + "\n\n")
		repos := m.matchingRepos()
		if len(m.repos) == 0 {
			b.WriteString(agentsDim.Render("  listing your repos with gh...") + "\n")
		}
		for i, r := range repos[:min(len(repos), 15)] {
			line := "  " + r.FullName
			if i == m.repoCursor {
				line = agentsSel.Render(line)
			}
			b.WriteString(line + "\n")
		}
		b.WriteString("\n" + agentsDim.Render("type to filter, up/down, enter picks, esc back"))
		return b.String()
	case agentsAskTask:
		b.WriteString("New session on " + agentsTitle.Render(m.repo) + ": what should Claude do?\n" + m.task.View() + "\n\n")
		b.WriteString(agentsDim.Render("enter starts a new box; Claude is asked to open a pull request when it's done. esc back"))
		return b.String()
	}

	switch {
	case !m.polled:
		b.WriteString(agentsDim.Render("looking for sessions...") + "\n")
	case len(m.sessions) == 0:
		b.WriteString(agentsDim.Render("no sessions -- n starts one") + "\n")
	default:
		rows := [][]string{{"BOX", "REPO", "BRANCH", "CLAUDE", "PULL REQUEST"}}
		for _, s := range m.sessions {
			status := s.status
			if s.err != "" {
				status = "error: " + s.err
			}
			rows = append(rows, []string{s.box, orDash(s.repo), orDash(s.branch), status, orDash(s.pr)})
		}
		widths := make([]int, len(rows[0]))
		for _, r := range rows {
			for i, c := range r {
				widths[i] = max(widths[i], lipgloss.Width(c))
			}
		}
		for i, r := range rows {
			cells := make([]string, len(r))
			for j, c := range r {
				cells[j] = padRight(c, widths[j])
			}
			line := strings.Join(cells, "  ")
			switch {
			case i == 0:
				line = agentsDim.Render(line)
			case i-1 == m.cursor:
				line = agentsSel.Render(line)
			case m.sessions[i-1].err != "":
				line = agentsErr.Render(line)
			case m.sessions[i-1].waiting():
				line = agentsWaiting.Render(line)
			case m.sessions[i-1].status == "working":
				line = agentsWorking.Render(line)
			}
			b.WriteString(line + "\n")
		}
	}
	if m.flash != "" {
		style := agentsDim
		if m.flashErr {
			style = agentsErr
		}
		b.WriteString("\n" + style.Render(m.flash) + "\n")
	}
	b.WriteString("\n" + agentsDim.Render("n new  enter attach  o open PR  P push+PR  x destroy  r refresh  q quit"))
	return b.String()
}
