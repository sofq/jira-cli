<div align="center">
<picture>
  <source media="(prefers-color-scheme: dark)" srcset=".github/readme/banner-dark.svg">
  <img alt="jr — The Jira CLI that speaks JSON, built for AI agents" src=".github/readme/banner.svg" width="900">
</picture>

<br><br>

[![npm](https://img.shields.io/npm/v/jira-jr?style=for-the-badge&logo=npm&logoColor=white&color=7c3aed)](https://www.npmjs.com/package/jira-jr)
[![PyPI](https://img.shields.io/pypi/v/jira-jr?style=for-the-badge&logo=pypi&logoColor=white&color=6d28d9)](https://pypi.org/project/jira-jr)
[![GitHub Release](https://img.shields.io/github/v/release/sofq/jira-cli?style=for-the-badge&logo=github&logoColor=white&color=4f46e5)](https://github.com/sofq/jira-cli/releases)
[![CI](https://img.shields.io/github/actions/workflow/status/sofq/jira-cli/ci.yml?style=for-the-badge&logo=githubactions&logoColor=white&label=CI&color=06b6d4)](https://github.com/sofq/jira-cli/actions/workflows/ci.yml)
[![codecov](https://img.shields.io/codecov/c/github/sofq/jira-cli?style=for-the-badge&logo=codecov&logoColor=white&color=22d3ee)](https://codecov.io/gh/sofq/jira-cli)
[![License](https://img.shields.io/badge/License-Apache_2.0-f59e0b?style=for-the-badge)](LICENSE)

</div>

<br>

> Pure JSON stdout. Structured errors on stderr. Semantic exit codes. **600+ auto-generated commands** from the Jira OpenAPI spec. Zero prompts, zero interactivity — just pipe and parse.

<img src=".github/readme/divider.svg" width="100%" alt="">

## Table of Contents

🔍 [Features](#-features) · 📦 [Install](#-install) · 🚀 [Quick Start](#-quick-start) · ⚡ [Key Commands](#-key-commands) · 🤖 [Agent Integration](#-agent-integration) · 🔒 [Security](#-security) · 🛠 [Development](#-development) · 📄 [License](#-license)

<img src=".github/readme/divider.svg" width="100%" alt="">

## 🔍 Features

<table>
<tr>
<td align="center" width="300">
<img src=".github/readme/icon-schema.svg" width="36" alt=""><br>
<b>Self-Describing</b><br>
<sub><code>jr schema</code> discovers every resource and verb at runtime — no hardcoded command lists</sub>
</td>
<td align="center" width="300">
<img src=".github/readme/icon-token.svg" width="36" alt=""><br>
<b>Token-Efficient</b><br>
<sub><code>--preset</code>, <code>--fields</code>, and <code>--jq</code> stack to shrink 10K tokens down to 50</sub>
</td>
<td align="center" width="300">
<img src=".github/readme/icon-workflow.svg" width="36" alt=""><br>
<b>Workflow Commands</b><br>
<sub>Plain-English ops — no transition IDs, account IDs, or sprint IDs required</sub>
</td>
</tr>
<tr>
<td align="center" width="300">
<img src=".github/readme/icon-batch.svg" width="36" alt=""><br>
<b>Batch Operations</b><br>
<sub>Run N operations in one process from a JSON array — single exit code</sub>
</td>
<td align="center" width="300">
<img src=".github/readme/icon-watch.svg" width="36" alt=""><br>
<b>Real-Time Watch</b><br>
<sub>NDJSON event stream with <code>initial</code>, <code>created</code>, <code>updated</code>, <code>removed</code> events</sub>
</td>
<td align="center" width="300">
<img src=".github/readme/icon-template.svg" width="36" alt=""><br>
<b>Issue Templates</b><br>
<sub>Built-in + custom templates for structured issue creation with variables</sub>
</td>
</tr>
</table>

<p align="right"><a href="#table-of-contents">↑ Back to top</a></p>

<img src=".github/readme/divider.svg" width="100%" alt="">

## 📦 Install

**Using `brew`** &nbsp; [![Homebrew](https://img.shields.io/badge/Homebrew-FBB040?style=flat-square&logo=homebrew&logoColor=white)](#)

```bash
brew install sofq/tap/jr
```

<details>
<summary><b>More install methods</b></summary>

<br>

**Using `npm`** &nbsp; [![npm](https://img.shields.io/badge/npm-CB3837?style=flat-square&logo=npm&logoColor=white)](#)

```bash
npm install -g jira-jr
```

**Using `pip`** &nbsp; [![PyPI](https://img.shields.io/badge/pip-3776AB?style=flat-square&logo=pypi&logoColor=white)](#)

```bash
pip install jira-jr
```

**Using `scoop`** &nbsp; [![Scoop](https://img.shields.io/badge/Scoop-4FC08D?style=flat-square&logoColor=white)](#)

```bash
scoop bucket add sofq https://github.com/sofq/scoop-bucket && scoop install jr
```

**Using `go install`** &nbsp; [![Go](https://img.shields.io/badge/Go-00ADD8?style=flat-square&logo=go&logoColor=white)](#)

```bash
go install github.com/sofq/jira-cli@latest
```

</details>

<p align="right"><a href="#table-of-contents">↑ Back to top</a></p>

<img src=".github/readme/divider.svg" width="100%" alt="">

## 🚀 Quick Start

```bash
# Configure credentials
jr configure --base-url https://yourorg.atlassian.net --token YOUR_API_TOKEN

# Verify the connection
jr configure --test

# Fetch an issue with agent-optimized output
jr issue get --issueIdOrKey PROJ-123 --preset agent
```

> [!TIP]
> Use `jr schema` to discover all available commands, and `jr schema issue get` to see the full flag reference for any operation.

<p align="right"><a href="#table-of-contents">↑ Back to top</a></p>

<img src=".github/readme/divider.svg" width="100%" alt="">

## ⚡ Key Commands

### Workflow — plain English, no Jira internals

```bash
jr workflow move --issue PROJ-123 --to "In Progress" --assign me
jr workflow comment --issue PROJ-123 --text "Fixed in latest deploy"
jr workflow create --project PROJ --type Bug --summary "Login broken" --priority High
jr workflow link --from PROJ-1 --to PROJ-2 --type blocks
jr workflow log-work --issue PROJ-123 --time "2h 30m"
jr workflow sprint --issue PROJ-123 --to "Sprint 5"
```

### Search — JQL with token-efficient output

```bash
jr search search-and-reconsile-issues-using-jql \
  --jql "project = PROJ AND status = 'In Progress'" \
  --fields "key,summary,status" \
  --jq '[.issues[] | {key, summary: .fields.summary}]'
```

### Batch — N operations, one process

```bash
echo '[
  {"command":"issue get","args":{"issueIdOrKey":"PROJ-1"},"jq":".key"},
  {"command":"issue get","args":{"issueIdOrKey":"PROJ-2"},"jq":".key"}
]' | jr batch
```

### Watch — real-time NDJSON stream

```bash
jr watch --jql "project = PROJ" --interval 30s --max-events 10
```

### Templates — structured issue creation

```bash
jr template apply bug-report --project PROJ --var summary="Login broken" --var severity=High
jr template create my-template --from PROJ-123
```

Built-in templates: `bug-report`, `story`, `task`, `epic`, `subtask`, `spike`.

### Diff — structured changelog

```bash
jr diff --issue PROJ-123 --since 2h --field status
```

### Raw — escape hatch for any endpoint

```bash
jr raw GET /rest/api/3/myself
jr raw POST /rest/api/3/search/jql --body '{"jql":"project=PROJ"}'
```

<details>
<summary><b>Error contract</b></summary>

<br>

```json
{"error_type":"rate_limited","status":429,"retry_after":30}
```

| Exit | Meaning | Agent action |
|------|---------|-------------|
| 0 | OK | Parse stdout |
| 2 | Auth failed | Re-authenticate |
| 3 | Not found | Check issue key |
| 5 | Rate limited | Wait `retry_after` |
| 7 | Server error | Retry with backoff |

</details>

<p align="right"><a href="#table-of-contents">↑ Back to top</a></p>

<img src=".github/readme/divider.svg" width="100%" alt="">

## 🤖 Agent Integration

### Claude Code skill (included)

```bash
cp -r skill/jira-cli ~/.claude/skills/    # global install
```

### Any agent

Add to your agent's system instructions:

```
Use `jr` for all Jira operations. Output is always JSON.
Use `jr schema` to discover commands. Use --jq to reduce tokens.
```

> [!NOTE]
> See [`skill/jira-cli/SKILL.md`](skill/jira-cli/SKILL.md) for the full agent integration guide with patterns, error handling, and token optimization strategies.

<p align="right"><a href="#table-of-contents">↑ Back to top</a></p>

<img src=".github/readme/divider.svg" width="100%" alt="">

## 🔒 Security

**Operation policies** — restrict per profile with glob patterns:

```json
{
  "allowed_operations": ["issue get", "search *", "workflow *"]
}
```

**Audit logging** — JSONL to `~/.config/jr/audit.log` via `--audit` flag or per-profile config.

**Batch limits** — default 50, override with `--max-batch N`.

<p align="right"><a href="#table-of-contents">↑ Back to top</a></p>

<img src=".github/readme/divider.svg" width="100%" alt="">

## 🛠 Development

```bash
make generate    # regenerate commands from OpenAPI spec
make build       # compile binary
make test        # run tests with coverage
make lint        # run golangci-lint
```

> [!NOTE]
> Commands in `cmd/generated/` are auto-generated from `spec/jira-v3.json`. Run `make generate` after spec updates.

<p align="right"><a href="#table-of-contents">↑ Back to top</a></p>

<img src=".github/readme/divider.svg" width="100%" alt="">

## 📄 License

[Apache 2.0](LICENSE) &copy; 2026 sofq
