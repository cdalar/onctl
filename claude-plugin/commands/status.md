---
description: Show whether Bash commands run locally or on a box
allowed-tools: Bash
---

Run exactly this and report the result in one sentence:

```
CLAUDE_SESSION_ID="${CLAUDE_SESSION_ID}" CLAUDE_PROJECT_DIR="${CLAUDE_PROJECT_DIR}" "${CLAUDE_PLUGIN_ROOT}/bin/onctl-claude" status
```
