# jr (Agent-Friendly Jira CLI) Implementation Plan

> **For agentic workers:** REQUIRED: Use superpowers:subagent-driven-development (if subagents available) or superpowers:executing-plans to implement this plan. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build an agent-friendly Go CLI tool (`jr`) that auto-generates cobra commands from the Jira Cloud OpenAPI v3 spec. JSON-only output, structured errors, deterministic exit codes, env-var auth, built-in jq filtering, schema discovery, and batch mode.

**Architecture:** Code generator (`gen/`) parses the OpenAPI spec and emits Go source files with cobra commands + schema metadata. Each generated command resolves its own flags and delegates to a `net/http`-based client. The client handles auth, pagination, jq filtering, and structured error output.

**Tech Stack:** Go 1.22+, cobra, pb33f/libopenapi, net/http, itchyny/gojq, tidwall/pretty

**Spec:** `docs/superpowers/specs/2026-03-15-jr-jira-cli-design.md`

---

## File Structure

```
jr/
├── main.go                          # Entry point
├── go.mod
├── Makefile
├── spec/
│   └── jira-v3.json                 # Downloaded Jira OpenAPI spec
├── internal/
│   ├── errors/
│   │   └── errors.go               # Structured error types, exit codes, JSON stderr output
│   ├── config/
│   │   └── config.go               # Env var priority > config file, profiles
│   ├── jq/
│   │   └── jq.go                   # gojq wrapper for --jq flag
│   ├── client/
│   │   └── client.go               # HTTP client: Do(), auth, pagination, jq, errors
│   └── schema/
│       └── schema.go               # Schema registry for jr schema command
├── cmd/
│   ├── root.go                      # Root command, global flags, client injection
│   ├── configure.go                 # jr configure (flag-only, no prompts)
│   ├── schema_cmd.go                # jr schema
│   ├── batch.go                     # jr batch
│   ├── version.go                   # jr version (JSON)
│   ├── raw.go                       # jr raw
│   └── generated/
│       ├── init.go                  # Registers all resource commands
│       ├── schema_data.go           # Generated schema metadata
│       └── <resource>.go            # One per resource group (~77 files)
├── gen/
│   ├── main.go                      # Code generator entry
│   ├── parser.go                    # OpenAPI parsing
│   ├── grouper.go                   # Resource grouping + verb naming
│   ├── generator.go                 # Code emission
│   └── templates/
│       ├── resource.go.tmpl
│       ├── schema_data.go.tmpl
│       └── init.go.tmpl
```

---

## Chunk 1: Foundation (errors, config, jq, client)

### Task 1: Initialize Go module and project skeleton

**Files:**
- Create: `main.go`
- Create: `go.mod`
- Create: `Makefile`

- [ ] **Step 1: Initialize Go module**

```bash
cd /Users/quan.hoang/quanhh/quanhoang/jira-cli-v2
go mod init github.com/quanhoang/jr
```

- [ ] **Step 2: Create main.go**

Create `main.go`:
```go
package main

import (
	"os"

	"github.com/quanhoang/jr/cmd"
	"github.com/quanhoang/jr/internal/errors"
)

func main() {
	code := cmd.Execute()
	os.Exit(code)
}
```

- [ ] **Step 3: Create Makefile**

Create `Makefile`:
```makefile
.PHONY: generate build install test spec-update clean

SPEC_URL := https://dac-static.atlassian.com/cloud/jira/platform/swagger-v3.v3.json?_v=1.8420.0

generate:
	go run ./gen/...

build: generate
	go build -o jr .

install: generate
	go install .

test:
	go test ./...

spec-update:
	curl -sL "$(SPEC_URL)" -o spec/jira-v3.json

clean:
	rm -f jr
	rm -f cmd/generated/*.go
```

- [ ] **Step 4: Download spec**

```bash
mkdir -p spec
curl -sL "https://dac-static.atlassian.com/cloud/jira/platform/swagger-v3.v3.json?_v=1.8420.0" -o spec/jira-v3.json
```

- [ ] **Step 5: Commit**

```bash
git add main.go go.mod Makefile spec/jira-v3.json
git commit -m "feat: initialize project skeleton"
```

---

### Task 2: Structured errors package

**Files:**
- Create: `internal/errors/errors.go`
- Create: `internal/errors/errors_test.go`

- [ ] **Step 1: Write the failing test**

Create `internal/errors/errors_test.go`:
```go
package errors

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestExitCodeFromStatus(t *testing.T) {
	tests := []struct {
		status int
		want   int
	}{
		{401, ExitAuth},
		{403, ExitAuth},
		{404, ExitNotFound},
		{400, ExitValidation},
		{422, ExitValidation},
		{429, ExitRateLimit},
		{409, ExitConflict},
		{500, ExitServer},
		{502, ExitServer},
		{0, ExitError}, // network error
	}
	for _, tt := range tests {
		got := ExitCodeFromStatus(tt.status)
		if got != tt.want {
			t.Errorf("ExitCodeFromStatus(%d) = %d, want %d", tt.status, got, tt.want)
		}
	}
}

func TestAPIError_JSON(t *testing.T) {
	e := &APIError{
		ErrorType: "not_found",
		Status:    404,
		Message:   "Issue FOO-999 not found",
		Hint:      "Check the issue key",
		Request:   &RequestInfo{Method: "GET", Path: "/rest/api/3/issue/FOO-999"},
	}

	var buf strings.Builder
	e.WriteJSON(&buf)
	output := buf.String()

	var parsed map[string]interface{}
	if err := json.Unmarshal([]byte(output), &parsed); err != nil {
		t.Fatalf("error output is not valid JSON: %v\n%s", err, output)
	}
	if parsed["error"] != "not_found" {
		t.Errorf("unexpected error type: %v", parsed["error"])
	}
	if parsed["status"].(float64) != 404 {
		t.Errorf("unexpected status: %v", parsed["status"])
	}
}

func TestErrorTypeFromStatus(t *testing.T) {
	tests := []struct {
		status int
		want   string
	}{
		{401, "auth_failed"},
		{403, "auth_failed"},
		{404, "not_found"},
		{400, "validation_error"},
		{429, "rate_limited"},
		{409, "conflict"},
		{500, "server_error"},
		{0, "connection_error"},
	}
	for _, tt := range tests {
		got := ErrorTypeFromStatus(tt.status)
		if got != tt.want {
			t.Errorf("ErrorTypeFromStatus(%d) = %q, want %q", tt.status, got, tt.want)
		}
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

```bash
go test ./internal/errors/... -v
```
Expected: FAIL — package doesn't exist.

- [ ] **Step 3: Implement errors package**

Create `internal/errors/errors.go`:
```go
package errors

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
)

// Exit codes
const (
	ExitOK         = 0
	ExitError      = 1
	ExitAuth       = 2
	ExitNotFound   = 3
	ExitValidation = 4
	ExitRateLimit  = 5
	ExitConflict   = 6
	ExitServer     = 7
)

type RequestInfo struct {
	Method string `json:"method"`
	Path   string `json:"path"`
}

type APIError struct {
	ErrorType  string       `json:"error"`
	Status     int          `json:"status"`
	Message    string       `json:"message"`
	Hint       string       `json:"hint,omitempty"`
	Request    *RequestInfo `json:"request,omitempty"`
	RetryAfter *int         `json:"retry_after"`
}

func (e *APIError) Error() string {
	return fmt.Sprintf("%s (HTTP %d): %s", e.ErrorType, e.Status, e.Message)
}

func (e *APIError) ExitCode() int {
	return ExitCodeFromStatus(e.Status)
}

func (e *APIError) WriteJSON(w io.Writer) {
	data, _ := json.Marshal(e)
	fmt.Fprintln(w, string(data))
}

func (e *APIError) WriteStderr() {
	e.WriteJSON(os.Stderr)
}

func ExitCodeFromStatus(status int) int {
	switch {
	case status == 401 || status == 403:
		return ExitAuth
	case status == 404:
		return ExitNotFound
	case status == 400 || status == 422:
		return ExitValidation
	case status == 429:
		return ExitRateLimit
	case status == 409:
		return ExitConflict
	case status >= 500:
		return ExitServer
	default:
		return ExitError
	}
}

func ErrorTypeFromStatus(status int) string {
	switch {
	case status == 401 || status == 403:
		return "auth_failed"
	case status == 404:
		return "not_found"
	case status == 400 || status == 422:
		return "validation_error"
	case status == 429:
		return "rate_limited"
	case status == 409:
		return "conflict"
	case status >= 500:
		return "server_error"
	default:
		return "connection_error"
	}
}

func HintFromStatus(status int) string {
	switch {
	case status == 401 || status == 403:
		return "Set JR_BASE_URL, JR_AUTH_USER, JR_AUTH_TOKEN or run: jr configure --help"
	case status == 429:
		return "Rate limited. Wait and retry."
	default:
		return ""
	}
}

func NewFromHTTP(status int, body string, method, path string) *APIError {
	retryAfter := (*int)(nil)
	return &APIError{
		ErrorType:  ErrorTypeFromStatus(status),
		Status:     status,
		Message:    body,
		Hint:       HintFromStatus(status),
		Request:    &RequestInfo{Method: method, Path: path},
		RetryAfter: retryAfter,
	}
}
```

- [ ] **Step 4: Run tests**

```bash
go test ./internal/errors/... -v
```
Expected: all PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/errors/
git commit -m "feat: add structured errors with exit codes and JSON stderr output"
```

---

### Task 3: Config package — env vars > config file

**Files:**
- Create: `internal/config/config.go`
- Create: `internal/config/config_test.go`

- [ ] **Step 1: Write the failing test**

Create `internal/config/config_test.go`:
```go
package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestResolve_EnvVarsOverrideConfig(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.json")

	// Write a config file
	cfg := &Config{
		DefaultProfile: "default",
		Profiles: map[string]Profile{
			"default": {
				BaseURL: "https://file.atlassian.net",
				Auth: AuthConfig{
					Type:     "basic",
					Username: "file@example.com",
					Token:    "file-token",
				},
			},
		},
	}
	SaveTo(cfg, cfgPath)

	// Set env vars that should override
	t.Setenv("JR_BASE_URL", "https://env.atlassian.net")
	t.Setenv("JR_AUTH_TOKEN", "env-token")
	t.Setenv("JR_AUTH_USER", "")  // not set, should fall back to file
	t.Setenv("JR_AUTH_TYPE", "")  // not set

	resolved, err := Resolve(cfgPath, "", nil)
	if err != nil {
		t.Fatalf("Resolve failed: %v", err)
	}
	if resolved.BaseURL != "https://env.atlassian.net" {
		t.Errorf("expected env base URL, got %s", resolved.BaseURL)
	}
	if resolved.Auth.Token != "env-token" {
		t.Errorf("expected env token, got %s", resolved.Auth.Token)
	}
	// Username should fall back to config file
	if resolved.Auth.Username != "file@example.com" {
		t.Errorf("expected file username, got %s", resolved.Auth.Username)
	}
}

func TestResolve_FlagsOverrideEverything(t *testing.T) {
	t.Setenv("JR_BASE_URL", "https://env.atlassian.net")

	flags := &FlagOverrides{
		BaseURL:  "https://flag.atlassian.net",
		AuthType: "bearer",
		Token:    "flag-token",
	}
	resolved, err := Resolve("", "", flags)
	if err != nil {
		t.Fatalf("Resolve failed: %v", err)
	}
	if resolved.BaseURL != "https://flag.atlassian.net" {
		t.Errorf("expected flag base URL, got %s", resolved.BaseURL)
	}
	if resolved.Auth.Type != "bearer" {
		t.Errorf("expected bearer, got %s", resolved.Auth.Type)
	}
}

func TestResolve_PureEnvVars(t *testing.T) {
	t.Setenv("JR_BASE_URL", "https://pure-env.atlassian.net")
	t.Setenv("JR_AUTH_USER", "agent@co.com")
	t.Setenv("JR_AUTH_TOKEN", "pure-token")
	t.Setenv("JR_AUTH_TYPE", "basic")

	resolved, err := Resolve("", "", nil)
	if err != nil {
		t.Fatalf("Resolve failed: %v", err)
	}
	if resolved.BaseURL != "https://pure-env.atlassian.net" {
		t.Errorf("got %s", resolved.BaseURL)
	}
	if resolved.Auth.Username != "agent@co.com" {
		t.Errorf("got %s", resolved.Auth.Username)
	}
}

func TestSaveAndLoadConfig(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")

	cfg := &Config{
		DefaultProfile: "default",
		Profiles: map[string]Profile{
			"default": {
				BaseURL: "https://test.atlassian.net",
				Auth:    AuthConfig{Type: "basic", Username: "u", Token: "t"},
			},
		},
	}
	if err := SaveTo(cfg, path); err != nil {
		t.Fatalf("save: %v", err)
	}
	loaded, err := LoadFrom(path)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if loaded.Profiles["default"].BaseURL != "https://test.atlassian.net" {
		t.Errorf("unexpected: %s", loaded.Profiles["default"].BaseURL)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

```bash
go test ./internal/config/... -v
```
Expected: FAIL.

- [ ] **Step 3: Implement config package**

Create `internal/config/config.go`:
```go
package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

type AuthConfig struct {
	Type         string `json:"type"`
	Username     string `json:"username,omitempty"`
	Token        string `json:"token,omitempty"`
	ClientID     string `json:"client_id,omitempty"`
	ClientSecret string `json:"client_secret,omitempty"`
	TokenURL     string `json:"token_url,omitempty"`
	Scopes       string `json:"scopes,omitempty"`
}

type Profile struct {
	BaseURL string     `json:"base_url"`
	Auth    AuthConfig `json:"auth"`
}

type Config struct {
	Profiles       map[string]Profile `json:"profiles"`
	DefaultProfile string             `json:"default_profile"`
}

// FlagOverrides holds CLI flag values (highest priority).
type FlagOverrides struct {
	BaseURL  string
	AuthType string
	Username string
	Token    string
}

// ResolvedConfig is the final merged config used by the client.
type ResolvedConfig struct {
	BaseURL string
	Auth    AuthConfig
}

func DefaultPath() string {
	if p := os.Getenv("JR_CONFIG_PATH"); p != "" {
		return p
	}
	var dir string
	switch runtime.GOOS {
	case "darwin":
		home, _ := os.UserHomeDir()
		dir = filepath.Join(home, "Library", "Application Support", "jr")
	case "windows":
		dir = filepath.Join(os.Getenv("APPDATA"), "jr")
	default:
		home, _ := os.UserHomeDir()
		dir = filepath.Join(home, ".config", "jr")
	}
	return filepath.Join(dir, "config.json")
}

func LoadFrom(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return &Config{DefaultProfile: "default", Profiles: map[string]Profile{}}, nil
		}
		return nil, fmt.Errorf("reading config: %w", err)
	}
	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parsing config: %w", err)
	}
	if cfg.Profiles == nil {
		cfg.Profiles = map[string]Profile{}
	}
	if cfg.DefaultProfile == "" {
		cfg.DefaultProfile = "default"
	}
	return &cfg, nil
}

func SaveTo(cfg *Config, path string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("creating config dir: %w", err)
	}
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return fmt.Errorf("marshaling config: %w", err)
	}
	return os.WriteFile(path, data, 0o600)
}

// Resolve merges flags > env vars > config file into a single ResolvedConfig.
// Priority: flags > env vars > config file profile.
func Resolve(configPath, profileName string, flags *FlagOverrides) (*ResolvedConfig, error) {
	// Start with config file as base
	var base Profile
	if configPath != "" {
		cfg, err := LoadFrom(configPath)
		if err == nil {
			name := profileName
			if name == "" {
				name = cfg.DefaultProfile
			}
			if p, ok := cfg.Profiles[name]; ok {
				base = p
			}
		}
	}

	resolved := &ResolvedConfig{
		BaseURL: base.BaseURL,
		Auth:    base.Auth,
	}

	// Layer env vars (override non-empty)
	if v := os.Getenv("JR_BASE_URL"); v != "" {
		resolved.BaseURL = v
	}
	if v := os.Getenv("JR_AUTH_TYPE"); v != "" {
		resolved.Auth.Type = v
	}
	if v := os.Getenv("JR_AUTH_USER"); v != "" {
		resolved.Auth.Username = v
	}
	if v := os.Getenv("JR_AUTH_TOKEN"); v != "" {
		resolved.Auth.Token = v
	}

	// Layer flags (highest priority)
	if flags != nil {
		if flags.BaseURL != "" {
			resolved.BaseURL = flags.BaseURL
		}
		if flags.AuthType != "" {
			resolved.Auth.Type = flags.AuthType
		}
		if flags.Username != "" {
			resolved.Auth.Username = flags.Username
		}
		if flags.Token != "" {
			resolved.Auth.Token = flags.Token
		}
	}

	// Default auth type
	if resolved.Auth.Type == "" {
		resolved.Auth.Type = "basic"
	}

	// Trim trailing slash
	resolved.BaseURL = strings.TrimRight(resolved.BaseURL, "/")

	return resolved, nil
}
```

- [ ] **Step 4: Run tests**

```bash
go test ./internal/config/... -v
```
Expected: all PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/config/
git commit -m "feat: add config with env var > config file priority resolution"
```

---

### Task 4: JQ filter wrapper

**Files:**
- Create: `internal/jq/jq.go`
- Create: `internal/jq/jq_test.go`

- [ ] **Step 1: Write the failing test**

Create `internal/jq/jq_test.go`:
```go
package jq

import (
	"testing"
)

func TestApply_SimpleField(t *testing.T) {
	input := []byte(`{"key":"FOO-1","fields":{"summary":"Bug"}}`)
	result, err := Apply(input, ".fields.summary")
	if err != nil {
		t.Fatalf("Apply failed: %v", err)
	}
	if string(result) != `"Bug"` {
		t.Errorf("expected \"Bug\", got %s", string(result))
	}
}

func TestApply_ObjectConstruction(t *testing.T) {
	input := []byte(`{"key":"FOO-1","fields":{"summary":"Bug","status":{"name":"Open"}}}`)
	result, err := Apply(input, `{key: .key, status: .fields.status.name}`)
	if err != nil {
		t.Fatalf("Apply failed: %v", err)
	}
	// Should be valid JSON with key and status
	if len(result) == 0 {
		t.Error("empty result")
	}
}

func TestApply_ArrayFilter(t *testing.T) {
	input := []byte(`{"issues":[{"key":"A"},{"key":"B"}]}`)
	result, err := Apply(input, `[.issues[].key]`)
	if err != nil {
		t.Fatalf("Apply failed: %v", err)
	}
	if string(result) != `["A","B"]` {
		t.Errorf("expected [\"A\",\"B\"], got %s", string(result))
	}
}

func TestApply_InvalidFilter(t *testing.T) {
	input := []byte(`{"key":"FOO"}`)
	_, err := Apply(input, ".invalid[[[")
	if err == nil {
		t.Error("expected error for invalid filter")
	}
}

func TestApply_EmptyFilter(t *testing.T) {
	input := []byte(`{"key":"FOO"}`)
	result, err := Apply(input, "")
	if err != nil {
		t.Fatalf("Apply failed: %v", err)
	}
	// Empty filter returns input unchanged
	if string(result) != `{"key":"FOO"}` {
		t.Errorf("expected unchanged input, got %s", string(result))
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

```bash
go test ./internal/jq/... -v
```
Expected: FAIL.

- [ ] **Step 3: Implement jq wrapper**

Create `internal/jq/jq.go`:
```go
package jq

import (
	"encoding/json"
	"fmt"

	"github.com/itchyny/gojq"
)

// Apply runs a jq filter expression on JSON input and returns the result as JSON bytes.
// If filter is empty, returns input unchanged.
func Apply(input []byte, filter string) ([]byte, error) {
	if filter == "" {
		return input, nil
	}

	query, err := gojq.Parse(filter)
	if err != nil {
		return nil, fmt.Errorf("invalid jq filter: %w", err)
	}

	var data interface{}
	if err := json.Unmarshal(input, &data); err != nil {
		return nil, fmt.Errorf("invalid JSON input: %w", err)
	}

	iter := query.Run(data)
	v, ok := iter.Next()
	if !ok {
		return []byte("null"), nil
	}
	if err, isErr := v.(error); isErr {
		return nil, fmt.Errorf("jq error: %w", err)
	}

	result, err := json.Marshal(v)
	if err != nil {
		return nil, fmt.Errorf("marshaling jq result: %w", err)
	}
	return result, nil
}
```

- [ ] **Step 4: Install dependency and run tests**

```bash
go get github.com/itchyny/gojq
go test ./internal/jq/... -v
```
Expected: all PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/jq/
git commit -m "feat: add jq filter wrapper using gojq"
```

---

### Task 5: HTTP client — Do(), auth, pagination, jq, structured errors

**Files:**
- Create: `internal/client/client.go`
- Create: `internal/client/client_test.go`

- [ ] **Step 1: Write the failing test**

Create `internal/client/client_test.go`:
```go
package client

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/quanhoang/jr/internal/config"
	jerrors "github.com/quanhoang/jr/internal/errors"
	"github.com/spf13/cobra"
)

func TestDo_GET_JSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/rest/api/3/issue/FOO-1" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		if r.URL.Query().Get("fields") != "summary" {
			t.Errorf("unexpected query: %s", r.URL.RawQuery)
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"key":"FOO-1","fields":{"summary":"Test"}}`))
	}))
	defer srv.Close()

	var stdout strings.Builder
	c := &Client{
		BaseURL:    srv.URL,
		Auth:       config.AuthConfig{Type: "basic", Username: "u", Token: "t"},
		HTTPClient: srv.Client(),
		Stdout:     &stdout,
		Stderr:     io.Discard,
		Paginate:   true,
	}

	q := url.Values{"fields": {"summary"}}
	code := c.Do(context.Background(), "GET", "/rest/api/3/issue/FOO-1", q, nil)
	if code != jerrors.ExitOK {
		t.Fatalf("expected exit 0, got %d", code)
	}
	// Stdout should be valid JSON
	var result map[string]interface{}
	if err := json.Unmarshal([]byte(stdout.String()), &result); err != nil {
		t.Fatalf("stdout is not valid JSON: %v\n%s", err, stdout.String())
	}
	if result["key"] != "FOO-1" {
		t.Errorf("unexpected key: %v", result["key"])
	}
}

func TestDo_BasicAuth(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		user, pass, ok := r.BasicAuth()
		if !ok || user != "u" || pass != "t" {
			t.Errorf("bad basic auth: %s:%s ok=%v", user, pass, ok)
		}
		w.Write([]byte(`{}`))
	}))
	defer srv.Close()

	c := &Client{
		BaseURL:    srv.URL,
		Auth:       config.AuthConfig{Type: "basic", Username: "u", Token: "t"},
		HTTPClient: srv.Client(),
		Stdout:     io.Discard,
		Stderr:     io.Discard,
	}
	code := c.Do(context.Background(), "GET", "/test", nil, nil)
	if code != 0 {
		t.Errorf("expected 0, got %d", code)
	}
}

func TestDo_BearerAuth(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer my-pat" {
			t.Errorf("bad auth: %s", r.Header.Get("Authorization"))
		}
		w.Write([]byte(`{}`))
	}))
	defer srv.Close()

	c := &Client{
		BaseURL:    srv.URL,
		Auth:       config.AuthConfig{Type: "bearer", Token: "my-pat"},
		HTTPClient: srv.Client(),
		Stdout:     io.Discard,
		Stderr:     io.Discard,
	}
	code := c.Do(context.Background(), "GET", "/test", nil, nil)
	if code != 0 {
		t.Errorf("expected 0, got %d", code)
	}
}

func TestDo_HTTPError_StructuredStderr(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(404)
		w.Write([]byte(`{"errorMessages":["Issue not found"]}`))
	}))
	defer srv.Close()

	var stderr strings.Builder
	c := &Client{
		BaseURL:    srv.URL,
		Auth:       config.AuthConfig{Type: "basic", Username: "u", Token: "t"},
		HTTPClient: srv.Client(),
		Stdout:     io.Discard,
		Stderr:     &stderr,
	}
	code := c.Do(context.Background(), "GET", "/rest/api/3/issue/NOPE", nil, nil)
	if code != jerrors.ExitNotFound {
		t.Errorf("expected exit %d, got %d", jerrors.ExitNotFound, code)
	}
	// Stderr should be structured JSON
	var errObj map[string]interface{}
	if err := json.Unmarshal([]byte(stderr.String()), &errObj); err != nil {
		t.Fatalf("stderr not valid JSON: %v\n%s", err, stderr.String())
	}
	if errObj["error"] != "not_found" {
		t.Errorf("unexpected error type: %v", errObj["error"])
	}
}

func TestDo_JQFilter(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"key":"FOO-1","fields":{"summary":"Bug"}}`))
	}))
	defer srv.Close()

	var stdout strings.Builder
	c := &Client{
		BaseURL:    srv.URL,
		Auth:       config.AuthConfig{Type: "basic", Username: "u", Token: "t"},
		HTTPClient: srv.Client(),
		Stdout:     &stdout,
		Stderr:     io.Discard,
		JQFilter:   ".fields.summary",
	}
	code := c.Do(context.Background(), "GET", "/test", nil, nil)
	if code != 0 {
		t.Fatalf("expected 0, got %d", code)
	}
	if strings.TrimSpace(stdout.String()) != `"Bug"` {
		t.Errorf("expected \"Bug\", got %s", stdout.String())
	}
}

func TestDo_Pagination(t *testing.T) {
	callCount := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Query().Get("startAt") == "2" {
			json.NewEncoder(w).Encode(map[string]interface{}{
				"startAt": 2, "maxResults": 2, "total": 3,
				"values": []map[string]string{{"key": "C"}},
			})
		} else {
			json.NewEncoder(w).Encode(map[string]interface{}{
				"startAt": 0, "maxResults": 2, "total": 3,
				"values": []map[string]string{{"key": "A"}, {"key": "B"}},
			})
		}
	}))
	defer srv.Close()

	var stdout strings.Builder
	c := &Client{
		BaseURL:    srv.URL,
		Auth:       config.AuthConfig{Type: "basic", Username: "u", Token: "t"},
		HTTPClient: srv.Client(),
		Stdout:     &stdout,
		Stderr:     io.Discard,
		Paginate:   true,
	}
	code := c.Do(context.Background(), "GET", "/test", nil, nil)
	if code != 0 {
		t.Fatalf("expected 0, got %d", code)
	}
	if callCount != 2 {
		t.Errorf("expected 2 requests, got %d", callCount)
	}
	out := stdout.String()
	if !strings.Contains(out, "A") || !strings.Contains(out, "C") {
		t.Errorf("missing paginated results: %s", out)
	}
}

func TestDo_DryRun(t *testing.T) {
	var stdout strings.Builder
	c := &Client{
		BaseURL: "https://test.atlassian.net",
		Auth:    config.AuthConfig{Type: "basic", Username: "u", Token: "t"},
		Stdout:  &stdout,
		Stderr:  io.Discard,
		DryRun:  true,
	}
	code := c.Do(context.Background(), "GET", "/rest/api/3/issue/FOO-1", url.Values{"fields": {"summary"}}, nil)
	if code != 0 {
		t.Fatalf("expected 0, got %d", code)
	}
	var dryRun map[string]interface{}
	if err := json.Unmarshal([]byte(stdout.String()), &dryRun); err != nil {
		t.Fatalf("dry-run output not JSON: %v\n%s", err, stdout.String())
	}
	if dryRun["method"] != "GET" {
		t.Errorf("expected GET, got %v", dryRun["method"])
	}
}

func TestDo_POST_WithBody(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			t.Errorf("expected POST, got %s", r.Method)
		}
		body, _ := io.ReadAll(r.Body)
		if string(body) != `{"name":"test"}` {
			t.Errorf("unexpected body: %s", body)
		}
		if r.Header.Get("Content-Type") != "application/json" {
			t.Errorf("unexpected content-type: %s", r.Header.Get("Content-Type"))
		}
		w.Write([]byte(`{"id":"1"}`))
	}))
	defer srv.Close()

	c := &Client{
		BaseURL:    srv.URL,
		Auth:       config.AuthConfig{Type: "basic", Username: "u", Token: "t"},
		HTTPClient: srv.Client(),
		Stdout:     io.Discard,
		Stderr:     io.Discard,
	}
	body := strings.NewReader(`{"name":"test"}`)
	code := c.Do(context.Background(), "POST", "/test", nil, body)
	if code != 0 {
		t.Errorf("expected 0, got %d", code)
	}
}

func TestQueryFromFlags(t *testing.T) {
	cmd := &cobra.Command{}
	cmd.Flags().String("fields", "", "")
	cmd.Flags().String("expand", "", "")
	cmd.Flags().Set("fields", "summary,status")

	q := QueryFromFlags(cmd, "fields", "expand")
	if q.Get("fields") != "summary,status" {
		t.Errorf("expected 'summary,status', got %q", q.Get("fields"))
	}
	if q.Has("expand") {
		t.Error("expand should not be set")
	}
}

func TestFromContext(t *testing.T) {
	c := &Client{BaseURL: "https://test.atlassian.net"}
	ctx := NewContext(context.Background(), c)
	got, err := FromContext(ctx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.BaseURL != "https://test.atlassian.net" {
		t.Errorf("unexpected: %s", got.BaseURL)
	}
	_, err = FromContext(context.Background())
	if err == nil {
		t.Error("expected error from empty context")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

```bash
go get github.com/spf13/cobra
go test ./internal/client/... -v
```
Expected: FAIL.

- [ ] **Step 3: Implement client**

Create `internal/client/client.go`:
```go
package client

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"

	"github.com/quanhoang/jr/internal/config"
	jerrors "github.com/quanhoang/jr/internal/errors"
	"github.com/quanhoang/jr/internal/jq"
	"github.com/spf13/cobra"
	"github.com/tidwall/pretty"
)

type contextKey struct{}

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
}

func NewContext(ctx context.Context, c *Client) context.Context {
	return context.WithValue(ctx, contextKey{}, c)
}

func FromContext(ctx context.Context) (*Client, error) {
	c, ok := ctx.Value(contextKey{}).(*Client)
	if !ok || c == nil {
		return nil, fmt.Errorf("client not configured; set JR_BASE_URL and JR_AUTH_TOKEN env vars")
	}
	return c, nil
}

func QueryFromFlags(cmd *cobra.Command, names ...string) url.Values {
	q := url.Values{}
	for _, name := range names {
		f := cmd.Flags().Lookup(name)
		if f != nil && f.Changed {
			q.Set(name, f.Value.String())
		}
	}
	return q
}

func (c *Client) stdout() io.Writer {
	if c.Stdout != nil {
		return c.Stdout
	}
	return os.Stdout
}

func (c *Client) stderr() io.Writer {
	if c.Stderr != nil {
		return c.Stderr
	}
	return os.Stderr
}

func (c *Client) applyAuth(req *http.Request) {
	switch c.Auth.Type {
	case "basic":
		req.SetBasicAuth(c.Auth.Username, c.Auth.Token)
	case "bearer":
		req.Header.Set("Authorization", "Bearer "+c.Auth.Token)
	}
}

// Do executes an API request. Returns an exit code (0 = success).
func (c *Client) Do(ctx context.Context, method, path string, query url.Values, body io.Reader) int {
	u, err := url.Parse(c.BaseURL + path)
	if err != nil {
		c.writeError(jerrors.ExitError, "invalid_url", 0, err.Error(), method, path)
		return jerrors.ExitError
	}
	if query != nil && len(query) > 0 {
		u.RawQuery = query.Encode()
	}

	// Dry-run: output request as JSON
	if c.DryRun {
		dryRun := map[string]interface{}{
			"method": method,
			"url":    u.String(),
		}
		if query != nil && len(query) > 0 {
			dryRun["query"] = query
		}
		data, _ := json.Marshal(dryRun)
		fmt.Fprintln(c.stdout(), string(data))
		return jerrors.ExitOK
	}

	req, err := http.NewRequestWithContext(ctx, method, u.String(), body)
	if err != nil {
		c.writeError(jerrors.ExitError, "request_error", 0, err.Error(), method, path)
		return jerrors.ExitError
	}
	c.applyAuth(req)
	req.Header.Set("Accept", "application/json")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	if c.Verbose {
		fmt.Fprintf(c.stderr(), "> %s %s\n", method, u.String())
	}

	httpClient := c.HTTPClient
	if httpClient == nil {
		httpClient = http.DefaultClient
	}

	resp, err := httpClient.Do(req)
	if err != nil {
		c.writeError(jerrors.ExitError, "connection_error", 0, err.Error(), method, path)
		return jerrors.ExitError
	}
	defer resp.Body.Close()

	if c.Verbose {
		fmt.Fprintf(c.stderr(), "< %d %s\n", resp.StatusCode, resp.Status)
	}

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		c.writeError(jerrors.ExitError, "read_error", 0, err.Error(), method, path)
		return jerrors.ExitError
	}

	// Handle HTTP errors
	if resp.StatusCode >= 400 {
		apiErr := jerrors.NewFromHTTP(resp.StatusCode, string(respBody), method, path)
		if resp.StatusCode == 429 {
			if ra := resp.Header.Get("Retry-After"); ra != "" {
				// Parse retry-after if present
				apiErr.Hint = fmt.Sprintf("Rate limited. Retry after %s seconds.", ra)
			}
		}
		apiErr.WriteJSON(c.stderr())
		return apiErr.ExitCode()
	}

	// Handle pagination for GET requests
	if c.Paginate && method == "GET" {
		respBody = c.autoPaginate(ctx, httpClient, u, respBody, method)
	}

	// Apply JQ filter
	if c.JQFilter != "" {
		filtered, err := jq.Apply(respBody, c.JQFilter)
		if err != nil {
			c.writeError(jerrors.ExitError, "jq_error", 0, err.Error(), method, path)
			return jerrors.ExitError
		}
		respBody = filtered
	}

	// Output
	if c.Pretty {
		respBody = pretty.Pretty(respBody)
	}
	c.stdout().Write(respBody)
	fmt.Fprintln(c.stdout())
	return jerrors.ExitOK
}

type paginatedResponse struct {
	StartAt    int               `json:"startAt"`
	MaxResults int               `json:"maxResults"`
	Total      int               `json:"total"`
	Values     []json.RawMessage `json:"values"`
}

func (c *Client) autoPaginate(ctx context.Context, httpClient *http.Client, baseURL *url.URL, firstPage []byte, method string) []byte {
	var pr paginatedResponse
	if err := json.Unmarshal(firstPage, &pr); err != nil {
		return firstPage
	}
	if pr.MaxResults <= 0 || pr.Total <= 0 || pr.StartAt+pr.MaxResults >= pr.Total {
		return firstPage
	}

	allValues := pr.Values
	nextStart := pr.StartAt + pr.MaxResults

	for nextStart < pr.Total {
		pageQuery := baseURL.Query()
		pageQuery.Set("startAt", fmt.Sprintf("%d", nextStart))
		pageQuery.Set("maxResults", fmt.Sprintf("%d", pr.MaxResults))

		pageURL := *baseURL
		pageURL.RawQuery = pageQuery.Encode()

		pageReq, err := http.NewRequestWithContext(ctx, "GET", pageURL.String(), nil)
		if err != nil {
			break
		}
		c.applyAuth(pageReq)
		pageReq.Header.Set("Accept", "application/json")

		pageResp, err := httpClient.Do(pageReq)
		if err != nil {
			break
		}
		pageBody, _ := io.ReadAll(pageResp.Body)
		pageResp.Body.Close()
		if pageResp.StatusCode >= 400 {
			break
		}

		var pagePR paginatedResponse
		if err := json.Unmarshal(pageBody, &pagePR); err != nil {
			break
		}
		allValues = append(allValues, pagePR.Values...)
		nextStart += pr.MaxResults
	}

	merged, _ := json.Marshal(allValues)
	return merged
}

func (c *Client) writeError(exitCode int, errType string, status int, message, method, path string) {
	apiErr := &jerrors.APIError{
		ErrorType: errType,
		Status:    status,
		Message:   message,
		Request:   &jerrors.RequestInfo{Method: method, Path: path},
	}
	apiErr.WriteJSON(c.stderr())
}
```

- [ ] **Step 4: Install dependencies and run tests**

```bash
go get github.com/tidwall/pretty
go test ./internal/client/... -v
```
Expected: all tests PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/client/
git commit -m "feat: add HTTP client with auth, pagination, jq, structured errors"
```

---

### Task 6: Root command + built-in commands

**Files:**
- Create: `cmd/root.go`
- Create: `cmd/configure.go`
- Create: `cmd/version.go`
- Create: `cmd/raw.go`

- [ ] **Step 1: Create root command**

Create `cmd/root.go`:
```go
package cmd

import (
	"fmt"
	"os"

	"github.com/quanhoang/jr/internal/client"
	"github.com/quanhoang/jr/internal/config"
	jerrors "github.com/quanhoang/jr/internal/errors"
	"github.com/spf13/cobra"
)

var rootCmd = &cobra.Command{
	Use:           "jr",
	Short:         "Agent-friendly Jira CLI",
	Long:          "jr is an agent-friendly CLI for the Jira Cloud REST API v3. JSON output, structured errors, deterministic exit codes.",
	SilenceUsage:  true,
	SilenceErrors: true,
	PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
		// Skip client injection for commands that don't need it
		switch cmd.Name() {
		case "configure", "version", "completion", "help", "schema":
			return nil
		}

		flags := &config.FlagOverrides{}
		flags.BaseURL, _ = cmd.Flags().GetString("base-url")
		flags.AuthType, _ = cmd.Flags().GetString("auth-type")
		flags.Username, _ = cmd.Flags().GetString("auth-user")
		flags.Token, _ = cmd.Flags().GetString("auth-token")

		profileName, _ := cmd.Flags().GetString("profile")
		jqFilter, _ := cmd.Flags().GetString("jq")
		prettyFlag, _ := cmd.Flags().GetBool("pretty")
		noPaginate, _ := cmd.Flags().GetBool("no-paginate")
		verbose, _ := cmd.Flags().GetBool("verbose")
		dryRun, _ := cmd.Flags().GetBool("dry-run")

		resolved, err := config.Resolve(config.DefaultPath(), profileName, flags)
		if err != nil {
			return err
		}

		if resolved.BaseURL == "" {
			apiErr := &jerrors.APIError{
				ErrorType: "invalid_input",
				Status:    0,
				Message:   "No base URL configured",
				Hint:      "Set JR_BASE_URL env var or run: jr configure --help",
			}
			apiErr.WriteJSON(os.Stderr)
			return fmt.Errorf("no base URL")
		}

		c := &client.Client{
			BaseURL:  resolved.BaseURL,
			Auth:     resolved.Auth,
			Stdout:   os.Stdout,
			Stderr:   os.Stderr,
			JQFilter: jqFilter,
			Paginate: !noPaginate,
			Verbose:  verbose,
			DryRun:   dryRun,
			Pretty:   prettyFlag,
		}

		cmd.SetContext(client.NewContext(cmd.Context(), c))
		return nil
	},
}

func init() {
	pf := rootCmd.PersistentFlags()
	pf.StringP("profile", "p", "", "config file profile name")
	pf.String("base-url", "", "Jira base URL (overrides JR_BASE_URL)")
	pf.String("auth-type", "", "auth type: basic, bearer, oauth2")
	pf.String("auth-user", "", "auth username/email")
	pf.String("auth-token", "", "auth token")
	pf.String("jq", "", "jq filter expression applied to response")
	pf.Bool("pretty", false, "pretty-print JSON output (for humans)")
	pf.Bool("no-paginate", false, "disable auto-pagination")
	pf.Bool("verbose", false, "print request/response details to stderr")
	pf.Bool("dry-run", false, "print request as JSON without executing")
}

// Execute runs the root command and returns an exit code.
func Execute() int {
	if err := rootCmd.Execute(); err != nil {
		return jerrors.ExitError
	}
	return jerrors.ExitOK
}
```

- [ ] **Step 2: Create version command (JSON output)**

Create `cmd/version.go`:
```go
package cmd

import (
	"encoding/json"
	"fmt"

	"github.com/spf13/cobra"
)

var Version = "dev"

var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Print version as JSON",
	Run: func(cmd *cobra.Command, args []string) {
		data, _ := json.Marshal(map[string]string{"version": Version})
		fmt.Println(string(data))
	},
}

func init() {
	rootCmd.AddCommand(versionCmd)
}
```

- [ ] **Step 3: Create configure command (flag-driven, no prompts)**

Create `cmd/configure.go`:
```go
package cmd

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/quanhoang/jr/internal/config"
	jerrors "github.com/quanhoang/jr/internal/errors"
	"github.com/spf13/cobra"
)

var configureCmd = &cobra.Command{
	Use:   "configure",
	Short: "Set up a Jira connection profile (flag-driven, no prompts)",
	Long: `Configure a profile with all parameters as flags. No interactive prompts.

Example:
  jr configure --base-url https://mysite.atlassian.net --auth-type basic --username user@co.com --token ATATT3x...
  jr configure --profile staging --base-url https://staging.atlassian.net --auth-type bearer --token PAT-xxx --test`,
	RunE: func(cmd *cobra.Command, args []string) error {
		profileName, _ := cmd.Flags().GetString("profile")
		baseURL, _ := cmd.Flags().GetString("base-url")
		authType, _ := cmd.Flags().GetString("auth-type")
		username, _ := cmd.Flags().GetString("username")
		token, _ := cmd.Flags().GetString("token")
		testFlag, _ := cmd.Flags().GetBool("test")

		if baseURL == "" {
			apiErr := &jerrors.APIError{
				ErrorType: "invalid_input",
				Message:   "--base-url is required",
				Hint:      "jr configure --base-url https://yoursite.atlassian.net --username user@co.com --token TOKEN",
			}
			apiErr.WriteJSON(os.Stderr)
			return fmt.Errorf("missing --base-url")
		}
		if token == "" {
			apiErr := &jerrors.APIError{
				ErrorType: "invalid_input",
				Message:   "--token is required",
			}
			apiErr.WriteJSON(os.Stderr)
			return fmt.Errorf("missing --token")
		}

		if profileName == "" {
			profileName = "default"
		}
		if authType == "" {
			authType = "basic"
		}
		baseURL = strings.TrimRight(baseURL, "/")

		auth := config.AuthConfig{
			Type:     authType,
			Username: username,
			Token:    token,
		}

		// Test connection if requested
		if testFlag {
			testURL := baseURL + "/rest/api/3/myself"
			req, _ := http.NewRequest("GET", testURL, nil)
			switch authType {
			case "basic":
				req.SetBasicAuth(username, token)
			case "bearer":
				req.Header.Set("Authorization", "Bearer "+token)
			}
			req.Header.Set("Accept", "application/json")

			httpClient := &http.Client{Timeout: 10 * time.Second}
			resp, err := httpClient.Do(req)
			if err != nil {
				apiErr := &jerrors.APIError{
					ErrorType: "connection_error",
					Message:   fmt.Sprintf("Connection test failed: %v", err),
					Request:   &jerrors.RequestInfo{Method: "GET", Path: "/rest/api/3/myself"},
				}
				apiErr.WriteJSON(os.Stderr)
				return fmt.Errorf("connection test failed")
			}
			defer resp.Body.Close()
			if resp.StatusCode != 200 {
				body, _ := io.ReadAll(resp.Body)
				apiErr := jerrors.NewFromHTTP(resp.StatusCode, string(body), "GET", "/rest/api/3/myself")
				apiErr.WriteJSON(os.Stderr)
				return fmt.Errorf("connection test failed: HTTP %d", resp.StatusCode)
			}
		}

		cfgPath := config.DefaultPath()
		cfg, _ := config.LoadFrom(cfgPath)
		cfg.Profiles[profileName] = config.Profile{
			BaseURL: baseURL,
			Auth:    auth,
		}
		if len(cfg.Profiles) == 1 {
			cfg.DefaultProfile = profileName
		}

		if err := config.SaveTo(cfg, cfgPath); err != nil {
			return err
		}

		result, _ := json.Marshal(map[string]string{
			"status":  "saved",
			"profile": profileName,
			"path":    cfgPath,
		})
		fmt.Println(string(result))
		return nil
	},
}

func init() {
	f := configureCmd.Flags()
	f.String("profile", "default", "profile name")
	f.String("base-url", "", "Jira base URL (required)")
	f.String("auth-type", "basic", "auth type: basic, bearer, oauth2")
	f.String("username", "", "username/email for basic auth")
	f.String("token", "", "API token or PAT (required)")
	f.Bool("test", false, "test connection before saving")
	rootCmd.AddCommand(configureCmd)
}
```

- [ ] **Step 4: Create raw command**

Create `cmd/raw.go`:
```go
package cmd

import (
	"fmt"
	"io"
	"net/url"
	"os"
	"strings"

	"github.com/quanhoang/jr/internal/client"
	jerrors "github.com/quanhoang/jr/internal/errors"
	"github.com/spf13/cobra"
)

var rawCmd = &cobra.Command{
	Use:   "raw <method> <path>",
	Short: "Make a raw API request",
	Long:  "Make a raw API request. Example: jr raw GET /rest/api/3/myself",
	Args:  cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		c, err := client.FromContext(cmd.Context())
		if err != nil {
			return err
		}
		method := strings.ToUpper(args[0])
		path := args[1]

		bodyFlag, _ := cmd.Flags().GetString("body")
		var body io.Reader
		if bodyFlag != "" {
			if strings.HasPrefix(bodyFlag, "@") {
				f, err := os.Open(bodyFlag[1:])
				if err != nil {
					return fmt.Errorf("opening body file: %w", err)
				}
				defer f.Close()
				body = f
			} else {
				body = strings.NewReader(bodyFlag)
			}
		} else if stat, _ := os.Stdin.Stat(); stat != nil && (stat.Mode()&os.ModeCharDevice) == 0 {
			body = os.Stdin
		}

		queryFlag, _ := cmd.Flags().GetStringSlice("query")
		query := url.Values{}
		for _, q := range queryFlag {
			k, v, ok := strings.Cut(q, "=")
			if !ok {
				return fmt.Errorf("invalid query param %q (expected key=value)", q)
			}
			query.Add(k, v)
		}

		code := c.Do(cmd.Context(), method, path, query, body)
		if code != 0 {
			os.Exit(code)
		}
		return nil
	},
}

func init() {
	rawCmd.Flags().String("body", "", "request body (JSON string or @filename)")
	rawCmd.Flags().StringSlice("query", nil, "query params (key=value, repeatable)")
	rootCmd.AddCommand(rawCmd)
}
```

- [ ] **Step 5: Verify compilation**

```bash
go build ./...
```
Expected: compiles.

- [ ] **Step 6: Commit**

```bash
git add cmd/root.go cmd/configure.go cmd/version.go cmd/raw.go
git commit -m "feat: add root, configure (flag-only), version (JSON), and raw commands"
```

---

## Chunk 2: Code Generator

### Task 7: OpenAPI parser

**Files:**
- Create: `gen/parser.go`
- Create: `gen/parser_test.go`

- [ ] **Step 1: Write the failing test**

Create `gen/parser_test.go`:
```go
package main

import (
	"testing"
)

func TestParseSpec(t *testing.T) {
	ops, err := ParseSpec("../spec/jira-v3.json")
	if err != nil {
		t.Fatalf("ParseSpec failed: %v", err)
	}
	if len(ops) == 0 {
		t.Fatal("expected operations, got none")
	}
	found := false
	for _, op := range ops {
		if op.OperationID == "getIssue" {
			found = true
			if op.Method != "GET" {
				t.Errorf("expected GET, got %s", op.Method)
			}
			if len(op.PathParams) == 0 {
				t.Error("expected path params")
			}
			break
		}
	}
	if !found {
		t.Error("getIssue not found")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

```bash
go test ./gen/... -v -run TestParseSpec
```

- [ ] **Step 3: Implement parser**

Create `gen/parser.go`:
```go
package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/pb33f/libopenapi"
	base "github.com/pb33f/libopenapi/datamodel/high/base"
	v3 "github.com/pb33f/libopenapi/datamodel/high/v3"
)

type Param struct {
	Name        string
	In          string
	Required    bool
	Type        string
	Description string
}

type Operation struct {
	OperationID string
	Method      string
	Path        string
	Summary     string
	Description string
	PathParams  []Param
	QueryParams []Param
	HasBody     bool
}

func ParseSpec(path string) ([]Operation, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading spec: %w", err)
	}
	doc, err := libopenapi.NewDocument(data)
	if err != nil {
		return nil, fmt.Errorf("parsing spec: %w", err)
	}
	model, errs := doc.BuildV3Model()
	if len(errs) > 0 {
		return nil, fmt.Errorf("building model: %v", errs[0])
	}

	var ops []Operation
	for pair := model.Model.Paths.PathItems.First(); pair != nil; pair = pair.Next() {
		pathStr := pair.Key()
		pathItem := pair.Value()
		for method, op := range pathItemOperations(pathItem) {
			if op == nil {
				continue
			}
			parsed := Operation{
				OperationID: op.OperationId,
				Method:      strings.ToUpper(method),
				Path:        pathStr,
				Summary:     op.Summary,
				Description: op.Description,
			}
			allParams := make([]*v3.Parameter, 0, len(pathItem.Parameters)+len(op.Parameters))
			allParams = append(allParams, pathItem.Parameters...)
			allParams = append(allParams, op.Parameters...)
			for _, p := range allParams {
				if p == nil {
					continue
				}
				param := Param{
					Name:        p.Name,
					In:          p.In,
					Required:    p.Required != nil && *p.Required,
					Description: p.Description,
					Type:        schemaType(p.Schema),
				}
				switch p.In {
				case "path":
					param.Required = true
					parsed.PathParams = append(parsed.PathParams, param)
				case "query":
					parsed.QueryParams = append(parsed.QueryParams, param)
				}
			}
			if op.RequestBody != nil {
				parsed.HasBody = true
			}
			ops = append(ops, parsed)
		}
	}
	return ops, nil
}

func pathItemOperations(p *v3.PathItem) map[string]*v3.Operation {
	m := map[string]*v3.Operation{}
	if p.Get != nil { m["get"] = p.Get }
	if p.Post != nil { m["post"] = p.Post }
	if p.Put != nil { m["put"] = p.Put }
	if p.Delete != nil { m["delete"] = p.Delete }
	if p.Patch != nil { m["patch"] = p.Patch }
	return m
}

func schemaType(schema *base.SchemaProxy) string {
	if schema == nil {
		return "string"
	}
	s := schema.Schema()
	if s == nil {
		return "string"
	}
	if len(s.Type) > 0 {
		return s.Type[0]
	}
	return "string"
}
```

- [ ] **Step 4: Get dependency and run test**

```bash
go get github.com/pb33f/libopenapi
go test ./gen/... -v -run TestParseSpec
```
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add gen/parser.go gen/parser_test.go
git commit -m "feat: add OpenAPI spec parser"
```

---

### Task 8: Resource grouper + verb naming

**Files:**
- Create: `gen/grouper.go`
- Create: `gen/grouper_test.go`

- [ ] **Step 1: Write the failing test**

Create `gen/grouper_test.go`:
```go
package main

import (
	"testing"
)

func TestGroupOperations(t *testing.T) {
	ops := []Operation{
		{OperationID: "getIssue", Method: "GET", Path: "/rest/api/3/issue/{issueIdOrKey}"},
		{OperationID: "createIssue", Method: "POST", Path: "/rest/api/3/issue"},
		{OperationID: "getIssueTransitions", Method: "GET", Path: "/rest/api/3/issue/{issueIdOrKey}/transitions"},
		{OperationID: "getAllProjects", Method: "GET", Path: "/rest/api/3/project"},
	}
	groups := GroupOperations(ops)
	if len(groups) != 2 {
		t.Fatalf("expected 2 groups, got %d", len(groups))
	}
	if len(groups["issue"]) != 3 {
		t.Errorf("expected 3 issue ops, got %d", len(groups["issue"]))
	}
	if len(groups["project"]) != 1 {
		t.Errorf("expected 1 project op, got %d", len(groups["project"]))
	}
}

func TestDeriveVerb(t *testing.T) {
	tests := []struct {
		opID, method, path, resource, want string
	}{
		{"getIssue", "GET", "/rest/api/3/issue/{id}", "issue", "get"},
		{"createIssue", "POST", "/rest/api/3/issue", "issue", "create"},
		{"getIssueTransitions", "GET", "/rest/api/3/issue/{id}/transitions", "issue", "get-transitions"},
		{"deleteIssue", "DELETE", "/rest/api/3/issue/{id}", "issue", "delete"},
		{"getAllProjects", "GET", "/rest/api/3/project", "project", "get-all"},
	}
	for _, tt := range tests {
		t.Run(tt.opID, func(t *testing.T) {
			got := DeriveVerb(tt.opID, tt.method, tt.path, tt.resource)
			if got != tt.want {
				t.Errorf("DeriveVerb(%q) = %q, want %q", tt.opID, got, tt.want)
			}
		})
	}
}

func TestExtractResource(t *testing.T) {
	tests := []struct{ path, want string }{
		{"/rest/api/3/issue/{id}", "issue"},
		{"/rest/api/3/project", "project"},
		{"/rest/atlassian-connect/1/addons/{key}/properties", "atlassian-connect"},
	}
	for _, tt := range tests {
		got := ExtractResource(tt.path)
		if got != tt.want {
			t.Errorf("ExtractResource(%q) = %q, want %q", tt.path, got, tt.want)
		}
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

```bash
go test ./gen/... -v -run "TestGroup|TestDeriveVerb|TestExtractResource"
```

- [ ] **Step 3: Implement grouper**

Create `gen/grouper.go`:
```go
package main

import (
	"strings"
	"unicode"
)

func GroupOperations(ops []Operation) map[string][]Operation {
	groups := map[string][]Operation{}
	for _, op := range ops {
		resource := ExtractResource(op.Path)
		groups[resource] = append(groups[resource], op)
	}
	return groups
}

func ExtractResource(path string) string {
	parts := strings.Split(strings.Trim(path, "/"), "/")
	for i := 0; i+3 < len(parts); i++ {
		if parts[i] == "rest" && parts[i+1] == "api" && (parts[i+2] == "3" || parts[i+2] == "2") {
			return parts[i+3]
		}
	}
	if len(parts) >= 2 && parts[0] == "rest" {
		return parts[1]
	}
	if len(parts) > 0 {
		return parts[0]
	}
	return "root"
}

func DeriveVerb(operationID, method, path, resource string) string {
	if operationID == "" {
		return strings.ToLower(method)
	}
	words := splitCamelCase(operationID)
	if len(words) == 0 {
		return strings.ToLower(method)
	}
	verb := strings.ToLower(words[0])
	if len(words) == 1 {
		return verb
	}

	rest := make([]string, len(words)-1)
	for i, w := range words[1:] {
		rest[i] = strings.ToLower(w)
	}
	suffix := strings.Join(rest, "-")

	normalizedResource := strings.ToLower(strings.ReplaceAll(resource, "-", ""))
	normalizedSuffix := strings.ReplaceAll(suffix, "-", "")

	// Exact match: suffix IS the resource name
	if normalizedSuffix == normalizedResource {
		return verb
	}

	// Suffix ends with resource name (e.g., "getAllProjects" → "all-projects", resource="project")
	if strings.HasSuffix(normalizedSuffix, normalizedResource) || strings.HasSuffix(normalizedSuffix, normalizedResource+"s") {
		if len(rest) > 1 {
			return verb + "-" + strings.Join(rest[:len(rest)-1], "-")
		}
		return verb
	}

	// Suffix starts with resource name (e.g., "getIssueTransitions" → "issue-transitions", resource="issue")
	if strings.HasPrefix(normalizedSuffix, normalizedResource) {
		for i := 1; i <= len(rest); i++ {
			joined := strings.Join(rest[:i], "")
			if joined == normalizedResource {
				remaining := rest[i:]
				if len(remaining) > 0 {
					return verb + "-" + strings.Join(remaining, "-")
				}
				return verb
			}
		}
	}

	return verb + "-" + suffix
}

func splitCamelCase(s string) []string {
	var words []string
	var current []rune
	for i, r := range s {
		if unicode.IsUpper(r) && i > 0 {
			if len(current) > 0 {
				words = append(words, string(current))
			}
			current = []rune{r}
		} else {
			current = append(current, r)
		}
	}
	if len(current) > 0 {
		words = append(words, string(current))
	}
	return words
}
```

- [ ] **Step 4: Run tests**

```bash
go test ./gen/... -v -run "TestGroup|TestDeriveVerb|TestExtractResource"
```
Expected: all PASS.

- [ ] **Step 5: Commit**

```bash
git add gen/grouper.go gen/grouper_test.go
git commit -m "feat: add resource grouper with verb derivation"
```

---

### Task 9: Code generation templates

**Files:**
- Create: `gen/templates/resource.go.tmpl`
- Create: `gen/templates/schema_data.go.tmpl`
- Create: `gen/templates/init.go.tmpl`

- [ ] **Step 1: Create resource template**

Create `gen/templates/resource.go.tmpl`:
```
// Code generated by jr gen. DO NOT EDIT.
package generated

import (
	"fmt"
	"io"
	"net/url"
	"os"
	"strings"

	"github.com/quanhoang/jr/internal/client"
	jerrors "github.com/quanhoang/jr/internal/errors"
	"github.com/spf13/cobra"
)

var (
	_ = fmt.Sprintf
	_ = io.Discard
	_ = url.PathEscape
	_ = os.Open
	_ = strings.NewReader
	_ = jerrors.ExitOK
)

var {{.ResourceVarName}} = &cobra.Command{
	Use:   "{{.ResourceName}}",
	Short: "{{.ResourceName}} operations",
}
{{range .Operations}}
var {{.VarName}} = &cobra.Command{
	Use:   "{{.Verb}}",
	Short: {{printf "%q" .Summary}},
	Long:  {{printf "%q" .Description}},
	RunE: func(cmd *cobra.Command, args []string) error {
		c, err := client.FromContext(cmd.Context())
		if err != nil {
			return err
		}
		{{- range .PathParams}}
		{{.GoName}}, _ := cmd.Flags().GetString("{{.Name}}")
		{{- end}}

		path := fmt.Sprintf("{{.PathTemplate}}"{{.PathArgs}})
		query := client.QueryFromFlags(cmd{{.QueryFlagNames}})

		var body io.Reader
		{{- if .HasBody}}
		bodyFlag, _ := cmd.Flags().GetString("body")
		if bodyFlag != "" {
			if strings.HasPrefix(bodyFlag, "@") {
				f, err := os.Open(bodyFlag[1:])
				if err != nil {
					return fmt.Errorf("opening body file: %w", err)
				}
				defer f.Close()
				body = f
			} else {
				body = strings.NewReader(bodyFlag)
			}
		} else if stat, _ := os.Stdin.Stat(); stat != nil && (stat.Mode()&os.ModeCharDevice) == 0 {
			body = os.Stdin
		}
		{{- end}}

		code := c.Do(cmd.Context(), "{{.Method}}", path, query, body)
		if code != 0 {
			os.Exit(code)
		}
		return nil
	},
}

func init() {
	{{- range .PathParams}}
	{{.CmdVarName}}.Flags().String("{{.Name}}", "", {{printf "%q" .Description}})
	{{.CmdVarName}}.MarkFlagRequired("{{.Name}}")
	{{- end}}
	{{- range .QueryParams}}
	{{.CmdVarName}}.Flags().String("{{.Name}}", "", {{printf "%q" .Description}})
	{{- end}}
	{{- if .HasBody}}
	{{.VarName}}.Flags().String("body", "", "request body (JSON string or @filename, or pipe via stdin)")
	{{- end}}
	{{.ResourceVarName}}.AddCommand({{.VarName}})
}
{{end}}
```

- [ ] **Step 2: Create schema data template**

Create `gen/templates/schema_data.go.tmpl`:
```
// Code generated by jr gen. DO NOT EDIT.
package generated

// SchemaFlag describes a command flag for schema discovery.
type SchemaFlag struct {
	Name        string `json:"name"`
	Required    bool   `json:"required"`
	Type        string `json:"type"`
	Description string `json:"description"`
	In          string `json:"in"` // path, query, body
}

// SchemaOp describes a CLI operation for schema discovery.
type SchemaOp struct {
	Resource string       `json:"resource"`
	Verb     string       `json:"verb"`
	Method   string       `json:"method"`
	Path     string       `json:"path"`
	Summary  string       `json:"summary"`
	HasBody  bool         `json:"has_body"`
	Flags    []SchemaFlag `json:"flags"`
}

// AllSchemaOps returns metadata for all generated operations.
func AllSchemaOps() []SchemaOp {
	return []SchemaOp{
		{{- range .Operations}}
		{
			Resource: {{printf "%q" .Resource}},
			Verb:     {{printf "%q" .Verb}},
			Method:   {{printf "%q" .Method}},
			Path:     {{printf "%q" .Path}},
			Summary:  {{printf "%q" .Summary}},
			HasBody:  {{.HasBody}},
			Flags: []SchemaFlag{
				{{- range .Flags}}
				{Name: {{printf "%q" .Name}}, Required: {{.Required}}, Type: {{printf "%q" .Type}}, Description: {{printf "%q" .Description}}, In: {{printf "%q" .In}}},
				{{- end}}
			},
		},
		{{- end}}
	}
}

// AllResources returns the list of resource names.
func AllResources() []string {
	return []string{
		{{- range .Resources}}
		{{printf "%q" .}},
		{{- end}}
	}
}
```

- [ ] **Step 3: Create init template**

Create `gen/templates/init.go.tmpl`:
```
// Code generated by jr gen. DO NOT EDIT.
package generated

import (
	"github.com/spf13/cobra"
)

func RegisterAll(root *cobra.Command) {
	{{- range .Resources}}
	root.AddCommand({{.VarName}})
	{{- end}}
}
```

- [ ] **Step 4: Commit**

```bash
git add gen/templates/
git commit -m "feat: add code generation templates with schema data support"
```

---

### Task 10: Code generator + main

**Files:**
- Create: `gen/generator.go`
- Create: `gen/main.go`
- Create: `gen/generator_test.go`

- [ ] **Step 1: Write the failing test**

Create `gen/generator_test.go`:
```go
package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestGenerateResource(t *testing.T) {
	ops := []Operation{
		{
			OperationID: "getIssue",
			Method:      "GET",
			Path:        "/rest/api/3/issue/{issueIdOrKey}",
			Summary:     "Get issue",
			Description: "Returns the details for an issue.",
			PathParams:  []Param{{Name: "issueIdOrKey", In: "path", Required: true, Type: "string", Description: "The ID or key."}},
			QueryParams: []Param{{Name: "fields", In: "query", Type: "string", Description: "Fields to return."}},
		},
		{
			OperationID: "createIssue",
			Method:      "POST",
			Path:        "/rest/api/3/issue",
			Summary:     "Create issue",
			HasBody:     true,
		},
	}

	outDir := t.TempDir()
	err := GenerateResource("issue", ops, outDir)
	if err != nil {
		t.Fatalf("GenerateResource failed: %v", err)
	}

	content, err := os.ReadFile(filepath.Join(outDir, "issue.go"))
	if err != nil {
		t.Fatalf("reading: %v", err)
	}
	code := string(content)

	checks := []string{
		"package generated",
		`Use:   "issue"`,
		`Use:   "get"`,
		`Use:   "create"`,
		`"issueIdOrKey"`,
		`MarkFlagRequired("issueIdOrKey")`,
		`"fields"`,
		`"body"`,
		"DO NOT EDIT",
		"os.Exit(code)",
	}
	for _, check := range checks {
		if !strings.Contains(code, check) {
			t.Errorf("missing %q in generated code", check)
		}
	}
}

func TestGenerateInit(t *testing.T) {
	outDir := t.TempDir()
	err := GenerateInit([]string{"issue", "project"}, outDir)
	if err != nil {
		t.Fatalf("GenerateInit failed: %v", err)
	}
	content, err := os.ReadFile(filepath.Join(outDir, "init.go"))
	if err != nil {
		t.Fatalf("reading: %v", err)
	}
	code := string(content)
	if !strings.Contains(code, "issueCmd") || !strings.Contains(code, "projectCmd") {
		t.Errorf("missing resource registrations: %s", code)
	}
	if !strings.Contains(code, "RegisterAll") {
		t.Errorf("missing RegisterAll")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

```bash
go test ./gen/... -v -run TestGenerate
```

- [ ] **Step 3: Implement generator**

Create `gen/generator.go`:
```go
package main

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"text/template"
	"unicode"
)

var pathParamRe = regexp.MustCompile(`\{([^}]+)\}`)

type TemplateOp struct {
	VarName         string
	Verb            string
	Method          string
	Summary         string
	Description     string
	PathTemplate    string
	PathArgs        string
	PathParams      []TemplateParam
	QueryParams     []TemplateParam
	QueryFlagNames  string
	HasBody         bool
	ResourceVarName string
}

type TemplateParam struct {
	Name        string
	GoName      string
	Description string
	CmdVarName  string
}

type ResourceData struct {
	ResourceName    string
	ResourceVarName string
	Operations      []TemplateOp
}

type SchemaTemplateOp struct {
	Resource string
	Verb     string
	Method   string
	Path     string
	Summary  string
	HasBody  bool
	Flags    []SchemaTemplateFlag
}

type SchemaTemplateFlag struct {
	Name        string
	Required    bool
	Type        string
	Description string
	In          string
}

type SchemaData struct {
	Operations []SchemaTemplateOp
	Resources  []string
}

type InitData struct {
	Resources []InitResource
}

type InitResource struct {
	VarName string
}

func toGoIdentifier(s string) string {
	s = strings.ReplaceAll(s, "-", "_")
	s = strings.ReplaceAll(s, ".", "_")
	var result []rune
	for i, r := range s {
		if unicode.IsLetter(r) || unicode.IsDigit(r) || r == '_' {
			if i == 0 && unicode.IsDigit(r) {
				result = append(result, '_')
			}
			result = append(result, r)
		}
	}
	return string(result)
}

func resourceVarName(resource string) string {
	return toGoIdentifier(resource) + "Cmd"
}

func opVarName(resource, verb string) string {
	return toGoIdentifier(resource) + "_" + toGoIdentifier(verb)
}

func buildPathTemplate(path string) (string, string) {
	var args []string
	tmpl := pathParamRe.ReplaceAllStringFunc(path, func(match string) string {
		paramName := match[1 : len(match)-1]
		goName := toGoIdentifier(paramName)
		args = append(args, "url.PathEscape("+goName+")")
		return "%s"
	})
	argStr := ""
	if len(args) > 0 {
		argStr = ", " + strings.Join(args, ", ")
	}
	return tmpl, argStr
}

func loadTemplate(name string) (string, error) {
	// Try from gen/templates/ (when run from project root)
	path := filepath.Join("gen", "templates", name)
	data, err := os.ReadFile(path)
	if err != nil {
		// Fallback: from templates/ (when run from gen/ dir, e.g., tests)
		data, err = os.ReadFile(filepath.Join("templates", name))
		if err != nil {
			return "", fmt.Errorf("reading template %s: %w", name, err)
		}
	}
	return string(data), nil
}

func GenerateResource(resource string, ops []Operation, outDir string) error {
	tmplStr, err := loadTemplate("resource.go.tmpl")
	if err != nil {
		return err
	}
	tmpl, err := template.New("resource").Parse(tmplStr)
	if err != nil {
		return fmt.Errorf("parsing template: %w", err)
	}

	resVarName := resourceVarName(resource)
	var templateOps []TemplateOp

	for _, op := range ops {
		verb := DeriveVerb(op.OperationID, op.Method, op.Path, resource)
		pathTmpl, pathArgs := buildPathTemplate(op.Path)
		varName := opVarName(resource, verb)

		var pathParams []TemplateParam
		for _, p := range op.PathParams {
			pathParams = append(pathParams, TemplateParam{
				Name: p.Name, GoName: toGoIdentifier(p.Name),
				Description: p.Description, CmdVarName: varName,
			})
		}

		var queryParams []TemplateParam
		var queryNames []string
		for _, p := range op.QueryParams {
			queryParams = append(queryParams, TemplateParam{
				Name: p.Name, GoName: toGoIdentifier(p.Name),
				Description: p.Description, CmdVarName: varName,
			})
			queryNames = append(queryNames, fmt.Sprintf("%q", p.Name))
		}
		queryFlagStr := ""
		if len(queryNames) > 0 {
			queryFlagStr = ", " + strings.Join(queryNames, ", ")
		}

		summary := op.Summary
		if summary == "" {
			summary = op.OperationID
		}

		templateOps = append(templateOps, TemplateOp{
			VarName: varName, Verb: verb, Method: op.Method,
			Summary: summary, Description: op.Description,
			PathTemplate: pathTmpl, PathArgs: pathArgs,
			PathParams: pathParams, QueryParams: queryParams,
			QueryFlagNames: queryFlagStr, HasBody: op.HasBody,
			ResourceVarName: resVarName,
		})
	}

	data := ResourceData{
		ResourceName: resource, ResourceVarName: resVarName,
		Operations: templateOps,
	}

	outPath := filepath.Join(outDir, toGoIdentifier(resource)+".go")
	f, err := os.Create(outPath)
	if err != nil {
		return err
	}
	defer f.Close()
	return tmpl.Execute(f, data)
}

func GenerateSchemaData(groups map[string][]Operation, resources []string, outDir string) error {
	tmplStr, err := loadTemplate("schema_data.go.tmpl")
	if err != nil {
		return err
	}
	tmpl, err := template.New("schema").Parse(tmplStr)
	if err != nil {
		return fmt.Errorf("parsing template: %w", err)
	}

	var schemaOps []SchemaTemplateOp
	for _, resource := range resources {
		for _, op := range groups[resource] {
			verb := DeriveVerb(op.OperationID, op.Method, op.Path, resource)
			var flags []SchemaTemplateFlag
			for _, p := range op.PathParams {
				flags = append(flags, SchemaTemplateFlag{
					Name: p.Name, Required: true, Type: p.Type,
					Description: p.Description, In: "path",
				})
			}
			for _, p := range op.QueryParams {
				flags = append(flags, SchemaTemplateFlag{
					Name: p.Name, Required: p.Required, Type: p.Type,
					Description: p.Description, In: "query",
				})
			}
			if op.HasBody {
				flags = append(flags, SchemaTemplateFlag{
					Name: "body", Required: false, Type: "string",
					Description: "Request body (JSON or @filename)", In: "body",
				})
			}
			schemaOps = append(schemaOps, SchemaTemplateOp{
				Resource: resource, Verb: verb, Method: op.Method,
				Path: op.Path, Summary: op.Summary, HasBody: op.HasBody,
				Flags: flags,
			})
		}
	}

	data := SchemaData{Operations: schemaOps, Resources: resources}
	outPath := filepath.Join(outDir, "schema_data.go")
	f, err := os.Create(outPath)
	if err != nil {
		return err
	}
	defer f.Close()
	return tmpl.Execute(f, data)
}

func GenerateInit(resources []string, outDir string) error {
	tmplStr, err := loadTemplate("init.go.tmpl")
	if err != nil {
		return err
	}
	tmpl, err := template.New("init").Parse(tmplStr)
	if err != nil {
		return fmt.Errorf("parsing template: %w", err)
	}

	sort.Strings(resources)
	var initResources []InitResource
	for _, r := range resources {
		initResources = append(initResources, InitResource{VarName: resourceVarName(r)})
	}

	outPath := filepath.Join(outDir, "init.go")
	f, err := os.Create(outPath)
	if err != nil {
		return err
	}
	defer f.Close()
	return tmpl.Execute(f, InitData{Resources: initResources})
}
```

- [ ] **Step 4: Create gen/main.go**

Create `gen/main.go`:
```go
package main

import (
	"fmt"
	"os"
	"sort"
)

func main() {
	specPath := "spec/jira-v3.json"
	outDir := "cmd/generated"

	fmt.Printf("Parsing %s...\n", specPath)
	ops, err := ParseSpec(specPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("Found %d operations\n", len(ops))

	groups := GroupOperations(ops)
	fmt.Printf("Grouped into %d resources\n", len(groups))

	os.RemoveAll(outDir)
	os.MkdirAll(outDir, 0o755)

	var resources []string
	for resource, ops := range groups {
		if err := GenerateResource(resource, ops, outDir); err != nil {
			fmt.Fprintf(os.Stderr, "Error generating %s: %v\n", resource, err)
			os.Exit(1)
		}
		resources = append(resources, resource)
	}
	sort.Strings(resources)

	if err := GenerateSchemaData(groups, resources, outDir); err != nil {
		fmt.Fprintf(os.Stderr, "Error generating schema data: %v\n", err)
		os.Exit(1)
	}

	if err := GenerateInit(resources, outDir); err != nil {
		fmt.Fprintf(os.Stderr, "Error generating init: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Generated %d resource files + schema_data.go + init.go in %s\n", len(resources), outDir)
	for _, r := range resources {
		fmt.Printf("  %-30s %d operations\n", r, len(groups[r]))
	}
}
```

- [ ] **Step 5: Run tests**

```bash
go test ./gen/... -v -run TestGenerate
```
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add gen/
git commit -m "feat: add code generator with schema data emission"
```

---

### Task 11: Schema command + wire everything up

**Files:**
- Create: `cmd/schema_cmd.go`
- Modify: `cmd/root.go` — add `generated.RegisterAll(rootCmd)` to init

- [ ] **Step 1: Create schema command**

Create `cmd/schema_cmd.go`:
```go
package cmd

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/quanhoang/jr/cmd/generated"
	"github.com/spf13/cobra"
)

var schemaCmd = &cobra.Command{
	Use:   "schema [resource] [verb]",
	Short: "Discover available commands and flags (JSON output for agents)",
	Long: `Machine-readable command/flag discovery.

  jr schema --list          # list all resource names
  jr schema issue           # list operations for a resource
  jr schema issue get       # full schema for one operation`,
	Args: cobra.MaximumNArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		listFlag, _ := cmd.Flags().GetBool("list")

		if listFlag || len(args) == 0 {
			data, _ := json.Marshal(generated.AllResources())
			fmt.Println(string(data))
			return nil
		}

		resource := args[0]
		allOps := generated.AllSchemaOps()

		if len(args) == 1 {
			var matching []generated.SchemaOp
			for _, op := range allOps {
				if op.Resource == resource {
					matching = append(matching, op)
				}
			}
			if len(matching) == 0 {
				fmt.Fprintf(os.Stderr, `{"error":"not_found","message":"resource %q not found"}`+"\n", resource)
				os.Exit(3)
			}
			data, _ := json.Marshal(matching)
			fmt.Println(string(data))
			return nil
		}

		verb := args[1]
		for _, op := range allOps {
			if op.Resource == resource && op.Verb == verb {
				data, _ := json.Marshal(op)
				fmt.Println(string(data))
				return nil
			}
		}
		fmt.Fprintf(os.Stderr, `{"error":"not_found","message":"operation %s %s not found"}`+"\n", resource, verb)
		os.Exit(3)
		return nil
	},
}

func init() {
	schemaCmd.Flags().Bool("list", false, "list all resource names")
	rootCmd.AddCommand(schemaCmd)
}
```

- [ ] **Step 2: Wire generated commands into root**

Modify `cmd/root.go`: add import and registration. Add to the import block:
```go
"github.com/quanhoang/jr/cmd/generated"
```

Add as the last line of the existing `init()` function:
```go
generated.RegisterAll(rootCmd)
```

The full `init()` should be:
```go
func init() {
	pf := rootCmd.PersistentFlags()
	pf.StringP("profile", "p", "", "config file profile name")
	pf.String("base-url", "", "Jira base URL (overrides JR_BASE_URL)")
	pf.String("auth-type", "", "auth type: basic, bearer, oauth2")
	pf.String("auth-user", "", "auth username/email")
	pf.String("auth-token", "", "auth token")
	pf.String("jq", "", "jq filter expression applied to response")
	pf.Bool("pretty", false, "pretty-print JSON output (for humans)")
	pf.Bool("no-paginate", false, "disable auto-pagination")
	pf.Bool("verbose", false, "print request/response details to stderr")
	pf.Bool("dry-run", false, "print request as JSON without executing")
	generated.RegisterAll(rootCmd)
}
```

- [ ] **Step 3: Run generator and build**

```bash
make generate
make build
```

- [ ] **Step 4: Verify**

```bash
./jr --help
./jr issue --help
./jr issue get --help
./jr schema --list
./jr schema issue
./jr schema issue get
./jr version
```
Expected: all commands work, schema returns JSON.

- [ ] **Step 5: Fix any compilation errors and re-generate**

If generated code has issues, fix templates and re-run `make generate`.

- [ ] **Step 6: Commit**

```bash
git add cmd/root.go cmd/schema_cmd.go cmd/generated/
git commit -m "feat: wire up generated commands + schema discovery"
```

---

## Chunk 3: Batch Mode + E2E + Polish

### Task 12: Batch command

**Files:**
- Create: `cmd/batch.go`

- [ ] **Step 1: Create batch command**

Create `cmd/batch.go`:
```go
package cmd

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/quanhoang/jr/internal/client"
	"github.com/quanhoang/jr/internal/config"
	jerrors "github.com/quanhoang/jr/internal/errors"
	"github.com/spf13/cobra"
)

type BatchOp struct {
	Command string            `json:"command"`
	Args    map[string]string `json:"args"`
	JQ      string            `json:"jq,omitempty"`
}

type BatchResult struct {
	Index    int              `json:"index"`
	ExitCode int              `json:"exit_code"`
	Data     json.RawMessage  `json:"data,omitempty"`
	Error    *jerrors.APIError `json:"error,omitempty"`
}

var batchCmd = &cobra.Command{
	Use:   "batch",
	Short: "Execute multiple operations from JSON input",
	Long: `Process multiple operations in one invocation. Input from --input file or stdin.

Input format: [{"command": "issue get", "args": {"issueIdOrKey": "FOO-1"}, "jq": ".key"}, ...]`,
	RunE: func(cmd *cobra.Command, args []string) error {
		c, err := client.FromContext(cmd.Context())
		if err != nil {
			return err
		}

		inputFile, _ := cmd.Flags().GetString("input")
		var inputData []byte
		if inputFile != "" {
			inputData, err = os.ReadFile(inputFile)
			if err != nil {
				return fmt.Errorf("reading input: %w", err)
			}
		} else {
			inputData, err = io.ReadAll(os.Stdin)
			if err != nil {
				return fmt.Errorf("reading stdin: %w", err)
			}
		}

		var ops []BatchOp
		if err := json.Unmarshal(inputData, &ops); err != nil {
			return fmt.Errorf("parsing batch input: %w", err)
		}

		results := make([]BatchResult, len(ops))
		for i, op := range ops {
			result := executeBatchOp(c, cmd.Context(), op)
			result.Index = i
			results[i] = result
		}

		data, _ := json.Marshal(results)
		fmt.Println(string(data))
		return nil
	},
}

func executeBatchOp(c *client.Client, ctx context.Context, op BatchOp) BatchResult {
	parts := strings.Fields(op.Command)
	if len(parts) < 2 {
		return BatchResult{
			ExitCode: jerrors.ExitValidation,
			Error: &jerrors.APIError{
				ErrorType: "invalid_input",
				Message:   fmt.Sprintf("invalid command: %q (expected 'resource verb')", op.Command),
			},
		}
	}

	// Find the operation in schema data
	resource, verb := parts[0], parts[1]
	allOps := generated.AllSchemaOps()
	var schemaOp *generated.SchemaOp
	for _, s := range allOps {
		if s.Resource == resource && s.Verb == verb {
			schemaOp = &s
			break
		}
	}
	if schemaOp == nil {
		return BatchResult{
			ExitCode: jerrors.ExitNotFound,
			Error: &jerrors.APIError{
				ErrorType: "not_found",
				Message:   fmt.Sprintf("operation %s %s not found", resource, verb),
			},
		}
	}

	// Build path with path params substituted
	path := schemaOp.Path
	query := url.Values{}
	var body io.Reader

	for _, flag := range schemaOp.Flags {
		val, ok := op.Args[flag.Name]
		if !ok {
			continue
		}
		switch flag.In {
		case "path":
			path = strings.ReplaceAll(path, "{"+flag.Name+"}", url.PathEscape(val))
		case "query":
			query.Set(flag.Name, val)
		case "body":
			if strings.HasPrefix(val, "@") {
				f, err := os.Open(val[1:])
				if err != nil {
					return BatchResult{
						ExitCode: jerrors.ExitValidation,
						Error:    &jerrors.APIError{ErrorType: "invalid_input", Message: err.Error()},
					}
				}
				defer f.Close()
				body = f
			} else {
				body = strings.NewReader(val)
			}
		}
	}

	// Execute with a capturing writer
	var stdout strings.Builder
	var stderr strings.Builder
	batchClient := &client.Client{
		BaseURL:    c.BaseURL,
		Auth:       c.Auth,
		HTTPClient: c.HTTPClient,
		Stdout:     &stdout,
		Stderr:     &stderr,
		JQFilter:   op.JQ,
		Paginate:   c.Paginate,
	}

	code := batchClient.Do(ctx, schemaOp.Method, path, query, body)
	if code != 0 {
		var apiErr jerrors.APIError
		json.Unmarshal([]byte(stderr.String()), &apiErr)
		return BatchResult{ExitCode: code, Error: &apiErr}
	}
	return BatchResult{ExitCode: 0, Data: json.RawMessage(strings.TrimSpace(stdout.String()))}
}

func init() {
	batchCmd.Flags().String("input", "", "input file (JSON array of operations)")
	rootCmd.AddCommand(batchCmd)
}
```

Note: This file imports `generated` and `url` packages — add these imports:
```go
import (
	"github.com/quanhoang/jr/cmd/generated"
	"net/url"
)
```

- [ ] **Step 2: Verify compilation**

```bash
go build ./...
```

- [ ] **Step 3: Commit**

```bash
git add cmd/batch.go
git commit -m "feat: add batch command for multi-operation execution"
```

---

### Task 13: End-to-end test

**Files:**
- Create: `e2e_test.go`

- [ ] **Step 1: Write e2e test**

Create `e2e_test.go`:
```go
package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func buildBinary(t *testing.T) string {
	t.Helper()
	binary := filepath.Join(t.TempDir(), "jr")
	build := exec.Command("go", "build", "-o", binary, ".")
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build failed: %v\n%s", err, out)
	}
	return binary
}

func mockJira(t *testing.T) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.URL.Path == "/rest/api/3/myself":
			json.NewEncoder(w).Encode(map[string]string{"displayName": "Agent"})
		case r.URL.Path == "/rest/api/3/issue/TEST-1":
			json.NewEncoder(w).Encode(map[string]interface{}{
				"key": "TEST-1",
				"fields": map[string]string{"summary": "Test issue"},
			})
		case r.URL.Path == "/rest/api/3/issue/NOPE":
			w.WriteHeader(404)
			w.Write([]byte(`{"errorMessages":["Issue not found"]}`))
		default:
			w.WriteHeader(404)
			w.Write([]byte(`{"errorMessages":["Not found"]}`))
		}
	}))
}

func runJR(t *testing.T, binary string, srv *httptest.Server, args ...string) (stdout, stderr string, exitCode int) {
	t.Helper()
	cmd := exec.Command(binary, args...)
	cmd.Env = append(os.Environ(),
		"JR_BASE_URL="+srv.URL,
		"JR_AUTH_USER=test@example.com",
		"JR_AUTH_TOKEN=test-token",
		"JR_AUTH_TYPE=basic",
	)
	var outBuf, errBuf strings.Builder
	cmd.Stdout = &outBuf
	cmd.Stderr = &errBuf
	err := cmd.Run()
	code := 0
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			code = exitErr.ExitCode()
		}
	}
	return outBuf.String(), errBuf.String(), code
}

func TestE2E_IssueGet_JSON(t *testing.T) {
	if testing.Short() { t.Skip() }
	binary := buildBinary(t)
	srv := mockJira(t)
	defer srv.Close()

	stdout, _, code := runJR(t, binary, srv, "issue", "get", "--issueIdOrKey", "TEST-1")
	if code != 0 {
		t.Fatalf("expected exit 0, got %d", code)
	}
	var result map[string]interface{}
	if err := json.Unmarshal([]byte(stdout), &result); err != nil {
		t.Fatalf("stdout not valid JSON: %v\n%s", err, stdout)
	}
	if result["key"] != "TEST-1" {
		t.Errorf("unexpected key: %v", result["key"])
	}
}

func TestE2E_IssueGet_JQFilter(t *testing.T) {
	if testing.Short() { t.Skip() }
	binary := buildBinary(t)
	srv := mockJira(t)
	defer srv.Close()

	stdout, _, code := runJR(t, binary, srv, "issue", "get", "--issueIdOrKey", "TEST-1", "--jq", ".fields.summary")
	if code != 0 {
		t.Fatalf("expected exit 0, got %d", code)
	}
	if strings.TrimSpace(stdout) != `"Test issue"` {
		t.Errorf("expected \"Test issue\", got %s", stdout)
	}
}

func TestE2E_NotFound_StructuredError(t *testing.T) {
	if testing.Short() { t.Skip() }
	binary := buildBinary(t)
	srv := mockJira(t)
	defer srv.Close()

	_, stderr, code := runJR(t, binary, srv, "issue", "get", "--issueIdOrKey", "NOPE")
	if code != 3 { // ExitNotFound
		t.Fatalf("expected exit 3, got %d", code)
	}
	var errObj map[string]interface{}
	if err := json.Unmarshal([]byte(stderr), &errObj); err != nil {
		t.Fatalf("stderr not valid JSON: %v\n%s", err, stderr)
	}
	if errObj["error"] != "not_found" {
		t.Errorf("unexpected error type: %v", errObj["error"])
	}
}

func TestE2E_Version_JSON(t *testing.T) {
	if testing.Short() { t.Skip() }
	binary := buildBinary(t)
	srv := mockJira(t)
	defer srv.Close()

	stdout, _, code := runJR(t, binary, srv, "version")
	if code != 0 {
		t.Fatalf("expected exit 0, got %d", code)
	}
	var result map[string]string
	if err := json.Unmarshal([]byte(stdout), &result); err != nil {
		t.Fatalf("version output not JSON: %v\n%s", err, stdout)
	}
}

func TestE2E_Schema_List(t *testing.T) {
	if testing.Short() { t.Skip() }
	binary := buildBinary(t)
	srv := mockJira(t)
	defer srv.Close()

	stdout, _, code := runJR(t, binary, srv, "schema", "--list")
	if code != 0 {
		t.Fatalf("expected exit 0, got %d", code)
	}
	var resources []string
	if err := json.Unmarshal([]byte(stdout), &resources); err != nil {
		t.Fatalf("schema list not JSON array: %v\n%s", err, stdout)
	}
	if len(resources) == 0 {
		t.Error("expected resources")
	}
}

func TestE2E_DryRun_JSON(t *testing.T) {
	if testing.Short() { t.Skip() }
	binary := buildBinary(t)
	srv := mockJira(t)
	defer srv.Close()

	stdout, _, code := runJR(t, binary, srv, "--dry-run", "issue", "get", "--issueIdOrKey", "TEST-1")
	if code != 0 {
		t.Fatalf("expected exit 0, got %d", code)
	}
	var dryRun map[string]interface{}
	if err := json.Unmarshal([]byte(stdout), &dryRun); err != nil {
		t.Fatalf("dry-run not JSON: %v\n%s", err, stdout)
	}
	if dryRun["method"] != "GET" {
		t.Errorf("expected GET, got %v", dryRun["method"])
	}
}
```

- [ ] **Step 2: Run e2e tests**

```bash
make build
go test -v -run TestE2E ./...
```
Expected: all PASS.

- [ ] **Step 3: Commit**

```bash
git add e2e_test.go
git commit -m "test: add e2e tests for JSON output, jq, errors, schema, dry-run"
```

---

### Task 14: Shell completion + final verification

- [ ] **Step 1: Add completion command to root.go**

Add to `cmd/root.go` init(), before `generated.RegisterAll`:
```go
rootCmd.AddCommand(&cobra.Command{
	Use:   "completion [bash|zsh|fish|powershell]",
	Short: "Generate shell completion script",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		switch args[0] {
		case "bash":
			return rootCmd.GenBashCompletion(os.Stdout)
		case "zsh":
			return rootCmd.GenZshCompletion(os.Stdout)
		case "fish":
			return rootCmd.GenFishCompletion(os.Stdout, true)
		case "powershell":
			return rootCmd.GenPowerShellCompletion(os.Stdout)
		default:
			return fmt.Errorf("unsupported shell: %s", args[0])
		}
	},
})
```

- [ ] **Step 2: Full build and verify**

```bash
make clean && make build
./jr --help
./jr version
./jr schema --list | head -5
./jr schema issue get
./jr completion bash | head -5
```

- [ ] **Step 3: Run all tests**

```bash
make test
```
Expected: all pass.

- [ ] **Step 4: Commit**

```bash
git add cmd/root.go
git commit -m "feat: add shell completion command"
```
