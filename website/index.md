---
layout: home

hero:
  name: jr
  text: Agent-Friendly Jira CLI
  tagline: 600+ auto-generated commands from Jira's OpenAPI spec. JSON everywhere. Semantic exit codes. Built for AI agents.
  actions:
    - theme: brand
      text: Get Started
      link: /guide/getting-started
    - theme: alt
      text: Command Reference
      link: /commands/

features:
  - title: Drop-in Agent Skill
    details: Ships with a SKILL.md for Claude Code, Cursor, Codex, Gemini CLI, and 30+ tools. Agents learn the CLI automatically.
  - title: 600+ Commands
    details: Auto-generated from Jira's OpenAPI spec. Every API endpoint as a CLI command, always in sync.
  - title: Semantic Exit Codes
    details: "0=ok, 2=auth, 3=not_found, 4=validation, 5=rate_limited. Branch reliably without parsing errors."
  - title: Token Efficient
    details: "--fields and --jq cut 10,000-token responses to ~50 tokens. Built for LLM context windows."
---

<div class="terminal-section">
  <div class="terminal-window">
    <div class="terminal-header">
      <div class="terminal-dots">
        <span class="dot red"></span>
        <span class="dot yellow"></span>
        <span class="dot green"></span>
      </div>
      <div class="terminal-title">terminal</div>
    </div>
    <div class="terminal-body">

```bash
# Install
brew install sofq/tap/jr

# Configure
jr configure --base-url https://yoursite.atlassian.net \
  --token YOUR_API_TOKEN --username your@email.com

# Get an issue (10,000 tokens → 50 tokens)
jr issue get --issueIdOrKey PROJ-123 \
  --fields key,summary --jq '{key: .key, summary: .fields.summary}'
```

  </div>
  </div>

  <div class="terminal-window output-window">
    <div class="terminal-header">
      <div class="terminal-dots">
        <span class="dot red"></span>
        <span class="dot yellow"></span>
        <span class="dot green"></span>
      </div>
      <div class="terminal-title">output — 50 tokens</div>
    </div>
    <div class="terminal-body">

```json
{"key": "PROJ-123", "summary": "Fix login timeout"}
```

  </div>
  </div>
</div>
