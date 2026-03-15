# jira-jr

**Jira CLI built for AI agents** — pure JSON output, semantic exit codes, 600+ auto-generated commands, and built-in jq filtering.

Give your AI agent (Claude Code, Cursor, Copilot, or custom bots) reliable, token-efficient access to Jira Cloud.

## Install

```bash
pip install jira-jr
# or
uv tool install jira-jr
```

## Why jr?

```bash
# Full Jira response: ~10,000 tokens
jr issue get --issueIdOrKey PROJ-123

# With jr's filtering: ~50 tokens
jr issue get --issueIdOrKey PROJ-123 --fields key,summary --jq '{key: .key, summary: .fields.summary}'
```

- **All output is JSON** — stdout for data, stderr for errors, always
- **Semantic exit codes** — 0=ok, 2=auth, 3=not_found, 5=rate_limited — agents can branch without parsing
- **600+ commands** from the official Jira OpenAPI spec, synced weekly
- **Batch operations** — N API calls in one process via `jr batch`
- **Self-describing** — `jr schema --compact` lets agents discover commands at runtime
- **Workflow helpers** — `jr workflow transition --to "Done"` resolves IDs automatically

## Quick start

```bash
# Configure
jr configure --base-url https://yoursite.atlassian.net --token YOUR_API_TOKEN

# Search issues
jr search search-and-reconsile-issues-using-jql \
  --jql "project = PROJ AND status = 'In Progress'" \
  --jq '[.issues[] | {key, summary: .fields.summary}]'

# Transition + assign in one call
echo '[
  {"command":"workflow transition","args":{"issue":"PROJ-123","to":"Done"}},
  {"command":"workflow assign","args":{"issue":"PROJ-123","to":"me"}}
]' | jr batch
```

## Also available via

- **Homebrew**: `brew install sofq/tap/jr`
- **npm**: `npm install -g jira-jr`
- **Scoop**: `scoop bucket add sofq https://github.com/sofq/scoop-bucket && scoop install jr`
- **Docker**: `docker run --rm ghcr.io/sofq/jr version`
- **Go**: `go install github.com/sofq/jira-cli@latest`

## Documentation

Full docs, Claude Code skill, and source at [github.com/sofq/jira-cli](https://github.com/sofq/jira-cli).
