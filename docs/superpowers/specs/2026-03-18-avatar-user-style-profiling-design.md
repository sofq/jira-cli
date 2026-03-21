# Avatar — User Style Profiling for AI Agents

## Overview

A two-phase system that extracts a Jira user's writing style, workflow habits, and interaction patterns, then generates a natural language profile that an AI agent can use to act as the user's avatar.

`jr` is the data provider only. Style application is the AI agent's responsibility.

## Approach: Two-Phase Hybrid

**Phase 1 — `jr avatar extract`**: Pure Go. Fetches Jira history, computes structured analysis (stats, patterns, examples). Deterministic, testable, no external dependencies.

**Phase 2 — `jr avatar build`**: Takes extraction output, generates the profile. Two engines:
- `local` — Go template-based prose (no dependencies, functional but formulaic)
- `llm` — pipes extraction to a user-configured external LLM command (rich, nuanced output)

## Command Interface

```bash
jr avatar extract [--user email|accountId] [--min-comments 50] [--min-updates 30] [--max-window 6m] [--no-cache]
jr avatar build [--engine local|llm] [--llm-cmd "..."] [--yes]  # --yes skips LLM consent prompt
jr avatar prompt [--format prose|json|both] [--section writing,workflow,interaction] [--redact]
jr avatar show       # outputs profile as JSON to stdout (consistent with jr contract; --pretty for formatted)
jr avatar edit       # open profile.yaml in $EDITOR (see note below)
jr avatar refresh    # shortcut for extract + build
jr avatar status     # check profile existence, age, engine, data point counts (JSON to stdout)
```

**Note on `jr avatar edit`**: This is an interactive command (along with the one-time consent prompt on `jr avatar build --engine llm`) — it opens `$EDITOR` (falls back to `vi` if unset). After the editor exits, `jr` validates the modified YAML against the profile schema. If validation fails, it reports the error on stderr and exits with code 4 (validation), leaving the original profile unchanged. If no profile exists, exits with code 3 (not_found) and suggests running `jr avatar refresh` first.

**Note on `jr avatar show`**: Outputs JSON to stdout (consistent with all other `jr` commands). Use `--pretty` for formatted output. The `jr avatar prompt` command serves the human-readable prose use case.

## Data Flow

```
Jira API
  ↓ (JQL queries, changelog, comments, worklogs)
jr avatar extract
  ↓ (extraction.json — structured stats + raw examples)
jr avatar build
  ↓ (profile.yaml — prose + examples + defaults)
jr avatar prompt
  ↓ (formatted text to stdout)
AI Agent (consumes as system prompt context)
```

## Storage Layout

Avatar data is stored under the platform config directory resolved by `os.UserConfigDir()` (e.g., `~/Library/Application Support/jr/` on macOS, `~/.config/jr/` on Linux), consistent with how the existing config and template systems resolve paths. If `os.UserConfigDir()` fails, falls back to `~/.config/jr/avatars/` (same fallback pattern used by the template and audit systems).

```
<config-dir>/jr/avatars/
  <user-hash>/
    extraction.json    # Phase 1 output
    profile.yaml       # Phase 2 output
    cache/             # raw API response cache (TTL 24h)
```

**User hash**: SHA-256 of the Jira `accountId` (not email, since emails can change). The `accountId` is resolved during extraction — either from the `--user` flag (looked up via Jira user search API) or from `/rest/api/3/myself`. The hash is truncated to 16 hex characters for filesystem friendliness.

## Config Integration

New `avatar` section in profile config. This requires adding an `AvatarConfig` struct to the existing `Profile` struct in `internal/config/config.go`:

```go
// New struct to add
type AvatarConfig struct {
    Enabled     bool              `json:"enabled,omitempty"`
    Engine      string            `json:"engine,omitempty"`      // "local" or "llm"
    LLMCmd      string            `json:"llm_cmd,omitempty"`
    MinComments int               `json:"min_comments,omitempty"`
    MinUpdates  int               `json:"min_updates,omitempty"`
    MaxWindow   string            `json:"max_window,omitempty"`
    Overrides   map[string]string `json:"overrides,omitempty"`
}

// Add to existing Profile struct
type Profile struct {
    // ... existing fields ...
    Avatar *AvatarConfig `json:"avatar,omitempty"`
}
```

Example config:

```json
{
  "profiles": {
    "default": {
      "base_url": "...",
      "avatar": {
        "enabled": true,
        "engine": "llm",
        "llm_cmd": "claude -p 'Generate a style profile from this Jira user data.'",
        "min_comments": 50,
        "min_updates": 30,
        "max_window": "6m",
        "overrides": {
          "tone": "Keep it professional, avoid slang",
          "sign_off": "Never use emoji"
        }
      }
    }
  }
}
```

`overrides` are hard rules from the user that always take precedence over extracted patterns.

**Note on `llm_cmd`**: Extraction JSON is always piped via **stdin**. The command string is executed as-is (no template substitution). Example: `"llm_cmd": "claude -p 'Generate a style profile from this Jira user data.'"` receives extraction JSON on stdin and should output profile YAML on stdout.

## Extraction Schema (Phase 1)

### Data Fetching

- Identify target user:
  - `--user email` or `--user accountId` — resolved to Jira `accountId` via user search API
  - No flag → current auth user via `/rest/api/3/myself`
  - If `--user` email/accountId does not match any Jira user → exit code 3 (not_found)
- Fetch in parallel (capped at 3 concurrent requests):
  - Comments authored by user
  - Issues created by user (`reporter = <user>`)
  - Issues assigned to user (`assignee was <user>`)
  - Worklogs by user
- Adaptive time window algorithm:
  - Initial window: 2 weeks from today
  - Expansion: double the window each iteration (2w → 4w → 8w → 16w → ...)
  - Stop when: all data type targets met (comments >= min_comments AND updates >= min_updates) OR window exceeds max_window
  - All data types expand together (single window, not per-type)

### Extraction Structure

```json
{
  "version": 1,
  "meta": {
    "user": "quan@company.com",
    "display_name": "Quan Hoang",
    "extracted_at": "2026-03-18T10:00:00Z",
    "window": { "from": "2026-01-18", "to": "2026-03-18" },
    "data_points": { "comments": 62, "issues_created": 18, "transitions": 145, "worklogs": 34 }
  },

  "writing": {
    "comments": {
      "avg_length_words": 32,
      "median_length_words": 24,
      "length_distribution": { "short_pct": 45, "medium_pct": 40, "long_pct": 15 },
      "formatting": {
        "uses_bullets": 0.65,
        "uses_code_blocks": 0.12,
        "uses_headings": 0.08,
        "uses_emoji": 0.03,
        "uses_mentions": 0.22
      },
      "vocabulary": {
        "common_phrases": ["looks good", "let me check", "blocked by", "will follow up"],
        "jargon": ["regression", "hotfix", "spike"],
        "sign_offs": ["thanks", "lmk"]
      },
      "tone_signals": {
        "question_ratio": 0.18,
        "exclamation_ratio": 0.02,
        "first_person_ratio": 0.35,
        "imperative_ratio": 0.12
      }
    },
    "descriptions": {
      "avg_length_words": 85,
      "structure_patterns": ["acceptance_criteria", "steps_to_reproduce", "background"],
      "formatting": { "uses_bullets": 0.80, "uses_headings": 0.70 }
    }
  },

  "workflow": {
    "field_preferences": {
      "always_sets": ["priority", "labels", "components"],
      "rarely_sets": ["fix_version", "environment"],
      "default_priority": "Medium",
      "common_labels": ["backend", "api", "tech-debt"],
      "common_components": ["auth-service", "core-api"]
    },
    "transition_patterns": {
      "avg_time_in_status": { "In Progress": "2d", "In Review": "4h", "To Do": "1d" },
      "common_sequences": [
        ["To Do", "In Progress", "In Review", "Done"],
        ["To Do", "In Progress", "Blocked", "In Progress", "Done"]
      ],
      "assigns_before_transition": true
    },
    "issue_creation": {
      "types_created": { "Bug": 0.45, "Task": 0.35, "Story": 0.15, "Spike": 0.05 },
      "avg_subtasks_per_story": 3.2
    }
  },

  "interaction": {
    "response_patterns": {
      "median_reply_time": "2h",
      "replies_to_own_issues_pct": 0.85,
      "replies_to_others_pct": 0.40,
      "avg_thread_depth": 2.1
    },
    "mention_habits": {
      "frequently_mentions": ["alice@co.com", "bob@co.com"],
      "mention_context": "blockers_and_reviews"
    },
    "escalation_signals": {
      "blocker_keywords": ["blocked", "waiting on", "need input from"],
      "escalation_pattern": "comments_before_reassign",
      "avg_comments_before_escalation": 3
    },
    "collaboration": {
      "active_hours": { "start": "09:00", "end": "18:00", "timezone": "Asia/Tokyo" },
      "peak_activity_days": ["Tuesday", "Wednesday", "Thursday"],
      "worklog_habits": { "logs_daily": false, "avg_entries_per_week": 4 }
    }
  },

  "examples": {
    "comments": [
      { "issue": "PROJ-123", "date": "2026-03-10", "text": "...", "context": "status_update" },
      { "issue": "PROJ-456", "date": "2026-03-08", "text": "...", "context": "blocker_report" },
      { "issue": "PROJ-789", "date": "2026-02-28", "text": "...", "context": "review_feedback" }
    ],
    "descriptions": [
      { "issue": "PROJ-101", "type": "Bug", "text": "..." },
      { "issue": "PROJ-202", "type": "Story", "text": "..." }
    ]
  }
}
```

Design decisions:
- Ratios over booleans (e.g., `uses_bullets: 0.65` not `true`) so the agent knows frequency
- Examples categorized by context for situational matching
- All data derivable from Jira API — no guessing

## Profile Schema (Phase 2)

```yaml
version: 1
user: "quan@company.com"
display_name: "Quan Hoang"
generated_at: "2026-03-18T10:05:00Z"
engine: "llm"

style_guide:
  writing: |
    Quan writes concise, direct comments — typically 20-35 words. He leads
    with the status or conclusion, then adds context only if needed. Bullet
    points are his default for anything with more than one item. Rarely uses
    emoji or exclamation marks. Code blocks appear when referencing specific
    errors or config. Ends messages with "thanks" or "lmk" but never both.

    For issue descriptions, he's more structured — almost always includes
    headings and acceptance criteria. Bug reports follow a consistent
    steps-to-reproduce format. Stories get a brief background paragraph
    then bullet-pointed requirements.

  workflow: |
    Quan always sets priority, labels, and components when creating issues.
    His go-to labels are "backend", "api", and "tech-debt". He assigns
    himself before transitioning to In Progress. Reviews move fast —
    typically 4 hours in "In Review". He creates subtasks for stories
    (usually 3-4 per story) and prefers Bug and Task types over Stories.

  interaction: |
    Responds to comments on his own issues quickly (within ~2 hours).
    Less active on others' issues unless directly mentioned. When blocked,
    he comments 2-3 times before reassigning or escalating — uses phrases
    like "blocked by", "waiting on", "need input from". Frequently
    collaborates with alice@co.com and bob@co.com, usually in the context
    of reviews and blockers. Most active Tue-Thu, 9am-6pm JST.

overrides:
  - "Keep it professional, avoid slang"
  - "Never use emoji"

defaults:
  priority: "Medium"
  labels: ["backend", "api"]
  components: ["auth-service", "core-api"]
  assign_self_on_transition: true

examples:
  - context: "status_update"
    source: "PROJ-123"
    text: "Fixed the auth timeout — was a misconfigured retry policy. Deployed to staging, will monitor for an hour then merge to prod. lmk if you see anything."

  - context: "blocker_report"
    source: "PROJ-456"
    text: |
      Blocked by PROJ-400 — need the updated API schema before I can
      wire up the new endpoints. @alice can you prioritize?

  - context: "review_feedback"
    source: "PROJ-789"
    text: |
      Looks good overall. Two things:
      - The error handling in `processQueue()` swallows the root cause — need to propagate
      - Missing test for the empty input case
      thanks

  - context: "bug_description"
    source: "PROJ-101"
    text: |
      ## Background
      Login fails intermittently on mobile web.

      ## Steps to Reproduce
      - Open login page on iOS Safari
      - Enter valid credentials
      - Tap "Sign in" — spinner hangs, no response after 30s

      ## Expected
      Redirect to dashboard within 3s

      ## Actual
      Infinite spinner, 504 in network tab
```

`jr avatar prompt` output modes:
- `--format prose` (default): style_guide + overrides + examples as formatted text
- `--format json`: structured profile data
- `--format both`: prose first, JSON appendix
- `--section writing,workflow,interaction`: filter to specific sections
- `--redact`: strip emails and issue keys (replace with placeholders)

## Profile Generation Engines

### Local Engine

Go `text/template` based. Produces functional but formulaic prose from extraction stats. No external dependencies.

Templates branch on patterns (e.g., high bullet ratio → "frequently uses bullet points"). Covers all three sections.

### LLM Engine

Pipes extraction JSON to a user-configured external command via stdin. Expects profile YAML on stdout.

```json
"llm_cmd": "claude -p 'Generate a style profile from this Jira user data.'"
```

No LLM SDK in `jr`. The command is fully user-defined. Examples: Claude CLI, curl to OpenAI, ollama for local models.

Built-in default prompt shipped at `internal/avatar/prompts/build_profile.txt` instructs the LLM to:
1. Synthesize stats into natural prose
2. Select representative examples
3. Identify non-obvious patterns
4. Respect the profile YAML schema

After LLM output, `jr` validates YAML against the profile schema. Invalid output → fallback to local engine with stderr warning.

## Extraction Engine Details

### Package Structure

```
internal/avatar/
  extract.go              — orchestrator: fetch → analyze → output
  fetch.go                — Jira API data fetching with adaptive window
  analyze_writing.go      — comment/description text analysis
  analyze_workflow.go     — transition patterns, field preferences
  analyze_interaction.go  — response times, mention habits, collaboration
  select_examples.go      — pick representative examples across contexts
  extraction.go           — extraction schema types
  build.go                — profile generation orchestrator
  build_local.go          — local template engine
  build_llm.go            — LLM engine (stdin pipe + validation)
  profile.go              — profile schema types
  prompts/
    build_profile.txt     — default LLM prompt
```

### Writing Analysis (pure Go)
- Word counts, sentence lengths → length stats
- Regex-based detection: bullets, headings, code blocks, emoji, @mentions
- N-gram counting for common phrases
- Tone signals: question marks, exclamation marks, first-person pronouns, imperative verbs

### Workflow Analysis
- Parse changelog for status transitions → sequence patterns
- Track fields set on creation → field preferences
- Time between transitions → avg time in status
- Subtask count per parent → subtask habits

### Interaction Analysis
- Comment timestamps on own vs others' issues → response time distribution
- @mention frequency by target → collaboration graph
- Comments before reassignment → escalation pattern
- Worklog timestamps → active hours and weekly patterns

### Example Selection
- Cluster comments by context (status update, blocker, review, question) using keyword heuristics
- From each cluster, pick comment closest to median length
- Cap at ~10 comments + ~3 descriptions (keeps profile under ~2K tokens for LLM context budgets; configurable via extraction params if needed)

## Error Handling & Edge Cases

### Insufficient Data
- Hard minimum: 10 comments + 5 issues (below this, profile is unreliable)
- Exit code 4 (validation) with structured error: `{"error": "insufficient_data", "found": {...}, "required": {...}}`
- Targets (50 comments, 30 updates) control scan window expansion; thresholds are the hard floor

### Rate Limiting
- Uses existing `jr` client which handles 429 with backoff
- Parallel fetches capped at 3 concurrent requests

### Caching
- Raw API responses cached at `<config-dir>/jr/avatars/<user-hash>/cache/` with 24h TTL
- `--no-cache` flag on `extract` to force fresh fetch
- Extraction JSON is the durable cache — `build` can re-run without re-fetching

### Stale Profiles
- `jr avatar prompt` checks profile age. If >30 days (configurable), emits stderr warning
- Does not block output — agent/user decides whether to refresh

### Privacy & Security
- Profiles stored locally only (never sent to Jira)
- `--redact` strips emails and issue keys from prompt output
- First-time LLM engine use (`jr avatar build --engine llm`): consent prompt on stderr asking if user accepts sending extraction data to the configured command. Consent is recorded in config so it's asked only once per profile. `--yes` flag on `build` skips the prompt (intended for non-interactive/CI use).
- Operation policy applies: `denied_operations: ["avatar *"]` blocks avatar commands
- Audit logging captures avatar operations if enabled

### Multi-Profile Support
- Each `jr` profile can have its own avatar config
- Avatars stored per user hash — two profiles for the same Jira user share avatar data

## Testing Strategy

### Unit Tests (table-driven, per analyzer)
- `analyze_writing_test.go` — known comment sets → assert stats
- `analyze_workflow_test.go` — changelog entries → assert transition patterns
- `analyze_interaction_test.go` — timestamped comments → assert response times
- `select_examples_test.go` — clustered comments → assert diversity and selection
- `build_local_test.go` — extraction JSON → assert template output

### Integration Tests
- `fetch_test.go` — mock HTTP server with Jira responses, assert JQL and pagination
- `extract_test.go` — mock server → extraction JSON, validate schema
- `build_test.go` — extraction → profile (local engine), validate schema
- `prompt_test.go` — profile → stdout, assert sections and formatting

### LLM Engine Tests
- `build_llm_test.go` — mock `llm_cmd` as script echoing valid YAML. Tests: stdin piping, YAML validation, fallback on failure
- No actual LLM quality testing

### CLI Tests
- `cmd/avatar_test.go` — flag parsing, subcommand routing, error format
- Follows patterns from `cmd/workflow_test.go`

### Golden File Tests
- `testdata/extraction_sample.json` → `testdata/profile_expected.yaml`
- Catches unintended changes to local engine template output

## Schema Versioning

Both `extraction.json` and `profile.yaml` carry a `version` field (starting at 1). When the schema evolves:
- `jr avatar build` checks extraction version — if outdated, warns on stderr and suggests re-extracting
- `jr avatar prompt` checks profile version — if outdated, warns on stderr and suggests rebuilding
- No automatic migration — re-extract and rebuild is the upgrade path (extraction is derived from Jira, so re-running is always safe)

## `jr avatar status` Command

Outputs JSON to stdout for agent consumption:

```json
{
  "exists": true,
  "user": "quan@company.com",
  "display_name": "Quan Hoang",
  "profile_age_days": 12,
  "extraction_age_days": 12,
  "engine": "llm",
  "schema_version": 1,
  "data_points": { "comments": 62, "issues_created": 18, "transitions": 145, "worklogs": 34 },
  "stale": false
}
```

If no profile exists: `{"exists": false}` with exit code 0 (not an error — informational).
