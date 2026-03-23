# Architecture

**Analysis Date:** 2026-03-23

## Pattern Overview

**Overall:** CLI application with generated + hand-written command layer over a shared HTTP client core. Agent-oriented: all output is JSON on stdout, errors are JSON on stderr, exit codes are semantic.

**Key Characteristics:**
- Cobra command tree with `PersistentPreRunE` injecting a configured `*client.Client` into context before every command runs
- OpenAPI spec → code generation pipeline produces `cmd/generated/` from `spec/jira-v3.json`
- Hand-written commands coexist with generated ones via a `mergeCommand` pattern that preserves generated subcommands
- All errors flow through `internal/errors.APIError` with typed `error_type` and semantic exit codes (0–7)
- Security enforced at the middleware layer: operation policy checked in `PersistentPreRunE` before commands execute

## Layers

**Entry Point:**
- Purpose: Parse CLI args, wire up cobra, delegate to `cmd.Execute()`
- Location: `main.go`
- Contains: `main()` — 12 lines, calls `cmd.Execute()` and `os.Exit(code)`
- Depends on: `cmd` package

**Command Layer:**
- Purpose: Map CLI invocations to API calls or internal operations
- Location: `cmd/` (hand-written) and `cmd/generated/` (auto-generated)
- Contains: Cobra `*cobra.Command` definitions with `RunE` handlers
- Depends on: `internal/client`, `internal/errors`, `internal/*` utility packages
- Used by: `main.go`

**Middleware (PersistentPreRunE):**
- Purpose: Resolve config, build the HTTP client, apply auth, check operation policy, set up audit logging
- Location: `cmd/root.go` lines 41–211
- Contains: Config resolution, `client.Client` construction, policy enforcement, audit setup
- Depends on: `internal/config`, `internal/client`, `internal/policy`, `internal/audit`, `internal/preset`

**HTTP Client:**
- Purpose: Execute HTTP requests against the Jira API with auth, pagination, caching, retries, jq filtering, output formatting
- Location: `internal/client/client.go`
- Contains: `Client` struct, `Do()` (writes to stdout), `Fetch()` (returns raw bytes), `WriteOutput()`, pagination handlers
- Depends on: `internal/cache`, `internal/audit`, `internal/jq`, `internal/format`, `internal/retry`, `internal/errors`

**Code Generation Pipeline:**
- Purpose: Generate `cmd/generated/` from the Jira OpenAPI spec
- Location: `gen/` (generator binary), `spec/jira-v3.json` (input spec), `gen/templates/*.go.tmpl` (templates)
- Contains: OpenAPI parser, operation grouper, Go code emitter via `text/template`
- Invoked by: `make generate`

**Internal Utility Packages:**
- Purpose: Shared logic consumed by both generated and hand-written commands
- Location: `internal/`
- Contains: One package per concern (see Key Abstractions)

**Agent/Autonomous Layer:**
- Purpose: User profiling, persona management, autonomous action loops
- Location: `internal/avatar/`, `internal/character/`, `internal/yolo/`, `cmd/avatar.go`, `cmd/avatar_act.go`, `cmd/character.go`, `cmd/yolo.go`
- Depends on: `internal/watch`, `internal/client`, `internal/character`

## Data Flow

**Standard Command Execution:**

1. `main()` calls `cmd.Execute()`
2. Cobra dispatches to the matching `*cobra.Command`
3. `PersistentPreRunE` runs: resolves config → builds `*client.Client` → checks policy → stores client in `context.Context`
4. `RunE` handler retrieves client via `client.FromContext(cmd.Context())`
5. Handler calls `c.Do(ctx, method, path, query, body)` or `c.Fetch(ctx, method, path, body)`
6. `Do()` executes HTTP request, handles pagination, applies jq filter, writes JSON to stdout
7. HTTP errors are serialized as `jrerrors.APIError` to stderr; process exits with semantic code

**Batch Execution:**

1. `cmd/batch.go` reads a JSON array of `BatchOp` from stdin or `--input`
2. For each op, it simulates CLI dispatch by invoking the matching generated command's `RunE` with a captured stdout buffer
3. Results are collected as `[]BatchResult` and written as a JSON array to stdout
4. `--parallel N` uses `sync.WaitGroup` + goroutines for concurrent execution

**Avatar Act Loop:**

1. `jr avatar act` starts a poll loop via `internal/watch.Run()`
2. `watch.Run()` POST to `/rest/api/3/search/jql` at `--interval`
3. Changed issues (detected by key+updated fingerprint) are emitted as `watch.Event`
4. `avatarActCmd` handler applies the active character's `Reactions` map to decide action
5. Actions are streamed as NDJSON `actEvent` records to stdout; dry-run mode skips execution

**State Management:**
- No in-process state between invocations — all state is in config files (`~/.config/jr/config.json`), avatar files (`~/.config/jr/avatars/<hash>/`), character files (`~/.config/jr/characters/`), and response caches (`~/.cache/jr/`)

## Key Abstractions

**`client.Client`:**
- Purpose: Single struct that carries all per-invocation settings and executes HTTP requests
- Examples: `internal/client/client.go`
- Pattern: Constructed fresh per invocation in `PersistentPreRunE`; passed via `context.Context`; never shared across goroutines unsafely

**`jrerrors.APIError`:**
- Purpose: Typed, JSON-serializable error with `error_type`, `status`, `message`, `hint`, semantic exit code
- Examples: `internal/errors/errors.go`
- Pattern: Every error path constructs an `APIError`, calls `.WriteJSON(os.Stderr)`, then returns `&AlreadyWrittenError{Code: ...}` to prevent double-printing

**`AlreadyWrittenError`:**
- Purpose: Sentinel that tells `Execute()` not to write a second error — the JSON was already written
- Examples: `internal/errors/errors.go`
- Pattern: All `RunE` functions either return nil or `&jrerrors.AlreadyWrittenError{Code: ...}`

**`config.ResolvedConfig`:**
- Purpose: Merged configuration from file + env vars + CLI flags
- Examples: `internal/config/config.go`
- Pattern: `config.Resolve()` applies precedence: CLI flags > env vars > config file

**`policy.Policy`:**
- Purpose: Glob-pattern operation access control (allowed_operations xor denied_operations)
- Examples: `internal/policy/policy.go`
- Pattern: A nil `*Policy` allows everything; non-nil enforced via `pol.Check(operation)` in middleware

**`avatar.Profile` / `character.Character`:**
- Purpose: Persona documents that shape agent writing style, workflow defaults, and reactions
- Examples: `internal/avatar/types.go`, `internal/character/types.go`
- Pattern: YAML on disk, loaded and passed to prompt formatters or reaction matchers

**Generated Commands:**
- Purpose: Cobra command tree covering the full Jira REST API surface from OpenAPI spec
- Examples: `cmd/generated/issue.go`, `cmd/generated/search.go`, `cmd/generated/init.go`
- Pattern: `generated.RegisterAll(rootCmd)` adds all generated commands; hand-written commands override specific ones via `mergeCommand()`

## Entry Points

**`main.go`:**
- Location: `main.go`
- Triggers: `jr` binary execution
- Responsibilities: Call `cmd.Execute()`, exit with returned code

**`cmd.Execute()`:**
- Location: `cmd/root.go` line 285
- Triggers: Called by `main()`
- Responsibilities: Run cobra dispatch; convert errors to JSON on stderr; return exit code

**`generated.RegisterAll(root)`:**
- Location: `cmd/generated/init.go`
- Triggers: Called during `cmd` package `init()`
- Responsibilities: Add all OpenAPI-generated resource commands to the root cobra command

**`gen/main.go`:**
- Location: `gen/main.go`
- Triggers: `make generate` (not part of the CLI binary)
- Responsibilities: Parse `spec/jira-v3.json`, group operations by resource, render `cmd/generated/*.go` files via Go templates

## Error Handling

**Strategy:** All errors are structured JSON on stderr. The `AlreadyWrittenError` sentinel prevents double-printing. Exit codes are semantic (0–7).

**Patterns:**
- `RunE` handlers: construct `jrerrors.APIError`, call `.WriteJSON(os.Stderr)`, return `&jrerrors.AlreadyWrittenError{Code: ...}`
- HTTP errors: `jrerrors.NewFromHTTP(status, body, method, path, resp)` creates the error; exit code derived from HTTP status via `ExitCodeFromStatus()`
- Config errors: exit 1 (`ExitError`)
- Auth errors (401/403): exit 2 (`ExitAuth`)
- Not found (404): exit 3 (`ExitNotFound`)
- Validation (400/422): exit 4 (`ExitValidation`)
- Rate limited (429): exit 5 (`ExitRateLimit`)
- Conflict (409): exit 6 (`ExitConflict`)
- Server errors (5xx): exit 7 (`ExitServer`)

## Cross-Cutting Concerns

**Logging:** `client.VerboseLog()` writes structured JSON to stderr when `--verbose` is set. No persistent application logger.

**Validation:** Performed at command flag level (cobra `MarkFlagRequired`), config resolution (`internal/config`), and policy enforcement (`internal/policy`).

**Authentication:** Three modes — `basic` (default: username + API token), `bearer` (token only), `oauth2` (client credentials grant). Applied per-request in `client.ApplyAuth()`. OAuth2 tokens are cached file-side (`internal/client/oauth2_cache.go`).

**Caching:** GET responses cached to disk by URL+auth hash. TTL set via `--cache` flag or metadata preset. Implementation in `internal/cache/`.

**Audit Logging:** JSONL appended to `~/.config/jr/audit.log`. Enabled per-profile or per-invocation. `audit.Logger` closed in `PersistentPostRunE`. Implementation in `internal/audit/`.

**Output Contract:** JSON always on stdout. Errors always on stderr. Human-readable table/CSV output goes to stderr (preserving JSON-only stdout). All `204 No Content` responses are normalized to `{}`.

---

*Architecture analysis: 2026-03-23*
