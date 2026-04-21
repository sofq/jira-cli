# Batch — running multiple ops in one process

`jr batch` takes a JSON array of operations on stdin (or `--input file.json`) and runs them in a single process. Use it any time you need 3+ Jira calls in sequence — each standalone invocation pays the cold-start cost of parsing the schema.

## Shape

```json
[
  {"command": "issue get",          "args": {"issueIdOrKey": "PROJ-1"}, "jq": ".key"},
  {"command": "workflow transition","args": {"issue": "PROJ-2", "to": "Done"}},
  {"command": "project search",     "args": {},                          "jq": "[.values[].key]"}
]
```

Each entry:
- **`command`** — `"resource verb"` string, exactly as it appears in `jr schema`.
- **`args`** — flag names without the `--` prefix, mapped to values.
- **`jq`** *(optional)* — jq filter applied to this op's response.
- **`fields`** *(optional)* — comma-separated Jira field list for GET ops.
- **`preset`** *(optional)* — named preset for this op.

## Command-name lookup table

Batch `"command"` strings use the full `resource verb` form. Hand-written (non-generated) commands have their own names:

| CLI invocation | Batch `"command"` |
|---|---|
| `jr issue get` | `"issue get"` |
| `jr issue edit` | `"issue edit"` |
| `jr issue delete` | `"issue delete"` |
| `jr issue create-issue` | `"issue create-issue"` |
| `jr issue add-comment` | `"issue add-comment"` |
| `jr workflow create` | `"workflow create"` |
| `jr workflow transition` | `"workflow transition"` |
| `jr workflow move` | `"workflow move"` |
| `jr workflow assign` | `"workflow assign"` |
| `jr workflow comment` | `"workflow comment"` (plain text only) |
| `jr workflow link` | `"workflow link"` |
| `jr workflow log-work` | `"workflow log-work"` |
| `jr workflow sprint` | `"workflow sprint"` |
| `jr template apply` | `"template apply"` |
| `jr diff` | `"diff diff"` ← resource is `diff`, verb is `diff` |
| `jr project search` | `"project search"` |
| `jr search search-and-reconsile-issues-using-jql` | `"search search-and-reconsile-issues-using-jql"` |
| `jr raw` | Not supported in batch — use individual invocations. |

When in doubt, check `jr schema <resource>` for the canonical verb name.

## Args conventions

Most commands just take their flag names (stripped of `--`):

```json
{"command": "workflow transition", "args": {"issue": "PROJ-1", "to": "Done"}}
```

### `template apply` — variables are flat keys

`--var key=value` flags flatten into top-level keys in `args`, alongside `name` / `project`:

```json
{"command": "template apply", "args": {
  "name": "bug-report",
  "project": "PROJ",
  "summary": "Login broken",
  "severity": "High"
}}
```

Not `"vars": {...}` — direct keys.

### `diff diff` — duplicated name is intentional

```json
{"command": "diff diff", "args": {"issue": "PROJ-1", "since": "2h"}}
```

The resource is named `diff` and the verb is also `diff`.

## Worked example

```bash
cat <<'EOF' | jr batch
[
  {"command": "issue get",          "args": {"issueIdOrKey": "PROJ-1"}, "jq": ".key"},
  {"command": "workflow transition","args": {"issue": "PROJ-2", "to": "Done"}},
  {"command": "diff diff",          "args": {"issue": "PROJ-3", "since": "2h"}},
  {"command": "template apply",     "args": {
    "name": "bug-report",
    "project": "PROJ",
    "summary": "Login broken",
    "severity": "High"
  }}
]
EOF

# From a file
jr batch --input ops.json
```

## Exit codes

The **process exit code is the highest-severity code from any operation.** If op #1 returns 0 and op #2 returns 5 (rate_limited), the process exits with 5.

Severity ordering (highest first): 7 > 6 > 5 > 4 > 3 > 2 > 1 > 0.

Per-operation results appear in the output as a JSON array:

```json
[
  {"exit_code": 0, "output": {...}},
  {"exit_code": 5, "error": {...}},
  {"exit_code": 0, "output": {...}}
]
```

Always check individual `exit_code` fields to know which ops succeeded — the process-level exit code tells you "at least one op hit this severity."

## Size limits

Default maximum batch size is **50 operations**. Override for a single invocation:

```bash
jr batch --max-batch 100 --input big-batch.json
```

If you need more than a few hundred ops, split into multiple batches and handle partial failure between them.

## Patterns

### Generating batch JSON from a list

Build the array in your language of choice, don't hand-edit:

```bash
# From a list of keys, generate transition ops
cat keys.txt | jq -R -s 'split("\n") | map(select(length > 0)) |
  map({command: "workflow transition", args: {issue: ., to: "Done"}})' | jr batch
```

### Parallel-looking, sequential-actually

`jr batch` runs ops **sequentially**, not in parallel. Order matters — if op #2 depends on op #1's output, that dependency is not automatic. Either:
- Chain via script (capture op #1's output, feed into op #2's args), or
- Accept that batch is about reducing process-startup overhead, not concurrency.

### Don't batch `jr raw`

`jr raw` is not supported as a batch command. Call it directly.

### Dry-run a batch

There is no batch-level `--dry-run` today — use individual `--dry-run` calls to verify the shape before bundling into a batch.
