package cmd

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"
	"path"
	"regexp"
	"strings"

	"github.com/spf13/cobra"
)

// onctl claude --repo owner/name: no project here. A box of the selected
// provider named claude-<name>, the repository's default branch cloned
// from GitHub onto a new branch of its own (onctl/<id>), and Claude
// started in it -- attached, or with --detach. The box is the project's
// box like any other: run with the same --repo again, onctl claude comes
// back to it (the clone isn't made again), and onctl pause/destroy apply.
// After boxctl's --repo, without its goals and ephemeral boxes.

// repoCloneRoot is where a repository is cloned on the box: /root/<name>.
const repoCloneRoot = "/root"

var repoPattern = regexp.MustCompile(`^[A-Za-z0-9_.-]+/[A-Za-z0-9_.-]+$`)

// parseRepo checks owner/name, accepting a trailing .git, and returns it
// without.
func parseRepo(s string) (string, error) {
	s = strings.TrimSuffix(strings.TrimSpace(s), ".git")
	if !repoPattern.MatchString(s) {
		return "", fmt.Errorf("--repo %q: use owner/name", s)
	}
	return s, nil
}

// repoDir is where repo is cloned on the box.
func repoDir(repo string) string {
	_, name, _ := strings.Cut(repo, "/")
	return path.Join(repoCloneRoot, name)
}

// repoBranch is the branch Claude works on: onctl/<6 hex digits>, new
// each time a repository is cloned.
func repoBranch() string {
	var b [3]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "onctl/work"
	}
	return "onctl/" + hex.EncodeToString(b[:])
}

// cloneScript clones url's default branch into dir and branches. The
// clone fetches one branch, but maps them all, so a later fetch sees the
// rest. GIT_TERMINAL_PROMPT=0 makes a repository GitHub answers 401 for
// (a private one, or a mistyped name) fail at once instead of waiting
// for a username nobody can type.
func cloneScript(url, dir, branch string) string {
	d := shellQuote(dir)
	return `set -e
mkdir -p "$(dirname ` + d + `)"
GIT_TERMINAL_PROMPT=0 git clone -q --single-branch ` + shellQuote(url) + ` ` + d + `
git -C ` + d + ` config remote.origin.fetch '+refs/heads/*:refs/remotes/origin/*'
git -C ` + d + ` checkout -q -b ` + shellQuote(branch)
}

// repoConflicts names the claude flags that make no sense with --repo:
// the ones about a project here, or a box that holds one.
func repoConflicts() []string {
	var conflicts []string
	for _, f := range []struct {
		flag string
		set  bool
	}{
		{"--task", claudeTask != ""}, {"--own-box", claudeOwnBox},
		{"--handoff", claudeHandoff != ""}, {"--project", claudeProject != ""},
	} {
		if f.set {
			conflicts = append(conflicts, f.flag)
		}
	}
	return conflicts
}

// runRepo is onctl claude --repo: runClaude's shape, with a clone from
// GitHub on the box in place of a copy from here.
func runRepo(cmd *cobra.Command, args []string) error {
	ctx := cmd.Context()
	repo, err := parseRepo(claudeRepo)
	if err != nil {
		return err
	}
	if conflicts := repoConflicts(); len(conflicts) > 0 {
		return fmt.Errorf("--repo clones a repository on a fresh box, so it can't be combined with %s", strings.Join(conflicts, ", "))
	}
	env, apiKey, err := claudeEnv(claudeAuth)
	if err != nil {
		return err
	}
	dir := repoDir(repo)
	name := claudeBox
	if name == "" {
		name = claudeBoxName(dir)
	}
	var box *boxSSH
	if onBoxes() {
		box, err = connectClaudeBox(ctx, name, true, cmd.Flags().Changed("idle-ttl"))
	} else {
		box, _, err = connectClaudeVM(ctx, name, true)
	}
	if err != nil {
		return err
	}
	defer box.close()
	if err := box.run(ctx, "true", nil); err != nil {
		return fmt.Errorf("can't ssh into %s: %w", name, err)
	}
	// A box that holds another project is refused, as with --box.
	needsClone, boxMux, err := checkProject(ctx, box, dir)
	if err != nil {
		return err
	}
	if err := useBoxMux(ctx, box, dir, boxMux); err != nil {
		return err
	}
	// First: the clone needs git, and a VM made from a plain image has none.
	if err := box.run(ctx, claudeSetupScript(), nil); err != nil {
		return fmt.Errorf("installing Claude Code on %s: %w", name, err)
	}
	// -C "": git reads your global identity from wherever this runs.
	if err := copyGitIdentity(ctx, box, ""); err != nil {
		fmt.Fprintf(os.Stderr, "warning: couldn't set your git identity on %s: %v\n", name, err)
	}
	if claudeGitHub == githubStore {
		if err := storeGitHubToken(ctx, box, repo); err != nil {
			return err
		}
	}
	if needsClone {
		branch := repoBranch()
		fmt.Fprintf(os.Stderr, "Cloning %s on %s...\n", repo, name)
		if err := box.run(ctx, cloneScript("https://github.com/"+repo+".git", dir, branch), nil); err != nil {
			if claudeGitHub != githubStore {
				return fmt.Errorf("cloning %s on %s: %w (a private repository needs --github store, a token kept on the box)", repo, name, err)
			}
			return fmt.Errorf("cloning %s on %s: %w", repo, name, err)
		}
		if err := markProject(ctx, box, dir); err != nil {
			return err
		}
		fmt.Fprintf(os.Stderr, "On branch %s. Come back to Claude with: %s\n", branch, repoAttachHint(repo))
	} else {
		fmt.Fprintf(os.Stderr, "%s is already on %s; working with the box's copy (it isn't cloned again).\n", repo, name)
	}
	if err := copyClaudeConfig(ctx, box); err != nil {
		fmt.Fprintf(os.Stderr, "warning: couldn't copy your Claude configuration: %v\n", err)
	}
	if err := box.run(ctx, claudeStateScript([]string{dir}, localClaudeTheme(), apiKey), nil); err != nil {
		fmt.Fprintf(os.Stderr, "warning: couldn't skip Claude's first-run setup on %s: %v\n", name, err)
	}
	sess := newTaskSession(dir, "")
	running := sessionRunning(ctx, box, sess)
	return startSession(ctx, box, name, sess, env, running, repoAttachHint(repo), args)
}

// repoAttachHint is the command that comes back to a --repo box: the
// same --repo (--box from a directory here would be another project),
// naming the provider when it isn't the default.
func repoAttachHint(repo string) string {
	if onBoxes() {
		return "onctl claude --repo " + repo
	}
	return "onctl -p " + cloudProvider + " claude --repo " + repo
}

func init() {
	claudeCmd.Flags().StringVarP(&claudeRepo, "repo", "R", "", "GitHub repository (owner/name) to clone on a box of its own (claude-<name>), on a new onctl/... branch, instead of copying a project from here")
}
