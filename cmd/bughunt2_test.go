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

// ============================================================
// Bug #68: raw --dry-run should not require --body for POST/PUT/PATCH
// (generated commands allow dry-run without body, raw should too)
// ============================================================

func TestRawCmd_DryRunPostWithoutBody(t *testing.T) {
	var stdout, stderr bytes.Buffer
	c := newTestClient("http://should-not-be-called", &stdout, &stderr)
	c.DryRun = true

	ctx := client.NewContext(t.Context(), c)
	cmd := &cobra.Command{Use: "raw"}
	cmd.Flags().String("body", "", "")
	cmd.Flags().StringArray("query", nil, "")
	cmd.Flags().String("fields", "", "")
	cmd.SetContext(ctx)

	err := runRaw(cmd, []string{"POST", "/rest/api/3/issue"})
	if err != nil {
		t.Fatalf("expected no error for dry-run POST without body, got: %v", err)
	}

	output := stdout.String()
	if !strings.Contains(output, `"method":"POST"`) {
		t.Errorf("expected method in dry-run output, got: %s", output)
	}
	if !strings.Contains(output, `/rest/api/3/issue`) {
		t.Errorf("expected path in dry-run output, got: %s", output)
	}
}

func TestRawCmd_DryRunPutWithoutBody(t *testing.T) {
	var stdout, stderr bytes.Buffer
	c := newTestClient("http://should-not-be-called", &stdout, &stderr)
	c.DryRun = true

	ctx := client.NewContext(t.Context(), c)
	cmd := &cobra.Command{Use: "raw"}
	cmd.Flags().String("body", "", "")
	cmd.Flags().StringArray("query", nil, "")
	cmd.Flags().String("fields", "", "")
	cmd.SetContext(ctx)

	err := runRaw(cmd, []string{"PUT", "/rest/api/3/issue/TEST-1/assignee"})
	if err != nil {
		t.Fatalf("expected no error for dry-run PUT without body, got: %v", err)
	}
}

func TestRawCmd_DryRunPatchWithoutBody(t *testing.T) {
	var stdout, stderr bytes.Buffer
	c := newTestClient("http://should-not-be-called", &stdout, &stderr)
	c.DryRun = true

	ctx := client.NewContext(t.Context(), c)
	cmd := &cobra.Command{Use: "raw"}
	cmd.Flags().String("body", "", "")
	cmd.Flags().StringArray("query", nil, "")
	cmd.Flags().String("fields", "", "")
	cmd.SetContext(ctx)

	err := runRaw(cmd, []string{"PATCH", "/rest/api/3/issue/TEST-1"})
	if err != nil {
		t.Fatalf("expected no error for dry-run PATCH without body, got: %v", err)
	}
}

// Verify non-dry-run POST without body still fails.
func TestRawCmd_NonDryRunPostWithoutBody(t *testing.T) {
	var stdout, stderr bytes.Buffer
	c := newTestClient("http://localhost", &stdout, &stderr)

	ctx := client.NewContext(t.Context(), c)
	cmd := &cobra.Command{Use: "raw"}
	cmd.Flags().String("body", "", "")
	cmd.Flags().StringArray("query", nil, "")
	cmd.Flags().String("fields", "", "")
	cmd.SetContext(ctx)

	err := runRaw(cmd, []string{"POST", "/test"})
	if err == nil {
		t.Fatal("expected error for non-dry-run POST without body")
	}
	aw, ok := err.(*errAlreadyWritten)
	if !ok {
		t.Fatalf("expected errAlreadyWritten, got %T", err)
	}
	if aw.code != jrerrors.ExitValidation {
		t.Errorf("expected exit %d, got %d", jrerrors.ExitValidation, aw.code)
	}
}

// ============================================================
// Bug #69: isLastPage infinite loop when total=0 but values non-empty
// ============================================================

func TestPagination_TotalZeroWithValues(t *testing.T) {
	// Server returns total=0, no isLast field, but has values.
	// Without the fix, pagination would loop infinitely.
	callCount := 0
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		w.Header().Set("Content-Type", "application/json")
		if callCount == 1 {
			fmt.Fprintln(w, `{"startAt":0,"maxResults":2,"total":0,"values":[{"id":"1"},{"id":"2"}]}`)
		} else {
			// Should never be called — pagination should stop after first page.
			fmt.Fprintln(w, `{"startAt":2,"maxResults":2,"total":0,"values":[]}`)
		}
	}))
	defer ts.Close()

	var stdout, stderr bytes.Buffer
	c := newTestClient(ts.URL, &stdout, &stderr)

	code := c.Do(t.Context(), "GET", "/test", nil, nil)
	if code != jrerrors.ExitOK {
		t.Fatalf("expected exit 0, got %d; stderr=%s", code, stderr.String())
	}

	// Should only have fetched one page.
	if callCount != 1 {
		t.Errorf("expected 1 HTTP call (single page), got %d — infinite pagination loop!", callCount)
	}

	// Output should include the two values from the first (only) page.
	output := stdout.String()
	if !strings.Contains(output, `"id":"1"`) || !strings.Contains(output, `"id":"2"`) {
		t.Errorf("expected values in output, got: %s", output)
	}
}

func TestPagination_TotalZeroEmptyValues(t *testing.T) {
	// Server returns total=0, values=[] — should produce an empty result.
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintln(w, `{"startAt":0,"maxResults":50,"total":0,"values":[]}`)
	}))
	defer ts.Close()

	var stdout, stderr bytes.Buffer
	c := newTestClient(ts.URL, &stdout, &stderr)

	code := c.Do(t.Context(), "GET", "/test", nil, nil)
	if code != jrerrors.ExitOK {
		t.Fatalf("expected exit 0, got %d; stderr=%s", code, stderr.String())
	}
}

// ============================================================
// Bug #70: buildBatchResult loses structured errors for multi-line stderr
// ============================================================

func TestBuildBatchResult_MultiLineErrors(t *testing.T) {
	var stdoutBuf, stderrBuf strings.Builder
	// Simulate two separate error JSON lines written to stderr.
	stderrBuf.WriteString(`{"error_type":"validation_error","message":"first error"}` + "\n")
	stderrBuf.WriteString(`{"error_type":"validation_error","message":"second error"}` + "\n")

	result := buildBatchResult(0, jrerrors.ExitValidation, &stdoutBuf, &stderrBuf, false)

	if result.ExitCode != jrerrors.ExitValidation {
		t.Errorf("expected exit %d, got %d", jrerrors.ExitValidation, result.ExitCode)
	}
	if result.Error == nil {
		t.Fatal("expected non-nil error in result")
	}

	// The error should be valid JSON (an array of the two error objects).
	if !json.Valid(result.Error) {
		t.Fatalf("error is not valid JSON: %s", string(result.Error))
	}

	// Parse and verify it's an array.
	var parsed interface{}
	if err := json.Unmarshal(result.Error, &parsed); err != nil {
		t.Fatal(err)
	}
	arr, ok := parsed.([]interface{})
	if !ok {
		// Single JSON line should also be fine.
		t.Logf("result was not an array: %s", string(result.Error))
		return
	}
	if len(arr) != 2 {
		t.Errorf("expected 2 error objects, got %d: %s", len(arr), string(result.Error))
	}
}

func TestBuildBatchResult_SingleLineError(t *testing.T) {
	var stdoutBuf, stderrBuf strings.Builder
	stderrBuf.WriteString(`{"error_type":"not_found","message":"issue not found"}` + "\n")

	result := buildBatchResult(0, jrerrors.ExitNotFound, &stdoutBuf, &stderrBuf, false)

	if !json.Valid(result.Error) {
		t.Fatalf("error is not valid JSON: %s", string(result.Error))
	}

	// Single line should be preserved as-is.
	var parsed map[string]interface{}
	if err := json.Unmarshal(result.Error, &parsed); err != nil {
		t.Fatalf("expected single JSON object, got: %s", string(result.Error))
	}
	if parsed["error_type"] != "not_found" {
		t.Errorf("expected error_type=not_found, got: %v", parsed["error_type"])
	}
}

func TestBuildBatchResult_VerboseMultiLineErrors(t *testing.T) {
	var stdoutBuf, stderrBuf strings.Builder
	// Simulate verbose request/response logs mixed with an error.
	stderrBuf.WriteString(`{"type":"request","method":"GET","url":"http://example.com/test"}` + "\n")
	stderrBuf.WriteString(`{"type":"response","status":404}` + "\n")
	stderrBuf.WriteString(`{"error_type":"not_found","message":"not found"}` + "\n")

	result := buildBatchResult(0, jrerrors.ExitNotFound, &stdoutBuf, &stderrBuf, true)

	if result.Error == nil {
		t.Fatal("expected error in result")
	}
	if !json.Valid(result.Error) {
		t.Fatalf("error is not valid JSON: %s", string(result.Error))
	}

	// Should only contain the error line, not the verbose logs.
	var parsed map[string]interface{}
	if err := json.Unmarshal(result.Error, &parsed); err != nil {
		t.Fatalf("expected single error object: %s", string(result.Error))
	}
	if parsed["error_type"] != "not_found" {
		t.Errorf("expected error_type=not_found, got: %v", parsed["error_type"])
	}
}

// ============================================================
// Bug #71: fetchJSONWithBody missing TrimSpace on error body
// ============================================================

func TestFetchJSONWithBody_TrimSpaceOnError(t *testing.T) {
	// Server returns an error with trailing whitespace.
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		fmt.Fprintln(w, `{"errorMessages":["Issue not found"]}  `)
	}))
	defer ts.Close()

	var stdout, stderr bytes.Buffer
	c := newTestClient(ts.URL, &stdout, &stderr)

	_, code := FetchJSONWithBody(c, t.Context(), "GET", "/rest/api/3/issue/NOPE-1", nil)
	if code != jrerrors.ExitNotFound {
		t.Errorf("expected exit %d, got %d", jrerrors.ExitNotFound, code)
	}

	// The error JSON on stderr should be well-formed.
	errOutput := strings.TrimSpace(stderr.String())
	if !json.Valid([]byte(errOutput)) {
		t.Errorf("stderr not valid JSON: %s", errOutput)
	}
}

// ============================================================
// Bug #72: testConnection dead code for oauth2 removed
// ============================================================

func TestTestConnection_NoOAuth2Branch(t *testing.T) {
	// After removing the dead oauth2 branch, testConnection with authType="oauth2"
	// should fall through to basic auth (default case), not return a special error.
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintln(w, `{"accountId":"test"}`)
	}))
	defer ts.Close()

	err := ExportTestConnection(ts.URL, "oauth2", "user", "token")
	if err != nil {
		t.Errorf("expected testConnection to succeed with oauth2 falling through to basic, got: %v", err)
	}
}

// ============================================================
// Additional edge case: OAuth2 with empty access_token
// ============================================================

func TestFetchOAuth2Token_EmptyAccessToken(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintln(w, `{"access_token":""}`)
	}))
	defer ts.Close()

	c := &client.Client{
		Auth: config.AuthConfig{
			Type:         "oauth2",
			ClientID:     "id",
			ClientSecret: "secret",
			TokenURL:     ts.URL + "/token",
		},
		HTTPClient: &http.Client{},
	}

	req, _ := http.NewRequest("GET", "http://example.com", nil)
	err := c.ApplyAuth(req)
	if err == nil {
		t.Fatal("expected error for empty access_token")
	}
	if !strings.Contains(err.Error(), "empty") {
		t.Errorf("expected 'empty' in error, got: %v", err)
	}
}

// ============================================================
// Additional edge case: raw --dry-run with body provided
// ============================================================

// ============================================================
// Bug #73: batch global --fields overwrites per-op fields args
// ============================================================

func TestBatch_PerOpFieldsOverrideGlobal(t *testing.T) {
	// When a batch op specifies "fields" in its args, that value should
	// take precedence over the global --fields flag, not be overwritten.
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fieldsParam := r.URL.Query().Get("fields")
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, `{"fields_received":"%s"}`, fieldsParam)
	}))
	defer ts.Close()

	var stdout, stderr bytes.Buffer
	c := newTestClient(ts.URL, &stdout, &stderr)
	c.Fields = "key" // Simulates global --fields key

	ctx := client.NewContext(t.Context(), c)
	cmd := &cobra.Command{Use: "batch"}
	cmd.Flags().String("input", "", "")
	cmd.SetContext(ctx)

	// Build a batch op that specifies its own "fields" arg.
	bop := BatchOp{
		Command: "issue get",
		Args:    map[string]string{"issueIdOrKey": "TEST-1", "fields": "summary,status"},
	}

	allOps := generated.AllSchemaOps()
	allOps = append(allOps, HandWrittenSchemaOps()...)
	opMap := make(map[string]generated.SchemaOp, len(allOps))
	for _, op := range allOps {
		opMap[op.Resource+" "+op.Verb] = op
	}

	result := executeBatchOp(cmd, c, 0, bop, opMap)

	if result.ExitCode != jrerrors.ExitOK {
		t.Fatalf("expected exit 0, got %d; error=%s", result.ExitCode, string(result.Error))
	}

	// Parse the data to check which fields value was sent.
	var data map[string]string
	if err := json.Unmarshal(result.Data, &data); err != nil {
		t.Fatalf("failed to parse result data: %v (raw: %s)", err, string(result.Data))
	}

	// The per-op "summary,status" should win over global "key".
	if data["fields_received"] != "summary,status" {
		t.Errorf("expected per-op fields 'summary,status', got '%s' — global --fields overwrote per-op fields", data["fields_received"])
	}
}

func TestBatch_GlobalFieldsAppliedWhenNoPerOpFields(t *testing.T) {
	// When a batch op does NOT specify "fields" in its args, the global --fields
	// should still be applied.
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fieldsParam := r.URL.Query().Get("fields")
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, `{"fields_received":"%s"}`, fieldsParam)
	}))
	defer ts.Close()

	var stdout, stderr bytes.Buffer
	c := newTestClient(ts.URL, &stdout, &stderr)
	c.Fields = "key" // Simulates global --fields key

	ctx := client.NewContext(t.Context(), c)
	cmd := &cobra.Command{Use: "batch"}
	cmd.Flags().String("input", "", "")
	cmd.SetContext(ctx)

	// Batch op WITHOUT "fields" in args.
	bop := BatchOp{
		Command: "issue get",
		Args:    map[string]string{"issueIdOrKey": "TEST-1"},
	}

	allOps := generated.AllSchemaOps()
	allOps = append(allOps, HandWrittenSchemaOps()...)
	opMap := make(map[string]generated.SchemaOp, len(allOps))
	for _, op := range allOps {
		opMap[op.Resource+" "+op.Verb] = op
	}

	result := executeBatchOp(cmd, c, 0, bop, opMap)

	if result.ExitCode != jrerrors.ExitOK {
		t.Fatalf("expected exit 0, got %d; error=%s", result.ExitCode, string(result.Error))
	}

	var data map[string]string
	if err := json.Unmarshal(result.Data, &data); err != nil {
		t.Fatalf("failed to parse result data: %v (raw: %s)", err, string(result.Data))
	}

	// Global "key" should be applied since no per-op fields.
	if data["fields_received"] != "key" {
		t.Errorf("expected global fields 'key', got '%s'", data["fields_received"])
	}
}

func TestRawCmd_DryRunPostWithBody(t *testing.T) {
	var stdout, stderr bytes.Buffer
	c := newTestClient("http://should-not-be-called", &stdout, &stderr)
	c.DryRun = true

	ctx := client.NewContext(t.Context(), c)
	cmd := &cobra.Command{Use: "raw"}
	cmd.Flags().String("body", "", "")
	cmd.Flags().StringArray("query", nil, "")
	cmd.Flags().String("fields", "", "")
	_ = cmd.Flags().Set("body", `{"fields":{"summary":"Test"}}`)
	cmd.SetContext(ctx)

	err := runRaw(cmd, []string{"POST", "/rest/api/3/issue"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	output := stdout.String()
	if !strings.Contains(output, "summary") {
		t.Errorf("expected body in dry-run output, got: %s", output)
	}
}
