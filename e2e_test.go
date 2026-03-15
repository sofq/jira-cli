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
		_, _ = w.Write([]byte(`{"displayName":"Agent"}`))
	})

	mux.HandleFunc("/rest/api/3/issue/TEST-1", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"key":"TEST-1","fields":{"summary":"Test issue"}}`))
	})

	mux.HandleFunc("/rest/api/3/issue/NOPE", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"errorMessages":["Issue not found"]}`))
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
