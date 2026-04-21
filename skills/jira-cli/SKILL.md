---
name: jira-cli
description: "Use when the user asks to interact with Jira — issues, projects, sprints, boards, workflows, tickets, JQL searches, or any Jira Cloud operation. Also use when you see `jr` CLI commands in the codebase. Covers setup, discovery, batch ops, templates, and error handling."
---

# jr — Jira CLI for AI Agents

`jr` is a Jira Cloud CLI designed for AI agents. Every command returns structured JSON on stdout, errors as JSON on stderr, and semantic exit codes — so you can parse, branch, and retry reliably.

**Sections:** [Setup](#setup) · [Discovering Commands](#discovering-commands) · [Common Operations](#common-operations) · [Token Efficiency](#token-efficiency) · [Batch Operations](#batch-operations) · [Error Handling](#error-handling) · [Global Flags](#global-flags) · [Common Agent Patterns](#common-agent-patterns) · [Security](#security) · [Troubleshooting](#troubleshooting)

## Setup

If `jr` is not configured yet, help the user set it up:

```bash
# Check if jr is installed
jr version

# Configure with Jira Cloud credentials
jr configure --base-url https://yoursite.atlassian.net --token YOUR_API_TOKEN --username your@email.com
```

**Auth types:** `basic` (default, username + API token), `bearer`, `oauth2`

> **Note:** `oauth2` profiles cannot be set up via `jr configure` — they must be configured manually in the config file (`~/.config/jr/config.json`) with `client_id`, `client_secret`, and `token_url`.

**Config resolution order:** CLI flags > environment variables > config file

```bash
# Environment variables (useful for CI/containers)
export JR_BASE_URL=https://yoursite.atlassian.net
export JR_AUTH_TOKEN=your-api-token
export JR_AUTH_TYPE=basic        # auth type (basic, bearer, oauth2)
export JR_AUTH_USER=your@email   # username for basic auth
export JR_CONFIG_PATH=/path/to/config.json  # override config file location

# Named profiles for multiple Jira instances
jr configure --base-url https://work.atlassian.net --token TOKEN --profile work
jr issue get --profile work --issueIdOrKey PROJ-1
jr configure --profile work --delete   # remove a profile (--profile is required)
```

```bash
# Validate credentials before saving (or test an existing profile)
jr configure --base-url https://yoursite.atlassian.net --token TOKEN --username your@email.com --test
jr configure --test                     # test the default profile's saved credentials
jr configure --test --profile work      # test a specific profile
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
# NOTE: "reconsile" is NOT a typo — it matches Jira's API spec. Do not "fix" it to "reconcile".
jr search search-and-reconsile-issues-using-jql \
  --jql "project = PROJ AND status = 'In Progress'" \
  --jq '[.issues[] | {key, summary: .fields.summary, status: .fields.status.name}]'
```

### Create an issue
```bash
# From flags (preferred — no raw JSON needed)
jr workflow create --project PROJ --type Bug --summary "Login broken" --description "Steps to reproduce..." --priority High --labels bug,urgent --assign me

# From raw JSON (for advanced fields)
jr issue create-issue --body '{"fields":{"project":{"key":"PROJ"},"summary":"Bug title","issuetype":{"name":"Bug"}}}'
```

### Edit an issue
```bash
jr issue edit --issueIdOrKey PROJ-123 --body '{"fields":{"summary":"Updated title"}}'
```

### Delete an issue
```bash
jr issue delete --issueIdOrKey PROJ-123
```

### Transition an issue (by status name)
```bash
# No need to look up transition IDs — jr resolves them automatically
jr workflow transition --issue PROJ-123 --to "Done"
jr workflow transition --issue PROJ-123 --to "In Progress"
```

> **Note on `--to` flag:** Multiple commands use `--to` with different semantics:
> - `workflow transition/move --to` = target **status** name (e.g. "Done", "In Progress")
> - `workflow assign --to` = **person** (email, display name, "me", or "none")
> - `workflow sprint --to` = **sprint** name (e.g. "Sprint 5")
> - `workflow link --to` = target **issue** key (e.g. "PROJ-2")

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

**Important:** Jira Cloud REST v3 stores rich content as **ADF (Atlassian Document Format)** — a JSON document tree. **Markdown (`**bold**`, `- item`, `# h1`) and wiki markup (`*bold*`, `h1.`) are NOT parsed** by the API; they render as literal characters. There are two paths:

```bash
# Plain text — wrapped in a single ADF paragraph (no formatting, no markdown parsing).
# Use this for simple one-line comments.
jr workflow comment --issue PROJ-123 --text "This is done"
```

```bash
# Rich content — write ADF directly. REQUIRED for headings, lists, code blocks, panels, tables, links, colored text, etc.
# Prefer --body @file.json for multi-node documents (shell-escaping large JSON is painful).
jr issue add-comment --issueIdOrKey PROJ-123 --body @comment.adf.json
```

**Minimal ADF envelope:**
```json
{"body": {"type": "doc", "version": 1, "content": [ ...nodes... ]}}
```

**Rich comment example** (heading + marks + list + code block + panel):
```json
{"body": {"type": "doc", "version": 1, "content": [
  {"type": "heading", "attrs": {"level": 2}, "content": [{"type": "text", "text": "Deploy summary"}]},
  {"type": "paragraph", "content": [
    {"type": "text", "text": "Shipped "},
    {"type": "text", "text": "v1.4.2", "marks": [{"type": "strong"}]},
    {"type": "text", "text": " at "},
    {"type": "text", "text": "14:20 UTC", "marks": [{"type": "code"}]},
    {"type": "text", "text": "."}
  ]},
  {"type": "bulletList", "content": [
    {"type": "listItem", "content": [{"type": "paragraph", "content": [{"type": "text", "text": "migrations applied"}]}]},
    {"type": "listItem", "content": [{"type": "paragraph", "content": [{"type": "text", "text": "smoke tests green"}]}]}
  ]},
  {"type": "codeBlock", "attrs": {"language": "bash"}, "content": [{"type": "text", "text": "kubectl rollout status deploy/api"}]},
  {"type": "panel", "attrs": {"panelType": "info"}, "content": [{"type": "paragraph", "content": [{"type": "text", "text": "Monitor dashboards for the next hour."}]}]}
]}}
```

**Common ADF node types:**

| Category | Nodes |
|----------|-------|
| Block | `paragraph`, `heading` (`attrs.level` 1–6), `bulletList` / `orderedList` + `listItem`, `taskList` + `taskItem` (`attrs.state`: `TODO`\|`DONE`), `blockquote`, `codeBlock` (`attrs.language`), `panel` (`attrs.panelType`: `info`\|`warning`\|`success`\|`error`\|`note`), `rule`, `table` + `tableRow` + `tableHeader` / `tableCell`, `expand` (`attrs.title`) |
| Inline | `text` with `marks` (`strong`, `em`, `strike`, `underline`, `code`, `textColor` with `attrs.color` hex, `link` with `attrs.href`), `emoji` (`attrs.shortName`), `status` (`attrs.text`, `attrs.color`), `mention` (`attrs.id`), `hardBreak` |

Note: a few nodes (`status`, `expand`, `taskList`) depend on site edition / feature flags — if they render as blank, the target instance may not support them.

**Converting markdown → ADF:** there is no built-in flag today. Either (a) hand-author ADF, (b) use an external converter (e.g. the Atlassian `md-to-adf` lib) and pipe the result into `--body @-`, or (c) fall back to `workflow comment` for plain text only.

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

### Watch for changes (NDJSON stream)
```bash
# Poll a JQL query and emit events as NDJSON (one JSON object per line)
jr watch --jql "project = PROJ AND updated > -5m" --interval 30s

# Watch with a preset for output shaping
jr watch --jql "status changed" --interval 1m --preset triage

# Watch a single issue
jr watch --issue PROJ-123 --interval 10s

# Stop after N events
jr watch --jql "project = PROJ" --max-events 10
```

Events: `initial` (first poll), `created`, `updated`, `removed`.

**Important:** Always use `--max-events` when calling from an automated/agent context — agents cannot send Ctrl-C (SIGINT) to stop the stream.

### Create issues from templates
```bash
# List available templates (bug-report, story, task, epic, subtask, spike)
jr template list

# Show a template's variables and fields
jr template show bug-report

# Create issue from a template (no raw JSON needed)
jr template apply bug-report --project PROJ --var summary="Login broken" --var severity=High --assign me

# Create a sub-task from template
jr template apply subtask --project PROJ --var summary="Fix auth" --var parent=PROJ-100

# Create a user-defined template from an existing issue
jr template create my-template --from PROJ-123

# Overwrite an existing user-defined template
jr template create my-template --from PROJ-123 --overwrite
```

User-defined templates are stored in `~/.config/jr/templates/` as YAML files.

### Diff / Changelog
```bash
# All changes on an issue
jr diff --issue PROJ-123

# Changes in last 2 hours
jr diff --issue PROJ-123 --since 2h

# Changes since a specific date
jr diff --issue PROJ-123 --since 2025-01-01

# Only status changes
jr diff --issue PROJ-123 --field status
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
# Pass query parameters (repeatable)
jr raw GET /rest/api/3/issue --query "fields=summary" --query "expand=changelog"
```

**Note:** POST/PUT/PATCH require `--body`. Without it, `jr raw` will error instead of hanging on stdin. Use `--query key=value` (repeatable) to append query parameters to any raw request.

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

### Preset field reference

| Preset | Server-side fields requested |
|--------|------------------------------|
| `agent` | `key,summary,status,assignee,issuetype,priority` |
| `detail` | `key,summary,status,assignee,issuetype,priority,description,comment,subtasks,issuelinks` |
| `triage` | `key,summary,status,priority,created,updated,reporter` |
| `board` | `key,summary,status,assignee,customfield_10020,customfield_10028,issuetype` |

The response is standard Jira JSON with `--fields` applied — the `fields` object contains only the requested fields. Use `--jq` to further shape the output (e.g., `--jq '{key, summary: .fields.summary}'`).

User-defined presets can include a `jq` filter; store them in `~/.config/jr/presets.json`.

## Batch Operations

When you need multiple Jira calls, use `jr batch` to run them in a single process:

```bash
echo '[
  {"command": "issue get", "args": {"issueIdOrKey": "PROJ-1"}, "jq": ".key"},
  {"command": "issue get", "args": {"issueIdOrKey": "PROJ-2"}, "jq": ".fields.summary"},
  {"command": "project search", "args": {}, "jq": "[.values[].key]"}
]' | jr batch
```

**Batch exit code:** The process exit code is the highest-severity exit code from all operations. If one op returns 0 and another returns 5 (rate_limited), the batch process exits with 5. Check individual `exit_code` fields for per-operation status.

### Batch command names

Batch uses `"resource verb"` strings matching `jr schema` output. Hand-written commands use their full resource+verb name:

| CLI command | Batch `"command"` string |
|---|---|
| `jr workflow transition` | `"workflow transition"` |
| `jr workflow assign` | `"workflow assign"` |
| `jr workflow move` | `"workflow move"` |
| `jr workflow create` | `"workflow create"` |
| `jr workflow comment` | `"workflow comment"` (plain text only) |
| `jr issue add-comment` | `"issue add-comment"` (raw ADF for rich formatting) |
| `jr workflow link` | `"workflow link"` |
| `jr workflow log-work` | `"workflow log-work"` |
| `jr workflow sprint` | `"workflow sprint"` |
| `jr template apply` | `"template apply"` |
| `jr diff` | `"diff diff"` |
| `jr issue get` | `"issue get"` |

Note: `"diff diff"` is correct — the resource is `diff` and the verb is `diff`.

Batch args use the flag names directly (without `--`), and `template apply` uses `name`/`project` plus variable names as direct keys (not `--var key=value`):

```bash
echo '[
  {"command": "issue get", "args": {"issueIdOrKey": "PROJ-1"}, "jq": ".key"},
  {"command": "workflow transition", "args": {"issue": "PROJ-2", "to": "Done"}},
  {"command": "diff diff", "args": {"issue": "PROJ-3", "since": "2h"}},
  {"command": "template apply", "args": {"name": "bug-report", "project": "PROJ", "summary": "Login broken", "severity": "High"}}
]' | jr batch

# Or read from a file
jr batch --input ops.json
```

## Error Handling

Errors are structured JSON on stderr. Branch on `exit_code` and `error_type`:

| Exit code | error_type | Meaning | What to do |
|-----------|-----------|---------|------------|
| 0 | — | Success | Parse stdout as JSON |
| 1 | `connection_error` | Network/unknown error | Check connectivity, retry |
| 2 | `auth_failed` | Auth failed (401/403) | Check token/credentials |
| 3 | `not_found` | Resource not found (404) | Verify issue key / resource ID |
| 3 | `gone` | Resource gone (410) | Resource was deleted; do not retry |
| 4 | `validation_error` | Bad request (400/422/4xx) | Fix the request payload |
| 4 | `client_error` | Other 4xx errors | Check request parameters |
| 5 | `rate_limited` | Rate limited (429) | Wait `retry_after` seconds, then retry |
| 6 | `conflict` | Conflict (409) | Retry or resolve conflict |
| 7 | `server_error` | Server error (5xx) | Retry with backoff |

Error JSON includes optional `hint` (actionable recovery text) and `retry_after` (integer seconds for rate limits):

```json
{
  "error_type": "rate_limited",
  "status": 429,
  "message": "Rate limit exceeded",
  "hint": "You are being rate limited. Wait before retrying.",
  "retry_after": 30,
  "request": {"method": "GET", "path": "/rest/api/3/search/jql"}
}
```

```json
{
  "error_type": "auth_failed",
  "status": 401,
  "message": "Unauthorized",
  "hint": "Run `jr configure --base-url <url> --token <token> --username <email>` to authenticate.",
  "request": {"method": "GET", "path": "/rest/api/3/issue/BAD-999"}
}
```

### Retry pattern for agents

For rate limits (exit 5) and server errors (exit 7), retry with backoff:

```
1. Run the command
2. If exit code is 5 (rate_limited):
   - Parse stderr JSON, read "retry_after" (integer seconds)
   - Wait that many seconds, then retry (max 3 retries)
3. If exit code is 7 (server_error):
   - Retry with exponential backoff: wait 1s, 2s, 4s (max 3 retries)
4. If exit code is 3 (not_found/gone) or 4 (validation_error):
   - Do NOT retry — fix the request or report to user
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
| `--audit` | enable audit logging for this invocation |
| `--audit-file <path>` | audit log file path (implies --audit) |
| `--base-url <url>` | override base URL for this invocation |
| `--auth-token <token>` | override auth token for this invocation |
| `--auth-user <email>` | override auth username for this invocation |
| `--auth-type <type>` | override auth type for this invocation |

## Common Agent Patterns

### Check-then-act: update if exists, create if not

```bash
# Try to get the issue; check exit code
jr issue get --issueIdOrKey PROJ-123 --preset agent

# If exit code 0 → issue exists, update it
jr issue edit --issueIdOrKey PROJ-123 --body '{"fields":{"summary":"Updated"}}'

# If exit code 3 (not_found) → create it
jr workflow create --project PROJ --type Task --summary "New task"
```

### Bulk status transitions

Search + batch to move all matching issues:

```bash
# Step 1: Find issues
ISSUES=$(jr search search-and-reconsile-issues-using-jql \
  --jql "project = PROJ AND status = 'In Progress' AND assignee = currentUser()" \
  --jq '[.issues[].key]')

# Step 2: Build batch payload and execute
# (construct a JSON array of workflow transition ops from the keys)
echo "$ISSUES" | jr raw POST /dev/null --dry-run  # use your language to build the array
echo '[
  {"command": "workflow transition", "args": {"issue": "PROJ-1", "to": "Done"}},
  {"command": "workflow transition", "args": {"issue": "PROJ-2", "to": "Done"}}
]' | jr batch
```

### Create epic with subtasks

```bash
# Step 1: Create the epic
EPIC=$(jr workflow create --project PROJ --type Epic --summary "Auth redesign" \
  --jq '.key')

# Step 2: Create subtasks under the epic (use batch for efficiency)
echo "[
  {\"command\": \"workflow create\", \"args\": {\"project\": \"PROJ\", \"type\": \"Subtask\", \"summary\": \"Design auth flow\", \"parent\": \"$EPIC\"}},
  {\"command\": \"workflow create\", \"args\": {\"project\": \"PROJ\", \"type\": \"Subtask\", \"summary\": \"Implement OAuth\", \"parent\": \"$EPIC\"}},
  {\"command\": \"workflow create\", \"args\": {\"project\": \"PROJ\", \"type\": \"Subtask\", \"summary\": \"Write tests\", \"parent\": \"$EPIC\"}}
]" | jr batch
```

### Validate before executing

Use `--dry-run` to preview requests before making API calls:

```bash
# Preview what would be sent (no API call made)
jr workflow create --project PROJ --type Bug --summary "Test" --dry-run
# Returns: {"method":"POST","url":"...","body":{"fields":{...}}}

# If the output looks correct, run without --dry-run
jr workflow create --project PROJ --type Bug --summary "Test"
```

## Security

### Operation Policy (per profile)
Restrict which operations a profile can execute:

```json
{
  "profiles": {
    "agent": {
      "allowed_operations": ["issue get", "search *", "workflow *"]
    },
    "readonly": {
      "denied_operations": ["* delete*", "bulk *", "raw *"]
    }
  }
}
```

- Use `allowed_operations` OR `denied_operations`, not both
- Patterns use glob matching: `*` matches any sequence
- `allowed_operations`: implicit deny-all, only matching ops run
- `denied_operations`: implicit allow-all, only matching ops blocked

### Batch Limits
Default max batch size is 50. Override with `--max-batch N`.

### Audit Logging
Enable per-profile (`"audit_log": true`) or per-invocation (`--audit`).
Logs to `~/.config/jr/audit.log` (JSONL). Override path with `--audit-file`.

## Pagination

`jr` auto-paginates by default — it fetches all pages and merges results into a single response. Use `--no-paginate` if you only need the first page.

## Troubleshooting

**"command not found"** — `jr` is not installed. Install via:
```bash
brew install sofq/tap/jr          # Homebrew
go install github.com/sofq/jira-cli@latest  # Go
```

**Exit code 2 (auth)** — Token expired or misconfigured. Test with:
```bash
jr configure --test
```

**Unknown command** — Command names are auto-generated from Jira's API and can be verbose. Use `jr schema` to find the right name, or use `jr raw` as an escape hatch.

**Large responses filling context** — Always use `--preset` or `--fields` + `--jq` to minimize output.

**"--dry-run"** — Use this to preview what `jr` will send without making the API call. Useful for debugging request bodies.

---

> **Note:** The project root also contains `CLAUDE.md` with a compressed quick-reference for `jr`. If both files are loaded in context, prefer this SKILL.md as the authoritative reference — `CLAUDE.md` is a contributor-oriented summary.
