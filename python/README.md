# jira-jr

**The Jira CLI that speaks JSON — built for AI agents**

Pure JSON stdout. Structured errors on stderr. Semantic exit codes. 600+ auto-generated commands from the Jira OpenAPI spec. Zero prompts, zero interactivity.

## Install

```bash
pip install jira-jr
# or
uv tool install jira-jr
```

## Quick start

```bash
jr configure --base-url https://yourorg.atlassian.net --token YOUR_API_TOKEN
jr issue get --issueIdOrKey PROJ-123 --preset agent
```

## Why agents use jr

### Self-describing commands

```bash
jr schema
jr schema issue get
```

### Token-efficient output

```bash
jr issue get --issueIdOrKey PROJ-123 \
  --fields key,summary --jq '{key: .key, summary: .fields.summary}'
```

### Workflow helpers

```bash
jr workflow move --issue PROJ-123 --to "In Progress" --assign me
jr workflow comment --issue PROJ-123 --text "Fixed in latest deploy"
jr workflow create --project PROJ --type Bug --summary "Login broken" --priority High
```

### Batch operations

```bash
echo '[
  {"command":"issue get","args":{"issueIdOrKey":"PROJ-1"},"jq":".key"},
  {"command":"issue get","args":{"issueIdOrKey":"PROJ-2"},"jq":".key"}
]' | jr batch
```

### Watch mode

```bash
jr watch --jql "project = PROJ" --interval 30s --max-events 10
```

### Error contract

```json
{"error_type":"rate_limited","status":429,"retry_after":30}
```

| Exit | Meaning | Agent action |
|------|---------|--------------|
| 0 | OK | Parse stdout |
| 1 | General error | Log and report |
| 2 | Auth failed | Re-authenticate |
| 3 | Not found | Check issue key |
| 4 | Validation error | Fix request payload |
| 5 | Rate limited | Wait `retry_after` |
| 6 | Conflict | Fetch latest and retry |
| 7 | Server error | Retry with backoff |

### Raw escape hatch

```bash
jr raw GET /rest/api/3/myself
```

## Also available via

- **Homebrew**: `brew install sofq/tap/jr`
- **npm**: `npm install -g jira-jr`
- **Scoop**: `scoop bucket add sofq https://github.com/sofq/scoop-bucket && scoop install jr`
- **Go**: `go install github.com/sofq/jira-cli@latest`

## Documentation

Full docs, skill setup, and source: [github.com/sofq/jira-cli](https://github.com/sofq/jira-cli).
