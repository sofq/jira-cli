# jr — Agent-Friendly Jira CLI

## Quick Start
```
jr configure --base-url https://yoursite.atlassian.net --token YOUR_API_TOKEN --username your@email.com
jr configure --profile myprofile --delete   # remove a profile
```

## Key Patterns

- **All output is JSON** on stdout. Errors are JSON on stderr.
- **Exit codes are semantic**: 0=ok, 1=error, 2=auth, 3=not_found, 4=validation, 5=rate_limited, 6=conflict, 7=server
- **Use `--jq`** to reduce output tokens: `jr issue get --issueIdOrKey PROJ-1 --jq '{key: .key, summary: .fields.summary}'`
- **Use `--fields`** to limit Jira response fields: `jr issue get --issueIdOrKey PROJ-1 --fields key,summary,status`
- **Use `jr batch`** to run multiple operations in one call
- **Use `jr raw`** for any API endpoint not covered by generated commands

## Common Operations

```bash
# Get issue
jr issue get --issueIdOrKey PROJ-123

# Search issues (use the new JQL endpoint — the old /search is deprecated)
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

# Add comment (plain text, auto-converted to ADF)
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

# Add comment (raw API — prefer `workflow comment` above)
jr issue add-comment --issueIdOrKey PROJ-123 --body '{"body":{"type":"doc","version":1,"content":[{"type":"paragraph","content":[{"text":"A comment","type":"text"}]}]}}'

# Raw API call (method is positional, not a flag; POST/PUT/PATCH require --body)
jr raw GET /rest/api/3/myself
jr raw POST /rest/api/3/search/jql --body '{"jql":"project=PROJ"}'
echo '{"jql":"project=PROJ"}' | jr raw POST /rest/api/3/search/jql --body -  # stdin

# List projects
jr project search --jq '[.values[] | {key, name}]'

# Batch operations
echo '[{"command":"issue get","args":{"issueIdOrKey":"PROJ-1"},"jq":".key"},{"command":"issue get","args":{"issueIdOrKey":"PROJ-2"},"jq":".key"}]' | jr batch
```

## Discovery

```bash
jr schema                     # resource → verbs mapping (default)
jr schema --list              # all resource names only
jr schema --compact           # same as default (resource → verbs)
jr schema issue               # all operations for 'issue'
jr schema issue get           # full schema with flags for one operation
```

## Global Flags

| Flag | Description |
|------|-------------|
| `--jq <expr>` | jq filter on response |
| `--fields <list>` | comma-separated fields to return (GET only) |
| `--cache <duration>` | cache GET responses (e.g. 5m, 1h) |
| `--pretty` | pretty-print JSON |
| `--no-paginate` | disable auto-pagination |
| `--dry-run` | show request without executing |
| `--verbose` | log HTTP details to stderr (JSON) |
| `--timeout <duration>` | HTTP request timeout (default 30s) |
| `--profile <name>` | use named config profile |

## Development

```bash
make generate    # regenerate commands from OpenAPI spec
make build       # build binary
make test        # run tests
make lint        # run golangci-lint
```
