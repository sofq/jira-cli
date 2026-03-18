package cmd

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/sofq/jira-cli/internal/client"
	jrerrors "github.com/sofq/jira-cli/internal/errors"
	"github.com/spf13/cobra"
)

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
	if !found["workflow comment"] {
		t.Error("HandWrittenSchemaOps missing 'workflow comment'")
	}
}

// newTransitionCmd creates a workflow transition command with flags and client context.
func newTransitionCmd(c *client.Client) *cobra.Command {
	cmd := &cobra.Command{Use: "transition", RunE: runTransition}
	cmd.Flags().String("issue", "", "")
	cmd.Flags().String("to", "", "")
	cmd.SetContext(client.NewContext(context.Background(), c))
	return cmd
}

// newAssignCmd creates a workflow assign command with flags and client context.
func newAssignCmd(c *client.Client) *cobra.Command {
	cmd := &cobra.Command{Use: "assign", RunE: runAssign}
	cmd.Flags().String("issue", "", "")
	cmd.Flags().String("to", "", "")
	cmd.SetContext(client.NewContext(context.Background(), c))
	return cmd
}

// newCommentCmd creates a workflow comment command with flags and client context.
func newCommentCmd(c *client.Client) *cobra.Command {
	cmd := &cobra.Command{Use: "comment", RunE: runComment}
	cmd.Flags().String("issue", "", "")
	cmd.Flags().String("text", "", "")
	cmd.SetContext(client.NewContext(context.Background(), c))
	return cmd
}

func TestRunTransition_DryRun(t *testing.T) {
	var stdout, stderr bytes.Buffer
	c := newTestClient("http://unused", &stdout, &stderr)
	c.DryRun = true

	cmd := newTransitionCmd(c)
	_ = cmd.Flags().Set("issue", "PROJ-1")
	_ = cmd.Flags().Set("to", "Done")

	err := cmd.RunE(cmd, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	output := stdout.String()
	if !strings.Contains(output, "POST") {
		t.Errorf("expected POST in dry-run output, got: %s", output)
	}
	if !strings.Contains(output, "PROJ-1") {
		t.Errorf("expected issue key in output, got: %s", output)
	}
}

func TestRunTransition_Success(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Method == "GET" && strings.Contains(r.URL.Path, "/transitions") {
			fmt.Fprintln(w, `{"transitions":[{"id":"31","name":"Done","to":{"name":"Done"}}]}`)
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

	cmd := newTransitionCmd(c)
	_ = cmd.Flags().Set("issue", "PROJ-1")
	_ = cmd.Flags().Set("to", "Done")

	err := cmd.RunE(cmd, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	output := stdout.String()
	if !strings.Contains(output, "transitioned") {
		t.Errorf("expected 'transitioned' in output, got: %s", output)
	}
}

func TestRunTransition_NoMatch(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintln(w, `{"transitions":[{"id":"31","name":"In Progress","to":{"name":"In Progress"}}]}`)
	}))
	defer ts.Close()

	var stdout, stderr bytes.Buffer
	c := newTestClient(ts.URL, &stdout, &stderr)

	cmd := newTransitionCmd(c)
	_ = cmd.Flags().Set("issue", "PROJ-1")
	_ = cmd.Flags().Set("to", "Nonexistent")

	err := cmd.RunE(cmd, nil)
	if err == nil {
		t.Fatal("expected error for no matching transition")
	}
	aw, ok := err.(*jrerrors.AlreadyWrittenError)
	if !ok {
		t.Fatalf("expected AlreadyWrittenError, got %T", err)
	}
	if aw.Code != jrerrors.ExitValidation {
		t.Errorf("expected exit %d, got %d", jrerrors.ExitValidation, aw.Code)
	}
}

func TestRunTransition_FetchError(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		fmt.Fprintln(w, `{"message":"Unauthorized"}`)
	}))
	defer ts.Close()

	var stdout, stderr bytes.Buffer
	c := newTestClient(ts.URL, &stdout, &stderr)

	cmd := newTransitionCmd(c)
	_ = cmd.Flags().Set("issue", "PROJ-1")
	_ = cmd.Flags().Set("to", "Done")

	err := cmd.RunE(cmd, nil)
	if err == nil {
		t.Fatal("expected error for fetch failure")
	}
}

func TestRunTransition_NoClient(t *testing.T) {
	cmd := &cobra.Command{Use: "transition", RunE: runTransition}
	cmd.Flags().String("issue", "", "")
	cmd.Flags().String("to", "", "")
	cmd.SetContext(context.Background()) // context without client

	err := cmd.RunE(cmd, nil)
	if err == nil {
		t.Fatal("expected error when no client in context")
	}
}

func TestRunAssign_DryRun(t *testing.T) {
	var stdout, stderr bytes.Buffer
	c := newTestClient("http://unused", &stdout, &stderr)
	c.DryRun = true

	cmd := newAssignCmd(c)
	_ = cmd.Flags().Set("issue", "PROJ-1")
	_ = cmd.Flags().Set("to", "me")

	err := cmd.RunE(cmd, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	output := stdout.String()
	if !strings.Contains(output, "PUT") {
		t.Errorf("expected PUT in dry-run output, got: %s", output)
	}
}

func TestRunAssign_Me(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if strings.Contains(r.URL.Path, "/myself") {
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

	cmd := newAssignCmd(c)
	_ = cmd.Flags().Set("issue", "PROJ-1")
	_ = cmd.Flags().Set("to", "me")

	err := cmd.RunE(cmd, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	output := stdout.String()
	if !strings.Contains(output, "assigned") {
		t.Errorf("expected 'assigned' in output, got: %s", output)
	}
}

func TestRunAssign_Unassign(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if strings.Contains(r.URL.Path, "/assignee") && r.Method == "PUT" {
			var body map[string]any
			if err := json.NewDecoder(r.Body).Decode(&body); err == nil {
				if body["accountId"] != nil {
					t.Error("expected null accountId for unassign")
				}
			}
			w.WriteHeader(http.StatusNoContent)
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer ts.Close()

	var stdout, stderr bytes.Buffer
	c := newTestClient(ts.URL, &stdout, &stderr)

	cmd := newAssignCmd(c)
	_ = cmd.Flags().Set("issue", "PROJ-1")
	_ = cmd.Flags().Set("to", "none")

	err := cmd.RunE(cmd, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	output := stdout.String()
	if !strings.Contains(output, "unassigned") {
		t.Errorf("expected 'unassigned' in output, got: %s", output)
	}
}

func TestRunAssign_UserSearch(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if strings.Contains(r.URL.Path, "/user/search") {
			fmt.Fprintln(w, `[{"accountId":"user1","displayName":"Test User"}]`)
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

	cmd := newAssignCmd(c)
	_ = cmd.Flags().Set("issue", "PROJ-1")
	_ = cmd.Flags().Set("to", "Test User")

	err := cmd.RunE(cmd, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	output := stdout.String()
	if !strings.Contains(output, "assigned") {
		t.Errorf("expected 'assigned' in output, got: %s", output)
	}
}

func TestRunAssign_NoClient(t *testing.T) {
	cmd := &cobra.Command{Use: "assign", RunE: runAssign}
	cmd.Flags().String("issue", "", "")
	cmd.Flags().String("to", "", "")
	cmd.SetContext(context.Background()) // context without client

	err := cmd.RunE(cmd, nil)
	if err == nil {
		t.Fatal("expected error when no client in context")
	}
}

func TestRunAssign_ResolveError(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		fmt.Fprintln(w, `{"message":"server error"}`)
	}))
	defer ts.Close()

	var stdout, stderr bytes.Buffer
	c := newTestClient(ts.URL, &stdout, &stderr)

	cmd := newAssignCmd(c)
	_ = cmd.Flags().Set("issue", "PROJ-1")
	_ = cmd.Flags().Set("to", "me")

	err := cmd.RunE(cmd, nil)
	if err == nil {
		t.Fatal("expected error when resolve fails")
	}
}

func TestRunAssign_UnassignHTTPError(t *testing.T) {
	callCount := 0
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		if callCount == 1 && strings.Contains(r.URL.Path, "/assignee") {
			w.WriteHeader(http.StatusForbidden)
			fmt.Fprintln(w, `{"message":"forbidden"}`)
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer ts.Close()

	var stdout, stderr bytes.Buffer
	c := newTestClient(ts.URL, &stdout, &stderr)

	cmd := newAssignCmd(c)
	_ = cmd.Flags().Set("issue", "PROJ-1")
	_ = cmd.Flags().Set("to", "none")

	err := cmd.RunE(cmd, nil)
	if err == nil {
		t.Fatal("expected error when PUT assignee fails")
	}
}

func TestRunComment_DryRun(t *testing.T) {
	var stdout, stderr bytes.Buffer
	c := newTestClient("http://unused", &stdout, &stderr)
	c.DryRun = true

	cmd := newCommentCmd(c)
	_ = cmd.Flags().Set("issue", "PROJ-1")
	_ = cmd.Flags().Set("text", "This is done")

	err := cmd.RunE(cmd, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	output := stdout.String()
	if !strings.Contains(output, "POST") {
		t.Errorf("expected POST in dry-run output, got: %s", output)
	}
	if !strings.Contains(output, "PROJ-1") {
		t.Errorf("expected issue key in dry-run output, got: %s", output)
	}
}

func TestRunComment_Success(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Method == "POST" && strings.Contains(r.URL.Path, "/comment") {
			w.WriteHeader(http.StatusCreated)
			fmt.Fprintln(w, `{"id":"10001"}`)
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer ts.Close()

	var stdout, stderr bytes.Buffer
	c := newTestClient(ts.URL, &stdout, &stderr)

	cmd := newCommentCmd(c)
	_ = cmd.Flags().Set("issue", "PROJ-1")
	_ = cmd.Flags().Set("text", "This is done")

	err := cmd.RunE(cmd, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	output := stdout.String()
	if !strings.Contains(output, "commented") {
		t.Errorf("expected 'commented' in output, got: %s", output)
	}
}

func TestRunComment_NoClient(t *testing.T) {
	cmd := &cobra.Command{Use: "comment", RunE: runComment}
	cmd.Flags().String("issue", "", "")
	cmd.Flags().String("text", "", "")
	cmd.SetContext(context.Background()) // context without client

	err := cmd.RunE(cmd, nil)
	if err == nil {
		t.Fatal("expected error when no client in context")
	}
}

// newCreateIssueCmd creates a workflow create command with flags and client context.
func newCreateIssueCmd(c *client.Client) *cobra.Command {
	cmd := &cobra.Command{Use: "create", RunE: runCreateIssue}
	cmd.Flags().String("project", "", "")
	cmd.Flags().String("type", "", "")
	cmd.Flags().String("summary", "", "")
	cmd.Flags().String("description", "", "")
	cmd.Flags().String("assign", "", "")
	cmd.Flags().String("priority", "", "")
	cmd.Flags().String("labels", "", "")
	cmd.Flags().String("parent", "", "")
	cmd.SetContext(client.NewContext(context.Background(), c))
	return cmd
}

func TestRunCreateIssue_DryRun(t *testing.T) {
	var stdout, stderr bytes.Buffer
	c := newTestClient("http://unused", &stdout, &stderr)
	c.DryRun = true

	cmd := newCreateIssueCmd(c)
	_ = cmd.Flags().Set("project", "PROJ")
	_ = cmd.Flags().Set("type", "Bug")
	_ = cmd.Flags().Set("summary", "Login broken")

	err := cmd.RunE(cmd, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	output := stdout.String()
	if !strings.Contains(output, "POST") {
		t.Errorf("expected POST in dry-run output, got: %s", output)
	}
	if !strings.Contains(output, "/rest/api/3/issue") {
		t.Errorf("expected issue endpoint in dry-run output, got: %s", output)
	}
}

func TestRunCreateIssue_Success(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Method == "POST" && r.URL.Path == "/rest/api/3/issue" {
			w.WriteHeader(http.StatusCreated)
			fmt.Fprintln(w, `{"id":"10001","key":"PROJ-1","self":"http://example.com/rest/api/3/issue/10001"}`)
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer ts.Close()

	var stdout, stderr bytes.Buffer
	c := newTestClient(ts.URL, &stdout, &stderr)

	cmd := newCreateIssueCmd(c)
	_ = cmd.Flags().Set("project", "PROJ")
	_ = cmd.Flags().Set("type", "Bug")
	_ = cmd.Flags().Set("summary", "Login broken")

	err := cmd.RunE(cmd, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	output := stdout.String()
	if !strings.Contains(output, "PROJ-1") {
		t.Errorf("expected 'PROJ-1' in output, got: %s", output)
	}
}

func TestRunCreateIssue_WithAllFlags(t *testing.T) {
	var capturedBody []byte
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Method == "POST" && r.URL.Path == "/rest/api/3/issue" {
			var err error
			capturedBody, err = io.ReadAll(r.Body)
			if err != nil {
				w.WriteHeader(http.StatusInternalServerError)
				return
			}
			w.WriteHeader(http.StatusCreated)
			fmt.Fprintln(w, `{"id":"10002","key":"PROJ-2","self":"http://example.com/rest/api/3/issue/10002"}`)
			return
		}
		if r.Method == "GET" && strings.Contains(r.URL.Path, "/myself") {
			fmt.Fprintln(w, `{"accountId":"abc123"}`)
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer ts.Close()

	var stdout, stderr bytes.Buffer
	c := newTestClient(ts.URL, &stdout, &stderr)

	cmd := newCreateIssueCmd(c)
	_ = cmd.Flags().Set("project", "PROJ")
	_ = cmd.Flags().Set("type", "Bug")
	_ = cmd.Flags().Set("summary", "Login broken")
	_ = cmd.Flags().Set("description", "Steps to reproduce")
	_ = cmd.Flags().Set("assign", "me")
	_ = cmd.Flags().Set("priority", "High")
	_ = cmd.Flags().Set("labels", "bug,urgent")
	_ = cmd.Flags().Set("parent", "PROJ-100")

	err := cmd.RunE(cmd, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	body := string(capturedBody)
	if !strings.Contains(body, `"High"`) {
		t.Errorf("expected priority 'High' in request body, got: %s", body)
	}
	if !strings.Contains(body, "PROJ-100") {
		t.Errorf("expected parent 'PROJ-100' in request body, got: %s", body)
	}
	if !strings.Contains(body, "bug") {
		t.Errorf("expected label 'bug' in request body, got: %s", body)
	}
	if !strings.Contains(body, "urgent") {
		t.Errorf("expected label 'urgent' in request body, got: %s", body)
	}
}

// newMoveCmd creates a workflow move command with flags and client context.
func newMoveCmd(c *client.Client) *cobra.Command {
	cmd := &cobra.Command{Use: "move", RunE: runMove}
	cmd.Flags().String("issue", "", "")
	cmd.Flags().String("to", "", "")
	cmd.Flags().String("assign", "", "")
	cmd.SetContext(client.NewContext(context.Background(), c))
	return cmd
}

func TestRunMove_DryRun(t *testing.T) {
	var stdout, stderr bytes.Buffer
	c := newTestClient("http://unused", &stdout, &stderr)
	c.DryRun = true

	cmd := newMoveCmd(c)
	_ = cmd.Flags().Set("issue", "PROJ-1")
	_ = cmd.Flags().Set("to", "In Progress")
	_ = cmd.Flags().Set("assign", "me")

	err := cmd.RunE(cmd, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	output := stdout.String()
	if !strings.Contains(output, "POST") {
		t.Errorf("expected POST in dry-run output, got: %s", output)
	}
	if !strings.Contains(output, "PROJ-1") {
		t.Errorf("expected issue key in dry-run output, got: %s", output)
	}
	if !strings.Contains(output, "assign") {
		t.Errorf("expected assign note in dry-run output, got: %s", output)
	}
}

func TestRunMove_TransitionOnly(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Method == "GET" && strings.Contains(r.URL.Path, "/transitions") {
			fmt.Fprintln(w, `{"transitions":[{"id":"41","name":"In Progress","to":{"name":"In Progress"}}]}`)
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

	cmd := newMoveCmd(c)
	_ = cmd.Flags().Set("issue", "PROJ-1")
	_ = cmd.Flags().Set("to", "In Progress")

	err := cmd.RunE(cmd, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	output := stdout.String()
	if !strings.Contains(output, "moved") {
		t.Errorf("expected 'moved' in output, got: %s", output)
	}
	if strings.Contains(output, "assigned") {
		t.Errorf("did not expect 'assigned' in transition-only output, got: %s", output)
	}
}

func TestRunMove_WithAssign(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Method == "GET" && strings.Contains(r.URL.Path, "/transitions") {
			fmt.Fprintln(w, `{"transitions":[{"id":"41","name":"In Progress","to":{"name":"In Progress"}}]}`)
			return
		}
		if r.Method == "POST" && strings.Contains(r.URL.Path, "/transitions") {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		if r.Method == "GET" && strings.Contains(r.URL.Path, "/myself") {
			fmt.Fprintln(w, `{"accountId":"user123"}`)
			return
		}
		if r.Method == "PUT" && strings.Contains(r.URL.Path, "/assignee") {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer ts.Close()

	var stdout, stderr bytes.Buffer
	c := newTestClient(ts.URL, &stdout, &stderr)

	cmd := newMoveCmd(c)
	_ = cmd.Flags().Set("issue", "PROJ-1")
	_ = cmd.Flags().Set("to", "In Progress")
	_ = cmd.Flags().Set("assign", "me")

	err := cmd.RunE(cmd, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	output := stdout.String()
	if !strings.Contains(output, "moved") {
		t.Errorf("expected 'moved' in output, got: %s", output)
	}
	if !strings.Contains(output, "assigned") {
		t.Errorf("expected 'assigned' in output, got: %s", output)
	}
}

// newLinkCmd creates a workflow link command with flags and client context.
func newLinkCmd(c *client.Client) *cobra.Command {
	cmd := &cobra.Command{Use: "link", RunE: runLink}
	cmd.Flags().String("from", "", "")
	cmd.Flags().String("to", "", "")
	cmd.Flags().String("type", "", "")
	cmd.SetContext(client.NewContext(context.Background(), c))
	return cmd
}

func TestRunLink_DryRun(t *testing.T) {
	var stdout, stderr bytes.Buffer
	c := newTestClient("http://unused", &stdout, &stderr)
	c.DryRun = true

	cmd := newLinkCmd(c)
	_ = cmd.Flags().Set("from", "PROJ-1")
	_ = cmd.Flags().Set("to", "PROJ-2")
	_ = cmd.Flags().Set("type", "blocks")

	err := cmd.RunE(cmd, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	output := stdout.String()
	if !strings.Contains(output, "POST") {
		t.Errorf("expected POST in dry-run output, got: %s", output)
	}
	if !strings.Contains(output, "/rest/api/3/issueLink") {
		t.Errorf("expected issueLink endpoint in dry-run output, got: %s", output)
	}
	if !strings.Contains(output, "PROJ-1") {
		t.Errorf("expected PROJ-1 in dry-run output, got: %s", output)
	}
}

func TestRunLink_Success(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Method == "GET" && r.URL.Path == "/rest/api/3/issueLinkType" {
			fmt.Fprintln(w, `{"issueLinkTypes":[{"id":"10000","name":"Blocks","inward":"is blocked by","outward":"blocks"}]}`)
			return
		}
		if r.Method == "POST" && r.URL.Path == "/rest/api/3/issueLink" {
			w.WriteHeader(http.StatusCreated)
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer ts.Close()

	var stdout, stderr bytes.Buffer
	c := newTestClient(ts.URL, &stdout, &stderr)

	cmd := newLinkCmd(c)
	_ = cmd.Flags().Set("from", "PROJ-1")
	_ = cmd.Flags().Set("to", "PROJ-2")
	_ = cmd.Flags().Set("type", "blocks")

	err := cmd.RunE(cmd, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	output := stdout.String()
	if !strings.Contains(output, "linked") {
		t.Errorf("expected 'linked' in output, got: %s", output)
	}
	if !strings.Contains(output, "PROJ-1") {
		t.Errorf("expected PROJ-1 in output, got: %s", output)
	}
	if !strings.Contains(output, "PROJ-2") {
		t.Errorf("expected PROJ-2 in output, got: %s", output)
	}
	if !strings.Contains(output, "Blocks") {
		t.Errorf("expected resolved type 'Blocks' in output, got: %s", output)
	}
}

func TestRunLink_TypeNotFound(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Method == "GET" && r.URL.Path == "/rest/api/3/issueLinkType" {
			fmt.Fprintln(w, `{"issueLinkTypes":[{"id":"10000","name":"Blocks","inward":"is blocked by","outward":"blocks"}]}`)
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer ts.Close()

	var stdout, stderr bytes.Buffer
	c := newTestClient(ts.URL, &stdout, &stderr)

	cmd := newLinkCmd(c)
	_ = cmd.Flags().Set("from", "PROJ-1")
	_ = cmd.Flags().Set("to", "PROJ-2")
	_ = cmd.Flags().Set("type", "nonexistent")

	err := cmd.RunE(cmd, nil)
	if err == nil {
		t.Fatal("expected error for unknown link type")
	}
	aw, ok := err.(*jrerrors.AlreadyWrittenError)
	if !ok {
		t.Fatalf("expected AlreadyWrittenError, got %T", err)
	}
	if aw.Code != jrerrors.ExitValidation {
		t.Errorf("expected exit %d, got %d", jrerrors.ExitValidation, aw.Code)
	}
	if !strings.Contains(stderr.String(), "nonexistent") {
		t.Errorf("expected type name in error output, got: %s", stderr.String())
	}
}

// newLogWorkCmd creates a workflow log-work command with flags and client context.
func newLogWorkCmd(c *client.Client) *cobra.Command {
	cmd := &cobra.Command{Use: "log-work", RunE: runLogWork}
	cmd.Flags().String("issue", "", "")
	cmd.Flags().String("time", "", "")
	cmd.Flags().String("comment", "", "")
	cmd.SetContext(client.NewContext(context.Background(), c))
	return cmd
}

func TestRunLogWork_DryRun(t *testing.T) {
	var stdout, stderr bytes.Buffer
	c := newTestClient("http://unused", &stdout, &stderr)
	c.DryRun = true

	cmd := newLogWorkCmd(c)
	_ = cmd.Flags().Set("issue", "PROJ-1")
	_ = cmd.Flags().Set("time", "2h 30m")

	err := cmd.RunE(cmd, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	output := stdout.String()
	if !strings.Contains(output, "POST") {
		t.Errorf("expected POST in dry-run output, got: %s", output)
	}
	if !strings.Contains(output, "PROJ-1") {
		t.Errorf("expected issue key in dry-run output, got: %s", output)
	}
	if !strings.Contains(output, "9000") {
		t.Errorf("expected timeSpentSeconds=9000 in dry-run output, got: %s", output)
	}
}

func TestRunLogWork_Success(t *testing.T) {
	var receivedBody []byte
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Method == "POST" && strings.Contains(r.URL.Path, "/worklog") {
			var err error
			receivedBody, err = io.ReadAll(r.Body)
			if err != nil {
				w.WriteHeader(http.StatusInternalServerError)
				return
			}
			w.WriteHeader(http.StatusCreated)
			fmt.Fprintln(w, `{"id":"10001"}`)
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer ts.Close()

	var stdout, stderr bytes.Buffer
	c := newTestClient(ts.URL, &stdout, &stderr)

	cmd := newLogWorkCmd(c)
	_ = cmd.Flags().Set("issue", "PROJ-1")
	_ = cmd.Flags().Set("time", "2h 30m")

	err := cmd.RunE(cmd, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	output := stdout.String()
	if !strings.Contains(output, "logged") {
		t.Errorf("expected 'logged' in output, got: %s", output)
	}

	body := string(receivedBody)
	if !strings.Contains(body, "9000") {
		t.Errorf("expected timeSpentSeconds=9000 in request body, got: %s", body)
	}
}

func TestRunLogWork_WithComment(t *testing.T) {
	var receivedBody []byte
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Method == "POST" && strings.Contains(r.URL.Path, "/worklog") {
			var err error
			receivedBody, err = io.ReadAll(r.Body)
			if err != nil {
				w.WriteHeader(http.StatusInternalServerError)
				return
			}
			w.WriteHeader(http.StatusCreated)
			fmt.Fprintln(w, `{"id":"10002"}`)
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer ts.Close()

	var stdout, stderr bytes.Buffer
	c := newTestClient(ts.URL, &stdout, &stderr)

	cmd := newLogWorkCmd(c)
	_ = cmd.Flags().Set("issue", "PROJ-1")
	_ = cmd.Flags().Set("time", "1h")
	_ = cmd.Flags().Set("comment", "Fixed the bug")

	err := cmd.RunE(cmd, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	body := string(receivedBody)
	if !strings.Contains(body, "comment") {
		t.Errorf("expected 'comment' field in request body, got: %s", body)
	}
	if !strings.Contains(body, "Fixed the bug") {
		t.Errorf("expected comment text in request body, got: %s", body)
	}
}

func TestRunLogWork_InvalidDuration(t *testing.T) {
	var stdout, stderr bytes.Buffer
	c := newTestClient("http://unused", &stdout, &stderr)

	cmd := newLogWorkCmd(c)
	_ = cmd.Flags().Set("issue", "PROJ-1")
	_ = cmd.Flags().Set("time", "invalid")

	err := cmd.RunE(cmd, nil)
	if err == nil {
		t.Fatal("expected error for invalid duration")
	}
	aw, ok := err.(*jrerrors.AlreadyWrittenError)
	if !ok {
		t.Fatalf("expected AlreadyWrittenError, got %T", err)
	}
	if aw.Code != jrerrors.ExitValidation {
		t.Errorf("expected exit %d, got %d", jrerrors.ExitValidation, aw.Code)
	}
}

// newSprintCmd creates a workflow sprint command with flags and client context.
func newSprintCmd(c *client.Client) *cobra.Command {
	cmd := &cobra.Command{Use: "sprint", RunE: runSprint}
	cmd.Flags().String("issue", "", "")
	cmd.Flags().String("to", "", "")
	cmd.SetContext(client.NewContext(context.Background(), c))
	return cmd
}

func TestRunSprint_DryRun(t *testing.T) {
	var stdout, stderr bytes.Buffer
	c := newTestClient("http://unused", &stdout, &stderr)
	c.DryRun = true

	cmd := newSprintCmd(c)
	_ = cmd.Flags().Set("issue", "PROJ-1")
	_ = cmd.Flags().Set("to", "Sprint 5")

	err := cmd.RunE(cmd, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	output := stdout.String()
	if !strings.Contains(output, "POST") {
		t.Errorf("expected POST in dry-run output, got: %s", output)
	}
	if !strings.Contains(output, "PROJ-1") {
		t.Errorf("expected issue key in dry-run output, got: %s", output)
	}
	if !strings.Contains(output, "Sprint 5") {
		t.Errorf("expected sprint name in dry-run output, got: %s", output)
	}
}

func TestRunSprint_Success(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Method == "GET" && r.URL.Path == "/rest/agile/1.0/board" {
			fmt.Fprintln(w, `{"values":[{"id":1,"name":"My Board"}]}`)
			return
		}
		if r.Method == "GET" && strings.Contains(r.URL.Path, "/sprint") {
			fmt.Fprintln(w, `{"values":[{"id":100,"name":"Sprint 5","state":"active"}]}`)
			return
		}
		if r.Method == "POST" && strings.Contains(r.URL.Path, "/sprint/100/issue") {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer ts.Close()

	var stdout, stderr bytes.Buffer
	c := newTestClient(ts.URL, &stdout, &stderr)

	cmd := newSprintCmd(c)
	_ = cmd.Flags().Set("issue", "PROJ-1")
	_ = cmd.Flags().Set("to", "Sprint 5")

	err := cmd.RunE(cmd, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	output := stdout.String()
	if !strings.Contains(output, "sprint_set") {
		t.Errorf("expected 'sprint_set' in output, got: %s", output)
	}
	if !strings.Contains(output, "Sprint 5") {
		t.Errorf("expected sprint name in output, got: %s", output)
	}
	if !strings.Contains(output, "PROJ-1") {
		t.Errorf("expected issue key in output, got: %s", output)
	}
}

func TestRunSprint_NotFound(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Method == "GET" && r.URL.Path == "/rest/agile/1.0/board" {
			fmt.Fprintln(w, `{"values":[{"id":1,"name":"My Board"}]}`)
			return
		}
		if r.Method == "GET" && strings.Contains(r.URL.Path, "/sprint") {
			fmt.Fprintln(w, `{"values":[{"id":100,"name":"Sprint 1","state":"active"}]}`)
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer ts.Close()

	var stdout, stderr bytes.Buffer
	c := newTestClient(ts.URL, &stdout, &stderr)

	cmd := newSprintCmd(c)
	_ = cmd.Flags().Set("issue", "PROJ-1")
	_ = cmd.Flags().Set("to", "Nonexistent Sprint")

	err := cmd.RunE(cmd, nil)
	if err == nil {
		t.Fatal("expected error for no matching sprint")
	}
	aw, ok := err.(*jrerrors.AlreadyWrittenError)
	if !ok {
		t.Fatalf("expected AlreadyWrittenError, got %T", err)
	}
	if aw.Code != jrerrors.ExitNotFound {
		t.Errorf("expected exit %d, got %d", jrerrors.ExitNotFound, aw.Code)
	}
	if !strings.Contains(stderr.String(), "Nonexistent Sprint") {
		t.Errorf("expected sprint name in error output, got: %s", stderr.String())
	}
}

func TestRunCreateIssue_NoClient(t *testing.T) {
	cmd := &cobra.Command{Use: "create", RunE: runCreateIssue}
	cmd.Flags().String("project", "", "")
	cmd.Flags().String("type", "", "")
	cmd.Flags().String("summary", "", "")
	cmd.Flags().String("description", "", "")
	cmd.Flags().String("assign", "", "")
	cmd.Flags().String("priority", "", "")
	cmd.Flags().String("labels", "", "")
	cmd.Flags().String("parent", "", "")
	cmd.SetContext(context.Background()) // context without client

	err := cmd.RunE(cmd, nil)
	if err == nil {
		t.Fatal("expected error when no client in context")
	}
}

// ---------------------------------------------------------------------------
// runMove — additional branch coverage
// ---------------------------------------------------------------------------

func TestRunMove_DryRunNoAssign(t *testing.T) {
	var stdout, stderr bytes.Buffer
	c := newTestClient("http://unused", &stdout, &stderr)
	c.DryRun = true

	cmd := newMoveCmd(c)
	_ = cmd.Flags().Set("issue", "PROJ-1")
	_ = cmd.Flags().Set("to", "In Progress")
	// Intentionally do NOT set --assign.

	err := cmd.RunE(cmd, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	output := stdout.String()
	if !strings.Contains(output, "POST") {
		t.Errorf("expected POST in dry-run output, got: %s", output)
	}
	if strings.Contains(output, "assign") {
		t.Errorf("did not expect 'assign' in dry-run output when --assign is empty, got: %s", output)
	}
}

func TestRunMove_WithUnassign(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Method == "GET" && strings.Contains(r.URL.Path, "/transitions") {
			fmt.Fprintln(w, `{"transitions":[{"id":"41","name":"In Progress","to":{"name":"In Progress"}}]}`)
			return
		}
		if r.Method == "POST" && strings.Contains(r.URL.Path, "/transitions") {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		if r.Method == "PUT" && strings.Contains(r.URL.Path, "/assignee") {
			var body map[string]any
			if err := json.NewDecoder(r.Body).Decode(&body); err == nil {
				if body["accountId"] != nil {
					w.WriteHeader(http.StatusBadRequest)
					return
				}
			}
			w.WriteHeader(http.StatusNoContent)
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer ts.Close()

	var stdout, stderr bytes.Buffer
	c := newTestClient(ts.URL, &stdout, &stderr)

	cmd := newMoveCmd(c)
	_ = cmd.Flags().Set("issue", "PROJ-1")
	_ = cmd.Flags().Set("to", "In Progress")
	_ = cmd.Flags().Set("assign", "none")

	err := cmd.RunE(cmd, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	output := stdout.String()
	if !strings.Contains(output, "moved") {
		t.Errorf("expected 'moved' in output, got: %s", output)
	}
	if !strings.Contains(output, "unassigned") {
		t.Errorf("expected 'unassigned' in output, got: %s", output)
	}
}

func TestRunMove_NoClient(t *testing.T) {
	cmd := &cobra.Command{Use: "move", RunE: runMove}
	cmd.Flags().String("issue", "", "")
	cmd.Flags().String("to", "", "")
	cmd.Flags().String("assign", "", "")
	cmd.SetContext(context.Background())

	err := cmd.RunE(cmd, nil)
	if err == nil {
		t.Fatal("expected error when no client in context")
	}
}

func TestRunMove_TransitionFetchFails(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		fmt.Fprintln(w, `{"message":"server error"}`)
	}))
	defer ts.Close()

	var stdout, stderr bytes.Buffer
	c := newTestClient(ts.URL, &stdout, &stderr)

	cmd := newMoveCmd(c)
	_ = cmd.Flags().Set("issue", "PROJ-1")
	_ = cmd.Flags().Set("to", "Done")

	err := cmd.RunE(cmd, nil)
	if err == nil {
		t.Fatal("expected error when transition fetch fails")
	}
}

func TestRunMove_AssignPUTFails(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Method == "GET" && strings.Contains(r.URL.Path, "/transitions") {
			fmt.Fprintln(w, `{"transitions":[{"id":"41","name":"In Progress","to":{"name":"In Progress"}}]}`)
			return
		}
		if r.Method == "POST" && strings.Contains(r.URL.Path, "/transitions") {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		if r.Method == "GET" && strings.Contains(r.URL.Path, "/myself") {
			fmt.Fprintln(w, `{"accountId":"user123"}`)
			return
		}
		if r.Method == "PUT" && strings.Contains(r.URL.Path, "/assignee") {
			w.WriteHeader(http.StatusForbidden)
			fmt.Fprintln(w, `{"message":"forbidden"}`)
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer ts.Close()

	var stdout, stderr bytes.Buffer
	c := newTestClient(ts.URL, &stdout, &stderr)

	cmd := newMoveCmd(c)
	_ = cmd.Flags().Set("issue", "PROJ-1")
	_ = cmd.Flags().Set("to", "In Progress")
	_ = cmd.Flags().Set("assign", "me")

	err := cmd.RunE(cmd, nil)
	if err == nil {
		t.Fatal("expected error when PUT assignee returns 403")
	}
}

// ---------------------------------------------------------------------------
// resolveLinkType / runLink — additional branch coverage
// ---------------------------------------------------------------------------

func TestRunLink_NoClient(t *testing.T) {
	cmd := &cobra.Command{Use: "link", RunE: runLink}
	cmd.Flags().String("from", "", "")
	cmd.Flags().String("to", "", "")
	cmd.Flags().String("type", "", "")
	cmd.SetContext(context.Background())

	err := cmd.RunE(cmd, nil)
	if err == nil {
		t.Fatal("expected error when no client in context")
	}
}

func TestRunLink_SubstringMatch(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Method == "GET" && r.URL.Path == "/rest/api/3/issueLinkType" {
			// "Blocks" is the canonical name; requesting "block" (partial) should resolve it.
			fmt.Fprintln(w, `{"issueLinkTypes":[{"id":"10000","name":"Blocks","inward":"is blocked by","outward":"blocks"}]}`)
			return
		}
		if r.Method == "POST" && r.URL.Path == "/rest/api/3/issueLink" {
			w.WriteHeader(http.StatusCreated)
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer ts.Close()

	var stdout, stderr bytes.Buffer
	c := newTestClient(ts.URL, &stdout, &stderr)

	cmd := newLinkCmd(c)
	_ = cmd.Flags().Set("from", "PROJ-1")
	_ = cmd.Flags().Set("to", "PROJ-2")
	_ = cmd.Flags().Set("type", "block") // partial — should match "Blocks"

	err := cmd.RunE(cmd, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	output := stdout.String()
	if !strings.Contains(output, "linked") {
		t.Errorf("expected 'linked' in output, got: %s", output)
	}
	if !strings.Contains(output, "Blocks") {
		t.Errorf("expected resolved type 'Blocks' in output, got: %s", output)
	}
}

func TestRunLink_LinkTypeFetchFails(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		fmt.Fprintln(w, `{"message":"server error"}`)
	}))
	defer ts.Close()

	var stdout, stderr bytes.Buffer
	c := newTestClient(ts.URL, &stdout, &stderr)

	cmd := newLinkCmd(c)
	_ = cmd.Flags().Set("from", "PROJ-1")
	_ = cmd.Flags().Set("to", "PROJ-2")
	_ = cmd.Flags().Set("type", "blocks")

	err := cmd.RunE(cmd, nil)
	if err == nil {
		t.Fatal("expected error when issueLinkType fetch returns 500")
	}
}

func TestRunLink_LinkTypeMalformedJSON(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Method == "GET" && r.URL.Path == "/rest/api/3/issueLinkType" {
			w.WriteHeader(http.StatusOK)
			fmt.Fprint(w, "not json")
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer ts.Close()

	var stdout, stderr bytes.Buffer
	c := newTestClient(ts.URL, &stdout, &stderr)

	cmd := newLinkCmd(c)
	_ = cmd.Flags().Set("from", "PROJ-1")
	_ = cmd.Flags().Set("to", "PROJ-2")
	_ = cmd.Flags().Set("type", "blocks")

	err := cmd.RunE(cmd, nil)
	if err == nil {
		t.Fatal("expected error when issueLinkType response is malformed JSON")
	}
}

func TestRunLink_PostFails(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Method == "GET" && r.URL.Path == "/rest/api/3/issueLinkType" {
			fmt.Fprintln(w, `{"issueLinkTypes":[{"id":"10000","name":"Blocks","inward":"is blocked by","outward":"blocks"}]}`)
			return
		}
		if r.Method == "POST" && r.URL.Path == "/rest/api/3/issueLink" {
			w.WriteHeader(http.StatusForbidden)
			fmt.Fprintln(w, `{"message":"forbidden"}`)
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer ts.Close()

	var stdout, stderr bytes.Buffer
	c := newTestClient(ts.URL, &stdout, &stderr)

	cmd := newLinkCmd(c)
	_ = cmd.Flags().Set("from", "PROJ-1")
	_ = cmd.Flags().Set("to", "PROJ-2")
	_ = cmd.Flags().Set("type", "blocks")

	err := cmd.RunE(cmd, nil)
	if err == nil {
		t.Fatal("expected error when POST issueLink returns 403")
	}
}

// ---------------------------------------------------------------------------
// runLogWork — additional branch coverage
// ---------------------------------------------------------------------------

func TestRunLogWork_NoClient(t *testing.T) {
	cmd := &cobra.Command{Use: "log-work", RunE: runLogWork}
	cmd.Flags().String("issue", "", "")
	cmd.Flags().String("time", "", "")
	cmd.Flags().String("comment", "", "")
	cmd.SetContext(context.Background())

	err := cmd.RunE(cmd, nil)
	if err == nil {
		t.Fatal("expected error when no client in context")
	}
}

func TestRunLogWork_PostFails(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		fmt.Fprintln(w, `{"message":"server error"}`)
	}))
	defer ts.Close()

	var stdout, stderr bytes.Buffer
	c := newTestClient(ts.URL, &stdout, &stderr)

	cmd := newLogWorkCmd(c)
	_ = cmd.Flags().Set("issue", "PROJ-1")
	_ = cmd.Flags().Set("time", "1h")

	err := cmd.RunE(cmd, nil)
	if err == nil {
		t.Fatal("expected error when POST worklog returns 500")
	}
}

// ---------------------------------------------------------------------------
// runSprint — additional branch coverage
// ---------------------------------------------------------------------------

func TestRunSprint_NoClient(t *testing.T) {
	cmd := &cobra.Command{Use: "sprint", RunE: runSprint}
	cmd.Flags().String("issue", "", "")
	cmd.Flags().String("to", "", "")
	cmd.SetContext(context.Background())

	err := cmd.RunE(cmd, nil)
	if err == nil {
		t.Fatal("expected error when no client in context")
	}
}

func TestRunSprint_BoardFetchFails(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		fmt.Fprintln(w, `{"message":"server error"}`)
	}))
	defer ts.Close()

	var stdout, stderr bytes.Buffer
	c := newTestClient(ts.URL, &stdout, &stderr)

	cmd := newSprintCmd(c)
	_ = cmd.Flags().Set("issue", "PROJ-1")
	_ = cmd.Flags().Set("to", "Sprint 5")

	err := cmd.RunE(cmd, nil)
	if err == nil {
		t.Fatal("expected error when board fetch returns 500")
	}
}

func TestRunSprint_PostFails(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Method == "GET" && r.URL.Path == "/rest/agile/1.0/board" {
			fmt.Fprintln(w, `{"values":[{"id":1,"name":"My Board"}]}`)
			return
		}
		if r.Method == "GET" && strings.Contains(r.URL.Path, "/sprint") {
			fmt.Fprintln(w, `{"values":[{"id":100,"name":"Sprint 5","state":"active"}]}`)
			return
		}
		if r.Method == "POST" && strings.Contains(r.URL.Path, "/sprint/100/issue") {
			w.WriteHeader(http.StatusInternalServerError)
			fmt.Fprintln(w, `{"message":"server error"}`)
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer ts.Close()

	var stdout, stderr bytes.Buffer
	c := newTestClient(ts.URL, &stdout, &stderr)

	cmd := newSprintCmd(c)
	_ = cmd.Flags().Set("issue", "PROJ-1")
	_ = cmd.Flags().Set("to", "Sprint 5")

	err := cmd.RunE(cmd, nil)
	if err == nil {
		t.Fatal("expected error when POST sprint issue returns 500")
	}
}

// ---------------------------------------------------------------------------
// Gap 1: runAssign — normal-assign PUT fails after /myself succeeds
// ---------------------------------------------------------------------------

func TestRunAssign_NormalAssignPUTFails(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if strings.Contains(r.URL.Path, "/myself") {
			fmt.Fprintln(w, `{"accountId":"abc123"}`)
			return
		}
		if r.Method == "PUT" && strings.Contains(r.URL.Path, "/assignee") {
			w.WriteHeader(http.StatusInternalServerError)
			fmt.Fprintln(w, `{"message":"server error"}`)
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer ts.Close()

	var stdout, stderr bytes.Buffer
	c := newTestClient(ts.URL, &stdout, &stderr)

	cmd := newAssignCmd(c)
	_ = cmd.Flags().Set("issue", "TEST-1")
	_ = cmd.Flags().Set("to", "me")

	err := cmd.RunE(cmd, nil)
	if err == nil {
		t.Fatal("expected error when PUT assignee returns 500")
	}
	aw, ok := err.(*jrerrors.AlreadyWrittenError)
	if !ok {
		t.Fatalf("expected AlreadyWrittenError, got %T", err)
	}
	if aw.Code == jrerrors.ExitOK {
		t.Errorf("expected non-zero exit code, got ExitOK")
	}
}

// ---------------------------------------------------------------------------
// Gap 2: runComment — POST /comment fails with 500
// ---------------------------------------------------------------------------

func TestRunComment_PostFails(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		fmt.Fprintln(w, `{"message":"server error"}`)
	}))
	defer ts.Close()

	var stdout, stderr bytes.Buffer
	c := newTestClient(ts.URL, &stdout, &stderr)

	cmd := newCommentCmd(c)
	_ = cmd.Flags().Set("issue", "PROJ-1")
	_ = cmd.Flags().Set("text", "This should fail")

	err := cmd.RunE(cmd, nil)
	if err == nil {
		t.Fatal("expected error when POST comment returns 500")
	}
	aw, ok := err.(*jrerrors.AlreadyWrittenError)
	if !ok {
		t.Fatalf("expected AlreadyWrittenError, got %T", err)
	}
	if aw.Code == jrerrors.ExitOK {
		t.Errorf("expected non-zero exit code, got ExitOK")
	}
}

// ---------------------------------------------------------------------------
// Gap 3: runCreateIssue — POST /issue fails with 500
// ---------------------------------------------------------------------------

func TestRunCreateIssue_PostFails(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		fmt.Fprintln(w, `{"message":"server error"}`)
	}))
	defer ts.Close()

	var stdout, stderr bytes.Buffer
	c := newTestClient(ts.URL, &stdout, &stderr)

	cmd := newCreateIssueCmd(c)
	_ = cmd.Flags().Set("project", "PROJ")
	_ = cmd.Flags().Set("type", "Bug")
	_ = cmd.Flags().Set("summary", "Login broken")

	err := cmd.RunE(cmd, nil)
	if err == nil {
		t.Fatal("expected error when POST issue returns 500")
	}
	aw, ok := err.(*jrerrors.AlreadyWrittenError)
	if !ok {
		t.Fatalf("expected AlreadyWrittenError, got %T", err)
	}
	if aw.Code == jrerrors.ExitOK {
		t.Errorf("expected non-zero exit code, got ExitOK")
	}
}

// ---------------------------------------------------------------------------
// Gap 4: runCreateIssue — resolveAssignee fails (GET /myself returns 500)
// ---------------------------------------------------------------------------

func TestRunCreateIssue_ResolveAssigneeFails(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Return 500 for everything, including /myself, to make resolveAssignee fail.
		w.WriteHeader(http.StatusInternalServerError)
		fmt.Fprintln(w, `{"message":"server error"}`)
	}))
	defer ts.Close()

	var stdout, stderr bytes.Buffer
	c := newTestClient(ts.URL, &stdout, &stderr)

	cmd := newCreateIssueCmd(c)
	_ = cmd.Flags().Set("project", "PROJ")
	_ = cmd.Flags().Set("type", "Bug")
	_ = cmd.Flags().Set("summary", "Login broken")
	_ = cmd.Flags().Set("assign", "me")

	err := cmd.RunE(cmd, nil)
	if err == nil {
		t.Fatal("expected error when resolveAssignee fails")
	}
	aw, ok := err.(*jrerrors.AlreadyWrittenError)
	if !ok {
		t.Fatalf("expected AlreadyWrittenError, got %T", err)
	}
	if aw.Code == jrerrors.ExitOK {
		t.Errorf("expected non-zero exit code, got ExitOK")
	}
}

// ---------------------------------------------------------------------------
// Gap 5: resolveSprint — malformed JSON for /rest/agile/1.0/board response
// ---------------------------------------------------------------------------

func TestResolveSprint_MalformedBoardsJSON(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Method == "GET" && r.URL.Path == "/rest/agile/1.0/board" {
			// Return invalid JSON to trigger the unmarshal error path.
			w.WriteHeader(http.StatusOK)
			fmt.Fprint(w, "not valid json {{{")
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer ts.Close()

	var stdout, stderr bytes.Buffer
	c := newTestClient(ts.URL, &stdout, &stderr)

	cmd := newSprintCmd(c)
	_ = cmd.Flags().Set("issue", "PROJ-1")
	_ = cmd.Flags().Set("to", "Sprint 5")

	err := cmd.RunE(cmd, nil)
	if err == nil {
		t.Fatal("expected error when boards response is malformed JSON")
	}
	aw, ok := err.(*jrerrors.AlreadyWrittenError)
	if !ok {
		t.Fatalf("expected AlreadyWrittenError, got %T", err)
	}
	if aw.Code == jrerrors.ExitOK {
		t.Errorf("expected non-zero exit code, got ExitOK")
	}
}

// ---------------------------------------------------------------------------
// Gap 6: resolveSprint — sprint endpoint returns invalid JSON (skip, not_found)
// ---------------------------------------------------------------------------

func TestResolveSprint_SprintEndpointMalformedJSON(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Method == "GET" && r.URL.Path == "/rest/agile/1.0/board" {
			// Return a valid board so the loop runs.
			fmt.Fprintln(w, `{"values":[{"id":42,"name":"Test Board"}]}`)
			return
		}
		if r.Method == "GET" && strings.Contains(r.URL.Path, "/sprint") {
			// Return garbage JSON for the sprint endpoint — should be skipped.
			w.WriteHeader(http.StatusOK)
			fmt.Fprint(w, "garbage {{{")
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer ts.Close()

	var stdout, stderr bytes.Buffer
	c := newTestClient(ts.URL, &stdout, &stderr)

	cmd := newSprintCmd(c)
	_ = cmd.Flags().Set("issue", "PROJ-1")
	_ = cmd.Flags().Set("to", "Sprint 5")

	err := cmd.RunE(cmd, nil)
	if err == nil {
		t.Fatal("expected not_found error when sprint endpoint returns malformed JSON")
	}
	aw, ok := err.(*jrerrors.AlreadyWrittenError)
	if !ok {
		t.Fatalf("expected AlreadyWrittenError, got %T", err)
	}
	if aw.Code != jrerrors.ExitNotFound {
		t.Errorf("expected ExitNotFound (%d), got %d", jrerrors.ExitNotFound, aw.Code)
	}
}

// ---------------------------------------------------------------------------
// resolveTransition — malformed JSON from GET /transitions
// ---------------------------------------------------------------------------

func TestResolveTransition_MalformedJSON(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Method == "GET" && strings.Contains(r.URL.Path, "/transitions") {
			w.WriteHeader(http.StatusOK)
			fmt.Fprint(w, "not json")
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer ts.Close()

	var stdout, stderr bytes.Buffer
	c := newTestClient(ts.URL, &stdout, &stderr)

	cmd := newTransitionCmd(c)
	_ = cmd.Flags().Set("issue", "TEST-1")
	_ = cmd.Flags().Set("to", "Done")

	err := cmd.RunE(cmd, nil)
	if err == nil {
		t.Fatal("expected error when transitions response is malformed JSON")
	}
	aw, ok := err.(*jrerrors.AlreadyWrittenError)
	if !ok {
		t.Fatalf("expected AlreadyWrittenError, got %T", err)
	}
	if aw.Code == jrerrors.ExitOK {
		t.Errorf("expected non-zero exit code, got ExitOK")
	}
}

// ---------------------------------------------------------------------------
// runTransition — GET transitions OK but POST transitions returns 500
// ---------------------------------------------------------------------------

func TestRunTransition_PostFails(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "GET" && strings.Contains(r.URL.Path, "/transitions") {
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprintln(w, `{"transitions":[{"id":"11","name":"Done","to":{"name":"Done"}}]}`)
			return
		}
		w.WriteHeader(http.StatusInternalServerError)
		fmt.Fprintln(w, `{"errorMessages":["fail"]}`)
	}))
	defer ts.Close()

	var stdout, stderr bytes.Buffer
	c := newTestClient(ts.URL, &stdout, &stderr)
	cmd := newTransitionCmd(c)
	_ = cmd.Flags().Set("issue", "TEST-1")
	_ = cmd.Flags().Set("to", "Done")

	err := cmd.RunE(cmd, nil)
	if err == nil {
		t.Fatal("expected error when POST transitions returns 500")
	}
}

// ---------------------------------------------------------------------------
// resolveAssignee "me" — malformed JSON from GET /myself
// ---------------------------------------------------------------------------

func TestResolveAssignee_MalformedMyselfJSON(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Method == "GET" && strings.Contains(r.URL.Path, "/myself") {
			w.WriteHeader(http.StatusOK)
			fmt.Fprint(w, "not json")
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer ts.Close()

	var stdout, stderr bytes.Buffer
	c := newTestClient(ts.URL, &stdout, &stderr)

	cmd := newAssignCmd(c)
	_ = cmd.Flags().Set("issue", "TEST-1")
	_ = cmd.Flags().Set("to", "me")

	err := cmd.RunE(cmd, nil)
	if err == nil {
		t.Fatal("expected error when /myself response is malformed JSON")
	}
	aw, ok := err.(*jrerrors.AlreadyWrittenError)
	if !ok {
		t.Fatalf("expected AlreadyWrittenError, got %T", err)
	}
	if aw.Code == jrerrors.ExitOK {
		t.Errorf("expected non-zero exit code, got ExitOK")
	}
}

func TestRunTransition_WriteOutputFails(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Method == "GET" && strings.Contains(r.URL.Path, "/transitions") {
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
	c.JQFilter = ".[[[invalid"

	cmd := newTransitionCmd(c)
	_ = cmd.Flags().Set("issue", "TEST-1")
	_ = cmd.Flags().Set("to", "Done")

	err := cmd.RunE(cmd, nil)
	if err == nil {
		t.Fatal("expected error from WriteOutput failure")
	}
}

func TestRunMove_WriteOutputFails(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Method == "GET" && strings.Contains(r.URL.Path, "/transitions") {
			fmt.Fprintln(w, `{"transitions":[{"id":"21","name":"In Progress","to":{"name":"In Progress"}}]}`)
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
	c.JQFilter = ".[[[invalid"

	cmd := newMoveCmd(c)
	_ = cmd.Flags().Set("issue", "TEST-1")
	_ = cmd.Flags().Set("to", "In Progress")
	// No --assign flag so the assign branch is skipped.

	err := cmd.RunE(cmd, nil)
	if err == nil {
		t.Fatal("expected error from WriteOutput failure")
	}
}

func TestRunAssign_WriteOutputFails(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Method == "GET" && strings.Contains(r.URL.Path, "/myself") {
			fmt.Fprintln(w, `{"accountId":"abc123"}`)
			return
		}
		if r.Method == "PUT" && strings.Contains(r.URL.Path, "/assignee") {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer ts.Close()

	var stdout, stderr bytes.Buffer
	c := newTestClient(ts.URL, &stdout, &stderr)
	c.JQFilter = ".[[[invalid"

	cmd := newAssignCmd(c)
	_ = cmd.Flags().Set("issue", "TEST-1")
	_ = cmd.Flags().Set("to", "me")

	err := cmd.RunE(cmd, nil)
	if err == nil {
		t.Fatal("expected error from WriteOutput failure")
	}
}

func TestRunComment_WriteOutputFails(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Method == "POST" && strings.Contains(r.URL.Path, "/comment") {
			fmt.Fprintln(w, `{"id":"10001"}`)
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer ts.Close()

	var stdout, stderr bytes.Buffer
	c := newTestClient(ts.URL, &stdout, &stderr)
	c.JQFilter = ".[[[invalid"

	cmd := newCommentCmd(c)
	_ = cmd.Flags().Set("issue", "TEST-1")
	_ = cmd.Flags().Set("text", "hello")

	err := cmd.RunE(cmd, nil)
	if err == nil {
		t.Fatal("expected error from WriteOutput failure")
	}
}

func TestRunLink_WriteOutputFails(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Method == "GET" && strings.Contains(r.URL.Path, "/issueLinkType") {
			fmt.Fprintln(w, `{"issueLinkTypes":[{"id":"10000","name":"Blocks","inward":"is blocked by","outward":"blocks"}]}`)
			return
		}
		if r.Method == "POST" && strings.Contains(r.URL.Path, "/issueLink") {
			w.WriteHeader(http.StatusCreated)
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer ts.Close()

	var stdout, stderr bytes.Buffer
	c := newTestClient(ts.URL, &stdout, &stderr)
	c.JQFilter = ".[[[invalid"

	cmd := newLinkCmd(c)
	_ = cmd.Flags().Set("from", "TEST-1")
	_ = cmd.Flags().Set("to", "TEST-2")
	_ = cmd.Flags().Set("type", "blocks")

	err := cmd.RunE(cmd, nil)
	if err == nil {
		t.Fatal("expected error from WriteOutput failure")
	}
}

func TestRunLogWork_WriteOutputFails(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Method == "POST" && strings.Contains(r.URL.Path, "/worklog") {
			fmt.Fprintln(w, `{"id":"10001","timeSpentSeconds":3600}`)
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer ts.Close()

	var stdout, stderr bytes.Buffer
	c := newTestClient(ts.URL, &stdout, &stderr)
	c.JQFilter = ".[[[invalid"

	cmd := newLogWorkCmd(c)
	_ = cmd.Flags().Set("issue", "TEST-1")
	_ = cmd.Flags().Set("time", "1h")

	err := cmd.RunE(cmd, nil)
	if err == nil {
		t.Fatal("expected error from WriteOutput failure")
	}
}

func TestRunSprint_WriteOutputFails(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Method == "GET" && strings.Contains(r.URL.Path, "/rest/agile/1.0/board") && !strings.Contains(r.URL.Path, "/sprint") {
			fmt.Fprintln(w, `{"values":[{"id":1}]}`)
			return
		}
		if r.Method == "GET" && strings.Contains(r.URL.Path, "/sprint") {
			fmt.Fprintln(w, `{"values":[{"id":42,"name":"Sprint 5","state":"active"}]}`)
			return
		}
		if r.Method == "POST" && strings.Contains(r.URL.Path, "/sprint/") {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer ts.Close()

	var stdout, stderr bytes.Buffer
	c := newTestClient(ts.URL, &stdout, &stderr)
	c.JQFilter = ".[[[invalid"

	cmd := newSprintCmd(c)
	_ = cmd.Flags().Set("issue", "TEST-1")
	_ = cmd.Flags().Set("to", "Sprint 5")

	err := cmd.RunE(cmd, nil)
	if err == nil {
		t.Fatal("expected error from WriteOutput failure")
	}
}

// ---------------------------------------------------------------------------
// runMove — remaining branch coverage
// ---------------------------------------------------------------------------

// TestRunMove_DryRunWriteOutputFails covers the DryRun branch in runMove where
// WriteOutput fails (workflow.go line 336-337).
func TestRunMove_DryRunWriteOutputFails(t *testing.T) {
	var stdout, stderr bytes.Buffer
	c := newTestClient("http://unused", &stdout, &stderr)
	c.DryRun = true
	c.JQFilter = ".[[[invalid"

	cmd := newMoveCmd(c)
	_ = cmd.Flags().Set("issue", "PROJ-1")
	_ = cmd.Flags().Set("to", "Done")
	_ = cmd.Flags().Set("assign", "me")

	err := cmd.RunE(cmd, nil)
	if err == nil {
		t.Fatal("expected error when DryRun WriteOutput fails")
	}
	aw, ok := err.(*jrerrors.AlreadyWrittenError)
	if !ok {
		t.Fatalf("expected AlreadyWrittenError, got %T: %v", err, err)
	}
	if aw.Code == jrerrors.ExitOK {
		t.Errorf("expected non-zero exit code, got %d", aw.Code)
	}
}

// TestRunMove_PostTransitionFails covers the POST /transitions failure path
// in runMove after a successful transition resolution (workflow.go line 351-352).
func TestRunMove_PostTransitionFails(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Method == "GET" && strings.Contains(r.URL.Path, "/transitions") {
			fmt.Fprintln(w, `{"transitions":[{"id":"11","name":"Done","to":{"name":"Done"}}]}`)
			return
		}
		if r.Method == "POST" && strings.Contains(r.URL.Path, "/transitions") {
			w.WriteHeader(http.StatusForbidden)
			fmt.Fprintln(w, `{"message":"forbidden"}`)
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer ts.Close()

	var stdout, stderr bytes.Buffer
	c := newTestClient(ts.URL, &stdout, &stderr)

	cmd := newMoveCmd(c)
	_ = cmd.Flags().Set("issue", "PROJ-1")
	_ = cmd.Flags().Set("to", "Done")

	err := cmd.RunE(cmd, nil)
	if err == nil {
		t.Fatal("expected error when POST transitions returns 403")
	}
	aw, ok := err.(*jrerrors.AlreadyWrittenError)
	if !ok {
		t.Fatalf("expected AlreadyWrittenError, got %T: %v", err, err)
	}
	if aw.Code == jrerrors.ExitOK {
		t.Errorf("expected non-zero exit code, got %d", aw.Code)
	}
}

// TestRunMove_AssignResolveError covers the resolveAssignee failure path in
// runMove after a successful transition (workflow.go line 363-364).
func TestRunMove_AssignResolveError(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Method == "GET" && strings.Contains(r.URL.Path, "/transitions") {
			fmt.Fprintln(w, `{"transitions":[{"id":"11","name":"Done","to":{"name":"Done"}}]}`)
			return
		}
		if r.Method == "POST" && strings.Contains(r.URL.Path, "/transitions") {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		if r.URL.Path == "/rest/api/3/user/search" {
			// Empty results — user not found.
			fmt.Fprintln(w, `[]`)
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer ts.Close()

	var stdout, stderr bytes.Buffer
	c := newTestClient(ts.URL, &stdout, &stderr)

	cmd := newMoveCmd(c)
	_ = cmd.Flags().Set("issue", "PROJ-1")
	_ = cmd.Flags().Set("to", "Done")
	_ = cmd.Flags().Set("assign", "unknown@user.com")

	err := cmd.RunE(cmd, nil)
	if err == nil {
		t.Fatal("expected error when resolveAssignee returns not found")
	}
	aw, ok := err.(*jrerrors.AlreadyWrittenError)
	if !ok {
		t.Fatalf("expected AlreadyWrittenError, got %T: %v", err, err)
	}
	if aw.Code == jrerrors.ExitOK {
		t.Errorf("expected non-zero exit code, got %d", aw.Code)
	}
}

// TestRunMove_UnassignPUTFails covers the unassign PUT failure path in runMove
// (workflow.go line 372-373).
func TestRunMove_UnassignPUTFails(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Method == "GET" && strings.Contains(r.URL.Path, "/transitions") {
			fmt.Fprintln(w, `{"transitions":[{"id":"11","name":"Done","to":{"name":"Done"}}]}`)
			return
		}
		if r.Method == "POST" && strings.Contains(r.URL.Path, "/transitions") {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		if r.Method == "PUT" && strings.Contains(r.URL.Path, "/assignee") {
			w.WriteHeader(http.StatusInternalServerError)
			fmt.Fprintln(w, `{"message":"server error"}`)
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer ts.Close()

	var stdout, stderr bytes.Buffer
	c := newTestClient(ts.URL, &stdout, &stderr)

	cmd := newMoveCmd(c)
	_ = cmd.Flags().Set("issue", "PROJ-1")
	_ = cmd.Flags().Set("to", "Done")
	_ = cmd.Flags().Set("assign", "none")

	err := cmd.RunE(cmd, nil)
	if err == nil {
		t.Fatal("expected error when unassign PUT returns 500")
	}
	aw, ok := err.(*jrerrors.AlreadyWrittenError)
	if !ok {
		t.Fatalf("expected AlreadyWrittenError, got %T: %v", err, err)
	}
	if aw.Code == jrerrors.ExitOK {
		t.Errorf("expected non-zero exit code, got %d", aw.Code)
	}
}

// ---------------------------------------------------------------------------
// runAssign — remaining branch coverage
// ---------------------------------------------------------------------------

// TestRunAssign_DryRunWriteOutputFails covers the DryRun branch in runAssign
// where WriteOutput fails (workflow.go line 413-414).
func TestRunAssign_DryRunWriteOutputFails(t *testing.T) {
	var stdout, stderr bytes.Buffer
	c := newTestClient("http://unused", &stdout, &stderr)
	c.DryRun = true
	c.JQFilter = ".[[[invalid"

	cmd := newAssignCmd(c)
	_ = cmd.Flags().Set("issue", "PROJ-1")
	_ = cmd.Flags().Set("to", "me")

	err := cmd.RunE(cmd, nil)
	if err == nil {
		t.Fatal("expected error when DryRun WriteOutput fails")
	}
	aw, ok := err.(*jrerrors.AlreadyWrittenError)
	if !ok {
		t.Fatalf("expected AlreadyWrittenError, got %T: %v", err, err)
	}
	if aw.Code == jrerrors.ExitOK {
		t.Errorf("expected non-zero exit code, got %d", aw.Code)
	}
}

// TestRunAssign_UnassignPUTFails covers the unassign PUT failure path in
// runAssign (workflow.go line 436-437).
func TestRunAssign_UnassignPUTFails(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Method == "PUT" && strings.Contains(r.URL.Path, "/assignee") {
			w.WriteHeader(http.StatusForbidden)
			fmt.Fprintln(w, `{"message":"forbidden"}`)
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer ts.Close()

	var stdout, stderr bytes.Buffer
	c := newTestClient(ts.URL, &stdout, &stderr)

	cmd := newAssignCmd(c)
	_ = cmd.Flags().Set("issue", "PROJ-1")
	_ = cmd.Flags().Set("to", "none")

	err := cmd.RunE(cmd, nil)
	if err == nil {
		t.Fatal("expected error when unassign PUT returns 403")
	}
	aw, ok := err.(*jrerrors.AlreadyWrittenError)
	if !ok {
		t.Fatalf("expected AlreadyWrittenError, got %T: %v", err, err)
	}
	if aw.Code == jrerrors.ExitOK {
		t.Errorf("expected non-zero exit code, got %d", aw.Code)
	}
}

// TestRunCreateIssue_WriteOutputFails covers the WriteOutput failure path in runCreateIssue.
func TestRunCreateIssue_WriteOutputFails(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Method == "POST" && strings.Contains(r.URL.Path, "/rest/api/3/issue") {
			fmt.Fprintln(w, `{"id":"10001","key":"TEST-1","self":"http://example.com/issue/10001"}`)
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer ts.Close()

	var stdout, stderr bytes.Buffer
	c := newTestClient(ts.URL, &stdout, &stderr)
	c.JQFilter = ".[[[invalid"

	cmd := newCreateIssueCmd(c)
	_ = cmd.Flags().Set("project", "TEST")
	_ = cmd.Flags().Set("type", "Bug")
	_ = cmd.Flags().Set("summary", "Login broken")

	err := cmd.RunE(cmd, nil)
	if err == nil {
		t.Fatal("expected error from WriteOutput failure")
	}
}

// ---------------------------------------------------------------------------
// runTransition — DryRun WriteOutput fail
// ---------------------------------------------------------------------------

// TestRunTransition_DryRunWriteOutputFails covers the DryRun WriteOutput failure
// in runTransition (workflow.go line 282-283).
func TestRunTransition_DryRunWriteOutputFails(t *testing.T) {
	var stdout, stderr bytes.Buffer
	c := newTestClient("http://unused", &stdout, &stderr)
	c.DryRun = true
	c.JQFilter = ".[[[invalid"

	cmd := newTransitionCmd(c)
	_ = cmd.Flags().Set("issue", "PROJ-1")
	_ = cmd.Flags().Set("to", "Done")

	err := cmd.RunE(cmd, nil)
	if err == nil {
		t.Fatal("expected error when DryRun WriteOutput fails")
	}
	aw, ok := err.(*jrerrors.AlreadyWrittenError)
	if !ok {
		t.Fatalf("expected AlreadyWrittenError, got %T: %v", err, err)
	}
	if aw.Code == jrerrors.ExitOK {
		t.Errorf("expected non-zero exit code, got %d", aw.Code)
	}
}

// ---------------------------------------------------------------------------
// runCreateIssue — DryRun WriteOutput fail
// ---------------------------------------------------------------------------

// TestRunCreateIssue_DryRunWriteOutputFails covers the DryRun WriteOutput failure
// in runCreateIssue (workflow.go line 504-505).
func TestRunCreateIssue_DryRunWriteOutputFails(t *testing.T) {
	var stdout, stderr bytes.Buffer
	c := newTestClient("http://unused", &stdout, &stderr)
	c.DryRun = true
	c.JQFilter = ".[[[invalid"

	cmd := newCreateIssueCmd(c)
	_ = cmd.Flags().Set("project", "PROJ")
	_ = cmd.Flags().Set("type", "Bug")
	_ = cmd.Flags().Set("summary", "Test issue")

	err := cmd.RunE(cmd, nil)
	if err == nil {
		t.Fatal("expected error when DryRun WriteOutput fails")
	}
	aw, ok := err.(*jrerrors.AlreadyWrittenError)
	if !ok {
		t.Fatalf("expected AlreadyWrittenError, got %T: %v", err, err)
	}
	if aw.Code == jrerrors.ExitOK {
		t.Errorf("expected non-zero exit code, got %d", aw.Code)
	}
}

// ---------------------------------------------------------------------------
// runLink — DryRun WriteOutput fail
// ---------------------------------------------------------------------------

// TestRunLink_DryRunWriteOutputFails covers the DryRun WriteOutput failure
// in runLink (workflow.go line 608-609).
func TestRunLink_DryRunWriteOutputFails(t *testing.T) {
	var stdout, stderr bytes.Buffer
	c := newTestClient("http://unused", &stdout, &stderr)
	c.DryRun = true
	c.JQFilter = ".[[[invalid"

	cmd := newLinkCmd(c)
	_ = cmd.Flags().Set("from", "PROJ-1")
	_ = cmd.Flags().Set("to", "PROJ-2")
	_ = cmd.Flags().Set("type", "blocks")

	err := cmd.RunE(cmd, nil)
	if err == nil {
		t.Fatal("expected error when DryRun WriteOutput fails")
	}
	aw, ok := err.(*jrerrors.AlreadyWrittenError)
	if !ok {
		t.Fatalf("expected AlreadyWrittenError, got %T: %v", err, err)
	}
	if aw.Code == jrerrors.ExitOK {
		t.Errorf("expected non-zero exit code, got %d", aw.Code)
	}
}

// ---------------------------------------------------------------------------
// runLogWork — DryRun WriteOutput fail
// ---------------------------------------------------------------------------

// TestRunLogWork_DryRunWriteOutputFails covers the DryRun WriteOutput failure
// in runLogWork (workflow.go line 670-671).
func TestRunLogWork_DryRunWriteOutputFails(t *testing.T) {
	var stdout, stderr bytes.Buffer
	c := newTestClient("http://unused", &stdout, &stderr)
	c.DryRun = true
	c.JQFilter = ".[[[invalid"

	cmd := newLogWorkCmd(c)
	_ = cmd.Flags().Set("issue", "PROJ-1")
	_ = cmd.Flags().Set("time", "1h")

	err := cmd.RunE(cmd, nil)
	if err == nil {
		t.Fatal("expected error when DryRun WriteOutput fails")
	}
	aw, ok := err.(*jrerrors.AlreadyWrittenError)
	if !ok {
		t.Fatalf("expected AlreadyWrittenError, got %T: %v", err, err)
	}
	if aw.Code == jrerrors.ExitOK {
		t.Errorf("expected non-zero exit code, got %d", aw.Code)
	}
}

// ---------------------------------------------------------------------------
// resolveSprint — substring match path
// ---------------------------------------------------------------------------

// TestResolveSprint_SubstringMatch covers the substring match return path
// in resolveSprint (workflow.go line 772-773) where the sprint name does not
// exactly match but contains the search string.
func TestResolveSprint_SubstringMatch(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Method == "GET" && r.URL.Path == "/rest/agile/1.0/board" {
			fmt.Fprintln(w, `{"values":[{"id":1,"name":"My Board"}]}`)
			return
		}
		if r.Method == "GET" && strings.Contains(r.URL.Path, "/sprint") {
			// Sprint name is "Team Sprint 7" — searching for "sprint 7" is a substring match.
			fmt.Fprintln(w, `{"values":[{"id":77,"name":"Team Sprint 7","state":"active"}]}`)
			return
		}
		if r.Method == "POST" && strings.Contains(r.URL.Path, "/sprint/77/issue") {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer ts.Close()

	var stdout, stderr bytes.Buffer
	c := newTestClient(ts.URL, &stdout, &stderr)

	cmd := newSprintCmd(c)
	_ = cmd.Flags().Set("issue", "PROJ-1")
	_ = cmd.Flags().Set("to", "sprint 7") // substring of "Team Sprint 7"

	err := cmd.RunE(cmd, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	output := stdout.String()
	if !strings.Contains(output, "sprint_set") {
		t.Errorf("expected 'sprint_set' in output, got: %s", output)
	}
	if !strings.Contains(output, "Team Sprint 7") {
		t.Errorf("expected resolved sprint name in output, got: %s", output)
	}
}

// ---------------------------------------------------------------------------
// runSprint — DryRun WriteOutput fail
// ---------------------------------------------------------------------------

// TestRunSprint_DryRunWriteOutputFails covers the DryRun WriteOutput failure
// in runSprint (workflow.go line 802-803).
func TestRunSprint_DryRunWriteOutputFails(t *testing.T) {
	var stdout, stderr bytes.Buffer
	c := newTestClient("http://unused", &stdout, &stderr)
	c.DryRun = true
	c.JQFilter = ".[[[invalid"

	cmd := newSprintCmd(c)
	_ = cmd.Flags().Set("issue", "PROJ-1")
	_ = cmd.Flags().Set("to", "Sprint 5")

	err := cmd.RunE(cmd, nil)
	if err == nil {
		t.Fatal("expected error when DryRun WriteOutput fails")
	}
	aw, ok := err.(*jrerrors.AlreadyWrittenError)
	if !ok {
		t.Fatalf("expected AlreadyWrittenError, got %T: %v", err, err)
	}
	if aw.Code == jrerrors.ExitOK {
		t.Errorf("expected non-zero exit code, got %d", aw.Code)
	}
}

// ---------------------------------------------------------------------------
// runComment — DryRun WriteOutput fail
// ---------------------------------------------------------------------------

// TestRunAssign_UnassignWriteOutputFails covers the WriteOutput failure after a
// successful unassign PUT in runAssign (workflow.go line 436-437).
func TestRunAssign_UnassignWriteOutputFails(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Method == "PUT" && strings.Contains(r.URL.Path, "/assignee") {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer ts.Close()

	var stdout, stderr bytes.Buffer
	c := newTestClient(ts.URL, &stdout, &stderr)
	c.JQFilter = ".[[[invalid"

	cmd := newAssignCmd(c)
	_ = cmd.Flags().Set("issue", "PROJ-1")
	_ = cmd.Flags().Set("to", "none")

	err := cmd.RunE(cmd, nil)
	if err == nil {
		t.Fatal("expected error when unassign WriteOutput fails")
	}
	aw, ok := err.(*jrerrors.AlreadyWrittenError)
	if !ok {
		t.Fatalf("expected AlreadyWrittenError, got %T: %v", err, err)
	}
	if aw.Code == jrerrors.ExitOK {
		t.Errorf("expected non-zero exit code, got %d", aw.Code)
	}
}

// ---------------------------------------------------------------------------
// resolveSprint — skip board when sprint endpoint fails
// ---------------------------------------------------------------------------

// TestResolveSprint_SkipsFailingBoard covers the path in resolveSprint where a
// board's sprint endpoint returns an error and the board is skipped
// (workflow.go line 739-741).
func TestResolveSprint_SkipsFailingBoard(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Method == "GET" && r.URL.Path == "/rest/agile/1.0/board" {
			// Two boards: board 1 has a broken sprint endpoint, board 2 succeeds.
			fmt.Fprintln(w, `{"values":[{"id":1,"name":"Broken Board"},{"id":2,"name":"Good Board"}]}`)
			return
		}
		if r.Method == "GET" && strings.Contains(r.URL.Path, "/board/1/sprint") {
			w.WriteHeader(http.StatusForbidden)
			fmt.Fprintln(w, `{"message":"no sprint support"}`)
			return
		}
		if r.Method == "GET" && strings.Contains(r.URL.Path, "/board/2/sprint") {
			fmt.Fprintln(w, `{"values":[{"id":99,"name":"Sprint 9","state":"active"}]}`)
			return
		}
		if r.Method == "POST" && strings.Contains(r.URL.Path, "/sprint/99/issue") {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer ts.Close()

	var stdout, stderr bytes.Buffer
	c := newTestClient(ts.URL, &stdout, &stderr)

	cmd := newSprintCmd(c)
	_ = cmd.Flags().Set("issue", "PROJ-1")
	_ = cmd.Flags().Set("to", "Sprint 9")

	err := cmd.RunE(cmd, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v; stderr: %s", err, stderr.String())
	}

	output := stdout.String()
	if !strings.Contains(output, "sprint_set") {
		t.Errorf("expected 'sprint_set' in output, got: %s", output)
	}
}

// ---------------------------------------------------------------------------
// runComment — DryRun WriteOutput fail
// ---------------------------------------------------------------------------

// TestRunComment_DryRunWriteOutputFails covers the DryRun WriteOutput failure
// in runComment (workflow.go line 850-851).
func TestRunComment_DryRunWriteOutputFails(t *testing.T) {
	var stdout, stderr bytes.Buffer
	c := newTestClient("http://unused", &stdout, &stderr)
	c.DryRun = true
	c.JQFilter = ".[[[invalid"

	cmd := newCommentCmd(c)
	_ = cmd.Flags().Set("issue", "PROJ-1")
	_ = cmd.Flags().Set("text", "test comment")

	err := cmd.RunE(cmd, nil)
	if err == nil {
		t.Fatal("expected error when DryRun WriteOutput fails")
	}
	aw, ok := err.(*jrerrors.AlreadyWrittenError)
	if !ok {
		t.Fatalf("expected AlreadyWrittenError, got %T: %v", err, err)
	}
	if aw.Code == jrerrors.ExitOK {
		t.Errorf("expected non-zero exit code, got %d", aw.Code)
	}
}
