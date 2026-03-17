# Global Flags

All persistent flags listed here can be used with any `jr` command. They control authentication, output formatting, caching, and request behavior.

## Summary

| Flag | Short | Type | Default | Description |
|------|-------|------|---------|-------------|
| `--profile` | `-p` | `string` | `""` | Config profile to use |
| `--base-url` | | `string` | `""` | Jira base URL (overrides config) |
| `--auth-type` | | `string` | `""` | Auth type: `basic`, `bearer`, or `oauth2` (overrides config) |
| `--auth-user` | | `string` | `""` | Username for basic auth (overrides config) |
| `--auth-token` | | `string` | `""` | API token or bearer token (overrides config) |
| `--jq` | | `string` | `""` | jq filter expression applied to the response |
| `--fields` | | `string` | `""` | Comma-separated list of fields to return (GET only) |
| `--cache` | | `duration` | `0` | Cache GET responses for this duration |
| `--pretty` | | `bool` | `false` | Pretty-print JSON output |
| `--no-paginate` | | `bool` | `false` | Disable automatic pagination |
| `--verbose` | | `bool` | `false` | Log HTTP request/response details to stderr |
| `--dry-run` | | `bool` | `false` | Print the request as JSON without executing it |
| `--timeout` | | `duration` | `30s` | HTTP request timeout |

## Detailed reference

---

### `--profile` / `-p`

**Type:** `string`
**Default:** `""` (uses default profile)

Select a named configuration profile. Profiles let you switch between multiple Jira instances without modifying environment variables.

```bash
# Use the "work" profile for this command
jr issue get --profile work --issueIdOrKey PROJ-123

# Short form
jr issue get -p work --issueIdOrKey PROJ-123
```

Profiles are created with `jr configure --profile <name>`. See [Getting Started](./getting-started#named-profiles) for setup instructions.

---

### `--base-url`

**Type:** `string`
**Default:** `""` (uses value from config or `JR_BASE_URL` env var)

Override the Jira instance URL for a single command. Useful for one-off requests to a different instance.

```bash
jr issue get --base-url https://other.atlassian.net --issueIdOrKey OTHER-1
```

---

### `--auth-type`

**Type:** `string`
**Default:** `""` (uses value from config)

Override the authentication type. Accepted values: `basic`, `bearer`, `oauth2`.

```bash
jr raw GET /rest/api/3/myself --auth-type bearer --auth-token MY_PAT
```

---

### `--auth-user`

**Type:** `string`
**Default:** `""` (uses value from config or `JR_AUTH_USER` env var)

Override the username for basic authentication. This is your Atlassian account email.

```bash
jr issue get --issueIdOrKey PROJ-1 --auth-user other@company.com --auth-token TOKEN
```

---

### `--auth-token`

**Type:** `string`
**Default:** `""` (uses value from config or `JR_AUTH_TOKEN` env var)

Override the API token or bearer token for a single command.

```bash
jr raw GET /rest/api/3/myself --auth-token NEW_TOKEN
```

::: warning
Avoid passing tokens directly on the command line in shared environments. Prefer environment variables (`JR_AUTH_TOKEN`) or config file profiles for sensitive credentials.
:::

---

### `--jq`

**Type:** `string`
**Default:** `""` (no filtering)

Apply a [jq](https://jqlang.github.io/jq/) filter expression to the JSON response. This is client-side filtering --- it runs after the response is received.

```bash
# Extract just the key and summary
jr issue get --issueIdOrKey PROJ-123 --jq '{key: .key, summary: .fields.summary}'

# Get all issue keys from a search
jr search search-and-reconsile-issues-using-jql \
  --jql "project = PROJ" \
  --jq '[.issues[].key]'

# Count results
jr search search-and-reconsile-issues-using-jql \
  --jql "project = PROJ" \
  --jq '.total'
```

::: tip
Combine `--jq` with `--fields` for maximum token efficiency. `--fields` reduces the Jira response at the server, and `--jq` shapes it into exactly what you need.
:::

---

### `--fields`

**Type:** `string`
**Default:** `""` (returns all fields)

Comma-separated list of Jira fields to include in the response. This is server-side filtering --- Jira only returns the requested fields, reducing response size and API load.

Only applies to GET requests.

```bash
# Return only key, summary, and status
jr issue get --issueIdOrKey PROJ-123 --fields key,summary,status

# Combine with --jq for minimal output
jr issue get --issueIdOrKey PROJ-123 \
  --fields key,summary \
  --jq '{key: .key, summary: .fields.summary}'
```

---

### `--cache`

**Type:** `duration`
**Default:** `0` (caching disabled)

Cache GET responses locally for the specified duration. Cached responses are returned without making an API call. Accepts Go duration strings: `5m`, `1h`, `30s`, etc.

Only applies to GET requests.

```bash
# Cache project list for 5 minutes
jr project search --cache 5m --jq '[.values[].key]'

# Cache for 1 hour
jr issue get --issueIdOrKey PROJ-123 --cache 1h --fields key,summary
```

::: tip
Caching is especially useful for data that changes infrequently, like project lists, issue types, and field metadata. This avoids redundant API calls and reduces rate limit consumption.
:::

---

### `--pretty`

**Type:** `bool`
**Default:** `false`

Pretty-print the JSON output with indentation. Useful for human inspection; agents should generally leave this off to save tokens.

```bash
jr issue get --issueIdOrKey PROJ-123 --pretty
```

---

### `--no-paginate`

**Type:** `bool`
**Default:** `false`

Disable automatic pagination. By default, `jr` fetches all pages for paginated endpoints and merges them into a single response. Use this flag when you only need the first page or want to handle pagination yourself.

```bash
# Only get the first page of results
jr project search --no-paginate
```

---

### `--verbose`

**Type:** `bool`
**Default:** `false`

Log HTTP request and response details to stderr as JSON. Stdout remains clean JSON output. Useful for debugging API interactions.

```bash
jr issue get --issueIdOrKey PROJ-123 --verbose
# stderr shows: {"method":"GET","url":"https://...","status":200,"duration":"123ms"}
# stdout shows: the issue JSON
```

---

### `--dry-run`

**Type:** `bool`
**Default:** `false`

Print the HTTP request that would be made as JSON, without actually executing it. Useful for debugging request payloads, especially for POST/PUT operations.

```bash
jr issue create-issue --body '{"fields":{"project":{"key":"PROJ"},"summary":"Test"}}' --dry-run
# Outputs the request details without calling Jira
```

---

### `--timeout`

**Type:** `duration`
**Default:** `30s`

Set the HTTP request timeout. Accepts Go duration strings: `10s`, `1m`, `2m30s`, etc.

```bash
# Increase timeout for slow responses
jr search search-and-reconsile-issues-using-jql \
  --jql "project = PROJ" \
  --timeout 2m

# Short timeout for quick checks
jr raw GET /rest/api/3/myself --timeout 5s
```

## Configuration resolution order

Flags override environment variables, which override the config file:

```
CLI flags  >  Environment variables  >  Config file (profile)
```

For example, if your config file sets `base-url` to `https://a.atlassian.net` but you pass `--base-url https://b.atlassian.net`, the flag value wins.
