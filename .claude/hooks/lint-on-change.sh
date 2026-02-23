#!/bin/bash
# PostToolUse hook: run golangci-lint on any modified .go file

INPUT=$(cat)
FILE=$(echo "$INPUT" | jq -r '.tool_input.file_path // empty')

# Skip non-Go files
[[ "$FILE" == *.go ]] || exit 0

# Derive package path relative to project root
# When dirname equals CLAUDE_PROJECT_DIR (e.g. main.go), use "." directly
_PKG_ABS=$(dirname "$FILE")
if [ "$_PKG_ABS" = "$CLAUDE_PROJECT_DIR" ]; then
  PKG_DIR="."
else
  PKG_DIR="${_PKG_ABS#${CLAUDE_PROJECT_DIR}/}"
fi

cd "$CLAUDE_PROJECT_DIR" || exit 0

LINT_OUTPUT=$(golangci-lint run --timeout 60s "./${PKG_DIR}/..." 2>&1)
EXIT_CODE=$?

if [ $EXIT_CODE -ne 0 ] && [ -n "$LINT_OUTPUT" ]; then
  REASON=$(printf "golangci-lint found issues:\n\n%s" "$LINT_OUTPUT" | jq -Rs .)
  printf '{"decision":"block","reason":%s}' "$REASON"
fi
