package cmd

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/spf13/cobra"
)

// The box as a git remote: `onctl claude fetch` brings the box's
// branches -- pushed to GitHub or not -- and its uncommitted work back as
// box/*, and `onctl claude push` sends local commits there. Plain git
// over the same ssh as everything else, so nothing is copied file by
// file and nothing is synced. See boxctl-vms's
// docs/plans/claude-on-the-box.md ("Getting results back").

// boxRemote is the name of the git remote for the project's box.
const boxRemote = "box"

// wipRef is where fetch snapshots the main checkout's uncommitted work, on
// the box, before fetching it as box/wip; a task's goes to wipRef-<task>,
// fetched as box/wip-<task>. (Not wip/<task>: git can't have both a ref
// and a directory of refs by one name.)
const wipRef = "refs/boxctl/wip"

var claudeFetchCmd = &cobra.Command{
	Use:   "fetch",
	Short: "Fetch the box's branches and uncommitted work as box/*",
	Long: `Fetches every branch of the project on its box into this repository as
box/<branch>, whether or not Claude has pushed it anywhere, plus
box/wip: a commit of the box's uncommitted work (tracked changes and
untracked files that aren't ignored) on top of whatever the box has
checked out. Nothing on the box changes.

  onctl claude fetch
  git log box/fix-auth
  git diff main box/fix-auth
  git show --stat box/wip           # what Claude is in the middle of
  git checkout -b fix-auth box/fix-auth

The box is a git remote named box (root@<box>.box:<project dir>);
onctl claude fetch and push supply the ssh to reach it.`,
	Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, _ []string) error {
		return claudeFetch(cmd.Context())
	},
}

var claudePushCmd = &cobra.Command{
	Use:   "push [refspec...]",
	Short: "Push local commits to the project on its box",
	Long: `Pushes to the project on its box -- the current branch, or the refspecs
given, as git push would -- so Claude has commits made here: a fix, a
rebase onto the latest main.

Pushing to the branch the box has checked out updates its files too, as
long as Claude has no uncommitted changes there; with some, git refuses
rather than overwrite them. Push to another branch then, or have Claude
commit first.

  onctl claude push                 # the current branch
  onctl claude push main:main`,
	RunE: func(cmd *cobra.Command, args []string) error {
		return claudePush(cmd.Context(), args)
	},
}

func init() {
	claudeCmd.AddCommand(claudeFetchCmd, claudePushCmd)
}

func claudeFetch(ctx context.Context) error {
	t, err := openClaudeGit(ctx)
	if err != nil {
		return err
	}
	defer t.box.close()

	// Snapshot the uncommitted work of the main checkout and of every
	// task's worktree: box/wip, box/wip-<task>.
	worktrees, err := boxWorktrees(ctx, t.box, t.dir)
	if err != nil {
		return err
	}
	refspecs := []string{"+refs/heads/*:refs/remotes/" + boxRemote + "/*"}
	var withWip []string
	for _, w := range worktrees {
		out, err := t.box.output(ctx, wipSnapshotScript(w.dir, w.wipRef()))
		if err != nil {
			return fmt.Errorf("snapshotting the uncommitted work in %s on %s: %w", w.dir, t.name, err)
		}
		if strings.TrimSpace(string(out)) != "none" {
			refspecs = append(refspecs, "+"+w.wipRef()+":refs/remotes/"+boxRemote+"/"+w.wipName())
			withWip = append(withWip, boxRemote+"/"+w.wipName())
		}
	}
	// One fetch, so --prune sees each box/wip* as a snapshot's destination
	// and keeps it while there is one -- and drops it, along with branches
	// deleted on the box, once there isn't.
	if err := t.git(ctx, append([]string{"fetch", "--prune", boxRemote}, refspecs...)...); err != nil {
		return err
	}

	out, err := exec.CommandContext(ctx, "git", "-C", t.dir, "for-each-ref",
		"--format=%(refname:short)\t%(objectname:short)\t%(subject)", "refs/remotes/"+boxRemote+"/").Output()
	if err != nil {
		return err
	}
	fmt.Fprintf(os.Stderr, "From %s:\n", t.name)
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		if line != "" {
			fmt.Fprintf(os.Stderr, "  %s\n", strings.ReplaceAll(line, "\t", "  "))
		}
	}
	if len(withWip) == 0 {
		fmt.Fprintln(os.Stderr, "No uncommitted work on the box.")
	} else {
		fmt.Fprintf(os.Stderr, "Uncommitted work on the box: git show --stat %s\n", strings.Join(withWip, ", "))
	}
	return nil
}

func claudePush(ctx context.Context, refspecs []string) error {
	t, err := openClaudeGit(ctx)
	if err != nil {
		return err
	}
	defer t.box.close()
	// Boxes from before boxctl set this when copying a project.
	if err := t.box.run(ctx, "git -C "+shellQuote(t.dir)+" config receive.denyCurrentBranch updateInstead", nil); err != nil {
		return err
	}
	if len(refspecs) == 0 {
		refspecs = []string{"HEAD"}
	}
	if err := t.git(ctx, append([]string{"push", boxRemote}, refspecs...)...); err != nil {
		return fmt.Errorf("%w (if the box's checked-out branch has uncommitted changes, push to another branch or have Claude commit first)", err)
	}
	return nil
}

// claudeGitTarget is a claudeTarget whose project is a git repository on
// both sides, with the box set up as its box remote.
type claudeGitTarget struct {
	*claudeTarget
}

func openClaudeGit(ctx context.Context) (*claudeGitTarget, error) {
	dir, err := projectDir()
	if err != nil {
		return nil, err
	}
	if err := exec.CommandContext(ctx, "git", "-C", dir, "rev-parse", "--git-dir").Run(); err != nil {
		return nil, fmt.Errorf("%s isn't a git repository", dir)
	}
	t, err := openClaudeBox(ctx, false, false)
	if err != nil {
		return nil, err
	}
	if t.needsCopy {
		t.box.close()
		return nil, fmt.Errorf("%s doesn't have %s yet -- run onctl claude first", t.name, t.dir)
	}
	g := &claudeGitTarget{t}
	if err := g.ensureRemote(ctx); err != nil {
		t.box.close()
		return nil, err
	}
	return g, nil
}

// boxRemoteURL is the box remote's URL. The host is <box>.box, so plain
// git reaches it too with the ssh config the README suggests (Host *.box,
// ProxyCommand onctl ssh-proxy -p boxes %n).
func boxRemoteURL(name, dir string) string {
	return "root@" + name + ".box:" + dir
}

// ensureRemote adds the box remote, or points it at this box.
func (g *claudeGitTarget) ensureRemote(ctx context.Context) error {
	want := boxRemoteURL(g.name, g.dir)
	out, err := exec.CommandContext(ctx, "git", "-C", g.dir, "remote", "get-url", boxRemote).Output()
	switch {
	case err != nil:
		return exec.CommandContext(ctx, "git", "-C", g.dir, "remote", "add", boxRemote, want).Run()
	case strings.TrimSpace(string(out)) != want:
		return exec.CommandContext(ctx, "git", "-C", g.dir, "remote", "set-url", boxRemote, want).Run()
	}
	return nil
}

// git runs git here with the box's ssh, its output passed through.
func (g *claudeGitTarget) git(ctx context.Context, args ...string) error {
	cmd := exec.CommandContext(ctx, "git", append([]string{"-C", g.dir}, args...)...)
	cmd.Env = append(os.Environ(), "GIT_SSH_COMMAND="+g.box.gitSSHCommand())
	cmd.Stdout, cmd.Stderr = os.Stderr, os.Stderr
	return cmd.Run()
}

// boxWorktree is the main checkout ("" task) or a task's worktree.
type boxWorktree struct {
	task, dir string
}

func (w boxWorktree) wipRef() string {
	if w.task == "" {
		return wipRef
	}
	return wipRef + "-" + w.task
}

func (w boxWorktree) wipName() string {
	if w.task == "" {
		return "wip"
	}
	return "wip-" + w.task
}

// boxWorktrees lists the project's main checkout and its tasks' worktrees
// on the box.
func boxWorktrees(ctx context.Context, box *boxSSH, project string) ([]boxWorktree, error) {
	out, err := box.output(ctx, "git -C "+shellQuote(project)+" worktree list --porcelain")
	if err != nil {
		return nil, fmt.Errorf("listing worktrees on %s: %w", box.name, err)
	}
	return parseWorktrees(string(out), project), nil
}

// parseWorktrees picks the main checkout and the tasks out of `git
// worktree list --porcelain`: worktrees at <project>@<task>, the only
// ones onctl makes.
func parseWorktrees(porcelain, project string) []boxWorktree {
	worktrees := []boxWorktree{{dir: project}}
	for _, line := range strings.Split(porcelain, "\n") {
		path, ok := strings.CutPrefix(line, "worktree ")
		if !ok {
			continue
		}
		task, ok := strings.CutPrefix(path, project+"@")
		if ok && taskNamePattern.MatchString(task) {
			worktrees = append(worktrees, boxWorktree{task: task, dir: path})
		}
	}
	return worktrees
}

// wipSnapshotScript commits a working tree on the box -- tracked changes
// and untracked files that aren't ignored, as `git add -A` sees them -- to
// ref on top of its HEAD, through a throwaway index so neither the real
// index nor the files change. Prints the commit, or "none" (and drops
// wipRef) when the tree matches HEAD.
func wipSnapshotScript(dir, ref string) string {
	return `set -e
cd ` + shellQuote(dir) + `
idx=$(mktemp); trap 'rm -f "$idx"' EXIT
real="$(git rev-parse --git-dir)/index"
# Start from the real index (keeps git's stat cache, so add -A is quick),
# or from none: an empty file isn't a valid index.
if [ -f "$real" ]; then cp "$real" "$idx"; else rm -f "$idx"; fi
export GIT_INDEX_FILE="$idx"
git add -A
tree=$(git write-tree)
export GIT_AUTHOR_NAME="${GIT_AUTHOR_NAME:-$(git config user.name || echo onctl)}"
export GIT_AUTHOR_EMAIL="${GIT_AUTHOR_EMAIL:-$(git config user.email || echo onctl@localhost)}"
export GIT_COMMITTER_NAME="$GIT_AUTHOR_NAME" GIT_COMMITTER_EMAIL="$GIT_AUTHOR_EMAIL"
if head=$(git rev-parse -q --verify 'HEAD^{commit}'); then
  if [ "$tree" = "$(git rev-parse 'HEAD^{tree}')" ]; then git update-ref -d ` + ref + ` 2>/dev/null || true; echo none; exit 0; fi
  c=$(git commit-tree "$tree" -p "$head" -m "Uncommitted work on the box")
else
  c=$(git commit-tree "$tree" -m "Uncommitted work on the box")
fi
git update-ref ` + ref + ` "$c"
echo "$c"`
}
