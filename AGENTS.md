## Brief overview
Guidelines for developing the onctl CLI tool, a Go-based utility for managing cloud resources across multiple providers (AWS, Azure, GCP, Hetzner). These rules focus on Go best practices, CLI design patterns, and cloud integration approaches observed in the project.

## Go coding conventions
- Use `gofmt` and `goimports` for consistent code formatting and import organization
- Follow standard Go naming conventions (PascalCase for exported, camelCase for unexported)
- Use meaningful variable and function names that reflect their purpose in cloud management context
- Prefer early returns and avoid nested if statements for cleaner control flow

## Error handling
- Return errors from functions rather than panicking, allowing callers to handle appropriately
- Use `fmt.Errorf` with `%w` verb for error wrapping to preserve error chains
- Check for context cancellation in long-running operations like cloud API calls
- Log errors at appropriate levels (debug, info, warn, error) using structured logging

## Testing practices
- Write table-driven tests for functions with multiple input scenarios
- Use test helpers and fixtures for common setup/teardown operations
- Include integration tests for end-to-end CLI command validation
- Aim for good test coverage, especially for cloud provider implementations

## CLI design patterns
- Use cobra framework for command structure with consistent flag naming
- Provide clear, concise help text for all commands and flags
- Support both interactive and non-interactive modes where appropriate
- Use consistent exit codes (0 for success, non-zero for errors)

## Cloud provider abstraction
- Implement cloud providers using interfaces for testability and extensibility
- Use dependency injection to pass provider implementations to commands
- Handle provider-specific quirks and API differences through adapter patterns
- Include retry logic with exponential backoff for transient cloud API failures

## Code organization
- Keep business logic in `internal/` packages with clear separation of concerns
- Use embedded files for templates and static assets to simplify distribution
- Maintain clear boundaries between CLI commands, domain logic, and infrastructure code
- Document public APIs and complex algorithms with comments

## Build and language guidelines
- Use `make` for building the project instead of direct `go build` commands
- Write all code comments, messages, and logs in English

## Multi-session workspace management

### Overview
This project supports multiple concurrent OpenCode sessions using git worktrees. Each session operates in an isolated workspace, preventing conflicts and enabling parallel development.

**Configuration:**
- Main repository: ~/workspace/onctl (main branch)
- Worktree location: ~/workspace/onctl-<descriptive-name>
- Maximum concurrent sessions: 10
- Branch naming: ai/<descriptive-name>

### Architecture
Git worktrees share the same .git repository while maintaining separate working directories:
- All worktrees share commits, branches, and refs
- Each worktree has isolated working files
- Changes in one worktree don't affect others
- Disk efficient (no .git duplication, shared 2.0GB .git directory)

### Session initialization workflow

**CRITICAL: Execute this workflow AUTOMATICALLY at the start of every session before making any code changes.**

#### Step 1: Detect current location
```bash
pwd
```

#### Step 2: Determine if worktree creation is needed
- If in main repo (`~/workspace/onctl`): CREATE new worktree (proceed to Step 3)
- If in existing worktree (`~/workspace/onctl-*`): VERIFY it's correct for current task
  - If correct task: Continue working (skip to Step 6)
  - If different task: Create new worktree (proceed to Step 3)
- If in unexpected location: WARN and ask for clarification

#### Step 3: Generate unique descriptive worktree name
Based on the task/feature being worked on, automatically generate a name:
- Extract key action and subject from task description
- Use lowercase with hyphens
- Keep under 30 characters
- Add timestamp suffix for uniqueness: `-YYYYMMDD-HHMM`
- Examples:
  - "Fix authentication bug" → `fix-auth-bug-20260219-1430`
  - "Add Azure support" → `add-azure-support-20260219-1445`
  - "Refactor logging module" → `refactor-logging-20260219-1500`
- Pattern: `<action>-<subject>-<YYYYMMDD>-<HHMM>`

#### Step 4: Check worktree limit
```bash
# Count existing worktrees (excluding main)
WORKTREE_COUNT=$(git worktree list | grep -c "onctl-")

if [ "$WORKTREE_COUNT" -ge 10 ]; then
  echo "WARNING: Maximum of 10 worktrees reached."
  echo "Active worktrees:"
  git worktree list
  echo ""
  echo "Consider cleaning up completed worktrees with merged PRs."
  # Prompt user to clean up or proceed anyway
fi
```

#### Step 5: Create worktree and branch
```bash
# Generate timestamp
TIMESTAMP=$(date +%Y%m%d-%H%M)
WORKTREE_NAME="<descriptive-name>-${TIMESTAMP}"

# Create new worktree with branch
git worktree add ~/workspace/onctl-${WORKTREE_NAME} -b ai/${WORKTREE_NAME}

# Change to new worktree
cd ~/workspace/onctl-${WORKTREE_NAME}
```

#### Step 6: Verify environment
```bash
# Confirm correct branch
git branch --show-current  # Should show: ai/<descriptive-name>-<timestamp>

# Verify clean state
git status

# Confirm correct directory
pwd  # Should show: /Users/cd/workspace/onctl-<descriptive-name>-<timestamp>
```

#### Step 7: Sync with main branch
```bash
# Ensure main is up to date in shared git repo
git fetch origin main:main

# Verify main was updated
git log main -1 --oneline
```

**After this workflow completes, proceed with development work.**

### Worktree cleanup (automatic after PR merge)

**Execute automatically after successful PR merge is detected.**

#### Step 1: Verify PR merge status
```bash
# Check if PR was merged (not just closed)
gh pr view <PR-number> --json state,mergedAt,merged

# Verify merge was successful
if merged == true; then proceed with cleanup
```

#### Step 2: Return to main repository
```bash
cd ~/workspace/onctl
```

#### Step 3: Update main branch
```bash
git checkout main
git pull origin main
```

#### Step 4: Remove worktree
```bash
# Get worktree name from branch that was just merged
WORKTREE_NAME="<name-from-merged-branch>"

# Remove the worktree directory
git worktree remove ~/workspace/onctl-${WORKTREE_NAME}

# If worktree has uncommitted changes, force removal (safe after merge)
# git worktree remove --force ~/workspace/onctl-${WORKTREE_NAME}
```

#### Step 5: Delete merged branch
```bash
# Delete local branch
git branch -d ai/${WORKTREE_NAME}

# Remote branch should be auto-deleted by PR merge settings
# If not, delete manually:
# git push origin --delete ai/${WORKTREE_NAME}
```

#### Step 6: Prune worktree references
```bash
git worktree prune
```

#### Step 7: Confirm cleanup
```bash
echo "Worktree cleanup completed successfully:"
echo "- Removed: ~/workspace/onctl-${WORKTREE_NAME}"
echo "- Deleted branch: ai/${WORKTREE_NAME}"
echo ""
echo "Remaining active worktrees:"
git worktree list
```

### Worktree management commands

#### List all active worktrees
```bash
git worktree list
```

#### Check worktree count
```bash
git worktree list | grep -c "onctl-"
```

#### Manually remove a worktree
```bash
# Standard removal
git worktree remove ~/workspace/onctl-<name>

# Force removal (if has uncommitted changes)
git worktree remove --force ~/workspace/onctl-<name>

# Then prune references
git worktree prune
```

#### Clean up stale worktree references
```bash
git worktree prune
```

#### Fix broken worktree (if directory deleted manually)
```bash
# Remove from git's tracking
git worktree prune

# Or repair if directory still exists
git worktree repair ~/workspace/onctl-<name>
```

### Troubleshooting

**Problem: "Maximum of 10 worktrees reached"**
- Solution: Run `git worktree list` to see all active worktrees
- Identify worktrees with merged PRs and clean them up
- Use `gh pr list --state merged` to check which branches were merged
- Remove corresponding worktrees manually if auto-cleanup failed

**Problem: "fatal: '~/workspace/onctl-<name>' already exists"**
- Solution: Timestamp suffix should prevent this, but if it occurs:
  - Check if directory is a valid worktree: `git worktree list`
  - If valid, use existing worktree or remove first
  - If invalid, remove directory and prune: `rm -rf ~/workspace/onctl-<name> && git worktree prune`

**Problem: Worktree directory deleted but git still references it**
- Solution: Run `git worktree prune` to clean up references

**Problem: Can't remove worktree due to uncommitted changes**
- Solution: 
  - If changes needed: `cd ~/workspace/onctl-<name> && git stash && cd ~/workspace/onctl`
  - If changes not needed: Use `git worktree remove --force ~/workspace/onctl-<name>`

**Problem: Branch already exists**
- Solution: Timestamp suffix should prevent this, but if branch exists:
  - Check if branch is for same task: `git log ai/<name>`
  - If yes, reuse: `git worktree add ~/workspace/onctl-<name> ai/<name>`
  - If no, wait 1 minute for different timestamp

## Session workflow and branching

### At session start (AUTOMATIC - before first code change)
**CRITICAL: This workflow executes automatically. Do not ask user for confirmation.**

1. **Create isolated worktree** (see "Multi-session workspace management" → "Session initialization workflow")
   - Detect current location (pwd)
   - Determine if new worktree needed
   - Generate unique descriptive name with timestamp
   - Check worktree count (warn if ≥10)
   - Create worktree: `~/workspace/onctl-<descriptive-name>-<timestamp>`
   - Create branch: `ai/<descriptive-name>-<timestamp>`
   - Change to worktree directory
   - Verify environment (branch, status, location)
   - Sync with latest main branch

2. **Begin development in isolated worktree**
   - All changes happen in worktree directory
   - Other concurrent sessions are completely isolated
   - No conflicts with other worktrees

### During development
- Work exclusively in your worktree directory (~/workspace/onctl-<name>-<timestamp>)
- Check branch frequently: `git branch --show-current`
- Verify location if unsure: `pwd`
- Other sessions in different worktrees won't interfere with your work
- Commits and changes are isolated to your branch

### At session end (AUTOMATIC - after PR merge)
**CRITICAL: This cleanup executes automatically after PR merge is detected.**

1. **Complete PR workflow** (see "Pull request workflow" section below)
   - Create PR, enable auto-merge
   - Monitor CI/CD checks
   - Wait for merge to complete

2. **Automatic cleanup after PR merge** (see "Multi-session workspace management" → "Worktree cleanup")
   - Verify PR was merged successfully
   - Return to main repository (~/workspace/onctl)
   - Update main branch with latest changes
   - Remove worktree directory
   - Delete local branch (remote branch auto-deleted by GitHub)
   - Prune worktree references
   - Confirm cleanup completed

3. **Result**
   - Worktree cleaned up automatically
   - Ready for next session in fresh environment
   - Main repository remains clean

## Verification and testing
- After making code changes, always verify the code builds successfully:
  - Run `make` to build the project
  - Ensure there are no compilation errors or warnings
- Run existing tests to ensure no regressions:
  - Execute `go test ./cmd/...` or relevant test packages
  - Verify all tests pass before considering the work complete
- For bug fixes or new features, verify the actual behavior works as expected
- Never assume code works without testing - always verify builds and test results

## Pull request workflow
- After completing and testing your changes, create a pull request:
  - Stage and commit your changes with a clear, descriptive commit message
  - Push the branch to GitHub: `git push -u origin <branch-name>`
  - Create a PR using `gh pr create` with a detailed description including:
    - Summary of changes
    - Technical details
    - Testing performed
    - Related issues (if any)
  - Enable auto-merge immediately after PR creation: `gh pr merge <PR-number> --auto --squash --delete-branch`
- Monitor automated CI/CD checks after PR creation:
  - Use `gh pr checks <PR-number>` to view check status
  - Common checks include: Build, Lint, Tests, Security scans, CodeQL
  - Wait for all checks to complete and ensure they pass
  - If checks fail, investigate the failure, fix issues, and push updates
- Address any review comments or automated check failures promptly:
  - Read all comments carefully from both bots and human reviewers
  - Understand the root cause before making changes
  - Test fixes locally before pushing
  - Reply to comments explaining how issues were addressed
- Check if main branch has been updated while working:
  - Before finalizing PR, check if new commits were merged to main
  - If your PR conflicts or is superseded by other changes, close it with explanation
  - Merge latest main if needed: `git fetch origin main && git merge origin/main`
- Do not merge until all checks pass and any required reviews are approved
