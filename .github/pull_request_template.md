## What does this PR do?

<!-- One or two sentences. Link the related issue if there is one. -->

Fixes #

---

## Code Review Checklist

> Tick everything that applies before requesting review. Leave items unchecked with a note if they genuinely don't apply.

### Correctness
- [ ] Change does what the PR description says
- [ ] Edge cases handled (nil, empty input, missing config)
- [ ] Error paths covered, not just happy path
- [ ] All supported providers (AWS / Azure / GCP / Hetzner) unaffected or explicitly updated
- [ ] CLI flags validated; invalid input exits non-zero with a clear message
- [ ] `go test ./...` passes locally

### Security
- [ ] No secrets, tokens, or credentials hardcoded or logged
- [ ] User input not passed unsanitised to shell, cloud APIs, or file paths
- [ ] SSH key/credential handling uses least-privilege; permissions set explicitly
- [ ] New dependencies are intentional and low-risk (`go.mod` / `go.sum` reviewed)
- [ ] Snyk, `govulncheck`, and Scorecard CI checks pass

### Performance
- [ ] No N×API-call patterns introduced for multi-provider operations
- [ ] Goroutines are bounded and have a clean exit path
- [ ] Context cancellation (`ctx.Done()`) respected so Ctrl-C works cleanly
- [ ] No data races — ran `go test -race ./...` if concurrency was touched

### Maintainability
- [ ] Code placed in the right package (`cmd/` for CLI wiring, `internal/` for logic)
- [ ] Errors wrapped with `fmt.Errorf("...: %w", err)` and handled at the right level
- [ ] User-facing error messages are actionable
- [ ] Public functions/types have godoc comments
- [ ] No backward-incompatible config/flag changes without a migration note
- [ ] Cross-platform paths use `filepath.Join` (tool ships on Windows too)
- [ ] `docs/` updated if CLI usage, flags, or providers changed

### CI
- [ ] `build`, `lint`, `vuln`, `snyk`, `scorecard` are all green

---

> Full checklist reference: [`docs/code-review-checklist.md`](../docs/code-review-checklist.md)
