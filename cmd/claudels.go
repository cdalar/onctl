package cmd

import (
	"context"
	"fmt"
	"os"
	"sort"
	"strconv"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/spf13/cobra"
)

var claudeLsCmd = &cobra.Command{
	Use:   "ls",
	Short: "List this project's Claude sessions: the main one and its tasks",
	Long: `Lists the Claude sessions working on this project: the main checkout and
each --task worktree on the project's box (with its branch, whether
Claude is running there, and when it last showed activity), plus the
boxes of --own-box tasks.`,
	Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, _ []string) error {
		return claudeList(cmd.Context())
	},
}

func init() {
	claudeCmd.AddCommand(claudeLsCmd)
}

func claudeList(ctx context.Context) error {
	dir, err := projectDir()
	if err != nil {
		return err
	}
	main := claudeBox
	if main == "" {
		main = claudeBoxName(dir)
	}
	vms, err := newClient().List(ctx)
	if err != nil {
		return err
	}

	tw := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	_, _ = fmt.Fprintln(tw, "TASK\tBRANCH\tCLAUDE\tACTIVE\tBOX")
	// The project's box first, then the --own-box tasks' boxes.
	sort.SliceStable(vms, func(i, j int) bool { return vms[i].Name == main && vms[j].Name != main })
	rows := 0
	for _, vm := range vms {
		switch {
		case vm.Name == main && vm.State == "running":
			n, err := listBoxSessions(ctx, tw, dir)
			if err != nil {
				return err
			}
			rows += n
		case vm.Name == main:
			_, _ = fmt.Fprintf(tw, "(all)\t-\t-\t-\t%s (%s)\n", vm.Name, vm.State)
			rows++
		case strings.HasPrefix(vm.Name, main+"-"):
			_, _ = fmt.Fprintf(tw, "%s\t-\t-\t-\t%s (own box, %s)\n", strings.TrimPrefix(vm.Name, main+"-"), vm.Name, vm.State)
			rows++
		}
	}
	if rows == 0 {
		fmt.Fprintf(os.Stderr, "No Claude sessions for %s -- start one with onctl claude.\n", dir)
		return nil
	}
	return tw.Flush()
}

// listBoxSessions writes a row per worktree on the project's (running)
// box.
func listBoxSessions(ctx context.Context, tw *tabwriter.Writer, dir string) (int, error) {
	t, err := openClaudeBox(ctx, false, false)
	if err != nil {
		return 0, err
	}
	defer t.box.close()
	if t.needsCopy {
		return 0, nil
	}
	out, err := t.box.output(ctx, "git -C "+shellQuote(dir)+" worktree list --porcelain 2>/dev/null; echo '--tmux--'; tmux ls -F '#{session_name} #{session_activity}' 2>/dev/null || true")
	if err != nil {
		return 0, err
	}
	porcelain, sessions, _ := strings.Cut(string(out), "--tmux--")
	branches := worktreeBranches(porcelain)
	activity := map[string]int64{}
	for _, line := range strings.Split(sessions, "\n") {
		name, at, ok := strings.Cut(strings.TrimSpace(line), " ")
		if !ok {
			continue
		}
		if n, err := strconv.ParseInt(at, 10, 64); err == nil {
			activity[name] = n
		}
	}
	worktrees := parseWorktrees(porcelain, dir)
	sort.SliceStable(worktrees, func(i, j int) bool { return worktrees[i].task < worktrees[j].task })
	for _, w := range worktrees {
		s := newTaskSession(dir, w.task)
		task := w.task
		if task == "" {
			task = "(main)"
		}
		state, active := "stopped", "-"
		if at, ok := activity[s.tmux]; ok {
			state, active = "running", ago(time.Unix(at, 0))
		}
		branch := branches[w.dir]
		if branch == "" {
			branch = "-"
		}
		_, _ = fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\n", task, branch, state, active, t.name)
	}
	return len(worktrees), nil
}

// worktreeBranches maps each worktree's path to its branch, from `git
// worktree list --porcelain`.
func worktreeBranches(porcelain string) map[string]string {
	branches := map[string]string{}
	path := ""
	for _, line := range strings.Split(porcelain, "\n") {
		if p, ok := strings.CutPrefix(line, "worktree "); ok {
			path = p
		} else if b, ok := strings.CutPrefix(line, "branch refs/heads/"); ok {
			branches[path] = b
		}
	}
	return branches
}

func ago(t time.Time) string {
	d := time.Since(t).Round(time.Second)
	switch {
	case d < time.Minute:
		return "just now"
	case d < time.Hour:
		return fmt.Sprintf("%dm ago", int(d.Minutes()))
	case d < 48*time.Hour:
		return fmt.Sprintf("%dh ago", int(d.Hours()))
	}
	return fmt.Sprintf("%dd ago", int(d.Hours()/24))
}
