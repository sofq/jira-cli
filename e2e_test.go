package main

import (
	"bytes"
	"encoding/json"
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
		_, _ = w.Write([]byte(`{"displayName":"Agent","emailAddress":"agent@test.com","active":true}`))
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
