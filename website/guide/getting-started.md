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

### Bearer token

For personal access tokens or service accounts that use bearer auth:

```bash
jr configure \
  --base-url https://yourorg.atlassian.net \
  --auth-type bearer \
  --token YOUR_BEARER_TOKEN
```

### OAuth2

```bash
jr configure \
  --base-url https://yourorg.atlassian.net \
  --auth-type oauth2 \
  --token YOUR_OAUTH2_ACCESS_TOKEN
```

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

## Filtering output

Jira API responses are large. A single issue can be 10,000+ tokens of JSON. `jr` provides two flags to cut that down dramatically.

### `--fields` --- server-side filtering

The `--fields` flag tells Jira to return only the fields you ask for. This reduces what comes over the wire:

```bash
jr issue get --issueIdOrKey PROJ-123 --fields key,summary,status,assignee
```

### `--jq` --- client-side filtering

The `--jq` flag applies a [jq](https://jqlang.github.io/jq/) expression to the response, reshaping or extracting data:

```bash
jr issue get --issueIdOrKey PROJ-123 --jq '{key: .key, summary: .fields.summary}'
```

### Combine both for maximum efficiency

Use `--fields` and `--jq` together to minimize tokens at every stage:

**Before** (no filtering) --- ~10,000 tokens:
```bash
jr issue get --issueIdOrKey PROJ-123
```

**After** (both flags) --- ~50 tokens:
```bash
jr issue get --issueIdOrKey PROJ-123 \
  --fields key,summary \
  --jq '{key: .key, summary: .fields.summary}'
```

::: tip
Always use `--fields` and `--jq` together. `--fields` reduces what Jira sends back (saving bandwidth and API quota), while `--jq` shapes the output into exactly the structure you need.
:::

### Cache read-heavy data

For data that changes infrequently (like project lists), use `--cache` to avoid redundant API calls:

```bash
jr project search --cache 5m --jq '[.values[].key]'
```

## Discovering commands

`jr` has over 600 commands, all auto-generated from the official Jira OpenAPI v3 spec. Rather than memorizing them, use `jr schema` to explore what is available.

### Four discovery modes

**1. Resource-to-verb mapping** (default, best starting point):
```bash
jr schema
# Shows every resource and its available verbs
```

**2. List all resource names:**
```bash
jr schema --list
# issue, project, search, workflow, board, sprint, ...
```

**3. All operations for a resource:**
```bash
jr schema issue
# Lists every operation under the "issue" resource, with flags
```

**4. Full schema for a single operation:**
```bash
jr schema issue get
# Shows all available flags, types, and descriptions for "issue get"
```

::: tip
Start with `jr schema` or `jr schema --list` to orient yourself, then drill into a specific resource and operation. This is especially useful for AI agents that need to discover commands at runtime.
:::

## Next steps

- [Global Flags](./global-flags) --- full reference for all persistent flags
- [Agent Integration](./agent-integration) --- how AI agents can use `jr` effectively
