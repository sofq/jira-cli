// Package integration runs tests against a real Jira Cloud instance.
// Skipped unless JR_BASE_URL and JR_AUTH_TOKEN are set.
package integration_test

import (
	"encoding/json"
	"os"
	"os/exec"
	"strings"
	"testing"
)

func skipUnlessConfigured(t *testing.T) {
	t.Helper()
	if os.Getenv("JR_BASE_URL") == "" || os.Getenv("JR_AUTH_TOKEN") == "" {
		t.Skip("JR_BASE_URL and JR_AUTH_TOKEN not set — skipping integration test")
	}
}

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

func runJR(t *testing.T, binary string, args ...string) (stdout, stderr string, exitCode int) {
	t.Helper()
	cmd := exec.Command(binary, args...)
	var outBuf, errBuf strings.Builder
	cmd.Stdout = &outBuf
	cmd.Stderr = &errBuf
	// Auth comes from env vars (JR_BASE_URL, JR_AUTH_TOKEN, etc.)
	cmd.Env = os.Environ()
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

// Test: GET /rest/api/3/myself — verifies auth works.
func TestIntegration_Myself(t *testing.T) {
	skipUnlessConfigured(t)
	binary := buildBinary(t)

	stdout, stderr, exitCode := runJR(t, binary, "myself", "get-current-user", "--jq", "{displayName: .displayName, accountId: .accountId}")
	if exitCode != 0 {
		t.Fatalf("expected exit 0, got %d; stderr=%s", exitCode, stderr)
	}

	var result map[string]any
	if err := json.Unmarshal([]byte(stdout), &result); err != nil {
		t.Fatalf("stdout not JSON: %s", stdout)
	}
	if result["accountId"] == nil || result["accountId"] == "" {
		t.Error("expected accountId in response")
	}
	if result["displayName"] == nil || result["displayName"] == "" {
		t.Error("expected displayName in response")
	}
}

// Test: project search — lists projects.
func TestIntegration_ProjectSearch(t *testing.T) {
	skipUnlessConfigured(t)
	binary := buildBinary(t)

	stdout, stderr, exitCode := runJR(t, binary, "project", "search", "--jq", "[.values[] | {key, name}]")
	if exitCode != 0 {
		t.Fatalf("expected exit 0, got %d; stderr=%s", exitCode, stderr)
	}

	var projects []map[string]any
	if err := json.Unmarshal([]byte(stdout), &projects); err != nil {
		t.Fatalf("stdout not JSON array: %s", stdout)
	}
	if len(projects) == 0 {
		t.Skip("no projects found — cannot test further")
	}
	if projects[0]["key"] == nil {
		t.Error("expected key field in project")
	}
}

// Test: search issues with new JQL endpoint.
func TestIntegration_SearchJQL(t *testing.T) {
	skipUnlessConfigured(t)
	binary := buildBinary(t)

	stdout, stderr, exitCode := runJR(t, binary,
		"search", "search-and-reconsile-issues-using-jql",
		"--jql", "created >= -30d ORDER BY created DESC",
		"--maxResults", "5",
		"--fields", "key,summary",
		"--jq", "[.issues[] | {key, summary: .fields.summary}]")
	if exitCode != 0 {
		t.Fatalf("expected exit 0, got %d; stderr=%s", exitCode, stderr)
	}

	var issues []map[string]any
	if err := json.Unmarshal([]byte(stdout), &issues); err != nil {
		t.Fatalf("stdout not JSON array: %s", stdout)
	}
	// May be empty if instance has no issues — that's OK.
}

// Test: issue get with --fields and --jq.
func TestIntegration_IssueGet(t *testing.T) {
	skipUnlessConfigured(t)
	binary := buildBinary(t)

	// First find an issue key.
	stdout, _, exitCode := runJR(t, binary,
		"search", "search-and-reconsile-issues-using-jql",
		"--jql", "created >= -30d ORDER BY created DESC",
		"--maxResults", "1",
		"--fields", "key",
		"--jq", "[.issues[].key]")
	if exitCode != 0 {
		t.Skip("no issues accessible — skipping issue get test")
	}

	var keys []string
	if err := json.Unmarshal([]byte(stdout), &keys); err != nil || len(keys) == 0 {
		t.Skip("no issues found — skipping issue get test")
	}

	issueKey := keys[0]
	stdout, stderr, exitCode := runJR(t, binary,
		"issue", "get",
		"--issueIdOrKey", issueKey,
		"--fields", "key,summary,status",
		"--jq", "{key: .key, summary: .fields.summary, status: .fields.status.name}")
	if exitCode != 0 {
		t.Fatalf("expected exit 0, got %d; stderr=%s", exitCode, stderr)
	}

	var result map[string]any
	if err := json.Unmarshal([]byte(stdout), &result); err != nil {
		t.Fatalf("stdout not JSON: %s", stdout)
	}
	if result["key"] != issueKey {
		t.Errorf("expected key=%s, got %v", issueKey, result["key"])
	}
}

// Test: not-found returns exit code 3.
func TestIntegration_NotFound(t *testing.T) {
	skipUnlessConfigured(t)
	binary := buildBinary(t)

	_, stderr, exitCode := runJR(t, binary, "issue", "get", "--issueIdOrKey", "NONEXISTENT-99999")
	if exitCode != 3 {
		t.Fatalf("expected exit 3 (not found), got %d; stderr=%s", exitCode, stderr)
	}

	var errObj map[string]any
	if err := json.Unmarshal([]byte(stderr), &errObj); err != nil {
		t.Fatalf("stderr not JSON: %s", stderr)
	}
	if errObj["error_type"] != "not_found" {
		t.Errorf("expected error_type=not_found, got %v", errObj["error_type"])
	}
}

// Test: --dry-run doesn't hit Jira.
func TestIntegration_DryRun(t *testing.T) {
	skipUnlessConfigured(t)
	binary := buildBinary(t)

	stdout, _, exitCode := runJR(t, binary, "--dry-run", "issue", "get", "--issueIdOrKey", "TEST-1")
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

// Test: batch operations against real Jira.
func TestIntegration_Batch(t *testing.T) {
	skipUnlessConfigured(t)
	binary := buildBinary(t)

	input := `[{"command":"myself get-current-user","args":{},"jq":".displayName"},{"command":"project search","args":{},"jq":"[.values[].key]"}]`
	cmd := exec.Command(binary, "batch", "--input", "/dev/stdin")
	cmd.Stdin = strings.NewReader(input)
	cmd.Env = os.Environ()
	var outBuf, errBuf strings.Builder
	cmd.Stdout = &outBuf
	cmd.Stderr = &errBuf
	if err := cmd.Run(); err != nil {
		t.Fatalf("batch failed: %v; stderr=%s", err, errBuf.String())
	}

	var results []map[string]any
	if err := json.Unmarshal([]byte(outBuf.String()), &results); err != nil {
		t.Fatalf("stdout not JSON: %s", outBuf.String())
	}
	if len(results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(results))
	}
	for i, r := range results {
		if r["exit_code"] != float64(0) {
			t.Errorf("batch op %d failed: exit_code=%v, error=%v", i, r["exit_code"], r["error"])
		}
	}
}

// Test: verbose output is structured JSON on stderr.
func TestIntegration_VerboseJSON(t *testing.T) {
	skipUnlessConfigured(t)
	binary := buildBinary(t)

	_, stderr, exitCode := runJR(t, binary, "--verbose", "myself", "get-current-user", "--jq", ".accountId")
	if exitCode != 0 {
		t.Fatalf("expected exit 0, got %d", exitCode)
	}

	lines := strings.Split(strings.TrimSpace(stderr), "\n")
	if len(lines) < 2 {
		t.Fatalf("expected at least 2 stderr lines, got %d: %s", len(lines), stderr)
	}

	var reqLog map[string]any
	if err := json.Unmarshal([]byte(lines[0]), &reqLog); err != nil {
		t.Fatalf("request log not JSON: %s", lines[0])
	}
	if reqLog["type"] != "request" {
		t.Errorf("expected type=request, got %v", reqLog["type"])
	}
}
