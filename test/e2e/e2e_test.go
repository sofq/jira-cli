package e2e_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"strings"
	"testing"
)

// buildBinary builds the jr binary for testing.
func buildBinary(t *testing.T) string {
	t.Helper()
	binary := t.TempDir() + "/jr"
	cmd := exec.Command("go", "build", "-o", binary, "../../.")
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		t.Fatalf("failed to build jr: %v", err)
	}
	return binary
}

// runJR executes jr with the given args against a test server.
func runJR(t *testing.T, binary, serverURL string, args ...string) (stdout, stderr string, exitCode int) {
	t.Helper()
	fullArgs := append([]string{"--base-url", serverURL, "--auth-type", "bearer", "--auth-token", "test"}, args...)
	cmd := exec.Command(binary, fullArgs...)
	var outBuf, errBuf strings.Builder
	cmd.Stdout = &outBuf
	cmd.Stderr = &errBuf
	err := cmd.Run()
	exitCode = 0
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			exitCode = exitErr.ExitCode()
		} else {
			t.Fatalf("failed to run jr: %v", err)
		}
	}
	return outBuf.String(), errBuf.String(), exitCode
}

func TestE2E_IssueGet(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/rest/api/3/issue/TEST-1" {
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprintln(w, `{"key":"TEST-1","fields":{"summary":"Test issue"}}`)
			return
		}
		w.WriteHeader(404)
	}))
	defer ts.Close()

	binary := buildBinary(t)
	stdout, _, exitCode := runJR(t, binary, ts.URL, "issue", "get", "--issueIdOrKey", "TEST-1")
	if exitCode != 0 {
		t.Fatalf("expected exit 0, got %d", exitCode)
	}
	var result map[string]any
	if err := json.Unmarshal([]byte(stdout), &result); err != nil {
		t.Fatalf("stdout not JSON: %s", stdout)
	}
	if result["key"] != "TEST-1" {
		t.Errorf("expected key=TEST-1, got %v", result["key"])
	}
}

func TestE2E_IssueGetWithJQ(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintln(w, `{"key":"TEST-1","fields":{"summary":"Test"}}`)
	}))
	defer ts.Close()

	binary := buildBinary(t)
	stdout, _, exitCode := runJR(t, binary, ts.URL, "issue", "get", "--issueIdOrKey", "TEST-1", "--jq", ".key")
	if exitCode != 0 {
		t.Fatalf("expected exit 0, got %d", exitCode)
	}
	if strings.TrimSpace(stdout) != `"TEST-1"` {
		t.Errorf("expected \"TEST-1\", got %s", stdout)
	}
}

func TestE2E_NotFound(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(404)
		fmt.Fprintln(w, `{"errorMessages":["Issue not found"]}`)
	}))
	defer ts.Close()

	binary := buildBinary(t)
	_, stderr, exitCode := runJR(t, binary, ts.URL, "issue", "get", "--issueIdOrKey", "NOPE-1")
	if exitCode != 3 {
		t.Fatalf("expected exit 3, got %d", exitCode)
	}
	var errObj map[string]any
	if err := json.Unmarshal([]byte(stderr), &errObj); err != nil {
		t.Fatalf("stderr not JSON: %s", stderr)
	}
	if errObj["error_type"] != "not_found" {
		t.Errorf("expected error_type=not_found, got %v", errObj["error_type"])
	}
}

func TestE2E_DryRun(t *testing.T) {
	binary := buildBinary(t)
	stdout, _, exitCode := runJR(t, binary, "https://fake.atlassian.net", "--dry-run", "issue", "get", "--issueIdOrKey", "X-1")
	if exitCode != 0 {
		t.Fatalf("expected exit 0, got %d", exitCode)
	}
	var obj map[string]string
	if err := json.Unmarshal([]byte(stdout), &obj); err != nil {
		t.Fatalf("stdout not JSON: %s", stdout)
	}
	if obj["method"] != "GET" {
		t.Errorf("expected method=GET, got %s", obj["method"])
	}
}

func TestE2E_SchemaList(t *testing.T) {
	binary := buildBinary(t)
	// schema doesn't need auth
	cmd := exec.Command(binary, "schema", "--list")
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("schema --list failed: %v", err)
	}
	var resources []string
	if err := json.Unmarshal(out, &resources); err != nil {
		t.Fatalf("stdout not JSON array: %s", out)
	}
	if len(resources) < 10 {
		t.Errorf("expected many resources, got %d", len(resources))
	}
}

func TestE2E_SchemaCompact(t *testing.T) {
	binary := buildBinary(t)
	cmd := exec.Command(binary, "schema", "--compact")
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("schema --compact failed: %v", err)
	}
	var compact map[string][]string
	if err := json.Unmarshal(out, &compact); err != nil {
		t.Fatalf("stdout not JSON object: %s", out)
	}
	if len(compact) < 10 {
		t.Errorf("expected many resources, got %d", len(compact))
	}
	// Check that issue resource has verbs
	if verbs, ok := compact["issue"]; !ok || len(verbs) == 0 {
		t.Error("expected issue resource with verbs")
	}
}

func TestE2E_Version(t *testing.T) {
	binary := buildBinary(t)
	cmd := exec.Command(binary, "version")
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("version failed: %v", err)
	}
	var obj map[string]string
	if err := json.Unmarshal(out, &obj); err != nil {
		t.Fatalf("not JSON: %s", out)
	}
	if _, ok := obj["version"]; !ok {
		t.Error("missing version field")
	}
}

func TestE2E_FieldsParam(t *testing.T) {
	var gotFields string
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotFields = r.URL.Query().Get("fields")
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintln(w, `{"key":"TEST-1"}`)
	}))
	defer ts.Close()

	binary := buildBinary(t)
	_, _, exitCode := runJR(t, binary, ts.URL, "--fields", "key,summary", "issue", "get", "--issueIdOrKey", "TEST-1")
	if exitCode != 0 {
		t.Fatalf("expected exit 0, got %d", exitCode)
	}
	if gotFields != "key,summary" {
		t.Errorf("expected fields=key,summary, got %q", gotFields)
	}
}
