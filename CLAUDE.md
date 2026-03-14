# onctl — Claude Code Instructions

## Project overview

`onctl` is a Go CLI tool for managing virtual machines across multiple cloud providers (AWS, Azure, GCP, Hetzner). Core packages:

- `cmd/` — CLI command wiring (cobra)
- `internal/` — provider implementations and business logic
- `main.go` — entry point

---

## Code Review Instructions

When reviewing a pull request, work through every section below and call out any issues. Use the labels **Must fix**, **Should fix**, or **Nit** to indicate severity.

### Correctness
- Does the change do what the PR description says?
- Are edge cases handled: nil pointers, empty input, zero values, missing/optional config fields?
- Are error paths exercised, not just the happy path?
- Are all supported providers (AWS, Azure, GCP, Hetzner) unaffected or explicitly updated?
- Are CLI flags validated before use? Invalid input must exit non-zero with a clear message.
- Does any cloud-init or SSH key logic work correctly across all providers?
- Are there tests covering the new behaviour? PRs that add features or fix bugs without tests need a clear explanation.

### Security
- Are secrets, tokens, or credentials hardcoded or logged anywhere? Check `fmt.Println`, `log.*`, and error messages.
- Is user-supplied input (CLI args, config values) sanitised before being passed to shell commands, cloud APIs, or file paths?
- Is SSH key/credential handling least-privilege? Are file permissions set explicitly?
- Are cloud credentials sourced from environment variables or provider SDKs only — never from code or checked-in config?
- Are new dependencies in `go.mod` / `go.sum` intentional and low-risk?
- Do Snyk, `govulncheck`, and the OpenSSF Scorecard CI checks pass?
- Are all network calls using TLS? Flag any `InsecureSkipVerify: true`.

### Performance
- Are there N×API-call patterns for multi-provider operations that could be batched?
- Are goroutines bounded and guaranteed to exit cleanly?
- Is context cancellation (`ctx.Done()`) respected so `Ctrl-C` works?
- If concurrency was touched, was `go test -race ./...` run?
- Are large structs passed by pointer?

### Maintainability
- Is new code in the right package? CLI wiring belongs in `cmd/`, business logic in `internal/`.
- Are errors wrapped with `fmt.Errorf("...: %w", err)` and handled at the right level — not swallowed?
- Are user-facing error messages actionable (e.g. "could not find SSH key at ~/.ssh/id_rsa — set SSH_KEY_PATH")?
- Do public functions and types have godoc comments?
- Are there any backward-incompatible changes to config file format or CLI flags without a migration path?
- Do file paths use `filepath.Join`? (This tool ships on Windows — no UNIX-only assumptions.)
- If CLI usage, flags, or supported providers changed, is documentation updated?

### CI
- Are `build`, `lint`, `vuln`, `snyk`, and `scorecard` checks all green?

---

## General coding conventions

- Follow standard Go idioms: `camelCase`, unexported by default, exported only when needed.
- Functions should do one thing; keep them short enough to understand without scrolling.
- Comments explain *why*, not *what*.
- No duplication of logic that already exists in the codebase.
- Cross-platform paths: always use `filepath.Join`, never hardcode `/` separators.
- Run `go test ./...` before marking a PR ready.
