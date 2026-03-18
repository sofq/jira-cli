# jr — Jira CLI built for AI agents

[![CI](https://github.com/sofq/jira-cli/actions/workflows/ci.yml/badge.svg)](https://github.com/sofq/jira-cli/actions/workflows/ci.yml)
[![codecov](https://codecov.io/gh/sofq/jira-cli/graph/badge.svg)](https://codecov.io/gh/sofq/jira-cli)
[![Security](https://github.com/sofq/jira-cli/actions/workflows/security.yml/badge.svg)](https://github.com/sofq/jira-cli/actions/workflows/security.yml)
[![Release](https://github.com/sofq/jira-cli/actions/workflows/release.yml/badge.svg)](https://github.com/sofq/jira-cli/releases)
[![License: Apache 2.0](https://img.shields.io/badge/License-Apache_2.0-blue.svg)](LICENSE)

**jr** gives AI agents (Claude Code, Cursor, Copilot, custom agents) reliable access to Jira. Every command returns structured JSON on stdout, errors as JSON on stderr, and semantic exit codes — so agents can parse, branch, and retry without guessing.

```bash
# An agent can search, filter, and get exactly the tokens it needs
jr search search-and-reconsile-issues-using-jql \
  --jql "project = PROJ AND status = 'In Progress'" \
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
| Token efficiency | `--preset`, `--jq`, and `--fields` to minimize response size |
| No interactivity | Flag-driven, zero prompts, stdin/stdout clean |
| Batch operations | `jr batch` executes N operations in one process |
| Self-description | `jr schema` lets agents discover commands at runtime |

## Install

### Homebrew

```bash
brew install sofq/tap/jr
```

### npm

```bash
npm install -g jira-jr
```

### pip / uv

```bash
pip install jira-jr
# or
uv tool install jira-jr
```

### Scoop (Windows)

```bash
scoop bucket add sofq https://github.com/sofq/scoop-bucket
scoop install jr
```

### Go

```bash
go install github.com/sofq/jira-cli@latest
```

### Docker

```bash
docker run --rm ghcr.io/sofq/jr version
```

### Binary

Download from [GitHub Releases](https://github.com/sofq/jira-cli/releases).

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

### Claude Code skill

A [Claude Code skill](skill/jira-cli/SKILL.md) is included in this repo. Copy it to your project or global skills directory to give Claude Code full knowledge of `jr` commands, patterns, and troubleshooting:

```bash
# Project-level
cp -r skill/jira-cli .claude/skills/

# Global (all projects)
cp -r skill/jira-cli ~/.claude/skills/
```

### For Claude Code / Cursor / AI coding agents

Or simply add to your agent's instructions:

```
Use `jr` for all Jira operations. Output is always JSON.
Use `jr schema` to discover available commands.
Use --jq to filter responses and reduce token usage.
```

### Discover commands at runtime

Agents don't need hardcoded command lists — they can discover everything:

```bash
jr schema                     # resource → verbs mapping (default, token-efficient)
jr schema --list              # all resource names only
jr schema issue               # all operations for 'issue' with flags
jr schema issue get           # full schema for one operation
```

### Batch operations (one process, N calls)

Agents can combine multiple operations to reduce subprocess overhead:

```bash
echo '[
  {"command": "issue get", "args": {"issueIdOrKey": "PROJ-1"}, "jq": ".key"},
  {"command": "issue get", "args": {"issueIdOrKey": "PROJ-2"}, "jq": ".key"},
  {"command": "project search", "args": {}, "jq": "[.values[].key]"}
]' | jr batch
# → [{"index":0,"exit_code":0,"data":"PROJ-1"}, ...]
```

### Workflow commands (agents don't need to know Jira internals)

High-level commands that resolve IDs automatically — agents don't need to look up transition IDs, account IDs, sprint IDs, or link type IDs:

```bash
# Transition by status name (resolves transition ID automatically)
jr workflow transition --issue PROJ-123 --to "Done"

# Assign by name or "me" (resolves account ID automatically)
jr workflow assign --issue PROJ-123 --to "me"

# Transition + assign in one step
jr workflow move --issue PROJ-123 --to "In Progress" --assign me

# Add a plain-text comment (auto-converted to ADF)
jr workflow comment --issue PROJ-123 --text "Fixed in latest deploy"

# Create issue from flags (no raw JSON needed)
jr workflow create --project PROJ --type Bug --summary "Login broken" --priority High

# Link issues by type name
jr workflow link --from PROJ-1 --to PROJ-2 --type blocks

# Log work with human-friendly duration
jr workflow log-work --issue PROJ-123 --time "2h 30m" --comment "Debugging"

# Move issue to sprint by name
jr workflow sprint --issue PROJ-123 --to "Sprint 5"
```

### Watch for changes (autonomous agents)

Stream Jira changes as NDJSON for autonomous monitoring agents:

```bash
# Poll a JQL query every 30s, emit change events
jr watch --jql "project = PROJ AND updated > -5m" --interval 30s

# Watch a single issue
jr watch --issue PROJ-123 --interval 10s

# Stop after 10 events, use a preset for output shaping
jr watch --jql "assignee = currentUser()" --max-events 10 --preset triage
```

Events: `initial` (first poll), `created`, `updated`, `removed`. Graceful shutdown on Ctrl-C.

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

# With --preset: agent-friendly field set
jr issue get --issueIdOrKey PROJ-123 --preset agent

# With --fields: ~500 tokens
jr issue get --issueIdOrKey PROJ-123 --fields key,summary,status,assignee

# With --fields + --jq: ~50 tokens
jr issue get --issueIdOrKey PROJ-123 --fields key,summary --jq '{key: .key, summary: .fields.summary}'

# Cache read-heavy data to avoid redundant API calls
jr project search --cache 5m --jq '[.values[].key]'
```

## 600+ commands, auto-generated

Commands are generated from the [official Jira OpenAPI v3 spec](https://developer.atlassian.com/cloud/jira/platform/rest/v3/intro/). A daily CI job detects spec drift and opens PRs automatically — so jr stays in sync with Jira's API.

```bash
jr issue get --issueIdOrKey PROJ-1
jr issue create-issue --body '{"fields":{...}}'
jr project search --jq '[.values[] | {key, name}]'
jr search search-and-reconsile-issues-using-jql --jql "assignee = currentUser()"
jr raw GET /rest/api/3/myself              # escape hatch for any endpoint
jr raw POST /path --body '{"key":"val"}'  # POST/PUT/PATCH require --body
echo '{}' | jr raw POST /path --body -    # read body from stdin with --body -
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
jr configure --profile work --delete   # remove a profile

# Auth types: basic (default), bearer, oauth2
jr configure --base-url https://yourorg.atlassian.net --auth-type bearer --token TOKEN
```

## Global flags

| Flag | Description |
|------|-------------|
| `--preset <name>` | Named output preset (`agent`, `detail`, `triage`, `board`) |
| `--jq <expr>` | jq filter applied to response |
| `--fields <list>` | Comma-separated fields to return (GET only) |
| `--cache <duration>` | Cache GET responses (e.g. `5m`, `1h`) |
| `--pretty` | Pretty-print JSON output |
| `--no-paginate` | Disable automatic pagination |
| `--dry-run` | Print request as JSON without executing |
| `--verbose` | Log HTTP details to stderr as JSON |
| `--timeout <duration>` | HTTP request timeout (default `30s`) |
| `--profile <name>` | Use a named config profile |

## Development

```bash
make generate    # regenerate commands from OpenAPI spec
make build       # build binary
make test        # run all tests (unit + conformance + e2e)
make lint        # run golangci-lint
make spec-update # fetch latest Jira OpenAPI spec
```

## Acknowledgments

jr is built on these excellent libraries:

- [cobra](https://github.com/spf13/cobra) — CLI framework
- [libopenapi](https://github.com/pb33f/libopenapi) — OpenAPI spec parsing that powers auto-generated commands
- [gojq](https://github.com/itchyny/gojq) — Pure Go jq implementation for the `--jq` flag
- [Restish](https://github.com/rest-sh/restish) — Inspiration for the "CLI generated from OpenAPI" approach

## License

[Apache 2.0](LICENSE)
