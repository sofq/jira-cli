---
layout: home
customHero: true

features:
  - title: "10,000 tokens → 50"
    details: "--fields, --jq, and --preset strip Jira's bloated responses down to exactly what you need. Your LLM's context window will thank you."
  - title: Drop-in Agent Skill
    details: "Ships with SKILL.md. Claude Code, Cursor, Codex, Gemini CLI — agents learn the CLI in one read. Zero prompt engineering."
  - title: Workflow in Plain English
    details: "jr workflow move --issue PROJ-1 --to 'Done' --assign me. No more hunting for transition IDs or account IDs."
  - title: Templates
    details: "jr template apply bug-report --var summary='Login broken'. Predefined issue patterns with variables. Create from scratch or clone from existing issues."
  - title: Real-time Watch
    details: "jr watch --jql 'status changed' --interval 30s. NDJSON stream of changes. Build automations that react to Jira in real time."
  - title: Diff & Changelog
    details: "jr diff --issue PROJ-1 --since 2h. See exactly what changed, when, and by whom. Structured JSON, not a wall of text."
  - title: Batch & Parallel
    details: "jr batch --parallel 5 runs operations concurrently. One process, one output array. 50 ops max by default."
  - title: Security Built In
    details: "Operation policies per profile. Audit logging. Batch limits. Lock down what agents can do — allowed_operations or denied_operations with glob patterns."
  - title: 600+ Commands
    details: "Auto-generated from Jira's OpenAPI spec. Every endpoint is a CLI command, always in sync. Run jr schema to explore."
  - title: Context
    details: "jr context PROJ-1 — issue, comments, and changelog in one call. Three API calls collapsed into one command."
  - title: Pipe
    details: "jr pipe 'search ...' 'workflow transition --to Done' --parallel 5. Chain commands together. Source → extract → target."
  - title: Retry & Resilience
    details: "--retry 3 for exponential backoff on 429/5xx. --parallel for concurrent batch. Built-in fault tolerance."
  - title: Avatar
    details: "jr avatar build — analyze Jira activity to build user style profiles. Agents adapt to how you work."
---

<!-- Why jr section -->
<div class="why-section">
  <h2>Why another Jira CLI?</h2>
  <div class="why-grid">
    <div class="why-card">
      <div class="why-label">Other CLIs</div>
      <div class="why-content">Human-readable tables. Agents can't parse them. Manual flag lookup. Breaks when Jira updates APIs.</div>
    </div>
    <div class="why-card highlight">
      <div class="why-label">jr</div>
      <div class="why-content">JSON everywhere. Agents read it natively. Ships with SKILL.md — agents learn it in one read. Auto-generated from OpenAPI — never out of date.</div>
    </div>
  </div>
</div>

<!-- Killer demos -->
<div class="terminal-section">
  <h2 class="section-title">See it in action</h2>

  <div class="terminal-window">
    <div class="terminal-header">
      <div class="terminal-dots">
        <span class="dot red"></span>
        <span class="dot yellow"></span>
        <span class="dot green"></span>
      </div>
      <div class="terminal-title">context — full issue context in one call</div>
    </div>
    <div class="terminal-body">

```bash
jr context PROJ-123 --jq '{key: .issue.key, summary: .issue.fields.summary, comments: [.comments.comments[].body]}'
```

```json
{
  "key": "PROJ-123",
  "summary": "Login page 500 on Safari",
  "comments": [
    "Reproduced on Safari 17. Stacktrace attached.",
    "Fix deployed to staging — please verify."
  ]
}
```

  </div>
  </div>

  <div class="terminal-window">
    <div class="terminal-header">
      <div class="terminal-dots">
        <span class="dot red"></span>
        <span class="dot yellow"></span>
        <span class="dot green"></span>
      </div>
      <div class="terminal-title">pipe — chain commands, fan out in parallel</div>
    </div>
    <div class="terminal-body">

```bash
jr pipe \
  "search search-and-reconsile-issues-using-jql --jql 'project=PROJ AND status=\"In Progress\"'" \
  "workflow transition --to Done" \
  --parallel 5
```

```json
[
  {"index":0,"exit_code":0,"data":{}},
  {"index":1,"exit_code":0,"data":{}},
  {"index":2,"exit_code":0,"data":{}}
]
```

  </div>
  </div>

  <div class="terminal-window">
    <div class="terminal-header">
      <div class="terminal-dots">
        <span class="dot red"></span>
        <span class="dot yellow"></span>
        <span class="dot green"></span>
      </div>
      <div class="terminal-title">templates — create issues from patterns</div>
    </div>
    <div class="terminal-body">

```bash
jr template apply bug-report \
  --project PROJ \
  --var summary="Login page 500 on Safari" \
  --var severity=High \
  --var steps="1. Open Safari\n2. Click Login\n3. 500 error"
```

  </div>
  </div>

  <div class="terminal-window">
    <div class="terminal-header">
      <div class="terminal-dots">
        <span class="dot red"></span>
        <span class="dot yellow"></span>
        <span class="dot green"></span>
      </div>
      <div class="terminal-title">watch — real-time NDJSON stream</div>
    </div>
    <div class="terminal-body">

```bash
jr watch --jql "project = PROJ AND status changed" --interval 30s
```

```json
{"event":"changed","key":"PROJ-1","field":"status","from":"Open","to":"In Progress","time":"14:32:01"}
{"event":"changed","key":"PROJ-5","field":"assignee","from":"null","to":"alice","time":"14:32:30"}
```

  </div>
  </div>

  <div class="terminal-window">
    <div class="terminal-header">
      <div class="terminal-dots">
        <span class="dot red"></span>
        <span class="dot yellow"></span>
        <span class="dot green"></span>
      </div>
      <div class="terminal-title">diff — structured changelog</div>
    </div>
    <div class="terminal-body">

```bash
jr diff --issue PROJ-123 --since 2h --field status,assignee
```

```json
[
  {"field":"status","from":"Open","to":"In Progress","author":"alice","time":"14:20:00"},
  {"field":"assignee","from":"null","to":"alice","time":"14:20:00"}
]
```

  </div>
  </div>

  <div class="terminal-window">
    <div class="terminal-header">
      <div class="terminal-dots">
        <span class="dot red"></span>
        <span class="dot yellow"></span>
        <span class="dot green"></span>
      </div>
      <div class="terminal-title">context — full issue in one call</div>
    </div>
    <div class="terminal-body">

```bash
jr context PROJ-123 --jq '{key: .issue.key, status: .issue.fields.status.name, comments: (.comments.comments | length), changes: (.changelog | length)}'
```

```json
{"key":"PROJ-123","status":"In Progress","comments":5,"changes":12}
```

  </div>
  </div>

  <div class="terminal-window">
    <div class="terminal-header">
      <div class="terminal-dots">
        <span class="dot red"></span>
        <span class="dot yellow"></span>
        <span class="dot green"></span>
      </div>
      <div class="terminal-title">pipe — chain source → extract → target</div>
    </div>
    <div class="terminal-body">

```bash
jr pipe "search search-and-reconsile-issues-using-jql --jql 'project=PROJ AND status=Done'" \
  "workflow transition --to Closed" \
  --extract "[.issues[].key]" --inject issueIdOrKey --parallel 5
```

  </div>
  </div>

  <div class="terminal-window">
    <div class="terminal-header">
      <div class="terminal-dots">
        <span class="dot red"></span>
        <span class="dot yellow"></span>
        <span class="dot green"></span>
      </div>
      <div class="terminal-title">doctor — health check</div>
    </div>
    <div class="terminal-body">

```bash
jr doctor
```

```json
{"status":"healthy","checks":[{"name":"config","status":"pass","message":"Config file found"},{"name":"auth","status":"pass","message":"Authenticated as alice@company.com"},{"name":"connectivity","status":"pass","message":"Jira Cloud reachable (204ms)"}]}
```

  </div>
  </div>
</div>
