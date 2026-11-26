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

// `:runs`: the boxes running Claude (runs.go), with what Claude is
// doing -- herdr's status: working, blocked, idle, done -- and its pull
// request. n picks a repository in :repos, enter there asks for the task,
// and the run starts on a new box. The boxes are polled on the tui's
// tick once the view has been opened, in whichever view is showing, so a
// desktop notification still goes out when a Claude starts waiting.

var runsView = &tableView{
	name:     "runs",
	aliases:  []string{"run"},
	title:    "Runs",
	resource: "runs",
	columns:  []column{{"BOX", 'B'}, {"REPO", 'O'}, {"BRANCH", 0}, {"CLAUDE", 'C'}, {"PULL REQUEST", 0}},
	stateCol: 3,
	empty:    "No runs. Press n to start one.",
	hotkeys: []hotkey{
		{"enter", "attach"}, {"n", "new run"}, {"o", "open PR"}, {"P", "push+PR"}, {"ctrl-d", "destroy"},
	},
	rows: func(m *tuiModel) []tableRow {
		var rows []tableRow
		for _, s := range m.runs {
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

// runOn is the run on the box named box, nil when there's none.
func (m *tuiModel) runOn(box string) *boxRun {
	for i := range m.runs {
		if m.runs[i].box == box {
			return &m.runs[i]
		}
	}
	return nil
}

// gotRuns takes a poll's runs, notifying for each that started
// waiting since the last one.
func (m *tuiModel) gotRuns(runs []boxRun) {
	for _, s := range newlyWaiting(m.runStatuses, runs) {
		go notifyDesktop("Claude is waiting: "+s.box, orDash(s.repo)+" -- "+s.status)
	}
	m.runStatuses = map[string]string{}
	for _, s := range runs {
		m.runStatuses[s.box] = s.status
	}
	m.runs = runs
}

// handleRunsKey is the runs view's own keys; ok is false for the
// keys every table has.
func (m tuiModel) handleRunsKey(key string) (next tea.Model, cmd tea.Cmd, ok bool) {
	r := m.currentRow()
	var s *boxRun
	if r != nil {
		s = m.runOn(r.key)
	}
	switch key {
	case "n":
		m.push(page{kind: pageTable, view: reposView, sortCol: -1})
		return m, tea.Batch(m.list("repos"), m.spinIfBusy(), m.say("pick the repo; enter asks for the task")), true
	case "enter":
		if s == nil {
			return m, nil, true
		}
		next, cmd := m.attachRun(*s)
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

// pickedForRun reports whether :repos was opened by the runs view's
// n, so enter on a repository asks for a task instead of a box.
func (m *tuiModel) pickedForRun() bool {
	return len(m.pages) > 1 && m.top().view == reposView && m.pages[len(m.pages)-2].view == runsView
}

// askTask opens the prompt for a task on repo: its description, for a new run.
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
		return m.startRun(m.taskRepo, task)
	}
	var cmd tea.Cmd
	m.task, cmd = m.task.Update(msg)
	return m, cmd
}

// startRun runs `onctl claude` for the run in the foreground (its
// progress and the GitHub token prompt show), on a new box named after
// the repo and the task, and goes back to the runs view.
func (m tuiModel) startRun(repo, task string) (tea.Model, tea.Cmd) {
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
	if m.pickedForRun() {
		m.pop()
	}
	return m.runChild("start", box, runStartArgs(repo, box, task))
}

// attachRun hands the terminal to the run on its box.
func (m tuiModel) attachRun(s boxRun) (tea.Model, tea.Cmd) {
	b := agentConn(context.Background(), s.box)
	if b == nil {
		return m, m.fail("can't reach %s", s.box)
	}
	c := exec.Command("ssh", b.attachArgs(runAttachScript(s), nil)...)
	c.Env = append(os.Environ(), "TERM=xterm-256color")
	return m, tea.ExecProcess(c, func(err error) tea.Msg {
		// Leaving herdr or tmux ends ssh with their status; there's
		// nothing to tell then.
		return tuiChildDoneMsg{verb: "attach", name: s.box}
	})
}

// openPR is P: the box's branch pushed and a pull request opened from
// it, its URL flashed.
func (m tuiModel) openPR(s boxRun) tea.Cmd {
	return func() tea.Msg {
		msg := tuiActionMsg{name: s.box, verb: "push"}
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
