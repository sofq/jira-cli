# Testing Patterns

**Analysis Date:** 2026-03-23

## Test Framework

**Runner:**
- Go's built-in `testing` package — no third-party test runner
- No testify, gomock, or assertion libraries; all assertions are manual `if` checks

**Assertion Library:**
- None — standard `t.Errorf`, `t.Fatalf`, `t.Fatal`, `t.Error`

**Run Commands:**
```bash
go test ./...              # Run all tests
go test -run TestName ./...  # Run specific test
go test -fuzz FuzzApply ./internal/jq/...  # Fuzz test
go test -coverprofile=coverage.out ./...  # Coverage
go tool cover -html=coverage.out          # View coverage
make test                  # Alias for go test ./...
make lint                  # Run golangci-lint
```

## Test File Organization

**Location:**
- Co-located with source: `internal/errors/errors_test.go` sits next to `internal/errors/errors.go`
- Same pattern throughout: `internal/avatar/fetch_test.go`, `cmd/workflow_test.go`, `gen/parser_test.go`

**Naming:**
- `<source_file>_test.go` — mirrors source file: `client.go` → `client_test.go`
- `export_test.go` — special file for exposing internals to black-box tests (compiled only during `go test`)

**Package naming — two patterns in use:**
- **White-box (internal) tests:** `package <pkg>` — same package as source, accesses unexported symbols. Used in `internal/yolo/decide_test.go` (`package yolo`), `internal/avatar/e2e_test.go` (`package avatar`)
- **Black-box (external) tests:** `package <pkg>_test` — separate package, tests public API only. Used in `internal/errors/errors_test.go` (`package errors_test`), `internal/client/client_test.go` (`package client_test`), `internal/jq/jq_test.go` (`package jq_test`)

**Dedicated test directories:**
- `test/e2e/e2e_test.go` — binary-level E2E tests (builds and executes the `jr` binary)
- `test/integration/integration_test.go` — live Jira Cloud integration tests (skipped unless env vars set)

## Test Structure

**Flat function style (most common):**
```go
func TestExitCodeFromStatus(t *testing.T) {
    tests := []struct {
        status   int
        expected int
    }{
        {200, jrerrors.ExitOK},
        {401, jrerrors.ExitAuth},
        {404, jrerrors.ExitNotFound},
    }
    for _, tt := range tests {
        got := jrerrors.ExitCodeFromStatus(tt.status)
        if got != tt.expected {
            t.Errorf("ExitCodeFromStatus(%d) = %d, want %d", tt.status, got, tt.expected)
        }
    }
}
```

**Table-driven subtests (used in some packages):**
```go
// internal/client/client_test.go, internal/changelog/changelog_test.go
t.Run("subtest name", func(t *testing.T) {
    // ...
})
```

**HTTP handler pattern (cmd/ tests):**
```go
func TestRunTransition_Success(t *testing.T) {
    ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        if r.Method == "GET" && strings.Contains(r.URL.Path, "/transitions") {
            w.Header().Set("Content-Type", "application/json")
            fmt.Fprintln(w, `{"transitions":[{"id":"31","name":"Done"}]}`)
            return
        }
        w.WriteHeader(http.StatusNoContent)
    }))
    defer ts.Close()

    var stdout, stderr bytes.Buffer
    c := newTestClient(ts.URL, &stdout, &stderr)
    // ...
}
```

**Test naming convention:**
- `Test<FunctionName>_<Scenario>` — e.g., `TestRunTransition_Success`, `TestRunTransition_DryRun`
- `Test<TypeName><MethodName>` — e.g., `TestDecideRuleBasedMatch`, `TestExitCodeFromStatus`

## Mocking

**Framework:** None — no mock generation library. All mocking is via `net/http/httptest`.

**HTTP mocking pattern** (used universally in cmd/ and internal/ tests):
```go
ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
    if r.Method == "GET" && strings.Contains(r.URL.Path, "/rest/api/3/issue/PROJ-1") {
        w.Header().Set("Content-Type", "application/json")
        fmt.Fprintln(w, `{"key":"PROJ-1","fields":{...}}`)
        return
    }
    w.WriteHeader(http.StatusNotFound)
}))
defer ts.Close()
```

**What to mock:**
- HTTP endpoints — always via `httptest.NewServer`
- File system paths — via `t.TempDir()` + package-level overrides exposed through `export_test.go`
- OS-level globals (e.g., GOOS) — via `export_test.go` overrides and `t.Cleanup` restoration

**What NOT to mock:**
- Internal business logic — test it directly
- The `client.Client` struct — construct it with real fields pointing at `httptest` servers

**Export test pattern** for overriding internal package-level vars:
```go
// internal/client/export_test.go (compiled only during go test)
package client

func SetOAuth2CacheFileForTest(t *testing.T, path string) {
    t.Helper()
    old := oauth2CacheFilePathOverride
    oauth2CacheFilePathOverride = path
    t.Cleanup(func() { oauth2CacheFilePathOverride = old })
}
```

## Fixtures and Factories

**Test client factory (cmd/ tests):**
```go
// cmd/helpers_test.go
func newTestClient(serverURL string, stdout, stderr *bytes.Buffer) *client.Client {
    return &client.Client{
        BaseURL:    serverURL,
        Auth:       config.AuthConfig{Type: "basic", Username: "u", Token: "t"},
        HTTPClient: &http.Client{},
        Stdout:     stdout,
        Stderr:     stderr,
        Paginate:   true,
    }
}
```

**Test env struct (internal/client tests):**
```go
// internal/client/client_test.go
type testEnv struct {
    Server *httptest.Server
    Client *client.Client
    Stdout *bytes.Buffer
    Stderr *bytes.Buffer
}

func newTestEnv(handler http.HandlerFunc) *testEnv { ... }
```

**In-package factory helpers (yolo tests):**
```go
func makeComposed(reactions map[string]character.Reaction) *character.ComposedCharacter {
    return &character.ComposedCharacter{Character: character.Character{Reactions: reactions}}
}
```

**Inline JSON fixtures:**
- Static JSON strings inlined directly in test functions
- Package-level `var sampleJSON = []byte(...)` for shared fixtures used across multiple tests in the same file
- ADF builder helpers in `internal/avatar/e2e_test.go` for complex mock responses

**Location:**
- No separate fixtures directory — all fixtures are inline or in the test file
- E2E tests build complex mock response builders as helper functions within the `*_test.go` file

## Coverage

**Requirements:** No enforced coverage threshold in CI config (none found).

**View Coverage:**
```bash
go test -coverprofile=coverage.out ./...
go tool cover -html=coverage.out
```

Coverage output files (`coverage*.out`, `cmd_cover*.out`) are present in the repo root but in `.gitignore` (untracked).

## Test Types

**Unit Tests:**
- Scope: individual functions and packages in isolation
- Location: co-located `*_test.go` files throughout `internal/` and `cmd/`
- Approach: direct function calls + `httptest.NewServer` for HTTP dependencies
- Examples: `internal/errors/errors_test.go`, `internal/retry/retry_test.go`, `internal/cache/cache_test.go`

**Integration Tests (package-internal "integration"):**
- `internal/avatar/e2e_test.go` — runs the full avatar extraction pipeline against a mock HTTP server; not true E2E but exercises multi-step flows within the package
- Label: `package avatar` (white-box)

**E2E Tests:**
- `test/e2e/e2e_test.go` — builds the `jr` binary via `exec.Command("go", "build", ...)` into `t.TempDir()`, then executes it against a mock HTTP server
- `e2e_test.go` (root) — similar pattern
- Helper: `buildBinary(t)` + `runJR(t, binary, serverURL, args...)` — both capture stdout/stderr and return exit code

**Integration Tests (live):**
- `test/integration/integration_test.go` — tests against real Jira Cloud
- Skipped unless `JR_BASE_URL` and `JR_AUTH_TOKEN` env vars are set
- Uses `skipUnlessConfigured(t)` helper at the top of each test

**Fuzz Tests:**
- `internal/jq/fuzz_test.go` — fuzz `jq.Apply` for panic safety
- Pattern: seed corpus with representative inputs, verify no panics

## Common Patterns

**Asserting JSON output:**
```go
var result map[string]string
if err := json.Unmarshal([]byte(strings.TrimSpace(stdout.String())), &result); err != nil {
    t.Fatalf("stdout not valid JSON: %s", stdout.String())
}
if result["status"] != "transitioned" {
    t.Errorf("expected status=transitioned, got %s", result["status"])
}
```

**Asserting exit codes:**
```go
code := batchTransition(t.Context(), c, "TEST-1", "Done")
if code != jrerrors.ExitOK {
    t.Fatalf("expected exit 0, got %d; stderr=%s", code, stderr.String())
}
```

**Dry-run testing:**
```go
c.DryRun = true
cmd := newTransitionCmd(c)
err := cmd.RunE(cmd, nil)
// Assert output contains HTTP method and issue key, not actual request
if !strings.Contains(output, "POST") {
    t.Errorf("expected POST in dry-run output, got: %s", output)
}
```

**Async/timing tests:**
```go
start := time.Now()
err := Do(ctx, fn, Config{MaxRetries: 3, BaseDelay: 5 * time.Millisecond})
elapsed := time.Since(start)
if elapsed < 5*time.Millisecond {
    t.Errorf("expected at least 5ms delay, got %v", elapsed)
}
```

**Context cancellation testing:**
```go
ctx, cancel := context.WithCancel(context.Background())
cancel() // cancel before calling
err := Do(ctx, fn, cfg)
if !errors.Is(err, context.Canceled) {
    t.Fatalf("expected context.Canceled, got %v", err)
}
```

**stdout capture for commands that write to os.Stdout directly:**
```go
// cmd/helpers_test.go
func captureStdout(t *testing.T, fn func()) string {
    t.Helper()
    oldStdout := os.Stdout
    r, w, _ := os.Pipe()
    os.Stdout = w
    fn()
    w.Close()
    os.Stdout = oldStdout
    var buf bytes.Buffer
    _, _ = buf.ReadFrom(r)
    return buf.String()
}
```

---

*Testing analysis: 2026-03-23*
