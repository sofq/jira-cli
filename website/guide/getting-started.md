# Getting Started

This guide walks you through installing `jr`, configuring it for your Jira Cloud instance, and running your first commands.

## Installation

::: code-group

```bash [Homebrew]
brew install sofq/tap/jr
```

```bash [npm]
npm install -g jira-jr
```

```bash [pip / uv]
pip install jira-jr
# or
uv tool install jira-jr
```

```bash [Go]
go install github.com/sofq/jira-cli@latest
```

```bash [Scoop (Windows)]
scoop bucket add sofq https://github.com/sofq/scoop-bucket
scoop install jr
```

```bash [Docker]
docker run --rm ghcr.io/sofq/jr version
```

:::

You can also download a pre-built binary from [GitHub Releases](https://github.com/sofq/jira-cli/releases).

Verify the installation:

```bash
jr version
# {"version":"0.x.x"}
```

## Configuration

### Basic auth (default)

The most common setup uses your Atlassian email and an API token. Generate a token at [id.atlassian.com](https://id.atlassian.com/manage-profile/security/api-tokens), then run:

```bash
jr configure \
  --base-url https://yourorg.atlassian.net \
  --token YOUR_API_TOKEN \
  --username your@email.com
```

This writes a config file to `~/.config/jr/config.json`.

To validate your credentials before saving:

```bash
jr configure \
  --base-url https://yourorg.atlassian.net \
  --token YOUR_API_TOKEN \
  --username your@email.com \
  --test
```

To test an already-saved profile:

```bash
jr configure --test                  # test default profile
jr configure --test --profile work   # test a specific profile
```

### Bearer token

For personal access tokens or service accounts that use bearer auth:

```bash
jr configure \
  --base-url https://yourorg.atlassian.net \
  --auth-type bearer \
  --token YOUR_BEARER_TOKEN
```

### OAuth2

OAuth2 requires fields (`client_id`, `client_secret`, `token_url`) that cannot be set via CLI flags, so you must configure it manually in the config file (`~/.config/jr/config.json`):

```json
{
  "profiles": {
    "default": {
      "base_url": "https://yourorg.atlassian.net",
      "auth_type": "oauth2",
      "client_id": "YOUR_CLIENT_ID",
      "client_secret": "YOUR_CLIENT_SECRET",
      "token_url": "https://auth.atlassian.com/oauth/token"
    }
  }
}
```

You can still use `--auth-type oauth2` as a runtime flag override for individual commands.

### Environment variables

Environment variables are ideal for CI pipelines and containerized agents. They override the config file:

```bash
export JR_BASE_URL=https://yourorg.atlassian.net
export JR_AUTH_TOKEN=your-api-token
export JR_AUTH_USER=your@email.com   # optional for bearer/oauth2
```

### Named profiles

If you work with multiple Jira instances, use named profiles:

```bash
# Create a "work" profile
jr configure --base-url https://work.atlassian.net --token TOKEN_A --profile work

# Create a "personal" profile
jr configure --base-url https://personal.atlassian.net --token TOKEN_B --profile personal

# Use a specific profile
jr issue get --profile work --issueIdOrKey PROJ-123

# Delete a profile you no longer need
jr configure --profile work --delete
```

::: tip Configuration resolution order
CLI flags take the highest priority, followed by environment variables, followed by the config file. This means you can override any config file setting with a flag or env var on a per-command basis.
:::

### Security settings

Profiles can include operation restrictions and audit logging. Edit `~/.config/jr/config.json` directly:

```json
{
  "profiles": {
    "agent": {
      "base_url": "https://yourorg.atlassian.net",
      "auth": {"type": "basic", "username": "...", "token": "..."},
      "allowed_operations": ["issue get", "search *", "workflow *"],
      "audit_log": true
    }
  }
}
```

- `allowed_operations` / `denied_operations` — glob patterns restricting which commands the profile can run (use one or the other, not both)
- `audit_log` — write a JSONL entry per operation to `~/.config/jr/audit.log`

See [Global Flags](./global-flags) for `--audit` and `--audit-file` flags.

## Your first commands

### Get an issue

```bash
jr issue get --issueIdOrKey PROJ-123
```

This returns the full JSON representation of the issue on stdout.

### Search with JQL

```bash
jr search search-and-reconsile-issues-using-jql \
  --jql "project = PROJ AND status = 'In Progress'"
```

::: info
Command names are auto-generated from Jira's OpenAPI spec, so they can be verbose. Use `jr schema` to discover the exact name you need (see [Discovering commands](#discovering-commands) below).
:::

### List projects

```bash
jr project search
```

## Workflow commands

`jr workflow` provides high-level commands that accept simple flags instead of raw JSON. These resolve human-friendly names (status names, user emails, sprint names) to Jira IDs automatically.

```bash
# Transition an issue by status name
jr workflow transition --issue PROJ-123 --to "Done"

# Assign by name, email, or "me"
jr workflow assign --issue PROJ-123 --to "me"

# Transition + assign in one step
jr workflow move --issue PROJ-123 --to "In Progress" --assign me

# Add a plain-text comment (auto-converted to ADF)
jr workflow comment --issue PROJ-123 --text "This is done"

# Create an issue from flags (no raw JSON)
jr workflow create --project PROJ --type Bug --summary "Login broken" --description "Steps to reproduce..." --priority High --assign me

# Link two issues by type name
jr workflow link --from PROJ-1 --to PROJ-2 --type blocks

# Log work with human-friendly duration
jr workflow log-work --issue PROJ-123 --time "2h 30m" --comment "Debugging"

# Move issue to sprint by name
jr workflow sprint --issue PROJ-123 --to "Sprint 5"
```

See the full [workflow command reference](/commands/workflow) for all flags and options.

## Next steps

- [Filtering & Presets](./filtering) --- cut 10,000-token responses down to ~50 tokens
- [Discovering Commands](./discovery) --- explore 600+ commands with `jr schema`
- [Templates](./templates) --- create issues from predefined patterns with variables
- [Global Flags](./global-flags) --- full reference for all persistent flags
- [Agent Integration](./agent-integration) --- how AI agents can use `jr` effectively
