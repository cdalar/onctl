---
sidebar_position: 4
---
# Claude Code on a box

`onctl claude` runs [Claude Code](https://claude.com/claude-code) on a box
instead of on your laptop, with your project on it, attached to your terminal.
Claude's file tools and its shell both live on the box, so it can't touch
anything on your machine, and it keeps working after you close the lid.

```bash
cd my-project
onctl claude
```

That creates a box of its own for the run, puts the project there, installs
Claude Code and your Claude configuration, and starts Claude in it. When
Claude exits, whatever it did comes back as git branches (`box/*`, see
[Getting the work back](#getting-the-work-back)) and the box is deleted.

## Where it runs

The box is a hosted one by default (`-p boxes`: `onctl login` once with a token
from [boxctl.io](https://boxctl.io), no cloud account, no host to run). It can
also be a VM of any other provider, or an Apple container on a Mac:

```bash
onctl claude               # a hosted box
onctl -p hetzner claude    # your own Hetzner VM (or aws, gcp, azure, ovh, fc, ch)
onctl -p ac claude         # an Apple container, from the claude-agent image
```

On a cloud VM, `--size` and `--image` are the provider's type and image (unset,
the `onctl.yaml` defaults), and onctl installs tmux, git and Claude Code on
first use. The `ac` provider boots `ghcr.io/boxctl-io/claude-agent`, which has
them baked in, so nothing is installed. A hosted box is paused after 24 hours
unused (`--idle-ttl`); a VM is paused and resumed with `onctl pause`/`resume`.

## What goes to the box

- A git repository with an `origin` is **cloned** from it: the default branch,
  then a new branch `onctl/<slug>-<id>` for Claude to work on. Nothing local,
  uncommitted or unpushed, goes along.
- Anything else is **copied**, with `.gitignore`'d files left out.

Either way the project lands at the same absolute path it has here. With
`--repo owner/name` there needs to be no local copy at all: a box named
`claude-<name>` gets the repository cloned from GitHub onto a new `onctl/...`
branch, and Claude starts in it.

## Logging in

Claude on the box needs to be logged in to Claude, separately from your
machine. The first time, with nothing saved, Claude on the box asks you to
`/login`: it prints a URL to open here and a code to paste back. That login
stays on that box, and the next box asks again.

To skip that on every new box, save a long-lived token once:

```bash
onctl claude login
```

It runs `claude setup-token`, which needs a Pro, Max, Team or Enterprise plan
and opens the browser for approval, and asks you to paste the token it prints
(input hidden). A token can also be piped in, made with `claude setup-token`
on any machine with Claude Code:

```bash
echo "$TOKEN" | onctl claude login
```

The token is kept in the macOS Keychain (elsewhere in
`~/.onctl/claude/oauth-token`, mode 0600). Each box's Claude gets it in its
environment when it starts, never on the box's disk. `onctl claude logout`
forgets it.

:::note
The token can only make model requests: Remote Control and claude.ai
connectors don't work with it. For a box that needs them, use
`--claude-auth login` and log in on the box.
:::

`--claude-auth` picks the method for a run:

| | |
|---|---|
| `auto` | the saved token if there is one, else `/login` on the box (default) |
| `token` | the saved token; fails if none is saved |
| `login` | `/login` on the box, even with a token saved |
| `api-key` | `ANTHROPIC_API_KEY` from this machine's environment, passed to the box |

## GitHub access

While you're attached, the box **borrows your `gh` token** for `git push` and
`gh pr ...`: it asks this machine per use over the ssh connection and keeps
nothing (each use is logged to `~/.onctl/claude/github.log`). Detached, it has
none.

- `--github store` keeps a token you paste on the box, for work that must
  reach GitHub while you're away. Make it a fine-grained one for this
  repository. A private `--repo` clone needs this too, since the clone runs
  before you're attached.
- `--github off` gives it nothing.

Your git `user.name` and `user.email` are set on the box either way.

## Detaching and coming back

Claude runs in tmux, so it keeps going when you detach (`Ctrl-b d`) or the
connection drops. The message on detaching says how to come back:

```bash
onctl claude --box <name>
```

`--mux herdr` runs Claude in [herdr](https://herdr.dev) instead, a terminal
workspace manager for coding agents that shows every task on the box in one
window, each marked working, blocked or idle (`Ctrl-b q` detaches).
`ONCTL_MUX=herdr` makes it the default.

## Keeping a box

A run's box is deleted when Claude exits. To keep one around:

```bash
onctl claude --keep                      # the project's own box, claude-<project>
onctl claude --detach --prompt "..."     # start Claude, don't attach; the box stays
```

With `--keep`, the first run creates `claude-<project>` and puts the project
there (a clone on branch `onctl/<project>`, or a copy). Later runs come back to
it and don't copy again. Nothing is synced back: Claude commits and pushes from
the box. `--box`, `--detach`, `--task` and `--repo` keep their box too.

## Several tasks at once

```bash
onctl claude --task auth                 # a worktree <project>@auth on branch auth
onctl claude --task docs --own-box       # a box of its own, claude-<project>-docs
onctl claude ls                          # what's running where
```

Each `--task` gets a git worktree of its own beside the project on the box,
with its own Claude session and its own GitHub forwarding, so several Claudes
can work on one project without touching each other's files. `onctl claude ls`
lists them with their branch, whether Claude is running and when it last showed
activity.

## Getting the work back

```bash
onctl claude fetch                       # every branch on the box, as box/<branch>
git log box/fix-auth
git diff main box/fix-auth
git show --stat box/wip                  # what Claude is in the middle of
git checkout -b fix-auth box/fix-auth

onctl claude push                        # the current branch, to the box
onctl claude push main:main
```

`fetch` brings every branch of the project on its box into this repository as
`box/<branch>`, plus `box/wip`: a commit of the box's uncommitted work on top of
whatever it has checked out. Nothing on the box changes. It runs on its own when
a run's box is deleted.

`push` sends local commits to the box, so Claude has a fix or a rebase onto the
latest `main`. Pushing to the branch the box has checked out updates its files
too, as long as Claude has no uncommitted changes there.

## Flags

```
onctl claude [flags] [-- claude-args...]

  -k, --keep               use the project's own box, claude-<project>, and keep it
      --box string         box to use, kept after the run
      --project string     project directory (default: the git repository, or directory, you're in)
  -R, --repo string        GitHub repository (owner/name) to clone on a box of its own
  -t, --task string        work on a task in parallel: its own worktree and Claude session
      --own-box            with --task: a box of its own instead of a worktree
      --prompt string      first message for a newly started Claude
  -d, --detach             start Claude on the box without attaching to it
      --handoff string     continue this local Claude Code session (its ID) on the box
      --claude-auth string how Claude on the box logs in: auto, token, login or api-key
      --github string      GitHub credentials for the box: forward, store or off (default forward)
  -s, --size string        size of a box created for this (default medium)
  -i, --image string       image of a box created for this (default claude-agent)
      --idle-ttl duration  pause the box after this long unused, 10m to 720h (default 24h)
      --mux string         tmux or herdr (default tmux, or $ONCTL_MUX)
```

Everything after `--` goes to `claude` itself:

```bash
onctl claude -- --model opus
```

## Subcommands

| | |
|---|---|
| `onctl claude login` | save a Claude token here, so Claude on every box is logged in ([Logging in](#logging-in)) |
| `onctl claude logout` | forget the saved token |
| `onctl claude ls` | list this project's Claude sessions: the main one and its tasks ([Several tasks at once](#several-tasks-at-once)) |
| `onctl claude fetch` | fetch the box's branches and uncommitted work as `box/*` ([Getting the work back](#getting-the-work-back)) |
| `onctl claude push [refspec...]` | push local commits to the project on its box |

:::tip From the TUI
In `onctl tui`, `a` runs Claude Code on the VM under the cursor, and `:repos`
lists your GitHub repositories so `enter` starts Claude on a box with one
cloned (`onctl claude --repo`), asking for a new box or the repository's own.
:::
