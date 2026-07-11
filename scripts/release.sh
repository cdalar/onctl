#!/usr/bin/env bash
# Bumps the patch version of the latest git tag and pushes it,
# which triggers the goreleaser pipeline (.github/workflows/goreleaser.yml).
# Also makes sure the local self-hosted Actions runner is running,
# since the release job runs on [self-hosted, macOS, ARM64].
set -euo pipefail

RUNNER_DIR="${RUNNER_DIR:-$HOME/cdalar/actions-runner}"
RELEASE_BRANCH="${RELEASE_BRANCH:-main}"

cd "$(git rev-parse --show-toplevel)"

current_branch=$(git rev-parse --abbrev-ref HEAD)
if [[ "$current_branch" != "$RELEASE_BRANCH" ]]; then
  echo "Refusing to release: on branch '$current_branch', expected '$RELEASE_BRANCH'" >&2
  exit 1
fi

if [[ -n "$(git status --porcelain)" ]]; then
  echo "Refusing to release: working tree is not clean" >&2
  exit 1
fi

git fetch origin "$RELEASE_BRANCH" --tags --quiet

local_head=$(git rev-parse HEAD)
remote_head=$(git rev-parse "origin/$RELEASE_BRANCH")
if [[ "$local_head" != "$remote_head" ]]; then
  echo "Refusing to release: local '$RELEASE_BRANCH' ($local_head) does not match origin/$RELEASE_BRANCH ($remote_head)" >&2
  exit 1
fi

latest_tag=$(git tag --list 'v*' --sort=-v:refname | grep -E '^v[0-9]+\.[0-9]+\.[0-9]+$' | head -1)
if [[ -z "$latest_tag" ]]; then
  echo "No existing stable vX.Y.Z tag found" >&2
  exit 1
fi

if [[ ! "$latest_tag" =~ ^v([0-9]+)\.([0-9]+)\.([0-9]+)$ ]]; then
  echo "Latest tag '$latest_tag' doesn't match vMAJOR.MINOR.PATCH" >&2
  exit 1
fi

major="${BASH_REMATCH[1]}"
minor="${BASH_REMATCH[2]}"
patch="${BASH_REMATCH[3]}"
new_tag="v${major}.${minor}.$((patch + 1))"

echo "Latest tag: $latest_tag"
echo "New tag:    $new_tag"

# Make sure the self-hosted runner is up (release job needs it).
if pgrep -f "$RUNNER_DIR/bin/Runner.Listener" >/dev/null 2>&1; then
  echo "Self-hosted runner already running."
else
  echo "Self-hosted runner not running, starting it..."
  nohup "$RUNNER_DIR/run.sh" >"$RUNNER_DIR/run.log" 2>&1 &
  disown
  sleep 2
  if pgrep -f "$RUNNER_DIR/bin/Runner.Listener" >/dev/null 2>&1; then
    echo "Runner started."
  else
    echo "Failed to start runner, check $RUNNER_DIR/run.log" >&2
    exit 1
  fi
fi

git tag -a "$new_tag" -m "Release $new_tag"
git push origin "$new_tag"

echo "Pushed $new_tag — goreleaser pipeline triggered."
