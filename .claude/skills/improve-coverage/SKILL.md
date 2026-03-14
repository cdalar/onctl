---
name: improve-coverage
description: Improve Go test coverage for this repo. Run when asked to increase coverage, add tests, or fix codecov. Analyzes current coverage, identifies uncovered packages and functions, then writes targeted tests.
allowed-tools: Read, Glob, Grep, Bash, Edit, Write
---

# Improve Code Coverage

## Current Coverage State

- Coverage report: !`go test ./... -coverprofile=/tmp/onctl-coverage.out 2>&1 | grep -E "coverage:|ok|FAIL"`
- Per-function breakdown: !`go test ./... -coverprofile=/tmp/onctl-coverage.out 2>/dev/null && go tool cover -func=/tmp/onctl-coverage.out | grep -v "100.0%" | sort -t% -k1 -n | head -40`

## Workflow

### Phase 1: Identify Gaps

1. Parse the coverage output above — note packages with 0% or low coverage
2. For each low-coverage package, list the source files (not `_test.go`)
3. Focus on packages in this priority order:
   - `internal/cloud` (core interfaces)
   - `internal/domain` (domain types)
   - `cmd` (CLI commands, currently ~24%)
   - `internal/tools` (utilities, currently ~12%)
   - `internal/provideraws`, `internal/providerhtz`, etc.

### Phase 2: Analyze Source

For each target file:
1. Read the source file to understand exported functions, types, and logic branches
2. Check the existing `_test.go` file (if any) to see what's already covered
3. Identify:
   - Functions with zero tests
   - Branches not exercised (error paths, edge cases, nil inputs)
   - Logic that can be tested without real cloud credentials

### Phase 3: Write Tests

Follow these conventions from the codebase:

**Pattern**: Table-driven tests with `testify/assert`
```go
func TestFunctionName(t *testing.T) {
    tests := []struct {
        name     string
        input    Type
        expected Type
    }{
        {"case description", input, expected},
    }
    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            result := FunctionName(tt.input)
            assert.Equal(t, tt.expected, result)
        })
    }
}
```

**Imports**: Use `github.com/stretchr/testify/assert` and `github.com/stretchr/testify/require`

**Mocking cloud providers**: Use interface mocks — the providers implement interfaces in `internal/cloud`. Create minimal mock structs that satisfy the interface.

**Avoiding os.Exit in tests**: If a function calls `os.Exit`, test what you can around it (inputs that don't trigger exit) and use `assert.NotPanics` for basic smoke tests.

**File placement**: Add tests to the existing `_test.go` file in the same package, or create `<file>_test.go` if none exists.

### Phase 4: Verify

After writing tests, run:
```
go test ./... -coverprofile=/tmp/onctl-coverage-new.out 2>&1
go tool cover -func=/tmp/onctl-coverage-new.out | tail -5
```

Report the coverage delta (before vs after).

## Key Project Facts

- Module: `github.com/cdalar/onctl`
- Test framework: `testify` (`assert`, `require`)
- Cloud providers: AWS, Azure, GCP, Hetzner — all behind interfaces in `internal/cloud`
- CLI framework: `cobra` with `viper` for config
- Tests that need cloud credentials should be skipped with `t.Skip("requires live credentials")`
- `ARGUMENTS` = specific package or file to focus on (e.g., `internal/tools`, `cmd/ssh.go`). If empty, target the lowest-coverage packages first.

$ARGUMENTS
