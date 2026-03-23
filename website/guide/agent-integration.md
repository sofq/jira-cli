# Agent Integration

`jr` is designed from the ground up for AI agents and LLM-powered tools. Every command returns structured JSON on stdout, errors as JSON on stderr, and semantic exit codes — so agents can parse, branch, and retry reliably.

## Why jr for Agents

| Property | How jr handles it |
|----------|-------------------|
| **Output format** | JSON on stdout, always. Errors on stderr, always. No mixed output. |
| **Error handling** | Semantic exit codes (0-7) — branch without parsing error messages |
| **Token efficiency** | `--preset`, `--fields` + `--jq` reduce 10,000-token responses to ~50 tokens |
| **Workflow commands** | `jr workflow` commands accept simple flags — no JSON body construction needed |
| **Discovery** | `jr schema` for runtime command discovery — no hardcoding needed |
| **Batch operations** | `jr batch` runs multiple commands in a single process, with `--parallel N` for concurrency |
| **Context in one call** | `jr context` fetches issue + comments + changelog in a single invocation |
| **Command chaining** | `jr pipe` chains a source command into a target command with jq extraction |
| **Auto-retry** | `--retry N` retries transient errors (429, 5xx) with exponential backoff |
| **Health checks** | `jr doctor` verifies config, auth, and connectivity in one command |
| **Error remediation** | `jr explain` parses error JSON and returns actionable fix advice |
| **Human-readable output** | `--format table\|csv` renders output to stderr alongside JSON on stdout |
| **Cache presets** | Metadata endpoints auto-cache with sensible TTLs — no manual `--cache` needed |
| **Security controls** | Per-profile operation allowlist/denylist, batch limits, audit logging |
| **600+ commands** | Auto-generated from Jira's OpenAPI spec, always up to date |

## Runtime Discovery

Agents should use `jr schema` to discover commands dynamically rather than hardcoding command names:

```bash
# Get all resources (compact overview)
jr schema

# List just resource names
jr schema --list

# See all verbs for a resource
jr schema issue

# Get full flag reference for one operation
jr schema issue get
```

::: tip
Always use `jr schema` to discover the exact command name and flags before running an unfamiliar operation. Command names come from Jira's API and can be verbose (e.g., `search-and-reconsile-issues-using-jql`).
:::

### Discovery flow for agents

```
1. jr schema --list          → pick the right resource
2. jr schema <resource>      → pick the right verb
3. jr schema <resource> <verb> → see required flags
4. jr <resource> <verb> --flags...  → execute
```

## Workflow Commands

`jr workflow` provides high-level commands that accept simple flags instead of raw JSON bodies. These resolve human-friendly names to Jira IDs automatically — agents don't need to look up transition IDs, account IDs, sprint IDs, or link type IDs.

```bash
# Transition by status name
jr workflow transition --issue PROJ-123 --to "Done"

# Assign by name, email, or "me"/"none"
jr workflow assign --issue PROJ-123 --to "me"

# Transition + assign in one step
jr workflow move --issue PROJ-123 --to "In Progress" --assign me

# Add a comment (plain text, auto-converted to ADF)
jr workflow comment --issue PROJ-123 --text "Fixed in latest deploy"

# Create issue from flags (no raw JSON)
jr workflow create --project PROJ --type Bug --summary "Login broken" --priority High

# Link issues by type name
jr workflow link --from PROJ-1 --to PROJ-2 --type blocks

# Log work with human-friendly duration
jr workflow log-work --issue PROJ-123 --time "2h 30m" --comment "Debugging"

# Move issue to sprint by name
jr workflow sprint --issue PROJ-123 --to "Sprint 5"
```

::: tip
Workflow commands save significant tokens compared to constructing raw JSON bodies. For example, `jr workflow create` replaces a multi-line `--body` JSON payload with simple flags.
:::

## Watch for Changes

`jr watch` polls Jira on an interval and emits change events as NDJSON (one JSON object per line). This enables autonomous agents to monitor Jira without repeated manual searches.

```bash
# Poll a JQL query every 30 seconds
jr watch --jql "project = PROJ AND updated > -5m" --interval 30s

# Watch a single issue
jr watch --issue PROJ-123 --interval 10s

# Use a preset for output shaping
jr watch --jql "status changed" --interval 1m --preset triage

# Stop after N events
jr watch --jql "project = PROJ" --max-events 10
```

Event types:
- `initial` — emitted on first poll for all matching issues
- `created` — new issue appeared in results
- `updated` — issue's `updated` timestamp changed
- `removed` — issue no longer matches the query

Each event is one JSON line: `{"type":"updated","issue":{...}}`

Use Ctrl-C (SIGINT) to stop gracefully. Auth errors (exit code 2) cause immediate termination; transient errors retry with backoff.

## Context Command

`jr context` fetches an issue together with its comments and changelog in a single call. This saves agents from making three separate API requests and merging the results manually.

```bash
# Full context for an issue
jr context PROJ-123

# Reshape with --jq
jr context PROJ-123 --jq '{key: .issue.key, comments: [.comments.comments[].body]}'
```

The response is a JSON object with three top-level keys: `issue`, `comments`, and `changelog`.

## Pipe Command

`jr pipe` chains a source command into a target command. The source runs first, then the results are fed into the target — optionally filtered through a jq expression and injected as a named argument.

```bash
# Pipe search results into issue get
jr pipe "search search-and-reconsile-issues-using-jql --jql 'project=PROJ'" "issue get"

# Extract keys from project search, inject as projectIdOrKey
jr pipe "project search" "project get" --extract "[.values[].key]" --inject projectIdOrKey

# Run target commands in parallel
jr pipe "search ..." "workflow transition --to Done" --parallel 5
```

This is especially useful for agents that need to perform bulk operations across search results without writing shell loops.

## Retry

The `--retry` flag enables automatic retry for transient errors (HTTP 429 and 5xx) with exponential backoff. This is available on any command.

```bash
# Retry up to 3 times on transient failures
jr issue get --issueIdOrKey PROJ-123 --retry 3

# Combine with other flags
jr search search-and-reconsile-issues-using-jql \
  --jql "project = PROJ" --retry 5 --timeout 1m
```

::: tip
Use `--retry` liberally for agent workflows. Jira's API occasionally returns 429 (rate limited) or 503 (service unavailable), and automatic retry avoids the need for agents to implement their own backoff logic.
:::

## Format Flag

The `--format` flag renders output as a human-readable table or CSV to stderr, while JSON output continues to go to stdout as usual. This is useful when an agent wants to display results to a human while still parsing JSON programmatically.

```bash
# Table output on stderr, JSON on stdout
jr project search --format table

# CSV output on stderr
jr project search --format csv

# Combine with --jq (the formatted output reflects the filtered JSON)
jr issue get --issueIdOrKey PROJ-123 --preset agent --format table
```

## Doctor Command

`jr doctor` runs a health check that verifies your configuration, authentication, and network connectivity to Jira. It outputs a JSON report, making it easy for agents to diagnose setup problems automatically.

```bash
# Check the default profile
jr doctor

# Check a specific profile
jr doctor --profile staging
```

The output includes the status of each check (config file, auth, API reachability) and any errors encountered.

## Explain Command

`jr explain` takes a `jr` error JSON (from stderr) and returns human-readable remediation advice. Agents can use this to self-diagnose and fix issues without user intervention.

```bash
# Pass error JSON as an argument
jr explain '{"error_type":"auth_failed","status":401,"message":"..."}'

# Or pipe from stdin
echo '{"error_type":"rate_limited","status":429,"message":"..."}' | jr explain
```

The output explains what the error means and suggests specific steps to resolve it.

## Cache Presets

Metadata endpoints that change infrequently are automatically cached with sensible TTLs. This applies to:

- **Projects** — `project search`, `project get`
- **Statuses** — status endpoints
- **Issue types** — `issuetype` endpoints
- **Priorities** — `priority` endpoints
- **Resolutions** — `resolution` endpoints
- **Issue link types** — `issueLinkType` endpoints

Automatic caching reduces redundant API calls and rate limit consumption without requiring agents to pass `--cache` manually. You can still use `--cache` to override the default TTL for any endpoint.

## Token Efficiency

Jira responses can be enormous. A single issue response can consume ~10,000 tokens. Several mechanisms solve this:

### Output presets

Use `--preset` to select a predefined set of fields without remembering `--fields` + `--jq` combos:

```bash
# Agent-friendly preset: key, summary, status, assignee, type, priority
jr issue get --issueIdOrKey PROJ-123 --preset agent

# Detailed preset: includes description, comments, subtasks, links
jr issue get --issueIdOrKey PROJ-123 --preset detail

# Triage preset: key, summary, status, priority, created, updated, reporter
jr issue get --issueIdOrKey PROJ-123 --preset triage

# Board preset: key, summary, status, assignee, sprint, story points, type
jr issue get --issueIdOrKey PROJ-123 --preset board

# List all available presets
jr preset list
```

### Manual field filtering

For custom field combinations, use `--fields` and `--jq`:

```bash
# --fields: server-side filtering (tell Jira what to return)
# --jq: client-side filtering (reshape the JSON locally)

# Without filtering: ~10,000 tokens
jr issue get --issueIdOrKey PROJ-123

# With filtering: ~50 tokens
jr issue get --issueIdOrKey PROJ-123 \
  --fields key,summary,status \
  --jq '{key: .key, summary: .fields.summary, status: .fields.status.name}'
```

::: warning
Always use `--preset` or `--fields` + `--jq` to minimize output. A single unfiltered issue can consume 10,000+ tokens of an agent's context window.
:::

### Caching repeated reads

Use `--cache` to avoid redundant API calls for stable data:

```bash
# Cache project list for 5 minutes
jr project search --cache 5m --jq '[.values[].key]'
```

## Batch Operations

When an agent needs multiple Jira calls, use `jr batch` to run them in a single process invocation:

```bash
echo '[
  {"command": "issue get", "args": {"issueIdOrKey": "PROJ-1"}, "jq": ".key"},
  {"command": "issue get", "args": {"issueIdOrKey": "PROJ-2"}, "jq": ".fields.summary"},
  {"command": "project search", "args": {}, "jq": "[.values[].key]"}
]' | jr batch
```

Output is an array of results:

```json
[
  {"index": 0, "exit_code": 0, "data": "PROJ-1"},
  {"index": 1, "exit_code": 0, "data": "Fix login timeout"},
  {"index": 2, "exit_code": 0, "data": ["PROJ", "TEAM", "OPS"]}
]
```

Each result includes `index`, `exit_code`, and either `data` (on success) or `error` (on failure). This avoids spawning N subprocesses.

### Parallel execution

Use `--parallel N` to run batch operations concurrently:

```bash
# Run up to 5 operations at a time
echo '[
  {"command": "issue get", "args": {"issueIdOrKey": "PROJ-1"}, "jq": ".key"},
  {"command": "issue get", "args": {"issueIdOrKey": "PROJ-2"}, "jq": ".key"},
  {"command": "issue get", "args": {"issueIdOrKey": "PROJ-3"}, "jq": ".key"}
]' | jr batch --parallel 5
```

The default is `0` (sequential). Setting `--parallel` to a positive number caps the number of concurrent operations. Results are still returned in the original order.

## Context — Full Issue in One Call

`jr context` fetches an issue's fields, comments, and changelog in a single CLI invocation — replacing three separate API calls:

```bash
jr context PROJ-123
jr context PROJ-123 --jq '{key: .issue.key, comments: [.comments.comments[].body]}'
```

## Pipe — Command Chaining

`jr pipe` chains a source command into a target command. It runs the source, extracts values with a jq expression, and executes the target once per value:

```bash
# Get full details for each issue in a search
jr pipe "search search-and-reconsile-issues-using-jql --jql 'project=PROJ'" "issue get"

# Chain with explicit extraction and parallel execution
jr pipe "project search" "project get" --extract "[.values[].key]" --inject projectIdOrKey --parallel 5
```

## Retry — Automatic Backoff

The `--retry N` flag retries transient errors (HTTP 429, 5xx) with exponential backoff:

```bash
jr issue get --issueIdOrKey PROJ-123 --retry 3
```

This works on any command. Combined with the structured error contract, agents can handle transient failures without custom retry logic.

## Doctor — Health Check

`jr doctor` runs diagnostic checks (config, auth, connectivity) and returns a JSON report:

```bash
jr doctor
jr doctor --profile staging
```

```json
{"status":"healthy","checks":[{"name":"config","status":"pass","message":"..."},{"name":"auth","status":"pass","message":"..."}]}
```

## Explain — Error Remediation

`jr explain` parses a jr error JSON and returns actionable remediation advice:

```bash
jr explain '{"error_type":"auth_failed","status":401,"message":"Unauthorized"}'
echo '{"error_type":"rate_limited",...}' | jr explain
```

Output includes `causes`, `fixes`, `retryable` flag, and `exit_code`.

## Format — Human-Readable Output

The `--format` flag renders JSON as a human-readable table or CSV to stderr, while stdout remains clean JSON:

```bash
jr project search --format table   # table on stderr, JSON on stdout
jr project search --format csv     # CSV on stderr, JSON on stdout
```

## Error Handling for Agents

Errors are structured JSON on stderr. Branch on exit codes, not error messages:

| Exit Code | Error Type | Meaning | Agent Action |
|-----------|-----------|---------|--------------|
| 0 | — | Success | Parse stdout as JSON |
| 1 | — | General error | Log and report to user |
| 2 | `auth_failed` | Auth failed (401/403) | Check token/credentials |
| 3 | `not_found` | Not found (404/410) | Verify issue key/resource ID |
| 4 | `validation_error` | Bad request (400/422) | Fix the request payload |
| 5 | `rate_limited` | Rate limited (429) | Wait `retry_after` seconds, then retry |
| 6 | `conflict` | Conflict (409) | Fetch latest and retry |
| 7 | `server_error` | Server error (5xx) | Retry with backoff |

Example error response:

```json
{
  "error_type": "not_found",
  "status": 404,
  "message": "Issue Does Not Exist",
  "hint": "",
  "request": {"method": "GET", "path": "/rest/api/3/issue/BAD-999"}
}
```

::: tip
For exit code 5 (rate limited), the error JSON includes a `retry_after` field with the number of seconds to wait.
:::

## Skill Setup

`jr` ships with a skill file that teaches AI agents everything on this page automatically. The skill follows the [Agent Skills](https://agentskills.io) open standard, supported by 30+ tools.

See the **[Skill Setup Guide](/guide/skill-setup)** for installation instructions for Claude Code, Cursor, VS Code Copilot, OpenAI Codex, Gemini CLI, Goose, Roo Code, and more.

## Troubleshooting

### "command not found"

`jr` is not installed. Install via:

```bash
brew install sofq/tap/jr          # Homebrew
go install github.com/sofq/jira-cli@latest  # Go
```

### Exit code 2 (auth)

Token expired or misconfigured. Verify with:

```bash
jr raw GET /rest/api/3/myself
```

If this fails, generate a new API token at `https://id.atlassian.com/manage-profile/security/api-tokens`.

### Unknown command

Command names are auto-generated from Jira's API and can be verbose. Use `jr schema` to find the right name:

```bash
jr schema --list          # find the resource
jr schema <resource>      # find the verb
```

Or use `jr raw` as an escape hatch for any API endpoint.

### Large responses filling context

::: danger
Always use `--preset` or `--fields` + `--jq` to minimize output. A single unfiltered issue can consume 10,000+ tokens of an agent's context window.
:::

### Dry-run before mutations

Use `--dry-run` to preview what `jr` will send without making the API call:

```bash
jr issue edit --issueIdOrKey PROJ-123 \
  --body '{"fields":{"summary":"New title"}}' \
  --dry-run
```
