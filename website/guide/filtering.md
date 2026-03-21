# Filtering & Presets

Jira API responses are large. A single issue can be 10,000+ tokens of JSON. `jr` provides several ways to cut that down dramatically.

## `--preset` — named output presets

The `--preset` flag selects a predefined combination of fields, so you don't need to remember which fields to request:

```bash
# Get key fields for agent use
jr issue get --issueIdOrKey PROJ-123 --preset agent

# Get detailed view with description and comments
jr issue get --issueIdOrKey PROJ-123 --preset detail
```

Available presets: `agent`, `detail`, `triage`, `board`. Run `jr preset list` to see all presets and what fields they include.

## `--fields` — server-side filtering

The `--fields` flag tells Jira to return only the fields you ask for. This reduces what comes over the wire:

```bash
jr issue get --issueIdOrKey PROJ-123 --fields key,summary,status,assignee
```

## `--jq` — client-side filtering

The `--jq` flag applies a [jq](https://jqlang.github.io/jq/) expression to the response, reshaping or extracting data:

```bash
jr issue get --issueIdOrKey PROJ-123 --jq '{key: .key, summary: .fields.summary}'
```

## Combine both for maximum efficiency

Use `--fields` and `--jq` together to minimize tokens at every stage:

**Before** (no filtering) — ~10,000 tokens:
```bash
jr issue get --issueIdOrKey PROJ-123
```

**After** (both flags) — ~50 tokens:
```bash
jr issue get --issueIdOrKey PROJ-123 \
  --fields key,summary \
  --jq '{key: .key, summary: .fields.summary}'
```

::: tip
Always use `--fields` and `--jq` together. `--fields` reduces what Jira sends back (saving bandwidth and API quota), while `--jq` shapes the output into exactly the structure you need.
:::

## `--format` — human-readable output

The `--format` flag renders the response as a table or CSV to stderr, while JSON output continues on stdout as usual. This is useful when you want to display results to a human while still piping JSON to another tool.

```bash
# Table output on stderr
jr project search --format table

# CSV output on stderr
jr project search --format csv

# Combine with presets
jr issue get --issueIdOrKey PROJ-123 --preset agent --format table
```

::: tip
The formatted output goes to stderr and JSON goes to stdout. This means you can redirect them independently — for example, pipe stdout to `jq` while viewing the table on your terminal.
:::

## Cache read-heavy data

For data that changes infrequently (like project lists), use `--cache` to avoid redundant API calls:

```bash
jr project search --cache 5m --jq '[.values[].key]'
```
