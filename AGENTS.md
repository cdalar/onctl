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

## Session workflow and branching
- Before starting work, ensure you're on an updated main/master branch:
  - Switch to main branch: `git checkout main` (or `master`)
  - Pull latest changes: `git pull origin main`
  - Verify you're on the right branch with `git branch --show-current`
- Create a new Git branch at the start of each development session before making the first code change
- Branch naming convention: Use `ai/` prefix followed by a descriptive name (e.g., `ai/fix-cursor-visibility`, `ai/add-logging-support`)
- Branch creation timing: Create the branch before making the first edit or code change in a session
- Check frequently with `git branch --show-current` to ensure you're on the correct branch
- This ensures each AI-assisted development session is tracked separately and can be reviewed independently

## Coordinating multiple agents on the same codebase (no conflicts)
Plain `git checkout -b ai/xxx` inside the *same directory* does **not** isolate agents. They will immediately see every file created or edited on other branches because they share one working tree.

**Use isolated worktrees** (strongly preferred) or separate clones for true parallelism:

1. From a clean main checkout create a dedicated worktree + branch for each agent:
   ```
   git worktree add -b ai/agent-foo ../onctl-ai-foo
   git worktree add -b ai/agent-bar ../onctl-ai-bar
   ```
2. Launch one `opencode` (any model) inside each isolated directory:
   ```
   cd ../onctl-ai-foo && opencode
   cd ../onctl-ai-bar && opencode   # fully independent filesystem
   ```
3. When finished, merge via the normal PR workflow. The worktree can be removed with:
   ```
   git worktree remove ../onctl-ai-foo
   git branch -D ai/agent-foo
   ```

Existing patterns in this repo (see `.claude/worktrees/` and Conductor workspaces) already follow this model.

Additional safeguards:
- Use the `freeze` skill to restrict edits to specific sub-directories within a worktree.
- One agent can stay in Plan mode (Tab) as an orchestrator that delegates disjoint tasks to other agents.
- After any external changes, always re-run the pre-work steps above before continuing.

This combination (per-agent worktrees + `ai/` branches + freeze) allows many opencode/grok-build instances to safely operate on the same repository at the same time.

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
- After PR creation, **do not merge yet**:
  - Use `gh pr checks <PR-number>` repeatedly to monitor all automated pipelines.
  - Wait until **every** required CI check (Build, Lint, Tests, Security, CodeQL, Analyze, etc.) is green/passed.
  - Use `gh pr reviews <PR-number>` (or `gh pr view`) to inspect any review comments or change requests.
  - Address failing checks and all review feedback before proceeding further.
  - If the repo branch protection enables **"Require conversation resolution before merging"** (current setting for this repo's main), return to the GitHub PR page and explicitly click **"Resolve conversation"** on *every* thread from Copilot, claude-review, Codex, etc. Simply replying in the thread is not enough — the conversations must be marked resolved in the UI to satisfy the protection rule before you can merge.
- Only enable auto-merge (or merge) when **both** of the following are true:
  - All required CI checks have passed, **and**
  - Any requested or required human/bot reviews have been approved (or the user has explicitly authorized the merge).
  Then you may run:
    `gh pr merge <PR-number> --auto --squash --delete-branch`
- After the PR successfully merges to main:
  - The `--delete-branch` flag already removes the remote tracking branch.
  - Clean up the local isolated worktree and session branch (run from your main clone directory):
    ```
    git worktree remove ../onctl-ai-xxx
    git branch -D ai/your-branch-name
    git worktree prune
    ```
- Check if main branch has been updated while working:
  - Before finalizing PR, check if new commits were merged to main
  - If your PR conflicts or is superseded by other changes, close it with explanation
  - Merge latest main if needed: `git fetch origin main && git merge origin/main`
- Do not merge until all checks pass and any required reviews are approved.
