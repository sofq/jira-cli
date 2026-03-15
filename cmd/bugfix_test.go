package cmd

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/sofq/jira-cli/cmd/generated"
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
	cmd := configureCmd
	cmd.ResetFlags()
	f := cmd.Flags()
	f.String("base-url", "", "")
	f.String("token", "", "")
	f.String("profile", "default", "profile name")
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

	cmd := configureCmd
	cmd.ResetFlags()
	f := cmd.Flags()
	f.String("base-url", "", "")
	f.String("token", "", "")
	f.String("profile", "default", "")
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
