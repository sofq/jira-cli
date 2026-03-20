package cmd

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/sofq/jira-cli/internal/client"
	jrerrors "github.com/sofq/jira-cli/internal/errors"
	"github.com/spf13/cobra"
)

func newContextCmd(c *client.Client) *cobra.Command {
	cmd := &cobra.Command{Use: "context", RunE: runContext}
	cmd.SetContext(client.NewContext(context.Background(), c))
	return cmd
}

// TestRunContext_Success verifies that all three endpoints are called and their
// responses are merged into a single combined JSON object.
func TestRunContext_Success(t *testing.T) {
	issueResp := map[string]any{
		"key": "PROJ-123",
		"fields": map[string]string{
			"summary": "Test issue",
		},
	}
	commentsResp := map[string]any{
		"comments": []map[string]string{
			{"body": "First comment"},
		},
		"total": 1,
	}
	changelogResp := map[string]any{
		"values": []map[string]any{
			{
				"created": "2026-01-15T10:30:00Z",
				"items": []map[string]string{
					{"field": "status", "fromString": "To Do", "toString": "In Progress"},
				},
			},
		},
	}

	var issueHit, commentsHit, changelogHit bool

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case strings.Contains(r.URL.Path, "/comment"):
			commentsHit = true
			_ = json.NewEncoder(w).Encode(commentsResp)
		case strings.Contains(r.URL.Path, "/changelog"):
			changelogHit = true
			_ = json.NewEncoder(w).Encode(changelogResp)
		default:
			// Base issue path (may include query params like expand=renderedFields)
			issueHit = true
			_ = json.NewEncoder(w).Encode(issueResp)
		}
	}))
	defer srv.Close()

	var stdout, stderr bytes.Buffer
	c := newTestClient(srv.URL, &stdout, &stderr)

	cmd := newContextCmd(c)
	err := cmd.RunE(cmd, []string{"PROJ-123"})
	if err == nil {
		t.Fatal("expected AlreadyWrittenError with ExitOK, got nil")
	}
	aw, ok := err.(*jrerrors.AlreadyWrittenError)
	if !ok {
		t.Fatalf("expected AlreadyWrittenError, got %T: %v", err, err)
	}
	if aw.Code != jrerrors.ExitOK {
		t.Errorf("expected exit code %d, got %d; stderr: %s", jrerrors.ExitOK, aw.Code, stderr.String())
	}

	if !issueHit {
		t.Error("issue endpoint was not called")
	}
	if !commentsHit {
		t.Error("comments endpoint was not called")
	}
	if !changelogHit {
		t.Error("changelog endpoint was not called")
	}

	var combined map[string]json.RawMessage
	if err := json.Unmarshal([]byte(strings.TrimSpace(stdout.String())), &combined); err != nil {
		t.Fatalf("failed to parse combined output: %v; output: %s", err, stdout.String())
	}

	for _, key := range []string{"issue", "comments", "changelog"} {
		if _, ok := combined[key]; !ok {
			t.Errorf("expected key %q in combined output", key)
		}
	}

	// Verify the issue key is present in the issue sub-object.
	var issueParsed map[string]any
	if err := json.Unmarshal(combined["issue"], &issueParsed); err != nil {
		t.Fatalf("failed to parse issue field: %v", err)
	}
	if issueParsed["key"] != "PROJ-123" {
		t.Errorf("expected issue key PROJ-123, got %v", issueParsed["key"])
	}
}

// TestRunContext_NotFound verifies that a 404 from the issue endpoint propagates
// the correct exit code without calling the remaining endpoints.
func TestRunContext_NotFound(t *testing.T) {
	var callCount int

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		w.WriteHeader(http.StatusNotFound)
		_ = json.NewEncoder(w).Encode(map[string]string{"errorMessages": "Issue does not exist"})
	}))
	defer srv.Close()

	var stdout, stderr bytes.Buffer
	c := newTestClient(srv.URL, &stdout, &stderr)

	cmd := newContextCmd(c)
	err := cmd.RunE(cmd, []string{"PROJ-999"})
	if err == nil {
		t.Fatal("expected error for 404")
	}
	aw, ok := err.(*jrerrors.AlreadyWrittenError)
	if !ok {
		t.Fatalf("expected AlreadyWrittenError, got %T: %v", err, err)
	}
	if aw.Code != jrerrors.ExitNotFound {
		t.Errorf("expected exit code %d (not_found), got %d", jrerrors.ExitNotFound, aw.Code)
	}
	// Only the issue endpoint should have been hit.
	if callCount != 1 {
		t.Errorf("expected 1 HTTP call (early exit on 404), got %d", callCount)
	}
}

// TestRunContext_JQFilter verifies that --jq is applied to the combined output.
func TestRunContext_JQFilter(t *testing.T) {
	issueResp := map[string]any{"key": "PROJ-42", "fields": map[string]string{"summary": "Hello"}}
	commentsResp := map[string]any{"comments": []any{}, "total": 0}
	changelogResp := map[string]any{"values": []any{}}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case strings.Contains(r.URL.Path, "/comment"):
			_ = json.NewEncoder(w).Encode(commentsResp)
		case strings.Contains(r.URL.Path, "/changelog"):
			_ = json.NewEncoder(w).Encode(changelogResp)
		default:
			_ = json.NewEncoder(w).Encode(issueResp)
		}
	}))
	defer srv.Close()

	var stdout, stderr bytes.Buffer
	c := newTestClient(srv.URL, &stdout, &stderr)
	// Extract only the issue key from the combined result.
	c.JQFilter = ".issue.key"

	cmd := newContextCmd(c)
	err := cmd.RunE(cmd, []string{"PROJ-42"})
	if err == nil {
		t.Fatal("expected AlreadyWrittenError with ExitOK, got nil")
	}
	aw, ok := err.(*jrerrors.AlreadyWrittenError)
	if !ok {
		t.Fatalf("expected AlreadyWrittenError, got %T: %v", err, err)
	}
	if aw.Code != jrerrors.ExitOK {
		t.Errorf("expected exit code %d, got %d; stderr: %s", jrerrors.ExitOK, aw.Code, stderr.String())
	}

	output := strings.TrimSpace(stdout.String())
	if output != `"PROJ-42"` {
		t.Errorf("expected jq output %q, got %q", `"PROJ-42"`, output)
	}
}

// TestRunContext_NoClient verifies the error path when no client is in context.
func TestRunContext_NoClient(t *testing.T) {
	cmd := &cobra.Command{Use: "context", RunE: runContext}
	cmd.SetContext(context.Background())

	err := cmd.RunE(cmd, []string{"PROJ-1"})
	if err == nil {
		t.Fatal("expected error when no client in context")
	}
	aw, ok := err.(*jrerrors.AlreadyWrittenError)
	if !ok {
		t.Fatalf("expected AlreadyWrittenError, got %T: %v", err, err)
	}
	if aw.Code != jrerrors.ExitError {
		t.Errorf("expected exit code %d, got %d", jrerrors.ExitError, aw.Code)
	}
}

// TestRunContext_CommentsFails verifies early exit when the comments endpoint returns an error.
func TestRunContext_CommentsFails(t *testing.T) {
	var callCount int

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		w.Header().Set("Content-Type", "application/json")
		if strings.Contains(r.URL.Path, "/comment") {
			w.WriteHeader(http.StatusInternalServerError)
			_ = json.NewEncoder(w).Encode(map[string]string{"errorMessages": "server error"})
			return
		}
		// Issue succeeds.
		_ = json.NewEncoder(w).Encode(map[string]string{"key": "PROJ-1"})
	}))
	defer srv.Close()

	var stdout, stderr bytes.Buffer
	c := newTestClient(srv.URL, &stdout, &stderr)

	cmd := newContextCmd(c)
	err := cmd.RunE(cmd, []string{"PROJ-1"})
	if err == nil {
		t.Fatal("expected error when comments endpoint fails")
	}
	aw, ok := err.(*jrerrors.AlreadyWrittenError)
	if !ok {
		t.Fatalf("expected AlreadyWrittenError, got %T: %v", err, err)
	}
	if aw.Code == jrerrors.ExitOK {
		t.Errorf("expected non-zero exit code, got %d", aw.Code)
	}
	// changelog endpoint must not be called.
	if callCount != 2 {
		t.Errorf("expected 2 HTTP calls (issue + comments), got %d", callCount)
	}
}

// TestContextSchemaOps verifies the schema registration for the context command.
func TestContextSchemaOps(t *testing.T) {
	ops := ContextSchemaOps()
	if len(ops) != 1 {
		t.Fatalf("expected 1 context schema op, got %d", len(ops))
	}
	op := ops[0]
	if op.Resource != "context" {
		t.Errorf("expected resource=context, got %s", op.Resource)
	}
	if op.Verb != "context" {
		t.Errorf("expected verb=context, got %s", op.Verb)
	}
	if op.Method != "GET" {
		t.Errorf("expected method=GET, got %s", op.Method)
	}
}
