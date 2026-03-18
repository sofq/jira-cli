# Security Features Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add operation allowlist/denylist per profile, batch size limits, and opt-in audit logging to jr CLI.

**Architecture:** Three new packages (`internal/policy`, `internal/audit`) plus modifications to config, client, root command, and batch command. Policy is checked at command dispatch time. Audit logging hooks into the client's `Do()` and `Fetch()` methods. Batch size is enforced in the batch command before execution.

**Tech Stack:** Go stdlib (`path/filepath`, `encoding/json`, `os`, `sync`), existing `internal/errors` for structured errors.

---

## File Structure

| File | Action | Responsibility |
|------|--------|---------------|
| `internal/policy/policy.go` | Create | Glob-based operation allowlist/denylist checking |
| `internal/policy/policy_test.go` | Create | Tests for policy matching |
| `internal/audit/audit.go` | Create | Append-only JSONL audit logger |
| `internal/audit/audit_test.go` | Create | Tests for audit logger |
| `internal/config/config.go` | Modify | Add `AllowedOperations`, `DeniedOperations`, `AuditLog` fields to `Profile`; add `ProfileName` to `ResolvedConfig`; return policy fields in `Resolve()` |
| `internal/config/config_test.go` | Modify | Test new profile fields roundtrip and mutual exclusivity |
| `internal/client/client.go` | Modify | Add `AuditLogger`, `Profile`, `Operation`, `Policy` fields; call audit in `Do()` and `Fetch()` |
| `internal/client/client_test.go` | Modify | Test audit logging integration |
| `cmd/root.go` | Modify | Add `--audit`/`--audit-file` flags; construct policy + audit logger; check policy before dispatch |
| `cmd/batch.go` | Modify | Add `--max-batch` flag; enforce batch limit; check policy per sub-op |
| `cmd/raw.go` | Modify | Set `Operation` on client for policy/audit |

---

### Task 1: Policy Package

**Files:**
- Create: `internal/policy/policy.go`
- Create: `internal/policy/policy_test.go`

- [ ] **Step 1: Write failing tests for policy package**

Create `internal/policy/policy_test.go`:

```go
package policy_test

import (
	"testing"

	"github.com/sofq/jira-cli/internal/policy"
)

func TestNewFromConfig_AllowOnly(t *testing.T) {
	p, err := policy.NewFromConfig([]string{"issue get", "search *"}, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if p == nil {
		t.Fatal("expected non-nil policy")
	}
}

func TestNewFromConfig_DenyOnly(t *testing.T) {
	p, err := policy.NewFromConfig(nil, []string{"* delete*", "bulk *"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if p == nil {
		t.Fatal("expected non-nil policy")
	}
}

func TestNewFromConfig_BothSetReturnsError(t *testing.T) {
	_, err := policy.NewFromConfig([]string{"issue get"}, []string{"bulk *"})
	if err == nil {
		t.Fatal("expected error when both allowed and denied are set")
	}
}

func TestNewFromConfig_NeitherSet(t *testing.T) {
	p, err := policy.NewFromConfig(nil, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// nil policy means unrestricted
	if p != nil {
		t.Fatal("expected nil policy when neither set")
	}
}

func TestCheck_AllowMode(t *testing.T) {
	p, _ := policy.NewFromConfig([]string{"issue get", "issue edit", "search *", "workflow *"}, nil)

	tests := []struct {
		op      string
		allowed bool
	}{
		{"issue get", true},
		{"issue edit", true},
		{"issue delete", false},
		{"search search-and-reconsile-issues-using-jql", true},
		{"workflow transition", true},
		{"bulk submit-delete", false},
		{"raw POST", false},
	}
	for _, tt := range tests {
		t.Run(tt.op, func(t *testing.T) {
			err := p.Check(tt.op)
			if tt.allowed && err != nil {
				t.Errorf("expected allowed, got error: %v", err)
			}
			if !tt.allowed && err == nil {
				t.Errorf("expected denied, got nil")
			}
		})
	}
}

func TestCheck_DenyMode(t *testing.T) {
	p, _ := policy.NewFromConfig(nil, []string{"* delete*", "bulk *", "raw *"})

	tests := []struct {
		op      string
		allowed bool
	}{
		{"issue get", true},
		{"issue create-issue", true},
		{"issue delete", false},
		{"issue delete-comment", false},
		{"bulk submit-delete", false},
		{"raw POST", false},
		{"raw GET", false},
		{"workflow transition", true},
	}
	for _, tt := range tests {
		t.Run(tt.op, func(t *testing.T) {
			err := p.Check(tt.op)
			if tt.allowed && err != nil {
				t.Errorf("expected allowed, got error: %v", err)
			}
			if !tt.allowed && err == nil {
				t.Errorf("expected denied, got nil")
			}
		})
	}
}

func TestCheck_NilPolicy(t *testing.T) {
	var p *policy.Policy
	if err := p.Check("anything"); err != nil {
		t.Errorf("nil policy should allow everything, got: %v", err)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd /Users/quan.hoang/quanhh/quanhoang/jira-cli-v2 && go test ./internal/policy/...`
Expected: compilation errors — package doesn't exist yet.

- [ ] **Step 3: Implement policy package**

Create `internal/policy/policy.go`:

```go
package policy

import (
	"fmt"
	"path/filepath"
)

// Policy enforces operation-level access control per profile.
// A nil *Policy allows all operations (unrestricted mode).
type Policy struct {
	allowedOps []string // glob patterns — if set, only matching ops are allowed
	deniedOps  []string // glob patterns — if set, matching ops are denied
	mode       string   // "allow", "deny"
}

// NewFromConfig creates a Policy from config fields. Returns nil if both
// slices are empty (unrestricted). Returns error if both are non-empty.
func NewFromConfig(allowed, denied []string) (*Policy, error) {
	hasAllow := len(allowed) > 0
	hasDeny := len(denied) > 0

	if hasAllow && hasDeny {
		return nil, fmt.Errorf("profile cannot have both allowed_operations and denied_operations; use one or the other")
	}
	if !hasAllow && !hasDeny {
		return nil, nil
	}

	// Validate all patterns are well-formed.
	all := allowed
	if hasDeny {
		all = denied
	}
	for _, pattern := range all {
		if _, err := filepath.Match(pattern, ""); err != nil {
			return nil, fmt.Errorf("invalid glob pattern %q: %w", pattern, err)
		}
	}

	p := &Policy{}
	if hasAllow {
		p.allowedOps = allowed
		p.mode = "allow"
	} else {
		p.deniedOps = denied
		p.mode = "deny"
	}
	return p, nil
}

// Check returns nil if the operation is permitted, or an error describing why
// it was denied. A nil *Policy allows everything.
func (p *Policy) Check(operation string) error {
	if p == nil {
		return nil
	}

	switch p.mode {
	case "allow":
		for _, pattern := range p.allowedOps {
			if matched, _ := filepath.Match(pattern, operation); matched {
				return nil
			}
		}
		return &DeniedError{Operation: operation, Reason: "not in allowed_operations"}
	case "deny":
		for _, pattern := range p.deniedOps {
			if matched, _ := filepath.Match(pattern, operation); matched {
				return &DeniedError{Operation: operation, Reason: "matched denied_operations pattern: " + pattern}
			}
		}
		return nil
	}
	return nil
}

// DeniedError is returned when an operation is blocked by policy.
type DeniedError struct {
	Operation string
	Reason    string
}

func (e *DeniedError) Error() string {
	return fmt.Sprintf("operation %q denied: %s", e.Operation, e.Reason)
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd /Users/quan.hoang/quanhh/quanhoang/jira-cli-v2 && go test ./internal/policy/... -v`
Expected: all tests PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/policy/policy.go internal/policy/policy_test.go
git commit -m "feat: add policy package for operation allowlist/denylist"
```

---

### Task 2: Audit Package

**Files:**
- Create: `internal/audit/audit.go`
- Create: `internal/audit/audit_test.go`

- [ ] **Step 1: Write failing tests for audit package**

Create `internal/audit/audit_test.go`:

```go
package audit_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sofq/jira-cli/internal/audit"
)

func TestNewLogger_CreatesFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "audit.log")

	l, err := audit.NewLogger(path)
	if err != nil {
		t.Fatalf("NewLogger: %v", err)
	}
	defer l.Close()

	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("file should exist: %v", err)
	}
	if perm := info.Mode().Perm(); perm != 0o600 {
		t.Errorf("permissions = %o, want 600", perm)
	}
}

func TestNewLogger_CreatesParentDirs(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "a", "b", "audit.log")

	l, err := audit.NewLogger(path)
	if err != nil {
		t.Fatalf("NewLogger: %v", err)
	}
	defer l.Close()

	if _, err := os.Stat(path); err != nil {
		t.Fatalf("file should exist: %v", err)
	}
}

func TestLogger_Log_WritesJSONL(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "audit.log")

	l, err := audit.NewLogger(path)
	if err != nil {
		t.Fatalf("NewLogger: %v", err)
	}

	l.Log(audit.Entry{
		Profile:   "test",
		Operation: "issue get",
		Method:    "GET",
		Path:      "/rest/api/3/issue/TEST-1",
		Status:    200,
		Exit:      0,
	})
	l.Log(audit.Entry{
		Profile:   "test",
		Operation: "issue delete",
		Method:    "DELETE",
		Path:      "/rest/api/3/issue/TEST-2",
		Status:    204,
		Exit:      0,
		DryRun:    true,
	})
	l.Close()

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}

	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	if len(lines) != 2 {
		t.Fatalf("expected 2 lines, got %d: %s", len(lines), string(data))
	}

	var entry1 audit.Entry
	if err := json.Unmarshal([]byte(lines[0]), &entry1); err != nil {
		t.Fatalf("line 1 not valid JSON: %v", err)
	}
	if entry1.Operation != "issue get" {
		t.Errorf("entry1.Operation = %q, want %q", entry1.Operation, "issue get")
	}
	if entry1.Timestamp == "" {
		t.Error("entry1.Timestamp should be set")
	}

	var entry2 audit.Entry
	if err := json.Unmarshal([]byte(lines[1]), &entry2); err != nil {
		t.Fatalf("line 2 not valid JSON: %v", err)
	}
	if !entry2.DryRun {
		t.Error("entry2.DryRun should be true")
	}
}

func TestLogger_Append(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "audit.log")

	// Write first entry, close.
	l1, _ := audit.NewLogger(path)
	l1.Log(audit.Entry{Operation: "first"})
	l1.Close()

	// Open again, write second entry.
	l2, _ := audit.NewLogger(path)
	l2.Log(audit.Entry{Operation: "second"})
	l2.Close()

	data, _ := os.ReadFile(path)
	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	if len(lines) != 2 {
		t.Fatalf("expected 2 lines after reopen, got %d", len(lines))
	}
}

func TestNilLogger_LogIsNoop(t *testing.T) {
	var l *audit.Logger
	// Should not panic.
	l.Log(audit.Entry{Operation: "test"})
	l.Close()
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd /Users/quan.hoang/quanhh/quanhoang/jira-cli-v2 && go test ./internal/audit/...`
Expected: compilation errors — package doesn't exist yet.

- [ ] **Step 3: Implement audit package**

Create `internal/audit/audit.go`:

```go
package audit

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// Entry is a single audit log record.
type Entry struct {
	Timestamp string `json:"ts,omitempty"`
	Profile   string `json:"profile,omitempty"`
	Operation string `json:"op,omitempty"`
	Method    string `json:"method,omitempty"`
	Path      string `json:"path,omitempty"`
	Status    int    `json:"status,omitempty"`
	Exit      int    `json:"exit,omitempty"`
	DryRun    bool   `json:"dry_run,omitempty"`
}

// Logger writes audit entries as JSONL to a file.
// A nil *Logger is safe to use — all methods are no-ops.
type Logger struct {
	mu   sync.Mutex
	file *os.File
}

// NewLogger opens (or creates) the audit log file for appending.
// The file is created with 0600 permissions. Parent directories are created
// if they do not exist.
func NewLogger(path string) (*Logger, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, err
	}
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return nil, err
	}
	return &Logger{file: f}, nil
}

// Log writes an entry to the audit log. It sets the timestamp automatically.
// Safe for concurrent use. No-op on nil receiver.
func (l *Logger) Log(entry Entry) {
	if l == nil {
		return
	}
	entry.Timestamp = time.Now().UTC().Format(time.RFC3339)

	l.mu.Lock()
	defer l.mu.Unlock()

	data, err := json.Marshal(entry)
	if err != nil {
		return
	}
	data = append(data, '\n')
	_, _ = l.file.Write(data)
}

// Close closes the underlying file. No-op on nil receiver.
func (l *Logger) Close() {
	if l == nil {
		return
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.file != nil {
		_ = l.file.Close()
	}
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd /Users/quan.hoang/quanhh/quanhoang/jira-cli-v2 && go test ./internal/audit/... -v`
Expected: all tests PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/audit/audit.go internal/audit/audit_test.go
git commit -m "feat: add audit package for JSONL operation logging"
```

---

### Task 3: Config Changes

**Files:**
- Modify: `internal/config/config.go:30-54` (Profile and ResolvedConfig structs, Resolve function)
- Modify: `internal/config/config_test.go` (add new tests)

- [ ] **Step 1: Write failing tests for new config fields**

Add to `internal/config/config_test.go`:

```go
// TestProfilePolicyFieldsRoundtrip verifies that allowed/denied operations
// survive a SaveTo/LoadFrom roundtrip.
func TestProfilePolicyFieldsRoundtrip(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.json")

	cfg := &config.Config{
		DefaultProfile: "agent",
		Profiles: map[string]config.Profile{
			"agent": {
				BaseURL: "https://example.com",
				Auth:    config.AuthConfig{Type: "basic", Token: "tok"},
				AllowedOperations: []string{"issue get", "search *"},
			},
			"restricted": {
				BaseURL: "https://example.com",
				Auth:    config.AuthConfig{Type: "basic", Token: "tok"},
				DeniedOperations: []string{"* delete*", "bulk *"},
			},
		},
	}
	if err := config.SaveTo(cfg, cfgPath); err != nil {
		t.Fatalf("SaveTo: %v", err)
	}

	loaded, err := config.LoadFrom(cfgPath)
	if err != nil {
		t.Fatalf("LoadFrom: %v", err)
	}

	agent := loaded.Profiles["agent"]
	if len(agent.AllowedOperations) != 2 {
		t.Errorf("agent AllowedOperations = %v, want 2 items", agent.AllowedOperations)
	}

	restricted := loaded.Profiles["restricted"]
	if len(restricted.DeniedOperations) != 2 {
		t.Errorf("restricted DeniedOperations = %v, want 2 items", restricted.DeniedOperations)
	}
}

// TestProfileAuditLogFieldRoundtrip verifies audit_log survives roundtrip.
func TestProfileAuditLogFieldRoundtrip(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.json")

	cfg := &config.Config{
		DefaultProfile: "audited",
		Profiles: map[string]config.Profile{
			"audited": {
				BaseURL:  "https://example.com",
				Auth:     config.AuthConfig{Type: "basic", Token: "tok"},
				AuditLog: true,
			},
		},
	}
	if err := config.SaveTo(cfg, cfgPath); err != nil {
		t.Fatalf("SaveTo: %v", err)
	}

	loaded, err := config.LoadFrom(cfgPath)
	if err != nil {
		t.Fatalf("LoadFrom: %v", err)
	}

	if !loaded.Profiles["audited"].AuditLog {
		t.Error("expected AuditLog=true after roundtrip")
	}
}

// TestResolveReturnsProfileName verifies that Resolve carries the profile name.
func TestResolveReturnsProfileName(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.json")

	cfg := &config.Config{
		DefaultProfile: "work",
		Profiles: map[string]config.Profile{
			"work": {
				BaseURL: "https://example.com",
				Auth:    config.AuthConfig{Type: "basic", Token: "tok"},
			},
		},
	}
	if err := config.SaveTo(cfg, cfgPath); err != nil {
		t.Fatalf("SaveTo: %v", err)
	}

	setEnv(t, map[string]string{
		"JR_BASE_URL": "", "JR_AUTH_TYPE": "", "JR_AUTH_USER": "", "JR_AUTH_TOKEN": "",
	})

	resolved, err := config.Resolve(cfgPath, "work", nil)
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if resolved.ProfileName != "work" {
		t.Errorf("ProfileName = %q, want %q", resolved.ProfileName, "work")
	}
}

// TestResolveReturnsPolicyFields verifies that Resolve carries allow/deny lists.
func TestResolveReturnsPolicyFields(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.json")

	cfg := &config.Config{
		DefaultProfile: "default",
		Profiles: map[string]config.Profile{
			"default": {
				BaseURL:           "https://example.com",
				Auth:              config.AuthConfig{Type: "basic", Token: "tok"},
				AllowedOperations: []string{"issue *"},
			},
		},
	}
	if err := config.SaveTo(cfg, cfgPath); err != nil {
		t.Fatalf("SaveTo: %v", err)
	}

	setEnv(t, map[string]string{
		"JR_BASE_URL": "", "JR_AUTH_TYPE": "", "JR_AUTH_USER": "", "JR_AUTH_TOKEN": "",
	})

	resolved, err := config.Resolve(cfgPath, "", nil)
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if len(resolved.AllowedOperations) != 1 || resolved.AllowedOperations[0] != "issue *" {
		t.Errorf("AllowedOperations = %v, want [issue *]", resolved.AllowedOperations)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd /Users/quan.hoang/quanhh/quanhoang/jira-cli-v2 && go test ./internal/config/... -run 'TestProfile|TestResolveReturnsProfile|TestResolveReturnsPolicy'`
Expected: compilation errors — new fields don't exist yet.

- [ ] **Step 3: Add new fields to Profile and ResolvedConfig**

In `internal/config/config.go`, modify the `Profile` struct (line 30-33):

```go
// Profile holds the configuration for a named Jira instance.
type Profile struct {
	BaseURL           string     `json:"base_url"`
	Auth              AuthConfig `json:"auth"`
	AllowedOperations []string   `json:"allowed_operations,omitempty"`
	DeniedOperations  []string   `json:"denied_operations,omitempty"`
	AuditLog          bool       `json:"audit_log,omitempty"`
}
```

Modify the `ResolvedConfig` struct (line 50-54):

```go
// ResolvedConfig is the final, merged configuration ready for use.
type ResolvedConfig struct {
	BaseURL           string
	Auth              AuthConfig
	ProfileName       string
	AllowedOperations []string
	DeniedOperations  []string
	AuditLog          bool
}
```

In the `Resolve` function, after loading the profile (around line 152-163), capture the new fields. At the return statement (line 240-252), add:

```go
	return &ResolvedConfig{
		BaseURL: baseURL,
		Auth: AuthConfig{
			Type:         authType,
			Username:     username,
			Token:        token,
			ClientID:     fileClientID,
			ClientSecret: fileClientSecret,
			TokenURL:     fileTokenURL,
			Scopes:       fileScopes,
		},
		ProfileName:       name,
		AllowedOperations: fileAllowedOps,
		DeniedOperations:  fileDeniedOps,
		AuditLog:          fileAuditLog,
	}, nil
```

Add these variable declarations after line 151 (where `fileBaseURL`, etc. are declared), and read them from the profile in the `if p, ok` block:

```go
	var fileAllowedOps, fileDeniedOps []string
	var fileAuditLog bool
	if p, ok := cfg.Profiles[name]; ok {
		// ... existing field reads ...
		fileAllowedOps = p.AllowedOperations
		fileDeniedOps = p.DeniedOperations
		fileAuditLog = p.AuditLog
	}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd /Users/quan.hoang/quanhh/quanhoang/jira-cli-v2 && go test ./internal/config/... -v`
Expected: all tests PASS (including existing tests).

- [ ] **Step 5: Commit**

```bash
git add internal/config/config.go internal/config/config_test.go
git commit -m "feat: add policy and audit fields to config profile"
```

---

### Task 4: Client Audit Integration

**Files:**
- Modify: `internal/client/client.go:27-40` (Client struct), `:128-181` (Do method), `:569-621` (Fetch method)
- Modify: `internal/client/client_test.go` (add audit test)

- [ ] **Step 1: Write failing test for audit in client**

Add to `internal/client/client_test.go`:

```go
func TestDo_AuditLogging(t *testing.T) {
	te := newTestEnv(jsonHandler(`{"ok":true}`))
	defer te.Close()

	// Set up audit logger.
	dir := t.TempDir()
	auditPath := filepath.Join(dir, "audit.log")
	logger, err := audit.NewLogger(auditPath)
	if err != nil {
		t.Fatalf("NewLogger: %v", err)
	}
	te.Client.AuditLogger = logger
	te.Client.Profile = "test-profile"
	te.Client.Operation = "issue get"

	code := te.Client.Do(context.Background(), "GET", "/rest/api/3/issue/TEST-1", nil, nil)
	if code != 0 {
		t.Fatalf("Do: exit %d", code)
	}
	logger.Close()

	data, _ := os.ReadFile(auditPath)
	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	if len(lines) != 1 {
		t.Fatalf("expected 1 audit line, got %d", len(lines))
	}

	var entry map[string]interface{}
	if err := json.Unmarshal([]byte(lines[0]), &entry); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if entry["profile"] != "test-profile" {
		t.Errorf("profile = %v, want test-profile", entry["profile"])
	}
	if entry["op"] != "issue get" {
		t.Errorf("op = %v, want issue get", entry["op"])
	}
	if entry["method"] != "GET" {
		t.Errorf("method = %v, want GET", entry["method"])
	}
}

func TestFetch_AuditLogging(t *testing.T) {
	te := newTestEnv(jsonHandler(`{"ok":true}`))
	defer te.Close()

	dir := t.TempDir()
	auditPath := filepath.Join(dir, "audit.log")
	logger, err := audit.NewLogger(auditPath)
	if err != nil {
		t.Fatalf("NewLogger: %v", err)
	}
	te.Client.AuditLogger = logger
	te.Client.Profile = "test-profile"
	te.Client.Operation = "workflow transition"

	_, code := te.Client.Fetch(context.Background(), "POST", "/rest/api/3/issue/TEST-1/transitions", nil)
	if code != 0 {
		t.Fatalf("Fetch: exit %d", code)
	}
	logger.Close()

	data, _ := os.ReadFile(auditPath)
	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	if len(lines) != 1 {
		t.Fatalf("expected 1 audit line, got %d", len(lines))
	}
}

func TestDo_DryRun_AuditLogging(t *testing.T) {
	te := newTestEnv(jsonHandler(`{}`))
	defer te.Close()

	dir := t.TempDir()
	auditPath := filepath.Join(dir, "audit.log")
	logger, err := audit.NewLogger(auditPath)
	if err != nil {
		t.Fatalf("NewLogger: %v", err)
	}
	te.Client.AuditLogger = logger
	te.Client.DryRun = true
	te.Client.Operation = "issue delete"

	te.Client.Do(context.Background(), "DELETE", "/rest/api/3/issue/TEST-1", nil, nil)
	logger.Close()

	data, _ := os.ReadFile(auditPath)
	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	if len(lines) != 1 {
		t.Fatalf("expected 1 audit line for dry-run, got %d", len(lines))
	}

	var entry map[string]interface{}
	json.Unmarshal([]byte(lines[0]), &entry)
	if entry["dry_run"] != true {
		t.Error("expected dry_run=true in audit entry")
	}
}
```

Add the import for audit at the top of `client_test.go`:

```go
import (
	// ... existing imports ...
	"github.com/sofq/jira-cli/internal/audit"
)
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd /Users/quan.hoang/quanhh/quanhoang/jira-cli-v2 && go test ./internal/client/... -run 'Audit'`
Expected: compilation errors — `AuditLogger`, `Profile`, `Operation` fields don't exist yet.

- [ ] **Step 3: Add audit fields to Client struct and logging to Do/Fetch**

In `internal/client/client.go`, add fields to the `Client` struct (after line 39):

```go
type Client struct {
	BaseURL    string
	Auth       config.AuthConfig
	HTTPClient *http.Client
	Stdout     io.Writer
	Stderr     io.Writer
	JQFilter   string
	Paginate   bool
	DryRun     bool
	Verbose    bool
	Pretty     bool
	Fields     string
	CacheTTL   time.Duration
	// Security fields
	AuditLogger *audit.Logger  // nil means no audit logging
	Profile     string         // profile name for audit entries
	Operation   string         // "resource verb" for audit entries
	Policy      *policy.Policy // nil means unrestricted
}
```

Add imports for audit and policy packages:

```go
import (
	// ... existing ...
	"github.com/sofq/jira-cli/internal/audit"
	"github.com/sofq/jira-cli/internal/policy"
)
```

In the `Do` method, capture the original path for audit before it's mutated, then add audit logging. At the top of `Do()`, before path mutation (before line 138):

```go
	origPath := path // preserve for audit logging
```

After the dry-run block (after line 171, the `return c.WriteOutput(...)` for dry-run), change to:

```go
	// DryRun: emit the request as JSON and return immediately.
	if c.DryRun {
		// ... existing dry-run code ...
		exitCode := c.WriteOutput(bytes.TrimRight(buf.Bytes(), "\n"))
		c.auditLog(method, origPath, 0, exitCode)
		return exitCode
	}
```

Note: the existing `Do` method's dry-run returns `c.WriteOutput(...)` directly. We need to capture the exit code first, log, then return. We use `origPath` (not `path`) because `path` has been modified by this point.

At the end of `doOnce` (around line 266), before `return c.WriteOutput(respBody)`, wrap similarly:

```go
	exitCode := c.WriteOutput(respBody)
	c.auditLog(method, path, resp.StatusCode, exitCode)
	return exitCode
```

Also audit on HTTP errors in `doOnce` (around line 248-251):

```go
	if resp.StatusCode >= 400 {
		apiErr := jrerrors.NewFromHTTP(resp.StatusCode, strings.TrimSpace(string(respBody)), method, path, resp)
		apiErr.WriteJSON(c.Stderr)
		c.auditLog(method, path, resp.StatusCode, apiErr.ExitCode())
		return apiErr.ExitCode()
	}
```

In the `Fetch` method, add similar audit logging before both success and error returns. After the HTTP error check (line 615-618):

```go
	if resp.StatusCode >= 400 {
		apiErr := jrerrors.NewFromHTTP(resp.StatusCode, strings.TrimSpace(string(respBody)), method, path, resp)
		apiErr.WriteJSON(c.Stderr)
		c.auditLog(method, path, resp.StatusCode, apiErr.ExitCode())
		return nil, apiErr.ExitCode()
	}
	c.auditLog(method, path, resp.StatusCode, jrerrors.ExitOK)
	return respBody, jrerrors.ExitOK
```

Add a private helper method:

```go
// auditLog writes an audit entry if an AuditLogger is configured.
func (c *Client) auditLog(method, path string, status, exitCode int) {
	if c.AuditLogger == nil {
		return
	}
	c.AuditLogger.Log(audit.Entry{
		Profile:   c.Profile,
		Operation: c.Operation,
		Method:    method,
		Path:      path,
		Status:    status,
		Exit:      exitCode,
		DryRun:    c.DryRun,
	})
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd /Users/quan.hoang/quanhh/quanhoang/jira-cli-v2 && go test ./internal/client/... -v`
Expected: all tests PASS (including existing tests).

- [ ] **Step 5: Commit**

```bash
git add internal/client/client.go internal/client/client_test.go
git commit -m "feat: add audit logging to client Do and Fetch methods"
```

---

### Task 5: Root Command — Policy Check + Audit Init + Flags

**Files:**
- Modify: `cmd/root.go:29-136` (PersistentPreRunE), `:139-154` (init)

- [ ] **Step 1: Add --audit and --audit-file flags to root init**

In `cmd/root.go` `init()`, add after the existing persistent flags (around line 153):

```go
	pf.Bool("audit", false, "enable audit logging for this invocation")
	pf.String("audit-file", "", "path to audit log file (implies --audit)")
```

- [ ] **Step 2: Add imports for policy and audit packages**

In `cmd/root.go`, add to imports:

```go
import (
	// ... existing ...
	"github.com/sofq/jira-cli/internal/audit"
	"github.com/sofq/jira-cli/internal/policy"
)
```

- [ ] **Step 3: Construct policy and audit logger in PersistentPreRunE**

After config resolution (after line 117 where `resolved.BaseURL` is checked), add:

```go
		// Build operation policy from profile config.
		pol, polErr := policy.NewFromConfig(resolved.AllowedOperations, resolved.DeniedOperations)
		if polErr != nil {
			apiErr := &jrerrors.APIError{
				ErrorType: "config_error",
				Message:   "invalid policy config: " + polErr.Error(),
			}
			apiErr.WriteJSON(os.Stderr)
			return &jrerrors.AlreadyWrittenError{Code: jrerrors.ExitValidation}
		}

		// Resolve the operation name: "resource verb" or "raw METHOD".
		operation := resolveOperation(cmd)

		// Check policy before proceeding.
		// Skip for commands that check policy themselves (raw needs the HTTP
		// method from positional args; batch checks per sub-operation).
		skipPolicyHere := map[string]bool{"raw": true, "batch": true, "watch": true, "diff": true}
		if pol != nil && !skipPolicyHere[cmd.Name()] {
			if err := pol.Check(operation); err != nil {
				apiErr := &jrerrors.APIError{
					ErrorType: "validation_error",
					Message:   err.Error(),
					Hint:      fmt.Sprintf("This operation is not allowed by profile %q", resolved.ProfileName),
				}
				apiErr.WriteJSON(os.Stderr)
				return &jrerrors.AlreadyWrittenError{Code: jrerrors.ExitValidation}
			}
		}

		// Set up audit logger if enabled.
		auditFlag, _ := cmd.Flags().GetBool("audit")
		auditFile, _ := cmd.Flags().GetString("audit-file")
		auditEnabled := auditFlag || auditFile != "" || resolved.AuditLog

		var auditLogger *audit.Logger
		if auditEnabled {
			if auditFile == "" {
				auditFile = audit.DefaultPath()
			}
			var auditErr error
			auditLogger, auditErr = audit.NewLogger(auditFile)
			if auditErr != nil {
				// Audit failure is non-fatal — warn on stderr, continue.
				warnMsg := map[string]string{
					"type":    "warning",
					"message": "audit log open failed: " + auditErr.Error(),
				}
				warnJSON, _ := json.Marshal(warnMsg)
				fmt.Fprintf(os.Stderr, "%s\n", warnJSON)
			}
		}
```

Then modify the client construction (line 119-132) to include the new fields:

```go
		c := &client.Client{
			BaseURL:     resolved.BaseURL,
			Auth:        resolved.Auth,
			HTTPClient:  &http.Client{Timeout: timeout},
			Stdout:      os.Stdout,
			Stderr:      os.Stderr,
			JQFilter:    jqFilter,
			Paginate:    !noPaginate,
			DryRun:      dryRun,
			Verbose:     verbose,
			Pretty:      pretty,
			Fields:      fields,
			CacheTTL:    cacheTTL,
			AuditLogger: auditLogger,
			Profile:     resolved.ProfileName,
			Operation:   operation,
			Policy:      pol,
		}
```

- [ ] **Step 4: Add resolveOperation helper and audit.DefaultPath**

Add to `cmd/root.go` after the `mergeCommand` function:

```go
// resolveOperation returns the operation name for the current command.
// For most commands: "parent child" (e.g., "issue get").
// For raw commands: "raw METHOD" (e.g., "raw GET").
func resolveOperation(cmd *cobra.Command) string {
	if cmd.Name() == "raw" {
		// For raw, we can't know the method until RunE.
		// Use "raw" as placeholder; raw.go will update it.
		return "raw"
	}
	if cmd.Parent() != nil && cmd.Parent().Name() != "jr" {
		return cmd.Parent().Name() + " " + cmd.Name()
	}
	return cmd.Name()
}
```

Add to `internal/audit/audit.go` (after the `NewLogger` function):

```go
// DefaultPath returns the default audit log file path.
func DefaultPath() string {
	switch runtime.GOOS {
	case "darwin":
		home, _ := os.UserHomeDir()
		return filepath.Join(home, "Library", "Application Support", "jr", "audit.log")
	case "windows":
		appdata := os.Getenv("APPDATA")
		return filepath.Join(appdata, "jr", "audit.log")
	default:
		home, _ := os.UserHomeDir()
		return filepath.Join(home, ".config", "jr", "audit.log")
	}
}
```

Add the `runtime` import to `internal/audit/audit.go`:

```go
import (
	// ... existing ...
	"runtime"
)
```

- [ ] **Step 5: Run all tests**

Run: `cd /Users/quan.hoang/quanhh/quanhoang/jira-cli-v2 && go build ./... && go test ./...`
Expected: builds and all tests PASS.

- [ ] **Step 6: Commit**

```bash
git add cmd/root.go internal/audit/audit.go
git commit -m "feat: add policy check and audit logger init to root command"
```

---

### Task 6: Raw Command — Set Operation for Policy/Audit

**Files:**
- Modify: `cmd/raw.go:40-143` (runRaw function)

Note: The `Policy` field was already added to `Client` in Task 4, and `PersistentPreRunE` already skips policy check for `raw` (Task 5). This task just adds the deferred policy check inside `runRaw`.

- [ ] **Step 1: Add operation tracking and policy check to raw command**

In `cmd/raw.go`, in the `runRaw` function, after getting the client (after line 54-62), add:

```go
	// Set operation for policy/audit: "raw METHOD".
	// Policy check is deferred to here because the HTTP method is a positional
	// arg not available during PersistentPreRunE.
	c.Operation = "raw " + method
	if c.Policy != nil {
		if err := c.Policy.Check(c.Operation); err != nil {
			apiErr := &jrerrors.APIError{
				ErrorType: "validation_error",
				Message:   err.Error(),
				Hint:      "This operation is not allowed by the current profile",
			}
			apiErr.WriteJSON(os.Stderr)
			return &jrerrors.AlreadyWrittenError{Code: jrerrors.ExitValidation}
		}
	}
```

- [ ] **Step 2: Run tests**

Run: `cd /Users/quan.hoang/quanhh/quanhoang/jira-cli-v2 && go build ./... && go test ./...`
Expected: builds and all tests PASS.

- [ ] **Step 3: Commit**

```bash
git add cmd/raw.go
git commit -m "feat: add policy check and operation tracking to raw command"
```

---

### Task 7: Batch Command — Size Limit + Policy Check

**Files:**
- Modify: `cmd/batch.go:40-63` (init, flags), `:66-159` (runBatch), `:214-300` (executeBatchOp)

- [ ] **Step 1: Add --max-batch flag**

In `cmd/batch.go` `init()` (line 61-63), add:

```go
func init() {
	batchCmd.Flags().String("input", "", "path to JSON input file (reads stdin if not set)")
	batchCmd.Flags().Int("max-batch", 50, "maximum number of operations per batch")
	rootCmd.AddCommand(batchCmd)
}
```

- [ ] **Step 2: Add batch size check in runBatch**

In `runBatch`, after parsing ops and the null check (after line 139), add:

```go
	// Enforce batch size limit.
	maxBatch, _ := cmd.Flags().GetInt("max-batch")
	if maxBatch > 0 && len(ops) > maxBatch {
		apiErr := &jrerrors.APIError{
			ErrorType: "validation_error",
			Message:   fmt.Sprintf("batch limit exceeded: %d operations, max %d", len(ops), maxBatch),
			Hint:      "Use --max-batch to increase the limit",
		}
		apiErr.WriteJSON(os.Stderr)
		return &jrerrors.AlreadyWrittenError{Code: jrerrors.ExitValidation}
	}
```

- [ ] **Step 3: Add policy check in executeBatchOp**

In `executeBatchOp`, after looking up the schema op (after line 226), add a policy check:

```go
	// Check policy for this operation.
	if baseClient.Policy != nil {
		if err := baseClient.Policy.Check(bop.Command); err != nil {
			apiErr := &jrerrors.APIError{
				ErrorType: "validation_error",
				Message:   err.Error(),
			}
			encoded, _ := json.Marshal(apiErr)
			return BatchResult{
				Index:    index,
				ExitCode: jrerrors.ExitValidation,
				Error:    json.RawMessage(encoded),
			}
		}
	}
```

Also set the `Operation` field on each `opClient` in `executeBatchOp` (after line 245):

```go
	opClient := &client.Client{
		// ... existing fields ...
		AuditLogger: baseClient.AuditLogger,
		Profile:     baseClient.Profile,
		Operation:   bop.Command,
	}
```

- [ ] **Step 4: Run tests**

Run: `cd /Users/quan.hoang/quanhh/quanhoang/jira-cli-v2 && go build ./... && go test ./...`
Expected: builds and all tests PASS.

- [ ] **Step 5: Commit**

```bash
git add cmd/batch.go
git commit -m "feat: add batch size limit and policy check per batch operation"
```

---

### Task 8: Update CLAUDE.md Documentation

**Files:**
- Modify: `CLAUDE.md`

- [ ] **Step 1: Add security features documentation**

Add to the CLAUDE.md's "Global Flags" section:

```
| `--audit` | enable audit logging for this invocation |
| `--audit-file <path>` | audit log file path (implies --audit) |
```

Add a new "Security" section to CLAUDE.md:

```markdown
## Security

### Operation Policy (per profile)
Restrict which operations a profile can execute:

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

Rules:
- Use `allowed_operations` OR `denied_operations`, not both
- Patterns use glob matching: `*` matches any sequence
- `allowed_operations`: implicit deny-all, only matching ops run
- `denied_operations`: implicit allow-all, only matching ops blocked

### Batch Limits
Default max batch size is 50. Override with `--max-batch N`.

### Audit Logging
Enable per-profile (`"audit_log": true`) or per-invocation (`--audit`).
Logs to `~/.config/jr/audit.log` (JSONL). Override path with `--audit-file`.
```

- [ ] **Step 2: Commit**

```bash
git add CLAUDE.md
git commit -m "docs: add security features to CLAUDE.md"
```

---

### Task 9: Integration Test

**Files:**
- Create: `cmd/security_test.go`

- [ ] **Step 1: Write integration tests**

Create `cmd/security_test.go` that tests the full flow via cobra command execution:

```go
package cmd

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sofq/jira-cli/internal/config"
)

func TestBatchMaxBatchFlag(t *testing.T) {
	// Create a config with a profile.
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.json")
	cfg := &config.Config{
		DefaultProfile: "default",
		Profiles: map[string]config.Profile{
			"default": {
				BaseURL: "https://example.com",
				Auth:    config.AuthConfig{Type: "basic", Username: "u", Token: "t"},
			},
		},
	}
	config.SaveTo(cfg, cfgPath)

	// Build a batch with 3 ops but set max-batch to 2.
	ops := `[{"command":"issue get","args":{"issueIdOrKey":"A-1"}},{"command":"issue get","args":{"issueIdOrKey":"A-2"}},{"command":"issue get","args":{"issueIdOrKey":"A-3"}}]`

	// Set up environment.
	os.Setenv("JR_CONFIG_PATH", cfgPath)
	defer os.Unsetenv("JR_CONFIG_PATH")

	var stderr bytes.Buffer
	cmd := rootCmd
	cmd.SetErr(&stderr)
	cmd.SetArgs([]string{"batch", "--max-batch", "2", "--input", writeTemp(t, ops)})

	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected error for batch exceeding max-batch")
	}
	if !strings.Contains(stderr.String(), "batch limit exceeded") {
		t.Errorf("stderr should mention batch limit, got: %s", stderr.String())
	}
}

func writeTemp(t *testing.T, content string) string {
	t.Helper()
	f, err := os.CreateTemp(t.TempDir(), "batch-*.json")
	if err != nil {
		t.Fatal(err)
	}
	f.WriteString(content)
	f.Close()
	return f.Name()
}
```

Note: this is a basic integration test. The full E2E test would require a mock Jira server. The unit tests in tasks 1-4 cover the core logic thoroughly.

- [ ] **Step 2: Run all tests**

Run: `cd /Users/quan.hoang/quanhh/quanhoang/jira-cli-v2 && go test ./...`
Expected: all tests PASS.

- [ ] **Step 3: Commit**

```bash
git add cmd/security_test.go
git commit -m "test: add integration tests for security features"
```

---

### Task 10: Final Verification

- [ ] **Step 1: Build**

Run: `cd /Users/quan.hoang/quanhh/quanhoang/jira-cli-v2 && make build`
Expected: builds without errors.

- [ ] **Step 2: Run all tests**

Run: `cd /Users/quan.hoang/quanhh/quanhoang/jira-cli-v2 && make test`
Expected: all tests PASS.

- [ ] **Step 3: Run linter**

Run: `cd /Users/quan.hoang/quanhh/quanhoang/jira-cli-v2 && make lint`
Expected: no lint errors.

- [ ] **Step 4: Verify dry-run with policy**

Run manually: `jr --dry-run --profile agent issue get --issueIdOrKey TEST-1`
Expected: works if "issue get" is in allowed_operations, errors if not.
