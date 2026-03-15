# jr — Agent-Friendly Jira CLI Tool Design Spec

**Date:** 2026-03-15
**Status:** Approved (v2 — agent-first redesign)

## Overview

`jr` is a Go CLI tool designed primarily for **AI agent consumption**. It provides full Jira Cloud REST API v3 coverage via auto-generated commands. Every design decision prioritizes machine-parseability, deterministic behavior, and zero-interaction operation. Human usability is a secondary benefit, not the primary driver.

Uses [cobra](https://github.com/spf13/cobra) for command structure, `net/http` for API calls, and [gojq](https://github.com/itchyny/gojq) for built-in response filtering. Commands are generated from the official Jira Cloud OpenAPI v3 spec.

## Design Principles (Agent-First)

1. **Structured output always** — JSON to stdout, errors as JSON to stderr. No colorization by default.
2. **Zero interactive prompts** — all config via flags and env vars. Agents can't type.
3. **Deterministic exit codes** — agents parse exit codes to decide next actions.
4. **Self-describing** — `jr schema` lets agents discover available commands and their flags at runtime.
5. **Pipe-friendly** — stdin for request bodies, stdout for responses, stderr for errors.
6. **Filter at source** — built-in `--jq` flag to reduce output before the agent sees it.
7. **Auto-paginate** — return all results by default. Agents shouldn't manage pagination.
8. **Env-var-first auth** — no config file needed. Set 2-3 env vars and go.
9. **Batch mode** — process multiple operations in one invocation.

## Goals

1. Full coverage of all Jira Cloud REST API v3 endpoints
2. Resource-verb command structure: `jr <resource> <verb> [flags]`
3. Auto-generate commands from OpenAPI spec — no manual command authoring
4. Agent-friendly: structured JSON output, structured errors, deterministic exit codes
5. Self-describing via `jr schema` for runtime command/flag discovery
6. Built-in `--jq` filtering to minimize output tokens for LLM agents
7. Env-var-first auth with optional config file
8. Auto-pagination by default

## Non-Goals

- GUI or TUI interactive modes
- Colorized/pretty terminal output as default (available via `--pretty` flag for humans)
- Interactive prompts of any kind
- Plugin system (for v1)
- Jira Server/Data Center support (v1 targets Cloud only)
- Jira Software (Agile) API (`/rest/agile/1.0/`)

## Command Structure

```
jr <resource> <verb> [flags]
```

### Resource Grouping

Operations are grouped by the first meaningful path segment after `/rest/api/3/`. Examples:

| API Path | Resource | Verb | CLI Command |
|----------|----------|------|-------------|
| `GET /rest/api/3/issue/{issueIdOrKey}` | `issue` | `get` | `jr issue get --issueIdOrKey FOO-123` |
| `POST /rest/api/3/issue` | `issue` | `create` | `jr issue create --body @issue.json` |
| `GET /rest/api/3/project` | `project` | `list` | `jr project list` |
| `GET /rest/api/3/search` | `search` | `jql` | `jr search jql --jql "assignee = currentUser()"` |
| `PUT /rest/api/3/issue/{issueIdOrKey}` | `issue` | `update` | `jr issue update --issueIdOrKey FOO-123 --body @update.json` |
| `DELETE /rest/api/3/issue/{issueIdOrKey}` | `issue` | `delete` | `jr issue delete --issueIdOrKey FOO-123` |

### Verb Naming Convention

The generator derives verb names using this priority:
1. `operationId` from the spec (e.g., `getIssue` → `get`, `createIssue` → `create`)
2. HTTP method fallback: GET (single) → `get`, GET (list) → `list`, POST → `create`, PUT → `update`, DELETE → `delete`

When multiple operations on the same resource share a verb prefix, the generator disambiguates by appending the last non-parameter path segment as a suffix, kebab-cased. Algorithm:
1. Strip the resource prefix from the path (e.g., `/rest/api/3/issue/{issueIdOrKey}/transitions` → `transitions`)
2. Take the last non-parameter segment (ignore `{param}` segments)
3. Append as `verb-suffix` (e.g., `get-transitions`, `get-comments`)
4. If still ambiguous after this, append the second-to-last non-parameter segment too (e.g., `get-remote-link`)

### Built-in Commands (Not Generated)

| Command | Purpose |
|---------|---------|
| `jr configure` | Flag-driven auth/base-URL setup (no prompts) |
| `jr schema` | Machine-readable command/flag discovery |
| `jr batch` | Execute multiple operations from JSON input |
| `jr raw <method> <path>` | Escape hatch for raw API calls |
| `jr version` | Print version as JSON |
| `jr completion` | Shell completion (bash, zsh, fish, powershell) |

## Exit Codes

Agents parse exit codes to decide their next action:

| Code | Constant | Meaning | Agent Action |
|------|----------|---------|--------------|
| 0 | `ExitOK` | Success | Parse stdout JSON |
| 1 | `ExitError` | General/unknown error | Read stderr JSON for details |
| 2 | `ExitAuth` | Authentication failed (401/403) | Re-configure credentials |
| 3 | `ExitNotFound` | Resource not found (404) | Verify resource ID/key |
| 4 | `ExitValidation` | Invalid input (flags, body) | Fix request parameters |
| 5 | `ExitRateLimit` | Rate limited (429) | Wait and retry (Retry-After in error) |
| 6 | `ExitConflict` | Conflict (409) | Resolve conflict |
| 7 | `ExitServer` | Server error (5xx) | Retry later |

## Structured Error Output

All errors are JSON written to **stderr**:

```json
{
  "error": "auth_failed",
  "status": 401,
  "message": "Invalid API token",
  "hint": "Set JR_AUTH_TOKEN or run: jr configure --base-url URL --auth-type basic --username USER --token TOKEN",
  "request": {"method": "GET", "path": "/rest/api/3/myself"},
  "retry_after": null
}
```

Error codes map to exit codes. Agents can parse `error` field for programmatic handling.

## Schema / Discovery Command

Agents need to discover what commands and flags are available at runtime:

```bash
# List all resources
jr schema --list
# Output: ["issue","project","search","user","filter",...]

# List operations for a resource
jr schema issue
# Output: [{"verb":"get","method":"GET","path":"/rest/api/3/issue/{issueIdOrKey}","flags":[...]},...]

# Get full schema for a specific operation
jr schema issue get
# Output: {"verb":"get","method":"GET","path":"...","flags":[{"name":"issueIdOrKey","required":true,"type":"string","description":"..."},...],"has_body":true,"response_example":null}
```

This lets agents build correct commands without hardcoding knowledge of every endpoint.

## Built-in JQ Filtering (`--jq`)

Agents consume tokens proportional to output size. The `--jq` flag filters responses server-side (in the CLI, before writing to stdout) using [gojq](https://github.com/itchyny/gojq):

```bash
# Get only the summary and status
jr issue get --issueIdOrKey FOO-1 --jq '{key: .key, summary: .fields.summary, status: .fields.status.name}'

# Get just issue keys from search
jr search jql --jql "project = FOO" --jq '[.issues[].key]'
```

This dramatically reduces token waste for LLM agents.

## Authentication

### Priority Order (highest wins)

1. **CLI flags** — `--base-url`, `--auth-type`, `--auth-user`, `--auth-token`
2. **Environment variables** — `JR_BASE_URL`, `JR_AUTH_TYPE`, `JR_AUTH_USER`, `JR_AUTH_TOKEN`
3. **Config file profile** — `~/.config/jr/config.json`

### Env Vars (Preferred for Agents)

| Env Var | Required | Purpose |
|---------|----------|---------|
| `JR_BASE_URL` | Yes | `https://yoursite.atlassian.net` |
| `JR_AUTH_TYPE` | No (default: `basic`) | `basic`, `bearer`, `oauth2` |
| `JR_AUTH_USER` | For basic | Email address |
| `JR_AUTH_TOKEN` | Yes | API token (basic), PAT (bearer) |
| `JR_PROFILE` | No | Config file profile name |

Minimal agent setup:
```bash
export JR_BASE_URL=https://mysite.atlassian.net
export JR_AUTH_USER=agent@company.com
export JR_AUTH_TOKEN=ATATT3xFf...
jr issue get --issueIdOrKey FOO-1
```

### `jr configure` (Flag-Driven, No Prompts)

```bash
jr configure \
  --profile default \
  --base-url https://mysite.atlassian.net \
  --auth-type basic \
  --username agent@company.com \
  --token ATATT3xFf... \
  --test
```

The `--test` flag validates credentials via `GET /rest/api/3/myself` before saving.

All parameters are required as flags. No interactive prompts. If a required flag is missing, exit with code 4 and a structured error.

### Config File

Location: `~/.config/jr/config.json` (overridden by `JR_CONFIG_PATH` env var)

```json
{
  "profiles": {
    "default": {
      "base_url": "https://yoursite.atlassian.net",
      "auth": {
        "type": "basic",
        "username": "email@example.com",
        "token": "your-api-token"
      }
    }
  },
  "default_profile": "default"
}
```

### Auth Types

| Type | Fields | Use Case |
|------|--------|----------|
| `basic` | `username`, `token` | Jira Cloud (email + API token) |
| `bearer` | `token` | Bearer token auth |
| `oauth2` | `client_id`, `client_secret`, `token_url`, `scopes` | Jira Cloud OAuth 2.0 |

## Batch Mode

Agents often need to perform multiple operations. Instead of N separate process invocations, `jr batch` accepts a JSON array of operations:

```bash
jr batch --input operations.json
# Or from stdin:
cat operations.json | jr batch
```

Input format:
```json
[
  {"command": "issue get", "args": {"issueIdOrKey": "FOO-1"}, "jq": ".fields.summary"},
  {"command": "issue get", "args": {"issueIdOrKey": "FOO-2"}, "jq": ".fields.summary"},
  {"command": "project list", "args": {}}
]
```

Output format (JSON array, one result per operation, preserving order):
```json
[
  {"index": 0, "exit_code": 0, "data": "Fix login bug"},
  {"index": 1, "exit_code": 0, "data": "Add search feature"},
  {"index": 2, "exit_code": 0, "data": [{"key": "FOO", "name": "Foo Project"}]}
]
```

Failed operations include error info but don't abort the batch:
```json
[
  {"index": 0, "exit_code": 3, "error": {"error": "not_found", "status": 404, "message": "Issue not found"}}
]
```

## Stdin Body Support

Agents can pipe JSON directly as request body:

```bash
echo '{"fields":{"summary":"Bug","project":{"key":"FOO"},"issuetype":{"name":"Bug"}}}' | jr issue create
```

Priority: `--body` flag > stdin (if stdin is not a TTY and no `--body` flag).

## Pagination

**Auto-paginate by default.** Jira uses offset-based pagination (`startAt`, `maxResults`, `total`). The client automatically fetches all pages and merges results.

Disable with `--no-paginate` for raw single-page responses.

## Global Flags

| Flag | Env Var | Purpose |
|------|---------|---------|
| `-p, --profile` | `JR_PROFILE` | Select auth profile |
| `--base-url` | `JR_BASE_URL` | Override base URL |
| `--auth-type` | `JR_AUTH_TYPE` | Override auth type |
| `--auth-user` | `JR_AUTH_USER` | Override auth username |
| `--auth-token` | `JR_AUTH_TOKEN` | Override auth token |
| `--jq` | — | JQ filter expression applied to response |
| `--pretty` | — | Pretty-print JSON (for humans, off by default) |
| `--no-paginate` | — | Disable auto-pagination |
| `--dry-run` | `JR_DRY_RUN` | Print request as JSON without executing |
| `--verbose` | `JR_VERBOSE` | Print request/response details to stderr |

## Output Behavior

| Scenario | stdout | stderr |
|----------|--------|--------|
| Success | JSON response (filtered if `--jq`) | Nothing (or verbose info if `--verbose`) |
| Error | Nothing | Structured JSON error |
| `--dry-run` | JSON request description | Nothing |
| `--pretty` | Pretty-printed JSON | Same as above |

**Key:** stdout is always machine-parseable JSON. Humans who want pretty output use `--pretty` or pipe through `jq`.

## Code Generation Pipeline

### Overview

```
jira-v3.json (OpenAPI spec)
        │
        ▼
   gen/main.go (parser + code generator)
        │
        ▼
   cmd/generated/*.go (one file per resource group)
```

### Steps

1. **Spec acquisition**: Download the Jira OpenAPI v3 spec from `https://dac-static.atlassian.com/cloud/jira/platform/swagger-v3.v3.json` and store in `spec/jira-v3.json`. This is committed to the repo.

2. **Parse**: Use `github.com/pb33f/libopenapi` to parse the spec into a typed model.

3. **Group**: Iterate all paths, extract the resource group from the first path segment after `/rest/api/3/`. Build a map of `resource → []Operation`.

4. **Generate**: For each resource group, emit a Go file containing:
   - A cobra `Command` for the resource (e.g., `issueCmd`)
   - Sub-commands for each operation
   - Flags derived from:
     - Path parameters → required string flags
     - Query parameters → optional flags with types matching the spec
     - Request body → `--body` flag (accepts JSON string or `@filename`)
   - Help text from spec `summary` + `description`
   - A `Run` function that calls the HTTP client with the constructed path, method, query params, headers, and body

5. **Generate schema data**: For each operation, emit metadata (verb, method, path, flags, body schema) used by `jr schema` at runtime.

6. **Register**: Generate an `init.go` that adds all resource commands to the root command.

### Generated Code Template (per operation)

```go
var issueGetCmd = &cobra.Command{
    Use:   "get",
    Short: "Returns the details for an issue.",
    Long:  `Returns the details for an issue. The issue is identified by its ID or key.`,
    RunE: func(cmd *cobra.Command, args []string) error {
        c, err := client.FromContext(cmd.Context())
        if err != nil {
            return err
        }
        issueIdOrKey, _ := cmd.Flags().GetString("issueIdOrKey")
        path := fmt.Sprintf("/rest/api/3/issue/%s", url.PathEscape(issueIdOrKey))
        query := client.QueryFromFlags(cmd, "fields", "expand")
        return c.Do(cmd.Context(), "GET", path, query, nil)
    },
}

func init() {
    issueGetCmd.Flags().String("issueIdOrKey", "", "The ID or key of the issue. (required)")
    issueGetCmd.MarkFlagRequired("issueIdOrKey")
    issueGetCmd.Flags().String("fields", "", "Comma-separated list of fields to return.")
    issueGetCmd.Flags().String("expand", "", "Comma-separated list of properties to expand.")
    issueCmd.AddCommand(issueGetCmd)
}
```

## HTTP Client (`internal/client`)

### Design

A thin wrapper around `net/http` with no coupling to cobra:

```go
package client

type Client struct {
    BaseURL    string
    Auth       AuthConfig
    HTTPClient *http.Client
    JQFilter   string
    Paginate   bool
    DryRun     bool
    Verbose    bool
    Pretty     bool
}

// Do executes an API request. Handles auth, pagination, jq filtering, error formatting.
func (c *Client) Do(ctx context.Context, method, path string, query url.Values, body io.Reader) error

// FromContext retrieves the Client from the cobra command context.
func FromContext(ctx context.Context) (*Client, error)

// QueryFromFlags extracts named flags as url.Values (only includes flags that were set).
func QueryFromFlags(cmd *cobra.Command, names ...string) url.Values
```

### Responsibilities

- Construct full URL from base + path + query
- Attach auth headers based on config/env vars
- Execute request via `net/http`
- Auto-paginate GET responses with Jira pagination fields
- Apply `--jq` filter to response before output
- Write JSON to stdout (success) or structured error JSON to stderr (failure)
- Return appropriate exit code

### Data Flow (generated command → client)

1. Generated `RunE` reads its own flags (path params, query params, `--body`)
2. Constructs the URL path with path params substituted
3. Calls `QueryFromFlags(cmd, "fields", "expand", ...)` to build query values
4. Reads `--body` flag or stdin → `io.Reader`
5. Calls `c.Do(ctx, method, path, query, body)` — no cobra dependency in `Do`

### Libraries

- `net/http` — HTTP requests
- `github.com/itchyny/gojq` — JQ filtering
- `github.com/tidwall/pretty` — JSON pretty-printing (only when `--pretty`)

## Project Structure

```
jr/
├── cmd/
│   ├── root.go              # Root cobra command, global flags, client injection
│   ├── configure.go          # jr configure (flag-driven, no prompts)
│   ├── schema.go             # jr schema — command/flag discovery
│   ├── batch.go              # jr batch — multi-operation execution
│   ├── version.go            # jr version (JSON output)
│   ├── raw.go                # jr raw <method> <path>
│   └── generated/            # Auto-generated resource commands
│       ├── init.go           # Registers all resource commands
│       ├── schema_data.go    # Generated schema metadata for jr schema
│       ├── issue.go
│       ├── project.go
│       ├── search.go
│       ├── user.go
│       └── ...               # One file per resource group (~77 files)
├── gen/
│   ├── main.go               # Code generator entry point
│   ├── parser.go             # OpenAPI spec parsing via libopenapi
│   ├── grouper.go            # Path → resource grouping + verb naming
│   ├── generator.go          # Go code emission via text/template
│   └── templates/
│       ├── resource.go.tmpl
│       ├── schema_data.go.tmpl
│       └── init.go.tmpl
├── internal/
│   ├── client/
│   │   └── client.go         # HTTP client: Do(), auth, pagination, jq, error formatting
│   ├── config/
│   │   └── config.go         # Config loading (env vars > config file), profiles
│   ├── errors/
│   │   └── errors.go         # Structured error types, exit codes, JSON error output
│   └── jq/
│       └── jq.go             # JQ filter wrapper around gojq
├── spec/
│   └── jira-v3.json          # Jira OpenAPI v3 spec (committed)
├── main.go                   # Entry point
├── go.mod
├── go.sum
├── Makefile
└── .goreleaser.yml           # Release config (optional)
```

## Build & Development

### Makefile Targets

| Target | Command | Purpose |
|--------|---------|---------|
| `make generate` | `go run ./gen/...` | Regenerate commands from spec |
| `make build` | `go build -o jr .` | Build binary |
| `make install` | `go install .` | Install to GOPATH/bin |
| `make test` | `go test ./...` | Run tests |
| `make spec-update` | `curl ... > spec/jira-v3.json` | Download latest spec |

### Development Workflow

1. `make spec-update` — fetch latest Jira OpenAPI spec
2. `make generate` — regenerate commands
3. `make build` — build binary
4. `export JR_BASE_URL=... JR_AUTH_USER=... JR_AUTH_TOKEN=...`
5. `./jr issue get --issueIdOrKey FOO-123`

## Testing Strategy

### Unit Tests
- `gen/` — test parser, grouper, generator with small OpenAPI spec fixtures
- `internal/client/` — test URL construction, auth, pagination, jq filtering, error formatting
- `internal/config/` — test env var priority, config load/save, profile selection
- `internal/errors/` — test structured error formatting, exit code mapping
- `internal/jq/` — test jq filter expressions

### Integration Tests
- Generated commands: verify flag parsing, URL construction, help text
- End-to-end: mock HTTP server returning canned responses, run commands, verify JSON output
- Schema command: verify discovery output matches generated commands
- Batch command: verify multi-operation execution and error handling

### Generator Tests
- Golden file tests: generate from a fixture spec, compare output to committed golden files
- Roundtrip: generate → compile → `--help` works for all commands

## Error Handling

All errors are structured JSON to stderr with appropriate exit codes.

| HTTP Status | Exit Code | Error Type | Agent Action |
|-------------|-----------|------------|-------------|
| 401, 403 | 2 | `auth_failed` | Reconfigure credentials |
| 404 | 3 | `not_found` | Check resource ID |
| 400, 422 | 4 | `validation_error` | Fix request |
| 429 | 5 | `rate_limited` | Wait `retry_after` seconds |
| 409 | 6 | `conflict` | Resolve conflict |
| 5xx | 7 | `server_error` | Retry later |
| Network | 1 | `connection_error` | Check URL/network |
| Bad flags | 4 | `invalid_input` | Fix flags |
| Bad JSON body | 4 | `invalid_body` | Fix JSON |

## Future Considerations (Not in v1)

- Jira Server/Data Center support (API v2 path rewriting, separate spec)
- Jira Software (Agile) API support (boards, sprints — separate OpenAPI spec)
- Aliases (`jr i` for `jr issue`)
- Webhook listener mode (agent receives Jira events)
- Response caching with TTL
- OpenAI/Anthropic tool-use JSON schema generation from `jr schema`
- Plugin system for custom commands
- Spec diffing tool to detect API changes
