---
description: Run Bash commands in a box (an onctl boxes microVM) (default: one box for this session)
argument-hint: "[--mode session|project|exec] [--workspace sync|copy|git|none] [box-name] [--size S] [--image I]"
allowed-tools: Bash
---

Route Bash commands to a box. Modes:

- `session` (default): one box for this Claude Code session, destroyed when the session ends.
- `project`: one box for this project directory, kept across sessions.
- `exec`: a fresh, disposable box for every command; nothing persists between commands.

Workspaces (how the project gets onto a session or project box):

- `sync` (default): copied there before every command and back after.
- `copy`: copied once, now.
- `git`: a git clone of its origin, at the local commit.
- `none`: an empty directory.

Run exactly this, with a timeout of 600000 ms (creating a box can take a few minutes):

```
CLAUDE_SESSION_ID="${CLAUDE_SESSION_ID}" CLAUDE_PROJECT_DIR="${CLAUDE_PROJECT_DIR}" "${CLAUDE_PLUGIN_ROOT}/bin/onctl-claude" on --workspace '${user_config.workspace}' --size '${user_config.size}' --image '${user_config.image}' $ARGUMENTS
```

Report the result in one or two sentences. If it fails because onctl isn't
logged in to boxes, tell the user to run `! onctl login`. If `onctl` isn't
found, tell them to install it (`go install github.com/cdalar/onctl@main`).
