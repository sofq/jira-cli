# Character System & Yolo Mode Design

## Overview

Extend the existing avatar feature into a **character system** — a general-purpose persona abstraction that lets AI agents act on behalf of users in configurable styles. Add a **yolo execution policy** that governs autonomous action-taking, with scope tiers, rate limiting, and audit integration.

The existing avatar (extracted from the user's own Jira data) becomes one source of characters among many: cloned users, hand-crafted YAML, built-in templates, and free-text LLM-generated personas.

## Goals

1. Let agents act autonomously in Jira using a configurable persona
2. Support both interactive batch ("handle these 5 tickets") and autonomous loop ("watch and react while I'm AFK") use cases
3. Make characters composable — mix writing style from one source with workflow patterns from another
4. Enforce safety through scope tiers, rate limits, mandatory audit, and integration with existing operation policies

## Non-Goals

- Webhook-based event triggering (future enhancement; poll-based via `jr watch` internals for now)
- Multi-profile simultaneous yolo (one active yolo config per profile)
- Character marketplace or sharing (local files only)

---

## Character System

### Character File Format

Characters are YAML files stored in `~/.config/jr/characters/`.

```yaml
version: "1"
name: "formal-pm"
source: "manual"           # avatar | clone | manual | template | persona
description: "Professional project manager tone"

style_guide:
  writing: |
    Write in complete sentences. Use bullet points for action items.
    Never use slang or emoji. End with next steps.
  workflow: |
    Always set priority and due date. Prefer subtasks over large issues.
  interaction: |
    Respond within context. Mention stakeholders by name.

defaults:
  priority: "Medium"
  labels: ["managed"]

examples:
  - context: "status_update"
    text: "The authentication module is complete. Remaining items: ..."

# Optional: rule-based reactions for autonomous mode
# Keys use canonical event types prefixed with on_
reactions:
  on_assigned:
    action: transition
    to: "In Progress"
  on_comment_mention:
    action: comment
    text: "Looking into this"
  on_review_requested:
    action: comment
    text: "Will review shortly"
  on_blocked:
    action: comment
    text: "Acknowledged, investigating"
```

### Character Sources

| Source | Command | How It Works |
|--------|---------|-------------|
| Avatar extraction | `jr avatar build` | Existing flow — output also saved as character file |
| Clone another user | `jr character create --from-user alice@co.com` | Extracts their public Jira activity |
| Hand-crafted | `jr character create formal-pm` | Scaffolds empty YAML for manual editing |
| Built-in template | `jr character create --template concise` | Copies from shipped archetype |
| Free-text persona | `jr character create --persona "A sarcastic senior engineer..."` | LLM generates character YAML from description |

### Built-in Templates

Four shipped templates:

| Template | Style |
|----------|-------|
| `concise` | Short sentences, no formatting, action-oriented |
| `detailed` | Thorough explanations, bullet points, context-heavy |
| `formal` | Professional tone, complete sentences, stakeholder-appropriate |
| `casual` | Conversational, contractions, light formatting |

### Composition

Characters are composable by section. A base character provides all sections; optional overrides replace specific sections from other characters.

```bash
# Single character
jr character use formal-pm

# Composed: workflow from my-avatar, writing from formal-pm
jr character use my-avatar --writing formal-pm

# Composed: base is formal-pm, override interaction from my-avatar
jr character use formal-pm --interaction my-avatar
```

Merge rules:
- Each section (`writing`, `workflow`, `interaction`) is an atomic unit — no partial merges within a section
- Later layers override earlier ones
- `defaults` are deep-merged (keys from override replace keys in base, new keys added)
- `examples` are concatenated
- `reactions` are deep-merged (override replaces matching keys)

### Storage

```
~/.config/jr/characters/
  my-avatar.yaml          # from jr avatar build (auto-linked)
  formal-pm.yaml          # hand-crafted
  concise.yaml            # built-in template (copied on first use)
  alice-clone.yaml        # cloned from colleague
```

### Profile-to-Character Conversion

When `jr avatar build` runs, it produces the existing `Profile` YAML and additionally converts it to a character file. The mapping:

| Profile field | Character field |
|--------------|----------------|
| `version` | `version` |
| `user` | (not mapped — characters are persona, not identity) |
| `display_name` | `name` (slugified, e.g., "quan-hoang") |
| — | `source: "avatar"` (always) |
| — | `description` (auto-generated: "Avatar profile for {display_name}") |
| `style_guide.writing` | `style_guide.writing` |
| `style_guide.workflow` | `style_guide.workflow` |
| `style_guide.interaction` | `style_guide.interaction` |
| `defaults.priority` | `defaults.priority` |
| `defaults.labels` | `defaults.labels` |
| `defaults.components` | `defaults.components` |
| `defaults.assign_self_on_transition` | `defaults.assign_self_on_transition` |
| `overrides` | (not mapped — overrides are config-level, not character-level) |
| `examples[].context` | `examples[].context` |
| `examples[].source` | (dropped — not needed in character) |
| `examples[].text` | `examples[].text` |

The character file is saved as `~/.config/jr/characters/{slugified-name}.yaml`. If a file with that name already exists and has `source: "avatar"`, it is overwritten. If it exists with a different source, the build warns and saves as `{name}-avatar.yaml` instead.

### Relationship to Existing Avatar

- All existing `jr avatar` commands remain unchanged (extract, build, refresh, show, edit, status)
- `jr avatar build` additionally saves a character file (see conversion table above)
- `jr avatar prompt` is a convenience alias: it resolves the self-character name from the avatar directory (the character with `source: "avatar"` matching the current profile's user), then delegates to `jr character prompt`. If no self-character exists, it falls back to reading the profile YAML directly (backward compatible)
- `jr avatar status` extended to show linked character name

---

## Yolo Execution Policy

Yolo is a config-driven execution policy that governs what the agent can do autonomously. It's independent of characters — characters define *how* (voice/style), yolo defines *what* and *how much* (permissions/limits).

### Scope Tiers

Yolo scopes use the same **operation string namespace** as the existing operation policy system (`"resource verb"` format, e.g., `"workflow comment"`, `"issue edit"`). Each tier expands to a set of operation patterns:

| Tier | Operation Patterns | Use Case |
|------|-------------------|----------|
| **safe** (default) | `"workflow comment"`, `"workflow transition"`, `"workflow assign"`, `"workflow log-work"` | Low-risk daily work |
| **standard** | safe + `"workflow create"`, `"issue edit"`, `"workflow link"`, `"workflow sprint"` | Active project management |
| **full** | `"*"` minus `"* delete*"`, `"bulk *"`, `"raw *"` | Power user / trusted agent |

Custom scopes via `allowed_actions` / `denied_actions` using glob pattern syntax (same `path.Match` as existing operation policy).

**Precedence:** When both `scope` tier and `allowed_actions`/`denied_actions` are set, they are **intersected** — an action must be allowed by the tier AND not denied by the custom rules. The tier provides the base set; custom rules further restrict it.

### Config

```json
{
  "profiles": {
    "default": {
      "yolo": {
        "enabled": false,
        "scope": "safe",
        "character": null,
        "compose": {},
        "tag": false,
        "tag_text": "[via jr]",
        "rate_limit": { "per_hour": 30, "burst": 10 },
        "allowed_actions": [],
        "denied_actions": [],
        "require_audit": true,
        "decision_engine": "rules",
        "llm_cmd": ""
      }
    }
  }
}
```

Key design decisions:
- `yolo` lives at profile level (not inside `avatar`) — it's an execution concern, not a persona concern
- `require_audit` defaults to `true` — yolo forces audit logging even if the profile doesn't have it globally
- Yolo scope is an **additional restriction** on top of profile operation policies, never a relaxation
- `character` names the base character; `compose` is a map of section overrides: `{"writing": "formal-pm", "interaction": "my-avatar"}`. Valid keys: `writing`, `workflow`, `interaction`. This is the config equivalent of `jr character use base --writing formal-pm --interaction my-avatar`

### Active Character Resolution

The "active character" is resolved in this order:
1. `--character` flag (highest priority, single invocation)
2. `yolo.character` + `yolo.compose` in profile config
3. `jr character use` state (persisted in `~/.config/jr/active-character.json` as `{"profile-name": {"base": "my-avatar", "compose": {"writing": "formal-pm"}}}`)
4. Self-avatar character (if it exists)
5. No character (yolo still works for mechanical actions like transitions; write actions use raw text as-is)

### Attribution

- Default: **silent** — actions look like the user performed them
- Configurable: `tag: true` appends `tag_text` to comments/descriptions
- Audit log always records the truth regardless of tag setting

### How Yolo Applies

**Interactive mode** — existing commands become yolo-aware:
- When yolo enabled (config or `--yolo` flag), character style auto-applied to write actions
- Audit entries tagged with character name and `"yolo": true`

**Batch mode** — yolo policy checked per operation:
- Each op in batch checked against scope
- If within scope: execute
- If outside scope: skip and log as `"denied by yolo policy"`

**The `--yolo` flag** — one-shot override:
```bash
jr workflow transition --issue PROJ-123 --to "Done" --yolo
jr workflow comment --issue PROJ-123 --text "auto" --yolo --character formal-pm
```

### Rate Limiting

- Token bucket algorithm: `burst` tokens, refilled at `per_hour / 60` per minute
- State persisted in `~/.config/jr/yolo-state.json` as a map keyed by profile name: `{"default": {"tokens": 8, "last_refill": "..."}, "staging": {...}}`
- **Interactive mode:** when limit hit, reject with exit code 5 (rate_limited) and JSON error on stderr
- **Autonomous mode:** when limit hit, **drop** the action and log it as skipped with `"reason": "rate_limit"`. No queue — the next poll cycle will re-detect the event if it's still relevant. This avoids stale-action problems and keeps the loop stateless

### Audit Integration

Every yolo action writes an extended audit entry. The existing `audit.Entry` struct gains an optional `Extra map[string]any` field (JSON: `"extra"`, omitted when nil). All yolo-specific metadata goes in `Extra`, preserving the existing flat struct for non-yolo entries:

```json
{
  "ts": "2026-03-23T10:05:00Z",
  "profile": "default",
  "op": "workflow comment",
  "method": "POST",
  "path": "/rest/api/3/issue/PROJ-123/comment",
  "status": 201,
  "exit": 0,
  "dry_run": false,
  "extra": {
    "yolo": true,
    "character": "formal-pm",
    "composed_from": ["my-avatar:workflow", "formal-pm:writing"],
    "trigger": "autonomous",
    "rate_budget_remaining": 22
  }
}
```

When `Extra` is nil (non-yolo operations), the `"extra"` key is omitted entirely — zero impact on existing audit consumers.

---

## Autonomous Loop (`jr avatar act`)

Single new command that wraps `jr watch` internals with a decide-and-execute step.

### Command

```bash
jr avatar act --jql "assignee = me AND updated > -5m" --interval 30s
jr avatar act --dry-run --jql "assignee = me"
jr avatar act --jql "..." --character my-avatar --writing formal-pm
jr avatar act --jql "..." --react-to "status_change,comment,assignment"
jr avatar act --jql "..." --max-actions 10
jr avatar act --jql "project = PROJ AND status = 'To Do'" --once   # one poll cycle, process all matches, exit
```

### Event Loop

```
poll (jr watch internals)
  -> detect changes (new comment, status change, assignment, etc.)
  -> classify event type
  -> check yolo policy: is this action within scope?
     |-- no  -> log as skipped, continue
     |-- yes -> check rate limit
                |-- exceeded -> drop and log
                |-- ok -> decide action
  -> decide action using character + context
  -> apply character style (if write action)
  -> execute via existing jr commands
  -> audit log entry
  -> repeat
```

### Canonical Event Types

All surfaces (character reactions, `--react-to` flag, NDJSON output) use the same event type names:

| Event Type | Trigger |
|-----------|---------|
| `assigned` | Issue assigned to the watched user |
| `unassigned` | Issue unassigned from the watched user |
| `comment_added` | New comment on a watched issue |
| `comment_mention` | New comment that @mentions the watched user |
| `status_change` | Issue status transitioned |
| `priority_change` | Issue priority changed |
| `field_change` | Any other field changed |
| `issue_created` | New issue matching JQL appears |
| `review_requested` | Issue moved to a review status (configurable status names) |
| `blocked` | Issue marked as blocked (via status, label, or blocker link) |
| `sprint_change` | Issue moved to/from a sprint |

Character reaction keys use the same names prefixed with `on_`: `on_assigned`, `on_comment_mention`, etc.

The `--react-to` flag accepts a comma-separated list of these event types to filter which events trigger actions.

### Decision Engine

**Rule-based (default):** Pattern-to-action mapping defined in character's `reactions` section. Deterministic, no external dependencies.

**LLM-assisted (opt-in):** Event context + character profile piped to configured LLM command via stdin as JSON. The LLM must return a JSON object with this schema:

```json
{
  "action": "comment",              // "comment" | "transition" | "assign" | "log-work" | "none"
  "params": {
    "text": "Looking into this",    // for comment
    "to": "In Progress",            // for transition
    "assignee": "me"                // for assign
  },
  "reason": "User was mentioned in a blocking issue"  // optional, logged in audit
}
```

If the LLM returns invalid JSON or an unrecognized action, the event is skipped and logged with `"reason": "invalid_llm_response"`. Timeout: 30 seconds per decision (configurable via `yolo.llm_timeout`).

Engine selection: flag > config `decision_engine` > default (`"rules"`).

### Output

NDJSON to stdout (consistent with `jr watch`):

```json
{"ts":"...","event":"comment_added","issue":"PROJ-123","action":"comment","status":"executed","character":"formal-pm"}
{"ts":"...","event":"assigned","issue":"PROJ-456","action":"transition","status":"skipped","reason":"rate_limit"}
{"ts":"...","event":"status_change","issue":"PROJ-789","action":"none","reason":"no_matching_rule"}
```

### Safety

- `--dry-run` always available
- First run prompts for consent on stderr (skippable with `--yes`)
- SIGINT/SIGTERM gracefully stops and flushes audit log
- Stale character warning (>30 days) on stderr at startup
- Foreground process only — no daemon/background mode. To stop, send SIGINT (Ctrl+C) or SIGTERM

---

## CLI Surface Changes

### New Commands

| Command | Purpose |
|---------|---------|
| `jr character list` | List all characters (source, age, active) |
| `jr character show <name>` | Display character YAML |
| `jr character create <name>` | Scaffold empty character YAML |
| `jr character create --from-user <email>` | Clone another user's style |
| `jr character create --persona "..."` | LLM-generate from free text |
| `jr character create --template <name>` | Copy from built-in template |
| `jr character edit <name>` | Open in $EDITOR |
| `jr character delete <name>` | Remove character file |
| `jr character use <name> [--writing X] [--workflow Y] [--interaction Z]` | Set active character with optional composition |
| `jr character prompt [--format prose\|json\|both] [--redact]` | Output active composed character |
| `jr avatar act [flags]` | Autonomous execution loop |
| `jr yolo status` | Show yolo state: enabled, scope, rate budget, active character |
| `jr yolo history [--since 1h]` | Review past yolo actions from audit log |

### New Global Flags

| Flag | Description |
|------|-------------|
| `--yolo` | Enable yolo mode for this invocation |
| `--character <name>` | Override active character for this invocation |

### Modified Commands

| Command | Change |
|---------|--------|
| `jr avatar build` | Also saves output as character file |
| `jr avatar prompt` | Delegates to `jr character prompt` for self-character |
| `jr avatar status` | Extended with linked character name |
| All `jr workflow *` | Respect yolo policy when enabled |
| `jr batch` | Yolo policy checked per operation |

### Unchanged

- All existing `jr avatar` subcommands (extract, build, refresh, show, edit, status)
- Operation policies (`allowed_operations`/`denied_operations`)
- Audit logging infrastructure (extended, not replaced)
- `jr watch` (wrapped by `jr avatar act`, not modified)

---

## Implementation Considerations

### Package Structure

- `internal/character/` — character loading, composition, validation, storage
- `internal/yolo/` — execution policy, scope tiers, rate limiter, state management
- `cmd/character.go` — character subcommand tree
- `cmd/yolo.go` — yolo status/history commands
- Extend `cmd/avatar.go` — add `act` subcommand
- Extend `cmd/root.go` — add `--yolo` and `--character` global flags

### Dependencies on Existing Code

- `internal/avatar/` — extraction, build pipeline (character source)
- `internal/policy/` — glob matching reused for yolo scope
- `internal/audit/` — extended with `Extra map[string]any` field
- `internal/config/` — new `YoloConfig` struct on `Profile`
- `internal/watch/` — **requires refactoring**: current `watcher.Run()` is monolithic (writes NDJSON directly to stdout). `jr avatar act` needs a per-event callback to feed events into the decision engine. Refactor `Run()` to accept an `EventHandler func(Event) error` callback, with the existing NDJSON-to-stdout behavior as the default handler. This is a prerequisite for `jr avatar act`
