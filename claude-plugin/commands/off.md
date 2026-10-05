---
description: Run Bash commands locally again (destroys a session box, keeps a project box)
allowed-tools: Bash
---

Run exactly this and report the result in one sentence:

```
CLAUDE_SESSION_ID="${CLAUDE_SESSION_ID}" CLAUDE_PROJECT_DIR="${CLAUDE_PROJECT_DIR}" "${CLAUDE_PLUGIN_ROOT}/bin/onctl-claude" off
```
