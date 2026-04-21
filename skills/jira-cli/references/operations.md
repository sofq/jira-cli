# Operations — detailed reference

Full examples for every common `jr` operation, plus agent-oriented patterns. The main SKILL.md has one-liners; use this file when you need flag-level detail or when chaining operations.

## Contents

- [Reading issues](#reading-issues) — `get`, `search`, `watch`, `diff`
- [Writing issues](#writing-issues) — `create`, `edit`, `delete`
- [Workflow](#workflow) — `transition`, `move`, `assign`, `link`, `log-work`, `sprint`
- [Comments](#comments) — plain text and ADF
- [Templates](#templates) — apply, show, create
- [Projects and metadata](#projects-and-metadata)
- [Raw API](#raw-api)
- [Agent patterns](#agent-patterns)

---

## Reading issues

### Get one issue

```bash
# Full response (~10K tokens — avoid without filtering)
jr issue get --issueIdOrKey PROJ-123

# With preset (recommended default)
jr issue get --issueIdOrKey PROJ-123 --preset agent

# With server-side field filter
jr issue get --issueIdOrKey PROJ-123 --fields key,summary,status,assignee

# Shape output with jq
jr issue get --issueIdOrKey PROJ-123 --jq '{key, summary: .fields.summary, status: .fields.status.name}'

# Expand changelog / renderedFields / etc.
jr issue get --issueIdOrKey PROJ-123 --expand changelog,renderedFields
```

### Search with JQL

```bash
# Use search-and-reconsile-issues-using-jql (NOT the deprecated /search).
# "reconsile" is NOT a typo — it matches the Jira API spec. Do not "fix" it.
jr search search-and-reconsile-issues-using-jql \
  --jql "project = PROJ AND status = 'In Progress' AND assignee = currentUser()" \
  --fields "key,summary,status,assignee" \
  --jq '[.issues[] | {key, summary: .fields.summary, status: .fields.status.name}]'

# Limit results
jr search search-and-reconsile-issues-using-jql --jql "..." --maxResults 50

# Next page (use nextPageToken from prior response)
jr search search-and-reconsile-issues-using-jql --jql "..." --nextPageToken "tok_abc"
```

### Watch (NDJSON stream)

```bash
# Poll a JQL query and emit events as one JSON object per line.
# ALWAYS use --max-events in automated contexts — agents cannot send SIGINT.
jr watch --jql "project = PROJ AND updated > -5m" --interval 30s --max-events 20

# Watch a single issue
jr watch --issue PROJ-123 --interval 10s --max-events 5

# With preset shaping
jr watch --jql "status changed" --interval 1m --preset triage --max-events 10
```

Event types: `initial` (first poll), `created`, `updated`, `removed`.

### Diff / Changelog

```bash
# Full changelog
jr diff --issue PROJ-123

# Time-bounded
jr diff --issue PROJ-123 --since 2h
jr diff --issue PROJ-123 --since 2025-01-01

# Single field only
jr diff --issue PROJ-123 --field status
jr diff --issue PROJ-123 --field assignee --since 1d
```

---

## Writing issues

### Create (from flags — no raw JSON)

```bash
jr workflow create \
  --project PROJ \
  --type Bug \
  --summary "Login broken on Safari" \
  --description "Steps to reproduce: ..." \
  --priority High \
  --labels bug,urgent \
  --assign me \
  --parent PROJ-100   # for subtasks/child issues
```

### Create (raw JSON — for custom fields)

```bash
jr issue create-issue --body '{
  "fields": {
    "project": {"key": "PROJ"},
    "summary": "Bug title",
    "issuetype": {"name": "Bug"},
    "customfield_10020": 5
  }
}'
```

### Edit

```bash
# Simple field update
jr issue edit --issueIdOrKey PROJ-123 --body '{"fields":{"summary":"Updated title"}}'

# Multiple fields
jr issue edit --issueIdOrKey PROJ-123 --body '{
  "fields": {
    "summary": "New title",
    "priority": {"name": "High"},
    "labels": ["bug", "p1"]
  }
}'

# Edit returns {} for 204 responses (success with no body)
```

### Delete

```bash
jr issue delete --issueIdOrKey PROJ-123
```

---

## Workflow

### Transition

```bash
# By status name (jr resolves transition ID automatically)
jr workflow transition --issue PROJ-123 --to "Done"
jr workflow transition --issue PROJ-123 --to "In Progress"
```

### Transition + assign in one step

```bash
jr workflow move --issue PROJ-123 --to "In Progress" --assign me
```

### Assign

```bash
# "me" resolves to the authenticated user
jr workflow assign --issue PROJ-123 --to me

# Email or display name
jr workflow assign --issue PROJ-123 --to "john@company.com"
jr workflow assign --issue PROJ-123 --to "John Smith"

# Unassign
jr workflow assign --issue PROJ-123 --to none
```

### Link issues

```bash
# Link type name resolves to ID automatically
jr workflow link --from PROJ-1 --to PROJ-2 --type blocks
jr workflow link --from PROJ-1 --to PROJ-2 --type "is blocked by"
jr workflow link --from PROJ-1 --to PROJ-2 --type "relates to"
```

### Log work

```bash
# Human-friendly durations: 2h, 1d 3h, 45m, 1w
jr workflow log-work --issue PROJ-123 --time "2h 30m" --comment "Debugging auth flow"

# With start date
jr workflow log-work --issue PROJ-123 --time "1h" --started "2025-01-15T09:00:00.000+0000"
```

### Sprint

```bash
# Move to sprint by name (jr resolves sprint ID)
jr workflow sprint --issue PROJ-123 --to "Sprint 5"
```

---

## Comments

### Plain text (one-line)

```bash
# Wrapped in a single ADF paragraph. No formatting — markdown renders literally.
jr workflow comment --issue PROJ-123 --text "LGTM, merging now."
```

### Rich content (ADF)

For anything with formatting — headings, lists, code blocks, panels, tables, links, colored text — you must hand-author ADF. See [adf.md](adf.md) for the full node catalog.

```bash
# Minimal ADF comment
jr issue add-comment --issueIdOrKey PROJ-123 --body '{
  "body": {
    "type": "doc",
    "version": 1,
    "content": [
      {"type": "paragraph", "content": [{"type": "text", "text": "A comment"}]}
    ]
  }
}'

# Multi-node documents — use a file, shell-escaping large JSON is painful
jr issue add-comment --issueIdOrKey PROJ-123 --body @comment.adf.json

# Or from stdin
cat comment.adf.json | jr issue add-comment --issueIdOrKey PROJ-123 --body -
```

---

## Templates

Templates are pre-defined issue shapes with variable substitution. Built-in templates: `bug-report`, `story`, `task`, `epic`, `subtask`, `spike`. User-defined templates live in `~/.config/jr/templates/` as YAML.

```bash
# List all templates
jr template list

# Show a template's variables and fields
jr template show bug-report

# Apply a template (variables via --var key=value)
jr template apply bug-report \
  --project PROJ \
  --var summary="Login broken" \
  --var severity=High \
  --assign me

# Subtask under a parent
jr template apply subtask \
  --project PROJ \
  --var summary="Fix auth check" \
  --var parent=PROJ-100

# Create a user-defined template from scratch
jr template create my-template

# Or from an existing issue (captures its fields as the template)
jr template create my-template --from PROJ-123

# Overwrite existing
jr template create my-template --from PROJ-123 --overwrite
```

---

## Projects and metadata

```bash
# List projects (returns a paginated Jira response — .values has the rows)
jr project search --jq '[.values[] | {key, name}]'

# Cache project list for 5 minutes
jr project search --cache 5m --jq '[.values[].key]'

# Get a specific project
jr project get --projectIdOrKey PROJ

# List issue types for a project
jr issuetypes get --jq '[.[] | {name, subtask}]'

# List transitions available for an issue
jr transitions get --issueIdOrKey PROJ-123 --jq '[.transitions[] | {id, name, to: .to.name}]'
```

---

## Raw API

Escape hatch for endpoints not covered by generated commands.

```bash
# GET
jr raw GET /rest/api/3/myself

# POST with body
jr raw POST /rest/api/3/some/endpoint --body '{"key":"value"}'

# POST with body from file
jr raw POST /rest/api/3/some/endpoint --body @request.json

# POST with body from stdin (must pass --body - explicitly)
echo '{"key":"value"}' | jr raw POST /rest/api/3/some/endpoint --body -

# Query parameters (repeatable)
jr raw GET /rest/api/3/issue --query "fields=summary" --query "expand=changelog"

# DELETE
jr raw DELETE /rest/api/3/issue/PROJ-123
```

**Gotchas:**
- POST/PUT/PATCH **require** `--body`. Without it, `jr raw` errors (won't hang on stdin).
- Method is positional, not a flag: `jr raw GET /path`, not `jr raw --method GET`.

---

## Agent patterns

### Check-then-act: update if exists, create if not

```bash
# Exit code 0 → exists; exit 3 → not found
if jr issue get --issueIdOrKey PROJ-123 --preset agent > /dev/null 2>&1; then
  jr issue edit --issueIdOrKey PROJ-123 --body '{"fields":{"summary":"Updated"}}'
else
  jr workflow create --project PROJ --type Task --summary "New task"
fi
```

### Bulk status transitions

Search + batch is 5–10x faster than N individual calls:

```bash
# Step 1: find issues
KEYS=$(jr search search-and-reconsile-issues-using-jql \
  --jql "project = PROJ AND status = 'In Progress' AND assignee = currentUser()" \
  --jq '[.issues[].key] | join(" ")')

# Step 2: build a batch array and execute
# (in practice, generate the JSON with your script/language)
echo '[
  {"command": "workflow transition", "args": {"issue": "PROJ-1", "to": "Done"}},
  {"command": "workflow transition", "args": {"issue": "PROJ-2", "to": "Done"}},
  {"command": "workflow transition", "args": {"issue": "PROJ-3", "to": "Done"}}
]' | jr batch
```

### Create epic with subtasks

```bash
# Step 1: create the epic, capture its key
EPIC=$(jr workflow create --project PROJ --type Epic --summary "Auth redesign" --jq -r '.key')

# Step 2: batch-create subtasks under it
cat <<EOF | jr batch
[
  {"command": "workflow create", "args": {"project": "PROJ", "type": "Subtask", "summary": "Design auth flow",  "parent": "$EPIC"}},
  {"command": "workflow create", "args": {"project": "PROJ", "type": "Subtask", "summary": "Implement OAuth",   "parent": "$EPIC"}},
  {"command": "workflow create", "args": {"project": "PROJ", "type": "Subtask", "summary": "Write tests",       "parent": "$EPIC"}}
]
EOF
```

### Validate before executing

Use `--dry-run` to preview requests without making API calls — especially useful for destructive or bulk ops:

```bash
# Preview
jr workflow create --project PROJ --type Bug --summary "Test" --dry-run
# → {"method":"POST","url":"...","body":{"fields":{...}}}

# Commit
jr workflow create --project PROJ --type Bug --summary "Test"
```

### Retry loop for transient errors

```bash
# Pseudocode — adapt to your language.
# Exit 5 (rate_limited): read retry_after seconds from stderr JSON, sleep, retry (max 3).
# Exit 7 (server_error): exponential backoff 1s, 2s, 4s (max 3).
# Exit 2/3/4: do NOT retry — fix config or request.
for attempt in 1 2 3; do
  OUT=$(jr issue get --issueIdOrKey PROJ-123 --preset agent 2> err.json)
  CODE=$?
  case $CODE in
    0) echo "$OUT"; break ;;
    5) sleep "$(jq -r .retry_after err.json)" ;;
    7) sleep "$((2 ** (attempt - 1)))" ;;
    *) cat err.json; exit $CODE ;;
  esac
done
```

### Sync loop (watch-and-act)

```bash
# Poll a JQL query, route events to handlers, stop after N events.
jr watch --jql "project = PROJ AND status changed" --interval 30s --max-events 100 \
  | while read -r line; do
      EVENT=$(echo "$line" | jq -r .event)
      KEY=$(echo "$line"   | jq -r .issue.key)
      case "$EVENT" in
        created) echo "new issue $KEY" ;;
        updated) echo "changed $KEY"   ;;
        removed) echo "deleted $KEY"   ;;
      esac
    done
```
