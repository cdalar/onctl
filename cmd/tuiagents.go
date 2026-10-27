package cmd

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

// `:agents`: the boxes running Claude (agents.go), with what Claude is
// doing -- herdr's status: working, blocked, idle, done -- and its pull
// request. n picks a repository in :repos, enter there asks for the task,
// and the session starts on a new box. The boxes are polled on the tui's
// tick once the view has been opened, in whichever view is showing, so a
// desktop notification still goes out when a Claude starts waiting.

var agentsView = &tableView{
	name:     "agents",
	aliases:  []string{"agent", "sessions", "session"},
	title:    "Agents",
	resource: "agents",
	columns:  []column{{"BOX", 'B'}, {"REPO", 'O'}, {"BRANCH", 0}, {"CLAUDE", 'C'}, {"PULL REQUEST", 0}},
	stateCol: 3,
	empty:    "No Claude sessions. Press n to start one.",
	hotkeys: []hotkey{
		{"enter", "attach"}, {"n", "new session"}, {"o", "open PR"}, {"P", "push+PR"}, {"ctrl-d", "destroy"},
	},
	rows: func(m *tuiModel) []tableRow {
		var rows []tableRow
		for _, s := range m.agents {
			status, state := s.status, s.status
			if s.err != "" {
				status, state = "error: "+s.err, "dead"
			}
			rows = append(rows, tableRow{
				key:   s.box,
				state: state,
				cells: []string{s.box, orDash(s.repo), orDash(s.branch), status, orDash(s.pr)},
			})
		}
		return rows
	},
}

// agent is the session on the box named box, nil when there's none.
func (m *tuiModel) agent(box string) *agentSession {
	for i := range m.agents {
		if m.agents[i].box == box {
			return &m.agents[i]
		}
	}
	return nil
}

// gotAgents takes a poll's sessions, notifying for each that started
// waiting since the last one.
func (m *tuiModel) gotAgents(sessions []agentSession) {
	for _, s := range newlyWaiting(m.agentStatuses, sessions) {
		go notifyDesktop("Claude is waiting: "+s.box, orDash(s.repo)+" -- "+s.status)
	}
	m.agentStatuses = map[string]string{}
	for _, s := range sessions {
		m.agentStatuses[s.box] = s.status
	}
	m.agents = sessions
}

// handleAgentsKey is the agents view's own keys; ok is false for the
// keys every table has.
func (m tuiModel) handleAgentsKey(key string) (next tea.Model, cmd tea.Cmd, ok bool) {
	r := m.currentRow()
	var s *agentSession
	if r != nil {
		s = m.agent(r.key)
	}
	switch key {
	case "n":
		m.push(page{kind: pageTable, view: reposView, sortCol: -1})
		return m, tea.Batch(m.list("repos"), m.spinIfBusy(), m.say("pick the repo; enter asks for the task")), true
	case "enter":
		if s == nil {
			return m, nil, true
		}
		next, cmd := m.attachAgent(*s)
		return next, cmd, true
	case "o":
		switch {
		case s == nil:
		case s.pr == "":
			return m, m.fail("%s has no pull request yet", s.box), true
		default:
			openbrowser(s.pr)
		}
		return m, nil, true
	case "P":
		if s == nil {
			return m, nil, true
		}
		return m, tea.Batch(m.openPR(*s), m.say("pushing %s and opening a pull request…", s.box)), true
	}
	return m, nil, false
}

// pickedForAgent reports whether :repos was opened by the agents view's
// n, so enter on a repository asks for a task instead of a box.
func (m *tuiModel) pickedForAgent() bool {
	return len(m.pages) > 1 && m.top().view == reposView && m.pages[len(m.pages)-2].view == agentsView
}

// askTask opens the task prompt for a new session on repo.
func (m tuiModel) askTask(repo string) (tea.Model, tea.Cmd) {
	m.tasking, m.taskRepo = true, repo
	m.task.SetValue("")
	m.task.Placeholder = "what should Claude do on " + repo + "? (it opens a pull request when done)"
	return m, m.task.Focus()
}

func (m tuiModel) handleTaskKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.Type {
	case tea.KeyEsc:
		m.tasking = false
		m.task.Blur()
		return m, nil
	case tea.KeyEnter:
		task := strings.TrimSpace(m.task.Value())
		if task == "" {
			return m, nil
		}
		m.tasking = false
		m.task.Blur()
		return m.startAgent(m.taskRepo, task)
	}
	var cmd tea.Cmd
	m.task, cmd = m.task.Update(msg)
	return m, cmd
}

// startAgent runs `onctl claude` for the session in the foreground (its
// progress and the GitHub token prompt show), on a new box named after
// the repo and the task, and goes back to the agents view.
func (m tuiModel) startAgent(repo, task string) (tea.Model, tea.Cmd) {
	taken := map[string]bool{}
	for _, vm := range m.vms {
		taken[vm.Name] = true
	}
	n, err := generateName(repoBoxPrefix(repo), task, func(s string) bool { return taken[s] })
	if err != nil {
		return m, m.fail("naming a box for %s: %v", repo, err)
	}
	box := n.box()
	if err := addToAgentsRegistry(box, repo); err != nil {
		return m, m.fail("remembering %s: %v", box, err)
	}
	if m.pickedForAgent() {
		m.pop()
	}
	return m.runChild("start", box, agentsStartArgs(repo, box, task))
}

// attachAgent hands the terminal to the session on its box.
func (m tuiModel) attachAgent(s agentSession) (tea.Model, tea.Cmd) {
	b := agentConn(context.Background(), s.box)
	if b == nil {
		return m, m.fail("can't reach %s", s.box)
	}
	c := exec.Command("ssh", b.attachArgs(agentAttachScript(s), nil)...)
	c.Env = append(os.Environ(), "TERM=xterm-256color")
	return m, tea.ExecProcess(c, func(err error) tea.Msg {
		// Leaving herdr or tmux ends ssh with their status; there's
		// nothing to tell then.
		return tuiChildDoneMsg{verb: "attach", name: s.box}
	})
}

// openPR is P: the box's branch pushed and a pull request opened from
// it, its URL flashed.
func (m tuiModel) openPR(s agentSession) tea.Cmd {
	return func() tea.Msg {
		msg := tuiActionMsg{name: s.box, verb: "open a pull request from"}
		b := agentConn(context.Background(), s.box)
		if b == nil {
			msg.err = errUnreachable
			return msg
		}
		ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
		defer cancel()
		out, err := b.output(ctx, openPRScript(s.dir))
		if err != nil {
			msg.err = err
			return msg
		}
		msg.result = "pull request: " + strings.TrimSpace(string(out))
		return msg
	}
}

var errUnreachable = errors.New("can't reach the box")
