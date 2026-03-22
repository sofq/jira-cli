# jr — Agent-Friendly Jira CLI

## Quick Start
```
jr configure --base-url https://yoursite.atlassian.net --token YOUR_API_TOKEN --username your@email.com
jr configure --profile myprofile --delete   # remove a profile
```

## Key Patterns

- **All output is JSON** on stdout. Errors are JSON on stderr.
- **Exit codes are semantic**: 0=ok, 1=error, 2=auth, 3=not_found, 4=validation, 5=rate_limited, 6=conflict, 7=server
- **Use `--preset`** for common field sets: `jr issue get --issueIdOrKey PROJ-1 --preset agent` (presets: agent, detail, triage, board)
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

# Watch for changes (NDJSON stream)
jr watch --jql "project = PROJ AND updated > -5m" --interval 30s
jr watch --jql "status changed" --interval 1m --preset triage
jr watch --issue PROJ-123 --interval 10s          # watch a single issue
jr watch --jql "project = PROJ" --max-events 10   # stop after 10 events

# Templates — create issues from predefined patterns
jr template list                                   # list all templates
jr template show bug-report                        # show template definition
jr template apply bug-report --project PROJ --var summary="Login broken" --var severity=High
jr template apply subtask --project PROJ --var summary="Fix auth" --var parent=PROJ-100
jr template create my-template                     # create empty template scaffold
jr template create my-template --from PROJ-123     # create template from existing issue

# Diff / Changelog — structured change history
jr diff --issue PROJ-123                           # all changes
jr diff --issue PROJ-123 --since 2h                # changes in last 2 hours
jr diff --issue PROJ-123 --since 2025-01-01        # changes since date
jr diff --issue PROJ-123 --field status            # only status changes

# Avatar — user style profiling
jr avatar extract                                  # extract Jira activity data for current user
jr avatar build                                    # build profile from extracted data (also saves character file)
jr avatar prompt                                   # output profile as agent-consumable prompt text
jr avatar show                                     # display the current profile (YAML)
jr avatar edit                                     # open profile in $EDITOR
jr avatar refresh                                  # re-extract and rebuild in one step
jr avatar status                                   # show extraction/build status

# Avatar — autonomous loop
jr avatar act --jql "assignee = me" --interval 30s # watch and react to Jira events
jr avatar act --dry-run --jql "project = PROJ"     # preview what it would do
jr avatar act --jql "..." --once                   # one poll cycle, process all matches, exit
jr avatar act --jql "..." --character formal-pm    # use specific character
jr avatar act --jql "..." --react-to "assigned,comment_mention"  # filter events
jr avatar act --jql "..." --max-actions 10         # stop after 10 actions

# Character — persona management
jr character list                                  # list all characters
jr character show <name>                           # display character YAML
jr character create <name>                         # scaffold empty character
jr character create --template concise             # from built-in template
jr character edit <name>                           # open in $EDITOR
jr character delete <name>                         # remove character
jr character use <name>                            # set active character
jr character use <name> --writing formal-pm        # compose: override writing section
jr character prompt                                # output active character for agent use
jr character prompt --format json --redact         # structured output, PII redacted

# Yolo — autonomous execution policy
jr yolo status                                     # show yolo state, rate budget
jr yolo history --since 1h                         # review past yolo actions

# Full issue context in one call (issue + comments + changelog)
jr context PROJ-123
jr context PROJ-123 --jq '{key: .issue.key, comments: [.comments.comments[].body]}'

# Health check
jr doctor                                          # check config, auth, connectivity
jr doctor --profile staging                        # check a specific profile

# Error remediation
jr explain '{"error_type":"auth_failed","status":401,"message":"..."}'
echo '{"error_type":"rate_limited",...}' | jr explain  # from stdin

# Pipe: chain source → extract → target
jr pipe "search search-and-reconsile-issues-using-jql --jql 'project=PROJ'" "issue get"
jr pipe "project search" "project get" --extract "[.values[].key]" --inject projectIdOrKey
jr pipe "search ..." "workflow transition --to Done" --parallel 5

# Parallel batch execution
echo '[...]' | jr batch --parallel 5               # run 5 operations concurrently

# Retry transient errors (429, 5xx) with exponential backoff
jr issue get --issueIdOrKey PROJ-123 --retry 3

# Table/CSV output to stderr (JSON still goes to stdout)
jr project search --format table                   # human-readable table on stderr
jr project search --format csv                     # CSV on stderr
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
jr avatar status              # check avatar profile status
jr avatar prompt              # get avatar profile for agent use
jr character list             # list available characters
jr character show <name>      # show a character's definition
jr yolo status                # check yolo execution policy state
```

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
| `--retry <N>` | retry transient errors (429, 5xx) with exponential backoff |
| `--parallel <N>` | concurrent operations in batch (default 0 = sequential) |
| `--format <type>` | additional output format to stderr: table or csv |

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
