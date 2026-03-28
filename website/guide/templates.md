# Templates

Templates let you create Jira issues from predefined patterns — no raw JSON, no remembering field structures. Define your variables, and `jr` handles the rest.

## Built-in Templates

jr ships with 6 templates out of the box:

| Template | Issue Type | Required Variables | Optional Variables |
|----------|-----------|-------------------|-------------------|
| `bug-report` | Bug | `summary` | `description`, `steps`, `expected`, `actual`, `severity` |
| `story` | Story | `summary` | `description`, `acceptance-criteria`, `story-points` |
| `task` | Task | `summary` | `description`, `assignee` |
| `epic` | Epic | `summary` | `description` |
| `subtask` | Sub-task | `summary`, `parent` | `description` |
| `spike` | Task | `summary` | `description`, `time-box` |

## Quick Start

```bash
# List all templates
jr template list

# See what a template expects
jr template show bug-report

# Create an issue from a template
jr template apply bug-report \
  --project PROJ \
  --var summary="Login page returns 500 on Safari" \
  --var severity=High \
  --var steps="1. Open Safari\n2. Click Login\n3. See 500 error"
```

Output:
```json
{"id":"10042","key":"PROJ-42","self":"https://yoursite.atlassian.net/rest/api/3/issue/10042"}
```

## Using Variables

Pass variables with `--var key=value`. Required variables must be provided — optional ones are omitted if not set.

```bash
# Minimal — only required vars
jr template apply task --project PROJ --var summary="Update dependencies"

# Full — all vars filled in
jr template apply story \
  --project PROJ \
  --var summary="User can reset password" \
  --var description="As a user, I want to reset my password via email" \
  --var acceptance-criteria="- Reset link sent within 30s\n- Link expires after 1h" \
  --var story-points=3
```

### Assigning on Creation

Use the `--assign` flag to assign the issue immediately:

```bash
jr template apply task \
  --project PROJ \
  --var summary="Deploy hotfix" \
  --assign me
```

### Dry Run

Preview the API request without creating anything:

```bash
jr template apply bug-report \
  --project PROJ \
  --var summary="Test" \
  --dry-run
```

## Creating Your Own Templates

### From Scratch

```bash
jr template create my-template
```

This creates a scaffold YAML file in your templates directory (`~/.config/jr/templates/` on Linux, `~/Library/Application Support/jr/templates/` on macOS). Edit it to define your fields and variables.

### From an Existing Issue

Clone the structure of any issue into a reusable template:

```bash
jr template create prod-incident --from PROJ-99
```

jr fetches the issue, extracts its fields (summary, description, priority, labels, parent), and generates a template with appropriate variables. The summary becomes a required variable; everything else becomes optional with defaults based on the original issue.

### Overwriting

If a template with the same name already exists:

```bash
jr template create my-template --overwrite
```

### Template File Format

Templates are YAML files:

<div v-pre>

```yaml
name: prod-incident
issuetype: Bug
description: Template for production incidents
variables:
  - name: summary
    required: true
    description: Incident title
  - name: severity
    required: false
    default: High
    description: Incident severity (Critical, High, Medium, Low)
  - name: impact
    required: false
    description: User impact description
fields:
  summary: "{{.summary}}"
  description: "[{{.severity}}] Production Incident\n\n{{if .impact}}Impact: {{.impact}}{{end}}"
  priority: "{{.severity}}"
  labels: "incident,production"
```

**Key rules:**
- Fields use Go template syntax (e.g. `{{.variable}}`)
- Use `{{if .var}}...{{end}}` for optional sections — empty fields are automatically omitted
- Hyphenated variable names use `{{index . "my-var"}}` syntax
- User templates override built-in templates with the same name

</div>

### Template Name Rules

Names must match `^[a-zA-Z0-9][a-zA-Z0-9_-]*$`:
- Valid: `my-template`, `task_v2`, `bug123`
- Invalid: `-starts-dash`, `has space`, `template/sub`

## Examples

### Sprint Planning

```bash
# Create a spike for investigation
jr template apply spike \
  --project PROJ \
  --var summary="Evaluate new caching strategy" \
  --var time-box="2 days" \
  --assign me

# Create the story that follows
jr template apply story \
  --project PROJ \
  --var summary="Implement Redis caching layer" \
  --var acceptance-criteria="- Cache hit rate > 90%\n- p99 latency < 50ms"

# Break it into subtasks
jr template apply subtask \
  --project PROJ \
  --var summary="Set up Redis cluster" \
  --var parent=PROJ-200

jr template apply subtask \
  --project PROJ \
  --var summary="Add cache invalidation logic" \
  --var parent=PROJ-200
```

### Batch Template Usage

Combine templates with `jr batch` for bulk creation. In batch mode, template variables are passed as direct keys in `args` (not as `--var key=value`):

```bash
echo '[
  {"command":"template apply","args":{"name":"subtask","project":"PROJ","summary":"Design API schema","parent":"PROJ-200"}},
  {"command":"template apply","args":{"name":"subtask","project":"PROJ","summary":"Write integration tests","parent":"PROJ-200"}},
  {"command":"template apply","args":{"name":"subtask","project":"PROJ","summary":"Update documentation","parent":"PROJ-200"}}
]' | jr batch
```
