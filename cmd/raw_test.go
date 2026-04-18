package cmd

import (
	"bytes"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sofq/jira-cli/internal/client"
	jrerrors "github.com/sofq/jira-cli/internal/errors"
	"github.com/spf13/cobra"
)

// --- Query parameter tests ---

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
	aw, ok := err.(*jrerrors.AlreadyWrittenError)
	if !ok {
		t.Fatalf("expected AlreadyWrittenError, got %T: %v", err, err)
	}
	if aw.Code != jrerrors.ExitValidation {
		t.Errorf("expected exit code %d, got %d", jrerrors.ExitValidation, aw.Code)
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

// --- Body validation tests ---

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

	// The client's stderr should contain a warning about body being ignored.
	err := runRaw(cmd, []string{"GET", "/test"})
	if err != nil {
		t.Fatalf("runRaw error: %v", err)
	}
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
	aw, ok := err.(*jrerrors.AlreadyWrittenError)
	if !ok {
		t.Fatalf("expected AlreadyWrittenError, got %T", err)
	}
	if aw.Code != jrerrors.ExitValidation {
		t.Errorf("expected exit %d, got %d", jrerrors.ExitValidation, aw.Code)
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
	aw, ok := err.(*jrerrors.AlreadyWrittenError)
	if !ok {
		t.Fatalf("expected AlreadyWrittenError, got %T", err)
	}
	if aw.Code != jrerrors.ExitValidation {
		t.Errorf("expected exit %d, got %d", jrerrors.ExitValidation, aw.Code)
	}
}

func TestRawCmd_FileOpenError_SingleOutput(t *testing.T) {
	// After the fix, a file open error should return AlreadyWrittenError,
	// not a raw error, to prevent double error output.
	cmd := &cobra.Command{Use: "raw"}
	f := cmd.Flags()
	f.String("body", "", "")
	f.StringArray("query", nil, "")
	f.String("fields", "", "")
	_ = cmd.Flags().Set("body", "@nonexistent-file-for-test.json")

	// A valid client in context is required for the code to reach the file open path.
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
	if _, ok := err.(*jrerrors.AlreadyWrittenError); !ok {
		t.Errorf("expected AlreadyWrittenError to prevent double error output, got %T: %v", err, err)
	}
}

// --- Method validation tests ---

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
	aw, ok := err.(*jrerrors.AlreadyWrittenError)
	if !ok {
		t.Fatalf("expected AlreadyWrittenError, got %T", err)
	}
	if aw.Code != jrerrors.ExitValidation {
		t.Errorf("expected exit %d, got %d", jrerrors.ExitValidation, aw.Code)
	}
}

// --- Dry-run tests ---

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
	aw, ok := err.(*jrerrors.AlreadyWrittenError)
	if !ok {
		t.Fatalf("expected AlreadyWrittenError, got %T", err)
	}
	if aw.Code != jrerrors.ExitValidation {
		t.Errorf("expected exit %d, got %d", jrerrors.ExitValidation, aw.Code)
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

func TestRawCmd_NoClient(t *testing.T) {
	cmd := &cobra.Command{Use: "raw"}
	cmd.Flags().String("body", "", "")
	cmd.Flags().StringArray("query", nil, "")
	cmd.SetContext(t.Context())

	err := runRaw(cmd, []string{"GET", "/test"})
	if err == nil {
		t.Fatal("expected error when no client in context")
	}
	aw, ok := err.(*jrerrors.AlreadyWrittenError)
	if !ok {
		t.Fatalf("expected AlreadyWrittenError, got %T", err)
	}
	if aw.Code != jrerrors.ExitError {
		t.Errorf("expected exit %d, got %d", jrerrors.ExitError, aw.Code)
	}
}

// TestRawCmd_FileBody exercises the @filename body reader path, including the
// deferred file close (line 123-124 in raw.go).
func TestRawCmd_FileBody(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintln(w, `{"ok":true}`)
	}))
	defer ts.Close()

	// Write a temp JSON file to use as body.
	tmpFile := filepath.Join(t.TempDir(), "body.json")
	os.WriteFile(tmpFile, []byte(`{"key":"value"}`), 0o644)

	var stdout, stderr bytes.Buffer
	c := newTestClient(ts.URL, &stdout, &stderr)

	ctx := client.NewContext(t.Context(), c)
	cmd := &cobra.Command{Use: "raw"}
	cmd.Flags().String("body", "", "")
	cmd.Flags().StringArray("query", nil, "")
	_ = cmd.Flags().Set("body", "@"+tmpFile)
	cmd.SetContext(ctx)

	err := runRaw(cmd, []string{"POST", "/test"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(stdout.String(), "ok") {
		t.Errorf("expected ok in output, got: %s", stdout.String())
	}
}

// TestRawCmd_DoFailure exercises the c.Do error return path (lines 155-157).
func TestRawCmd_DoFailure(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		fmt.Fprintln(w, `{"errorMessages":["server error"]}`)
	}))
	defer ts.Close()

	var stdout, stderr bytes.Buffer
	c := newTestClient(ts.URL, &stdout, &stderr)

	ctx := client.NewContext(t.Context(), c)
	cmd := &cobra.Command{Use: "raw"}
	cmd.Flags().String("body", "", "")
	cmd.Flags().StringArray("query", nil, "")
	cmd.SetContext(ctx)

	err := runRaw(cmd, []string{"GET", "/test"})
	if err == nil {
		t.Fatal("expected error when server returns 500")
	}
	aw, ok := err.(*jrerrors.AlreadyWrittenError)
	if !ok {
		t.Fatalf("expected AlreadyWrittenError, got %T", err)
	}
	if aw.Code == 0 {
		t.Error("expected non-zero exit code")
	}
}

func TestRawCmd_PolicyDenied(t *testing.T) {
	var stdout, stderr bytes.Buffer
	c := newTestClient("http://localhost", &stdout, &stderr)

	// Set a policy that denies all operations.
	p, err := newTestDenyPolicy()
	if err != nil {
		t.Fatalf("failed to create deny policy: %v", err)
	}
	c.Policy = p

	ctx := client.NewContext(t.Context(), c)
	cmd := &cobra.Command{Use: "raw"}
	cmd.Flags().String("body", "", "")
	cmd.Flags().StringArray("query", nil, "")
	cmd.SetContext(ctx)

	err = runRaw(cmd, []string{"GET", "/test"})
	if err == nil {
		t.Fatal("expected error when policy denies operation")
	}
	aw, ok := err.(*jrerrors.AlreadyWrittenError)
	if !ok {
		t.Fatalf("expected AlreadyWrittenError, got %T", err)
	}
	if aw.Code != jrerrors.ExitValidation {
		t.Errorf("expected exit %d, got %d", jrerrors.ExitValidation, aw.Code)
	}
}

// TestRawCmd_BodyAtWithoutFilename exercises the --body @ validation path
// where no filename is provided after the @ prefix (raw.go line 106-113).
func TestRawCmd_BodyAtWithoutFilename(t *testing.T) {
	var stdout, stderr bytes.Buffer
	c := newTestClient("http://localhost", &stdout, &stderr)

	ctx := client.NewContext(t.Context(), c)
	cmd := &cobra.Command{Use: "raw"}
	cmd.Flags().String("body", "", "")
	cmd.Flags().StringArray("query", nil, "")
	_ = cmd.Flags().Set("body", "@")
	cmd.SetContext(ctx)

	err := runRaw(cmd, []string{"POST", "/test"})
	if err == nil {
		t.Fatal("expected error for --body @ without filename")
	}
	aw, ok := err.(*jrerrors.AlreadyWrittenError)
	if !ok {
		t.Fatalf("expected AlreadyWrittenError, got %T", err)
	}
	if aw.Code != jrerrors.ExitValidation {
		t.Errorf("expected ExitValidation, got %d", aw.Code)
	}
}

// TestRawCmd_StdinBody exercises --body - (explicit stdin).
func TestRawCmd_StdinBody(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body := make([]byte, 1024)
		n, _ := r.Body.Read(body)
		if !strings.Contains(string(body[:n]), "piped") {
			t.Errorf("expected piped body, got %q", string(body[:n]))
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintln(w, `{"ok":true}`)
	}))
	defer ts.Close()

	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe: %v", err)
	}
	origStdin := os.Stdin
	os.Stdin = r
	defer func() { os.Stdin = origStdin }()

	go func() {
		_, _ = w.Write([]byte(`{"from":"piped"}`))
		_ = w.Close()
	}()

	var stdout, stderr bytes.Buffer
	c := newTestClient(ts.URL, &stdout, &stderr)
	ctx := client.NewContext(t.Context(), c)
	cmd := &cobra.Command{Use: "raw"}
	cmd.Flags().String("body", "", "")
	cmd.Flags().StringArray("query", nil, "")
	_ = cmd.Flags().Set("body", "-")
	cmd.SetContext(ctx)

	if err := runRaw(cmd, []string{"POST", "/test"}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}
