---
name: pr
description: Create a branch, commit staged/unstaged changes, push to origin, open a pull request, then monitor GitHub Actions and fix any failures. Use when asked to "create a PR", "push and open PR", or "branch + PR".
allowed-tools: Read, Glob, Grep, Bash, Edit, Write
---

# Create Branch, PR, and Monitor CI

## Current State

- Git status: !`git status --short`
- Current branch: !`git branch --show-current`
- Recent commits: !`git log --oneline -5`
- Diff to commit: !`git diff HEAD`

## Workflow

### Phase 1: Branch

1. If already on `main` or `master`, create a new branch:
   - Derive branch name from the work done (e.g. `feat/...`, `fix/...`, `test/...`, `chore/...`)
   - `git checkout -b <branch-name>`
2. If already on a feature branch, continue using it.

### Phase 2: Commit

1. Stage only relevant files — never `coverage.out`, `.env`, secrets, or large binaries
2. Write a conventional commit message: `<type>: <short description>`
   - Types: `feat`, `fix`, `test`, `chore`, `refactor`, `docs`, `build`
   - Add body lines if multiple logical changes
   - Do NOT add any Co-Authored-By trailer
3. Commit with a HEREDOC to preserve formatting

### Phase 3: Push & Open PR

1. `git push -u origin <branch-name>`
2. Create PR with `gh pr create`:
   - Title: same as commit subject (under 70 chars)
   - Body: Summary bullets + Test plan checklist
   - Footer: `🤖 Generated with [Claude Code](https://claude.com/claude-code)`

### Phase 4: Monitor GitHub Actions

1. List running workflows: `gh run list --limit 10`
2. Wait for completion (poll with `gh run list` or `gh run watch <run-id>`)
3. For each failed run:
   - Get logs: `gh run view <run-id> --log-failed`
   - Read the failure, identify root cause
   - Fix the issue (edit source files, add/fix tests, fix lint errors)
   - Commit the fix and push
   - Re-check until all checks pass

### Phase 5: Report

Return to the user with:
- PR URL
- Final CI status (pass/fail per workflow)
- Summary of any fixes applied

## Rules

- NEVER use `--no-verify` or skip hooks
- NEVER force-push unless explicitly asked
- NEVER commit `.env`, `coverage.out`, secrets, or unrelated files
- ALWAYS fix CI failures before reporting success
- If a fix is unclear, describe the error and ask the user

## Arguments

`ARGUMENTS` can be:
- A branch name to use (e.g. `fix/my-bug`)
- A PR title override
- Empty — derive everything from context
