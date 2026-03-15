# jr — Jira CLI built for AI agents

[![CI](https://github.com/sofq/jira-cli/actions/workflows/ci.yml/badge.svg)](https://github.com/sofq/jira-cli/actions/workflows/ci.yml)
[![Release](https://github.com/sofq/jira-cli/actions/workflows/release.yml/badge.svg)](https://github.com/sofq/jira-cli/releases)
[![License: MIT](https://img.shields.io/badge/License-MIT-blue.svg)](LICENSE)

**jr** gives AI agents (Claude Code, Cursor, Copilot, custom agents) reliable access to Jira. Every command returns structured JSON on stdout, errors as JSON on stderr, and semantic exit codes — so agents can parse, branch, and retry without guessing.

```bash
# An agent can search, filter, and get exactly the tokens it needs
jr search jql --jql "project = PROJ AND status = 'In Progress'" \
  --fields key,summary,status \
  --jq '[.issues[] | {key, summary: .fields.summary, status: .fields.status.name}]'
```

## Why another Jira CLI?

Most CLIs are built for humans — they print tables, prompt for input, and bury errors in prose. Agents need something different:

| What agents need | What jr does |
|-----------------|-------------|
| Parseable output | Pure JSON on stdout, always |
| Machine-readable errors | Structured JSON on stderr with `error_type`, `status`, `retry_after` |
| Predictable exit codes | 0-7, each mapped to a specific failure class |
| Token efficiency | `--jq` and `--fields` to minimize response size |
| No interactivity | Flag-driven, zero prompts, stdin/stdout clean |
| Batch operations | `jr batch` executes N operations in one process |
| Self-description | `jr schema` lets agents discover commands at runtime |

## Install

```bash
# Homebrew
brew install sofq/tap/jr

# Go
go install github.com/sofq/jira-cli@latest

# Binary
# → https://github.com/sofq/jira-cli/releases
```

## Quick start

```bash
# 1. Configure (flag-driven, no prompts)
jr configure --base-url https://yourorg.atlassian.net --token YOUR_API_TOKEN

# 2. Get an issue
jr issue get --issueIdOrKey PROJ-123

# 3. Filter to just what you need (saves agent context tokens)
jr issue get --issueIdOrKey PROJ-123 --fields key,summary,status --jq '{key: .key, summary: .fields.summary}'
```

## Agent integration

### For Claude Code / Cursor / AI coding agents

Drop a `CLAUDE.md` (included in this repo) or add to your agent's instructions:

```
Use `jr` for all Jira operations. Output is always JSON.
Use `jr schema --compact` to discover available commands.
Use --jq to filter responses and reduce token usage.
```

### Discover commands at runtime

Agents don't need hardcoded command lists — they can discover everything:

```bash
jr schema --list              # all resource names
jr schema --compact           # resource → verbs mapping (token-efficient)
jr schema issue               # all operations for 'issue' with flags
jr schema issue get           # full schema for one operation
```

### Batch operations (one process, N calls)

Agents can combine multiple operations to reduce subprocess overhead:

```bash
echo '[
  {"command": "issue get", "args": {"issueIdOrKey": "PROJ-1"}, "jq": ".key"},
  {"command": "issue get", "args": {"issueIdOrKey": "PROJ-2"}, "jq": ".key"},
  {"command": "project list", "args": {}, "jq": "[.values[].key]"}
]' | jr batch
# → [{"index":0,"exit_code":0,"data":"PROJ-1"}, ...]
```

### Workflow commands (agents don't need to know Jira internals)

High-level commands that resolve IDs automatically — agents don't need to look up transition IDs or account IDs:

```bash
# Transition by status name (resolves transition ID automatically)
jr workflow transition --issue PROJ-123 --to "Done"

# Assign by name or "me" (resolves account ID automatically)
jr workflow assign --issue PROJ-123 --to "me"
jr workflow assign --issue PROJ-123 --to "john.doe@company.com"
```

### Error handling for agents

Every error is a JSON object on stderr with a typed `error_type` agents can branch on:

```json
{
  "error_type": "rate_limited",
  "status": 429,
  "message": "...",
  "hint": "You are being rate limited. Wait before retrying.",
  "retry_after": 30,
  "request": {"method": "GET", "path": "/rest/api/3/issue/X-1"}
}
```

| Exit code | Meaning | Agent action |
|-----------|---------|-------------|
| 0 | Success | Parse stdout |
| 1 | General error | Log and report |
| 2 | Auth failed | Re-authenticate |
| 3 | Not found | Check issue key |
| 4 | Validation error | Fix request body |
| 5 | Rate limited | Wait `retry_after` seconds |
| 6 | Conflict | Retry or resolve |
| 7 | Server error | Retry with backoff |

### Token efficiency

Jira responses can be 10K+ tokens for a single issue. jr helps agents stay within context budgets:

```bash
# Full response: ~10,000 tokens
jr issue get --issueIdOrKey PROJ-123

# With --fields: ~500 tokens
jr issue get --issueIdOrKey PROJ-123 --fields key,summary,status,assignee

# With --fields + --jq: ~50 tokens
jr issue get --issueIdOrKey PROJ-123 --fields key,summary --jq '{key: .key, summary: .fields.summary}'

# Cache read-heavy data to avoid redundant API calls
jr project list --cache 5m --jq '[.values[].key]'
```

## 600+ commands, auto-generated

Commands are generated from the [official Jira OpenAPI v3 spec](https://developer.atlassian.com/cloud/jira/platform/rest/v3/intro/). A weekly CI job detects spec drift and opens PRs automatically — so jr stays in sync with Jira's API.

```bash
jr issue get --issueIdOrKey PROJ-1
jr issue create-issue --body '{"fields":{...}}'
jr project list
jr search jql --jql "assignee = currentUser()"
jr raw GET /rest/api/3/myself    # escape hatch for any endpoint
```

## Configuration

Resolved in priority order: **CLI flags > env vars > config file**

```bash
# Env vars (great for CI/agents)
export JR_BASE_URL=https://yourorg.atlassian.net
export JR_AUTH_TOKEN=your-api-token

# Config file (supports named profiles)
jr configure --base-url https://work.atlassian.net --token TOKEN --profile work
jr issue get --profile work --issueIdOrKey PROJ-1

# Auth types: basic (default), bearer, oauth2
jr configure --base-url https://yourorg.atlassian.net --auth-type bearer --token TOKEN
```

## Global flags

| Flag | Description |
|------|-------------|
| `--jq <expr>` | jq filter applied to response |
| `--fields <list>` | Comma-separated fields to return (GET only) |
| `--cache <duration>` | Cache GET responses (e.g. `5m`, `1h`) |
| `--pretty` | Pretty-print JSON output |
| `--no-paginate` | Disable automatic pagination |
| `--dry-run` | Print request as JSON without executing |
| `--verbose` | Log HTTP details to stderr as JSON |
| `--profile <name>` | Use a named config profile |

## Development

```bash
make generate    # regenerate commands from OpenAPI spec
make build       # build binary
make test        # run all tests (unit + conformance + e2e)
make lint        # run golangci-lint
make spec-update # fetch latest Jira OpenAPI spec
```

## License

[MIT](LICENSE)
