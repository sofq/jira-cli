package cmd

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/sofq/jira-cli/internal/changelog"
	"github.com/sofq/jira-cli/internal/client"
	jrerrors "github.com/sofq/jira-cli/internal/errors"
	"github.com/spf13/cobra"
)

func newDiffCmd(c *client.Client) *cobra.Command {
	cmd := &cobra.Command{Use: "diff", RunE: runDiff}
	cmd.Flags().String("issue", "", "")
	cmd.Flags().String("since", "", "")
	cmd.Flags().String("field", "", "")
	cmd.SetContext(client.NewContext(context.Background(), c))
	return cmd
}

func TestRunDiff_DryRun(t *testing.T) {
	var stdout, stderr bytes.Buffer
	c := newTestClient("http://unused", &stdout, &stderr)
	c.DryRun = true

	cmd := newDiffCmd(c)
	_ = cmd.Flags().Set("issue", "PROJ-1")

	err := cmd.RunE(cmd, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	output := stdout.String()
	if !strings.Contains(output, "GET") {
		t.Errorf("expected GET in dry-run output, got: %s", output)
	}
	if !strings.Contains(output, "changelog") {
		t.Errorf("expected changelog in dry-run output, got: %s", output)
	}
	if !strings.Contains(output, "PROJ-1") {
		t.Errorf("expected issue key in dry-run output, got: %s", output)
	}
}

func TestRunDiff_Success(t *testing.T) {
	changelogResp := map[string]any{
		"values": []map[string]any{
			{
				"created": "2026-01-15T10:30:00Z",
				"author": map[string]string{
					"emailAddress": "john@example.com",
					"displayName":  "John Doe",
				},
				"items": []map[string]string{
					{
						"field":      "status",
						"fromString": "To Do",
						"toString":   "In Progress",
					},
					{
						"field":      "assignee",
						"fromString": "",
						"toString":   "Jane Doe",
					},
				},
			},
		},
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "GET" {
			t.Errorf("expected GET, got %s", r.Method)
		}
		if !strings.Contains(r.URL.Path, "/changelog") {
			t.Errorf("expected /changelog path, got %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(changelogResp)
	}))
	defer srv.Close()

	var stdout, stderr bytes.Buffer
	c := newTestClient(srv.URL, &stdout, &stderr)

	cmd := newDiffCmd(c)
	_ = cmd.Flags().Set("issue", "PROJ-123")

	err := cmd.RunE(cmd, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v; stderr: %s", err, stderr.String())
	}

	var result changelog.Result
	if err := json.Unmarshal([]byte(strings.TrimSpace(stdout.String())), &result); err != nil {
		t.Fatalf("failed to parse output: %v; output: %s", err, stdout.String())
	}

	if result.Issue != "PROJ-123" {
		t.Errorf("expected issue PROJ-123, got %s", result.Issue)
	}
	if len(result.Changes) != 2 {
		t.Fatalf("expected 2 changes, got %d", len(result.Changes))
	}
	if result.Changes[0].Field != "status" {
		t.Errorf("expected field 'status', got %q", result.Changes[0].Field)
	}
	if result.Changes[0].Author != "john@example.com" {
		t.Errorf("expected author 'john@example.com', got %q", result.Changes[0].Author)
	}
	if result.Changes[1].From != nil {
		t.Errorf("expected nil from for assignee, got %v", result.Changes[1].From)
	}
}

func TestRunDiff_FieldFilter(t *testing.T) {
	changelogResp := map[string]any{
		"values": []map[string]any{
			{
				"created": "2026-01-15T10:30:00Z",
				"author":  map[string]string{"emailAddress": "john@example.com"},
				"items": []map[string]string{
					{"field": "status", "fromString": "To Do", "toString": "In Progress"},
					{"field": "assignee", "fromString": "", "toString": "Jane Doe"},
				},
			},
		},
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(changelogResp)
	}))
	defer srv.Close()

	var stdout, stderr bytes.Buffer
	c := newTestClient(srv.URL, &stdout, &stderr)

	cmd := newDiffCmd(c)
	_ = cmd.Flags().Set("issue", "PROJ-123")
	_ = cmd.Flags().Set("field", "status")

	err := cmd.RunE(cmd, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var result changelog.Result
	if err := json.Unmarshal([]byte(strings.TrimSpace(stdout.String())), &result); err != nil {
		t.Fatalf("failed to parse output: %v", err)
	}

	if len(result.Changes) != 1 {
		t.Fatalf("expected 1 change (filtered to status), got %d", len(result.Changes))
	}
	if result.Changes[0].Field != "status" {
		t.Errorf("expected field 'status', got %q", result.Changes[0].Field)
	}
}

func TestRunDiff_NotFound(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_ = json.NewEncoder(w).Encode(map[string]string{"errorMessages": "Issue Does not exist"})
	}))
	defer srv.Close()

	var stdout, stderr bytes.Buffer
	c := newTestClient(srv.URL, &stdout, &stderr)

	cmd := newDiffCmd(c)
	_ = cmd.Flags().Set("issue", "PROJ-999")

	err := cmd.RunE(cmd, nil)
	if err == nil {
		t.Fatal("expected error for 404")
	}
	aw, ok := err.(*jrerrors.AlreadyWrittenError)
	if !ok {
		t.Fatalf("expected AlreadyWrittenError, got %T", err)
	}
	if aw.Code != jrerrors.ExitNotFound {
		t.Errorf("expected exit code %d, got %d", jrerrors.ExitNotFound, aw.Code)
	}
}

func TestRunDiff_EmptyChangelog(t *testing.T) {
	changelogResp := map[string]any{
		"values": []map[string]any{},
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(changelogResp)
	}))
	defer srv.Close()

	var stdout, stderr bytes.Buffer
	c := newTestClient(srv.URL, &stdout, &stderr)

	cmd := newDiffCmd(c)
	_ = cmd.Flags().Set("issue", "PROJ-123")

	err := cmd.RunE(cmd, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var result changelog.Result
	if err := json.Unmarshal([]byte(strings.TrimSpace(stdout.String())), &result); err != nil {
		t.Fatalf("failed to parse output: %v", err)
	}

	if result.Issue != "PROJ-123" {
		t.Errorf("expected issue PROJ-123, got %s", result.Issue)
	}
	if len(result.Changes) != 0 {
		t.Errorf("expected 0 changes, got %d", len(result.Changes))
	}
}

func TestRunDiff_SinceFilter(t *testing.T) {
	changelogResp := map[string]any{
		"values": []map[string]any{
			{
				"created": "2025-06-01T10:00:00Z",
				"author":  map[string]string{"emailAddress": "old@example.com"},
				"items": []map[string]string{
					{"field": "status", "fromString": "A", "toString": "B"},
				},
			},
			{
				"created": "2026-03-01T10:00:00Z",
				"author":  map[string]string{"emailAddress": "recent@example.com"},
				"items": []map[string]string{
					{"field": "status", "fromString": "B", "toString": "C"},
				},
			},
		},
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(changelogResp)
	}))
	defer srv.Close()

	var stdout, stderr bytes.Buffer
	c := newTestClient(srv.URL, &stdout, &stderr)

	cmd := newDiffCmd(c)
	_ = cmd.Flags().Set("issue", "PROJ-123")
	_ = cmd.Flags().Set("since", "2026-01-01")

	err := cmd.RunE(cmd, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var result changelog.Result
	if err := json.Unmarshal([]byte(strings.TrimSpace(stdout.String())), &result); err != nil {
		t.Fatalf("failed to parse output: %v", err)
	}

	if len(result.Changes) != 1 {
		t.Fatalf("expected 1 change after 2026-01-01, got %d", len(result.Changes))
	}
	if result.Changes[0].Author != "recent@example.com" {
		t.Errorf("expected recent@example.com, got %q", result.Changes[0].Author)
	}
}

func TestDiffSchemaOps(t *testing.T) {
	ops := DiffSchemaOps()
	if len(ops) != 1 {
		t.Fatalf("expected 1 diff schema op, got %d", len(ops))
	}
	op := ops[0]
	if op.Resource != "diff" || op.Verb != "diff" {
		t.Errorf("expected resource=diff verb=diff, got %s %s", op.Resource, op.Verb)
	}
}

func TestRunDiff_NoClient(t *testing.T) {
	cmd := &cobra.Command{Use: "diff", RunE: runDiff}
	cmd.Flags().String("issue", "", "")
	cmd.Flags().String("since", "", "")
	cmd.Flags().String("field", "", "")
	_ = cmd.Flags().Set("issue", "PROJ-1")
	cmd.SetContext(context.Background())

	err := cmd.RunE(cmd, nil)
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

func TestRunDiff_InvalidSince(t *testing.T) {
	changelogResp := map[string]any{
		"values": []map[string]any{
			{
				"created": "2026-01-15T10:30:00Z",
				"author":  map[string]string{"emailAddress": "user@example.com"},
				"items": []map[string]string{
					{"field": "status", "fromString": "To Do", "toString": "In Progress"},
				},
			},
		},
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(changelogResp)
	}))
	defer srv.Close()

	var stdout, stderr bytes.Buffer
	c := newTestClient(srv.URL, &stdout, &stderr)

	cmd := newDiffCmd(c)
	_ = cmd.Flags().Set("issue", "PROJ-1")
	_ = cmd.Flags().Set("since", "notaduration")

	err := cmd.RunE(cmd, nil)
	if err == nil {
		t.Fatal("expected error for invalid --since value")
	}
	aw, ok := err.(*jrerrors.AlreadyWrittenError)
	if !ok {
		t.Fatalf("expected AlreadyWrittenError, got %T: %v", err, err)
	}
	if aw.Code != jrerrors.ExitValidation {
		t.Errorf("expected exit code %d, got %d", jrerrors.ExitValidation, aw.Code)
	}
}

// TestRunDiff_DryRunWriteOutputFails covers the DryRun branch where WriteOutput
// fails due to an invalid JQ filter (diff.go line 57-58).
func TestRunDiff_DryRunWriteOutputFails(t *testing.T) {
	var stdout, stderr bytes.Buffer
	c := newTestClient("http://unused", &stdout, &stderr)
	c.DryRun = true
	c.JQFilter = ".[[[invalid"

	cmd := newDiffCmd(c)
	_ = cmd.Flags().Set("issue", "PROJ-1")

	err := cmd.RunE(cmd, nil)
	if err == nil {
		t.Fatal("expected error when DryRun WriteOutput fails due to invalid JQ filter")
	}
	aw, ok := err.(*jrerrors.AlreadyWrittenError)
	if !ok {
		t.Fatalf("expected AlreadyWrittenError, got %T: %v", err, err)
	}
	if aw.Code == jrerrors.ExitOK {
		t.Errorf("expected non-zero exit code, got %d", aw.Code)
	}
}

// TestRunDiff_WriteOutputFails covers the final WriteOutput failure after
// a successful changelog fetch (diff.go line 84-85).
func TestRunDiff_WriteOutputFails(t *testing.T) {
	changelogResp := map[string]any{
		"values": []map[string]any{
			{
				"created": "2026-03-01T10:00:00Z",
				"author":  map[string]string{"emailAddress": "user@example.com"},
				"items": []map[string]string{
					{"field": "status", "fromString": "To Do", "toString": "In Progress"},
				},
			},
		},
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(changelogResp)
	}))
	defer srv.Close()

	var stdout, stderr bytes.Buffer
	c := newTestClient(srv.URL, &stdout, &stderr)
	c.JQFilter = ".[[[invalid"

	cmd := newDiffCmd(c)
	_ = cmd.Flags().Set("issue", "PROJ-123")

	err := cmd.RunE(cmd, nil)
	if err == nil {
		t.Fatal("expected error when WriteOutput fails due to invalid JQ filter")
	}
	aw, ok := err.(*jrerrors.AlreadyWrittenError)
	if !ok {
		t.Fatalf("expected AlreadyWrittenError, got %T: %v", err, err)
	}
	if aw.Code == jrerrors.ExitOK {
		t.Errorf("expected non-zero exit code, got %d", aw.Code)
	}
}
