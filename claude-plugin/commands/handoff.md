---
description: Move this session to a box -- Claude carries on there, with this conversation
argument-hint: "[note for Claude on the box]"
allowed-tools: Bash
---

Hand this Claude Code session over to a box: `onctl claude` copies the
project to the project's box (the first time), copies this conversation
there, and starts Claude on the box resuming it. From then on the work
happens on the box, not here.

Run this command, with a timeout of 600000 ms (creating a box and
installing Claude on it can take a few minutes). If the user gave a note
-- here it is: `$ARGUMENTS` -- append `--prompt` and the note as one
shell-quoted argument; it becomes the first message on the box. Leave
everything else exactly as written: the command must start with
`onctl`, so it runs here even while remote bash is on.

```
onctl claude --project "${CLAUDE_PROJECT_DIR}" --handoff "${CLAUDE_SESSION_ID}" --detach
```

Then tell the user, briefly:

- To continue, run `onctl claude` in a terminal in this project (it
  attaches to Claude on the box; the first time, log in there with
  `/login`). Its output names the box.
- This session should stop here. Edits made locally from now on don't
  reach the box -- the box has its own copy. Results come back as
  commits: Claude pushes from the box, or `onctl claude fetch` brings
  the box's branches and uncommitted work back as `box/*`.

If it fails with "unknown command" or "unknown flag: --handoff", onctl is
too old: tell the user to update it (`go install github.com/cdalar/onctl@main`).
If it fails because onctl isn't logged in to boxes, tell them to run
`! onctl login`.
Do not retry with other flags.
