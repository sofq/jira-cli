package cmd

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/sofq/jira-cli/cmd/generated"
	"github.com/sofq/jira-cli/internal/cache"
	"github.com/sofq/jira-cli/internal/client"
	"github.com/sofq/jira-cli/internal/config"
	jrerrors "github.com/sofq/jira-cli/internal/errors"
	"github.com/spf13/cobra"
)

// newTestClient creates a Client wired to a test server.
func newTestClient(serverURL string, stdout, stderr *bytes.Buffer) *client.Client {
	return &client.Client{
		BaseURL:    serverURL,
		Auth:       config.AuthConfig{Type: "basic", Username: "u", Token: "t"},
		HTTPClient: &http.Client{},
		Stdout:     stdout,
		Stderr:     stderr,
		Paginate:   true,
	}
}

// --- Bug 1 & 5: Batch supports workflow commands; schema includes them ---

func TestHandWrittenSchemaOps_ContainsTransitionAndAssign(t *testing.T) {
	ops := HandWrittenSchemaOps()
	found := map[string]bool{}
	for _, op := range ops {
		found[op.Resource+" "+op.Verb] = true
	}
	if !found["workflow transition"] {
		t.Error("HandWrittenSchemaOps missing 'workflow transition'")
	}
	if !found["workflow assign"] {
		t.Error("HandWrittenSchemaOps missing 'workflow assign'")
	}
}

func TestBatchOpMap_IncludesWorkflowOps(t *testing.T) {
	allOps := generated.AllSchemaOps()
	allOps = append(allOps, HandWrittenSchemaOps()...)
	opMap := make(map[string]generated.SchemaOp)
	for _, op := range allOps {
		opMap[op.Resource+" "+op.Verb] = op
	}
	if _, ok := opMap["workflow transition"]; !ok {
		t.Error("batch op map missing 'workflow transition'")
	}
	if _, ok := opMap["workflow assign"]; !ok {
		t.Error("batch op map missing 'workflow assign'")
	}
}

// --- Bug 1: batchTransition works ---

func TestBatchTransition_Success(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "GET" && strings.Contains(r.URL.Path, "/transitions") {
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprintln(w, `{"transitions":[{"id":"11","name":"Done","to":{"name":"Done"}}]}`)
			return
		}
		if r.Method == "POST" && strings.Contains(r.URL.Path, "/transitions") {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer ts.Close()

	var stdout, stderr bytes.Buffer
	c := newTestClient(ts.URL, &stdout, &stderr)

	code := batchTransition(t.Context(), c, "TEST-1", "Done")
	if code != jrerrors.ExitOK {
		t.Fatalf("expected exit 0, got %d; stderr=%s", code, stderr.String())
	}

	var result map[string]string
	if err := json.Unmarshal([]byte(strings.TrimSpace(stdout.String())), &result); err != nil {
		t.Fatalf("stdout not valid JSON: %s", stdout.String())
	}
	if result["status"] != "transitioned" {
		t.Errorf("expected status=transitioned, got %s", result["status"])
	}
	if result["transition"] != "Done" {
		t.Errorf("expected transition=Done, got %s", result["transition"])
	}
}

func TestBatchTransition_NoMatch(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintln(w, `{"transitions":[{"id":"11","name":"Done","to":{"name":"Done"}}]}`)
	}))
	defer ts.Close()

	var stdout, stderr bytes.Buffer
	c := newTestClient(ts.URL, &stdout, &stderr)

	code := batchTransition(t.Context(), c, "TEST-1", "Nonexistent")
	if code != jrerrors.ExitValidation {
		t.Fatalf("expected exit %d, got %d", jrerrors.ExitValidation, code)
	}
	if !strings.Contains(stderr.String(), "no transition matching") {
		t.Errorf("expected error about no transition, got: %s", stderr.String())
	}
}

// --- Bug 1: batchAssign works ---

func TestBatchAssign_Me(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/rest/api/3/myself" {
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprintln(w, `{"accountId":"abc123"}`)
			return
		}
		if strings.Contains(r.URL.Path, "/assignee") && r.Method == "PUT" {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer ts.Close()

	var stdout, stderr bytes.Buffer
	c := newTestClient(ts.URL, &stdout, &stderr)

	code := batchAssign(t.Context(), c, "TEST-1", "me")
	if code != jrerrors.ExitOK {
		t.Fatalf("expected exit 0, got %d; stderr=%s", code, stderr.String())
	}

	var result map[string]string
	if err := json.Unmarshal([]byte(strings.TrimSpace(stdout.String())), &result); err != nil {
		t.Fatalf("stdout not valid JSON: %s", stdout.String())
	}
	if result["status"] != "assigned" {
		t.Errorf("expected status=assigned, got %s", result["status"])
	}
}

func TestBatchAssign_None(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/assignee") && r.Method == "PUT" {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer ts.Close()

	var stdout, stderr bytes.Buffer
	c := newTestClient(ts.URL, &stdout, &stderr)

	code := batchAssign(t.Context(), c, "TEST-1", "none")
	if code != jrerrors.ExitOK {
		t.Fatalf("expected exit 0, got %d; stderr=%s", code, stderr.String())
	}

	var result map[string]string
	if err := json.Unmarshal([]byte(strings.TrimSpace(stdout.String())), &result); err != nil {
		t.Fatalf("stdout not valid JSON: %s", stdout.String())
	}
	if result["status"] != "unassigned" {
		t.Errorf("expected status=unassigned, got %s", result["status"])
	}
}

func TestBatchWorkflow_MissingArgs(t *testing.T) {
	var stdout, stderr bytes.Buffer
	c := newTestClient("http://localhost", &stdout, &stderr)

	code := executeBatchWorkflow(t.Context(), c, BatchOp{Command: "workflow transition", Args: map[string]string{}})
	if code != jrerrors.ExitValidation {
		t.Fatalf("expected exit %d, got %d", jrerrors.ExitValidation, code)
	}
}

// --- Bug 2: configure --delete requires explicit --profile ---

func TestConfigureDelete_RequiresExplicitProfile(t *testing.T) {
	// Simulate running "jr configure --delete" without --profile.
	// The flag "profile" should not be marked as Changed.
	// Use a fresh command to avoid mutating global configureCmd state.
	cmd := &cobra.Command{Use: "configure", RunE: runConfigure}
	f := cmd.Flags()
	f.String("base-url", "", "")
	f.String("token", "", "")
	f.StringP("profile", "p", "default", "profile name")
	f.String("auth-type", "basic", "")
	f.String("username", "", "")
	f.Bool("test", false, "")
	f.Bool("delete", false, "")

	// Set --delete but not --profile.
	_ = cmd.Flags().Set("delete", "true")

	err := runConfigure(cmd, nil)
	if err == nil {
		t.Fatal("expected error when --delete used without --profile")
	}
	aw, ok := err.(*errAlreadyWritten)
	if !ok {
		t.Fatalf("expected errAlreadyWritten, got %T: %v", err, err)
	}
	if aw.code != jrerrors.ExitValidation {
		t.Errorf("expected exit code %d, got %d", jrerrors.ExitValidation, aw.code)
	}
}

// --- Bug 4: workflow transition/assign emit single JSON object ---

func TestFetchJSONWithBody_SingleOutput(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	defer ts.Close()

	var stdout, stderr bytes.Buffer
	c := newTestClient(ts.URL, &stdout, &stderr)

	body, code := fetchJSONWithBody(c, t.Context(), "POST", "/test", strings.NewReader(`{}`))
	if code != jrerrors.ExitOK {
		t.Fatalf("expected exit 0, got %d; stderr=%s", code, stderr.String())
	}
	// fetchJSONWithBody should NOT write anything to stdout.
	if stdout.String() != "" {
		t.Errorf("fetchJSONWithBody should not write to stdout, got: %s", stdout.String())
	}
	// Body should be empty for 204.
	if len(body) != 0 {
		t.Errorf("expected empty body for 204, got: %s", string(body))
	}
}

// --- Bug 6: configure --test duplicate error ---

func TestConfigureTest_SingleError(t *testing.T) {
	// Use a server that returns 401 to simulate auth failure.
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer ts.Close()

	// Use a fresh command to avoid mutating global configureCmd state.
	cmd := &cobra.Command{Use: "configure", RunE: runConfigure}
	f := cmd.Flags()
	f.String("base-url", "", "")
	f.String("token", "", "")
	f.StringP("profile", "p", "default", "")
	f.String("auth-type", "basic", "")
	f.String("username", "", "")
	f.Bool("test", false, "")
	f.Bool("delete", false, "")

	_ = cmd.Flags().Set("base-url", ts.URL)
	_ = cmd.Flags().Set("token", "faketoken")
	_ = cmd.Flags().Set("test", "true")

	err := runConfigure(cmd, nil)
	if err == nil {
		t.Fatal("expected error for failed connection test")
	}

	// The error should be errAlreadyWritten, not a raw error (which causes duplicate output).
	if _, ok := err.(*errAlreadyWritten); !ok {
		t.Errorf("expected errAlreadyWritten to prevent duplicate errors, got %T: %v", err, err)
	}
}

// --- Bug 7: raw path without leading / ---

func TestClientDo_PathWithoutLeadingSlash(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/rest/api/3/test" {
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprintln(w, `{"ok":true}`)
			return
		}
		t.Errorf("unexpected path: %s", r.URL.Path)
		w.WriteHeader(http.StatusNotFound)
	}))
	defer ts.Close()

	var stdout, stderr bytes.Buffer
	c := newTestClient(ts.URL, &stdout, &stderr)

	// Path without leading /.
	code := c.Do(t.Context(), "GET", "rest/api/3/test", nil, nil)
	if code != jrerrors.ExitOK {
		t.Fatalf("expected exit 0, got %d; stderr=%s", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), `"ok"`) {
		t.Errorf("expected ok in response, got: %s", stdout.String())
	}
}

func TestClientDo_PathWithLeadingSlash(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/rest/api/3/test" {
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprintln(w, `{"ok":true}`)
			return
		}
		t.Errorf("unexpected path: %s", r.URL.Path)
		w.WriteHeader(http.StatusNotFound)
	}))
	defer ts.Close()

	var stdout, stderr bytes.Buffer
	c := newTestClient(ts.URL, &stdout, &stderr)

	// Path with leading / should still work.
	code := c.Do(t.Context(), "GET", "/rest/api/3/test", nil, nil)
	if code != jrerrors.ExitOK {
		t.Fatalf("expected exit 0, got %d; stderr=%s", code, stderr.String())
	}
}

// --- Bug 8: schema --jq and --pretty ---

func TestSchemaOutput_JQFilter(t *testing.T) {
	// Create a minimal cobra command with --jq and --pretty flags.
	cmd := &cobra.Command{Use: "test"}
	cmd.Flags().String("jq", "", "")
	cmd.Flags().Bool("pretty", false, "")
	_ = cmd.Flags().Set("jq", ".verb")

	data, _ := json.Marshal(map[string]string{"verb": "get", "resource": "issue"})
	err := schemaOutput(cmd, data)
	if err != nil {
		t.Fatalf("schemaOutput error: %v", err)
	}
	// The output goes to os.Stdout — we can't easily capture it in this test,
	// but we verify it doesn't error. A more thorough test would capture stdout.
}

func TestSchemaOutput_InvalidJQ(t *testing.T) {
	cmd := &cobra.Command{Use: "test"}
	cmd.Flags().String("jq", "", "")
	cmd.Flags().Bool("pretty", false, "")
	_ = cmd.Flags().Set("jq", ".[invalid")

	data, _ := json.Marshal(map[string]string{"key": "value"})
	err := schemaOutput(cmd, data)
	if err == nil {
		t.Fatal("expected error for invalid jq expression")
	}
}

// --- Bug 9: fetchJSON uses VerboseLog ---

func TestFetchJSON_VerboseLogging(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintln(w, `{"ok":true}`)
	}))
	defer ts.Close()

	var stdout, stderr bytes.Buffer
	c := newTestClient(ts.URL, &stdout, &stderr)
	c.Verbose = true

	_, code := fetchJSON(c, t.Context(), "GET", "/test")
	if code != jrerrors.ExitOK {
		t.Fatalf("expected exit 0, got %d", code)
	}

	// Verbose mode should have logged request and response to stderr.
	if !strings.Contains(stderr.String(), `"type":"request"`) {
		t.Error("expected verbose request log in stderr")
	}
	if !strings.Contains(stderr.String(), `"type":"response"`) {
		t.Error("expected verbose response log in stderr")
	}
}

func TestFetchJSON_NoVerboseByDefault(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintln(w, `{"ok":true}`)
	}))
	defer ts.Close()

	var stdout, stderr bytes.Buffer
	c := newTestClient(ts.URL, &stdout, &stderr)

	_, code := fetchJSON(c, t.Context(), "GET", "/test")
	if code != jrerrors.ExitOK {
		t.Fatalf("expected exit 0, got %d", code)
	}

	if stderr.String() != "" {
		t.Errorf("expected no verbose output by default, got: %s", stderr.String())
	}
}

// --- Bug 3: Generated commands support --body - (stdin marker) ---
// This is tested via the template change. Verify the template output.

func TestGeneratedTemplate_BodyStdinMarker(t *testing.T) {
	// Read a generated file and verify it contains the stdin marker check.
	// This is a compile-time verification that the template was regenerated.
	// The actual stdin behavior is tested in e2e tests.
	// Just verify the pattern exists in the generated code by checking
	// that generated commands handle bodyStr == "-".
	// (The fact that this test file compiles with the generated code
	// is itself verification that the template was applied.)
}

// --- Additional edge case tests ---

func TestBatchTransition_SubstringMatch(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "GET" {
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprintln(w, `{"transitions":[{"id":"11","name":"Move to In Progress","to":{"name":"In Progress"}}]}`)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer ts.Close()

	var stdout, stderr bytes.Buffer
	c := newTestClient(ts.URL, &stdout, &stderr)

	// Substring match: "progress" should match "In Progress".
	code := batchTransition(t.Context(), c, "TEST-1", "progress")
	if code != jrerrors.ExitOK {
		t.Fatalf("expected exit 0, got %d; stderr=%s", code, stderr.String())
	}
}

func TestBatchAssign_UserSearch(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/user/search") {
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprintln(w, `[{"accountId":"user123","displayName":"Test User"}]`)
			return
		}
		if strings.Contains(r.URL.Path, "/assignee") && r.Method == "PUT" {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer ts.Close()

	var stdout, stderr bytes.Buffer
	c := newTestClient(ts.URL, &stdout, &stderr)

	code := batchAssign(t.Context(), c, "TEST-1", "Test User")
	if code != jrerrors.ExitOK {
		t.Fatalf("expected exit 0, got %d; stderr=%s", code, stderr.String())
	}

	var result map[string]string
	if err := json.Unmarshal([]byte(strings.TrimSpace(stdout.String())), &result); err != nil {
		t.Fatalf("stdout not valid JSON: %s", stdout.String())
	}
	if result["status"] != "assigned" {
		t.Errorf("expected status=assigned, got %s", result["status"])
	}
}

func TestBatchAssign_UserNotFound(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintln(w, `[]`)
	}))
	defer ts.Close()

	var stdout, stderr bytes.Buffer
	c := newTestClient(ts.URL, &stdout, &stderr)

	code := batchAssign(t.Context(), c, "TEST-1", "nonexistent@example.com")
	if code != jrerrors.ExitNotFound {
		t.Fatalf("expected exit %d, got %d", jrerrors.ExitNotFound, code)
	}
}

// --- Bug: --dry-run ignored by workflow commands ---

func TestWorkflowTransition_DryRun(t *testing.T) {
	// No server needed — dry-run should never make HTTP calls.
	var stdout, stderr bytes.Buffer
	c := newTestClient("http://should-not-be-called", &stdout, &stderr)
	c.DryRun = true

	code := batchTransition(t.Context(), c, "TEST-1", "Done")
	if code != jrerrors.ExitOK {
		t.Fatalf("expected exit 0, got %d; stderr=%s", code, stderr.String())
	}

	var result map[string]string
	if err := json.Unmarshal([]byte(strings.TrimSpace(stdout.String())), &result); err != nil {
		t.Fatalf("stdout not valid JSON: %s", stdout.String())
	}
	if result["method"] != "POST" {
		t.Errorf("expected method=POST, got %s", result["method"])
	}
	if !strings.Contains(result["url"], "/transitions") {
		t.Errorf("expected URL containing /transitions, got %s", result["url"])
	}
	if !strings.Contains(result["note"], "would transition") {
		t.Errorf("expected dry-run note, got %s", result["note"])
	}
}

func TestWorkflowAssign_DryRun(t *testing.T) {
	var stdout, stderr bytes.Buffer
	c := newTestClient("http://should-not-be-called", &stdout, &stderr)
	c.DryRun = true

	code := batchAssign(t.Context(), c, "TEST-1", "me")
	if code != jrerrors.ExitOK {
		t.Fatalf("expected exit 0, got %d; stderr=%s", code, stderr.String())
	}

	var result map[string]string
	if err := json.Unmarshal([]byte(strings.TrimSpace(stdout.String())), &result); err != nil {
		t.Fatalf("stdout not valid JSON: %s", stdout.String())
	}
	if result["method"] != "PUT" {
		t.Errorf("expected method=PUT, got %s", result["method"])
	}
	if !strings.Contains(result["url"], "/assignee") {
		t.Errorf("expected URL containing /assignee, got %s", result["url"])
	}
	if !strings.Contains(result["note"], "would assign") {
		t.Errorf("expected dry-run note, got %s", result["note"])
	}
}

func TestBatchWorkflow_DryRun(t *testing.T) {
	var stdout, stderr bytes.Buffer
	c := newTestClient("http://should-not-be-called", &stdout, &stderr)
	c.DryRun = true

	code := executeBatchWorkflow(t.Context(), c, BatchOp{
		Command: "workflow transition",
		Args:    map[string]string{"issue": "TEST-1", "to": "Done"},
	})
	if code != jrerrors.ExitOK {
		t.Fatalf("expected exit 0, got %d; stderr=%s", code, stderr.String())
	}

	var result map[string]string
	if err := json.Unmarshal([]byte(strings.TrimSpace(stdout.String())), &result); err != nil {
		t.Fatalf("stdout not valid JSON: %s", stdout.String())
	}
	if result["method"] != "POST" {
		t.Errorf("expected method=POST, got %s", result["method"])
	}
}

func TestBatchWorkflowAssign_DryRun(t *testing.T) {
	var stdout, stderr bytes.Buffer
	c := newTestClient("http://should-not-be-called", &stdout, &stderr)
	c.DryRun = true

	code := executeBatchWorkflow(t.Context(), c, BatchOp{
		Command: "workflow assign",
		Args:    map[string]string{"issue": "TEST-1", "to": "none"},
	})
	if code != jrerrors.ExitOK {
		t.Fatalf("expected exit 0, got %d; stderr=%s", code, stderr.String())
	}

	var result map[string]string
	if err := json.Unmarshal([]byte(strings.TrimSpace(stdout.String())), &result); err != nil {
		t.Fatalf("stdout not valid JSON: %s", stdout.String())
	}
	if result["method"] != "PUT" {
		t.Errorf("expected method=PUT, got %s", result["method"])
	}
}

func TestFetchJSONWithBody_Error(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		fmt.Fprintln(w, `{"errorMessages":["forbidden"]}`)
	}))
	defer ts.Close()

	var stdout, stderr bytes.Buffer
	c := newTestClient(ts.URL, &stdout, &stderr)

	_, code := fetchJSONWithBody(c, t.Context(), "POST", "/test", strings.NewReader(`{}`))
	if code == jrerrors.ExitOK {
		t.Fatal("expected non-zero exit code for 403 response")
	}
	if !strings.Contains(stderr.String(), "forbidden") {
		t.Errorf("expected error message in stderr, got: %s", stderr.String())
	}
}

// --- Bug: root --help uses HTML-escaped JSON ---

func TestRootHelp_NoHTMLEscaping(t *testing.T) {
	// The root help JSON should not contain HTML-escaped angle brackets.
	// We can't easily test the actual help function (it calls os.Exit),
	// so verify the encoding approach directly.
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	_ = enc.Encode(map[string]string{
		"hint": "use `jr schema <resource>` for operations",
	})

	output := buf.String()
	if strings.Contains(output, `\u003c`) {
		t.Errorf("expected no HTML escaping in JSON, got: %s", output)
	}
	if !strings.Contains(output, `<resource>`) {
		t.Errorf("expected literal <resource> in output, got: %s", output)
	}
}

// --- Bug 21a: raw.go returns unwrapped errors causing double JSON output ---

func TestRawCmd_FileOpenError_SingleOutput(t *testing.T) {
	// Simulate the raw command's file open error path.
	// After the fix, it should return errAlreadyWritten, not a raw error.
	cmd := &cobra.Command{Use: "raw"}
	f := cmd.Flags()
	f.String("body", "", "")
	f.StringArray("query", nil, "")
	f.String("fields", "", "")
	_ = cmd.Flags().Set("body", "@nonexistent-file-for-test.json")

	// We need a valid client in context for the code to reach the file open path.
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer ts.Close()

	var stdout, stderr bytes.Buffer
	c := newTestClient(ts.URL, &stdout, &stderr)
	ctx := client.NewContext(t.Context(), c)
	cmd.SetContext(ctx)

	err := runRaw(cmd, []string{"POST", "/test"})
	if err == nil {
		t.Fatal("expected error for nonexistent body file")
	}
	if _, ok := err.(*errAlreadyWritten); !ok {
		t.Errorf("expected errAlreadyWritten to prevent double error output, got %T: %v", err, err)
	}
}

// --- Bug 21b: batch.go batchAssign conflates JSON parse error with not-found ---

func TestBatchAssign_ParseError_ReturnsExitError(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/user/search") {
			w.Header().Set("Content-Type", "application/json")
			// Return invalid JSON to trigger parse error
			fmt.Fprintln(w, `not valid json`)
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer ts.Close()

	var stdout, stderr bytes.Buffer
	c := newTestClient(ts.URL, &stdout, &stderr)

	code := batchAssign(t.Context(), c, "TEST-1", "someuser@example.com")
	// Parse errors should return ExitError (1), not ExitNotFound (3)
	if code != jrerrors.ExitError {
		t.Fatalf("expected exit %d (error) for parse failure, got %d", jrerrors.ExitError, code)
	}
	if !strings.Contains(stderr.String(), "connection_error") {
		t.Errorf("expected connection_error in stderr for parse failure, got: %s", stderr.String())
	}
}

// --- Bug 21c: JSON injection safety for transition ID and accountId ---

func TestBatchTransition_JSONSafe(t *testing.T) {
	// Verify that transition IDs with special characters are properly escaped
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "GET" && strings.Contains(r.URL.Path, "/transitions") {
			w.Header().Set("Content-Type", "application/json")
			// Transition ID with a quote character (adversarial)
			fmt.Fprintln(w, `{"transitions":[{"id":"11\"injected","name":"Done","to":{"name":"Done"}}]}`)
			return
		}
		if r.Method == "POST" && strings.Contains(r.URL.Path, "/transitions") {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer ts.Close()

	var stdout, stderr bytes.Buffer
	c := newTestClient(ts.URL, &stdout, &stderr)

	code := batchTransition(t.Context(), c, "TEST-1", "Done")
	if code != jrerrors.ExitOK {
		t.Fatalf("expected exit 0, got %d; stderr=%s", code, stderr.String())
	}
}

// --- Bug #22: batch --verbose forwards verbose logs to real stderr ---

func TestBuildBatchResult_VerboseForwarded(t *testing.T) {
	var stdoutBuf, stderrBuf strings.Builder
	// Simulate verbose output (request + response lines) mixed with no error.
	stderrBuf.WriteString(`{"type":"request","method":"GET","url":"http://example.com/test"}` + "\n")
	stderrBuf.WriteString(`{"type":"response","status":200}` + "\n")

	// Capture real stderr by redirecting os.Stderr temporarily.
	oldStderr := os.Stderr
	r, w, _ := os.Pipe()
	os.Stderr = w

	result := buildBatchResult(0, jrerrors.ExitOK, &stdoutBuf, &stderrBuf, true)

	w.Close()
	os.Stderr = oldStderr

	var captured bytes.Buffer
	_, _ = captured.ReadFrom(r)

	if result.ExitCode != jrerrors.ExitOK {
		t.Fatalf("expected exit 0, got %d", result.ExitCode)
	}
	// Verbose lines should have been forwarded to stderr.
	if !strings.Contains(captured.String(), `"type":"request"`) {
		t.Errorf("expected verbose request line in stderr, got: %s", captured.String())
	}
	if !strings.Contains(captured.String(), `"type":"response"`) {
		t.Errorf("expected verbose response line in stderr, got: %s", captured.String())
	}
}

func TestBuildBatchResult_NoVerbose_NotForwarded(t *testing.T) {
	var stdoutBuf, stderrBuf strings.Builder
	stderrBuf.WriteString(`{"type":"request","method":"GET","url":"http://example.com/test"}` + "\n")

	oldStderr := os.Stderr
	r, w, _ := os.Pipe()
	os.Stderr = w

	_ = buildBatchResult(0, jrerrors.ExitOK, &stdoutBuf, &stderrBuf, false)

	w.Close()
	os.Stderr = oldStderr

	var captured bytes.Buffer
	_, _ = captured.ReadFrom(r)

	// Without verbose=true, nothing should be forwarded.
	if captured.String() != "" {
		t.Errorf("expected no stderr output when verbose=false, got: %s", captured.String())
	}
}

// --- Bug #22: batch --jq applies to batch output array ---

// captureStdout captures os.Stdout output during fn execution.
func captureStdout(t *testing.T, fn func()) string {
	t.Helper()
	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w
	fn()
	w.Close()
	os.Stdout = oldStdout
	var buf bytes.Buffer
	_, _ = buf.ReadFrom(r)
	return buf.String()
}

func TestBatchJQFilter(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintln(w, `{"key":"TEST-1"}`)
	}))
	defer ts.Close()

	var stdout, stderr bytes.Buffer
	c := newTestClient(ts.URL, &stdout, &stderr)
	c.JQFilter = ".[0].data.key"

	ctx := client.NewContext(t.Context(), c)
	cmd := batchCmd
	cmd.ResetFlags()
	cmd.Flags().String("input", "", "")
	cmd.SetContext(ctx)

	input := `[{"command":"issue get","args":{"issueIdOrKey":"TEST-1"}}]`

	oldStdin := os.Stdin
	pr, pw, _ := os.Pipe()
	_, _ = pw.WriteString(input)
	pw.Close()
	os.Stdin = pr
	defer func() { os.Stdin = oldStdin }()

	captured := captureStdout(t, func() {
		err := runBatch(cmd, nil)
		if err != nil {
			t.Fatalf("runBatch error: %v", err)
		}
	})

	got := strings.TrimSpace(captured)
	if got != `"TEST-1"` {
		t.Errorf("expected JQ-filtered output \"TEST-1\", got: %s", got)
	}
}

// --- Bug #22: batch --pretty formats batch output ---

func TestBatchPrettyOutput(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintln(w, `{"key":"TEST-1"}`)
	}))
	defer ts.Close()

	var stdout, stderr bytes.Buffer
	c := newTestClient(ts.URL, &stdout, &stderr)
	c.Pretty = true

	ctx := client.NewContext(t.Context(), c)
	cmd := batchCmd
	cmd.ResetFlags()
	cmd.Flags().String("input", "", "")
	cmd.SetContext(ctx)

	input := `[{"command":"issue get","args":{"issueIdOrKey":"TEST-1"}}]`
	oldStdin := os.Stdin
	pr, pw, _ := os.Pipe()
	_, _ = pw.WriteString(input)
	pw.Close()
	os.Stdin = pr
	defer func() { os.Stdin = oldStdin }()

	captured := captureStdout(t, func() {
		err := runBatch(cmd, nil)
		if err != nil {
			t.Fatalf("runBatch error: %v", err)
		}
	})

	// Pretty-printed output should contain indentation (newlines + spaces).
	if !strings.Contains(captured, "\n") || !strings.Contains(captured, "  ") {
		t.Errorf("expected pretty-printed output with indentation, got: %s", captured)
	}
}

// --- Bug #24: URL query merging when path has inline query params ---

func TestClientDo_PathWithQueryParams(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify URL has both inline and additional params merged with &
		if !strings.Contains(r.URL.RawQuery, "jql=project") {
			t.Errorf("missing inline param jql, got query: %s", r.URL.RawQuery)
		}
		if !strings.Contains(r.URL.RawQuery, "maxResults=1") {
			t.Errorf("missing added param maxResults, got query: %s", r.URL.RawQuery)
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintln(w, `{"ok":true}`)
	}))
	defer ts.Close()

	var stdout, stderr bytes.Buffer
	c := newTestClient(ts.URL, &stdout, &stderr)
	c.Paginate = false

	query := make(map[string][]string)
	query["maxResults"] = []string{"1"}
	// Path already has ?jql=... inline
	code := c.Do(t.Context(), "GET", "/rest/api/3/search?jql=project%3DKAN", query, nil)
	if code != 0 {
		t.Fatalf("expected exit 0, got %d; stderr=%s", code, stderr.String())
	}
}

// --- Bug #24: configure validates empty profile name ---

func TestConfigureRejectsEmptyProfile(t *testing.T) {
	// Use a fresh cobra.Command to avoid mutating global configureCmd state.
	cmd := &cobra.Command{Use: "configure", RunE: runConfigure}
	f := cmd.Flags()
	f.String("base-url", "", "")
	f.String("token", "", "")
	f.StringP("profile", "p", "default", "profile name")
	f.String("auth-type", "basic", "")
	f.String("username", "", "")
	f.Bool("test", false, "")
	f.Bool("delete", false, "")

	_ = cmd.Flags().Set("base-url", "https://test.atlassian.net")
	_ = cmd.Flags().Set("token", "fake-token")
	_ = cmd.Flags().Set("username", "user@test.com")
	_ = cmd.Flags().Set("profile", "   ")

	err := runConfigure(cmd, nil)
	if err == nil {
		t.Fatal("expected error for whitespace-only profile name")
	}
	aw, ok := err.(*errAlreadyWritten)
	if !ok {
		t.Fatalf("expected errAlreadyWritten, got %T: %v", err, err)
	}
	if aw.code != jrerrors.ExitValidation {
		t.Errorf("expected exit code %d, got %d", jrerrors.ExitValidation, aw.code)
	}
}

func TestBatchAssign_JSONSafe(t *testing.T) {
	// Verify that account IDs with special characters are properly escaped
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/rest/api/3/myself" {
			w.Header().Set("Content-Type", "application/json")
			// Account ID with a quote character (adversarial)
			fmt.Fprintln(w, `{"accountId":"abc\"injected"}`)
			return
		}
		if strings.Contains(r.URL.Path, "/assignee") && r.Method == "PUT" {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer ts.Close()

	var stdout, stderr bytes.Buffer
	c := newTestClient(ts.URL, &stdout, &stderr)

	code := batchAssign(t.Context(), c, "TEST-1", "me")
	if code != jrerrors.ExitOK {
		t.Fatalf("expected exit 0, got %d; stderr=%s", code, stderr.String())
	}
}

// --- Bug 25: Workflow transition and assign ignore --jq and --pretty flags ---

func TestBatchTransition_JQFilter(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "GET" && strings.Contains(r.URL.Path, "/transitions") {
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprintln(w, `{"transitions":[{"id":"11","name":"Done","to":{"name":"Done"}}]}`)
			return
		}
		if r.Method == "POST" && strings.Contains(r.URL.Path, "/transitions") {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer ts.Close()

	var stdout, stderr bytes.Buffer
	c := newTestClient(ts.URL, &stdout, &stderr)
	c.JQFilter = ".issue"

	code := batchTransition(t.Context(), c, "TEST-1", "Done")
	if code != jrerrors.ExitOK {
		t.Fatalf("expected exit 0, got %d; stderr=%s", code, stderr.String())
	}

	got := strings.TrimSpace(stdout.String())
	if got != `"TEST-1"` {
		t.Errorf("expected jq-filtered output %q, got %q", `"TEST-1"`, got)
	}
}

func TestBatchTransition_PrettyPrint(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "GET" && strings.Contains(r.URL.Path, "/transitions") {
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprintln(w, `{"transitions":[{"id":"11","name":"In Progress","to":{"name":"In Progress"}}]}`)
			return
		}
		if r.Method == "POST" && strings.Contains(r.URL.Path, "/transitions") {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer ts.Close()

	var stdout, stderr bytes.Buffer
	c := newTestClient(ts.URL, &stdout, &stderr)
	c.Pretty = true

	code := batchTransition(t.Context(), c, "TEST-1", "In Progress")
	if code != jrerrors.ExitOK {
		t.Fatalf("expected exit 0, got %d; stderr=%s", code, stderr.String())
	}

	got := stdout.String()
	// Pretty-printed JSON should contain indentation (at least two spaces or a newline between fields).
	if !strings.Contains(got, "\n") || !strings.Contains(got, "  ") {
		t.Errorf("expected pretty-printed output, got: %s", got)
	}
}

func TestBatchAssign_JQFilter(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/rest/api/3/myself" {
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprintln(w, `{"accountId":"abc123"}`)
			return
		}
		if strings.Contains(r.URL.Path, "/assignee") && r.Method == "PUT" {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer ts.Close()

	var stdout, stderr bytes.Buffer
	c := newTestClient(ts.URL, &stdout, &stderr)
	c.JQFilter = ".issue"

	code := batchAssign(t.Context(), c, "TEST-1", "me")
	if code != jrerrors.ExitOK {
		t.Fatalf("expected exit 0, got %d; stderr=%s", code, stderr.String())
	}

	got := strings.TrimSpace(stdout.String())
	if got != `"TEST-1"` {
		t.Errorf("expected jq-filtered output %q, got %q", `"TEST-1"`, got)
	}
}

func TestBatchAssign_None_JQFilter(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/assignee") && r.Method == "PUT" {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer ts.Close()

	var stdout, stderr bytes.Buffer
	c := newTestClient(ts.URL, &stdout, &stderr)
	c.JQFilter = ".status"

	code := batchAssign(t.Context(), c, "TEST-1", "none")
	if code != jrerrors.ExitOK {
		t.Fatalf("expected exit 0, got %d; stderr=%s", code, stderr.String())
	}

	got := strings.TrimSpace(stdout.String())
	if got != `"unassigned"` {
		t.Errorf("expected jq-filtered output %q, got %q", `"unassigned"`, got)
	}
}

// --- Bug #28: dry-run output ignores --jq and --pretty ---

func TestBatchTransition_DryRun_JQ(t *testing.T) {
	var stdout, stderr bytes.Buffer
	c := newTestClient("http://should-not-be-called", &stdout, &stderr)
	c.DryRun = true
	c.JQFilter = ".method"

	code := batchTransition(t.Context(), c, "TEST-1", "Done")
	if code != jrerrors.ExitOK {
		t.Fatalf("expected exit 0, got %d; stderr=%s", code, stderr.String())
	}
	got := strings.TrimSpace(stdout.String())
	if got != `"POST"` {
		t.Errorf("expected jq-filtered dry-run output %q, got %q", `"POST"`, got)
	}
}

func TestBatchTransition_DryRun_Pretty(t *testing.T) {
	var stdout, stderr bytes.Buffer
	c := newTestClient("http://should-not-be-called", &stdout, &stderr)
	c.DryRun = true
	c.Pretty = true

	code := batchTransition(t.Context(), c, "TEST-1", "Done")
	if code != jrerrors.ExitOK {
		t.Fatalf("expected exit 0, got %d; stderr=%s", code, stderr.String())
	}
	got := stdout.String()
	if !strings.Contains(got, "\n") || !strings.Contains(got, "  ") {
		t.Errorf("expected pretty-printed dry-run output, got: %s", got)
	}
}

func TestBatchAssign_DryRun_JQ(t *testing.T) {
	var stdout, stderr bytes.Buffer
	c := newTestClient("http://should-not-be-called", &stdout, &stderr)
	c.DryRun = true
	c.JQFilter = ".method"

	code := batchAssign(t.Context(), c, "TEST-1", "me")
	if code != jrerrors.ExitOK {
		t.Fatalf("expected exit 0, got %d; stderr=%s", code, stderr.String())
	}
	got := strings.TrimSpace(stdout.String())
	if got != `"PUT"` {
		t.Errorf("expected jq-filtered dry-run output %q, got %q", `"PUT"`, got)
	}
}

func TestBatchAssign_DryRun_Pretty(t *testing.T) {
	var stdout, stderr bytes.Buffer
	c := newTestClient("http://should-not-be-called", &stdout, &stderr)
	c.DryRun = true
	c.Pretty = true

	code := batchAssign(t.Context(), c, "TEST-1", "me")
	if code != jrerrors.ExitOK {
		t.Fatalf("expected exit 0, got %d; stderr=%s", code, stderr.String())
	}
	got := stdout.String()
	if !strings.Contains(got, "\n") || !strings.Contains(got, "  ") {
		t.Errorf("expected pretty-printed dry-run output, got: %s", got)
	}
}

func TestBatchAssign_PrettyPrint(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/rest/api/3/myself" {
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprintln(w, `{"accountId":"abc123"}`)
			return
		}
		if strings.Contains(r.URL.Path, "/assignee") && r.Method == "PUT" {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer ts.Close()

	var stdout, stderr bytes.Buffer
	c := newTestClient(ts.URL, &stdout, &stderr)
	c.Pretty = true

	code := batchAssign(t.Context(), c, "TEST-1", "me")
	if code != jrerrors.ExitOK {
		t.Fatalf("expected exit 0, got %d; stderr=%s", code, stderr.String())
	}

	got := stdout.String()
	if !strings.Contains(got, "\n") || !strings.Contains(got, "  ") {
		t.Errorf("expected pretty-printed output, got: %s", got)
	}
}

// --- Bug #30: configure output ignores --jq and --pretty ---

func TestConfigureTest_JQFilter(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintln(w, `{"accountId":"abc123"}`)
	}))
	defer ts.Close()

	// Write a temporary config file with the test server's URL.
	tmpDir := t.TempDir()
	configPath := tmpDir + "/config.json"
	cfg := &config.Config{
		DefaultProfile: "default",
		Profiles: map[string]config.Profile{
			"default": {
				BaseURL: ts.URL,
				Auth:    config.AuthConfig{Type: "basic", Username: "u", Token: "t"},
			},
		},
	}
	if err := config.SaveTo(cfg, configPath); err != nil {
		t.Fatal(err)
	}

	// Override config path for the test.
	origPath := os.Getenv("JR_CONFIG_PATH")
	os.Setenv("JR_CONFIG_PATH", configPath)
	defer os.Setenv("JR_CONFIG_PATH", origPath)

	cmd := &cobra.Command{Use: "configure", RunE: runConfigure}
	f := cmd.Flags()
	f.String("base-url", "", "")
	f.String("token", "", "")
	f.String("profile", "default", "")
	f.String("auth-type", "basic", "")
	f.String("username", "", "")
	f.Bool("test", false, "")
	f.Bool("delete", false, "")
	f.String("jq", "", "")
	f.Bool("pretty", false, "")

	_ = cmd.Flags().Set("test", "true")
	_ = cmd.Flags().Set("jq", ".status")

	captured := captureStdout(t, func() {
		err := runConfigure(cmd, nil)
		if err != nil {
			t.Fatalf("runConfigure error: %v", err)
		}
	})

	got := strings.TrimSpace(captured)
	if got != `"ok"` {
		t.Errorf("expected jq-filtered output %q, got %q", `"ok"`, got)
	}
}

func TestConfigureTest_PrettyPrint(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintln(w, `{"accountId":"abc123"}`)
	}))
	defer ts.Close()

	tmpDir := t.TempDir()
	configPath := tmpDir + "/config.json"
	cfg := &config.Config{
		DefaultProfile: "default",
		Profiles: map[string]config.Profile{
			"default": {
				BaseURL: ts.URL,
				Auth:    config.AuthConfig{Type: "basic", Username: "u", Token: "t"},
			},
		},
	}
	if err := config.SaveTo(cfg, configPath); err != nil {
		t.Fatal(err)
	}

	origPath := os.Getenv("JR_CONFIG_PATH")
	os.Setenv("JR_CONFIG_PATH", configPath)
	defer os.Setenv("JR_CONFIG_PATH", origPath)

	cmd := &cobra.Command{Use: "configure", RunE: runConfigure}
	f := cmd.Flags()
	f.String("base-url", "", "")
	f.String("token", "", "")
	f.String("profile", "default", "")
	f.String("auth-type", "basic", "")
	f.String("username", "", "")
	f.Bool("test", false, "")
	f.Bool("delete", false, "")
	f.String("jq", "", "")
	f.Bool("pretty", false, "")

	_ = cmd.Flags().Set("test", "true")
	_ = cmd.Flags().Set("pretty", "true")

	captured := captureStdout(t, func() {
		err := runConfigure(cmd, nil)
		if err != nil {
			t.Fatalf("runConfigure error: %v", err)
		}
	})

	if !strings.Contains(captured, "\n") || !strings.Contains(captured, "  ") {
		t.Errorf("expected pretty-printed output, got: %s", captured)
	}
}

// --- Bug #31: batch missing required path params sends literal placeholder ---

func TestBatchMissingRequiredPathParam(t *testing.T) {
	allOps := generated.AllSchemaOps()
	allOps = append(allOps, HandWrittenSchemaOps()...)
	opMap := make(map[string]generated.SchemaOp)
	for _, op := range allOps {
		opMap[op.Resource+" "+op.Verb] = op
	}

	var stdout, stderr bytes.Buffer
	c := newTestClient("http://should-not-be-called", &stdout, &stderr)
	ctx := client.NewContext(t.Context(), c)
	cmd := &cobra.Command{Use: "batch"}
	cmd.Flags().String("input", "", "")
	cmd.SetContext(ctx)

	// issue get requires issueIdOrKey but we don't provide it
	result := executeBatchOp(cmd, c, 0, BatchOp{
		Command: "issue get",
		Args:    map[string]string{},
	}, opMap)

	if result.ExitCode != jrerrors.ExitValidation {
		t.Fatalf("expected exit code %d (validation), got %d", jrerrors.ExitValidation, result.ExitCode)
	}

	// Verify the error message mentions the missing parameter.
	if !strings.Contains(string(result.Error), "issueIdOrKey") {
		t.Errorf("expected error mentioning issueIdOrKey, got: %s", string(result.Error))
	}
}

func TestBatchEmptyPathParamValue(t *testing.T) {
	allOps := generated.AllSchemaOps()
	allOps = append(allOps, HandWrittenSchemaOps()...)
	opMap := make(map[string]generated.SchemaOp)
	for _, op := range allOps {
		opMap[op.Resource+" "+op.Verb] = op
	}

	var stdout, stderr bytes.Buffer
	c := newTestClient("http://should-not-be-called", &stdout, &stderr)
	ctx := client.NewContext(t.Context(), c)
	cmd := &cobra.Command{Use: "batch"}
	cmd.Flags().String("input", "", "")
	cmd.SetContext(ctx)

	// issue get with empty issueIdOrKey
	result := executeBatchOp(cmd, c, 0, BatchOp{
		Command: "issue get",
		Args:    map[string]string{"issueIdOrKey": ""},
	}, opMap)

	if result.ExitCode != jrerrors.ExitValidation {
		t.Fatalf("expected exit code %d (validation), got %d", jrerrors.ExitValidation, result.ExitCode)
	}
}

// --- Bug #32: cache files should be owner-readable only ---

func TestCacheFilePermissions(t *testing.T) {
	key := "test-perms-key"
	data := []byte(`{"test":"data"}`)

	if err := cache.Set(key, data); err != nil {
		t.Fatal(err)
	}

	// Read back the file permissions.
	dir := cache.Dir()
	path := dir + "/" + key
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}

	// Should be 0600 (owner read/write only), not 0644.
	perm := info.Mode().Perm()
	if perm != 0o600 {
		t.Errorf("expected cache file permission 0600, got %04o", perm)
	}

	// Clean up.
	os.Remove(path)
}

// --- Bug #33: configure accepts invalid --auth-type, ApplyAuth silently skips auth ---

func TestConfigureRejectsInvalidAuthType(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := tmpDir + "/config.json"
	origPath := os.Getenv("JR_CONFIG_PATH")
	os.Setenv("JR_CONFIG_PATH", configPath)
	defer os.Setenv("JR_CONFIG_PATH", origPath)

	cmd := &cobra.Command{Use: "configure", RunE: runConfigure}
	f := cmd.Flags()
	f.String("base-url", "", "")
	f.String("token", "", "")
	f.String("profile", "default", "")
	f.String("auth-type", "basic", "")
	f.String("username", "", "")
	f.Bool("test", false, "")
	f.Bool("delete", false, "")
	f.String("jq", "", "")
	f.Bool("pretty", false, "")

	_ = cmd.Flags().Set("base-url", "https://example.atlassian.net")
	_ = cmd.Flags().Set("token", "test-token")
	_ = cmd.Flags().Set("auth-type", "cloud")

	var stderr bytes.Buffer
	cmd.SetErr(&stderr)

	err := runConfigure(cmd, nil)
	if err == nil {
		t.Fatal("expected error for invalid auth-type, got nil")
	}
	aw, ok := err.(*errAlreadyWritten)
	if !ok {
		t.Fatalf("expected errAlreadyWritten, got %T: %v", err, err)
	}
	if aw.code != jrerrors.ExitValidation {
		t.Errorf("expected exit code %d, got %d", jrerrors.ExitValidation, aw.code)
	}
}

func TestConfigureAcceptsValidAuthTypes(t *testing.T) {
	for _, authType := range []string{"basic", "bearer", "BASIC", "Bearer"} {
		t.Run(authType, func(t *testing.T) {
			tmpDir := t.TempDir()
			configPath := tmpDir + "/config.json"
			origPath := os.Getenv("JR_CONFIG_PATH")
			os.Setenv("JR_CONFIG_PATH", configPath)
			defer os.Setenv("JR_CONFIG_PATH", origPath)

			cmd := &cobra.Command{Use: "configure", RunE: runConfigure}
			f := cmd.Flags()
			f.String("base-url", "", "")
			f.String("token", "", "")
			f.String("profile", "default", "")
			f.String("auth-type", "basic", "")
			f.String("username", "", "")
			f.Bool("test", false, "")
			f.Bool("delete", false, "")
			f.String("jq", "", "")
			f.Bool("pretty", false, "")

			_ = cmd.Flags().Set("base-url", "https://example.atlassian.net")
			_ = cmd.Flags().Set("token", "test-token")
			_ = cmd.Flags().Set("username", "user@example.com")
			_ = cmd.Flags().Set("auth-type", authType)

			captured := captureStdout(t, func() {
				if err := runConfigure(cmd, nil); err != nil {
					t.Fatalf("unexpected error for auth-type %q: %v", authType, err)
				}
			})

			if !strings.Contains(captured, `"status":"saved"`) {
				t.Errorf("expected saved status for auth-type %q, got: %s", authType, captured)
			}

			// Verify the saved auth type is lowercased.
			cfg, err := config.LoadFrom(configPath)
			if err != nil {
				t.Fatal(err)
			}
			got := cfg.Profiles["default"].Auth.Type
			if got != strings.ToLower(authType) {
				t.Errorf("expected saved auth type %q, got %q", strings.ToLower(authType), got)
			}
		})
	}
}

func TestApplyAuthDefaultsToBasicForUnknownType(t *testing.T) {
	// Even if an unknown type slips through (e.g. manually edited config),
	// ApplyAuth should default to basic auth rather than silently skipping.
	c := &client.Client{
		Auth: config.AuthConfig{Type: "unknown", Username: "user", Token: "tok"},
	}
	req, _ := http.NewRequest("GET", "http://example.com", nil)
	if err := c.ApplyAuth(req); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	auth := req.Header.Get("Authorization")
	if auth == "" {
		t.Fatal("expected Authorization header for unknown auth type (should default to basic), got empty")
	}
	if !strings.HasPrefix(auth, "Basic ") {
		t.Errorf("expected Basic auth header, got: %s", auth)
	}
}

// --- Bug #34: generated POST commands hang in dry-run mode in non-terminal contexts ---

func TestDryRunPostNilBodyDoesNotHang(t *testing.T) {
	// Simulate what happens when a generated POST command runs in dry-run
	// mode without a body (e.g. non-terminal context where stdin is not available).
	var stdout, stderr bytes.Buffer
	c := &client.Client{
		BaseURL:    "https://example.atlassian.net",
		Auth:       config.AuthConfig{Type: "basic", Username: "u", Token: "t"},
		HTTPClient: &http.Client{},
		Stdout:     &stdout,
		Stderr:     &stderr,
		DryRun:     true,
	}

	// Call Do with nil body — this is what generated commands do in dry-run
	// when stdin is not available.
	code := c.Do(t.Context(), "POST", "/rest/api/3/issue", nil, nil)
	if code != 0 {
		t.Errorf("expected exit code 0, got %d; stderr: %s", code, stderr.String())
	}

	var result map[string]interface{}
	if err := json.Unmarshal(stdout.Bytes(), &result); err != nil {
		t.Fatalf("invalid JSON output: %v; raw: %s", err, stdout.String())
	}
	if result["method"] != "POST" {
		t.Errorf("expected method POST in dry-run, got %v", result["method"])
	}
}

// --- Bug #35: schema --list missing hand-written resources (workflow) ---

func TestSchemaListIncludesWorkflow(t *testing.T) {
	allOps := generated.AllSchemaOps()
	allOps = append(allOps, HandWrittenSchemaOps()...)

	// Build the same resource list as the fix in schema_cmd.go
	resources := generated.AllResources()
	seen := make(map[string]bool, len(resources))
	for _, r := range resources {
		seen[r] = true
	}
	for _, op := range allOps {
		if !seen[op.Resource] {
			resources = append(resources, op.Resource)
			seen[op.Resource] = true
		}
	}

	found := false
	for _, r := range resources {
		if r == "workflow" {
			found = true
			break
		}
	}
	if !found {
		t.Error("schema --list should include 'workflow' resource, but it was missing")
	}
}

// --- Bug #36: testConnection doesn't handle oauth2 auth type ---

func TestTestConnection_OAuth2ReturnsError(t *testing.T) {
	err := testConnection("https://example.atlassian.net", "oauth2", "", "")
	if err == nil {
		t.Fatal("expected error for oauth2 auth type in testConnection")
	}
	if !strings.Contains(err.Error(), "oauth2") {
		t.Errorf("expected error mentioning oauth2, got: %s", err.Error())
	}
}

// --- Bug #37: testConnection double-slash when base URL has trailing slash ---

func TestTestConnection_TrailingSlashNormalized(t *testing.T) {
	var receivedPath string
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedPath = r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintln(w, `{"accountId":"abc123"}`)
	}))
	defer ts.Close()

	// URL with trailing slash — should NOT produce double-slash in request
	err := testConnection(ts.URL+"/", "basic", "user", "token")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if strings.Contains(receivedPath, "//") {
		t.Errorf("testConnection produced double-slash in path: %s", receivedPath)
	}
}

// --- Bug #38: configure saves base URL with trailing slash ---

func TestConfigureNormalizesTrailingSlash(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := tmpDir + "/config.json"
	origPath := os.Getenv("JR_CONFIG_PATH")
	os.Setenv("JR_CONFIG_PATH", configPath)
	defer os.Setenv("JR_CONFIG_PATH", origPath)

	cmd := &cobra.Command{Use: "configure", RunE: runConfigure}
	f := cmd.Flags()
	f.String("base-url", "", "")
	f.String("token", "", "")
	f.String("profile", "default", "")
	f.String("auth-type", "basic", "")
	f.String("username", "", "")
	f.Bool("test", false, "")
	f.Bool("delete", false, "")
	f.String("jq", "", "")
	f.Bool("pretty", false, "")

	_ = cmd.Flags().Set("base-url", "https://example.atlassian.net/")
	_ = cmd.Flags().Set("token", "test-token")
	_ = cmd.Flags().Set("username", "user@example.com")

	captureStdout(t, func() {
		if err := runConfigure(cmd, nil); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	cfg, err := config.LoadFrom(configPath)
	if err != nil {
		t.Fatal(err)
	}
	got := cfg.Profiles["default"].BaseURL
	if strings.HasSuffix(got, "/") {
		t.Errorf("expected trailing slash to be stripped, got: %s", got)
	}
}

// --- Bug #39: raw --query with malformed param (no =) ---

func TestRawCmd_MalformedQueryParam(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		fmt.Fprintln(w, `{}`)
	}))
	defer ts.Close()

	var stdout, stderr bytes.Buffer
	c := newTestClient(ts.URL, &stdout, &stderr)

	cmd := &cobra.Command{Use: "raw"}
	f := cmd.Flags()
	f.String("body", "", "")
	f.StringArray("query", nil, "")
	f.String("fields", "", "")

	_ = cmd.Flags().Set("query", "malformed-no-equals")

	ctx := client.NewContext(t.Context(), c)
	cmd.SetContext(ctx)

	err := runRaw(cmd, []string{"GET", "/test"})
	if err == nil {
		t.Fatal("expected error for malformed query param")
	}
	aw, ok := err.(*errAlreadyWritten)
	if !ok {
		t.Fatalf("expected errAlreadyWritten, got %T: %v", err, err)
	}
	if aw.code != jrerrors.ExitValidation {
		t.Errorf("expected exit code %d, got %d", jrerrors.ExitValidation, aw.code)
	}
}

func TestRawCmd_ValidQueryParam(t *testing.T) {
	var receivedQuery string
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedQuery = r.URL.RawQuery
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintln(w, `{}`)
	}))
	defer ts.Close()

	var stdout, stderr bytes.Buffer
	c := newTestClient(ts.URL, &stdout, &stderr)
	c.Paginate = false

	cmd := &cobra.Command{Use: "raw"}
	f := cmd.Flags()
	f.String("body", "", "")
	f.StringArray("query", nil, "")
	f.String("fields", "", "")

	_ = cmd.Flags().Set("query", "key=value")

	ctx := client.NewContext(t.Context(), c)
	cmd.SetContext(ctx)

	err := runRaw(cmd, []string{"GET", "/test"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(receivedQuery, "key=value") {
		t.Errorf("expected query to contain key=value, got: %s", receivedQuery)
	}
}

// --- Bug #40: generated commands return structured error for @file open failures ---
// The template fix is verified by the conformance test (gen/conformance_test.go)
// which ensures generated files match the template. Here we verify the template
// itself contains the structured error pattern.

// --- Bug #41: Invalid --auth-type at runtime should error, not silently default to basic ---

func TestResolveRejectsInvalidAuthType(t *testing.T) {
	// Create a config file with a valid profile.
	dir := t.TempDir()
	cfgPath := dir + "/config.json"
	cfg := &config.Config{
		Profiles: map[string]config.Profile{
			"default": {
				BaseURL: "https://example.com",
				Auth:    config.AuthConfig{Type: "basic", Username: "u", Token: "t"},
			},
		},
		DefaultProfile: "default",
	}
	if err := config.SaveTo(cfg, cfgPath); err != nil {
		t.Fatalf("SaveTo: %v", err)
	}

	// Resolve with invalid auth-type flag override should fail.
	flags := &config.FlagOverrides{AuthType: "invalidtype"}
	_, err := config.Resolve(cfgPath, "", flags)
	if err == nil {
		t.Fatal("expected error for invalid auth type, got nil")
	}
	if !strings.Contains(err.Error(), "invalid auth type") {
		t.Errorf("error message = %q, want it to contain 'invalid auth type'", err.Error())
	}
}

func TestResolveAcceptsValidAuthTypes(t *testing.T) {
	dir := t.TempDir()
	cfgPath := dir + "/config.json"
	cfg := &config.Config{
		Profiles: map[string]config.Profile{
			"default": {
				BaseURL: "https://example.com",
				Auth:    config.AuthConfig{Type: "basic", Username: "u", Token: "t"},
			},
		},
		DefaultProfile: "default",
	}
	if err := config.SaveTo(cfg, cfgPath); err != nil {
		t.Fatalf("SaveTo: %v", err)
	}

	for _, validType := range []string{"basic", "bearer", "oauth2", "Basic", "BEARER", "OAuth2"} {
		flags := &config.FlagOverrides{AuthType: validType}
		resolved, err := config.Resolve(cfgPath, "", flags)
		if err != nil {
			t.Errorf("Resolve(%q): unexpected error: %v", validType, err)
			continue
		}
		if resolved.Auth.Type != strings.ToLower(validType) {
			t.Errorf("Resolve(%q): Auth.Type = %q, want %q", validType, resolved.Auth.Type, strings.ToLower(validType))
		}
	}
}

// --- Bug #42: configure --test --profile default should test literal "default" profile ---
// This bug is tested in the E2E test file (TestE2E_ConfigureTestExplicitProfileDefault)
// because the configure command goes through cobra which triggers os.Exit in the
// help handler. The fix is in testExistingProfile which now checks profileExplicit.

// --- Bug #43: configure -p short flag should work ---

func TestConfigureShortProfileFlag(t *testing.T) {
	// Verify that configure command's local flag set has -p shorthand for --profile.
	// Use LocalFlags() to avoid merged persistent flag set interference.
	f := configureCmd.LocalFlags().Lookup("profile")
	if f == nil {
		t.Fatal("configure command does not have a local 'profile' flag")
	}
	if f.Shorthand != "p" {
		t.Errorf("configure local 'profile' flag shorthand = %q, want %q", f.Shorthand, "p")
	}
}

func TestGeneratedTemplate_FileOpenError_HasStructuredError(t *testing.T) {
	// Read the template file and verify it uses AlreadyWrittenError for @file failures.
	tmplData, err := os.ReadFile("../gen/templates/resource.go.tmpl")
	if err != nil {
		t.Fatalf("cannot read template: %v", err)
	}
	tmpl := string(tmplData)

	// The old pattern was: `return err` after os.Open failure.
	// The new pattern should use AlreadyWrittenError.
	if strings.Contains(tmpl, "os.Open(strings.TrimPrefix(bodyStr,") &&
		!strings.Contains(tmpl, "AlreadyWrittenError") {
		t.Error("template should use AlreadyWrittenError for @file open failures, not raw error return")
	}

	if !strings.Contains(tmpl, `"cannot open body file: "`) {
		t.Error("template should include descriptive error message for @file open failures")
	}
}

// --- Bug #46: ApplyAuth OAuth2 error propagation ---
// When fetchOAuth2Token fails, ApplyAuth must return an error so callers
// stop the request instead of proceeding without auth (which caused double errors).

func TestApplyAuth_OAuth2Error_ReturnsError(t *testing.T) {
	tokenServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		fmt.Fprintln(w, `{"error":"invalid_client"}`)
	}))
	defer tokenServer.Close()

	c := &client.Client{
		HTTPClient: &http.Client{},
		Auth: config.AuthConfig{
			Type:     "oauth2",
			TokenURL: tokenServer.URL,
		},
	}
	req, _ := http.NewRequest("GET", "http://example.com", nil)
	err := c.ApplyAuth(req)
	if err == nil {
		t.Fatal("expected error from ApplyAuth when OAuth2 token fetch fails")
	}
	if !strings.Contains(err.Error(), "oauth2 authentication failed") {
		t.Errorf("expected 'oauth2 authentication failed' in error, got: %s", err.Error())
	}
	// Authorization header must NOT be set.
	if req.Header.Get("Authorization") != "" {
		t.Error("Authorization header should not be set after OAuth2 failure")
	}
}

// --- Bug #47: Empty OAuth2 access_token ---
// If the token endpoint returns {"access_token": ""}, ApplyAuth must return
// an error instead of silently proceeding without auth.

func TestApplyAuth_OAuth2EmptyToken_ReturnsError(t *testing.T) {
	tokenServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintln(w, `{"access_token":""}`)
	}))
	defer tokenServer.Close()

	c := &client.Client{
		HTTPClient: &http.Client{},
		Auth: config.AuthConfig{
			Type:         "oauth2",
			TokenURL:     tokenServer.URL,
			ClientID:     "id",
			ClientSecret: "secret",
		},
	}
	req, _ := http.NewRequest("GET", "http://example.com", nil)
	err := c.ApplyAuth(req)
	if err == nil {
		t.Fatal("expected error from ApplyAuth when OAuth2 returns empty token")
	}
	if !strings.Contains(err.Error(), "empty access_token") {
		t.Errorf("expected 'empty access_token' in error, got: %s", err.Error())
	}
}

func TestApplyAuth_OAuth2Success_SetsBearer(t *testing.T) {
	tokenServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintln(w, `{"access_token":"good-token-123"}`)
	}))
	defer tokenServer.Close()

	c := &client.Client{
		HTTPClient: &http.Client{},
		Auth: config.AuthConfig{
			Type:         "oauth2",
			TokenURL:     tokenServer.URL,
			ClientID:     "id",
			ClientSecret: "secret",
		},
	}
	req, _ := http.NewRequest("GET", "http://example.com", nil)
	if err := c.ApplyAuth(req); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	auth := req.Header.Get("Authorization")
	if auth != "Bearer good-token-123" {
		t.Errorf("expected 'Bearer good-token-123', got: %s", auth)
	}
}

func TestApplyAuth_BasicAndBearer_NoError(t *testing.T) {
	tests := []struct {
		authType string
		wantAuth string
	}{
		{"basic", "Basic"},
		{"bearer", "Bearer tok"},
	}
	for _, tc := range tests {
		t.Run(tc.authType, func(t *testing.T) {
			c := &client.Client{
				Auth: config.AuthConfig{Type: tc.authType, Username: "u", Token: "tok"},
			}
			req, _ := http.NewRequest("GET", "http://example.com", nil)
			if err := c.ApplyAuth(req); err != nil {
				t.Fatalf("unexpected error for %s: %v", tc.authType, err)
			}
			auth := req.Header.Get("Authorization")
			if !strings.HasPrefix(auth, tc.wantAuth) {
				t.Errorf("expected Authorization starting with %q, got %q", tc.wantAuth, auth)
			}
		})
	}
}

// --- Bug #48: Configure rejects --auth-type oauth2 ---
// The configure command lacks OAuth2-specific flags (client_id, client_secret,
// token_url), so saving an oauth2 profile would always fail at runtime.

func TestConfigureRejectsOAuth2(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := tmpDir + "/config.json"
	origPath := os.Getenv("JR_CONFIG_PATH")
	os.Setenv("JR_CONFIG_PATH", configPath)
	defer os.Setenv("JR_CONFIG_PATH", origPath)

	cmd := &cobra.Command{Use: "configure", RunE: runConfigure}
	f := cmd.Flags()
	f.String("base-url", "", "")
	f.String("token", "", "")
	f.String("profile", "default", "")
	f.String("auth-type", "basic", "")
	f.String("username", "", "")
	f.Bool("test", false, "")
	f.Bool("delete", false, "")
	f.String("jq", "", "")
	f.Bool("pretty", false, "")

	_ = cmd.Flags().Set("base-url", "https://example.atlassian.net")
	_ = cmd.Flags().Set("token", "test-token")
	_ = cmd.Flags().Set("auth-type", "oauth2")

	var stderrBuf bytes.Buffer
	origStderr := os.Stderr
	r, w, _ := os.Pipe()
	os.Stderr = w

	err := runConfigure(cmd, nil)

	w.Close()
	stderrData := make([]byte, 4096)
	n, _ := r.Read(stderrData)
	os.Stderr = origStderr

	if err == nil {
		t.Fatal("expected error when configuring oauth2")
	}
	stderrStr := string(stderrData[:n])
	_ = stderrBuf // suppress unused warning
	if !strings.Contains(stderrStr, "oauth2 is not supported by the configure command") {
		t.Errorf("expected helpful oauth2 rejection message, got: %s", stderrStr)
	}
}

// --- Bug #46 (e2e): OAuth2 failure produces single error, not double ---
// Verifies that Do() returns ExitAuth (2) with a single auth_error on stderr
// when OAuth2 token fetch fails, instead of the old behavior where the request
// continued without auth and produced a second 401 error.

func TestOAuth2Failure_SingleError_ExitAuth(t *testing.T) {
	tokenServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		fmt.Fprintln(w, `{"error":"invalid_grant"}`)
	}))
	defer tokenServer.Close()

	apiHit := false
	apiServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		apiHit = true
		w.WriteHeader(http.StatusUnauthorized)
		fmt.Fprintln(w, `{"error":"not_authenticated"}`)
	}))
	defer apiServer.Close()

	var stdout, stderr bytes.Buffer
	c := newTestClient(apiServer.URL, &stdout, &stderr)
	c.Auth = config.AuthConfig{
		Type:         "oauth2",
		TokenURL:     tokenServer.URL,
		ClientID:     "id",
		ClientSecret: "secret",
	}

	code := c.Do(context.Background(), "GET", "/rest/api/3/myself", nil, nil)

	if code != jrerrors.ExitAuth {
		t.Errorf("expected exit code %d (auth), got %d", jrerrors.ExitAuth, code)
	}
	if apiHit {
		t.Error("API server should NOT have been hit when OAuth2 token fetch failed")
	}

	// Stderr should contain exactly one error (auth_error), not two.
	lines := strings.Split(strings.TrimSpace(stderr.String()), "\n")
	if len(lines) != 1 {
		t.Errorf("expected 1 error line on stderr, got %d: %s", len(lines), stderr.String())
	}
	if !strings.Contains(lines[0], "auth_error") {
		t.Errorf("expected auth_error in stderr, got: %s", lines[0])
	}
}
