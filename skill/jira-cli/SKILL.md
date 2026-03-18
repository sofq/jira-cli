---
name: jira-cli
description: "How to use `jr`, the agent-friendly Jira CLI, to interact with Jira Cloud. Use this skill whenever the user asks to work with Jira issues, projects, sprints, boards, or any Jira operation — searching issues, creating tickets, transitioning statuses, assigning work, querying project data, or automating Jira workflows. Also use when you see `jr` commands in the codebase or the user mentions Jira in the context of CLI tooling. Even if the user just says 'check my Jira tickets' or 'move this to Done', this skill applies."
---

# jr — Jira CLI for AI Agents

`jr` is a Jira Cloud CLI designed for AI agents. Every command returns structured JSON on stdout, errors as JSON on stderr, and semantic exit codes — so you can parse, branch, and retry reliably.

## Setup

If `jr` is not configured yet, help the user set it up:

```bash
# Check if jr is installed
jr version

# Configure with Jira Cloud credentials
jr configure --base-url https://yoursite.atlassian.net --token YOUR_API_TOKEN --username your@email.com
```

**Auth types:** `basic` (default, username + API token), `bearer`, `oauth2`

**Config resolution order:** CLI flags > environment variables > config file

```bash
# Environment variables (useful for CI/containers)
export JR_BASE_URL=https://yoursite.atlassian.net
export JR_AUTH_TOKEN=your-api-token

# Named profiles for multiple Jira instances
jr configure --base-url https://work.atlassian.net --token TOKEN --profile work
jr issue get --profile work --issueIdOrKey PROJ-1
jr configure --profile work --delete   # remove a profile
```

If you get exit code 2 (auth error), the token is likely expired or wrong. Ask the user to generate a new API token at https://id.atlassian.com/manage-profile/security/api-tokens.

## Discovering Commands

`jr` has 600+ commands auto-generated from Jira's OpenAPI spec. You don't need to memorize them — discover at runtime:

```bash
jr schema                     # resource → verbs mapping (default, most useful overview)
jr schema --list              # all resource names only (issue, project, search, ...)
jr schema issue               # all operations for a resource
jr schema issue get           # full schema with all flags for one operation
```

Always use `jr schema` to discover the exact command name and flags before running an unfamiliar operation. Command names come directly from Jira's API and can be verbose (e.g., `search-and-reconsile-issues-using-jql`).

`jr schema` (no flags) defaults to the compact resource→verbs mapping, which is the most useful starting point.

## Common Operations

### Get an issue
```bash
jr issue get --issueIdOrKey PROJ-123
```

### Search issues with JQL
```bash
# Use the search resource (not the deprecated /search endpoint)
jr search search-and-reconsile-issues-using-jql \
  --jql "project = PROJ AND status = 'In Progress'" \
  --jq '[.issues[] | {key, summary: .fields.summary, status: .fields.status.name}]'
```

### Create an issue
```bash
# From flags (preferred — no raw JSON needed)
jr workflow create --project PROJ --type Bug --summary "Login broken" --priority High --labels bug,urgent

# From raw JSON (for advanced fields)
jr issue create-issue --body '{"fields":{"project":{"key":"PROJ"},"summary":"Bug title","issuetype":{"name":"Bug"}}}'
```

### Transition an issue (by status name)
```bash
# No need to look up transition IDs — jr resolves them automatically
jr workflow transition --issue PROJ-123 --to "Done"
jr workflow transition --issue PROJ-123 --to "In Progress"
```

### Transition + assign in one step
```bash
jr workflow move --issue PROJ-123 --to "In Progress" --assign me
```

### Assign an issue
```bash
# "me" resolves to the authenticated user's account ID
jr workflow assign --issue PROJ-123 --to "me"
# Also accepts email or display name
jr workflow assign --issue PROJ-123 --to "john@company.com"
# Unassign
jr workflow assign --issue PROJ-123 --to "none"
```

### Add a comment
```bash
# Plain text, auto-converted to ADF
jr workflow comment --issue PROJ-123 --text "This is done"
```

### Link issues
```bash
# Resolves link type ID automatically
jr workflow link --from PROJ-1 --to PROJ-2 --type blocks
```

### Log work
```bash
# Human-friendly duration (e.g. 2h, 1d 3h, 45m)
jr workflow log-work --issue PROJ-123 --time "2h 30m" --comment "Debugging"
```

### Move to sprint
```bash
# Resolves sprint ID by name automatically
jr workflow sprint --issue PROJ-123 --to "Sprint 5"
```

### List projects
```bash
jr project search --jq '[.values[] | {key, name}]'
```

### Raw API call (escape hatch)
```bash
# For any endpoint not covered by generated commands
jr raw GET /rest/api/3/myself
jr raw POST /rest/api/3/some/endpoint --body '{"key":"value"}'
jr raw POST /rest/api/3/some/endpoint --body @request.json
# Read body from stdin (must use --body - explicitly)
echo '{"key":"value"}' | jr raw POST /rest/api/3/some/endpoint --body -
```

**Note:** POST/PUT/PATCH require `--body`. Without it, `jr raw` will error instead of hanging on stdin.

## Token Efficiency

Jira responses can be huge (10K+ tokens for a single issue). Always minimize output:

```bash
# --preset: use a named preset for common field combinations
jr issue get --issueIdOrKey PROJ-123 --preset agent    # key, summary, status, assignee, type, priority
jr issue get --issueIdOrKey PROJ-123 --preset detail   # above + description, comments, subtasks, links
jr issue get --issueIdOrKey PROJ-123 --preset triage   # key, summary, status, priority, created, updated, reporter
jr issue get --issueIdOrKey PROJ-123 --preset board    # key, summary, status, assignee, sprint, story points, type

# List all available presets
jr preset list

# --fields: tell Jira to return only these fields (server-side filtering)
jr issue get --issueIdOrKey PROJ-123 --fields key,summary,status,assignee

# --jq: filter the JSON response (client-side filtering)
jr issue get --issueIdOrKey PROJ-123 --jq '{key: .key, summary: .fields.summary}'

# Combine both for maximum efficiency (~50 tokens vs ~10,000)
jr issue get --issueIdOrKey PROJ-123 --fields key,summary --jq '{key: .key, summary: .fields.summary}'

# Cache read-heavy data to avoid redundant API calls
jr project search --cache 5m --jq '[.values[].key]'
```

**Always use `--preset` or `--fields` + `--jq`.** `--preset` gives you common field sets with zero effort. `--fields` reduces what Jira sends back, `--jq` shapes the output into exactly what you need.

## Batch Operations

When you need multiple Jira calls, use `jr batch` to run them in a single process:

```bash
echo '[
  {"command": "issue get", "args": {"issueIdOrKey": "PROJ-1"}, "jq": ".key"},
  {"command": "issue get", "args": {"issueIdOrKey": "PROJ-2"}, "jq": ".fields.summary"},
  {"command": "project search", "args": {}, "jq": "[.values[].key]"}
]' | jr batch
```

Output is an array of results, each with `index`, `exit_code`, and `data` (or `error`). This avoids spawning N subprocesses.

## Error Handling

Errors are structured JSON on stderr. Branch on `exit_code` and `error_type`:

| Exit code | error_type | Meaning | What to do |
|-----------|-----------|---------|------------|
| 0 | — | Success | Parse stdout as JSON |
| 1 | — | General error | Log and report to user |
| 2 | `auth_failed` | Auth failed (401/403) | Check token/credentials |
| 3 | `not_found` | Resource not found (404/410) | Verify issue key / resource ID |
| 4 | `validation_error` | Bad request (400/422/4xx) | Fix the request payload |
| 5 | `rate_limited` | Rate limited (429) | Wait `retry_after` seconds, then retry |
| 6 | `conflict` | Conflict (409) | Retry or resolve conflict |
| 7 | `server_error` | Server error (5xx) | Retry with backoff |

Example error output:
```json
{
  "error_type": "not_found",
  "status": 404,
  "message": "Issue Does Not Exist",
  "request": {"method": "GET", "path": "/rest/api/3/issue/BAD-999"}
}
```

## Global Flags

| Flag | Description |
|------|-------------|
| `--preset <name>` | named output preset (agent, detail, triage, board) |
| `--jq <expr>` | jq filter on response |
| `--fields <list>` | comma-separated fields to return (GET only) |
| `--cache <duration>` | cache GET responses (e.g. `5m`, `1h`) |
| `--pretty` | pretty-print JSON output |
| `--no-paginate` | disable automatic pagination |
| `--dry-run` | show the request without executing it |
| `--verbose` | log HTTP details to stderr (JSON) |
| `--timeout <duration>` | HTTP request timeout (default 30s) |
| `--profile <name>` | use a named config profile |

## Pagination

`jr` auto-paginates by default — it fetches all pages and merges results into a single response. Use `--no-paginate` if you want to handle pagination manually or only need the first page.

## Troubleshooting

**"command not found"** — `jr` is not installed. Install via:
```bash
brew install sofq/tap/jr          # Homebrew
go install github.com/sofq/jira-cli@latest  # Go
```

**Exit code 2 (auth)** — Token expired or misconfigured. Check with:
```bash
jr raw GET /rest/api/3/myself
```

**Unknown command** — Command names are auto-generated from Jira's API and can be verbose. Use `jr schema` to find the right name, or use `jr raw` as an escape hatch.

**Large responses filling context** — Always use `--preset` or `--fields` + `--jq` to minimize output.

**"--dry-run"** — Use this to preview what `jr` will send without making the API call. Useful for debugging request bodies.
