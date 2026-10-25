package cmd

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/cdalar/onctl/pkg/cloud"
)

// An ephemeral run -- onctl claude without --keep, --box, --detach or
// --task -- gets a box of its own, created for this run and deleted when
// it ends: Claude exits, or setup fails. Not when you detach or the
// connection drops with Claude still running: the box is kept, its
// session going (see runClaude). Before the box goes, its branches and
// uncommitted work are fetched here as box/* (the same as onctl claude
// fetch), so nothing Claude did is lost with it. After boxctl's.

var (
	claudeKeep bool
	// claudeBranch is the branch a generated box works on, set with its
	// name; "" for a kept box, whose branch is onctl/<project>.
	claudeBranch string
)

// ephemeralRun reports whether this run's box is deleted when it ends.
// Call it after resolveTask, which turns --own-box into --box.
func ephemeralRun() bool {
	return !claudeKeep && claudeBox == "" && !claudeDetach && claudeTask == ""
}

// ephemeralBoxName is a fresh box name for this project:
// <project>-<slug>-<id>, the slug from --prompt if there is one (see
// boxName).
func ephemeralBoxName(ctx context.Context, dir string) (boxName, error) {
	return newBoxName(ctx, filepath.Base(dir), claudePrompt)
}

// saveBoxWork fetches the box's branches and uncommitted work as box/*,
// if the project is a git repository that made it onto the box. It
// reports whether the box can go: not when there was work to fetch and
// fetching it failed.
func saveBoxWork(ctx context.Context, t *claudeTarget) bool {
	if t.needsCopy || exec.CommandContext(ctx, "git", "-C", t.dir, "rev-parse", "--git-dir").Run() != nil {
		return true
	}
	g := &claudeGitTarget{t}
	fmt.Fprintf(os.Stderr, "Fetching %s's work...\n", t.name)
	err := g.ensureRemote(ctx)
	if err == nil {
		err = fetchBox(ctx, g)
	}
	if err != nil {
		fmt.Fprintf(os.Stderr, "Couldn't fetch %s's work (%v), so it's kept: get it with %s, then %s.\n",
			t.name, err, providerHint("onctl claude fetch --box "+t.name), destroyHint(t.name))
		return false
	}
	return true
}

// end closes the ssh to t's box and, for an ephemeral run, saves its
// work and deletes it. It runs on a context of its own: the run's may
// have been cancelled by Ctrl-C.
func (t *claudeTarget) end() {
	discard := false
	if t.ephemeral {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
		discard = saveBoxWork(ctx, t)
		cancel()
	}
	t.box.close()
	if discard {
		discardBox(t.name)
	}
}

// discardBox deletes this run's box.
func discardBox(name string) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	fmt.Fprintf(os.Stderr, "Deleting %s...\n", name)
	var err error
	if onBoxes() {
		err = newClient().Destroy(ctx, name)
	} else {
		err = provider.Destroy(cloud.Vm{Name: name})
	}
	if err != nil {
		fmt.Fprintf(os.Stderr, "Couldn't delete %s: %v -- delete it with %s\n", name, err, destroyHint(name))
		return
	}
	fmt.Fprintf(os.Stderr, "Deleted %s.\n", name)
}

// discardIfMade deletes the box named name if it exists: for a run that
// failed before it knew whether creating the box got as far as that.
func discardIfMade(ctx context.Context, name string) {
	vms, err := listClaudeVMs(ctx)
	if err != nil {
		return
	}
	for _, vm := range vms {
		if vm.name == name {
			discardBox(name)
			return
		}
	}
}

// destroyHint is the command that deletes the box by hand.
func destroyHint(name string) string { return providerHint("onctl destroy " + name) }

// providerHint is an onctl command line naming the provider when it isn't
// the default.
func providerHint(line string) string {
	if onBoxes() {
		return line
	}
	return "onctl -p " + cloudProvider + " " + strings.TrimPrefix(line, "onctl ")
}
