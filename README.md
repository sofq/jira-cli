<p align="center">
  <h1 align="center">jr</h1>
  <p align="center"><strong>The Jira CLI that speaks JSON — built for AI agents</strong></p>
</p>

<p align="center">
  <a href="https://www.npmjs.com/package/jira-jr"><img src="https://img.shields.io/npm/v/jira-jr?style=for-the-badge&logo=npm&logoColor=white&color=CB3837" alt="npm"></a>
  <a href="https://github.com/sofq/jira-cli/releases"><img src="https://img.shields.io/github/v/release/sofq/jira-cli?style=for-the-badge&logo=github&logoColor=white&color=181717" alt="GitHub Release"></a>
  <a href="https://github.com/sofq/jira-cli/actions/workflows/ci.yml"><img src="https://img.shields.io/github/actions/workflow/status/sofq/jira-cli/ci.yml?style=for-the-badge&logo=githubactions&logoColor=white&label=CI" alt="CI"></a>
  <a href="https://codecov.io/gh/sofq/jira-cli"><img src="https://img.shields.io/codecov/c/github/sofq/jira-cli?style=for-the-badge&logo=codecov&logoColor=white" alt="codecov"></a>
  <a href="LICENSE"><img src="https://img.shields.io/badge/License-Apache_2.0-blue?style=for-the-badge" alt="License"></a>
</p>

<br>

> Pure JSON stdout. Structured errors on stderr. Semantic exit codes. 600+ auto-generated commands from the Jira OpenAPI spec. Zero prompts, zero interactivity — just pipe and parse.

---

## Install

```bash
brew install sofq/tap/jr          # macOS / Linux
npm install -g jira-jr            # Node
pip install jira-jr               # Python
scoop bucket add sofq https://github.com/sofq/scoop-bucket && scoop install jr  # Windows
go install github.com/sofq/jira-cli@latest                                      # Go
```

## Quick start

```bash
jr configure --base-url https://yourorg.atlassian.net --token YOUR_API_TOKEN
jr issue get --issueIdOrKey PROJ-123 --preset agent
```

## Why agents love jr

### Self-describing — no hardcoded command lists

```bash
jr schema                  # all resources and verbs
jr schema issue get        # full flags for one operation
```

### Token-efficient — 10K tokens → 50

```bash
jr issue get --issueIdOrKey PROJ-123 \
  --fields key,summary --jq '{key: .key, summary: .fields.summary}'
```

`--preset`, `--fields`, and `--jq` stack so agents only consume what they need.

### Workflow commands — no Jira internals

Agents don't need to resolve transition IDs, account IDs, or sprint IDs:

```bash
jr workflow move --issue PROJ-123 --to "In Progress" --assign me
jr workflow comment --issue PROJ-123 --text "Fixed in latest deploy"
jr workflow create --project PROJ --type Bug --summary "Login broken" --priority High
jr workflow link --from PROJ-1 --to PROJ-2 --type blocks
jr workflow log-work --issue PROJ-123 --time "2h 30m"
jr workflow sprint --issue PROJ-123 --to "Sprint 5"
```

### Batch — N operations, one process

```bash
echo '[
  {"command":"issue get","args":{"issueIdOrKey":"PROJ-1"},"jq":".key"},
  {"command":"issue get","args":{"issueIdOrKey":"PROJ-2"},"jq":".key"}
]' | jr batch
```

### Watch — NDJSON event stream

```bash
jr watch --jql "project = PROJ" --interval 30s --max-events 10
```

Events: `initial`, `created`, `updated`, `removed`.

### Templates — structured issue creation

```bash
jr template apply bug-report --project PROJ --var summary="Login broken" --var severity=High
jr template create my-template --from PROJ-123
```

Built-in: `bug-report`, `story`, `task`, `epic`, `subtask`, `spike`.

### Diff — structured changelog

```bash
jr diff --issue PROJ-123 --since 2h --field status
```

### Error contract agents can branch on

```json
{"error_type":"rate_limited","status":429,"retry_after":30}
```

| Exit | Meaning | Agent action |
|------|---------|-------------|
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
jr raw POST /rest/api/3/search/jql --body '{"jql":"project=PROJ"}'
```

## Agent integration

### Claude Code skill (included)

```bash
cp -r skill/jira-cli ~/.claude/skills/    # global
```

### Any agent

Add to your agent's instructions:

```
Use `jr` for all Jira operations. Output is always JSON.
Use `jr schema` to discover commands. Use --jq to reduce tokens.
```

## Security

**Operation policies** — restrict per profile with glob patterns:

```json
{"allowed_operations": ["issue get", "search *", "workflow *"]}
```

**Audit logging** — JSONL to `~/.config/jr/audit.log` via `--audit` flag or per-profile config.

**Batch limits** — default 50, override with `--max-batch N`.

## Development

```bash
make generate    # regenerate from OpenAPI spec
make build && make test && make lint
```

## License

[Apache 2.0](LICENSE)
