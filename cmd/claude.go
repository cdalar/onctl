package cmd

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
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
	// claudeSession is the tmux session Claude runs in on the box: one
	// per box, so a second `onctl claude` attaches to it.
	claudeSession = "claude"
	// claudeInstall is Anthropic's installer, run on first use rather than
	// baking Claude Code into the image (which would redistribute it). It
	// installs to ~/.local/bin and keeps itself updated.
	claudeInstall = "curl -fsSL https://claude.ai/install.sh | bash"
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
)

var claudeCmd = &cobra.Command{
	Use:   "claude [-- claude-args...]",
	Short: "Run Claude Code on a box, with this project on it",
	Long: `Runs Claude Code on a box instead of on this machine, attached to your
terminal. The first time, it creates the project's box (claude-<project>),
copies the project there -- the same absolute path, .git and uncommitted
changes included, .gitignore'd files left out -- installs Claude Code and
your Claude configuration, and starts Claude in it.

Claude runs in tmux, so it keeps going when you detach (Ctrl-b d) or the
connection drops; run onctl claude again to come back to it. The box's
copy of the project is the one Claude works on: later runs don't copy it
again, and nothing is synced back -- Claude commits and pushes from the
box.

  onctl claude                           # create or reattach
  onctl claude --prompt "fix the flaky auth test"
  onctl claude --detach --prompt "..."   # start it, don't attach
  onctl claude -- --model opus           # arguments for claude itself

The first time, log Claude in on the box with /login: it prints a URL to
open here and a code to paste back. That login stays on the box.

GitHub: while you're attached, the box borrows your gh token for git
push and gh pr ... -- it asks this machine per use, over the ssh
connection, and keeps nothing (--github forward, the default; each use is
logged to ~/.onctl/claude/github.log). Detached, it has none. For work
that must reach GitHub while you're away, --github store keeps a token
you paste on the box -- make it a fine-grained one for this repository.
--github off gives it nothing. Your git user.name and user.email are set
on the box either way.

To keep Claude Code on this machine and send only its Bash commands to a
box instead, there's a plugin:

  claude plugin marketplace add cdalar/onctl
  claude plugin install onctl@onctl`,
	RunE: runClaude,
}

func init() {
	claudeCmd.PersistentFlags().StringVar(&claudeBox, "box", "", "box to use (default: claude-<project directory name>)")
	claudeCmd.PersistentFlags().StringVar(&claudeProject, "project", "", "project directory (default: the git repository, or directory, you're in)")
	claudeCmd.Flags().StringVar(&claudeHandoff, "handoff", "", "continue this local Claude Code session (its ID) on the box")
	claudeCmd.PersistentFlags().StringVarP(&claudeTask, "task", "t", "", "work on a task in parallel: its own worktree (<dir>@<task>, branch <task>) and Claude on the box")
	claudeCmd.PersistentFlags().BoolVar(&claudeOwnBox, "own-box", false, "with --task: give the task a box of its own (claude-<project>-<task>) instead of a worktree")
	claudeCmd.Flags().StringVar(&claudeAuth, "claude-auth", claudeAuthAuto, "how Claude on the box logs in: auto (the token onctl claude login saved, else /login), token, login or api-key")
	_ = claudeCmd.RegisterFlagCompletionFunc("claude-auth", cobra.FixedCompletions([]string{claudeAuthAuto, claudeAuthToken, claudeAuthLogin, claudeAuthAPIKey}, cobra.ShellCompDirectiveNoFileComp))
	claudeCmd.Flags().StringVarP(&claudeSize, "size", "s", claudeDefaultSize, "size of a box created for this")
	_ = claudeCmd.RegisterFlagCompletionFunc("size", completeSizes)
	claudeCmd.Flags().StringVarP(&claudeImage, "image", "i", claudeDefaultImage, "image of a box created for this")
	// No -p shorthand: in onctl that is the global --provider.
	claudeCmd.Flags().StringVar(&claudePrompt, "prompt", "", "first message for a newly started Claude")
	claudeCmd.Flags().BoolVarP(&claudeDetach, "detach", "d", false, "start Claude on the box without attaching to it")
	claudeCmd.Flags().StringVar(&claudeGitHub, "github", githubForward, "GitHub credentials for the box: forward (lent while attached), store (a token kept on the box) or off")
	_ = claudeCmd.RegisterFlagCompletionFunc("github", cobra.FixedCompletions([]string{githubForward, githubStore, githubOff}, cobra.ShellCompDirectiveNoFileComp))
	claudeCmd.Flags().DurationVar(&claudeIdle, "idle-ttl", claudeDefaultIdleTTL, "pause the box after this long unused, 10m to 720h (set on a new box, or on an existing one when given)")
	rootCmd.AddCommand(claudeCmd)
}

func runClaude(cmd *cobra.Command, args []string) error {
	ctx := cmd.Context()
	switch claudeGitHub {
	case githubForward, githubStore, githubOff:
	default:
		return fmt.Errorf("--github must be %s, %s or %s", githubForward, githubStore, githubOff)
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

	t, err := openClaudeBox(ctx, true, cmd.Flags().Changed("idle-ttl"))
	if err != nil {
		return err
	}
	defer t.box.close()
	dir, name, box := t.dir, t.name, t.box

	if t.needsCopy {
		if err := copyProject(ctx, box, dir); err != nil {
			return err
		}
		if err := markProject(ctx, box, dir); err != nil {
			return err
		}
	} else if claudeTask == "" {
		fmt.Fprintf(os.Stderr, "%s is already on %s; working with the box's copy (it isn't copied again).\n", dir, name)
		if claudeHandoff != "" {
			fmt.Fprintf(os.Stderr, "Your local changes since then aren't on the box: commit them and run onctl claude push, or hand off to a fresh box with --box.\n")
		}
	}
	sess := newTaskSession(dir, claudeTask)
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
	if claudeGitHub == githubStore {
		if err := storeGitHubToken(ctx, box, githubRepo(dir)); err != nil {
			return err
		}
	}
	if err := box.run(ctx, claudeSetupScript, nil); err != nil {
		return fmt.Errorf("installing Claude Code on %s: %w", name, err)
	}
	if err := box.run(ctx, claudeStateScript([]string{sess.dir}, localClaudeTheme(), apiKey), nil); err != nil {
		fmt.Fprintf(os.Stderr, "warning: couldn't skip Claude's first-run setup on %s: %v\n", name, err)
	}

	// =name: exact. A bare -t name falls back to a prefix match, so "claude"
	// would find "claude-auth" and take the main session for running.
	running := box.run(ctx, "tmux has-session -t ="+sess.tmux+" 2>/dev/null", nil) == nil
	if claudeHandoff != "" {
		if running {
			return fmt.Errorf("a Claude session is already running there -- attach with %s and quit it, then hand off again", sess.attachHint(name))
		}
		if err := copyTranscript(ctx, box, sess.dir, transcript, claudeHandoff); err != nil {
			return err
		}
		args = append([]string{"--resume", claudeHandoff}, args...)
	}
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
			if err := box.run(ctx, sess.tmuxCommand(line, true), nil); err != nil {
				return fmt.Errorf("starting Claude on %s: %w", name, err)
			}
		}
		fmt.Fprintf(os.Stderr, "Claude is running on %s; attach with: %s\n", name, sess.attachHint(name))
		return nil
	}
	return attachClaude(ctx, box, sess, sess.tmuxCommand(line, false))
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
	if claudeGitHub != githubForward {
		return box.attach(ctx, command, nil)
	}
	logPath := ""
	if home, err := os.UserHomeDir(); err == nil {
		logPath = filepath.Join(home, ".onctl", "claude", "github.log")
	}
	srv, err := startTokenServer(box.name, logPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "warning: not forwarding GitHub credentials: %v\n", err)
		return box.attach(ctx, command, nil)
	}
	// A socket a dropped connection left behind would stop sshd binding
	// the new one on an image without StreamLocalBindUnlink.
	_ = box.run(ctx, "rm -f "+shellQuote(sess.ghSocket), nil)
	err = box.attach(ctx, command, []string{"-R", sess.ghSocket + ":" + srv.socket})
	// Without the socket the box's helpers say "attach to forward", rather
	// than failing to reach a stale one.
	_ = box.run(context.Background(), "rm -f "+shellQuote(sess.ghSocket), nil)
	if given, refused := srv.close(); given+refused > 0 {
		fmt.Fprintf(os.Stderr, "%s used your GitHub token %d time(s)", box.name, given)
		if refused > 0 {
			fmt.Fprintf(os.Stderr, ", %d refused", refused)
		}
		fmt.Fprintf(os.Stderr, " (%s).\n", logPath)
	}
	return err
}

// claudeTarget is the project here and its box, reachable over ssh.
type claudeTarget struct {
	dir, name string
	box       *boxSSH
	// needsCopy: the project isn't on the box yet.
	needsCopy bool
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
	box, err := newBoxSSH(name, key)
	if err != nil {
		return nil, err
	}
	if err := box.run(ctx, "true", nil); err != nil {
		box.close()
		return nil, fmt.Errorf("can't ssh into %s: %w", name, err)
	}
	needsCopy, err := checkProject(ctx, box, dir)
	if err != nil {
		box.close()
		return nil, err
	}
	return &claudeTarget{dir: dir, name: name, box: box, needsCopy: needsCopy}, nil
}

// checkProject reports whether dir still has to be copied to the box,
// refusing a box that holds a different project. A box from before the
// marker existed is taken to hold whichever project it has at dir.
func checkProject(ctx context.Context, box *boxSSH, dir string) (needsCopy bool, err error) {
	out, err := box.output(ctx, projectCheckScript(dir))
	switch status := strings.TrimSpace(string(out)); {
	case err != nil:
		return false, fmt.Errorf("checking %s's project: %w", box.name, err)
	case status == "copy":
		return true, nil
	case status == "ok":
		return false, nil
	case strings.HasPrefix(status, "other "):
		other := strings.TrimPrefix(status, "other ")
		return false, fmt.Errorf("%s holds %s, not %s -- run onctl claude from there, or pick another box with --box", box.name, other, dir)
	default:
		return false, fmt.Errorf("checking %s's project: unexpected %q", box.name, status)
	}
}

func projectCheckScript(dir string) string { return projectCheckScriptAt(projectMarker, dir) }

func projectCheckScriptAt(marker, dir string) string {
	d := shellQuote(dir)
	return `m=` + shellQuote(marker) + `
if [ -s "$m" ] && [ "$(cat "$m")" != ` + d + ` ]; then printf 'other %s' "$(cat "$m")"; exit 0; fi
if [ -e ` + d + ` ]; then mkdir -p "$(dirname "$m")" && printf '%s' ` + d + ` >"$m"; echo ok; exit 0; fi
echo copy`
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

// claudeSetupScript makes sure the box can run Claude in tmux. On the
// claude-agent image only the Claude Code install does anything, once;
// other images get tmux from apt.
const claudeSetupScript = `set -e
export PATH="$HOME/.local/bin:$PATH"
command -v tmux >/dev/null || { echo "Installing tmux..." >&2; apt-get update -qq && DEBIAN_FRONTEND=noninteractive apt-get install -y -qq tmux >/dev/null; }
command -v claude >/dev/null || { echo "Installing Claude Code..." >&2; ` + claudeInstall + ` >&2; }`

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
	fmt.Fprintf(os.Stderr, "Creating %s (%s, %s)...\n", name, claudeImage, claudeSize)
	if _, err := c.Create(ctx, name, claudeImage, claudeSize); err != nil {
		return false, fmt.Errorf("creating %s: %w", name, err)
	}
	_, err = c.WaitReady(ctx, name, claudeReadyTimeout)
	return true, err
}

// ensureClaudeKey returns the private key onctl uses for boxes it runs
// Claude on -- the Claude Code plugin's key, so either tool can reach a
// box the other set up -- generating it the first time.
func ensureClaudeKey() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	key := filepath.Join(home, ".onctl", "claude", "id_ed25519")
	if _, err := os.Stat(key); err == nil {
		return key, nil
	}
	// onctl's key, if it made one: already authorized on its boxes, and
	// the one the Claude Code plugin uses.
	if legacy := filepath.Join(home, ".boxctl", "claude", "id_ed25519"); fileExists(legacy) {
		return legacy, nil
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
	args []string // everything before the remote command
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
	return &boxSSH{name: name, args: []string{
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

// run runs a command on the box, with stdin from in (nil for none) and
// its output passed through to stderr.
func (b *boxSSH) run(ctx context.Context, command string, in io.Reader) error {
	cmd := exec.CommandContext(ctx, "ssh", append(append([]string{"-o", "BatchMode=yes"}, b.args...), command)...)
	cmd.Stdin = in
	cmd.Stdout, cmd.Stderr = os.Stderr, os.Stderr
	return cmd.Run()
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

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}
