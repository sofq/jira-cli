package cmd

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
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
