# jr — Agent-friendly Jira CLI

[![CI](https://github.com/sofq/jira-cli/actions/workflows/ci.yml/badge.svg)](https://github.com/sofq/jira-cli/actions/workflows/ci.yml)
[![Release](https://github.com/sofq/jira-cli/actions/workflows/release.yml/badge.svg)](https://github.com/sofq/jira-cli/releases)
[![License: MIT](https://img.shields.io/badge/License-MIT-blue.svg)](LICENSE)

A CLI for Jira that outputs structured JSON, supports jq filtering, and auto-generates commands from the Jira OpenAPI spec. Designed for scripting, automation, and AI agent integration.

## Features

- **Structured JSON output** — every response is valid JSON, errors go to stderr as JSON
- **Built-in jq filtering** — `--jq '.issues[].key'` on any command
- **OpenAPI-driven** — commands are generated from the official Jira REST API v3 spec
- **Pagination** — automatic, transparent pagination with `--no-paginate` opt-out
- **Batch operations** — execute multiple API calls in one invocation
- **Multiple profiles** — switch between Jira instances with `--profile`
- **Dry-run mode** — preview requests with `--dry-run`
- **Shell completions** — bash, zsh, fish, powershell

## Install

### Homebrew

```bash
brew install sofq/tap/jr
```

### Go

```bash
go install github.com/sofq/jira-cli@latest
```

### Binary releases

Download from [GitHub Releases](https://github.com/sofq/jira-cli/releases).

## Quick start

```bash
# Configure your Jira instance
jr configure --base-url https://yourorg.atlassian.net --token YOUR_API_TOKEN

# List issues in a project
jr issue search --jql "project = PROJ ORDER BY created DESC" --jq '.issues[].key'

# Get a specific issue
jr issue get --issueIdOrKey PROJ-123

# Create an issue
jr issue create --body '{"fields":{"project":{"key":"PROJ"},"summary":"Bug report","issuetype":{"name":"Bug"}}}'

# Batch operations
jr batch --body '[{"method":"GET","path":"/rest/api/3/issue/PROJ-1"},{"method":"GET","path":"/rest/api/3/issue/PROJ-2"}]'
```

## Configuration

`jr` resolves configuration in this order (highest priority first):

1. CLI flags (`--base-url`, `--auth-token`, etc.)
2. Environment variables (`JR_BASE_URL`, `JR_AUTH_TOKEN`, etc.)
3. Config file (`~/.config/jr/config.json`)

### Environment variables

| Variable | Description |
|----------|-------------|
| `JR_BASE_URL` | Jira instance URL |
| `JR_AUTH_TYPE` | `basic` or `bearer` |
| `JR_AUTH_USER` | Username (for basic auth) |
| `JR_AUTH_TOKEN` | API token |

### Config file

```bash
jr configure --base-url https://yourorg.atlassian.net --token YOUR_TOKEN
```

Use `--profile` to manage multiple instances:

```bash
jr configure --profile work --base-url https://work.atlassian.net --token TOKEN1
jr configure --profile personal --base-url https://personal.atlassian.net --token TOKEN2
jr issue search --profile work --jql "assignee = currentUser()"
```

## Global flags

| Flag | Description |
|------|-------------|
| `--jq <expr>` | Apply a jq filter to the response |
| `--pretty` | Pretty-print JSON output |
| `--no-paginate` | Disable automatic pagination |
| `--verbose` | Log HTTP details to stderr |
| `--dry-run` | Print the request without executing |
| `--profile <name>` | Use a named config profile |

## Shell completion

```bash
# Bash
jr completion bash > /etc/bash_completion.d/jr

# Zsh
jr completion zsh > "${fpath[1]}/_jr"

# Fish
jr completion fish > ~/.config/fish/completions/jr.fish
```

## Development

```bash
# Build
make build

# Run tests
make test

# Regenerate commands from OpenAPI spec
make generate

# Update the OpenAPI spec
make spec-update
```

## License

[MIT](LICENSE)
