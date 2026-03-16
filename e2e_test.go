package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// buildBinary compiles the jr binary into a temp directory and returns its path.
func buildBinary(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	out := filepath.Join(dir, "jr")
	cmd := exec.Command("go", "build", "-o", out, ".")
	cmd.Dir = "."
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		t.Fatalf("build failed: %v\n%s", err, stderr.String())
	}
	return out
}

// mockJira returns a test HTTP server that mimics a subset of the Jira REST API.
func mockJira(t *testing.T) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()

	mux.HandleFunc("/rest/api/3/myself", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"accountId":"abc123","displayName":"Agent","emailAddress":"agent@test.com","active":true}`))
	})

	mux.HandleFunc("/rest/api/3/issue/TEST-1", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"key":"TEST-1","fields":{"summary":"Test issue","status":{"name":"Open"}}}`))
	})

	mux.HandleFunc("/rest/api/3/issue/NOPE", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"errorMessages":["Issue not found"]}`))
	})

	// Paginated endpoint: project search with 2 pages.
	mux.HandleFunc("/rest/api/3/project/search", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		startAt := r.URL.Query().Get("startAt")
		if startAt == "" || startAt == "0" {
			_, _ = w.Write([]byte(`{"self":"http://localhost/rest/api/3/project/search","startAt":0,"maxResults":2,"total":3,"isLast":false,"values":[{"key":"PROJ1","name":"Project One"},{"key":"PROJ2","name":"Project Two"}]}`))
		} else {
			_, _ = w.Write([]byte(`{"self":"http://localhost/rest/api/3/project/search","startAt":2,"maxResults":2,"total":3,"isLast":true,"values":[{"key":"PROJ3","name":"Project Three"}]}`))
		}
	})

	// Paginated endpoint: empty results.
	mux.HandleFunc("/rest/api/3/priority/search", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"startAt":0,"maxResults":50,"total":0,"isLast":true,"values":[]}`))
	})

	// 204 No Content endpoint (for edit/delete operations).
	mux.HandleFunc("/rest/api/3/issue/TEST-204", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "PUT" || r.Method == "DELETE" {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"key":"TEST-204","fields":{"summary":"204 test"}}`))
	})

	// URL with ampersand (for HTML escaping test).
	mux.HandleFunc("/rest/api/3/project/search-amp", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"startAt":0,"maxResults":50,"total":1,"isLast":true,"values":[{"self":"http://localhost/rest?a=1&b=2","key":"AMP"}]}`))
	})

	// Transition endpoints for workflow tests.
	mux.HandleFunc("/rest/api/3/issue/TEST-1/transitions", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.Method {
		case "GET":
			_, _ = w.Write([]byte(`{"transitions":[{"id":"21","name":"Done","to":{"name":"Done"}},{"id":"31","name":"In Progress","to":{"name":"In Progress"}}]}`))
		case "POST":
			w.WriteHeader(http.StatusNoContent)
		}
	})

	// Assign endpoint.
	mux.HandleFunc("/rest/api/3/issue/TEST-1/assignee", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})

	// User search endpoint.
	mux.HandleFunc("/rest/api/3/user/search", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`[{"accountId":"abc123","displayName":"Test User"}]`))
	})

	// Rate limit endpoint.
	mux.HandleFunc("/rest/api/3/ratelimit", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Retry-After", "30")
		w.WriteHeader(http.StatusTooManyRequests)
		_, _ = w.Write([]byte(`{"message":"rate limited"}`))
	})

	// Server error endpoint.
	mux.HandleFunc("/rest/api/3/servererr", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"errorMessages":["internal error"]}`))
	})

	// Echo body endpoint (returns whatever body was sent).
	mux.HandleFunc("/rest/api/3/echo", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Body != nil {
			body, _ := io.ReadAll(r.Body)
			if len(body) > 0 {
				_, _ = w.Write(body)
				return
			}
		}
		_, _ = w.Write([]byte(`{}`))
	})

	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"errorMessages":["not found"]}`))
	})

	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

// runJR executes the jr binary with the mock server env vars and returns
// stdout, stderr, and the process exit code.
func runJR(t *testing.T, binary string, srv *httptest.Server, args ...string) (stdout, stderr string, exitCode int) {
	t.Helper()
	cmd := exec.Command(binary, args...)
	cmd.Env = append(os.Environ(),
		"JR_BASE_URL="+srv.URL,
		"JR_AUTH_USER=test@example.com",
		"JR_AUTH_TOKEN=test-token",
		"JR_AUTH_TYPE=basic",
		// Prevent the binary from reading any real config file.
		"JR_CONFIG_PATH="+filepath.Join(t.TempDir(), "config.json"),
	)

	var outBuf, errBuf bytes.Buffer
	cmd.Stdout = &outBuf
	cmd.Stderr = &errBuf

	err := cmd.Run()
	stdout = strings.TrimSpace(outBuf.String())
	stderr = strings.TrimSpace(errBuf.String())

	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			exitCode = exitErr.ExitCode()
		} else {
			t.Fatalf("unexpected error running binary: %v", err)
		}
	}
	return stdout, stderr, exitCode
}

func TestE2E_IssueGet_JSON(t *testing.T) {
	if testing.Short() {
		t.Skip()
	}
	binary := buildBinary(t)
	srv := mockJira(t)

	stdout, _, exitCode := runJR(t, binary, srv, "issue", "get", "--issueIdOrKey", "TEST-1", "--no-paginate")

	if exitCode != 0 {
		t.Fatalf("expected exit 0, got %d", exitCode)
	}

	var result map[string]interface{}
	if err := json.Unmarshal([]byte(stdout), &result); err != nil {
		t.Fatalf("stdout is not valid JSON: %v\nstdout: %s", err, stdout)
	}

	if key, ok := result["key"]; !ok || key != "TEST-1" {
		t.Fatalf("expected key=TEST-1 in JSON, got: %v", result)
	}
}

func TestE2E_IssueGet_JQFilter(t *testing.T) {
	if testing.Short() {
		t.Skip()
	}
	binary := buildBinary(t)
	srv := mockJira(t)

	stdout, _, exitCode := runJR(t, binary, srv, "issue", "get", "--issueIdOrKey", "TEST-1", "--no-paginate", "--jq", ".fields.summary")

	if exitCode != 0 {
		t.Fatalf("expected exit 0, got %d", exitCode)
	}

	// jq outputs the JSON-encoded string value, i.e. "Test issue" with quotes.
	if stdout != `"Test issue"` {
		t.Fatalf("expected stdout to be %q, got %q", `"Test issue"`, stdout)
	}
}

func TestE2E_NotFound_StructuredError(t *testing.T) {
	if testing.Short() {
		t.Skip()
	}
	binary := buildBinary(t)
	srv := mockJira(t)

	_, stderr, exitCode := runJR(t, binary, srv, "issue", "get", "--issueIdOrKey", "NOPE")

	if exitCode != 3 {
		t.Fatalf("expected exit 3 (ExitNotFound), got %d", exitCode)
	}

	var errObj map[string]interface{}
	if err := json.Unmarshal([]byte(stderr), &errObj); err != nil {
		t.Fatalf("stderr is not valid JSON: %v\nstderr: %s", err, stderr)
	}

	if errType, ok := errObj["error_type"]; !ok || errType != "not_found" {
		t.Fatalf("expected error_type=not_found in JSON error, got: %v", errObj)
	}
}

func TestE2E_Version_JSON(t *testing.T) {
	if testing.Short() {
		t.Skip()
	}
	binary := buildBinary(t)
	srv := mockJira(t)

	stdout, _, exitCode := runJR(t, binary, srv, "version")

	if exitCode != 0 {
		t.Fatalf("expected exit 0, got %d", exitCode)
	}

	var result map[string]interface{}
	if err := json.Unmarshal([]byte(stdout), &result); err != nil {
		t.Fatalf("stdout is not valid JSON: %v\nstdout: %s", err, stdout)
	}

	if _, ok := result["version"]; !ok {
		t.Fatalf("expected 'version' key in JSON output, got: %v", result)
	}
}

func TestE2E_Schema_List(t *testing.T) {
	if testing.Short() {
		t.Skip()
	}
	binary := buildBinary(t)
	srv := mockJira(t)

	stdout, _, exitCode := runJR(t, binary, srv, "schema", "--list")

	if exitCode != 0 {
		t.Fatalf("expected exit 0, got %d", exitCode)
	}

	var result []string
	if err := json.Unmarshal([]byte(stdout), &result); err != nil {
		t.Fatalf("stdout is not a JSON array of strings: %v\nstdout: %s", err, stdout)
	}

	if len(result) == 0 {
		t.Fatal("expected at least one resource name in schema list, got empty array")
	}
}

func TestE2E_DryRun_JSON(t *testing.T) {
	if testing.Short() {
		t.Skip()
	}
	binary := buildBinary(t)
	srv := mockJira(t)

	stdout, _, exitCode := runJR(t, binary, srv, "--dry-run", "issue", "get", "--issueIdOrKey", "TEST-1")

	if exitCode != 0 {
		t.Fatalf("expected exit 0, got %d", exitCode)
	}

	var result map[string]interface{}
	if err := json.Unmarshal([]byte(stdout), &result); err != nil {
		t.Fatalf("stdout is not valid JSON: %v\nstdout: %s", err, stdout)
	}

	if method, ok := result["method"]; !ok || method != "GET" {
		t.Fatalf("expected method=GET in dry-run JSON, got: %v", result)
	}
}

// --- Pagination + jq e2e tests (cover the bugs that were fixed) ---

// Non-paginated endpoint with pagination enabled should pass through unchanged.
func TestE2E_Myself_WithPaginationEnabled(t *testing.T) {
	if testing.Short() {
		t.Skip()
	}
	binary := buildBinary(t)
	srv := mockJira(t)

	// No --no-paginate: pagination is ON by default.
	stdout, stderr, exitCode := runJR(t, binary, srv, "myself", "get-current-user")

	if exitCode != 0 {
		t.Fatalf("expected exit 0, got %d; stderr=%s", exitCode, stderr)
	}

	var result map[string]interface{}
	if err := json.Unmarshal([]byte(stdout), &result); err != nil {
		t.Fatalf("stdout is not valid JSON: %v\nstdout: %s", err, stdout)
	}

	if result["displayName"] != "Agent" {
		t.Errorf("expected displayName=Agent, got %v", result["displayName"])
	}
}

// Non-paginated endpoint with pagination + jq filter.
func TestE2E_Myself_WithPagination_JQ(t *testing.T) {
	if testing.Short() {
		t.Skip()
	}
	binary := buildBinary(t)
	srv := mockJira(t)

	stdout, stderr, exitCode := runJR(t, binary, srv, "myself", "get-current-user", "--jq", ".displayName")

	if exitCode != 0 {
		t.Fatalf("expected exit 0, got %d; stderr=%s", exitCode, stderr)
	}

	if stdout != `"Agent"` {
		t.Errorf("expected %q, got %q", `"Agent"`, stdout)
	}
}

// Issue get (non-paginated) with pagination enabled.
func TestE2E_IssueGet_WithPaginationEnabled(t *testing.T) {
	if testing.Short() {
		t.Skip()
	}
	binary := buildBinary(t)
	srv := mockJira(t)

	stdout, stderr, exitCode := runJR(t, binary, srv, "issue", "get", "--issueIdOrKey", "TEST-1")

	if exitCode != 0 {
		t.Fatalf("expected exit 0, got %d; stderr=%s", exitCode, stderr)
	}

	var result map[string]interface{}
	if err := json.Unmarshal([]byte(stdout), &result); err != nil {
		t.Fatalf("stdout is not valid JSON: %v\nstdout: %s", err, stdout)
	}

	if result["key"] != "TEST-1" {
		t.Errorf("expected key=TEST-1, got %v", result["key"])
	}
}

// Issue get with pagination + jq filter.
func TestE2E_IssueGet_WithPagination_JQ(t *testing.T) {
	if testing.Short() {
		t.Skip()
	}
	binary := buildBinary(t)
	srv := mockJira(t)

	stdout, stderr, exitCode := runJR(t, binary, srv, "issue", "get", "--issueIdOrKey", "TEST-1", "--jq", "{key: .key, status: .fields.status.name}")

	if exitCode != 0 {
		t.Fatalf("expected exit 0, got %d; stderr=%s", exitCode, stderr)
	}

	var result map[string]interface{}
	if err := json.Unmarshal([]byte(stdout), &result); err != nil {
		t.Fatalf("stdout is not valid JSON: %v\nstdout: %s", err, stdout)
	}

	if result["key"] != "TEST-1" {
		t.Errorf("expected key=TEST-1, got %v", result["key"])
	}
	if result["status"] != "Open" {
		t.Errorf("expected status=Open, got %v", result["status"])
	}
}

// Paginated endpoint auto-paginates and merges all pages.
func TestE2E_ProjectSearch_Pagination(t *testing.T) {
	if testing.Short() {
		t.Skip()
	}
	binary := buildBinary(t)
	srv := mockJira(t)

	stdout, stderr, exitCode := runJR(t, binary, srv, "project", "search")

	if exitCode != 0 {
		t.Fatalf("expected exit 0, got %d; stderr=%s", exitCode, stderr)
	}

	var envelope struct {
		Values []map[string]interface{} `json:"values"`
		Total  int                      `json:"total"`
		IsLast bool                     `json:"isLast"`
	}
	if err := json.Unmarshal([]byte(stdout), &envelope); err != nil {
		t.Fatalf("stdout is not valid JSON: %v\nstdout: %s", err, stdout)
	}

	if len(envelope.Values) != 3 {
		t.Errorf("expected 3 projects (merged from 2 pages), got %d", len(envelope.Values))
	}
	if !envelope.IsLast {
		t.Error("expected isLast=true after merging all pages")
	}
}

// Paginated endpoint with jq extracting from envelope — .values[].key.
func TestE2E_ProjectSearch_Pagination_JQ(t *testing.T) {
	if testing.Short() {
		t.Skip()
	}
	binary := buildBinary(t)
	srv := mockJira(t)

	stdout, stderr, exitCode := runJR(t, binary, srv, "project", "search", "--jq", "[.values[].key]")

	if exitCode != 0 {
		t.Fatalf("expected exit 0, got %d; stderr=%s", exitCode, stderr)
	}

	var keys []string
	if err := json.Unmarshal([]byte(stdout), &keys); err != nil {
		t.Fatalf("stdout is not valid JSON array: %v\nstdout: %s", err, stdout)
	}

	expected := []string{"PROJ1", "PROJ2", "PROJ3"}
	if len(keys) != len(expected) {
		t.Fatalf("expected %d keys, got %d: %v", len(expected), len(keys), keys)
	}
	for i, k := range expected {
		if keys[i] != k {
			t.Errorf("key[%d]: expected %q, got %q", i, k, keys[i])
		}
	}
}

// Paginated endpoint with --no-paginate returns single page.
func TestE2E_ProjectSearch_NoPaginate(t *testing.T) {
	if testing.Short() {
		t.Skip()
	}
	binary := buildBinary(t)
	srv := mockJira(t)

	stdout, stderr, exitCode := runJR(t, binary, srv, "project", "search", "--no-paginate")

	if exitCode != 0 {
		t.Fatalf("expected exit 0, got %d; stderr=%s", exitCode, stderr)
	}

	var envelope struct {
		Values []map[string]interface{} `json:"values"`
		Total  int                      `json:"total"`
		IsLast bool                     `json:"isLast"`
	}
	if err := json.Unmarshal([]byte(stdout), &envelope); err != nil {
		t.Fatalf("stdout is not valid JSON: %v\nstdout: %s", err, stdout)
	}

	// With --no-paginate, only the first page is returned.
	if len(envelope.Values) != 2 {
		t.Errorf("expected 2 projects (first page only), got %d", len(envelope.Values))
	}
}

// Same jq filter works with and without --no-paginate.
func TestE2E_ProjectSearch_JQConsistency(t *testing.T) {
	if testing.Short() {
		t.Skip()
	}
	binary := buildBinary(t)
	srv := mockJira(t)

	jqFilter := ".values[0].key"

	// With pagination.
	stdout1, stderr1, exit1 := runJR(t, binary, srv, "project", "search", "--jq", jqFilter)
	if exit1 != 0 {
		t.Fatalf("paginated: expected exit 0, got %d; stderr=%s", exit1, stderr1)
	}

	// Without pagination.
	stdout2, stderr2, exit2 := runJR(t, binary, srv, "project", "search", "--no-paginate", "--jq", jqFilter)
	if exit2 != 0 {
		t.Fatalf("no-paginate: expected exit 0, got %d; stderr=%s", exit2, stderr2)
	}

	if stdout1 != stdout2 {
		t.Errorf("jq results differ:\n  paginated:    %s\n  no-paginate:  %s", stdout1, stdout2)
	}
}

// Paginated endpoint with empty results.
func TestE2E_PrioritySearch_EmptyResults(t *testing.T) {
	if testing.Short() {
		t.Skip()
	}
	binary := buildBinary(t)
	srv := mockJira(t)

	stdout, stderr, exitCode := runJR(t, binary, srv, "priority", "search")

	if exitCode != 0 {
		t.Fatalf("expected exit 0, got %d; stderr=%s", exitCode, stderr)
	}

	var envelope struct {
		Values []interface{} `json:"values"`
		Total  int           `json:"total"`
	}
	if err := json.Unmarshal([]byte(stdout), &envelope); err != nil {
		t.Fatalf("stdout is not valid JSON: %v\nstdout: %s", err, stdout)
	}

	if len(envelope.Values) != 0 {
		t.Errorf("expected 0 values for empty search, got %d", len(envelope.Values))
	}
}

// === Bug fix regression tests ===

// Bug #1: Empty --base-url should return validation error.
func TestE2E_Configure_EmptyBaseURL(t *testing.T) {
	if testing.Short() {
		t.Skip()
	}
	binary := buildBinary(t)
	srv := mockJira(t)

	cmd := exec.Command(binary, "configure", "--base-url", "", "--token", "tok")
	cmd.Env = append(os.Environ(), "JR_CONFIG_PATH="+filepath.Join(t.TempDir(), "config.json"))
	var outBuf, errBuf bytes.Buffer
	cmd.Stdout = &outBuf
	cmd.Stderr = &errBuf
	_ = srv // not needed, just keeping helper usage consistent

	err := cmd.Run()
	if err == nil {
		t.Fatal("expected non-zero exit for empty --base-url")
	}
	exitErr, ok := err.(*exec.ExitError)
	if !ok {
		t.Fatalf("unexpected error type: %v", err)
	}
	if exitErr.ExitCode() != 4 {
		t.Errorf("expected exit code 4 (validation), got %d", exitErr.ExitCode())
	}

	var errObj map[string]interface{}
	if jsonErr := json.Unmarshal(errBuf.Bytes(), &errObj); jsonErr != nil {
		t.Fatalf("stderr not valid JSON: %s", errBuf.String())
	}
	if errObj["error_type"] != "validation_error" {
		t.Errorf("expected error_type=validation_error, got %v", errObj["error_type"])
	}
}

// Bug #2: Empty --token should return validation error.
func TestE2E_Configure_EmptyToken(t *testing.T) {
	if testing.Short() {
		t.Skip()
	}
	binary := buildBinary(t)

	cmd := exec.Command(binary, "configure", "--base-url", "https://example.com", "--token", "")
	cmd.Env = append(os.Environ(), "JR_CONFIG_PATH="+filepath.Join(t.TempDir(), "config.json"))
	var errBuf bytes.Buffer
	cmd.Stderr = &errBuf

	err := cmd.Run()
	if err == nil {
		t.Fatal("expected non-zero exit for empty --token")
	}
	exitErr, ok := err.(*exec.ExitError)
	if !ok {
		t.Fatalf("unexpected error type: %v", err)
	}
	if exitErr.ExitCode() != 4 {
		t.Errorf("expected exit code 4 (validation), got %d", exitErr.ExitCode())
	}
}

// Bug #3: POST without --body should error immediately, not hang.
func TestE2E_Raw_PostWithoutBody(t *testing.T) {
	if testing.Short() {
		t.Skip()
	}
	binary := buildBinary(t)
	srv := mockJira(t)

	_, stderr, exitCode := runJR(t, binary, srv, "raw", "POST", "/rest/api/3/issue")

	if exitCode != 4 {
		t.Fatalf("expected exit code 4 (validation), got %d; stderr=%s", exitCode, stderr)
	}

	var errObj map[string]interface{}
	if err := json.Unmarshal([]byte(stderr), &errObj); err != nil {
		t.Fatalf("stderr not valid JSON: %s", stderr)
	}
	if errObj["error_type"] != "validation_error" {
		t.Errorf("expected error_type=validation_error, got %v", errObj["error_type"])
	}
	if !strings.Contains(errObj["message"].(string), "body") {
		t.Errorf("error message should mention body, got: %v", errObj["message"])
	}
}

// Bug #3: PUT without --body should also error.
func TestE2E_Raw_PutWithoutBody(t *testing.T) {
	if testing.Short() {
		t.Skip()
	}
	binary := buildBinary(t)
	srv := mockJira(t)

	_, stderr, exitCode := runJR(t, binary, srv, "raw", "PUT", "/rest/api/3/issue/TEST-1")

	if exitCode != 4 {
		t.Fatalf("expected exit code 4 (validation), got %d; stderr=%s", exitCode, stderr)
	}
}

// Bug #3: POST with --body should work fine.
func TestE2E_Raw_PostWithBody(t *testing.T) {
	if testing.Short() {
		t.Skip()
	}
	binary := buildBinary(t)
	srv := mockJira(t)

	stdout, stderr, exitCode := runJR(t, binary, srv, "raw", "POST", "/rest/api/3/myself", "--body", `{}`)

	// The mock returns 404 for POST to /myself, but that's fine - we're testing that it doesn't hang.
	// Any response (even an error) means the request was sent.
	_ = stdout
	_ = stderr
	if exitCode == 4 {
		t.Fatal("should NOT get validation error when --body is provided")
	}
}

// Bug #4: HTTP 204 should return {} on stdout.
func TestE2E_Issue_Edit_Returns_EmptyJSON(t *testing.T) {
	if testing.Short() {
		t.Skip()
	}
	binary := buildBinary(t)
	srv := mockJira(t)

	stdout, stderr, exitCode := runJR(t, binary, srv, "raw", "PUT", "/rest/api/3/issue/TEST-204", "--body", `{"fields":{"summary":"updated"}}`)

	if exitCode != 0 {
		t.Fatalf("expected exit 0, got %d; stderr=%s", exitCode, stderr)
	}
	if stdout != "{}" {
		t.Errorf("expected '{}' for 204 response, got %q", stdout)
	}
}

// Bug #5: --jq on 204 response should work.
func TestE2E_Issue_Edit_WithJQ(t *testing.T) {
	if testing.Short() {
		t.Skip()
	}
	binary := buildBinary(t)
	srv := mockJira(t)

	stdout, stderr, exitCode := runJR(t, binary, srv, "raw", "PUT", "/rest/api/3/issue/TEST-204",
		"--body", `{"fields":{"summary":"updated"}}`,
		"--jq", `.status // "ok"`)

	if exitCode != 0 {
		t.Fatalf("expected exit 0, got %d; stderr=%s", exitCode, stderr)
	}
	if stdout != `"ok"` {
		t.Errorf("expected jq result %q, got %q", `"ok"`, stdout)
	}
}

// Bug #6: jr --help should exit 0 with JSON on stdout.
func TestE2E_Help_ExitZero(t *testing.T) {
	if testing.Short() {
		t.Skip()
	}
	binary := buildBinary(t)
	srv := mockJira(t)

	stdout, _, exitCode := runJR(t, binary, srv, "--help")

	if exitCode != 0 {
		t.Fatalf("expected exit 0 for --help, got %d", exitCode)
	}

	// Stdout should be valid JSON.
	var result map[string]interface{}
	if err := json.Unmarshal([]byte(stdout), &result); err != nil {
		t.Fatalf("--help stdout is not valid JSON: %v\nstdout: %s", err, stdout)
	}

	// Should contain a hint.
	if _, ok := result["hint"]; !ok {
		t.Error("expected 'hint' key in help JSON output")
	}
}

// Bug #7: Invalid HTTP method should return validation error.
func TestE2E_Raw_InvalidMethod(t *testing.T) {
	if testing.Short() {
		t.Skip()
	}
	binary := buildBinary(t)
	srv := mockJira(t)

	_, stderr, exitCode := runJR(t, binary, srv, "raw", "INVALID", "/rest/api/3/myself")

	if exitCode != 4 {
		t.Fatalf("expected exit code 4 (validation), got %d", exitCode)
	}

	var errObj map[string]interface{}
	if err := json.Unmarshal([]byte(stderr), &errObj); err != nil {
		t.Fatalf("stderr not valid JSON: %s", stderr)
	}
	if errObj["error_type"] != "validation_error" {
		t.Errorf("expected error_type=validation_error, got %v", errObj["error_type"])
	}
}

// Bug #9: Batch with failing ops should exit non-zero.
func TestE2E_Batch_FailingOps_NonZeroExit(t *testing.T) {
	if testing.Short() {
		t.Skip()
	}
	binary := buildBinary(t)
	srv := mockJira(t)

	input := `[{"command":"issue get","args":{"issueIdOrKey":"NOPE"}}]`
	cmd := exec.Command(binary, "batch")
	cmd.Stdin = strings.NewReader(input)
	cmd.Env = append(os.Environ(),
		"JR_BASE_URL="+srv.URL,
		"JR_AUTH_USER=test@example.com",
		"JR_AUTH_TOKEN=test-token",
		"JR_CONFIG_PATH="+filepath.Join(t.TempDir(), "config.json"),
	)

	var outBuf, errBuf bytes.Buffer
	cmd.Stdout = &outBuf
	cmd.Stderr = &errBuf

	err := cmd.Run()
	if err == nil {
		t.Fatal("expected non-zero exit for batch with failing ops")
	}
	exitErr, ok := err.(*exec.ExitError)
	if !ok {
		t.Fatalf("unexpected error type: %v", err)
	}
	if exitErr.ExitCode() == 0 {
		t.Error("expected non-zero exit code")
	}

	// Stdout should still have the results array.
	var results []map[string]interface{}
	if jsonErr := json.Unmarshal(outBuf.Bytes(), &results); jsonErr != nil {
		t.Fatalf("stdout not valid JSON array: %s", outBuf.String())
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if results[0]["exit_code"].(float64) == 0 {
		t.Error("expected non-zero exit_code in result")
	}
}

// Bug #9: Batch with all succeeding ops should exit 0.
func TestE2E_Batch_AllSuccess_ExitZero(t *testing.T) {
	if testing.Short() {
		t.Skip()
	}
	binary := buildBinary(t)
	srv := mockJira(t)

	input := `[{"command":"issue get","args":{"issueIdOrKey":"TEST-1"},"jq":".key"}]`
	cmd := exec.Command(binary, "batch")
	cmd.Stdin = strings.NewReader(input)
	cmd.Env = append(os.Environ(),
		"JR_BASE_URL="+srv.URL,
		"JR_AUTH_USER=test@example.com",
		"JR_AUTH_TOKEN=test-token",
		"JR_CONFIG_PATH="+filepath.Join(t.TempDir(), "config.json"),
	)

	var outBuf bytes.Buffer
	cmd.Stdout = &outBuf

	if err := cmd.Run(); err != nil {
		t.Fatalf("expected exit 0, got error: %v", err)
	}

	var results []map[string]interface{}
	if jsonErr := json.Unmarshal(outBuf.Bytes(), &results); jsonErr != nil {
		t.Fatalf("stdout not valid JSON: %s", outBuf.String())
	}
	if results[0]["exit_code"].(float64) != 0 {
		t.Errorf("expected exit_code=0, got %v", results[0]["exit_code"])
	}
}

// Bug #10: Nonexistent --profile should give clear error.
func TestE2E_NonexistentProfile(t *testing.T) {
	if testing.Short() {
		t.Skip()
	}
	binary := buildBinary(t)
	srv := mockJira(t)

	// Use a fresh config file with no "nonexistent" profile.
	cfgDir := t.TempDir()
	cfgPath := filepath.Join(cfgDir, "config.json")

	cmd := exec.Command(binary, "raw", "GET", "/rest/api/3/myself", "--profile", "nonexistent")
	cmd.Env = append(os.Environ(),
		"JR_BASE_URL=",
		"JR_AUTH_USER=",
		"JR_AUTH_TOKEN=",
		"JR_CONFIG_PATH="+cfgPath,
	)
	_ = srv

	var errBuf bytes.Buffer
	cmd.Stderr = &errBuf

	err := cmd.Run()
	if err == nil {
		t.Fatal("expected error for nonexistent profile")
	}

	errStr := errBuf.String()
	if !strings.Contains(errStr, "nonexistent") {
		t.Errorf("error should mention profile name, got: %s", errStr)
	}
}

// Bug #16: GET with --body should warn on stderr.
func TestE2E_Raw_GetWithBody_Warning(t *testing.T) {
	if testing.Short() {
		t.Skip()
	}
	binary := buildBinary(t)
	srv := mockJira(t)

	stdout, stderr, exitCode := runJR(t, binary, srv, "raw", "GET", "/rest/api/3/myself", "--body", `{"test":true}`)

	if exitCode != 0 {
		t.Fatalf("expected exit 0 (body ignored), got %d", exitCode)
	}

	// Stderr should contain a warning.
	if !strings.Contains(stderr, "warning") {
		t.Errorf("expected warning about --body being ignored on stderr, got: %s", stderr)
	}

	// Stdout should still have valid JSON response.
	var result map[string]interface{}
	if err := json.Unmarshal([]byte(stdout), &result); err != nil {
		t.Fatalf("stdout not valid JSON: %s", stdout)
	}
}

// Bug #17: null batch input should return validation error.
func TestE2E_Batch_NullInput(t *testing.T) {
	if testing.Short() {
		t.Skip()
	}
	binary := buildBinary(t)
	srv := mockJira(t)

	cmd := exec.Command(binary, "batch")
	cmd.Stdin = strings.NewReader("null")
	cmd.Env = append(os.Environ(),
		"JR_BASE_URL="+srv.URL,
		"JR_AUTH_USER=test@example.com",
		"JR_AUTH_TOKEN=test-token",
		"JR_CONFIG_PATH="+filepath.Join(t.TempDir(), "config.json"),
	)

	var errBuf bytes.Buffer
	cmd.Stderr = &errBuf

	err := cmd.Run()
	if err == nil {
		t.Fatal("expected non-zero exit for null batch input")
	}

	exitErr, ok := err.(*exec.ExitError)
	if !ok {
		t.Fatalf("unexpected error type: %v", err)
	}
	if exitErr.ExitCode() != 4 {
		t.Errorf("expected exit code 4 (validation), got %d", exitErr.ExitCode())
	}

	if !strings.Contains(errBuf.String(), "null") {
		t.Errorf("error should mention null, got: %s", errBuf.String())
	}
}

// Bug #18: Non-HTTP errors should not include "status":0 in JSON.
func TestE2E_NonHTTPError_NoStatusZero(t *testing.T) {
	if testing.Short() {
		t.Skip()
	}
	binary := buildBinary(t)
	srv := mockJira(t)

	_, stderr, exitCode := runJR(t, binary, srv, "schema", "issue", "nonexistent-verb")

	if exitCode == 0 {
		t.Fatal("expected non-zero exit for nonexistent verb")
	}

	// Parse the error JSON and check no "status":0.
	var errObj map[string]interface{}
	if err := json.Unmarshal([]byte(stderr), &errObj); err != nil {
		t.Fatalf("stderr not valid JSON: %s", stderr)
	}
	if _, exists := errObj["status"]; exists {
		t.Errorf("non-HTTP error should not have 'status' field, got: %v", errObj["status"])
	}
}

// Bug #19: schema (no args) should return compact resource→verbs mapping.
func TestE2E_Schema_NoArgs_CompactOutput(t *testing.T) {
	if testing.Short() {
		t.Skip()
	}
	binary := buildBinary(t)
	srv := mockJira(t)

	stdout, _, exitCode := runJR(t, binary, srv, "schema")

	if exitCode != 0 {
		t.Fatalf("expected exit 0, got %d", exitCode)
	}

	// Should be a map of resource → verbs array (not just a list of resource names).
	var result map[string][]string
	if err := json.Unmarshal([]byte(stdout), &result); err != nil {
		t.Fatalf("stdout is not a map[string][]string: %v\nstdout: %s", err, stdout)
	}

	// Should have resources with verb arrays.
	if verbs, ok := result["issue"]; !ok || len(verbs) == 0 {
		t.Error("expected 'issue' resource with verbs in compact output")
	}
}

// Bug #19: schema --list should return just resource names (different from default).
func TestE2E_Schema_List_DifferentFromDefault(t *testing.T) {
	if testing.Short() {
		t.Skip()
	}
	binary := buildBinary(t)
	srv := mockJira(t)

	stdoutDefault, _, _ := runJR(t, binary, srv, "schema")
	stdoutList, _, exitCode := runJR(t, binary, srv, "schema", "--list")

	if exitCode != 0 {
		t.Fatalf("expected exit 0 for --list, got %d", exitCode)
	}

	// --list should return an array of strings.
	var listResult []string
	if err := json.Unmarshal([]byte(stdoutList), &listResult); err != nil {
		t.Fatalf("--list stdout is not a JSON string array: %v", err)
	}

	// Default and --list should differ.
	if stdoutDefault == stdoutList {
		t.Error("schema (default) and schema --list should return different formats")
	}
}

// Bug #20: Configure --delete should remove a profile.
func TestE2E_Configure_DeleteProfile(t *testing.T) {
	if testing.Short() {
		t.Skip()
	}
	binary := buildBinary(t)

	cfgDir := t.TempDir()
	cfgPath := filepath.Join(cfgDir, "config.json")

	// First, create a profile.
	cmd1 := exec.Command(binary, "configure", "--base-url", "https://example.com", "--token", "tok", "--profile", "to-delete")
	cmd1.Env = append(os.Environ(), "JR_CONFIG_PATH="+cfgPath)
	if err := cmd1.Run(); err != nil {
		t.Fatalf("failed to create profile: %v", err)
	}

	// Now delete it.
	cmd2 := exec.Command(binary, "configure", "--base-url", "x", "--token", "x", "--profile", "to-delete", "--delete")
	cmd2.Env = append(os.Environ(), "JR_CONFIG_PATH="+cfgPath)
	var outBuf bytes.Buffer
	cmd2.Stdout = &outBuf
	if err := cmd2.Run(); err != nil {
		t.Fatalf("failed to delete profile: %v", err)
	}

	var result map[string]string
	if err := json.Unmarshal(outBuf.Bytes(), &result); err != nil {
		t.Fatalf("stdout not valid JSON: %s", outBuf.String())
	}
	if result["status"] != "deleted" {
		t.Errorf("expected status=deleted, got %v", result["status"])
	}

	// Verify the profile is gone from config file.
	data, _ := os.ReadFile(cfgPath)
	if strings.Contains(string(data), "to-delete") {
		t.Error("profile 'to-delete' should be removed from config file")
	}
}

// Bug #22: Unknown subcommand should report "unknown command", not "unknown flag".
func TestE2E_UnknownSubcommand_ErrorMessage(t *testing.T) {
	if testing.Short() {
		t.Skip()
	}
	binary := buildBinary(t)
	srv := mockJira(t)

	_, stderr, exitCode := runJR(t, binary, srv, "issue", "foobar", "--issueIdOrKey", "TEST-1")

	if exitCode == 0 {
		t.Fatal("expected non-zero exit for unknown subcommand")
	}

	// Error should mention "unknown command", not "unknown flag".
	if strings.Contains(stderr, "unknown flag") {
		t.Errorf("error should NOT say 'unknown flag', got: %s", stderr)
	}
	if !strings.Contains(stderr, "unknown command") {
		t.Errorf("error should say 'unknown command', got: %s", stderr)
	}
}

// Bug #11: Paginated output should preserve literal & (not \u0026).
func TestE2E_Pagination_NoHTMLEscape(t *testing.T) {
	if testing.Short() {
		t.Skip()
	}
	binary := buildBinary(t)
	srv := mockJira(t)

	stdout, stderr, exitCode := runJR(t, binary, srv, "raw", "GET", "/rest/api/3/project/search-amp")

	if exitCode != 0 {
		t.Fatalf("expected exit 0, got %d; stderr=%s", exitCode, stderr)
	}

	if strings.Contains(stdout, `\u0026`) {
		t.Errorf("output should not contain HTML-escaped ampersand, got: %s", stdout)
	}
	if !strings.Contains(stdout, `&b=2`) {
		t.Errorf("output should contain literal '&', got: %s", stdout)
	}
}

// === Raw command tests ===

// TestE2E_Raw_GetWithJQ verifies that --jq filters a raw GET response.
func TestE2E_Raw_GetWithJQ(t *testing.T) {
	if testing.Short() {
		t.Skip()
	}
	binary := buildBinary(t)
	srv := mockJira(t)

	stdout, _, exitCode := runJR(t, binary, srv, "raw", "GET", "/rest/api/3/myself", "--jq", ".displayName")

	if exitCode != 0 {
		t.Fatalf("expected exit 0, got %d", exitCode)
	}
	if stdout != `"Agent"` {
		t.Errorf("expected %q, got %q", `"Agent"`, stdout)
	}
}

// TestE2E_Raw_GetWithFields verifies that --fields appends a fields query param.
func TestE2E_Raw_GetWithFields(t *testing.T) {
	if testing.Short() {
		t.Skip()
	}
	binary := buildBinary(t)
	srv := mockJira(t)

	stdout, _, exitCode := runJR(t, binary, srv, "raw", "GET", "/rest/api/3/issue/TEST-1", "--fields", "key,summary")

	if exitCode != 0 {
		t.Fatalf("expected exit 0, got %d", exitCode)
	}

	var result map[string]interface{}
	if err := json.Unmarshal([]byte(stdout), &result); err != nil {
		t.Fatalf("stdout is not valid JSON: %v\nstdout: %s", err, stdout)
	}
	if result["key"] != "TEST-1" {
		t.Errorf("expected key=TEST-1, got %v", result["key"])
	}
}

// TestE2E_Raw_PostWithBodyString verifies that a POST with --body echoes back the sent JSON.
func TestE2E_Raw_PostWithBodyString(t *testing.T) {
	if testing.Short() {
		t.Skip()
	}
	binary := buildBinary(t)
	srv := mockJira(t)

	stdout, _, exitCode := runJR(t, binary, srv, "raw", "POST", "/rest/api/3/echo", "--body", `{"msg":"hello"}`)

	if exitCode != 0 {
		t.Fatalf("expected exit 0, got %d", exitCode)
	}

	var result map[string]interface{}
	if err := json.Unmarshal([]byte(stdout), &result); err != nil {
		t.Fatalf("stdout is not valid JSON: %v\nstdout: %s", err, stdout)
	}
	if result["msg"] != "hello" {
		t.Errorf("expected msg=hello echoed back, got %v", result["msg"])
	}
}

// TestE2E_Raw_PostWithBodyFile verifies that @file syntax reads the body from disk.
func TestE2E_Raw_PostWithBodyFile(t *testing.T) {
	if testing.Short() {
		t.Skip()
	}
	binary := buildBinary(t)
	srv := mockJira(t)

	tmpFile := filepath.Join(t.TempDir(), "body.json")
	if err := os.WriteFile(tmpFile, []byte(`{"msg":"from-file"}`), 0600); err != nil {
		t.Fatalf("failed to write temp file: %v", err)
	}

	stdout, _, exitCode := runJR(t, binary, srv, "raw", "POST", "/rest/api/3/echo", "--body", "@"+tmpFile)

	if exitCode != 0 {
		t.Fatalf("expected exit 0, got %d", exitCode)
	}

	var result map[string]interface{}
	if err := json.Unmarshal([]byte(stdout), &result); err != nil {
		t.Fatalf("stdout is not valid JSON: %v\nstdout: %s", err, stdout)
	}
	if result["msg"] != "from-file" {
		t.Errorf("expected msg=from-file echoed back, got %v", result["msg"])
	}
}

// TestE2E_Raw_DeleteRequest verifies that DELETE returns {} and exits 0.
func TestE2E_Raw_DeleteRequest(t *testing.T) {
	if testing.Short() {
		t.Skip()
	}
	binary := buildBinary(t)
	srv := mockJira(t)

	stdout, stderr, exitCode := runJR(t, binary, srv, "raw", "DELETE", "/rest/api/3/issue/TEST-204")

	if exitCode != 0 {
		t.Fatalf("expected exit 0, got %d; stderr=%s", exitCode, stderr)
	}
	if stdout != "{}" {
		t.Errorf("expected '{}' for 204 DELETE response, got %q", stdout)
	}
}

// TestE2E_Raw_PatchWithoutBody verifies that PATCH without a body returns a validation error.
func TestE2E_Raw_PatchWithoutBody(t *testing.T) {
	if testing.Short() {
		t.Skip()
	}
	binary := buildBinary(t)
	srv := mockJira(t)

	_, stderr, exitCode := runJR(t, binary, srv, "raw", "PATCH", "/rest/api/3/issue/TEST-1")

	if exitCode != 4 {
		t.Fatalf("expected exit 4 (validation), got %d; stderr=%s", exitCode, stderr)
	}

	var errObj map[string]interface{}
	if err := json.Unmarshal([]byte(stderr), &errObj); err != nil {
		t.Fatalf("stderr not valid JSON: %s", stderr)
	}
	if errObj["error_type"] != "validation_error" {
		t.Errorf("expected error_type=validation_error, got %v", errObj["error_type"])
	}
}

// TestE2E_Raw_QueryParams verifies that multiple --query flags are forwarded to the server.
func TestE2E_Raw_QueryParams(t *testing.T) {
	if testing.Short() {
		t.Skip()
	}
	binary := buildBinary(t)
	srv := mockJira(t)

	_, _, exitCode := runJR(t, binary, srv, "raw", "GET", "/rest/api/3/myself", "--query", "foo=bar", "--query", "baz=qux")

	if exitCode != 0 {
		t.Fatalf("expected exit 0, got %d", exitCode)
	}
}

// TestE2E_Raw_MethodCaseInsensitive verifies that lowercase HTTP methods are accepted.
func TestE2E_Raw_MethodCaseInsensitive(t *testing.T) {
	if testing.Short() {
		t.Skip()
	}
	binary := buildBinary(t)
	srv := mockJira(t)

	_, _, exitCode := runJR(t, binary, srv, "raw", "get", "/rest/api/3/myself")

	if exitCode != 0 {
		t.Fatalf("expected exit 0 for lowercase method, got %d", exitCode)
	}
}

// TestE2E_Raw_NoArgs verifies that `raw` with no args exits non-zero.
func TestE2E_Raw_NoArgs(t *testing.T) {
	if testing.Short() {
		t.Skip()
	}
	binary := buildBinary(t)
	srv := mockJira(t)

	_, _, exitCode := runJR(t, binary, srv, "raw")

	if exitCode == 0 {
		t.Fatal("expected non-zero exit when raw has no args")
	}
}

// TestE2E_Raw_OneArg verifies that `raw GET` (method only, no path) exits non-zero.
func TestE2E_Raw_OneArg(t *testing.T) {
	if testing.Short() {
		t.Skip()
	}
	binary := buildBinary(t)
	srv := mockJira(t)

	_, _, exitCode := runJR(t, binary, srv, "raw", "GET")

	if exitCode == 0 {
		t.Fatal("expected non-zero exit when raw has method but no path")
	}
}

// TestE2E_Raw_DryRun verifies that --dry-run on raw POST shows method+url and exits 0.
func TestE2E_Raw_DryRun(t *testing.T) {
	if testing.Short() {
		t.Skip()
	}
	binary := buildBinary(t)
	srv := mockJira(t)

	stdout, _, exitCode := runJR(t, binary, srv, "--dry-run", "raw", "POST", "/rest/api/3/echo", "--body", "{}")

	if exitCode != 0 {
		t.Fatalf("expected exit 0, got %d", exitCode)
	}

	var result map[string]interface{}
	if err := json.Unmarshal([]byte(stdout), &result); err != nil {
		t.Fatalf("stdout is not valid JSON: %v\nstdout: %s", err, stdout)
	}
	if result["method"] != "POST" {
		t.Errorf("expected method=POST in dry-run output, got %v", result["method"])
	}
	if _, ok := result["url"]; !ok {
		t.Error("expected 'url' key in dry-run output")
	}
}

// TestE2E_Raw_Verbose verifies that --verbose logs request/response info to stderr.
func TestE2E_Raw_Verbose(t *testing.T) {
	if testing.Short() {
		t.Skip()
	}
	binary := buildBinary(t)
	srv := mockJira(t)

	_, stderr, exitCode := runJR(t, binary, srv, "--verbose", "raw", "GET", "/rest/api/3/myself")

	if exitCode != 0 {
		t.Fatalf("expected exit 0, got %d", exitCode)
	}
	if stderr == "" {
		t.Fatal("expected verbose output on stderr, got nothing")
	}
	// Verbose output should contain HTTP method or URL details.
	if !strings.Contains(stderr, "GET") && !strings.Contains(stderr, "myself") {
		t.Errorf("verbose stderr should mention GET or myself, got: %s", stderr)
	}
}

// === Configure command tests ===

// TestE2E_Configure_SavesProfile verifies that configure with valid args outputs status=saved.
func TestE2E_Configure_SavesProfile(t *testing.T) {
	if testing.Short() {
		t.Skip()
	}
	binary := buildBinary(t)

	cfgPath := filepath.Join(t.TempDir(), "config.json")
	cmd := exec.Command(binary, "configure", "--base-url", "https://example.atlassian.net", "--token", "mytoken", "--username", "user@example.com")
	cmd.Env = append(os.Environ(), "JR_CONFIG_PATH="+cfgPath)
	var outBuf, errBuf bytes.Buffer
	cmd.Stdout = &outBuf
	cmd.Stderr = &errBuf

	if err := cmd.Run(); err != nil {
		t.Fatalf("expected exit 0, got error: %v; stderr=%s", err, errBuf.String())
	}

	var result map[string]interface{}
	if err := json.Unmarshal(outBuf.Bytes(), &result); err != nil {
		t.Fatalf("stdout not valid JSON: %s", outBuf.String())
	}
	if result["status"] != "saved" {
		t.Errorf("expected status=saved, got %v", result["status"])
	}
}

// TestE2E_Configure_WhitespaceBaseURL verifies that a whitespace-only --base-url is rejected.
func TestE2E_Configure_WhitespaceBaseURL(t *testing.T) {
	if testing.Short() {
		t.Skip()
	}
	binary := buildBinary(t)

	cfgPath := filepath.Join(t.TempDir(), "config.json")
	cmd := exec.Command(binary, "configure", "--base-url", "   ", "--token", "mytoken")
	cmd.Env = append(os.Environ(), "JR_CONFIG_PATH="+cfgPath)
	var errBuf bytes.Buffer
	cmd.Stderr = &errBuf

	err := cmd.Run()
	if err == nil {
		t.Fatal("expected non-zero exit for whitespace --base-url")
	}
	exitErr, ok := err.(*exec.ExitError)
	if !ok {
		t.Fatalf("unexpected error type: %v", err)
	}
	if exitErr.ExitCode() != 4 {
		t.Errorf("expected exit code 4 (validation), got %d", exitErr.ExitCode())
	}

	var errObj map[string]interface{}
	if jsonErr := json.Unmarshal(errBuf.Bytes(), &errObj); jsonErr != nil {
		t.Fatalf("stderr not valid JSON: %s", errBuf.String())
	}
	if errObj["error_type"] != "validation_error" {
		t.Errorf("expected error_type=validation_error, got %v", errObj["error_type"])
	}
}

// TestE2E_Configure_WithTest verifies that --test against the mock server succeeds.
func TestE2E_Configure_WithTest(t *testing.T) {
	if testing.Short() {
		t.Skip()
	}
	binary := buildBinary(t)
	srv := mockJira(t)

	cfgPath := filepath.Join(t.TempDir(), "config.json")
	cmd := exec.Command(binary, "configure",
		"--base-url", srv.URL,
		"--token", "test-token",
		"--username", "test@example.com",
		"--test",
	)
	cmd.Env = append(os.Environ(), "JR_CONFIG_PATH="+cfgPath)
	var outBuf, errBuf bytes.Buffer
	cmd.Stdout = &outBuf
	cmd.Stderr = &errBuf

	if err := cmd.Run(); err != nil {
		t.Fatalf("expected exit 0 with --test against mock, got error: %v; stderr=%s", err, errBuf.String())
	}

	var result map[string]interface{}
	if err := json.Unmarshal(outBuf.Bytes(), &result); err != nil {
		t.Fatalf("stdout not valid JSON: %s", outBuf.String())
	}
	if result["status"] == nil {
		t.Error("expected status field in configure --test output")
	}
}

// TestE2E_Configure_WithTestFail verifies that --test with an unreachable URL returns an error.
func TestE2E_Configure_WithTestFail(t *testing.T) {
	if testing.Short() {
		t.Skip()
	}
	binary := buildBinary(t)

	cfgPath := filepath.Join(t.TempDir(), "config.json")
	cmd := exec.Command(binary, "configure",
		"--base-url", "http://127.0.0.1:19999",
		"--token", "test-token",
		"--test",
	)
	cmd.Env = append(os.Environ(), "JR_CONFIG_PATH="+cfgPath)
	var errBuf bytes.Buffer
	cmd.Stderr = &errBuf

	err := cmd.Run()
	if err == nil {
		t.Fatal("expected non-zero exit when --test points to unreachable server")
	}
	if errBuf.Len() == 0 {
		t.Error("expected error output on stderr for failed --test")
	}
}

// TestE2E_Configure_DeleteNonexistentProfile verifies that deleting a missing profile gives a not_found error.
func TestE2E_Configure_DeleteNonexistentProfile(t *testing.T) {
	if testing.Short() {
		t.Skip()
	}
	binary := buildBinary(t)

	cfgPath := filepath.Join(t.TempDir(), "config.json")
	cmd := exec.Command(binary, "configure", "--base-url", "x", "--token", "x", "--profile", "nope", "--delete")
	cmd.Env = append(os.Environ(), "JR_CONFIG_PATH="+cfgPath)
	var errBuf bytes.Buffer
	cmd.Stderr = &errBuf

	err := cmd.Run()
	if err == nil {
		t.Fatal("expected non-zero exit when deleting nonexistent profile")
	}
	exitErr, ok := err.(*exec.ExitError)
	if !ok {
		t.Fatalf("unexpected error type: %v", err)
	}
	if exitErr.ExitCode() != 3 {
		t.Errorf("expected exit code 3 (not_found), got %d", exitErr.ExitCode())
	}

	var errObj map[string]interface{}
	if jsonErr := json.Unmarshal(errBuf.Bytes(), &errObj); jsonErr != nil {
		t.Fatalf("stderr not valid JSON: %s", errBuf.String())
	}
	if errObj["error_type"] != "not_found" {
		t.Errorf("expected error_type=not_found, got %v", errObj["error_type"])
	}
}

// TestE2E_Configure_MultipleProfiles verifies that two profiles can be created and used independently.
func TestE2E_Configure_MultipleProfiles(t *testing.T) {
	if testing.Short() {
		t.Skip()
	}
	binary := buildBinary(t)
	srv := mockJira(t)

	cfgPath := filepath.Join(t.TempDir(), "config.json")

	// Create profile "alpha".
	cmd1 := exec.Command(binary, "configure", "--base-url", srv.URL, "--token", "tok-alpha", "--username", "alpha@example.com", "--profile", "alpha")
	cmd1.Env = append(os.Environ(), "JR_CONFIG_PATH="+cfgPath)
	if err := cmd1.Run(); err != nil {
		t.Fatalf("failed to create profile alpha: %v", err)
	}

	// Create profile "beta".
	cmd2 := exec.Command(binary, "configure", "--base-url", srv.URL, "--token", "tok-beta", "--username", "beta@example.com", "--profile", "beta")
	cmd2.Env = append(os.Environ(), "JR_CONFIG_PATH="+cfgPath)
	if err := cmd2.Run(); err != nil {
		t.Fatalf("failed to create profile beta: %v", err)
	}

	// Use profile "alpha".
	cmd3 := exec.Command(binary, "raw", "GET", "/rest/api/3/myself", "--profile", "alpha")
	cmd3.Env = append(os.Environ(), "JR_CONFIG_PATH="+cfgPath, "JR_BASE_URL=", "JR_AUTH_TOKEN=", "JR_AUTH_USER=")
	var outBuf3 bytes.Buffer
	cmd3.Stdout = &outBuf3
	if err := cmd3.Run(); err != nil {
		t.Fatalf("expected exit 0 using profile alpha, got: %v", err)
	}
	var r3 map[string]interface{}
	if err := json.Unmarshal(outBuf3.Bytes(), &r3); err != nil {
		t.Fatalf("stdout not valid JSON: %s", outBuf3.String())
	}
	if r3["displayName"] != "Agent" {
		t.Errorf("expected displayName=Agent with profile alpha, got %v", r3["displayName"])
	}

	// Use profile "beta".
	cmd4 := exec.Command(binary, "raw", "GET", "/rest/api/3/myself", "--profile", "beta")
	cmd4.Env = append(os.Environ(), "JR_CONFIG_PATH="+cfgPath, "JR_BASE_URL=", "JR_AUTH_TOKEN=", "JR_AUTH_USER=")
	var outBuf4 bytes.Buffer
	cmd4.Stdout = &outBuf4
	if err := cmd4.Run(); err != nil {
		t.Fatalf("expected exit 0 using profile beta, got: %v", err)
	}
	var r4 map[string]interface{}
	if err := json.Unmarshal(outBuf4.Bytes(), &r4); err != nil {
		t.Fatalf("stdout not valid JSON: %s", outBuf4.String())
	}
	if r4["displayName"] != "Agent" {
		t.Errorf("expected displayName=Agent with profile beta, got %v", r4["displayName"])
	}
}

// TestE2E_Configure_TestStandalone verifies that --test without --base-url/--token
// loads the saved profile and tests its connection.
func TestE2E_Configure_TestStandalone(t *testing.T) {
	if testing.Short() {
		t.Skip()
	}
	binary := buildBinary(t)
	srv := mockJira(t)

	cfgPath := filepath.Join(t.TempDir(), "config.json")

	// First, save a valid profile.
	cmd1 := exec.Command(binary, "configure", "--base-url", srv.URL, "--token", "tok", "--username", "user@test.com")
	cmd1.Env = append(os.Environ(), "JR_CONFIG_PATH="+cfgPath)
	if err := cmd1.Run(); err != nil {
		t.Fatalf("failed to create profile: %v", err)
	}

	// Now run --test standalone (no --base-url or --token).
	cmd2 := exec.Command(binary, "configure", "--test")
	cmd2.Env = append(os.Environ(), "JR_CONFIG_PATH="+cfgPath)
	var outBuf, errBuf bytes.Buffer
	cmd2.Stdout = &outBuf
	cmd2.Stderr = &errBuf

	if err := cmd2.Run(); err != nil {
		t.Fatalf("expected exit 0 for --test standalone, got error: %v; stderr=%s", err, errBuf.String())
	}

	var result map[string]interface{}
	if err := json.Unmarshal(outBuf.Bytes(), &result); err != nil {
		t.Fatalf("stdout not valid JSON: %s", outBuf.String())
	}
	if result["status"] != "ok" {
		t.Errorf("expected status=ok, got %v", result["status"])
	}
}

// TestE2E_Configure_TestStandaloneWithProfile verifies that --test --profile loads
// the named profile and tests its connection.
func TestE2E_Configure_TestStandaloneWithProfile(t *testing.T) {
	if testing.Short() {
		t.Skip()
	}
	binary := buildBinary(t)
	srv := mockJira(t)

	cfgPath := filepath.Join(t.TempDir(), "config.json")

	// Create a named profile.
	cmd1 := exec.Command(binary, "configure", "--base-url", srv.URL, "--token", "tok", "--username", "user@test.com", "--profile", "myprofile")
	cmd1.Env = append(os.Environ(), "JR_CONFIG_PATH="+cfgPath)
	if err := cmd1.Run(); err != nil {
		t.Fatalf("failed to create profile: %v", err)
	}

	// Test the named profile standalone.
	cmd2 := exec.Command(binary, "configure", "--test", "--profile", "myprofile")
	cmd2.Env = append(os.Environ(), "JR_CONFIG_PATH="+cfgPath)
	var outBuf, errBuf bytes.Buffer
	cmd2.Stdout = &outBuf
	cmd2.Stderr = &errBuf

	if err := cmd2.Run(); err != nil {
		t.Fatalf("expected exit 0 for --test --profile, got error: %v; stderr=%s", err, errBuf.String())
	}

	var result map[string]interface{}
	if err := json.Unmarshal(outBuf.Bytes(), &result); err != nil {
		t.Fatalf("stdout not valid JSON: %s", outBuf.String())
	}
	if result["status"] != "ok" {
		t.Errorf("expected status=ok, got %v", result["status"])
	}
	if result["profile"] != "myprofile" {
		t.Errorf("expected profile=myprofile, got %v", result["profile"])
	}
}

// TestE2E_Configure_TestStandaloneNonexistentProfile verifies that --test with a
// nonexistent profile returns not_found.
func TestE2E_Configure_TestStandaloneNonexistentProfile(t *testing.T) {
	if testing.Short() {
		t.Skip()
	}
	binary := buildBinary(t)

	cfgPath := filepath.Join(t.TempDir(), "config.json")

	// Create a profile so the config file exists.
	cmd1 := exec.Command(binary, "configure", "--base-url", "http://example.com", "--token", "tok")
	cmd1.Env = append(os.Environ(), "JR_CONFIG_PATH="+cfgPath)
	if err := cmd1.Run(); err != nil {
		t.Fatalf("failed to create profile: %v", err)
	}

	// Test with nonexistent profile.
	cmd2 := exec.Command(binary, "configure", "--test", "--profile", "nonexistent")
	cmd2.Env = append(os.Environ(), "JR_CONFIG_PATH="+cfgPath)
	var errBuf bytes.Buffer
	cmd2.Stderr = &errBuf

	err := cmd2.Run()
	if err == nil {
		t.Fatal("expected non-zero exit for --test with nonexistent profile")
	}
	exitErr, ok := err.(*exec.ExitError)
	if !ok {
		t.Fatalf("unexpected error type: %v", err)
	}
	if exitErr.ExitCode() != 3 {
		t.Errorf("expected exit code 3 (not_found), got %d", exitErr.ExitCode())
	}

	var errObj map[string]interface{}
	if jsonErr := json.Unmarshal(errBuf.Bytes(), &errObj); jsonErr != nil {
		t.Fatalf("stderr not valid JSON: %s", errBuf.String())
	}
	if errObj["error_type"] != "not_found" {
		t.Errorf("expected error_type=not_found, got %v", errObj["error_type"])
	}
}

// TestE2E_Configure_TestStandaloneConnectionFail verifies that --test standalone
// with a saved profile that has an unreachable URL returns a connection error.
func TestE2E_Configure_TestStandaloneConnectionFail(t *testing.T) {
	if testing.Short() {
		t.Skip()
	}
	binary := buildBinary(t)

	cfgPath := filepath.Join(t.TempDir(), "config.json")

	// Save a profile pointing at an unreachable URL.
	cmd1 := exec.Command(binary, "configure", "--base-url", "http://127.0.0.1:19999", "--token", "tok")
	cmd1.Env = append(os.Environ(), "JR_CONFIG_PATH="+cfgPath)
	if err := cmd1.Run(); err != nil {
		t.Fatalf("failed to create profile: %v", err)
	}

	// Test standalone — should fail with connection error.
	cmd2 := exec.Command(binary, "configure", "--test")
	cmd2.Env = append(os.Environ(), "JR_CONFIG_PATH="+cfgPath)
	var errBuf bytes.Buffer
	cmd2.Stderr = &errBuf

	err := cmd2.Run()
	if err == nil {
		t.Fatal("expected non-zero exit when testing unreachable saved profile")
	}
	if errBuf.Len() == 0 {
		t.Error("expected error output on stderr for failed connection test")
	}

	var errObj map[string]interface{}
	if jsonErr := json.Unmarshal(errBuf.Bytes(), &errObj); jsonErr != nil {
		t.Fatalf("stderr not valid JSON: %s", errBuf.String())
	}
	if errObj["error_type"] != "connection_error" {
		t.Errorf("expected error_type=connection_error, got %v", errObj["error_type"])
	}
}

// === Batch command tests ===

// TestE2E_Batch_EmptyArray verifies that an empty batch input yields an empty array output.
func TestE2E_Batch_EmptyArray(t *testing.T) {
	if testing.Short() {
		t.Skip()
	}
	binary := buildBinary(t)
	srv := mockJira(t)

	cmd := exec.Command(binary, "batch")
	cmd.Stdin = strings.NewReader("[]")
	cmd.Env = append(os.Environ(),
		"JR_BASE_URL="+srv.URL,
		"JR_AUTH_USER=test@example.com",
		"JR_AUTH_TOKEN=test-token",
		"JR_CONFIG_PATH="+filepath.Join(t.TempDir(), "config.json"),
	)
	var outBuf, errBuf bytes.Buffer
	cmd.Stdout = &outBuf
	cmd.Stderr = &errBuf

	if err := cmd.Run(); err != nil {
		t.Fatalf("expected exit 0, got error: %v; stderr=%s", err, errBuf.String())
	}

	var results []interface{}
	if err := json.Unmarshal(outBuf.Bytes(), &results); err != nil {
		t.Fatalf("stdout not valid JSON array: %s", outBuf.String())
	}
	if len(results) != 0 {
		t.Errorf("expected empty array, got %d items", len(results))
	}
}

// TestE2E_Batch_InvalidJSON verifies that malformed batch input returns a validation error.
func TestE2E_Batch_InvalidJSON(t *testing.T) {
	if testing.Short() {
		t.Skip()
	}
	binary := buildBinary(t)
	srv := mockJira(t)

	cmd := exec.Command(binary, "batch")
	cmd.Stdin = strings.NewReader("{not json}")
	cmd.Env = append(os.Environ(),
		"JR_BASE_URL="+srv.URL,
		"JR_AUTH_USER=test@example.com",
		"JR_AUTH_TOKEN=test-token",
		"JR_CONFIG_PATH="+filepath.Join(t.TempDir(), "config.json"),
	)
	var errBuf bytes.Buffer
	cmd.Stderr = &errBuf

	err := cmd.Run()
	if err == nil {
		t.Fatal("expected non-zero exit for invalid JSON batch input")
	}
	exitErr, ok := err.(*exec.ExitError)
	if !ok {
		t.Fatalf("unexpected error type: %v", err)
	}
	if exitErr.ExitCode() != 4 {
		t.Errorf("expected exit code 4 (validation), got %d", exitErr.ExitCode())
	}
}

// TestE2E_Batch_UnknownCommand verifies that an unknown command inside batch exits non-zero.
func TestE2E_Batch_UnknownCommand(t *testing.T) {
	if testing.Short() {
		t.Skip()
	}
	binary := buildBinary(t)
	srv := mockJira(t)

	cmd := exec.Command(binary, "batch")
	cmd.Stdin = strings.NewReader(`[{"command":"fake cmd","args":{}}]`)
	cmd.Env = append(os.Environ(),
		"JR_BASE_URL="+srv.URL,
		"JR_AUTH_USER=test@example.com",
		"JR_AUTH_TOKEN=test-token",
		"JR_CONFIG_PATH="+filepath.Join(t.TempDir(), "config.json"),
	)
	var outBuf, errBuf bytes.Buffer
	cmd.Stdout = &outBuf
	cmd.Stderr = &errBuf

	err := cmd.Run()
	if err == nil {
		t.Fatal("expected non-zero exit for batch with unknown command")
	}

	// Stdout should be a results array with an error entry.
	var results []map[string]interface{}
	if jsonErr := json.Unmarshal(outBuf.Bytes(), &results); jsonErr != nil {
		t.Fatalf("stdout not valid JSON array: %s; stderr=%s", outBuf.String(), errBuf.String())
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if results[0]["exit_code"].(float64) == 0 {
		t.Error("expected non-zero exit_code in result for unknown command")
	}
}

// TestE2E_Batch_MixedSuccessFailure verifies that mixed results return both and exit with highest code.
func TestE2E_Batch_MixedSuccessFailure(t *testing.T) {
	if testing.Short() {
		t.Skip()
	}
	binary := buildBinary(t)
	srv := mockJira(t)

	input := `[{"command":"issue get","args":{"issueIdOrKey":"TEST-1"}},{"command":"issue get","args":{"issueIdOrKey":"NOPE"}}]`
	cmd := exec.Command(binary, "batch")
	cmd.Stdin = strings.NewReader(input)
	cmd.Env = append(os.Environ(),
		"JR_BASE_URL="+srv.URL,
		"JR_AUTH_USER=test@example.com",
		"JR_AUTH_TOKEN=test-token",
		"JR_CONFIG_PATH="+filepath.Join(t.TempDir(), "config.json"),
	)
	var outBuf, errBuf bytes.Buffer
	cmd.Stdout = &outBuf
	cmd.Stderr = &errBuf

	err := cmd.Run()
	// Expect non-zero due to the failing op.
	if err == nil {
		t.Fatal("expected non-zero exit for batch with mixed success/failure")
	}

	var results []map[string]interface{}
	if jsonErr := json.Unmarshal(outBuf.Bytes(), &results); jsonErr != nil {
		t.Fatalf("stdout not valid JSON array: %s", outBuf.String())
	}
	if len(results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(results))
	}

	// First op should succeed.
	if results[0]["exit_code"].(float64) != 0 {
		t.Errorf("expected first op exit_code=0, got %v", results[0]["exit_code"])
	}
	// Second op should fail.
	if results[1]["exit_code"].(float64) == 0 {
		t.Errorf("expected second op to have non-zero exit_code")
	}
}

// TestE2E_Batch_WithJQ verifies that a jq filter inside a batch operation filters the result.
func TestE2E_Batch_WithJQ(t *testing.T) {
	if testing.Short() {
		t.Skip()
	}
	binary := buildBinary(t)
	srv := mockJira(t)

	input := `[{"command":"issue get","args":{"issueIdOrKey":"TEST-1"},"jq":".key"}]`
	cmd := exec.Command(binary, "batch")
	cmd.Stdin = strings.NewReader(input)
	cmd.Env = append(os.Environ(),
		"JR_BASE_URL="+srv.URL,
		"JR_AUTH_USER=test@example.com",
		"JR_AUTH_TOKEN=test-token",
		"JR_CONFIG_PATH="+filepath.Join(t.TempDir(), "config.json"),
	)
	var outBuf bytes.Buffer
	cmd.Stdout = &outBuf

	if err := cmd.Run(); err != nil {
		t.Fatalf("expected exit 0, got error: %v", err)
	}

	var results []map[string]interface{}
	if err := json.Unmarshal(outBuf.Bytes(), &results); err != nil {
		t.Fatalf("stdout not valid JSON array: %s", outBuf.String())
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	// The jq-filtered data field should contain "TEST-1".
	data := fmt.Sprintf("%v", results[0]["data"])
	if !strings.Contains(data, "TEST-1") {
		t.Errorf("expected jq result to contain TEST-1, got %v", data)
	}
}

// TestE2E_Batch_InputFile verifies that --input reads batch commands from a file.
func TestE2E_Batch_InputFile(t *testing.T) {
	if testing.Short() {
		t.Skip()
	}
	binary := buildBinary(t)
	srv := mockJira(t)

	tmpFile := filepath.Join(t.TempDir(), "batch.json")
	batchContent := `[{"command":"issue get","args":{"issueIdOrKey":"TEST-1"},"jq":".key"}]`
	if err := os.WriteFile(tmpFile, []byte(batchContent), 0600); err != nil {
		t.Fatalf("failed to write batch file: %v", err)
	}

	stdout, stderr, exitCode := runJR(t, binary, srv, "batch", "--input", tmpFile)

	if exitCode != 0 {
		t.Fatalf("expected exit 0, got %d; stderr=%s", exitCode, stderr)
	}

	var results []map[string]interface{}
	if err := json.Unmarshal([]byte(stdout), &results); err != nil {
		t.Fatalf("stdout not valid JSON array: %s", stdout)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if results[0]["exit_code"].(float64) != 0 {
		t.Errorf("expected exit_code=0, got %v", results[0]["exit_code"])
	}
}

// TestE2E_Batch_InputFileNotFound verifies that a missing --input file returns a validation error.
func TestE2E_Batch_InputFileNotFound(t *testing.T) {
	if testing.Short() {
		t.Skip()
	}
	binary := buildBinary(t)
	srv := mockJira(t)

	_, stderr, exitCode := runJR(t, binary, srv, "batch", "--input", "/nonexistent/path/batch.json")

	if exitCode == 0 {
		t.Fatal("expected non-zero exit for nonexistent input file")
	}
	if stderr == "" {
		t.Error("expected error output on stderr for missing input file")
	}
}

// === Schema command tests ===

// TestE2E_Schema_ResourceVerbs verifies that `schema issue` returns an array of operations.
func TestE2E_Schema_ResourceVerbs(t *testing.T) {
	if testing.Short() {
		t.Skip()
	}
	binary := buildBinary(t)
	srv := mockJira(t)

	stdout, _, exitCode := runJR(t, binary, srv, "schema", "issue")

	if exitCode != 0 {
		t.Fatalf("expected exit 0, got %d", exitCode)
	}

	var result []map[string]interface{}
	if err := json.Unmarshal([]byte(stdout), &result); err != nil {
		t.Fatalf("stdout is not a JSON array: %v\nstdout: %s", err, stdout)
	}
	if len(result) == 0 {
		t.Fatal("expected at least one operation for 'issue'")
	}
	// Each entry should have a verb or method.
	first := result[0]
	if first["verb"] == nil && first["method"] == nil && first["path"] == nil {
		t.Errorf("expected verb/method/path fields in operation, got: %v", first)
	}
}

// TestE2E_Schema_ResourceVerbFull verifies that `schema issue get` returns a single operation with flags.
func TestE2E_Schema_ResourceVerbFull(t *testing.T) {
	if testing.Short() {
		t.Skip()
	}
	binary := buildBinary(t)
	srv := mockJira(t)

	stdout, _, exitCode := runJR(t, binary, srv, "schema", "issue", "get")

	if exitCode != 0 {
		t.Fatalf("expected exit 0, got %d", exitCode)
	}

	var result map[string]interface{}
	if err := json.Unmarshal([]byte(stdout), &result); err != nil {
		t.Fatalf("stdout is not valid JSON object: %v\nstdout: %s", err, stdout)
	}
	// Should include path or method info.
	if result["path"] == nil && result["method"] == nil {
		t.Errorf("expected path or method in schema issue get output, got: %v", result)
	}
}

// TestE2E_Schema_NonexistentResource verifies that an unknown resource exits 3.
func TestE2E_Schema_NonexistentResource(t *testing.T) {
	if testing.Short() {
		t.Skip()
	}
	binary := buildBinary(t)
	srv := mockJira(t)

	_, stderr, exitCode := runJR(t, binary, srv, "schema", "nonexistent")

	if exitCode != 3 {
		t.Fatalf("expected exit 3 (not_found), got %d; stderr=%s", exitCode, stderr)
	}

	var errObj map[string]interface{}
	if err := json.Unmarshal([]byte(stderr), &errObj); err != nil {
		t.Fatalf("stderr not valid JSON: %s", stderr)
	}
	if errObj["error_type"] != "not_found" {
		t.Errorf("expected error_type=not_found, got %v", errObj["error_type"])
	}
}

// TestE2E_Schema_NonexistentVerb verifies that an unknown verb for a valid resource exits 3.
func TestE2E_Schema_NonexistentVerb(t *testing.T) {
	if testing.Short() {
		t.Skip()
	}
	binary := buildBinary(t)
	srv := mockJira(t)

	_, stderr, exitCode := runJR(t, binary, srv, "schema", "issue", "nonexistent-verb")

	if exitCode != 3 {
		t.Fatalf("expected exit 3 (not_found), got %d; stderr=%s", exitCode, stderr)
	}

	var errObj map[string]interface{}
	if err := json.Unmarshal([]byte(stderr), &errObj); err != nil {
		t.Fatalf("stderr not valid JSON: %s", stderr)
	}
	if errObj["error_type"] != "not_found" {
		t.Errorf("expected error_type=not_found, got %v", errObj["error_type"])
	}
}

// TestE2E_Schema_Compact verifies that `schema --compact` produces the same format as `schema`.
func TestE2E_Schema_Compact(t *testing.T) {
	if testing.Short() {
		t.Skip()
	}
	binary := buildBinary(t)
	srv := mockJira(t)

	stdoutCompact, _, exitCode := runJR(t, binary, srv, "schema", "--compact")

	if exitCode != 0 {
		t.Fatalf("expected exit 0, got %d", exitCode)
	}

	// --compact should produce a map of resource → verbs, same format as default.
	var result map[string][]string
	if err := json.Unmarshal([]byte(stdoutCompact), &result); err != nil {
		t.Fatalf("stdout is not a map[string][]string: %v\nstdout: %s", err, stdoutCompact)
	}
	if len(result) == 0 {
		t.Error("expected non-empty compact schema output")
	}
}

// === Workflow command tests ===

// lastJSONLine returns the last non-empty line of s (for commands that output
// multiple JSON objects, e.g. workflow commands that get {} from 204 then a result).
func lastJSONLine(s string) string {
	lines := strings.Split(strings.TrimSpace(s), "\n")
	for i := len(lines) - 1; i >= 0; i-- {
		l := strings.TrimSpace(lines[i])
		if l != "" {
			return l
		}
	}
	return s
}

// TestE2E_Workflow_Transition verifies that transitioning an issue returns status=transitioned.
func TestE2E_Workflow_Transition(t *testing.T) {
	if testing.Short() {
		t.Skip()
	}
	binary := buildBinary(t)
	srv := mockJira(t)

	stdout, stderr, exitCode := runJR(t, binary, srv, "workflow", "transition", "--issue", "TEST-1", "--to", "Done")

	if exitCode != 0 {
		t.Fatalf("expected exit 0, got %d; stderr=%s", exitCode, stderr)
	}

	var result map[string]interface{}
	if err := json.Unmarshal([]byte(lastJSONLine(stdout)), &result); err != nil {
		t.Fatalf("stdout not valid JSON: %v\nstdout: %s", err, stdout)
	}
	if result["status"] != "transitioned" {
		t.Errorf("expected status=transitioned, got %v", result["status"])
	}
}

// TestE2E_Workflow_AssignMe verifies that assigning to "me" returns status=assigned.
func TestE2E_Workflow_AssignMe(t *testing.T) {
	if testing.Short() {
		t.Skip()
	}
	binary := buildBinary(t)
	srv := mockJira(t)

	stdout, stderr, exitCode := runJR(t, binary, srv, "workflow", "assign", "--issue", "TEST-1", "--to", "me")

	if exitCode != 0 {
		t.Fatalf("expected exit 0, got %d; stderr=%s", exitCode, stderr)
	}

	var result map[string]interface{}
	if err := json.Unmarshal([]byte(lastJSONLine(stdout)), &result); err != nil {
		t.Fatalf("stdout not valid JSON: %v\nstdout: %s", err, stdout)
	}
	if result["status"] != "assigned" {
		t.Errorf("expected status=assigned, got %v", result["status"])
	}
}

// TestE2E_Workflow_Unassign verifies that assigning to "none" returns status=unassigned.
func TestE2E_Workflow_Unassign(t *testing.T) {
	if testing.Short() {
		t.Skip()
	}
	binary := buildBinary(t)
	srv := mockJira(t)

	stdout, stderr, exitCode := runJR(t, binary, srv, "workflow", "assign", "--issue", "TEST-1", "--to", "none")

	if exitCode != 0 {
		t.Fatalf("expected exit 0, got %d; stderr=%s", exitCode, stderr)
	}

	var result map[string]interface{}
	if err := json.Unmarshal([]byte(lastJSONLine(stdout)), &result); err != nil {
		t.Fatalf("stdout not valid JSON: %v\nstdout: %s", err, stdout)
	}
	if result["status"] != "unassigned" {
		t.Errorf("expected status=unassigned, got %v", result["status"])
	}
}

// TestE2E_Workflow_AssignByName verifies that assigning by email returns status=assigned.
func TestE2E_Workflow_AssignByName(t *testing.T) {
	if testing.Short() {
		t.Skip()
	}
	binary := buildBinary(t)
	srv := mockJira(t)

	stdout, stderr, exitCode := runJR(t, binary, srv, "workflow", "assign", "--issue", "TEST-1", "--to", "test@example.com")

	if exitCode != 0 {
		t.Fatalf("expected exit 0, got %d; stderr=%s", exitCode, stderr)
	}

	var result map[string]interface{}
	if err := json.Unmarshal([]byte(lastJSONLine(stdout)), &result); err != nil {
		t.Fatalf("stdout not valid JSON: %v\nstdout: %s", err, stdout)
	}
	if result["status"] != "assigned" {
		t.Errorf("expected status=assigned, got %v", result["status"])
	}
}

// === Error handling tests ===

// TestE2E_RateLimit_ExitCode5 verifies that a 429 response exits with code 5.
func TestE2E_RateLimit_ExitCode5(t *testing.T) {
	if testing.Short() {
		t.Skip()
	}
	binary := buildBinary(t)
	srv := mockJira(t)

	_, stderr, exitCode := runJR(t, binary, srv, "raw", "GET", "/rest/api/3/ratelimit")

	if exitCode != 5 {
		t.Fatalf("expected exit 5 (rate_limited), got %d; stderr=%s", exitCode, stderr)
	}

	var errObj map[string]interface{}
	if err := json.Unmarshal([]byte(stderr), &errObj); err != nil {
		t.Fatalf("stderr not valid JSON: %s", stderr)
	}
	if errObj["error_type"] != "rate_limited" {
		t.Errorf("expected error_type=rate_limited, got %v", errObj["error_type"])
	}
}

// TestE2E_ServerError_ExitCode7 verifies that a 500 response exits with code 7.
func TestE2E_ServerError_ExitCode7(t *testing.T) {
	if testing.Short() {
		t.Skip()
	}
	binary := buildBinary(t)
	srv := mockJira(t)

	_, stderr, exitCode := runJR(t, binary, srv, "raw", "GET", "/rest/api/3/servererr")

	if exitCode != 7 {
		t.Fatalf("expected exit 7 (server_error), got %d; stderr=%s", exitCode, stderr)
	}

	var errObj map[string]interface{}
	if err := json.Unmarshal([]byte(stderr), &errObj); err != nil {
		t.Fatalf("stderr not valid JSON: %s", stderr)
	}
	if errObj["error_type"] != "server_error" {
		t.Errorf("expected error_type=server_error, got %v", errObj["error_type"])
	}
}

// TestE2E_NotFound_ExitCode3 verifies that a 404 response exits with code 3.
func TestE2E_NotFound_ExitCode3(t *testing.T) {
	if testing.Short() {
		t.Skip()
	}
	binary := buildBinary(t)
	srv := mockJira(t)

	_, stderr, exitCode := runJR(t, binary, srv, "issue", "get", "--issueIdOrKey", "NOPE")

	if exitCode != 3 {
		t.Fatalf("expected exit 3 (not_found), got %d; stderr=%s", exitCode, stderr)
	}

	var errObj map[string]interface{}
	if err := json.Unmarshal([]byte(stderr), &errObj); err != nil {
		t.Fatalf("stderr not valid JSON: %s", stderr)
	}
	if errObj["error_type"] != "not_found" {
		t.Errorf("expected error_type=not_found, got %v", errObj["error_type"])
	}
}

// === Global flag tests ===

// TestE2E_PrettyPrint verifies that --pretty produces indented JSON output.
func TestE2E_PrettyPrint(t *testing.T) {
	if testing.Short() {
		t.Skip()
	}
	binary := buildBinary(t)
	srv := mockJira(t)

	stdout, _, exitCode := runJR(t, binary, srv, "--pretty", "raw", "GET", "/rest/api/3/myself")

	if exitCode != 0 {
		t.Fatalf("expected exit 0, got %d", exitCode)
	}

	// Pretty-printed JSON should contain newlines and indentation.
	if !strings.Contains(stdout, "\n") {
		t.Errorf("expected newlines in pretty-printed output, got: %s", stdout)
	}
	if !strings.Contains(stdout, "  ") {
		t.Errorf("expected indentation in pretty-printed output, got: %s", stdout)
	}

	// Must still be valid JSON.
	var result map[string]interface{}
	if err := json.Unmarshal([]byte(stdout), &result); err != nil {
		t.Fatalf("pretty-printed stdout is not valid JSON: %v\nstdout: %s", err, stdout)
	}
}

// TestE2E_FieldsFlag verifies that --fields appends the fields parameter to the request.
func TestE2E_FieldsFlag(t *testing.T) {
	if testing.Short() {
		t.Skip()
	}
	binary := buildBinary(t)
	srv := mockJira(t)

	stdout, _, exitCode := runJR(t, binary, srv, "--fields", "key,summary", "issue", "get", "--issueIdOrKey", "TEST-1")

	if exitCode != 0 {
		t.Fatalf("expected exit 0, got %d", exitCode)
	}

	var result map[string]interface{}
	if err := json.Unmarshal([]byte(stdout), &result); err != nil {
		t.Fatalf("stdout not valid JSON: %v\nstdout: %s", err, stdout)
	}
	if result["key"] != "TEST-1" {
		t.Errorf("expected key=TEST-1, got %v", result["key"])
	}
}

// TestE2E_CacheFlag verifies that --cache causes identical GET calls to return consistent results.
func TestE2E_CacheFlag(t *testing.T) {
	if testing.Short() {
		t.Skip()
	}
	binary := buildBinary(t)
	srv := mockJira(t)

	// First call with cache.
	stdout1, _, exitCode1 := runJR(t, binary, srv, "--cache", "1m", "raw", "GET", "/rest/api/3/myself")
	if exitCode1 != 0 {
		t.Fatalf("first call: expected exit 0, got %d", exitCode1)
	}

	// Second call with cache — should return the same data.
	stdout2, _, exitCode2 := runJR(t, binary, srv, "--cache", "1m", "raw", "GET", "/rest/api/3/myself")
	if exitCode2 != 0 {
		t.Fatalf("second call: expected exit 0, got %d", exitCode2)
	}

	// Both calls should produce valid JSON with the same display name.
	var r1, r2 map[string]interface{}
	if err := json.Unmarshal([]byte(stdout1), &r1); err != nil {
		t.Fatalf("first call stdout not valid JSON: %s", stdout1)
	}
	if err := json.Unmarshal([]byte(stdout2), &r2); err != nil {
		t.Fatalf("second call stdout not valid JSON: %s", stdout2)
	}
	if r1["displayName"] != r2["displayName"] {
		t.Errorf("expected same displayName across cached calls, got %v vs %v", r1["displayName"], r2["displayName"])
	}
}

// === Version command tests ===

// TestE2E_Version_AsSubcommand verifies that `version` subcommand outputs JSON with a version key.
func TestE2E_Version_AsSubcommand(t *testing.T) {
	if testing.Short() {
		t.Skip()
	}
	binary := buildBinary(t)
	srv := mockJira(t)

	stdout, _, exitCode := runJR(t, binary, srv, "version")

	if exitCode != 0 {
		t.Fatalf("expected exit 0, got %d", exitCode)
	}

	var result map[string]interface{}
	if err := json.Unmarshal([]byte(stdout), &result); err != nil {
		t.Fatalf("stdout not valid JSON: %v\nstdout: %s", err, stdout)
	}
	if _, ok := result["version"]; !ok {
		t.Error("expected 'version' key in JSON output")
	}
}

// TestE2E_Version_Flag verifies that `--version` flag outputs JSON with a version key.
func TestE2E_Version_Flag(t *testing.T) {
	if testing.Short() {
		t.Skip()
	}
	binary := buildBinary(t)
	srv := mockJira(t)

	stdout, _, exitCode := runJR(t, binary, srv, "--version")

	if exitCode != 0 {
		t.Fatalf("expected exit 0, got %d", exitCode)
	}

	var result map[string]interface{}
	if err := json.Unmarshal([]byte(stdout), &result); err != nil {
		t.Fatalf("stdout not valid JSON: %v\nstdout: %s", err, stdout)
	}
	if _, ok := result["version"]; !ok {
		t.Error("expected 'version' key in JSON output from --version flag")
	}
}

// === Miscellaneous tests ===

// TestE2E_UnknownTopLevelCommand verifies that an unknown top-level command hints about schema.
func TestE2E_UnknownTopLevelCommand(t *testing.T) {
	if testing.Short() {
		t.Skip()
	}
	binary := buildBinary(t)
	srv := mockJira(t)

	_, stderr, exitCode := runJR(t, binary, srv, "foobar")

	if exitCode == 0 {
		t.Fatal("expected non-zero exit for unknown top-level command")
	}

	// Error output should be valid JSON.
	var errObj map[string]interface{}
	if err := json.Unmarshal([]byte(stderr), &errObj); err != nil {
		t.Fatalf("stderr not valid JSON: %s", stderr)
	}
	// Should include a hint about schema or how to discover commands.
	hasHint := errObj["hint"] != nil || strings.Contains(stderr, "schema") || strings.Contains(stderr, "unknown command")
	if !hasHint {
		t.Errorf("expected hint about schema or unknown command in error, got: %s", stderr)
	}
}

// TestE2E_IssueNoSubcommand verifies that `jr issue` with no subcommand returns an error.
func TestE2E_IssueNoSubcommand(t *testing.T) {
	if testing.Short() {
		t.Skip()
	}
	binary := buildBinary(t)
	srv := mockJira(t)

	_, stderr, exitCode := runJR(t, binary, srv, "issue")

	if exitCode == 0 {
		t.Fatal("expected non-zero exit when 'issue' is given without a subcommand")
	}

	if stderr == "" {
		t.Error("expected error output on stderr when no subcommand provided")
	}
}

// TestE2E_Workflow_AssignMe_MissingAccountId verifies that assign --to me
// returns a clear error when /myself does not contain accountId (BUG-7 regression).
func TestE2E_Workflow_AssignMe_MissingAccountId(t *testing.T) {
	if testing.Short() {
		t.Skip()
	}
	binary := buildBinary(t)

	// Custom mock that returns /myself WITHOUT accountId.
	mux := http.NewServeMux()
	mux.HandleFunc("/rest/api/3/myself", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"displayName":"Agent","emailAddress":"agent@test.com"}`))
	})
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{}`))
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	_, stderr, exitCode := runJR(t, binary, srv, "workflow", "assign", "--issue", "TEST-1", "--to", "me")

	if exitCode == 0 {
		t.Fatal("expected non-zero exit when /myself has no accountId")
	}
	if !strings.Contains(stderr, "could not determine your account ID") {
		t.Errorf("expected error about missing account ID, got: %s", stderr)
	}
}

// TestE2E_Workflow_AssignMe_InvalidJSON verifies that assign --to me
// returns a clear error when /myself returns invalid JSON (BUG-7 regression).
func TestE2E_Workflow_AssignMe_InvalidJSON(t *testing.T) {
	if testing.Short() {
		t.Skip()
	}
	binary := buildBinary(t)

	// Custom mock that returns invalid JSON for /myself.
	mux := http.NewServeMux()
	mux.HandleFunc("/rest/api/3/myself", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`not json`))
	})
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{}`))
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	_, stderr, exitCode := runJR(t, binary, srv, "workflow", "assign", "--issue", "TEST-1", "--to", "me")

	if exitCode == 0 {
		t.Fatal("expected non-zero exit when /myself returns invalid JSON")
	}
	if !strings.Contains(stderr, "failed to parse") {
		t.Errorf("expected parse error in stderr, got: %s", stderr)
	}
}

// TestE2E_Schema_NotFound_NoOsExit verifies that schema with unknown resource
// returns proper exit code without os.Exit (BUG-9 regression).
func TestE2E_Schema_NotFound_NoOsExit(t *testing.T) {
	if testing.Short() {
		t.Skip()
	}
	binary := buildBinary(t)
	srv := mockJira(t)

	_, stderr, exitCode := runJR(t, binary, srv, "schema", "nonexistent_resource")

	if exitCode != 3 {
		t.Fatalf("expected exit code 3 (not_found), got %d", exitCode)
	}
	if !strings.Contains(stderr, "not_found") {
		t.Errorf("expected not_found error, got: %s", stderr)
	}
}

// TestE2E_Schema_OpNotFound_NoOsExit verifies that schema with unknown verb
// returns proper exit code without os.Exit (BUG-9 regression).
func TestE2E_Schema_OpNotFound_NoOsExit(t *testing.T) {
	if testing.Short() {
		t.Skip()
	}
	binary := buildBinary(t)
	srv := mockJira(t)

	_, stderr, exitCode := runJR(t, binary, srv, "schema", "issue", "nonexistent_verb")

	if exitCode != 3 {
		t.Fatalf("expected exit code 3 (not_found), got %d", exitCode)
	}
	if !strings.Contains(stderr, "not_found") {
		t.Errorf("expected not_found error, got: %s", stderr)
	}
}

// TestE2E_Raw_ErrorReturnsExitCode verifies that raw command errors
// return proper exit codes via errAlreadyWritten instead of os.Exit (BUG-6 regression).
func TestE2E_Raw_ErrorReturnsExitCode(t *testing.T) {
	if testing.Short() {
		t.Skip()
	}
	binary := buildBinary(t)
	srv := mockJira(t)

	_, stderr, exitCode := runJR(t, binary, srv, "raw", "GET", "/rest/api/3/nonexistent")

	if exitCode != 3 {
		t.Fatalf("expected exit code 3 (not_found), got %d", exitCode)
	}
	if !strings.Contains(stderr, "not_found") {
		t.Errorf("expected not_found error, got: %s", stderr)
	}
}

// === Bug #41-43 regression tests ===

// TestE2E_InvalidAuthType_RuntimeError verifies that --auth-type with an invalid
// value produces a config_error instead of silently falling back to basic auth.
func TestE2E_InvalidAuthType_RuntimeError(t *testing.T) {
	if testing.Short() {
		t.Skip()
	}
	binary := buildBinary(t)
	srv := mockJira(t)

	_, stderr, exitCode := runJR(t, binary, srv, "--auth-type", "invalidtype", "raw", "GET", "/rest/api/3/myself")

	if exitCode == 0 {
		t.Fatal("expected non-zero exit for invalid auth type, got 0")
	}
	if !strings.Contains(stderr, "invalid auth type") {
		t.Errorf("expected 'invalid auth type' in stderr, got: %s", stderr)
	}
}

// TestE2E_InvalidAuthType_EnvVar verifies that JR_AUTH_TYPE with an invalid
// value produces an error instead of silently falling back to basic auth.
func TestE2E_InvalidAuthType_EnvVar(t *testing.T) {
	if testing.Short() {
		t.Skip()
	}
	binary := buildBinary(t)
	srv := mockJira(t)

	cmd := exec.Command(binary, "raw", "GET", "/rest/api/3/myself")
	cmd.Env = append(os.Environ(),
		"JR_BASE_URL="+srv.URL,
		"JR_AUTH_TYPE=invalidtype",
		"JR_AUTH_TOKEN=test-token",
		"JR_CONFIG_PATH="+filepath.Join(t.TempDir(), "config.json"),
	)
	var outBuf, errBuf bytes.Buffer
	cmd.Stdout = &outBuf
	cmd.Stderr = &errBuf
	err := cmd.Run()

	if err == nil {
		t.Fatal("expected error for invalid JR_AUTH_TYPE env var")
	}
	stderr := errBuf.String()
	if !strings.Contains(stderr, "invalid auth type") {
		t.Errorf("expected 'invalid auth type' in stderr, got: %s", stderr)
	}
}

// TestE2E_ConfigureTestExplicitProfileDefault verifies that `jr configure --test --profile default`
// tests the profile literally named "default", not the default_profile.
func TestE2E_ConfigureTestExplicitProfileDefault(t *testing.T) {
	if testing.Short() {
		t.Skip()
	}
	binary := buildBinary(t)

	// Set up two mock servers: "default" profile returns 200, "staging" returns 401.
	defaultSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"accountId":"abc123"}`))
	}))
	defer defaultSrv.Close()

	stagingSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"errorMessages":["unauthorized"]}`))
	}))
	defer stagingSrv.Close()

	// Create config: default_profile=staging, but profile "default" points to the 200 server.
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.json")
	cfgData := fmt.Sprintf(`{
		"profiles": {
			"default": {"base_url": %q, "auth": {"type": "basic", "username": "u", "token": "t"}},
			"staging": {"base_url": %q, "auth": {"type": "basic", "username": "u", "token": "t"}}
		},
		"default_profile": "staging"
	}`, defaultSrv.URL, stagingSrv.URL)
	if err := os.WriteFile(cfgPath, []byte(cfgData), 0o600); err != nil {
		t.Fatal(err)
	}

	// Run: jr configure --test --profile default
	cmd := exec.Command(binary, "configure", "--test", "--profile", "default")
	cmd.Env = append(os.Environ(), "JR_CONFIG_PATH="+cfgPath)
	var outBuf, errBuf bytes.Buffer
	cmd.Stdout = &outBuf
	cmd.Stderr = &errBuf
	err := cmd.Run()

	stdout := strings.TrimSpace(outBuf.String())
	stderr := strings.TrimSpace(errBuf.String())

	// Should succeed (testing "default" profile → 200 OK server).
	// Before the fix, it would test "staging" profile → 401 → error.
	if err != nil {
		t.Fatalf("expected success testing 'default' profile, got error: exit=%v stderr=%s", err, stderr)
	}
	if !strings.Contains(stdout, `"ok"`) {
		t.Errorf("expected ok status in stdout, got: %s", stdout)
	}
}

// TestE2E_ConfigureTestImplicitDefault_UsesDefaultProfile verifies that
// `jr configure --test` (no explicit --profile) uses default_profile.
func TestE2E_ConfigureTestImplicitDefault_UsesDefaultProfile(t *testing.T) {
	if testing.Short() {
		t.Skip()
	}
	binary := buildBinary(t)

	// Set up: "staging" profile returns 200, "default" profile returns 401.
	stagingSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"accountId":"abc123"}`))
	}))
	defer stagingSrv.Close()

	defaultSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"errorMessages":["unauthorized"]}`))
	}))
	defer defaultSrv.Close()

	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.json")
	cfgData := fmt.Sprintf(`{
		"profiles": {
			"default": {"base_url": %q, "auth": {"type": "basic", "username": "u", "token": "t"}},
			"staging": {"base_url": %q, "auth": {"type": "basic", "username": "u", "token": "t"}}
		},
		"default_profile": "staging"
	}`, defaultSrv.URL, stagingSrv.URL)
	if err := os.WriteFile(cfgPath, []byte(cfgData), 0o600); err != nil {
		t.Fatal(err)
	}

	// Run: jr configure --test (no --profile → should use default_profile=staging)
	cmd := exec.Command(binary, "configure", "--test")
	cmd.Env = append(os.Environ(), "JR_CONFIG_PATH="+cfgPath)
	var outBuf, errBuf bytes.Buffer
	cmd.Stdout = &outBuf
	cmd.Stderr = &errBuf
	err := cmd.Run()

	stdout := strings.TrimSpace(outBuf.String())
	stderr := strings.TrimSpace(errBuf.String())

	// Should succeed (testing "staging" profile → 200 OK server).
	if err != nil {
		t.Fatalf("expected success testing default_profile='staging', got error: exit=%v stderr=%s", err, stderr)
	}
	if !strings.Contains(stdout, `"ok"`) {
		t.Errorf("expected ok status in stdout, got: %s", stdout)
	}
}

// TestE2E_Configure_ShortProfileFlag verifies that -p works with configure.
func TestE2E_Configure_ShortProfileFlag(t *testing.T) {
	if testing.Short() {
		t.Skip()
	}
	binary := buildBinary(t)

	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.json")

	// Run: jr configure -p staging --base-url ... --token ...
	cmd := exec.Command(binary, "configure", "-p", "staging", "--base-url", "https://staging.example.com", "--token", "test-token")
	cmd.Env = append(os.Environ(), "JR_CONFIG_PATH="+cfgPath)
	var outBuf, errBuf bytes.Buffer
	cmd.Stdout = &outBuf
	cmd.Stderr = &errBuf
	err := cmd.Run()

	stdout := strings.TrimSpace(outBuf.String())
	stderr := strings.TrimSpace(errBuf.String())

	if err != nil {
		t.Fatalf("expected success, got error: exit=%v stderr=%s", err, stderr)
	}

	// Verify the profile was saved as "staging" (not "default").
	if !strings.Contains(stdout, `"staging"`) {
		t.Errorf("expected profile name 'staging' in output, got: %s", stdout)
	}

	// Also verify the config file has the staging profile.
	cfgData, err := os.ReadFile(cfgPath)
	if err != nil {
		t.Fatal(err)
	}
	var cfg map[string]interface{}
	if err := json.Unmarshal(cfgData, &cfg); err != nil {
		t.Fatal(err)
	}
	profiles := cfg["profiles"].(map[string]interface{})
	if _, ok := profiles["staging"]; !ok {
		t.Errorf("expected 'staging' profile in config, got profiles: %v", profiles)
	}
}

// --- Bug #56: raw --body @ (empty filename) should produce clear error ---

func TestE2E_Raw_BodyAtEmptyFilename(t *testing.T) {
	if testing.Short() {
		t.Skip()
	}
	binary := buildBinary(t)
	srv := mockJira(t)

	_, stderr, exitCode := runJR(t, binary, srv, "raw", "POST", "/rest/api/3/echo", "--body", "@")

	if exitCode != 4 {
		t.Fatalf("expected exit code 4 (validation), got %d; stderr=%s", exitCode, stderr)
	}

	var errObj map[string]interface{}
	if err := json.Unmarshal([]byte(stderr), &errObj); err != nil {
		t.Fatalf("stderr not valid JSON: %s", stderr)
	}
	if errObj["error_type"] != "validation_error" {
		t.Errorf("expected error_type=validation_error, got %v", errObj["error_type"])
	}
	msg, _ := errObj["message"].(string)
	if !strings.Contains(msg, "filename") {
		t.Errorf("error message should mention 'filename', got: %s", msg)
	}
}

// --- Bug #53: OAuth2 without required fields should error at resolve time ---

func TestE2E_OAuth2MissingFieldsError(t *testing.T) {
	if testing.Short() {
		t.Skip()
	}
	binary := buildBinary(t)
	srv := mockJira(t)

	// Run a command with oauth2 auth type but no oauth2 fields configured.
	cmd := exec.Command(binary, "issue", "get", "--issueIdOrKey", "TEST-1")
	cmd.Env = append(os.Environ(),
		"JR_BASE_URL="+srv.URL,
		"JR_AUTH_TYPE=oauth2",
		"JR_CONFIG_PATH="+filepath.Join(t.TempDir(), "config.json"),
	)

	var outBuf, errBuf bytes.Buffer
	cmd.Stdout = &outBuf
	cmd.Stderr = &errBuf

	err := cmd.Run()
	if err == nil {
		t.Fatal("expected non-zero exit for oauth2 without required fields")
	}

	var errObj map[string]interface{}
	if jsonErr := json.Unmarshal(errBuf.Bytes(), &errObj); jsonErr != nil {
		t.Fatalf("stderr not valid JSON: %s", errBuf.String())
	}
	if errObj["error_type"] != "config_error" {
		t.Errorf("expected error_type=config_error, got %v", errObj["error_type"])
	}
	msg, _ := errObj["message"].(string)
	if !strings.Contains(msg, "oauth2") || !strings.Contains(msg, "client_id") {
		t.Errorf("error should mention oauth2 required fields, got: %s", msg)
	}
}
