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
- This ensures each AI-assisted development session is tracked separately and can be reviewed independently

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
- Monitor automated CI/CD checks after PR creation:
  - Use `gh pr checks <PR-number>` to view check status
  - Common checks include: Build, Lint, Tests, Security scans, CodeQL
  - Wait for all checks to complete and ensure they pass
  - If checks fail, investigate the failure, fix issues, and push updates
- Address any review comments or automated check failures promptly
- Do not merge until all checks pass and any required reviews are approved
