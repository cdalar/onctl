---
name: onctl-remote-bash
description: How Bash behaves while the onctl plugin routes commands to a microVM. Use when a Bash command's text was rewritten to `onctl-claude run ...`, when a command's output doesn't match local files, or before working with files while remote bash is on.
---

While remote bash is on (`/onctl:on`), every Bash command runs as root in a
Firecracker microVM ("box") on onctl boxes (boxctl.io), not on the user's machine. A hook
rewrites the command to `onctl-claude run ...` or `onctl-claude exec ...`;
that is expected, not an error. `/onctl:status` says which mode is on:

- `session`: one box for this session, destroyed when the session ends.
- `project`: one box for this project, kept across sessions.
- `exec`: every command gets a brand-new box, so **nothing** carries over
  between commands -- not files, not the working directory. Put dependent
  steps in one command (`git clone ... && cd repo && make`).

What's different:

- **Files depend on the workspace** (`/onctl:status` doesn't show it; the
  `/onctl:on` output and session-start note do). The project lives on the
  box at the same absolute path as here.
  - `sync` (default): the project is copied to the box before every Bash
    command and back after it, so Read/Edit/Write here and Bash there see
    the same files. A file deleted by a command on the box is deleted here
    too, just as if it ran locally.
  - `copy`: copied once when the box was turned on; later edits made with
    Read/Edit/Write here are **not** on the box.
  - `git`: a clone of the origin at the local commit; uncommitted local
    changes are **not** there.
  - `none`: the box starts with an empty directory.
  - `.gitignore`'d files (node_modules, build output, .env) are never
    copied either way: install dependencies and build on the box itself.
- **The working directory carries over** between commands on a session or
  project box (kept on the box),
  but environment variables and shell functions do not: each command is a
  fresh non-interactive shell.
- **No stdin, no TTY**, and a hard **5-minute limit** per command (sync
  time not included). Split long
  builds into steps, or start them with `nohup ... &` and poll a log file.
- `run_in_background` still works -- it backgrounds the local wrapper.
- A paused session or project box is resumed automatically on the next command.

To run one command locally anyway, start it with a `# local` comment line:

```
# local
git push origin HEAD
```

Commands starting with `onctl ` always run locally too, since they drive
the box from here. Use `/onctl:off` to go back to local bash entirely.
