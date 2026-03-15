# jr — Jira CLI Tool Design Spec

**Date:** 2026-03-15
**Status:** Approved

## Overview

`jr` is a Go CLI tool that provides full Jira REST API coverage via auto-generated commands. It uses [cobra](https://github.com/spf13/cobra) for command structure and `net/http` for API calls. Commands are generated from the official Jira Cloud OpenAPI v3 spec. The design is inspired by [restish](https://github.com/rest-sh/restish) (output formatting, CLI ergonomics) but does not import restish as a library, since restish is a standalone CLI framework with global state — not an embeddable HTTP client.

## Goals

1. Full coverage of all Jira Cloud REST API v3 endpoints
2. Resource-verb command structure: `jr <resource> <verb> [flags]`
3. Auto-generate commands from OpenAPI spec — no manual command authoring
4. Rich terminal output (colorized JSON, table format)
5. Configurable base URL and auth (Cloud API tokens, bearer tokens, OAuth 2.0)

## Non-Goals

- GUI or TUI interactive modes
- Jira-specific business logic (e.g., workflow wizards)
- Plugin system (for v1)
- Jira Server/Data Center support (v1 targets Cloud only; Server/DC uses `/rest/api/2/` with different schemas — deferred to v2)
- Jira Software (Agile) API (`/rest/agile/1.0/`) — boards, sprints, etc. are a separate spec

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
| `jr configure` | Interactive auth/base-URL setup |
| `jr version` | Print version |
| `jr completion` | Shell completion (bash, zsh, fish, powershell) |
| `jr raw <method> <path>` | Escape hatch for raw API calls |

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

5. **Register**: Generate an `init.go` that adds all resource commands to the root command.

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
    // query params
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
    Output     *output.Formatter
}

// Do executes an API request. params are decoupled from cobra flags.
func (c *Client) Do(ctx context.Context, method, path string, query url.Values, body io.Reader) error

// FromContext retrieves the Client from the cobra command context (set in root PersistentPreRunE).
func FromContext(ctx context.Context) (*Client, error)

// QueryFromFlags is a helper used in generated code to extract named flags as url.Values.
func QueryFromFlags(cmd *cobra.Command, names ...string) url.Values
```

### Responsibilities

- Construct full URL from base + path + query
- Attach auth headers based on config
- Execute request via `net/http`
- Pass response to `output.Formatter` for display
- Handle pagination (follow `next` links if present)

### Data Flow (generated command → client)

1. Generated `RunE` reads its own flags (path params, query params, `--body`)
2. Constructs the URL path with path params substituted
3. Calls `QueryFromFlags(cmd, "fields", "expand", ...)` to build query values
4. Reads `--body` flag → `io.Reader` (JSON string or `@filename`)
5. Calls `c.Do(ctx, method, path, query, body)` — no cobra dependency in `Do`

### Libraries

- `net/http` — HTTP requests
- `github.com/tidwall/pretty` — JSON colorization
- `github.com/olekukonko/tablewriter` — table output

## Configuration (`internal/config`)

### Config File

Location: `~/.config/jr/config.json`

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
    },
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

### `jr configure` Flow

1. Prompt for profile name (default: "default")
2. Prompt for base URL
3. Prompt for auth type (basic/bearer/oauth2)
4. Prompt for auth credentials
5. Test connection with `GET /rest/api/3/myself`
6. Save to config file

### Global Flags

| Flag | Env Var | Purpose |
|------|---------|---------|
| `-p, --profile` | `JR_PROFILE` | Select auth profile |
| `-o, --output` | `JR_OUTPUT` | Output format: auto (default), json, table, raw |
| `--verbose` | `JR_VERBOSE` | Print request/response details |
| `--dry-run` | `JR_DRY_RUN` | Print request without executing |

## Output Formatting (`internal/output`)

### Modes

| Mode | When | Behavior |
|------|------|----------|
| `auto` | Default | Colorized JSON in TTY, raw JSON when piped |
| `json` | `-o json` | Pretty-printed JSON |
| `table` | `-o table` | Tabular output for list responses |
| `raw` | `-o raw` | Unformatted response body |

### Table Output

For list endpoints, extract array items and display key fields as columns. The generator can embed default column selections per resource in the generated code (e.g., issue list → key, summary, status, assignee).

## Project Structure

```
jr/
├── cmd/
│   ├── root.go              # Root cobra command, global flags
│   ├── configure.go          # jr configure
│   ├── version.go            # jr version
│   ├── raw.go                # jr raw <method> <path>
│   └── generated/            # Auto-generated resource commands
│       ├── init.go           # Registers all resource commands
│       ├── issue.go
│       ├── project.go
│       ├── search.go
│       ├── user.go
│       └── ...               # One file per resource group
├── gen/
│   ├── main.go               # Code generator entry point
│   ├── parser.go             # OpenAPI spec parsing
│   ├── grouper.go            # Path → resource grouping logic
│   ├── generator.go          # Go code emission
│   └── templates/            # Go text/template files for code gen
│       ├── resource.go.tmpl
│       └── init.go.tmpl
├── internal/
│   ├── client/
│   │   ├── client.go         # HTTP client wrapper
│   │   ├── auth.go           # Auth header injection
│   │   └── pagination.go     # Pagination handling
│   ├── config/
│   │   ├── config.go         # Config loading/saving
│   │   └── profile.go        # Profile management
│   └── output/
│       ├── formatter.go      # Output mode dispatch
│       ├── json.go           # JSON formatting
│       └── table.go          # Table formatting
├── spec/
│   └── jira-v3.json          # Jira OpenAPI v3 spec (committed)
├── main.go                   # Entry point
├── go.mod
├── go.sum
├── Makefile                  # build, generate, test targets
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
4. `./jr configure` — set up auth
5. `./jr issue get --issueIdOrKey FOO-123` — test

## Testing Strategy

### Unit Tests
- `gen/` — test parser, grouper, generator with small OpenAPI spec fixtures
- `internal/client/` — test URL construction, auth header injection, pagination
- `internal/config/` — test config load/save, profile selection
- `internal/output/` — test JSON and table formatting

### Integration Tests
- Generated commands: verify flag parsing, URL construction, help text
- End-to-end: mock HTTP server returning canned responses, run commands, verify output

### Generator Tests
- Golden file tests: generate from a fixture spec, compare output to committed golden files
- Roundtrip: generate → compile → `--help` works for all commands

## Error Handling

- HTTP errors: print status code, Jira error message body, and suggestion
- Auth errors (401/403): suggest `jr configure`
- Network errors: print connection error with base URL
- Invalid flags: cobra's built-in validation + required flag checking
- Invalid JSON body: parse and report error before sending request

## Future Considerations (Not in v1)

- Jira Server/Data Center support (API v2 path rewriting, separate spec)
- Jira Software (Agile) API support (boards, sprints — separate OpenAPI spec)
- Aliases (`jr i` for `jr issue`)
- Interactive mode / fuzzy finding for issue keys
- Bulk operations
- Output templates (Go template strings)
- Plugin system for custom commands
- Spec diffing tool to detect API changes
