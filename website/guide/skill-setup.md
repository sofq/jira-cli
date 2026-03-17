# Skill Setup

`jr` ships with a skill file (`SKILL.md`) that teaches AI agents how to use the CLI — runtime command discovery, token-efficient patterns, error handling, and batch operations.

The skill follows the [Agent Skills](https://agentskills.io) open standard, supported by 30+ tools including Claude Code, Cursor, VS Code Copilot, OpenAI Codex, Gemini CLI, Goose, and more.

## Download the Skill

```bash
curl -sL https://raw.githubusercontent.com/sofq/jira-cli/main/skill/jira-cli/SKILL.md \
  -o SKILL.md
```

Or copy from a local `jr` install:

```bash
cp $(brew --prefix jr)/share/jr/skill/jira-cli/SKILL.md SKILL.md
```

## Setup by Tool

### Claude Code

| Scope | Path |
|-------|------|
| Project (shared via git) | `.claude/skills/jira-cli/SKILL.md` |
| Personal (all projects) | `~/.claude/skills/jira-cli/SKILL.md` |

```bash
mkdir -p .claude/skills/jira-cli
cp SKILL.md .claude/skills/jira-cli/SKILL.md
```

Grant permission to run `jr`:

```json
// .claude/settings.json
{
  "permissions": {
    "allow": ["Bash(jr *)"]
  }
}
```

Claude loads the skill automatically when you mention Jira, or invoke it directly with `/jira-cli`.

See [Claude Code skills docs](https://code.claude.com/docs/en/skills) for more details.

### Cursor

| Scope | Path |
|-------|------|
| Project | `.cursor/skills/jira-cli/SKILL.md` or `.agents/skills/jira-cli/SKILL.md` |
| Personal | `~/.cursor/skills/jira-cli/SKILL.md` |

```bash
mkdir -p .cursor/skills/jira-cli
cp SKILL.md .cursor/skills/jira-cli/SKILL.md
```

Cursor discovers skills automatically at startup and applies them when relevant. Invoke manually with `/jira-cli` in Agent chat.

See [Cursor skills docs](https://cursor.com/docs/context/skills) for more details.

### VS Code Copilot

| Scope | Path |
|-------|------|
| Project | `.github/skills/jira-cli/SKILL.md` or `.agents/skills/jira-cli/SKILL.md` |
| Personal | `~/.copilot/skills/jira-cli/SKILL.md` |

```bash
mkdir -p .github/skills/jira-cli
cp SKILL.md .github/skills/jira-cli/SKILL.md
```

Copilot discovers skills progressively — reads metadata first, loads full instructions when relevant.

See [VS Code skills docs](https://code.visualstudio.com/docs/copilot/customization/agent-skills) for more details.

### OpenAI Codex

| Scope | Path |
|-------|------|
| Project | `.agents/skills/jira-cli/SKILL.md` |
| Personal | `~/.agents/skills/jira-cli/SKILL.md` |
| System | `/etc/codex/skills/jira-cli/SKILL.md` |

```bash
mkdir -p .agents/skills/jira-cli
cp SKILL.md .agents/skills/jira-cli/SKILL.md
```

Codex scans `.agents/skills/` from the current directory up to the repo root. Invoke with `/skills` or `$jira-cli` in the prompt.

See [Codex skills docs](https://developers.openai.com/codex/skills/) for more details.

### Gemini CLI

| Scope | Path |
|-------|------|
| Workspace | `.gemini/skills/jira-cli/SKILL.md` or `.agents/skills/jira-cli/SKILL.md` |
| Personal | `~/.gemini/skills/jira-cli/SKILL.md` or `~/.agents/skills/jira-cli/SKILL.md` |

```bash
mkdir -p .gemini/skills/jira-cli
cp SKILL.md .gemini/skills/jira-cli/SKILL.md
```

Gemini CLI injects skill descriptions into the system prompt at session start. When a matching task is detected, it calls `activate_skill` with user confirmation.

::: tip
`.agents/skills/` takes precedence over `.gemini/skills/` when both exist.
:::

See [Gemini CLI skills docs](https://geminicli.com/docs/cli/skills/) for more details.

### Goose

| Scope | Path |
|-------|------|
| Project | `.goose/skills/jira-cli/SKILL.md` or `.agents/skills/jira-cli/SKILL.md` |

```bash
mkdir -p .goose/skills/jira-cli
cp SKILL.md .goose/skills/jira-cli/SKILL.md
```

See [Goose skills docs](https://block.github.io/goose/docs/guides/context-engineering/using-skills/) for more details.

### Universal Path (`.agents/skills/`)

Most tools scan `.agents/skills/` as a vendor-neutral directory. If your tool isn't listed above, or you want one path that works across multiple tools:

```bash
mkdir -p .agents/skills/jira-cli
cp SKILL.md .agents/skills/jira-cli/SKILL.md
```

This works with Codex, Cursor, VS Code Copilot, Gemini CLI, Goose, and others that support the [Agent Skills](https://agentskills.io) standard.

## What the Skill Teaches

The `SKILL.md` teaches agents to:

| Capability | How |
|-----------|-----|
| **Discover commands** | `jr schema` for runtime discovery — no hardcoded knowledge |
| **Minimize tokens** | Always combine `--fields` + `--jq` (10,000 → 50 tokens) |
| **Batch calls** | `jr batch` for multiple operations in one process |
| **Handle errors** | Branch on exit codes: 0=ok, 2=auth, 3=not_found, 5=rate_limited |
| **Configure auth** | Setup with `jr configure`, profiles, env vars |
| **Troubleshoot** | Common issues: auth failures, unknown commands, large responses |

## For Tools Without Skill Support

If your tool doesn't support the Agent Skills standard, add this to the system prompt or agent instructions:

```
You have access to `jr`, an agent-friendly Jira CLI.
- All output is JSON on stdout. Errors are JSON on stderr.
- Use --fields and --jq to limit token usage.
- Exit codes: 0=ok, 2=auth, 3=not_found, 4=validation, 5=rate_limited.
- Run `jr schema` to discover commands at runtime.
- Use `jr batch` to run multiple operations efficiently.
```
