# Coding Conventions

**Analysis Date:** 2026-03-23

## Naming Patterns

**Files:**
- Snake_case for multi-word files: `shell_split.go`, `analyze_writing.go`, `build_local.go`
- Suffix `_schema.go` for files that register schema metadata: `workflow_schema.go`, `diff_schema.go`, `context_schema.go`
- Suffix `_test.go` for all test files (co-located with source)
- `export_test.go` for exposing internal symbols to external test packages: `internal/client/export_test.go`, `internal/config/export_test.go`

**Functions:**
- `run<Command>` pattern for cobra command handlers: `runTransition`, `runAssign`, `runComment`, `runConfigure`, `runRaw`
- `new<Thing>` for constructors: `newTestClient`, `newTestEnv`, `newE2EClient`
- `make<Thing>` for in-test factory helpers: `makeComposed`
- `<Verb><Noun>` for utility functions: `QueryFromFlags`, `ExitCodeFromStatus`, `ErrorTypeFromStatus`

**Variables:**
- camelCase for local variables: `serverURL`, `authType`, `exitCode`
- Package-level vars use camelCase: `workflowCmd`, `transitionCmd`, `skipClientCommands`
- Unexported package-level constants use camelCase: `goos`
- Exported constants use PascalCase: `ExitOK`, `ExitAuth`, `ExitNotFound`

**Types:**
- PascalCase structs and interfaces: `APIError`, `RetryableError`, `AuthConfig`, `ComposedCharacter`
- JSON/YAML struct tags on all exported fields: `json:"error_type"`, `yaml:"style_guide"`
- Both json and yaml tags present on dual-serialized types (e.g., `Character` in `internal/character/types.go`)
- Unexported context key types: `type contextKey struct{}`

**Import Aliases:**
- `jrerrors` alias for the internal errors package: `jrerrors "github.com/sofq/jira-cli/internal/errors"`
- Module path is `github.com/sofq/jira-cli` (all internal imports use this)

## Code Style

**Formatting:**
- Standard `gofmt`/`goimports` formatting (no separate config, enforced by `golangci-lint`)

**Linting:**
- `golangci-lint` with `default: standard` linter set
- Config at `.golangci.yml`
- `errcheck` excluded for: `fmt.Fprintf`, `fmt.Fprintln`, `fmt.Fprint`, `(io.Writer).Write`, `(*net/http.Response.Body).Close`, `(io.Closer).Close`, `os.Setenv`, `os.Unsetenv`, `os.Remove`, `os.WriteFile`, `(*os.File).Close`
- Inline `//nolint:errcheck` for intentional ignores in tests and deferred cleanup

## Import Organization

Standard Go import grouping (goimports enforces):
1. Standard library
2. External packages
3. Internal packages (`github.com/sofq/jira-cli/...`)

Example from `cmd/workflow.go`:
```go
import (
    "bytes"
    "context"
    "encoding/json"

    "github.com/spf13/cobra"

    "github.com/sofq/jira-cli/internal/adf"
    "github.com/sofq/jira-cli/internal/client"
    jrerrors "github.com/sofq/jira-cli/internal/errors"
)
```

## Error Handling

**Structured error type:** All errors are `*jrerrors.APIError` (defined in `internal/errors/errors.go`).

**Pattern for command handlers:**
1. Construct `&jrerrors.APIError{ErrorType: "...", Message: "..."}`
2. Call `apiErr.WriteJSON(os.Stderr)` to emit JSON to stderr
3. Return `&jrerrors.AlreadyWrittenError{Code: jrerrors.ExitValidation}` to signal error already written

**Pattern for internal functions returning errors:**
- Use `fmt.Errorf("context: %w", err)` for wrapped errors
- Return `error` interface values, not `*APIError`
- Command layer converts internal errors to `APIError` at the boundary

**Exit codes are semantic (defined in `internal/errors/errors.go`):**
- `ExitOK = 0`, `ExitError = 1`, `ExitAuth = 2`, `ExitNotFound = 3`
- `ExitValidation = 4`, `ExitRateLimit = 5`, `ExitConflict = 6`, `ExitServer = 7`

**HTTP → exit code mapping** via `ExitCodeFromStatus(status int)` — do not hardcode exit codes from HTTP status.

## Output Convention

- **stdout**: JSON responses only
- **stderr**: structured JSON errors (via `APIError.WriteJSON(os.Stderr)`)
- `--dry-run` flag outputs the request JSON to stdout without executing
- `--verbose` logs HTTP details to stderr as JSON

## Logging

No logging framework — uses `fmt.Fprintln(os.Stderr, ...)` or `APIError.WriteStderr()` for structured errors. Verbose mode writes JSON to `client.Stderr`.

## Comments

**Package doc comments:** Present on packages with non-obvious purpose:
```go
// Package yolo implements the "You Only Live Once" execution policy for jr.
// Package character defines types and logic for managing reusable character profiles...
// Package format provides helpers to render JSON data as human-readable...
```

**Exported symbols:** GoDoc comments on all exported types, functions, and constants:
```go
// AlreadyWrittenError is a sentinel error indicating that the JSON error has
// already been written to stderr, so the caller should not write a second one.
type AlreadyWrittenError struct{ Code int }

// ExitCodeFromStatus maps an HTTP status code to a structured exit code.
func ExitCodeFromStatus(status int) int {
```

**Internal unexported helpers:** Short inline comments explaining non-obvious behavior.

**Test helpers:** `t.Helper()` called at top of all test helper functions.

## Function Design

**Cobra command handlers** always have signature `func run<Cmd>(cmd *cobra.Command, args []string) error`.

**Command registration** uses `init()` to attach flags and `RunE` (not `Run`) so errors propagate:
```go
var transitionCmd = &cobra.Command{
    Use:  "transition",
    RunE: runTransition,
}
func init() {
    transitionCmd.Flags().String("issue", "", "issue key (e.g. PROJ-123)")
    _ = transitionCmd.MarkFlagRequired("issue")
}
```

**Context threading:** `client.Client` is stored in cobra's command context and retrieved via `client.FromContext(ctx)`. Use `client.NewContext(ctx, c)` to inject; use `client.FromContext(cmd.Context())` in handlers.

**Return values:** Internal functions return `(result, error)`; command handlers return only `error`.

## Module Design

**Exports:**
- Commands package (`cmd/`) exports nothing — all cobra command vars are package-level vars consumed by `main.go`
- `internal/` packages export only what's needed by the cmd layer
- Generated commands in `cmd/generated/` expose `AllSchemaOps()` and per-resource init functions

**Barrel Files:**
- Not used. Each `internal/<pkg>` package is imported directly by path.

**Generated Code:**
- `cmd/generated/` is fully regenerated by `make generate` — do not edit manually
- `gen/` contains the generator itself (source of truth is `spec/jira-v3.json`)

---

*Convention analysis: 2026-03-23*
