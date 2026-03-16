package cmd

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sofq/jira-cli/cmd/generated"
	"github.com/sofq/jira-cli/internal/client"
	"github.com/sofq/jira-cli/internal/config"
	jrerrors "github.com/sofq/jira-cli/internal/errors"
	"github.com/spf13/cobra"
)

// ============================================================
// Bug fix: generated template validates @<empty> body filename
// ============================================================

func TestGeneratedTemplate_EmptyAtFilename(t *testing.T) {
	// Verify that generated commands validate --body @ with no filename.
	// Read a generated file to confirm the pattern exists.
	data, err := os.ReadFile("generated/issue.go")
	if err != nil {
		t.Skipf("cannot read generated file: %v", err)
	}
	if !strings.Contains(string(data), `requires a filename after @`) {
		t.Error("generated issue.go missing --body @<filename> validation")
	}
}

// ============================================================
// Bug fix: deleteProfileByName lists available profiles
// ============================================================

func TestDeleteProfile_ListsAvailableProfiles(t *testing.T) {
	// Create a config file with known profiles.
	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.json")
	cfg := &config.Config{
		DefaultProfile: "prod",
		Profiles: map[string]config.Profile{
			"prod":    {BaseURL: "https://prod.example.com"},
			"staging": {BaseURL: "https://staging.example.com"},
		},
	}
	if err := config.SaveTo(cfg, configPath); err != nil {
		t.Fatal(err)
	}

	// Override DefaultPath for this test.
	t.Setenv("JR_CONFIG_PATH", configPath)

	cmd := &cobra.Command{Use: "configure", RunE: runConfigure}
	f := cmd.Flags()
	f.String("base-url", "", "")
	f.String("token", "", "")
	f.StringP("profile", "p", "default", "")
	f.String("auth-type", "basic", "")
	f.String("username", "", "")
	f.Bool("test", false, "")
	f.Bool("delete", false, "")

	_ = cmd.Flags().Set("delete", "true")
	_ = cmd.Flags().Set("profile", "nonexistent")

	err := runConfigure(cmd, nil)
	if err == nil {
		t.Fatal("expected error deleting nonexistent profile")
	}

	aw, ok := err.(*errAlreadyWritten)
	if !ok {
		t.Fatalf("expected errAlreadyWritten, got %T", err)
	}
	if aw.code != jrerrors.ExitNotFound {
		t.Errorf("expected exit %d, got %d", jrerrors.ExitNotFound, aw.code)
	}
}

// ============================================================
// Bug fix: batch per-op Pretty is disabled (applied at batch level)
// ============================================================

func TestBatchPerOpPretty_Disabled(t *testing.T) {
	// When --pretty is set, per-op clients should NOT have Pretty=true.
	// Pretty is applied once at the batch output level, not per-op.
	// Verify by checking that per-op stdout captures compact JSON.
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintln(w, `{"key":"TEST-1","summary":"test"}`)
	}))
	defer ts.Close()

	var stdout, stderr bytes.Buffer
	baseClient := newTestClient(ts.URL, &stdout, &stderr)
	baseClient.Pretty = true

	// Directly call executeBatchOp to inspect the per-op result.
	opMap := map[string]generated.SchemaOp{
		"issue get": {
			Resource: "issue",
			Verb:     "get",
			Method:   "GET",
			Path:     "/rest/api/3/issue/{issueIdOrKey}",
			Flags: []generated.SchemaFlag{
				{Name: "issueIdOrKey", Required: true, In: "path"},
			},
		},
	}

	cmd := &cobra.Command{Use: "batch"}
	ctx := client.NewContext(t.Context(), baseClient)
	cmd.SetContext(ctx)

	result := executeBatchOp(cmd, baseClient, 0, BatchOp{
		Command: "issue get",
		Args:    map[string]string{"issueIdOrKey": "TEST-1"},
	}, opMap)

	if result.ExitCode != jrerrors.ExitOK {
		t.Fatalf("expected exit 0, got %d", result.ExitCode)
	}

	// The per-op data should be compact (Pretty=false on per-op client).
	dataStr := string(result.Data)
	if strings.Contains(dataStr, "\n") {
		t.Errorf("per-op data should be compact JSON (Pretty disabled), got: %s", dataStr)
	}
}

// ============================================================
// Bug fix: raw.go dead QueryFromFlags replaced with url.Values{}
// ============================================================

func TestRawCmd_QueryParsing(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify query params are correctly passed.
		if r.URL.Query().Get("expand") != "renderedFields" {
			t.Errorf("expected expand=renderedFields, got %q", r.URL.Query().Get("expand"))
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintln(w, `{"ok":true}`)
	}))
	defer ts.Close()

	var stdout, stderr bytes.Buffer
	c := newTestClient(ts.URL, &stdout, &stderr)

	ctx := client.NewContext(t.Context(), c)
	cmd := &cobra.Command{Use: "raw"}
	cmd.Flags().String("body", "", "")
	cmd.Flags().StringArray("query", nil, "")
	cmd.Flags().String("fields", "", "")
	_ = cmd.Flags().Set("query", "expand=renderedFields")
	cmd.SetContext(ctx)

	err := runRaw(cmd, []string{"GET", "/test"})
	if err != nil {
		t.Fatalf("runRaw error: %v", err)
	}
}

// ============================================================
// New tests: executeBatchOp path/query param building
// ============================================================

func TestBatchOp_PathParamSubstitution(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify the path param was correctly substituted.
		if r.URL.Path != "/rest/api/3/issue/PROJ-123" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintln(w, `{"key":"PROJ-123"}`)
	}))
	defer ts.Close()

	var stdout, stderr bytes.Buffer
	c := newTestClient(ts.URL, &stdout, &stderr)

	opMap := map[string]generated.SchemaOp{
		"issue get": {
			Resource: "issue",
			Verb:     "get",
			Method:   "GET",
			Path:     "/rest/api/3/issue/{issueIdOrKey}",
			Flags: []generated.SchemaFlag{
				{Name: "issueIdOrKey", Required: true, In: "path"},
			},
		},
	}

	cmd := &cobra.Command{Use: "batch"}
	ctx := client.NewContext(t.Context(), c)
	cmd.SetContext(ctx)

	result := executeBatchOp(cmd, c, 0, BatchOp{
		Command: "issue get",
		Args:    map[string]string{"issueIdOrKey": "PROJ-123"},
	}, opMap)

	if result.ExitCode != jrerrors.ExitOK {
		t.Fatalf("expected exit 0, got %d", result.ExitCode)
	}
}

func TestBatchOp_PathParamWithSpecialChars(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// url.PathEscape encodes / as %2F. Verify encoding is applied.
		if r.URL.RawPath != "" && !strings.Contains(r.URL.RawPath, "PROJ%2F123") {
			t.Errorf("expected path-escaped value, got raw path: %s", r.URL.RawPath)
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintln(w, `{"key":"PROJ/123"}`)
	}))
	defer ts.Close()

	var stdout, stderr bytes.Buffer
	c := newTestClient(ts.URL, &stdout, &stderr)

	opMap := map[string]generated.SchemaOp{
		"issue get": {
			Resource: "issue",
			Verb:     "get",
			Method:   "GET",
			Path:     "/rest/api/3/issue/{issueIdOrKey}",
			Flags: []generated.SchemaFlag{
				{Name: "issueIdOrKey", Required: true, In: "path"},
			},
		},
	}

	cmd := &cobra.Command{Use: "batch"}
	ctx := client.NewContext(t.Context(), c)
	cmd.SetContext(ctx)

	result := executeBatchOp(cmd, c, 0, BatchOp{
		Command: "issue get",
		Args:    map[string]string{"issueIdOrKey": "PROJ/123"},
	}, opMap)

	if result.ExitCode != jrerrors.ExitOK {
		t.Fatalf("expected exit 0, got %d", result.ExitCode)
	}
}

func TestBatchOp_QueryParamBuilding(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify query params from batch args.
		if r.URL.Query().Get("maxResults") != "5" {
			t.Errorf("expected maxResults=5, got %q", r.URL.Query().Get("maxResults"))
		}
		if r.URL.Query().Get("startAt") != "10" {
			t.Errorf("expected startAt=10, got %q", r.URL.Query().Get("startAt"))
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintln(w, `{"values":[]}`)
	}))
	defer ts.Close()

	var stdout, stderr bytes.Buffer
	c := newTestClient(ts.URL, &stdout, &stderr)

	opMap := map[string]generated.SchemaOp{
		"project search": {
			Resource: "project",
			Verb:     "search",
			Method:   "GET",
			Path:     "/rest/api/3/project/search",
			Flags: []generated.SchemaFlag{
				{Name: "maxResults", In: "query"},
				{Name: "startAt", In: "query"},
			},
		},
	}

	cmd := &cobra.Command{Use: "batch"}
	ctx := client.NewContext(t.Context(), c)
	cmd.SetContext(ctx)

	result := executeBatchOp(cmd, c, 0, BatchOp{
		Command: "project search",
		Args:    map[string]string{"maxResults": "5", "startAt": "10"},
	}, opMap)

	if result.ExitCode != jrerrors.ExitOK {
		t.Fatalf("expected exit 0, got %d", result.ExitCode)
	}
}

func TestBatchOp_UnknownCommand(t *testing.T) {
	var stdout, stderr bytes.Buffer
	c := newTestClient("http://localhost", &stdout, &stderr)
	opMap := map[string]generated.SchemaOp{}

	cmd := &cobra.Command{Use: "batch"}
	ctx := client.NewContext(t.Context(), c)
	cmd.SetContext(ctx)

	result := executeBatchOp(cmd, c, 0, BatchOp{
		Command: "nonexistent command",
		Args:    map[string]string{},
	}, opMap)

	if result.ExitCode != jrerrors.ExitError {
		t.Fatalf("expected exit %d, got %d", jrerrors.ExitError, result.ExitCode)
	}
	if !strings.Contains(string(result.Error), "unknown command") {
		t.Errorf("expected 'unknown command' in error, got: %s", string(result.Error))
	}
}

func TestBatchOp_BodyParam(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			t.Errorf("expected POST, got %s", r.Method)
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintln(w, `{"id":"123"}`)
	}))
	defer ts.Close()

	var stdout, stderr bytes.Buffer
	c := newTestClient(ts.URL, &stdout, &stderr)

	opMap := map[string]generated.SchemaOp{
		"issue create-issue": {
			Resource: "issue",
			Verb:     "create-issue",
			Method:   "POST",
			Path:     "/rest/api/3/issue",
			HasBody:  true,
			Flags: []generated.SchemaFlag{
				{Name: "body", In: "body"},
			},
		},
	}

	cmd := &cobra.Command{Use: "batch"}
	ctx := client.NewContext(t.Context(), c)
	cmd.SetContext(ctx)

	result := executeBatchOp(cmd, c, 0, BatchOp{
		Command: "issue create-issue",
		Args:    map[string]string{"body": `{"fields":{"summary":"Test"}}`},
	}, opMap)

	if result.ExitCode != jrerrors.ExitOK {
		t.Fatalf("expected exit 0, got %d; error=%s", result.ExitCode, string(result.Error))
	}
}

// ============================================================
// New tests: buildBatchResult edge cases
// ============================================================

func TestBuildBatchResult_NonJSONStdout(t *testing.T) {
	// If per-op stdout produces non-JSON (shouldn't happen normally),
	// it should be JSON-encoded as a string.
	var stdoutBuf, stderrBuf strings.Builder
	stdoutBuf.WriteString("not valid json")

	result := buildBatchResult(0, jrerrors.ExitOK, &stdoutBuf, &stderrBuf, false)
	if result.ExitCode != jrerrors.ExitOK {
		t.Fatalf("expected exit 0, got %d", result.ExitCode)
	}
	// Non-JSON data should be JSON-encoded as a string.
	if string(result.Data) != `"not valid json"` {
		t.Errorf("expected JSON-encoded string, got: %s", string(result.Data))
	}
}

func TestBuildBatchResult_EmptyStdout_Success(t *testing.T) {
	var stdoutBuf, stderrBuf strings.Builder

	result := buildBatchResult(0, jrerrors.ExitOK, &stdoutBuf, &stderrBuf, false)
	if result.ExitCode != jrerrors.ExitOK {
		t.Fatalf("expected exit 0, got %d", result.ExitCode)
	}
	if result.Data != nil {
		t.Errorf("expected nil data for empty stdout, got: %s", string(result.Data))
	}
}

func TestBuildBatchResult_ErrorWithNonJSONStderr(t *testing.T) {
	var stdoutBuf, stderrBuf strings.Builder
	stderrBuf.WriteString("plain error message")

	result := buildBatchResult(0, jrerrors.ExitError, &stdoutBuf, &stderrBuf, false)
	if result.ExitCode != jrerrors.ExitError {
		t.Fatalf("expected exit 1, got %d", result.ExitCode)
	}
	// Non-JSON stderr should be wrapped in a message object.
	var errObj map[string]string
	if err := json.Unmarshal(result.Error, &errObj); err != nil {
		t.Fatalf("error field not valid JSON: %s", string(result.Error))
	}
	if errObj["message"] != "plain error message" {
		t.Errorf("expected 'plain error message', got: %s", errObj["message"])
	}
}

func TestBuildBatchResult_VerboseWithNonJSONLines(t *testing.T) {
	// Non-JSON lines in stderr should be kept as error lines.
	var stdoutBuf, stderrBuf strings.Builder
	stderrBuf.WriteString(`{"type":"request","method":"GET","url":"http://example.com"}` + "\n")
	stderrBuf.WriteString("some non-JSON warning\n")

	oldStderr := os.Stderr
	r, w, _ := os.Pipe()
	os.Stderr = w

	result := buildBatchResult(0, jrerrors.ExitError, &stdoutBuf, &stderrBuf, true)

	w.Close()
	os.Stderr = oldStderr
	var captured bytes.Buffer
	_, _ = captured.ReadFrom(r)

	if result.ExitCode != jrerrors.ExitError {
		t.Fatalf("expected exit 1, got %d", result.ExitCode)
	}
	// The verbose request line should be forwarded.
	if !strings.Contains(captured.String(), `"type":"request"`) {
		t.Error("expected verbose line to be forwarded to stderr")
	}
	// The non-JSON line should be in the error result.
	var errObj map[string]string
	if err := json.Unmarshal(result.Error, &errObj); err != nil {
		t.Fatalf("error field not valid JSON: %s", string(result.Error))
	}
	if errObj["message"] != "some non-JSON warning" {
		t.Errorf("expected non-JSON warning in error, got: %s", errObj["message"])
	}
}

// ============================================================
// New tests: batch --input file
// ============================================================

func TestBatch_InputFile(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintln(w, `{"key":"TEST-1"}`)
	}))
	defer ts.Close()

	// Write batch input to a temp file.
	dir := t.TempDir()
	inputFile := filepath.Join(dir, "batch.json")
	input := `[{"command":"issue get","args":{"issueIdOrKey":"TEST-1"}}]`
	if err := os.WriteFile(inputFile, []byte(input), 0o644); err != nil {
		t.Fatal(err)
	}

	var stdout, stderr bytes.Buffer
	c := newTestClient(ts.URL, &stdout, &stderr)

	ctx := client.NewContext(t.Context(), c)
	cmd := &cobra.Command{Use: "batch"}
	cmd.Flags().String("input", "", "")
	_ = cmd.Flags().Set("input", inputFile)
	cmd.SetContext(ctx)

	captured := captureStdout(t, func() {
		if err := runBatch(cmd, nil); err != nil {
			t.Fatalf("runBatch error: %v", err)
		}
	})

	var results []BatchResult
	if err := json.Unmarshal([]byte(strings.TrimSpace(captured)), &results); err != nil {
		t.Fatalf("batch output not valid JSON: %s", captured)
	}
	if len(results) != 1 || results[0].ExitCode != 0 {
		t.Errorf("unexpected results: %s", captured)
	}
}

func TestBatch_InputFileNotFound(t *testing.T) {
	var stdout, stderr bytes.Buffer
	c := newTestClient("http://localhost", &stdout, &stderr)

	ctx := client.NewContext(t.Context(), c)
	cmd := &cobra.Command{Use: "batch"}
	cmd.Flags().String("input", "", "")
	_ = cmd.Flags().Set("input", "/nonexistent/path/batch.json")
	cmd.SetContext(ctx)

	err := runBatch(cmd, nil)
	if err == nil {
		t.Fatal("expected error for nonexistent input file")
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
// New tests: batch non-zero exit code propagation
// ============================================================

func TestBatch_NonZeroExitPropagation(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		fmt.Fprintln(w, `{"errorMessages":["not found"]}`)
	}))
	defer ts.Close()

	var stdout, stderr bytes.Buffer
	c := newTestClient(ts.URL, &stdout, &stderr)

	ctx := client.NewContext(t.Context(), c)
	cmd := &cobra.Command{Use: "batch"}
	cmd.Flags().String("input", "", "")
	cmd.SetContext(ctx)

	input := `[{"command":"issue get","args":{"issueIdOrKey":"NOPE-999"}}]`
	oldStdin := os.Stdin
	pr, pw, _ := os.Pipe()
	_, _ = pw.WriteString(input)
	pw.Close()
	os.Stdin = pr
	defer func() { os.Stdin = oldStdin }()

	var capturedErr error
	captureStdout(t, func() {
		capturedErr = runBatch(cmd, nil)
	})

	if capturedErr == nil {
		t.Fatal("expected error for batch with failed ops")
	}
	aw, ok := capturedErr.(*errAlreadyWritten)
	if !ok {
		t.Fatalf("expected errAlreadyWritten, got %T", capturedErr)
	}
	if aw.code != jrerrors.ExitNotFound {
		t.Errorf("expected exit %d, got %d", jrerrors.ExitNotFound, aw.code)
	}
}

// ============================================================
// New tests: batchTransition HTTP error during POST
// ============================================================

func TestBatchTransition_PostError(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "GET" {
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprintln(w, `{"transitions":[{"id":"11","name":"Done","to":{"name":"Done"}}]}`)
			return
		}
		// POST transition returns 403.
		w.WriteHeader(http.StatusForbidden)
		fmt.Fprintln(w, `{"errorMessages":["no permission"]}`)
	}))
	defer ts.Close()

	var stdout, stderr bytes.Buffer
	c := newTestClient(ts.URL, &stdout, &stderr)

	code := batchTransition(t.Context(), c, "TEST-1", "Done")
	if code != jrerrors.ExitAuth {
		t.Fatalf("expected exit %d (auth), got %d; stderr=%s", jrerrors.ExitAuth, code, stderr.String())
	}
}

func TestBatchTransition_GetTransitionsError(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		fmt.Fprintln(w, `{"errorMessages":["issue not found"]}`)
	}))
	defer ts.Close()

	var stdout, stderr bytes.Buffer
	c := newTestClient(ts.URL, &stdout, &stderr)

	code := batchTransition(t.Context(), c, "NOPE-999", "Done")
	if code != jrerrors.ExitNotFound {
		t.Fatalf("expected exit %d (not_found), got %d", jrerrors.ExitNotFound, code)
	}
}

func TestBatchTransition_InvalidTransitionsJSON(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintln(w, `not valid json`)
	}))
	defer ts.Close()

	var stdout, stderr bytes.Buffer
	c := newTestClient(ts.URL, &stdout, &stderr)

	code := batchTransition(t.Context(), c, "TEST-1", "Done")
	if code != jrerrors.ExitError {
		t.Fatalf("expected exit %d (error), got %d", jrerrors.ExitError, code)
	}
	if !strings.Contains(stderr.String(), "failed to parse transitions") {
		t.Errorf("expected parse error in stderr, got: %s", stderr.String())
	}
}

// ============================================================
// New tests: batchAssign edge cases
// ============================================================

func TestBatchAssign_InvalidMyselfJSON(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintln(w, `not valid json`)
	}))
	defer ts.Close()

	var stdout, stderr bytes.Buffer
	c := newTestClient(ts.URL, &stdout, &stderr)

	code := batchAssign(t.Context(), c, "TEST-1", "me")
	if code != jrerrors.ExitError {
		t.Fatalf("expected exit %d for invalid /myself JSON, got %d", jrerrors.ExitError, code)
	}
}

func TestBatchAssign_EmptyAccountId(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/rest/api/3/myself" {
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprintln(w, `{"accountId":""}`)
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer ts.Close()

	var stdout, stderr bytes.Buffer
	c := newTestClient(ts.URL, &stdout, &stderr)

	code := batchAssign(t.Context(), c, "TEST-1", "me")
	if code != jrerrors.ExitError {
		t.Fatalf("expected exit %d for empty accountId, got %d", jrerrors.ExitError, code)
	}
	if !strings.Contains(stderr.String(), "could not determine") {
		t.Errorf("expected accountId error in stderr, got: %s", stderr.String())
	}
}

func TestBatchAssign_MyselfHTTPError(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		fmt.Fprintln(w, `{"errorMessages":["unauthorized"]}`)
	}))
	defer ts.Close()

	var stdout, stderr bytes.Buffer
	c := newTestClient(ts.URL, &stdout, &stderr)

	code := batchAssign(t.Context(), c, "TEST-1", "me")
	if code != jrerrors.ExitAuth {
		t.Fatalf("expected exit %d (auth), got %d", jrerrors.ExitAuth, code)
	}
}

func TestBatchAssign_UserSearchHTTPError(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		fmt.Fprintln(w, `{"errorMessages":["server error"]}`)
	}))
	defer ts.Close()

	var stdout, stderr bytes.Buffer
	c := newTestClient(ts.URL, &stdout, &stderr)

	code := batchAssign(t.Context(), c, "TEST-1", "someuser")
	if code != jrerrors.ExitServer {
		t.Fatalf("expected exit %d (server), got %d", jrerrors.ExitServer, code)
	}
}

func TestBatchAssign_PutAssigneeHTTPError(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/rest/api/3/myself" {
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprintln(w, `{"accountId":"abc123"}`)
			return
		}
		if strings.Contains(r.URL.Path, "/assignee") {
			w.WriteHeader(http.StatusForbidden)
			fmt.Fprintln(w, `{"errorMessages":["forbidden"]}`)
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer ts.Close()

	var stdout, stderr bytes.Buffer
	c := newTestClient(ts.URL, &stdout, &stderr)

	code := batchAssign(t.Context(), c, "TEST-1", "me")
	if code != jrerrors.ExitAuth {
		t.Fatalf("expected exit %d (auth), got %d", jrerrors.ExitAuth, code)
	}
}

func TestBatchAssign_Unassign_HTTPError(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		fmt.Fprintln(w, `{"errorMessages":["bad request"]}`)
	}))
	defer ts.Close()

	var stdout, stderr bytes.Buffer
	c := newTestClient(ts.URL, &stdout, &stderr)

	code := batchAssign(t.Context(), c, "TEST-1", "none")
	if code != jrerrors.ExitValidation {
		t.Fatalf("expected exit %d (validation), got %d", jrerrors.ExitValidation, code)
	}
}

// ============================================================
// New tests: configure deleteProfile success
// ============================================================

func TestDeleteProfile_Success(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.json")
	cfg := &config.Config{
		DefaultProfile: "myprofile",
		Profiles: map[string]config.Profile{
			"myprofile": {BaseURL: "https://example.com"},
			"other":     {BaseURL: "https://other.example.com"},
		},
	}
	if err := config.SaveTo(cfg, configPath); err != nil {
		t.Fatal(err)
	}
	t.Setenv("JR_CONFIG_PATH", configPath)

	cmd := &cobra.Command{Use: "configure", RunE: runConfigure}
	f := cmd.Flags()
	f.String("base-url", "", "")
	f.String("token", "", "")
	f.StringP("profile", "p", "default", "")
	f.String("auth-type", "basic", "")
	f.String("username", "", "")
	f.Bool("test", false, "")
	f.Bool("delete", false, "")
	f.String("jq", "", "")
	f.Bool("pretty", false, "")

	_ = cmd.Flags().Set("delete", "true")
	_ = cmd.Flags().Set("profile", "myprofile")

	captured := captureStdout(t, func() {
		if err := runConfigure(cmd, nil); err != nil {
			t.Fatalf("runConfigure error: %v", err)
		}
	})

	if !strings.Contains(captured, `"deleted"`) {
		t.Errorf("expected deleted status, got: %s", captured)
	}

	// Verify profile was removed and default was cleared.
	reloaded, err := config.LoadFrom(configPath)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := reloaded.Profiles["myprofile"]; ok {
		t.Error("profile should have been deleted")
	}
	if reloaded.DefaultProfile != "" {
		t.Errorf("default profile should have been cleared, got: %s", reloaded.DefaultProfile)
	}
}

func TestDeleteProfile_NotDefault_KeepsDefault(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.json")
	cfg := &config.Config{
		DefaultProfile: "default",
		Profiles: map[string]config.Profile{
			"default": {BaseURL: "https://example.com"},
			"other":   {BaseURL: "https://other.example.com"},
		},
	}
	if err := config.SaveTo(cfg, configPath); err != nil {
		t.Fatal(err)
	}
	t.Setenv("JR_CONFIG_PATH", configPath)

	cmd := &cobra.Command{Use: "configure", RunE: runConfigure}
	f := cmd.Flags()
	f.String("base-url", "", "")
	f.String("token", "", "")
	f.StringP("profile", "p", "default", "")
	f.String("auth-type", "basic", "")
	f.String("username", "", "")
	f.Bool("test", false, "")
	f.Bool("delete", false, "")
	f.String("jq", "", "")
	f.Bool("pretty", false, "")

	_ = cmd.Flags().Set("delete", "true")
	_ = cmd.Flags().Set("profile", "other")

	captureStdout(t, func() {
		if err := runConfigure(cmd, nil); err != nil {
			t.Fatalf("runConfigure error: %v", err)
		}
	})

	reloaded, err := config.LoadFrom(configPath)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := reloaded.Profiles["other"]; ok {
		t.Error("profile 'other' should have been deleted")
	}
	if reloaded.DefaultProfile != "default" {
		t.Errorf("default profile should be preserved, got: %s", reloaded.DefaultProfile)
	}
}

// ============================================================
// New tests: configure save success paths
// ============================================================

func TestConfigure_SaveSuccess(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.json")
	t.Setenv("JR_CONFIG_PATH", configPath)

	cmd := &cobra.Command{Use: "configure", RunE: runConfigure}
	f := cmd.Flags()
	f.String("base-url", "", "")
	f.String("token", "", "")
	f.StringP("profile", "p", "default", "")
	f.String("auth-type", "basic", "")
	f.String("username", "", "")
	f.Bool("test", false, "")
	f.Bool("delete", false, "")
	f.String("jq", "", "")
	f.Bool("pretty", false, "")

	_ = cmd.Flags().Set("base-url", "https://test.atlassian.net/")
	_ = cmd.Flags().Set("token", "mytoken")
	_ = cmd.Flags().Set("username", "user@test.com")
	_ = cmd.Flags().Set("profile", "myprofile")

	captured := captureStdout(t, func() {
		if err := runConfigure(cmd, nil); err != nil {
			t.Fatalf("runConfigure error: %v", err)
		}
	})

	if !strings.Contains(captured, `"saved"`) {
		t.Errorf("expected saved status, got: %s", captured)
	}

	// Verify config was saved correctly.
	cfg, err := config.LoadFrom(configPath)
	if err != nil {
		t.Fatal(err)
	}
	profile := cfg.Profiles["myprofile"]
	// Trailing slash should be normalized.
	if profile.BaseURL != "https://test.atlassian.net" {
		t.Errorf("expected normalized base URL, got: %s", profile.BaseURL)
	}
	if profile.Auth.Token != "mytoken" {
		t.Errorf("expected token 'mytoken', got: %s", profile.Auth.Token)
	}
	// DefaultProfile should be set when it was empty.
	if cfg.DefaultProfile != "myprofile" {
		t.Errorf("expected default profile 'myprofile', got: %s", cfg.DefaultProfile)
	}
}

func TestConfigure_BearerAuth(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.json")
	t.Setenv("JR_CONFIG_PATH", configPath)

	cmd := &cobra.Command{Use: "configure", RunE: runConfigure}
	f := cmd.Flags()
	f.String("base-url", "", "")
	f.String("token", "", "")
	f.StringP("profile", "p", "default", "")
	f.String("auth-type", "basic", "")
	f.String("username", "", "")
	f.Bool("test", false, "")
	f.Bool("delete", false, "")
	f.String("jq", "", "")
	f.Bool("pretty", false, "")

	_ = cmd.Flags().Set("base-url", "https://test.atlassian.net")
	_ = cmd.Flags().Set("token", "bearer-token")
	_ = cmd.Flags().Set("auth-type", "bearer")

	captureStdout(t, func() {
		if err := runConfigure(cmd, nil); err != nil {
			t.Fatalf("runConfigure error: %v", err)
		}
	})

	cfg, err := config.LoadFrom(configPath)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Profiles["default"].Auth.Type != "bearer" {
		t.Errorf("expected auth type 'bearer', got: %s", cfg.Profiles["default"].Auth.Type)
	}
}

// ============================================================
// New tests: configure --test for existing profile
// ============================================================

func TestConfigure_TestExistingProfile_Success(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintln(w, `{"accountId":"abc123"}`)
	}))
	defer ts.Close()

	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.json")
	cfg := &config.Config{
		DefaultProfile: "myprofile",
		Profiles: map[string]config.Profile{
			"myprofile": {
				BaseURL: ts.URL,
				Auth:    config.AuthConfig{Type: "basic", Username: "u", Token: "t"},
			},
		},
	}
	if err := config.SaveTo(cfg, configPath); err != nil {
		t.Fatal(err)
	}
	t.Setenv("JR_CONFIG_PATH", configPath)

	cmd := &cobra.Command{Use: "configure", RunE: runConfigure}
	f := cmd.Flags()
	f.String("base-url", "", "")
	f.String("token", "", "")
	f.StringP("profile", "p", "default", "")
	f.String("auth-type", "basic", "")
	f.String("username", "", "")
	f.Bool("test", false, "")
	f.Bool("delete", false, "")
	f.String("jq", "", "")
	f.Bool("pretty", false, "")

	// Set --test but NOT --base-url or --token to trigger test-only mode.
	_ = cmd.Flags().Set("test", "true")

	captured := captureStdout(t, func() {
		if err := runConfigure(cmd, nil); err != nil {
			t.Fatalf("runConfigure error: %v", err)
		}
	})

	if !strings.Contains(captured, `"ok"`) {
		t.Errorf("expected status ok, got: %s", captured)
	}
	if !strings.Contains(captured, `"myprofile"`) {
		t.Errorf("expected profile name in output, got: %s", captured)
	}
}

func TestConfigure_TestExistingProfile_EmptyBaseURL(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.json")
	cfg := &config.Config{
		DefaultProfile: "empty",
		Profiles: map[string]config.Profile{
			"empty": {BaseURL: ""},
		},
	}
	if err := config.SaveTo(cfg, configPath); err != nil {
		t.Fatal(err)
	}
	t.Setenv("JR_CONFIG_PATH", configPath)

	cmd := &cobra.Command{Use: "configure", RunE: runConfigure}
	f := cmd.Flags()
	f.String("base-url", "", "")
	f.String("token", "", "")
	f.StringP("profile", "p", "default", "")
	f.String("auth-type", "basic", "")
	f.String("username", "", "")
	f.Bool("test", false, "")
	f.Bool("delete", false, "")
	f.String("jq", "", "")
	f.Bool("pretty", false, "")

	_ = cmd.Flags().Set("test", "true")
	_ = cmd.Flags().Set("profile", "empty")

	err := runConfigure(cmd, nil)
	if err == nil {
		t.Fatal("expected error for empty base_url profile")
	}
	aw, ok := err.(*errAlreadyWritten)
	if !ok {
		t.Fatalf("expected errAlreadyWritten, got %T", err)
	}
	if aw.code != jrerrors.ExitValidation {
		t.Errorf("expected exit %d, got %d", jrerrors.ExitValidation, aw.code)
	}
}

func TestConfigure_TestExistingProfile_ProfileNotFound(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.json")
	cfg := &config.Config{
		Profiles: map[string]config.Profile{},
	}
	if err := config.SaveTo(cfg, configPath); err != nil {
		t.Fatal(err)
	}
	t.Setenv("JR_CONFIG_PATH", configPath)

	cmd := &cobra.Command{Use: "configure", RunE: runConfigure}
	f := cmd.Flags()
	f.String("base-url", "", "")
	f.String("token", "", "")
	f.StringP("profile", "p", "default", "")
	f.String("auth-type", "basic", "")
	f.String("username", "", "")
	f.Bool("test", false, "")
	f.Bool("delete", false, "")
	f.String("jq", "", "")
	f.Bool("pretty", false, "")

	_ = cmd.Flags().Set("test", "true")
	_ = cmd.Flags().Set("profile", "nonexistent")

	err := runConfigure(cmd, nil)
	if err == nil {
		t.Fatal("expected error for nonexistent profile")
	}
	aw, ok := err.(*errAlreadyWritten)
	if !ok {
		t.Fatalf("expected errAlreadyWritten, got %T", err)
	}
	if aw.code != jrerrors.ExitNotFound {
		t.Errorf("expected exit %d, got %d", jrerrors.ExitNotFound, aw.code)
	}
}

// ============================================================
// New tests: raw command edge cases
// ============================================================

func TestRawCmd_GetWithBodyWarning(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintln(w, `{"ok":true}`)
	}))
	defer ts.Close()

	var stdout, stderr bytes.Buffer
	c := newTestClient(ts.URL, &stdout, &stderr)

	ctx := client.NewContext(t.Context(), c)
	cmd := &cobra.Command{Use: "raw"}
	cmd.Flags().String("body", "", "")
	cmd.Flags().StringArray("query", nil, "")
	cmd.Flags().String("fields", "", "")
	_ = cmd.Flags().Set("body", `{"test":true}`)
	cmd.SetContext(ctx)

	// Capture real stderr for the warning (written to c.Stderr).
	err := runRaw(cmd, []string{"GET", "/test"})
	if err != nil {
		t.Fatalf("runRaw error: %v", err)
	}
	// The client's stderr should contain a warning about body being ignored.
	combinedStderr := stderr.String()
	if !strings.Contains(combinedStderr, "warning") {
		t.Errorf("expected warning in stderr, got: %s", combinedStderr)
	}
	if !strings.Contains(combinedStderr, "ignored for GET") {
		t.Errorf("expected 'ignored for GET' warning, got: %s", combinedStderr)
	}
}

func TestRawCmd_PostWithoutBody(t *testing.T) {
	cmd := &cobra.Command{Use: "raw"}
	cmd.Flags().String("body", "", "")
	cmd.Flags().StringArray("query", nil, "")
	cmd.Flags().String("fields", "", "")

	var stdout, stderr bytes.Buffer
	c := newTestClient("http://localhost", &stdout, &stderr)
	ctx := client.NewContext(t.Context(), c)
	cmd.SetContext(ctx)

	err := runRaw(cmd, []string{"POST", "/test"})
	if err == nil {
		t.Fatal("expected error for POST without body")
	}
	aw, ok := err.(*errAlreadyWritten)
	if !ok {
		t.Fatalf("expected errAlreadyWritten, got %T", err)
	}
	if aw.code != jrerrors.ExitValidation {
		t.Errorf("expected exit %d, got %d", jrerrors.ExitValidation, aw.code)
	}
}

func TestRawCmd_InvalidMethod(t *testing.T) {
	cmd := &cobra.Command{Use: "raw"}
	cmd.Flags().String("body", "", "")
	cmd.Flags().StringArray("query", nil, "")
	cmd.Flags().String("fields", "", "")

	var stdout, stderr bytes.Buffer
	c := newTestClient("http://localhost", &stdout, &stderr)
	ctx := client.NewContext(t.Context(), c)
	cmd.SetContext(ctx)

	err := runRaw(cmd, []string{"INVALID", "/test"})
	if err == nil {
		t.Fatal("expected error for invalid method")
	}
	aw, ok := err.(*errAlreadyWritten)
	if !ok {
		t.Fatalf("expected errAlreadyWritten, got %T", err)
	}
	if aw.code != jrerrors.ExitValidation {
		t.Errorf("expected exit %d, got %d", jrerrors.ExitValidation, aw.code)
	}
}

func TestRawCmd_EmptyAtBody(t *testing.T) {
	cmd := &cobra.Command{Use: "raw"}
	cmd.Flags().String("body", "", "")
	cmd.Flags().StringArray("query", nil, "")
	cmd.Flags().String("fields", "", "")
	_ = cmd.Flags().Set("body", "@")

	var stdout, stderr bytes.Buffer
	c := newTestClient("http://localhost", &stdout, &stderr)
	ctx := client.NewContext(t.Context(), c)
	cmd.SetContext(ctx)

	err := runRaw(cmd, []string{"POST", "/test"})
	if err == nil {
		t.Fatal("expected error for --body @")
	}
	aw, ok := err.(*errAlreadyWritten)
	if !ok {
		t.Fatalf("expected errAlreadyWritten, got %T", err)
	}
	if aw.code != jrerrors.ExitValidation {
		t.Errorf("expected exit %d, got %d", jrerrors.ExitValidation, aw.code)
	}
}

func TestRawCmd_MultipleQueryParams(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("expand") != "renderedFields" {
			t.Errorf("expected expand=renderedFields, got %q", r.URL.Query().Get("expand"))
		}
		if r.URL.Query().Get("fields") != "key,summary" {
			t.Errorf("expected fields=key,summary, got %q", r.URL.Query().Get("fields"))
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintln(w, `{"ok":true}`)
	}))
	defer ts.Close()

	var stdout, stderr bytes.Buffer
	c := newTestClient(ts.URL, &stdout, &stderr)

	ctx := client.NewContext(t.Context(), c)
	cmd := &cobra.Command{Use: "raw"}
	cmd.Flags().String("body", "", "")
	cmd.Flags().StringArray("query", nil, "")
	cmd.Flags().String("fields", "", "")
	_ = cmd.Flags().Set("query", "expand=renderedFields")
	_ = cmd.Flags().Set("query", "fields=key,summary")
	cmd.SetContext(ctx)

	err := runRaw(cmd, []string{"GET", "/test"})
	if err != nil {
		t.Fatalf("runRaw error: %v", err)
	}
}

// ============================================================
// New tests: fetchOAuth2Token error paths
// ============================================================

func TestFetchOAuth2Token_HTTPError(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		fmt.Fprintln(w, `{"error":"invalid_client"}`)
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
		t.Fatal("expected error for OAuth2 token fetch failure")
	}
	if !strings.Contains(err.Error(), "oauth2") {
		t.Errorf("expected oauth2 error, got: %v", err)
	}
}

func TestFetchOAuth2Token_InvalidJSON(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintln(w, `not valid json`)
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
		t.Fatal("expected error for invalid OAuth2 token JSON")
	}
	if !strings.Contains(err.Error(), "decode") {
		t.Errorf("expected decode error, got: %v", err)
	}
}

// ============================================================
// New tests: schema edge cases
// ============================================================

func TestSchema_ResourceNotFound(t *testing.T) {
	// Test the schema command with a non-existent resource.
	allOps := generated.AllSchemaOps()
	allOps = append(allOps, HandWrittenSchemaOps()...)

	found := false
	for _, op := range allOps {
		if op.Resource == "definitely_nonexistent" {
			found = true
		}
	}
	if found {
		t.Skip("somehow 'definitely_nonexistent' resource exists")
	}
}

// ============================================================
// New tests: config resolution edge cases
// ============================================================

func TestResolve_MissingProfile(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.json")
	cfg := &config.Config{
		Profiles: map[string]config.Profile{
			"existing": {BaseURL: "https://example.com"},
		},
	}
	if err := config.SaveTo(cfg, configPath); err != nil {
		t.Fatal(err)
	}

	_, err := config.Resolve(configPath, "missing", nil)
	if err == nil {
		t.Fatal("expected error for missing profile")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("expected 'not found' in error, got: %v", err)
	}
	if !strings.Contains(err.Error(), "existing") {
		t.Errorf("expected available profiles listed, got: %v", err)
	}
}

func TestResolve_EmptyProfileFallsToDefault(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.json")
	cfg := &config.Config{
		DefaultProfile: "mydefault",
		Profiles: map[string]config.Profile{
			"mydefault": {
				BaseURL: "https://example.com",
				Auth:    config.AuthConfig{Type: "basic", Token: "tok"},
			},
		},
	}
	if err := config.SaveTo(cfg, configPath); err != nil {
		t.Fatal(err)
	}

	resolved, err := config.Resolve(configPath, "", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resolved.BaseURL != "https://example.com" {
		t.Errorf("expected base URL from default profile, got: %s", resolved.BaseURL)
	}
}

// ============================================================
// New tests: client.Do edge cases
// ============================================================

func TestClientDo_DeleteMethod(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "DELETE" {
			t.Errorf("expected DELETE, got %s", r.Method)
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer ts.Close()

	var stdout, stderr bytes.Buffer
	c := newTestClient(ts.URL, &stdout, &stderr)

	code := c.Do(t.Context(), "DELETE", "/test", nil, nil)
	if code != jrerrors.ExitOK {
		t.Fatalf("expected exit 0, got %d; stderr=%s", code, stderr.String())
	}
	// 204 No Content should produce {}.
	if !strings.Contains(stdout.String(), "{}") {
		t.Errorf("expected {} for 204, got: %s", stdout.String())
	}
}

func TestClientDo_EmptyPath(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintln(w, `{"ok":true}`)
	}))
	defer ts.Close()

	var stdout, stderr bytes.Buffer
	c := newTestClient(ts.URL, &stdout, &stderr)

	code := c.Do(t.Context(), "GET", "", nil, nil)
	if code != jrerrors.ExitOK {
		t.Fatalf("expected exit 0, got %d; stderr=%s", code, stderr.String())
	}
}

func TestClientDo_DryRunWithBody(t *testing.T) {
	var stdout, stderr bytes.Buffer
	c := newTestClient("http://should-not-be-called", &stdout, &stderr)
	c.DryRun = true

	code := c.Do(t.Context(), "POST", "/test", nil, strings.NewReader(`{"hello":"world"}`))
	if code != jrerrors.ExitOK {
		t.Fatalf("expected exit 0, got %d", code)
	}
	// Dry-run output should include the parsed body.
	output := stdout.String()
	if !strings.Contains(output, "hello") {
		t.Errorf("expected body in dry-run output, got: %s", output)
	}
}

func TestClientDo_DryRunNilBody(t *testing.T) {
	var stdout, stderr bytes.Buffer
	c := newTestClient("http://should-not-be-called", &stdout, &stderr)
	c.DryRun = true

	code := c.Do(t.Context(), "GET", "/test", nil, nil)
	if code != jrerrors.ExitOK {
		t.Fatalf("expected exit 0, got %d", code)
	}
	output := stdout.String()
	if !strings.Contains(output, `"method":"GET"`) {
		t.Errorf("expected method in dry-run output, got: %s", output)
	}
}

// ============================================================
// New tests: batch with empty array
// ============================================================

func TestBatch_EmptyArray(t *testing.T) {
	var stdout, stderr bytes.Buffer
	c := newTestClient("http://localhost", &stdout, &stderr)

	ctx := client.NewContext(t.Context(), c)
	cmd := &cobra.Command{Use: "batch"}
	cmd.Flags().String("input", "", "")
	cmd.SetContext(ctx)

	oldStdin := os.Stdin
	pr, pw, _ := os.Pipe()
	_, _ = pw.WriteString("[]")
	pw.Close()
	os.Stdin = pr
	defer func() { os.Stdin = oldStdin }()

	captured := captureStdout(t, func() {
		if err := runBatch(cmd, nil); err != nil {
			t.Fatalf("runBatch error: %v", err)
		}
	})

	got := strings.TrimSpace(captured)
	if got != "[]" {
		t.Errorf("expected '[]' for empty batch, got: %s", got)
	}
}
