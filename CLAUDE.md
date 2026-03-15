# jr — Agent-Friendly Jira CLI

## Quick Start
```
jr configure --base-url https://yoursite.atlassian.net --token YOUR_API_TOKEN --username your@email.com
```

## Key Patterns

- **All output is JSON** on stdout. Errors are JSON on stderr.
- **Exit codes are semantic**: 0=ok, 1=error, 2=auth, 3=not_found, 4=validation, 5=rate_limited, 6=conflict, 7=server
- **Use `--jq`** to reduce output tokens: `jr issue get --issueIdOrKey PROJ-1 --jq '{key: .key, summary: .fields.summary}'`
- **Use `--fields`** to limit Jira response fields: `jr issue get --issueIdOrKey PROJ-1 --fields key,summary,status`
- **Use `jr batch`** to run multiple operations in one call
- **Use `jr raw`** for any API endpoint not covered by generated commands

## Common Operations

```bash
# Get issue
jr issue get --issueIdOrKey PROJ-123

# Search issues (use the new JQL endpoint — the old /search is deprecated)
jr search search-and-reconsile-issues-using-jql --jql "project = PROJ AND status = 'In Progress'" --jq '[.issues[] | {key, summary: .fields.summary}]'

# Create issue
jr issue create-issue --body '{"fields":{"project":{"key":"PROJ"},"summary":"Bug title","issuetype":{"name":"Bug"}}}'

# Transition issue
jr workflow transition --issue PROJ-123 --to "Done"

# Assign issue
jr workflow assign --issue PROJ-123 --to "me"

# List projects
jr project search --jq '[.values[] | {key, name}]'

# Batch operations
echo '[{"command":"issue get","args":{"issueIdOrKey":"PROJ-1"},"jq":".key"},{"command":"issue get","args":{"issueIdOrKey":"PROJ-2"},"jq":".key"}]' | jr batch
```

## Discovery

```bash
jr schema --list              # all resource names
jr schema --compact           # resource → verbs mapping
jr schema issue               # all operations for 'issue'
jr schema issue get           # full schema with flags for one operation
```

## Global Flags

| Flag | Description |
|------|-------------|
| `--jq <expr>` | jq filter on response |
| `--fields <list>` | comma-separated fields to return (GET only) |
| `--cache <duration>` | cache GET responses (e.g. 5m, 1h) |
| `--pretty` | pretty-print JSON |
| `--no-paginate` | disable auto-pagination |
| `--dry-run` | show request without executing |
| `--verbose` | log HTTP details to stderr (JSON) |
| `--profile <name>` | use named config profile |

## Development

```bash
make generate    # regenerate commands from OpenAPI spec
make build       # build binary
make test        # run tests
make lint        # run golangci-lint
```

## Pre-PR Checklist (MUST pass before creating a PR)

Run these locally — CI failures waste time and slow down iteration.

```bash
# 1. Build must succeed
go build ./...

# 2. All tests must pass (unit + e2e)
go test ./...

# 3. Linter must pass (this is what usually fails in CI)
golangci-lint run

# 4. If you touched generated code or templates, regenerate and verify
make generate
go test ./gen/...   # conformance test catches stale generated code
```

### Common lint issues to watch for
- **errcheck**: Every function that returns an error must have its return value handled. Use `_ =` for intentional ignores (e.g. `_ = json.Unmarshal(...)`). Check `.golangci.yml` for excluded functions.
- **Do NOT add new exclude-functions to `.golangci.yml`** unless the function is truly fire-and-forget (like `fmt.Fprintf` to a Writer).
- If `golangci-lint` is not installed: `go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest`
