---
name: jira-cli
description: "Use when the user asks to interact with Jira — issues, projects, sprints, boards, workflows, tickets, JQL searches, or any Jira Cloud operation. Also use when you see `jr` CLI commands in the codebase."
---

# jr — Jira CLI for AI Agents

`jr` is a Jira Cloud CLI built for agents. Every command returns structured JSON on stdout, errors as JSON on stderr, and semantic exit codes — so you can parse, branch, and retry reliably.

## Core principles

1. **Minimize output.** A raw issue GET returns ~10K tokens. Use `--preset`, `--fields`, or `--jq` — combined they cut that to ~50.
2. **Discover at runtime.** 600+ commands auto-generated from Jira's OpenAPI spec. Don't guess names — run `jr schema`.
3. **Branch on exit codes.** `0`=ok, `2`=auth, `3`=not_found, `4`=validation, `5`=rate_limited, `6`=conflict, `7`=server_error.
4. **Rich content requires ADF.** Jira Cloud v3 does **not** parse markdown or wiki markup — only ADF JSON. See [references/adf.md](references/adf.md).

## Setup

```bash
jr configure --base-url https://yoursite.atlassian.net --token YOUR_API_TOKEN --username your@email.com
jr configure --test        # verify credentials
```

Env vars: `JR_BASE_URL`, `JR_AUTH_TOKEN`, `JR_AUTH_USER`, `JR_AUTH_TYPE`, `JR_CONFIG_PATH`. Named profiles via `--profile <name>`.

Auth failure (exit 2)? Token is likely expired — regenerate at https://id.atlassian.com/manage-profile/security/api-tokens.

Full auth, profiles, env vars, policy, and audit docs: [references/config.md](references/config.md).

## Discovery

```bash
jr schema                   # resource → verbs (start here)
jr schema issue             # all operations for a resource
jr schema issue get         # full flags for one operation
```

Command names come straight from Jira's OpenAPI spec and can be verbose (e.g. `search-and-reconsile-issues-using-jql`). **"reconsile" is not a typo — do not "fix" it to "reconcile".**

## Token efficiency

Three filters, use all three for max compression:

```bash
# --preset: named field set (agent, detail, triage, board)
jr issue get --issueIdOrKey PROJ-1 --preset agent

# --fields: server-side filter (Jira only returns these fields)
jr issue get --issueIdOrKey PROJ-1 --fields key,summary,status

# --jq: client-side shaping
jr issue get --issueIdOrKey PROJ-1 --jq '{key, summary: .fields.summary}'

# Combined — ~10K tokens → ~50
jr issue get --issueIdOrKey PROJ-1 --fields key,summary --jq '{key, summary: .fields.summary}'
```

Preset field sets:

| Preset | Fields |
|--------|--------|
| `agent` | key, summary, status, assignee, type, priority |
| `detail` | `agent` + description, comments, subtasks, links |
| `triage` | key, summary, status, priority, created, updated, reporter |
| `board` | key, summary, status, assignee, sprint, story points, type |

Also: `--cache 5m` for read-heavy data, `--no-paginate` to skip auto-pagination, `jr preset list` for user-defined presets.

## Common operations

```bash
# Get
jr issue get --issueIdOrKey PROJ-1 --preset agent

# Search with JQL (not the deprecated /search endpoint)
jr search search-and-reconsile-issues-using-jql \
  --jql "project = PROJ AND status = 'In Progress'" \
  --jq '[.issues[] | {key, summary: .fields.summary}]'

# Create from flags (no raw JSON)
jr workflow create --project PROJ --type Bug --summary "Login broken" --priority High --assign me

# Edit
jr issue edit --issueIdOrKey PROJ-1 --body '{"fields":{"summary":"Updated"}}'

# Delete
jr issue delete --issueIdOrKey PROJ-1

# Transition (by status name — IDs resolved automatically)
jr workflow transition --issue PROJ-1 --to "Done"

# Transition + assign in one call
jr workflow move --issue PROJ-1 --to "In Progress" --assign me

# Assign (also: email, display name, "none")
jr workflow assign --issue PROJ-1 --to me

# Plain-text comment (wrapped in a single ADF paragraph; markdown renders literally)
jr workflow comment --issue PROJ-1 --text "LGTM"

# Rich comment — requires ADF; see references/adf.md
jr issue add-comment --issueIdOrKey PROJ-1 --body @comment.adf.json

# Link issues (link-type name resolves automatically)
jr workflow link --from PROJ-1 --to PROJ-2 --type blocks

# Log work (human-friendly duration)
jr workflow log-work --issue PROJ-1 --time "2h 30m" --comment "Debugging"

# Sprint (by name)
jr workflow sprint --issue PROJ-1 --to "Sprint 5"

# Template apply (built-ins: bug-report, story, task, epic, subtask, spike)
jr template apply bug-report --project PROJ --var summary="Login broken" --var severity=High

# Changelog
jr diff --issue PROJ-1 --since 2h

# Watch (NDJSON stream — ALWAYS use --max-events in automated contexts)
jr watch --jql "project = PROJ AND updated > -5m" --interval 30s --max-events 20

# Raw API escape hatch (POST/PUT/PATCH require --body)
jr raw GET /rest/api/3/myself
jr raw POST /rest/api/3/search/jql --body '{"jql":"project=PROJ"}'
```

**Watch for `--to` overloading:**
| Command | `--to` means |
|---|---|
| `workflow transition` / `move` | status name |
| `workflow assign` | person (email, display name, `me`, `none`) |
| `workflow sprint` | sprint name |
| `workflow link` | target issue key |

Detailed flags and patterns: [references/operations.md](references/operations.md).

## Error handling

Errors are JSON on stderr. Branch on `exit_code`:

| Exit | `error_type` | Meaning | Action |
|------|--------------|---------|--------|
| 0 | — | Success | Parse stdout |
| 1 | `connection_error` | Network/unknown | Retry once |
| 2 | `auth_failed` | 401/403 | Check token — do NOT retry |
| 3 | `not_found` / `gone` | 404/410 | Verify key — do NOT retry |
| 4 | `validation_error` / `client_error` | 4xx | Fix request — do NOT retry |
| 5 | `rate_limited` | 429 | Wait `retry_after` seconds, retry (max 3) |
| 6 | `conflict` | 409 | Resolve or retry |
| 7 | `server_error` | 5xx | Exponential backoff: 1s, 2s, 4s (max 3) |

Error JSON includes optional `hint` (recovery text) and `retry_after` (seconds):

```json
{"error_type":"rate_limited","status":429,"retry_after":30,"hint":"Wait before retrying."}
```

## Batch

For any sequence of 3+ ops, use `jr batch` — one process instead of N cold starts:

```bash
echo '[
  {"command": "issue get",          "args": {"issueIdOrKey": "PROJ-1"}, "jq": ".key"},
  {"command": "workflow transition","args": {"issue": "PROJ-2", "to": "Done"}},
  {"command": "template apply",     "args": {"name": "bug-report", "project": "PROJ", "summary": "Bug"}}
]' | jr batch
```

- Command strings are `"resource verb"` (e.g. `"diff diff"`, `"workflow transition"`).
- Args use flag names **without** `--`. `template apply` variables are flat keys, not `--var`.
- Process exit code = highest-severity op code. Check per-op `exit_code` fields for individual status.
- Default cap: 50 ops (`--max-batch N`).

Full command-name table and arg conventions: [references/batch.md](references/batch.md).

## Global flags (most used)

| Flag | Purpose |
|------|---------|
| `--preset <name>` | Named output preset |
| `--jq <expr>` | jq filter on response |
| `--fields <list>` | Server-side field filter (GET only) |
| `--cache <duration>` | Cache GET responses (e.g. `5m`, `1h`) |
| `--dry-run` | Show request without executing |
| `--verbose` | Log HTTP details to stderr (JSON) |
| `--profile <name>` | Named config profile |
| `--timeout <duration>` | HTTP timeout (default 30s) |
| `--no-paginate` | Disable auto-pagination |
| `--pretty` | Pretty-print JSON output |

## Common mistakes

- **Writing markdown in comments.** Jira Cloud v3 renders it literally. Use `workflow comment --text` for plain text, or hand-author ADF (see [references/adf.md](references/adf.md)).
- **Omitting `--max-events` on `jr watch`.** Agents can't send Ctrl-C. Without a cap, the stream runs forever.
- **Not filtering responses.** A single unfiltered `issue get` can burn 10K tokens. Always use `--preset` or `--fields` + `--jq`.
- **"Fixing" `reconsile`.** It matches the Jira API spec. Leaving it misspelled is correct.
- **Retrying exit 3 or 4 errors.** Not-found, gone, and validation errors won't become success on retry. Fix the request instead.
- **Looking up transition / link-type / sprint IDs.** `jr` resolves these from names automatically — pass `"Done"`, `"blocks"`, `"Sprint 5"`, never the numeric ID.
- **Running many single invocations.** Each cold-starts the schema parser. Use `jr batch` for 3+ calls.

## References

| File | Contents |
|------|----------|
| [references/adf.md](references/adf.md) | Atlassian Document Format: envelope, node catalog, marks, worked examples, site-edition caveats |
| [references/operations.md](references/operations.md) | Per-operation detail (get, search, create, edit, transition, assign, log-work, watch, diff, raw, templates) + agent patterns (check-then-act, bulk transitions, epic+subtasks, retry loops, sync loops) |
| [references/batch.md](references/batch.md) | Batch command-name lookup, args conventions, exit-code rules, size limits |
| [references/config.md](references/config.md) | Auth types, env vars, profiles, operation policy, audit logging, file locations |

> The project root's `CLAUDE.md` contains a contributor-oriented quick reference for `jr` that overlaps with this skill. If both are in context, treat this SKILL.md + its references as the authoritative guide.
