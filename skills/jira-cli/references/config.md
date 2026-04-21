# Config, auth, profiles, security

## Setup

### Basic auth (username + API token)

The default. Most Jira Cloud users authenticate this way.

```bash
jr configure \
  --base-url https://yoursite.atlassian.net \
  --token YOUR_API_TOKEN \
  --username your@email.com
```

Generate API tokens at https://id.atlassian.com/manage-profile/security/api-tokens.

### Bearer auth

```bash
jr configure --base-url https://... --token YOUR_BEARER_TOKEN --auth-type bearer
```

### OAuth2

**Cannot be set up via `jr configure`.** Edit `~/.config/jr/config.json` manually:

```json
{
  "profiles": {
    "default": {
      "base_url": "https://yoursite.atlassian.net",
      "auth": {
        "type": "oauth2",
        "client_id": "...",
        "client_secret": "...",
        "token_url": "https://auth.atlassian.com/oauth/token"
      }
    }
  }
}
```

### Validate credentials

```bash
jr configure --test                      # test the default profile
jr configure --test --profile work       # test a specific profile

# Validate AND save in one step
jr configure --base-url https://... --token ... --username ... --test
```

Exit code 2 means auth failed — the token is likely expired or the username is wrong.

## Config resolution order

Highest precedence first:

1. **CLI flags** — `--base-url`, `--auth-token`, `--auth-user`, `--auth-type`
2. **Environment variables**
3. **Config file** — `~/.config/jr/config.json` (default) or `$JR_CONFIG_PATH`

This means you can override a saved profile for one invocation:

```bash
jr issue get --issueIdOrKey PROJ-1 --base-url https://other.atlassian.net --auth-token XXX
```

## Environment variables

Useful for CI, containers, and ephemeral environments:

| Variable | Purpose |
|----------|---------|
| `JR_BASE_URL` | Jira base URL |
| `JR_AUTH_TOKEN` | API token / bearer token |
| `JR_AUTH_USER` | Username for basic auth |
| `JR_AUTH_TYPE` | `basic` (default), `bearer`, `oauth2` |
| `JR_CONFIG_PATH` | Override config file location |

## Profiles

Named profiles let you work with multiple Jira instances from one machine.

```bash
# Create a new profile
jr configure --base-url https://work.atlassian.net --token TOKEN --profile work

# Use a profile
jr issue get --profile work --issueIdOrKey PROJ-1

# Delete a profile (--profile is required)
jr configure --profile work --delete

# Test a profile
jr configure --test --profile work
```

Profiles are stored as keys in `~/.config/jr/config.json`:

```json
{
  "profiles": {
    "default": { "base_url": "...", "auth": {...} },
    "work":    { "base_url": "...", "auth": {...} }
  }
}
```

## Operation policy

Restrict which operations a profile can execute. Useful for agent profiles that should only be allowed to read, or for read-only service accounts.

```json
{
  "profiles": {
    "agent": {
      "base_url": "...",
      "auth": {"type": "basic", "token": "..."},
      "allowed_operations": ["issue get", "search *", "workflow *"]
    },
    "readonly": {
      "base_url": "...",
      "auth": {"type": "basic", "token": "..."},
      "denied_operations": ["* delete*", "bulk *", "raw *"]
    }
  }
}
```

### Rules

- Use `allowed_operations` **or** `denied_operations`, never both on the same profile.
- Patterns use glob matching: `*` matches any sequence of characters.
- `allowed_operations` is **deny-by-default** — only matching ops run.
- `denied_operations` is **allow-by-default** — only matching ops are blocked.

### Pattern examples

| Pattern | Matches |
|---------|---------|
| `issue get` | Exactly `issue get` |
| `issue *` | All verbs under `issue` |
| `* delete*` | Any resource's `delete` verbs (`issue delete`, `comment delete`, ...) |
| `workflow *` | All workflow operations |
| `raw *` | All raw API calls |
| `bulk *` | All bulk operations |

When an operation is blocked, `jr` exits with a policy error before making any HTTP call.

## Audit logging

Enable to record every operation (command, args, exit code, timing) for later review.

### Per-profile

```json
{
  "profiles": {
    "agent": {
      "audit_log": true,
      "audit_log_path": "/var/log/jr-audit.log"
    }
  }
}
```

### Per-invocation

```bash
# Enable for this call
jr issue get --issueIdOrKey PROJ-1 --audit

# Custom audit file (implies --audit)
jr issue get --issueIdOrKey PROJ-1 --audit-file /tmp/jr-session.log
```

Default path: `~/.config/jr/audit.log` (JSONL — one event per line).

Audit entries look like:

```json
{
  "ts": "2025-01-15T09:12:34.001Z",
  "profile": "agent",
  "command": "issue get",
  "args": {"issueIdOrKey": "PROJ-1"},
  "exit_code": 0,
  "duration_ms": 412
}
```

## Batch limits

Default max batch size is **50 operations**. Override:

```bash
jr batch --max-batch 100 --input ops.json
```

The limit is per-invocation, not a rate limit against Jira — it exists to prevent runaway scripts from making hundreds of API calls unintentionally.

## File locations

| File | Purpose |
|------|---------|
| `~/.config/jr/config.json` | Profiles, auth, policy (default) |
| `~/.config/jr/presets.json` | User-defined output presets |
| `~/.config/jr/templates/*.yaml` | User-defined issue templates |
| `~/.config/jr/audit.log` | Default audit log |

Override the config directory with `JR_CONFIG_PATH`.
