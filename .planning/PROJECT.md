# jr Agent Intelligence — Rules, Playbooks & Autonomous Loop

## What This Is

Making jr's avatar/character features the core of the tool by adding agent-readable team rules, playbooks, and an autonomous event loop. The agent (Claude Code, Gemini CLI, Codex, etc.) is the brain — jr provides the eyes (watch events), hands (execute commands), and instructions (Markdown-based rules and playbooks). Designed to be dead simple for "lazy" people: Markdown files, natural language, no YAML rule engines.

## Core Value

**Any AI agent can act as me on Jira — following my team's rules, in my voice — with zero friction.**

## Requirements

### Validated

- ✓ Avatar style profiling (extract + build) — existing
- ✓ Character persona management — existing
- ✓ Watch event stream (NDJSON polling) — existing
- ✓ Avatar act autonomous loop (decision output) — existing (actions not executed)
- ✓ All workflow commands (comment, transition, assign, etc.) — existing
- ✓ Context command (issue + comments + changelog) — existing
- ✓ `--llm-cmd` pattern for LLM invocation — existing (avatar build)

### Active

- [ ] Agent skill system: SKILL.md + references/ that tell any agent how to act as me on Jira
- [ ] `jr agent init` scaffolds SKILL.md + references/ with good defaults and jr command reference
- [ ] `jr agent prompt` renders full agent context (skill + references + character) for debugging/preview
- [ ] `jr agent import <url-or-path>` imports someone else's rules/playbooks
- [ ] Event enrichment: changelog-based classification into 11 canonical event types (assigned, status_change, comment_added, etc.) instead of just "created"/"field_change"
- [ ] Watch improvements: pagination (>50 issues), seen-state persistence (survives restart), retry on fetch
- [ ] Avatar act v2: on events → gather SKILL.md + references + event data → call $LLM_CMD → agent executes jr commands → jr logs results
- [ ] Avatar act audit trail: log every agent action with timestamp, issue, action taken, agent response
- [ ] `--dry-run` for avatar act: agent decides but doesn't execute (preview mode)
- [ ] Built-in reference templates: team-rules.md, standup.md, triage.md, handoff.md, stale-check.md, sprint-prep.md
- [ ] Character v2: character references rules + playbooks, `jr character prompt` includes full context
- [ ] Rate limiting enforcement in avatar act (wire existing RateLimiter)

### Out of Scope

- YAML rule engine with condition evaluation — agent handles rule interpretation natively
- Convention checker (Go code) — agent reads rules in natural language and checks
- Playbook executor (Go code) — agent reads playbook .md and follows steps
- Built-in LLM API client (anthropic-sdk-go, openai-go) — use `--llm-cmd` subprocess pattern, provider-agnostic
- MCP server for jr — defer to future, `-p` mode is universal across agent CLIs
- Webhook/push-based event delivery — Jira doesn't support push to external CLIs, polling is the only option
- Daemon/service management — user runs in tmux/screen/launchd, jr doesn't manage itself
- GUI/web interface — CLI-first, agent-first

## Context

### Architecture: Agent is the Brain, jr is Eyes + Hands

```
jr avatar act (long-running Go process)
    │
    │ every N minutes:
    ├── poll Jira (eyes) → detect events
    ├── gather context: SKILL.md + references/ + character + events
    └── call $LLM_CMD (brain) → agent reads instructions, decides, acts
        │
        └── agent calls jr commands (hands):
            jr workflow comment, jr workflow transition, etc.
```

### Three Usage Modes

1. **Autonomous loop** (`jr avatar act --interval 10m`): jr watches, calls agent on events, agent acts unattended
2. **On-demand** (human in Claude Code: "check my jira"): agent reads SKILL.md, uses jr commands
3. **Simple commands** (human runs `jr` directly): existing CLI usage, no agent involved

### Agent Skill Structure

```
~/.config/jr/agent/
    SKILL.md                    # Entry point: "how to be me on Jira"
    references/
        team-rules.md           # Team policies (natural language)
        my-style.md             # Writing style (from avatar build)
        standup.md              # How to write standups
        triage.md               # How to triage issues
        handoff.md              # How to do handoffs
        sprint-prep.md          # How to prep sprints
```

### LLM Integration: --llm-cmd (Provider-Agnostic)

All major agent CLIs support `-p` mode (non-interactive pipe):
- `claude -p --bare --output-format json`
- `gemini -p --output-format json`
- `cline -y --json`
- `copilot -p -s`
- `codex exec --json`

jr calls `$LLM_CMD` as a subprocess. No LLM SDK in jr. No vendor lock-in.

### Why Not a Go Rule Engine?

Research showed that:
- Jira already has built-in automation for simple event→action rules
- The differentiator is LLM-powered intelligent decisions (triage, context-aware responses)
- Natural language rules + agent brain replaces ~30 Go files with ~5 Markdown files
- Agents can interpret nuance ("if the bug seems security-related, escalate") that condition evaluators can't

### Current Watch Limitations (to fix)

- Event types collapse to only 2 of 11 (need changelog enrichment)
- No pagination (queries >50 issues silently truncate)
- No seen-state persistence (restart = re-emit all events)
- No retry on client.Fetch
- Actions never execute (avatar act only logs decisions)
- Rate limiter implemented but not wired

## Constraints

- **Tech stack**: Go (existing codebase), no new LLM SDK dependencies
- **LLM integration**: `--llm-cmd` subprocess only, provider-agnostic
- **Agent compatibility**: Must work with Claude Code, Gemini CLI, Codex, and any tool supporting `-p` mode
- **Simplicity**: Markdown-based configuration, no YAML rule engines, designed for "lazy" people
- **Backward compat**: Existing character v1 and avatar act behavior must still work

## Key Decisions

| Decision | Rationale | Outcome |
|----------|-----------|---------|
| Agent is the brain, not jr | AI agents interpret rules better than Go condition evaluators. Eliminates ~30 files of rule engine code. | — Pending |
| Markdown rules, not YAML | Natural language is simpler to write, read, and share. Agents consume it natively. | — Pending |
| --llm-cmd subprocess, no SDK | -p mode is universal across agent CLIs. No vendor lock-in. Zero Go dependencies. | — Pending |
| Conventions merged into rules | A convention is just a rule the agent checks on-demand. No separate subsystem needed. | — Pending |
| Drop Go playbook executor | Agent reads playbook .md and follows steps. No step-by-step Go executor needed. | — Pending |
| Polling, not webhooks | Jira doesn't push to external CLIs. Polling with enrichment is the realistic path. | — Pending |

## Evolution

This document evolves at phase transitions and milestone boundaries.

**After each phase transition** (via `/gsd:transition`):
1. Requirements invalidated? → Move to Out of Scope with reason
2. Requirements validated? → Move to Validated with phase reference
3. New requirements emerged? → Add to Active
4. Decisions to log? → Add to Key Decisions
5. "What This Is" still accurate? → Update if drifted

**After each milestone** (via `/gsd:complete-milestone`):
1. Full review of all sections
2. Core Value check — still the right priority?
3. Audit Out of Scope — reasons still valid?
4. Update Context with current state

---
*Last updated: 2026-03-23 after initialization*
