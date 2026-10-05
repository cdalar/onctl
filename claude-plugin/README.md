# The onctl plugin for Claude Code

Runs Claude Code's Bash commands on a box -- an onctl boxes microVM --
instead of on your machine.

```
/plugin marketplace add cdalar/onctl
/plugin install onctl@onctl
```

It needs `onctl` (logged in to boxes: `onctl login`) and `jq` on your
`PATH`. Then:

```
/onctl:on [--mode session|project|exec] [--workspace sync|copy|git|none] [box-name] [--size medium] [--image name]
/onctl:status
/onctl:off
/onctl:handoff [note]
```

- **Modes:** `session` (default) -- one box for this session, destroyed
  when it ends; `project` -- one box for the project, kept across
  sessions; `exec` -- a fresh box for every command.
- **Workspaces:** `sync` (default) copies the project to the box before
  every command and back after; `copy` once; `git` clones its origin;
  `none` starts empty. `.gitignore`'d files never move.
- A command starting with `onctl ` or a `# local` line runs here instead.
- `/onctl:handoff` moves the whole session to a box with `onctl claude
  --handoff`: Claude carries on there, with this conversation.

The plugin's settings (`/plugin` → onctl → configure) can turn it on by
itself at every session start, and set the workspace, size and image.

Coming from boxctl's plugin: it reuses boxctl's ssh key
(`~/.boxctl/claude/id_ed25519`), so boxes either one set up work with
both; uninstall `boxctl@boxctl` so only one routes Bash.
