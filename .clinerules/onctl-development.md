## Brief overview
Guidelines for developing the onctl CLI tool, a Go-based utility for managing cloud resources across multiple providers (AWS, Azure, GCP, Hetzner). These rules focus on Go best practices, CLI design patterns, and cloud integration approaches observed in the project.

## Go coding conventions
- Use `gofmt` and `goimports` for consistent code formatting and import organization
- Follow standard Go naming conventions (PascalCase for exported, camelCase for unexported)
- Use meaningful variable and function names that reflect their purpose in cloud management context
- Prefer early returns and avoid nested if statements for cleaner control flow

## Linting practices
- Use `golangci-lint` for comprehensive code quality checking
- Run `make lint` or `golangci-lint run` before committing changes
- Address all lint warnings and errors to maintain code quality standards
- Configure linters in `.golangci.yml` with project-specific settings when needed

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
- Ensure test coverage does not decrease; cover all new lines added with tests to prevent codecov from blocking PR merges

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
