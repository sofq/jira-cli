# jr — Agent-Friendly Jira CLI

## Quick Start
```
jr configure --base-url https://yoursite.atlassian.net --token YOUR_API_TOKEN --username your@email.com
jr configure --test                        # test saved credentials
jr configure --test --profile work         # test a specific profile
jr configure --profile myprofile --delete  # remove a profile
```

## Key Patterns

- **All output is JSON** on stdout. Errors are JSON on stderr.
- **Exit codes are semantic**: 0=ok, 1=error, 2=auth, 3=not_found, 4=validation, 5=rate_limited, 6=conflict, 7=server
- **Use `--preset`** for common field sets: `jr issue get --issueIdOrKey PROJ-1 --preset agent` (presets: agent, detail, triage, board)
- **Use `--jq`** to reduce output tokens: `jr issue get --issueIdOrKey PROJ-1 --jq '{key: .key, summary: .fields.summary}'`
- **Use `--fields`** to limit Jira response fields: `jr issue get --issueIdOrKey PROJ-1 --fields key,summary,status`
- **Use `jr batch`** to run multiple operations in one call
- **Use `jr raw`** for any API endpoint not covered by generated commands
- **Jira Cloud v3 uses ADF, not markdown.** Any rich-content field (comment body, issue description) must be an ADF JSON document. Markdown (`**bold**`, `- item`, `# h1`) and wiki markup (`*bold*`, `h1.`) are NOT parsed — they render as literal characters. `jr workflow comment --text` wraps your string in a single ADF paragraph; for anything richer, use `jr issue add-comment --body <adf-json>` (see examples below).

## Common Operations

```bash
# Get issue
jr issue get --issueIdOrKey PROJ-123

# Search issues (use the new JQL endpoint — the old /search is deprecated)
# NOTE: "reconsile" is NOT a typo — it matches Jira's API spec. Do not "fix" it.
jr search search-and-reconsile-issues-using-jql --jql "project = PROJ AND status = 'In Progress'" --fields "key,summary,status" --jq '[.issues[] | {key, summary: .fields.summary}]'

# Create issue
jr issue create-issue --body '{"fields":{"project":{"key":"PROJ"},"summary":"Bug title","issuetype":{"name":"Bug"}}}'

# Transition issue
jr workflow transition --issue PROJ-123 --to "Done"

# Assign issue
jr workflow assign --issue PROJ-123 --to "me"

# Unassign issue
jr workflow assign --issue PROJ-123 --to "none"

# Transition + optional assign in one call
jr workflow move --issue PROJ-123 --to "In Progress" --assign me

# Add comment (plain text only — wrapped in a single ADF paragraph; markdown/wiki syntax is NOT parsed)
jr workflow comment --issue PROJ-123 --text "This is done"

# Create issue from flags (no raw JSON)
jr workflow create --project PROJ --type Bug --summary "Login broken" --description "Steps..." --priority High --labels bug,urgent

# Create issue link by type name
jr workflow link --from PROJ-1 --to PROJ-2 --type blocks

# Log work with human-friendly duration
jr workflow log-work --issue PROJ-123 --time "2h 30m" --comment "Debugging"

# Move issue to sprint by name
jr workflow sprint --issue PROJ-123 --to "Sprint 5"

# Edit issue (returns {} for 204 responses)
jr issue edit --issueIdOrKey PROJ-123 --body '{"fields":{"summary":"Updated title"}}'

# Delete issue
jr issue delete --issueIdOrKey PROJ-123

# Add comment (raw ADF — REQUIRED for any rich formatting: headings, lists, code blocks, panels, tables, links, etc.)
# Jira Cloud REST v3 accepts ONLY ADF (JSON). Markdown and wiki markup are NOT parsed — they render as literal text.
# Use `--body @/path/to/file.json` for multi-node documents.
jr issue add-comment --issueIdOrKey PROJ-123 --body '{"body":{"type":"doc","version":1,"content":[{"type":"paragraph","content":[{"text":"A comment","type":"text"}]}]}}'

# Rich comment example — headings, bold/italic/code marks, bullet list, code block, info panel
jr issue add-comment --issueIdOrKey PROJ-123 --body '{"body":{"type":"doc","version":1,"content":[
  {"type":"heading","attrs":{"level":2},"content":[{"type":"text","text":"Deploy summary"}]},
  {"type":"paragraph","content":[
    {"type":"text","text":"Shipped "},
    {"type":"text","text":"v1.4.2","marks":[{"type":"strong"}]},
    {"type":"text","text":" at "},
    {"type":"text","text":"14:20 UTC","marks":[{"type":"code"}]},
    {"type":"text","text":"."}
  ]},
  {"type":"bulletList","content":[
    {"type":"listItem","content":[{"type":"paragraph","content":[{"type":"text","text":"migrations applied"}]}]},
    {"type":"listItem","content":[{"type":"paragraph","content":[{"type":"text","text":"smoke tests green"}]}]}
  ]},
  {"type":"codeBlock","attrs":{"language":"bash"},"content":[{"type":"text","text":"kubectl rollout status deploy/api"}]},
  {"type":"panel","attrs":{"panelType":"info"},"content":[{"type":"paragraph","content":[{"type":"text","text":"Monitor dashboards for the next hour."}]}]}
]}}'

# Common ADF node types to know:
#   Block:   paragraph, heading (level 1-6), bulletList/orderedList + listItem,
#            taskList + taskItem (attrs.state: TODO|DONE), blockquote, codeBlock (attrs.language),
#            panel (attrs.panelType: info|warning|success|error|note), rule, table + tableRow + tableHeader/tableCell,
#            expand (attrs.title)
#   Inline:  text (with marks: strong, em, strike, underline, code, textColor, link),
#            emoji (attrs.shortName), status (attrs.text, attrs.color), mention, hardBreak

# Raw API call (method is positional, not a flag; POST/PUT/PATCH require --body)
jr raw GET /rest/api/3/myself
jr raw POST /rest/api/3/search/jql --body '{"jql":"project=PROJ"}'
echo '{"jql":"project=PROJ"}' | jr raw POST /rest/api/3/search/jql --body -  # stdin

# List projects
jr project search --jq '[.values[] | {key, name}]'

# Batch operations
echo '[{"command":"issue get","args":{"issueIdOrKey":"PROJ-1"},"jq":".key"},{"command":"issue get","args":{"issueIdOrKey":"PROJ-2"},"jq":".key"}]' | jr batch

# Watch for changes (NDJSON stream — always use --max-events in automated contexts)
jr watch --jql "project = PROJ AND updated > -5m" --interval 30s --max-events 20
jr watch --jql "status changed" --interval 1m --preset triage --max-events 10
jr watch --issue PROJ-123 --interval 10s --max-events 5

# Templates — create issues from predefined patterns
jr template list                                   # list all templates
jr template show bug-report                        # show template definition
jr template apply bug-report --project PROJ --var summary="Login broken" --var severity=High
jr template apply subtask --project PROJ --var summary="Fix auth" --var parent=PROJ-100
jr template create my-template                     # create empty template scaffold
jr template create my-template --from PROJ-123     # create template from existing issue
jr template create my-template --from PROJ-123 --overwrite  # overwrite existing

# Diff / Changelog — structured change history
jr diff --issue PROJ-123                           # all changes
jr diff --issue PROJ-123 --since 2h                # changes in last 2 hours
jr diff --issue PROJ-123 --since 2025-01-01        # changes since date
jr diff --issue PROJ-123 --field status            # only status changes
```

## Discovery

```bash
jr schema                     # resource → verbs mapping (default)
jr schema --list              # all resource names only
jr schema --compact           # same as default (resource → verbs)
jr schema issue               # all operations for 'issue'
jr schema issue get           # full schema with flags for one operation
jr preset list                # list available output presets
jr template list              # list available issue templates
jr template show <name>       # show a template's variables and fields
jr diff --issue <key>         # show changelog for an issue
```

## Batch Command Names

In `jr batch`, use `"resource verb"` strings. Notable: `"diff diff"` (not just `"diff"`), `"template apply"` (args use `name`/`project` + variable names as direct keys).

```bash
echo '[
  {"command": "workflow transition", "args": {"issue": "PROJ-1", "to": "Done"}},
  {"command": "diff diff", "args": {"issue": "PROJ-2", "since": "2h"}},
  {"command": "template apply", "args": {"name": "bug-report", "project": "PROJ", "summary": "Login broken"}}
]' | jr batch --input ops.json  # or pipe to stdin
```

Batch exit code = highest-severity code from all operations.

## Global Flags

| Flag | Description |
|------|-------------|
| `--preset <name>` | named output preset (agent, detail, triage, board) |
| `--jq <expr>` | jq filter on response |
| `--fields <list>` | comma-separated fields to return (GET only) |
| `--cache <duration>` | cache GET responses (e.g. 5m, 1h) |
| `--pretty` | pretty-print JSON |
| `--no-paginate` | disable auto-pagination |
| `--dry-run` | show request without executing |
| `--verbose` | log HTTP details to stderr (JSON) |
| `--timeout <duration>` | HTTP request timeout (default 30s) |
| `--profile <name>` | use named config profile |
| `--audit` | enable audit logging for this invocation |
| `--audit-file <path>` | audit log file path (implies --audit) |
| `--max-batch <N>` | max operations per batch (default 50, batch command only) |

## Security

### Operation Policy (per profile)
Restrict which operations a profile can execute:

```json
{
  "profiles": {
    "agent": {
      "base_url": "...",
      "auth": {"type": "basic", "token": "..."},
      "allowed_operations": ["issue get", "search *", "workflow *"]
    },
    "readonly": {
      "base_url": "...",
      "auth": {"type": "basic", "token": "..."},
      "denied_operations": ["* delete*", "bulk *", "raw *"]
    }
  }
}
```

Rules:
- Use `allowed_operations` OR `denied_operations`, not both
- Patterns use glob matching: `*` matches any sequence
- `allowed_operations`: implicit deny-all, only matching ops run
- `denied_operations`: implicit allow-all, only matching ops blocked

### Batch Limits
Default max batch size is 50. Override with `--max-batch N`.

### Audit Logging
Enable per-profile (`"audit_log": true`) or per-invocation (`--audit`).
Logs to `~/.config/jr/audit.log` (JSONL). Override path with `--audit-file`.

## Development

```bash
make generate    # regenerate commands from OpenAPI spec
make build       # build binary
make test        # run tests
make lint        # run golangci-lint
```
