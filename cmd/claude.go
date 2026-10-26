package cmd

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/cdalar/onctl/internal/providerboxes"
	"github.com/cdalar/onctl/pkg/cloud"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

// Running Claude Code itself on a box, with the project there -- see
// boxctl-vms's docs/plans/claude-on-the-box.md. Unlike the Claude Code
// plugin, which keeps Claude here and sends only its Bash commands to a
// box, this moves Claude: its file tools and its shell are both on the
// box, so the project is copied there once and never synced.
//
// Everything after the box is up goes over real ssh to the box's sshd,
// tunneled by `onctl ssh-proxy` as ssh's ProxyCommand: copying the
// project needs a byte stream into the box, and attaching needs a pty
// plus a command, neither of which the boxes service's own terminal protocol
// carries.

const (
	claudeDefaultImage = "claude-agent"
	claudeDefaultSize  = "medium"
	// claudeSession is the session Claude runs in on the box (in tmux, or
	// herdr): one per box, so a second `onctl claude` attaches to it.
	claudeSession = "claude"
	// claudeInstall is Anthropic's installer, run on first use on an image
	// without Claude Code (a cloud VM's; the claude-agent image has it
	// baked in). It installs to ~/.local/bin and keeps itself updated.
	claudeInstall = "curl -fsSL https://claude.ai/install.sh | bash"
	// acClaudeImage is what a sandbox for Claude boots from on the ac
	// provider unless --image says otherwise: boxctl's claude-agent image
	// (images/claude-agent in boxctl-io/boxctl-images), published to ghcr
	// for Apple containers, with sshd, tmux, git, gh, the GitHub
	// forwarding helpers and Claude Code baked in -- so a new sandbox
	// starts in seconds instead of installing them all. Only ac: the other
	// VM providers take their cloud's images, not OCI ones.
	acClaudeImage = "ghcr.io/boxctl-io/claude-agent:latest"
	// claudeReadyTimeout bounds waiting for a new or resumed box to boot.
	claudeReadyTimeout = 3 * time.Minute
	// claudeDefaultIdleTTL is the idle TTL of a box created for Claude.
	// The reaper counts relayed traffic as use, and a detached Claude
	// working on its own makes none, so the server's 6h would pause it
	// mid-task.
	claudeDefaultIdleTTL = 24 * time.Hour
	// projectMarker on the box records which project it holds, so a box
	// is never handed a second one by running onctl claude --box from
	// another directory.
	projectMarker = "/root/.boxctl/project"
)

var (
	claudeBox     string
	claudeSize    string
	claudeImage   string
	claudePrompt  string
	claudeDetach  bool
	claudeGitHub  string
	claudeIdle    time.Duration
	claudeProject string
	claudeHandoff string
	claudeTask    string
	claudeOwnBox  bool
	claudeAuth    string
	claudeRepo    string
)

var claudeCmd = &cobra.Command{
	Use:   "claude [-- claude-args...]",
	Short: "Run Claude Code on a box, with this project on it",
	Long: `Runs Claude Code on a box instead of on this machine, attached to your
terminal. Each run creates a box of its own (<project>-<slug>-<id>, the
slug from --prompt) and puts the project there, at the same absolute
path: a git repository with an origin is cloned from it -- the default
branch, then a new branch onctl/<slug>-<id> for Claude to work on;
nothing local, uncommitted or unpushed, goes -- and anything else is
copied (.gitignore'd files left out). Then it installs Claude Code and
your Claude configuration, and starts Claude in it. When Claude exits,
the box's branches and uncommitted work are fetched here as box/* (as
onctl claude fetch does) and the box is deleted; detaching (Ctrl-b d;
Ctrl-b q in herdr) with Claude still running keeps it, and the message
says how to come back.

--keep uses the project's own box (claude-<project>) instead, and keeps
it: the first time it's created and the project put there the same way
(a clone on branch onctl/<project>, or a copy); later runs come back to
it, don't copy again, and nothing is synced back -- Claude commits and
pushes from the box. --box, --detach and --task keep their box too.

The box is a hosted one (-p boxes, the default) or a VM of any other
provider: onctl -p hetzner claude creates claude-<project> on Hetzner
the way onctl create does (--size and --image are the provider's type
and image; unset, its onctl.yaml defaults), reaches it over ssh with your
key, installs tmux, git and Claude Code, and from there on it's the same.
onctl -p ac claude (Apple containers, on a Mac) boots the image a box
has, ghcr.io/boxctl-io/claude-agent, so nothing is installed. A VM is
paused and resumed with onctl pause/resume, not on its own.

Claude runs in tmux, so it keeps going when you detach (Ctrl-b d) or the
connection drops; onctl claude --box <name> comes back to it. --mux herdr
runs it in herdr (herdr.dev) instead: a terminal workspace manager made
for coding agents that shows every task on the box in one window, each
marked working, blocked or idle; Ctrl-b q detaches. ONCTL_MUX=herdr makes
it the default, and a box keeps using whichever its sessions were started
in.

  onctl claude                           # a new box, deleted when Claude exits
  onctl claude --keep                    # create or reattach to claude-<project>
  onctl claude --prompt "fix the flaky auth test"
  onctl claude --detach --prompt "..."   # start it, don't attach
  onctl claude -- --model opus           # arguments for claude itself
  onctl claude --mux herdr               # in herdr instead of tmux
  onctl -p hetzner claude                # the same, on your own Hetzner VM
  onctl claude --repo owner/name         # no project here: a box with the repo cloned, on a new branch

--repo owner/name needs no local copy: a box named claude-<name> gets the
repository's default branch cloned from GitHub onto a new onctl/... branch,
and Claude starts in it. The same --repo again comes back to that box. A
private repository needs --github store (the clone runs before you're
attached, so forwarding can't serve it).

The first time, log Claude in on the box with /login: it prints a URL to
open here and a code to paste back. That login stays on the box.

GitHub: while you're attached, the box borrows your gh token for git
push and gh pr ... -- it asks this machine per use, over the ssh
connection, and keeps nothing (--github forward, the default; each use is
logged to ~/.onctl/claude/github.log). Detached, it has none. For work
that must reach GitHub while you're away, --github store keeps a token
you paste on the box -- make it a fine-grained one for this repository.
--github off gives it nothing. Your git user.name and user.email are set
on the box either way. Forwarding needs the box image's helpers, so on
a VM without them (any image but claude-agent) it's off; store works
there once gh is installed on it.`,
	RunE: runClaude,
}

func init() {
	claudeCmd.PersistentFlags().StringVar(&claudeBox, "box", "", "box to use, kept after the run (default: claude-<project directory name> with --keep and for ls/fetch/push; otherwise a new box of the run's own)")
	claudeCmd.Flags().BoolVarP(&claudeKeep, "keep", "k", false, "use the project's own box, claude-<project>, and keep it -- instead of a new box deleted when Claude exits (--box, --detach and --task keep it too)")
	claudeCmd.PersistentFlags().StringVar(&claudeProject, "project", "", "project directory (default: the git repository, or directory, you're in)")
	claudeCmd.Flags().StringVar(&claudeHandoff, "handoff", "", "continue this local Claude Code session (its ID) on the box")
	claudeCmd.PersistentFlags().StringVarP(&claudeTask, "task", "t", "", "work on a task in parallel: its own worktree (<dir>@<task>, branch <task>) and Claude on the box")
	claudeCmd.PersistentFlags().BoolVar(&claudeOwnBox, "own-box", false, "with --task: give the task a box of its own (claude-<project>-<task>) instead of a worktree")
	claudeCmd.Flags().StringVar(&claudeAuth, "claude-auth", claudeAuthAuto, "how Claude on the box logs in: auto (the token onctl claude login saved, else /login), token, login or api-key")
	_ = claudeCmd.RegisterFlagCompletionFunc("claude-auth", cobra.FixedCompletions([]string{claudeAuthAuto, claudeAuthToken, claudeAuthLogin, claudeAuthAPIKey}, cobra.ShellCompDirectiveNoFileComp))
	claudeCmd.Flags().StringVarP(&claudeSize, "size", "s", claudeDefaultSize, "size of a box created for this (another provider: its VM type; default its onctl.yaml setting)")
	_ = claudeCmd.RegisterFlagCompletionFunc("size", completeSizes)
	claudeCmd.Flags().StringVarP(&claudeImage, "image", "i", claudeDefaultImage, "image of a box created for this (another provider: its VM image; default its onctl.yaml setting)")
	// No -p shorthand: in onctl that is the global --provider.
	claudeCmd.Flags().StringVar(&claudePrompt, "prompt", "", "first message for a newly started Claude")
	claudeCmd.Flags().BoolVarP(&claudeDetach, "detach", "d", false, "start Claude on the box without attaching to it")
	claudeCmd.Flags().StringVar(&claudeGitHub, "github", githubForward, "GitHub credentials for the box: forward (lent while attached), store (a token kept on the box) or off")
	_ = claudeCmd.RegisterFlagCompletionFunc("github", cobra.FixedCompletions([]string{githubForward, githubStore, githubOff}, cobra.ShellCompDirectiveNoFileComp))
	claudeCmd.Flags().DurationVar(&claudeIdle, "idle-ttl", claudeDefaultIdleTTL, "pause the box after this long unused, 10m to 720h (set on a new box, or on an existing one when given; boxes only)")
	claudeCmd.PersistentFlags().StringVar(&muxArg, "mux", defaultMux(), "what keeps Claude running on the box between attaches: tmux, or herdr (herdr.dev, installed on the box the first time). "+muxEnv+" sets the default; a box keeps the one its sessions started in")
	_ = claudeCmd.RegisterFlagCompletionFunc("mux", cobra.FixedCompletions([]string{"tmux", "herdr"}, cobra.ShellCompDirectiveNoFileComp))
	rootCmd.AddCommand(claudeCmd)
}

func runClaude(cmd *cobra.Command, args []string) error {
	ctx := cmd.Context()
	if err := resolveMux(cmd); err != nil {
		return err
	}
	switch claudeGitHub {
	case githubForward, githubStore, githubOff:
	default:
		return fmt.Errorf("--github must be %s, %s or %s", githubForward, githubStore, githubOff)
	}
	if !onBoxes() {
		// The box image's credential helper and gh wrapper do the
		// forwarding; whether the VM has them is settled once it's up
		// (resolveVMGitHub). The flag defaults are box image and size
		// names; a VM takes the provider's own defaults unless told
		// otherwise -- except the image on ac, which runs a box's.
		if !cmd.Flags().Changed("size") {
			claudeSize = ""
		}
		if !cmd.Flags().Changed("image") {
			claudeImage = vmClaudeImage()
		}
	}
	if claudeRepo != "" {
		return runRepo(cmd, args)
	}
	if err := resolveTask(); err != nil {
		return err
	}
	// Everything that can fail here fails before any box is created or
	// touched: a handoff's transcript, the Claude credentials.
	transcript := ""
	if claudeHandoff != "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return err
		}
		if transcript, err = findTranscript(filepath.Join(home, ".claude", "projects"), claudeHandoff); err != nil {
			return err
		}
	}
	env, apiKey, err := claudeEnv(claudeAuth)
	if err != nil {
		return err
	}
	// Ctrl-C reaches the deferred end below: an ephemeral box made so far
	// is deleted rather than left behind.
	ctx, stop := signal.NotifyContext(ctx, os.Interrupt, syscall.SIGTERM)
	defer stop()
	ephemeral := ephemeralRun()
	if ephemeral {
		dir, err := projectDir()
		if err != nil {
			return err
		}
		n, err := ephemeralBoxName(ctx, dir)
		if err != nil {
			return err
		}
		claudeBox, claudeBranch = n.box(), n.branch()
	}

	t, err := openClaudeBox(ctx, true, cmd.Flags().Changed("idle-ttl"))
	if err != nil {
		if ephemeral {
			discardIfMade(context.Background(), claudeBox)
		}
		return err
	}
	t.ephemeral = ephemeral
	defer t.end()
	dir, name, box := t.dir, t.name, t.box
	if ephemeral {
		fmt.Fprintf(os.Stderr, "Ephemeral: %s is deleted when Claude exits, after its work is fetched here as box/* (--keep keeps a box).\n", name)
	}
	resolveVMGitHub(ctx, box, cmd.Flags().Changed("github"))

	// First: everything after this needs git on the box, and a VM made
	// from a plain image has none.
	if err := box.run(ctx, claudeSetupScript(), nil); err != nil {
		return fmt.Errorf("installing Claude Code on %s: %w", name, err)
	}
	// Before the project: a clone of a private repository needs the token.
	if claudeGitHub == githubStore {
		if err := storeGitHubToken(ctx, box, githubRepo(dir)); err != nil {
			return err
		}
	}
	if t.needsCopy {
		if url := cloneURL(dir); url != "" {
			branch := claudeBranch
			if branch == "" {
				branch = "onctl/" + strings.TrimPrefix(name, "claude-")
			}
			if err := cloneProject(ctx, box, dir, url, branch); err != nil {
				return err
			}
		} else if err := copyProject(ctx, box, dir); err != nil {
			return err
		}
		if err := markProject(ctx, box, dir); err != nil {
			return err
		}
		t.needsCopy = false
	} else if claudeTask == "" {
		fmt.Fprintf(os.Stderr, "%s is already on %s; working with the box's copy (it isn't copied again).\n", dir, name)
		if claudeHandoff != "" {
			fmt.Fprintf(os.Stderr, "Your local changes since then aren't on the box: commit them and run onctl claude push, or hand off to a fresh box with --box.\n")
		}
	}
	sess := newTaskSession(dir, claudeTask)
	sess.ephemeral = t.ephemeral
	if sess.task != "" {
		if err := box.run(ctx, worktreeScript(dir, sess.task), nil); err != nil {
			return fmt.Errorf("making the %s worktree on %s: %w", sess.task, name, err)
		}
	}
	if err := copyClaudeConfig(ctx, box); err != nil {
		fmt.Fprintf(os.Stderr, "warning: couldn't copy your Claude configuration: %v\n", err)
	}
	if err := copyGitIdentity(ctx, box, dir); err != nil {
		fmt.Fprintf(os.Stderr, "warning: couldn't set your git identity on %s: %v\n", name, err)
	}
	if err := box.run(ctx, claudeStateScript([]string{sess.dir}, localClaudeTheme(), apiKey), nil); err != nil {
		fmt.Fprintf(os.Stderr, "warning: couldn't skip Claude's first-run setup on %s: %v\n", name, err)
	}

	running := sessionRunning(ctx, box, sess)
	if claudeHandoff != "" {
		if running {
			return fmt.Errorf("a Claude session is already running there -- attach with %s and quit it, then hand off again", attachHint(sess, name))
		}
		if err := copyTranscript(ctx, box, sess.dir, transcript, claudeHandoff); err != nil {
			return err
		}
		args = append([]string{"--resume", claudeHandoff}, args...)
	}
	err = startSession(ctx, box, name, sess, env, running, attachHint(sess, name), args)
	if err == nil && t.ephemeral && !claudeDetach {
		// Detaching (Ctrl-b d in tmux, Ctrl-b q in herdr) returns here with
		// Claude still running -- maybe mid-task -- so the box isn't this
		// run's to end: kept, work and all.
		sctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		if sessionRunning(sctx, box, sess) {
			t.ephemeral = false
			fmt.Fprintf(os.Stderr, "Claude is still running on %s, so it's kept -- attach with: %s\n", name, attachHint(sess, name))
		}
	}
	return err
}

// startSession starts Claude in sess on the box named name -- or, with
// running set, leaves the one there be -- and attaches to it, unless
// --detach, when hint is the command that attaches later. env is what
// claudeEnv gave; args are claude's own.
func startSession(ctx context.Context, box *boxSSH, name string, sess taskSession, env string, running bool, hint string, args []string) error {
	if running {
		if claudePrompt != "" || len(args) > 0 {
			fmt.Fprintf(os.Stderr, "Claude is already running there; attaching to it (--prompt and claude arguments only apply to a new session).\n")
		}
	} else {
		if env == "" && claudeAuth == claudeAuthAuto {
			fmt.Fprintln(os.Stderr, "Log Claude in on the box with /login -- or run onctl claude login once, and every box's Claude is logged in.")
		}
		if err := stageClaudeEnv(ctx, box, sess, env); err != nil {
			return fmt.Errorf("passing Claude its credentials: %w", err)
		}
	}
	line := sess.commandLine(claudePrompt, args)
	if claudeDetach {
		if !running {
			if err := box.run(ctx, mux.start(sess, line, true), nil); err != nil {
				return fmt.Errorf("starting Claude on %s: %w", name, err)
			}
		}
		fmt.Fprintf(os.Stderr, "Claude is running on %s; attach with: %s\n", name, hint)
		return nil
	}
	return attachClaude(ctx, box, sess, mux.start(sess, line, false))
}

// resolveMux makes --mux's multiplexer the running command's.
func resolveMux(cmd *cobra.Command) error {
	m, err := muxNamed(muxArg)
	if err != nil {
		return err
	}
	mux, muxExplicit = m, cmd.Flags().Changed("mux")
	return nil
}

// sessionRunning reports whether sess's Claude is running on the box, in
// its multiplexer.
func sessionRunning(ctx context.Context, box *boxSSH, sess taskSession) bool {
	return box.run(ctx, mux.running(sess), nil) == nil
}

// attachHint is the command that comes back to this session, naming the
// provider when it isn't the default.
func attachHint(sess taskSession, box string) string {
	hint := sess.attachHint(box)
	if onBoxes() {
		return hint
	}
	return "onctl -p " + cloudProvider + " " + strings.TrimPrefix(hint, "onctl ")
}

// resolveTask checks --task and --own-box, and turns --own-box into the
// task's own box: claude-<project>-<task>, where it's the main session.
func resolveTask() error {
	if claudeTask != "" && !taskNamePattern.MatchString(claudeTask) {
		return fmt.Errorf("--task %q: use lowercase letters, digits and dashes, at most 30", claudeTask)
	}
	if !claudeOwnBox {
		return nil
	}
	if claudeTask == "" {
		return fmt.Errorf("--own-box needs --task")
	}
	if claudeBox == "" {
		dir, err := projectDir()
		if err != nil {
			return err
		}
		claudeBox = claudeBoxName(dir) + "-" + claudeTask
	}
	claudeTask = ""
	return nil
}

// attachClaude attaches to Claude on the box, lending it GitHub
// credentials for as long as the attach lasts unless --github says not to.
func attachClaude(ctx context.Context, box *boxSSH, sess taskSession, command string) error {
	extra, done := forwardGitHub(ctx, box, sess.ghSocket)
	defer done()
	return box.attach(ctx, command, extra)
}

// forwardGitHub lends this machine's GitHub token to the box for one ssh
// connection -- an attach, or the clone before it -- as the ssh arguments
// that carry the socket there; done takes the socket away again and
// reports what was used. Unless --github forward (and a token server to
// serve it), extra is nil and done does nothing.
func forwardGitHub(ctx context.Context, box *boxSSH, socket string) (extra []string, done func()) {
	if claudeGitHub != githubForward {
		return nil, func() {}
	}
	logPath := ""
	if home, err := os.UserHomeDir(); err == nil {
		logPath = filepath.Join(home, ".onctl", "claude", "github.log")
	}
	srv, err := startTokenServer(box.name, logPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "warning: not forwarding GitHub credentials: %v\n", err)
		return nil, func() {}
	}
	// A socket a dropped connection left behind would stop sshd binding
	// the new one on an image without StreamLocalBindUnlink.
	_ = box.run(ctx, "rm -f "+shellQuote(socket), nil)
	return []string{"-R", socket + ":" + srv.socket}, func() {
		// Without the socket the box's helpers say "attach to forward",
		// rather than failing to reach a stale one.
		_ = box.run(context.Background(), "rm -f "+shellQuote(socket), nil)
		if given, refused := srv.close(); given+refused > 0 {
			fmt.Fprintf(os.Stderr, "%s used your GitHub token %d time(s)", box.name, given)
			if refused > 0 {
				fmt.Fprintf(os.Stderr, ", %d refused", refused)
			}
			fmt.Fprintf(os.Stderr, " (%s).\n", logPath)
		}
	}
}

// claudeTarget is the project here and its box, reachable over ssh.
type claudeTarget struct {
	dir, name string
	box       *boxSSH
	// needsCopy: the project isn't on the box yet.
	needsCopy bool
	// ephemeral: the box is this run's, deleted by end.
	ephemeral bool
}

// openClaudeBox resolves the project and its box (--box, or
// claude-<dir>), makes sure it's running and reachable with onctl's
// key, and checks it holds this project and no other. create says
// whether a missing box is created (onctl claude) or an error (fetch,
// push); setTTL applies --idle-ttl to a box that already exists.
func openClaudeBox(ctx context.Context, create, setTTL bool) (*claudeTarget, error) {
	dir, err := projectDir()
	if err != nil {
		return nil, err
	}
	name := claudeBox
	if name == "" {
		name = claudeBoxName(dir)
	}
	var box *boxSSH
	if onBoxes() {
		box, err = connectClaudeBox(ctx, name, create, setTTL)
	} else {
		if setTTL {
			fmt.Fprintf(os.Stderr, "warning: --idle-ttl is for boxes; %s VMs aren't paused on their own\n", cloudProvider)
		}
		box, _, err = connectClaudeVM(ctx, name, create)
	}
	if err != nil {
		return nil, err
	}
	if err := box.run(ctx, "true", nil); err != nil {
		box.close()
		return nil, fmt.Errorf("can't ssh into %s: %w", name, err)
	}
	needsCopy, boxMux, err := checkProject(ctx, box, dir)
	if err != nil {
		box.close()
		return nil, err
	}
	if err := useBoxMux(ctx, box, dir, boxMux); err != nil {
		box.close()
		return nil, err
	}
	return &claudeTarget{dir: dir, name: name, box: box, needsCopy: needsCopy}, nil
}

// useBoxMux makes the box's multiplexer -- the one its sessions were
// started in, recorded on it by mux.start -- the run's, unless --mux was
// given: then a session running in the other one is refused, rather than
// started again beside it. A box with no record (nothing started yet, or
// from before the record) takes --mux or its default.
func useBoxMux(ctx context.Context, box *boxSSH, dir, name string) error {
	if name == "" || name == mux.name() {
		return nil
	}
	other, err := muxNamed(name)
	if err != nil {
		return nil // a newer onctl's: nothing this one can do with it
	}
	if !muxExplicit {
		mux = other
		return nil
	}
	if box.run(ctx, other.running(newTaskSession(dir, claudeTask)), nil) == nil {
		return fmt.Errorf("Claude is running in %s on %s -- attach with --mux %s, or quit it first", name, box.name, name)
	}
	return nil
}

// connectClaudeBox is openClaudeBox's boxes half: the box resumed or
// created, with its idle TTL set, and onctl's key authorized on it.
func connectClaudeBox(ctx context.Context, name string, create, setTTL bool) (*boxSSH, error) {
	c := newClient()
	created, err := ensureClaudeBox(ctx, c, name, create)
	if err != nil {
		return nil, err
	}
	if created || setTTL {
		if _, err := c.SetIdleTTL(ctx, name, claudeIdle); err != nil {
			fmt.Fprintf(os.Stderr, "warning: couldn't set %s's idle TTL: %v\n", name, err)
		}
	}
	key, err := ensureClaudeKey()
	if err != nil {
		return nil, err
	}
	if err := authorizeKey(ctx, c, name, key); err != nil {
		return nil, err
	}
	return newBoxSSH(name, key)
}

// checkProject reports whether dir still has to be copied to the box,
// refusing a box that holds a different project, and the multiplexer
// the box's sessions run in ("" if none has started). A box from before
// the marker existed is taken to hold whichever project it has at dir.
func checkProject(ctx context.Context, box *boxSSH, dir string) (needsCopy bool, boxMux string, err error) {
	out, err := box.output(ctx, projectCheckScript(dir))
	status, boxMux := parseProjectCheck(string(out))
	switch {
	case err != nil:
		return false, "", fmt.Errorf("checking %s's project: %w", box.name, err)
	case status == "copy":
		return true, boxMux, nil
	case status == "ok":
		return false, boxMux, nil
	case strings.HasPrefix(status, "other "):
		other := strings.TrimPrefix(status, "other ")
		return false, "", fmt.Errorf("%s holds %s, not %s -- run onctl claude from there, or pick another box with --box", box.name, other, dir)
	default:
		return false, "", fmt.Errorf("checking %s's project: unexpected %q", box.name, status)
	}
}

// parseProjectCheck splits projectCheckScript's output: the project's
// status on the first line, the box's multiplexer on the second.
func parseProjectCheck(out string) (status, boxMux string) {
	status, boxMux, _ = strings.Cut(strings.TrimSpace(out), "\n")
	return strings.TrimSpace(status), strings.TrimSpace(boxMux)
}

func projectCheckScript(dir string) string { return projectCheckScriptAt(projectMarker, dir) }

func projectCheckScriptAt(marker, dir string) string {
	d := shellQuote(dir)
	return `m=` + shellQuote(marker) + `; s=copy
if [ -s "$m" ] && [ "$(cat "$m")" != ` + d + ` ]; then s="other $(cat "$m")"
elif [ -e ` + d + ` ]; then mkdir -p "$(dirname "$m")" && printf '%s' ` + d + ` >"$m"; s=ok; fi
printf '%s\n' "$s"; cat ` + shellQuote(muxMarker) + ` 2>/dev/null || true`
}

// markProject records dir as the box's project, and lets onctl claude
// push update the branch checked out there (receive.denyCurrentBranch
// updateInstead: the working tree follows the push, and git refuses it if
// Claude has uncommitted changes, rather than overwriting them).
func markProject(ctx context.Context, box *boxSSH, dir string) error {
	d := shellQuote(dir)
	return box.run(ctx, `mkdir -p "$(dirname `+projectMarker+`)" && printf '%s' `+d+` >`+projectMarker+`
[ ! -d `+d+`/.git ] || git -C `+d+` config receive.denyCurrentBranch updateInstead`, nil)
}

// claudeSetupScript makes sure the box can run Claude in its multiplexer.
// On the claude-agent image only the Claude Code install (and herdr's,
// with --mux herdr) does anything, once; other images (a cloud VM's) get
// tmux, git and curl from apt -- after cloud-init is done with apt, on a
// VM that was just created -- and the multiplexer its own way (mux.setup).
func claudeSetupScript() string {
	return `set -e
export PATH="$HOME/.local/bin:$PATH"
missing=""; for t in tmux git curl; do command -v $t >/dev/null || missing="$missing $t"; done
if [ -n "$missing" ]; then
  ! command -v cloud-init >/dev/null || cloud-init status --wait >/dev/null 2>&1 || true
  echo "Installing$missing..." >&2; apt-get update -qq && DEBIAN_FRONTEND=noninteractive apt-get install -y -qq $missing >/dev/null
fi
` + mux.setup() + `
command -v claude >/dev/null || { echo "Installing Claude Code..." >&2; ` + claudeInstall + ` >&2; }`
}

// projectDir is the git repository's top level, or the directory itself
// outside one, for --project or else the current directory -- the
// project goes to the same absolute path on the box.
func projectDir() (string, error) {
	start := claudeProject
	if start == "" {
		wd, err := os.Getwd()
		if err != nil {
			return "", err
		}
		start = wd
	}
	start, err := filepath.Abs(start)
	if err != nil {
		return "", err
	}
	if st, err := os.Stat(start); err != nil || !st.IsDir() {
		return "", fmt.Errorf("%s isn't a directory", start)
	}
	if out, err := exec.Command("git", "-C", start, "rev-parse", "--show-toplevel").Output(); err == nil {
		if dir := strings.TrimSpace(string(out)); dir != "" {
			return dir, nil
		}
	}
	return start, nil
}

var slugUnsafe = regexp.MustCompile(`[^a-z0-9-]+`)

// claudeBoxName is the project's box, named like the Claude Code plugin's
// project-mode box: claude-<directory name, slugged, at most 20 chars>.
func claudeBoxName(dir string) string {
	slug := slugUnsafe.ReplaceAllString(strings.ToLower(filepath.Base(dir)), "-")
	if len(slug) > 20 {
		slug = slug[:20]
	}
	slug = strings.Trim(slug, "-")
	if slug == "" {
		slug = "project"
	}
	return "claude-" + slug
}

// ensureClaudeBox resumes name if it exists, or creates it when create
// is set, reporting whether it did.
func ensureClaudeBox(ctx context.Context, c *providerboxes.Client, name string, create bool) (created bool, err error) {
	vms, err := c.List(ctx)
	if err != nil {
		return false, err
	}
	for _, vm := range vms {
		if vm.Name == name {
			return false, ensureRunning(ctx, c, name, claudeReadyTimeout)
		}
	}
	if !create {
		return false, fmt.Errorf("no box %s -- start one with onctl claude", name)
	}
	fmt.Fprintf(os.Stderr, "Creating %s %s (%s, %s)...\n", name, providerWhere(), claudeImage, claudeSize)
	if _, err := c.Create(ctx, name, claudeImage, claudeSize); err != nil {
		return false, fmt.Errorf("creating %s: %w", name, err)
	}
	_, err = c.WaitReady(ctx, name, claudeReadyTimeout)
	return true, err
}

// ensureClaudeKey returns the private key onctl uses for boxes it runs
// Claude on, ~/.onctl/claude/id_ed25519, generating it the first time.
func ensureClaudeKey() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	key := filepath.Join(home, ".onctl", "claude", "id_ed25519")
	if _, err := os.Stat(key); err == nil {
		return key, nil
	}
	if err := os.MkdirAll(filepath.Dir(key), 0o700); err != nil {
		return "", err
	}
	out, err := exec.Command("ssh-keygen", "-q", "-t", "ed25519", "-N", "", "-C", "onctl-claude", "-f", key).CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("ssh-keygen: %w: %s", err, out)
	}
	return key, nil
}

// authorizeKey adds key's public half to the box's root authorized_keys
// over the boxes exec endpoint -- the one way in that needs no key.
func authorizeKey(ctx context.Context, c *providerboxes.Client, name, key string) error {
	pub, err := os.ReadFile(key + ".pub")
	if err != nil {
		return err
	}
	line := shellQuote(strings.TrimSpace(string(pub)))
	script := "mkdir -p ~/.ssh && chmod 700 ~/.ssh && { grep -qxF " + line + " ~/.ssh/authorized_keys 2>/dev/null || echo " + line + " >>~/.ssh/authorized_keys; }"
	res, err := c.Exec(ctx, name, script, 30*time.Second)
	if err != nil {
		return fmt.Errorf("authorizing onctl's key on %s: %w", name, err)
	}
	if res.Error != "" || res.ExitCode != 0 {
		return fmt.Errorf("authorizing onctl's key on %s: %s%s", name, res.Error, res.Stderr)
	}
	return nil
}

// boxSSH runs ssh to one box: through `onctl ssh-proxy`, as root, with
// onctl's key, and over one shared connection (ControlMaster) so each
// step after the first costs a round trip, not a new tunnel and
// handshake.
type boxSSH struct {
	name string
	// user, host and port are the ssh destination; host is the box's
	// name (reached through the proxy) or a VM's address.
	user, host string
	port       int
	args       []string // everything before the remote command
}

func newBoxSSH(name, key string) (*boxSSH, error) {
	self, err := os.Executable()
	if err != nil {
		return nil, err
	}
	if _, err := exec.LookPath("ssh"); err != nil {
		return nil, errors.New("onctl claude needs ssh on this machine")
	}
	control := filepath.Join(filepath.Dir(key), "cm-%C")
	return &boxSSH{name: name, user: "root", host: name, port: 22, args: []string{
		// None of the user's ssh config applies -- least of all macOS's
		// SendEnv LC_*, which the box has no locales for.
		"-F", "/dev/null",
		"-i", key, "-o", "IdentitiesOnly=yes",
		// The tunnel is already authenticated end to end (your boxes
		// token, over TLS to the service), and every box has fresh host
		// keys, so there's nothing for known_hosts to add.
		"-o", "StrictHostKeyChecking=no", "-o", "UserKnownHostsFile=/dev/null", "-o", "LogLevel=ERROR",
		"-o", "ProxyCommand=" + shellQuote(self) + " ssh-proxy -p boxes " + shellQuote(name),
		"-o", "ServerAliveInterval=30",
		"-o", "ControlMaster=auto", "-o", "ControlPath=" + control, "-o", "ControlPersist=60s",
		"root@" + name,
	}}, nil
}

// newVMSSH is ssh to a VM at its own address, as user, with the key
// onctl created it with: the same options as a box's, less the proxy.
func newVMSSH(name, user, host string, port int, key string) (*boxSSH, error) {
	if _, err := exec.LookPath("ssh"); err != nil {
		return nil, errors.New("onctl claude needs ssh on this machine")
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, err
	}
	control := filepath.Join(home, ".onctl", "claude", "cm-%C")
	if err := os.MkdirAll(filepath.Dir(control), 0o700); err != nil {
		return nil, err
	}
	return &boxSSH{name: name, user: user, host: host, port: port, args: []string{
		"-F", "/dev/null",
		"-i", key, "-o", "IdentitiesOnly=yes",
		// A VM onctl made has host keys nothing here has seen; a resumed
		// Hetzner VM has new ones at the same address.
		"-o", "StrictHostKeyChecking=no", "-o", "UserKnownHostsFile=/dev/null", "-o", "LogLevel=ERROR",
		"-o", "ServerAliveInterval=30",
		"-o", "ControlMaster=auto", "-o", "ControlPath=" + control, "-o", "ControlPersist=60s",
		"-p", strconv.Itoa(port),
		user + "@" + host,
	}}, nil
}

// remoteURL is dir on the box as a git remote URL. A box's host is
// <name>.box, which the ssh config the README suggests resolves through
// the proxy too; a VM's is its address.
func (b *boxSSH) remoteURL(dir string) string {
	if b.host == b.name {
		return boxRemoteURL(b.name, dir)
	}
	if b.port == 22 {
		return b.user + "@" + b.host + ":" + dir
	}
	return fmt.Sprintf("ssh://%s@%s:%d%s", b.user, b.host, b.port, dir)
}

// run runs a command on the box, with stdin from in (nil for none) and
// its output passed through to stderr.
func (b *boxSSH) run(ctx context.Context, command string, in io.Reader) error {
	cmd := exec.CommandContext(ctx, "ssh", append(append([]string{"-o", "BatchMode=yes"}, b.args...), command)...)
	cmd.Stdin = in
	cmd.Stdout, cmd.Stderr = os.Stderr, os.Stderr
	return cmd.Run()
}

// runWith is run with extra ssh arguments -- a socket forward -- on a
// connection of its own, like attach: the shared control master doesn't
// carry a forward opened after it. Without extra it is run.
func (b *boxSSH) runWith(ctx context.Context, extra []string, command string) error {
	if len(extra) == 0 {
		return b.run(ctx, command, nil)
	}
	cmd := exec.CommandContext(ctx, "ssh", b.runWithArgs(extra, command)...)
	cmd.Stdout, cmd.Stderr = os.Stderr, os.Stderr
	return cmd.Run()
}

func (b *boxSSH) runWithArgs(extra []string, command string) []string {
	argv := append([]string{"-o", "BatchMode=yes", "-o", "ControlPath=none", "-o", "ExitOnForwardFailure=no"}, extra...)
	return append(append(argv, b.args...), command)
}

// output runs a command on the box and returns its stdout; stderr passes
// through.
func (b *boxSSH) output(ctx context.Context, command string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, "ssh", append(append([]string{"-o", "BatchMode=yes"}, b.args...), command)...)
	cmd.Stderr = os.Stderr
	return cmd.Output()
}

// gitSSHCommand is ssh as git should run it to reach the box: everything
// in b.args but the destination, which git adds from the remote's URL.
func (b *boxSSH) gitSSHCommand() string {
	words := []string{"ssh", "-o", "BatchMode=yes"}
	for _, a := range b.args[:len(b.args)-1] {
		words = append(words, shellQuote(a))
	}
	return strings.Join(words, " ")
}

// attach runs command on the box with a terminal, returning when it ends,
// with extra ssh arguments (a socket forward). It gets a connection of its
// own rather than sharing the control master, so a forward lives exactly
// as long as the attach: ssh takes the first value of an option it's
// given, so ControlPath=none here beats the shared one in b.args.
func (b *boxSSH) attach(ctx context.Context, command string, extra []string) error {
	cmd := exec.CommandContext(ctx, "ssh", b.attachArgs(command, extra)...)
	cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
	// The box has terminfo for the common terminals, not for every one
	// (xterm-ghostty, xterm-kitty), and tmux won't start without it.
	cmd.Env = append(os.Environ(), "TERM=xterm-256color")
	err := cmd.Run()
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		// Detaching or quitting Claude ends ssh normally; anything else
		// was already printed by ssh or the box.
		return nil
	}
	return err
}

func (b *boxSSH) attachArgs(command string, extra []string) []string {
	argv := append([]string{"-t", "-o", "ControlPath=none", "-o", "ExitOnForwardFailure=no"}, extra...)
	return append(append(argv, b.args...), command)
}

// close ends the shared ssh connection.
func (b *boxSSH) close() {
	_ = exec.Command("ssh", append(append([]string{"-O", "exit"}, b.args[:len(b.args)-1]...), b.args[len(b.args)-1])...).Run()
}

// shellQuote quotes s for a POSIX shell.
func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

// newClient is the boxes API client: onctl claude runs on boxes, whatever
// provider is otherwise selected (see claudeCmd's PersistentPreRunE).
func newClient() *providerboxes.Client {
	if p, ok := provider.(*cloud.ProviderBoxes); ok {
		return p.Client
	}
	creds, err := providerboxes.LoadCredentials(viper.GetString("boxes.apiURL"))
	if err != nil {
		log.Fatalln(err)
	}
	return providerboxes.New(creds.APIURL, creds.Token)
}

// ensureRunning resumes name if it's paused and waits for it to be
// ready; a running box returns at once.
func ensureRunning(ctx context.Context, c *providerboxes.Client, name string, timeout time.Duration) error {
	vms, err := c.List(ctx)
	if err != nil {
		return err
	}
	for _, vm := range vms {
		if vm.Name != name {
			continue
		}
		switch vm.State {
		case "running":
			if vm.Ready {
				return nil
			}
		case "paused":
			fmt.Fprintf(os.Stderr, "Resuming %s...\n", name)
			if _, err := c.Resume(ctx, name); err != nil {
				return fmt.Errorf("resuming %s: %w", name, err)
			}
		default:
			return fmt.Errorf("%s is %s", name, vm.State)
		}
		_, err := c.WaitReady(ctx, name, timeout)
		return err
	}
	return fmt.Errorf("no box named %s", name)
}
