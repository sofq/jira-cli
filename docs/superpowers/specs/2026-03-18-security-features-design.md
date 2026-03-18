# Security Features for jr CLI

## Overview

Three security features to protect against unintended agent actions:

1. **Operation allowlist/denylist** per config profile
2. **Batch size limits** with configurable max
3. **Opt-in audit logging** per profile or per invocation

## 1. Operation Allowlist/Denylist

### Config Format

New optional fields on each profile in `~/.config/jr/config.json`:

```json
{
  "profiles": {
    "agent": {
      "base_url": "https://yoursite.atlassian.net",
      "auth_token": "...",
      "allowed_operations": ["issue get", "search *", "workflow *"]
    },
    "readonly": {
      "base_url": "https://yoursite.atlassian.net",
      "auth_token": "...",
      "denied_operations": ["* delete*", "bulk *", "raw *"]
    }
  }
}
```

### Rules

- Only one of `allowed_operations` or `denied_operations` may be set per profile. Config validation rejects both.
- Patterns use `filepath.Match` glob matching against the operation string `"resource verb"`.
  - Examples: `"issue get"`, `"workflow transition"`, `"raw POST"`
- For `raw` commands, the operation string is `"raw METHOD"` (e.g., `"raw GET"`, `"raw DELETE"`).
- If `allowed_operations` is set: implicit deny-all, only matching ops run.
- If `denied_operations` is set: implicit allow-all, only matching ops blocked.
- If neither is set: everything allowed (current behavior, backward compatible).

### Enforcement Points

- `PersistentPreRunE` in `cmd/root.go` — for all direct commands, after client construction.
- `executeBatchOp` in `cmd/batch.go` — for each batch sub-operation, before dispatch.
- Blocked operations exit with code 4 (validation):
  ```json
  {"error":"operation denied","operation":"issue delete","hint":"This operation is not allowed by profile 'agent'"}
  ```

### New Package: `internal/policy`

```go
type Policy struct {
    AllowedOps []string // glob patterns
    DeniedOps  []string // glob patterns
    Mode       string   // "allow", "deny", or "" (unrestricted)
}

func NewFromConfig(allowed, denied []string) (*Policy, error)
func (p *Policy) Check(operation string) error
```

- `NewFromConfig` validates mutual exclusivity.
- `Check` returns nil if allowed, or a structured error if denied.
- Glob matching via `filepath.Match` on the full operation string.

## 2. Batch Size Limits

### Behavior

- Default max: **50 operations** per batch.
- Override with `--max-batch N` flag on the `jr batch` command.
- Enforced after JSON parsing, before the execution loop in `cmd/batch.go`.
- Exceeding the limit exits with code 4:
  ```json
  {"error":"batch limit exceeded","count":200,"max":50,"hint":"Use --max-batch to increase the limit"}
  ```

### Implementation

- Add `--max-batch` persistent flag to the batch command (default 50).
- Check `len(ops) > maxBatch` after unmarshaling, before iteration.

## 3. Audit Logging

### Enabling

- **Per-profile config**: `"audit_log": true` in the profile.
- **Per-invocation flag**: `--audit` on any command.
- **Custom log path**: `--audit-file /path/to/file` (implies `--audit`).
- **Default path**: `~/.config/jr/audit.log` (OS-specific via `os.UserConfigDir`).
- Not enabled by default.

### Log Format

One JSONL line per operation, appended:

```json
{"ts":"2026-03-18T10:30:00Z","profile":"agent","op":"issue get","args":{"issueIdOrKey":"PROJ-123"},"method":"GET","path":"/rest/api/3/issue/PROJ-123","status":200,"exit":0,"dry_run":false}
```

### What's Logged

- Timestamp (UTC ISO 8601)
- Profile name
- Operation string (`"resource verb"`)
- Command args (flag key-value pairs, no body content)
- HTTP method, URL path
- Response status code, exit code
- Whether it was a dry-run
- For batch: each sub-operation logged individually

### What's NOT Logged

- Request/response bodies
- Auth tokens or credentials

### New Package: `internal/audit`

```go
type Entry struct {
    Timestamp string            `json:"ts"`
    Profile   string            `json:"profile"`
    Operation string            `json:"op"`
    Args      map[string]string `json:"args,omitempty"`
    Method    string            `json:"method"`
    Path      string            `json:"path"`
    Status    int               `json:"status"`
    Exit      int               `json:"exit"`
    DryRun    bool              `json:"dry_run"`
}

type Logger struct { /* wraps os.File, append-only */ }

func NewLogger(path string) (*Logger, error) // O_APPEND|O_CREATE, 0600
func (l *Logger) Log(entry Entry)
func (l *Logger) Close()
```

### Enforcement Points

- Audit config (enabled + path) resolved in `PersistentPreRunE` alongside the client.
- `Logger` stored in the client struct.
- `Do()` and `Fetch()` in `internal/client/client.go` call `Logger.Log()` after HTTP response (or after dry-run short-circuit).

## Integration with Existing Architecture

### Config Changes (`internal/config/config.go`)

Add to `Profile` struct:
- `AllowedOperations []string` (`json:"allowed_operations,omitempty"`)
- `DeniedOperations []string` (`json:"denied_operations,omitempty"`)
- `AuditLog bool` (`json:"audit_log,omitempty"`)

Add to `ResolvedConfig`:
- `Policy *policy.Policy`
- `AuditLog bool`

### Client Changes (`internal/client/client.go`)

Add to `Client` struct:
- `AuditLogger *audit.Logger` (nil when disabled)
- `Profile string` (for audit entries)
- `Operation string` (set before Do/Fetch, used for audit)

### Root Command Changes (`cmd/root.go`)

- Add `--audit` and `--audit-file` persistent flags.
- After config resolution: construct `policy.Policy` and validate.
- After client construction: if audit enabled, create `audit.Logger` and attach to client.
- After client construction: call `policy.Check()` with the resolved operation string.

### Batch Command Changes (`cmd/batch.go`)

- Add `--max-batch` flag (default 50).
- After parsing ops: check `len(ops) > maxBatch`.
- Before each op dispatch: call `policy.Check()`.

## File Summary

| File | Action |
|------|--------|
| `internal/policy/policy.go` | New — allowlist/denylist logic |
| `internal/policy/policy_test.go` | New — tests |
| `internal/audit/audit.go` | New — JSONL audit logger |
| `internal/audit/audit_test.go` | New — tests |
| `internal/config/config.go` | Modify — add new profile fields, validation |
| `internal/client/client.go` | Modify — add audit + operation fields, log in Do/Fetch |
| `cmd/root.go` | Modify — add flags, policy check, audit init |
| `cmd/batch.go` | Modify — add max-batch flag, policy check per op |
| `cmd/raw.go` | Modify — set operation string for policy/audit |
