# Agent Integration

`jr` is designed from the ground up for AI agents and LLM-powered tools. Every command returns structured JSON on stdout, errors as JSON on stderr, and semantic exit codes — so agents can parse, branch, and retry reliably.

## Why jr for Agents

| Property | How jr handles it |
|----------|-------------------|
| **Output format** | JSON on stdout, always. Errors on stderr, always. No mixed output. |
| **Error handling** | Semantic exit codes (0-7) — branch without parsing error messages |
| **Token efficiency** | `--fields` + `--jq` reduce 10,000-token responses to ~50 tokens |
| **Discovery** | `jr schema` for runtime command discovery — no hardcoding needed |
| **Batch operations** | `jr batch` runs multiple commands in a single process |
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

## Token Efficiency

Jira responses can be enormous. A single issue response can consume ~10,000 tokens. Two flags solve this:

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
Always use `--fields` and `--jq` together. `--fields` reduces what Jira sends back (saves network and time). `--jq` shapes the output into exactly what the agent needs (saves tokens).
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

## Error Handling for Agents

Errors are structured JSON on stderr. Branch on exit codes, not error messages:

| Exit Code | Error Type | Meaning | Agent Action |
|-----------|-----------|---------|--------------|
| 0 | — | Success | Parse stdout as JSON |
| 1 | `connection_error` | General error | Log and report to user |
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
Always use `--fields` and `--jq` to minimize output. A single unfiltered issue can consume 10,000+ tokens of an agent's context window.
:::

### Dry-run before mutations

Use `--dry-run` to preview what `jr` will send without making the API call:

```bash
jr issue edit --issueIdOrKey PROJ-123 \
  --body '{"fields":{"summary":"New title"}}' \
  --dry-run
```
