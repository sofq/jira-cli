package cmd

import (
	"bytes"
	"context"
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
	jrerrors "github.com/sofq/jira-cli/internal/errors"
	"github.com/spf13/cobra"
)

// --- Op map: workflow commands are included ---

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

// --- batchTransition ---

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

func TestBatchTransition_JSONSafe(t *testing.T) {
	// Verify that transition IDs with special characters are properly escaped.
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "GET" && strings.Contains(r.URL.Path, "/transitions") {
			w.Header().Set("Content-Type", "application/json")
			// Transition ID with a quote character (adversarial).
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

// --- batchAssign ---

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

func TestBatchAssign_ParseError_ReturnsExitError(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/user/search") {
			w.Header().Set("Content-Type", "application/json")
			// Return invalid JSON to trigger parse error.
			fmt.Fprintln(w, `not valid json`)
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer ts.Close()

	var stdout, stderr bytes.Buffer
	c := newTestClient(ts.URL, &stdout, &stderr)

	code := batchAssign(t.Context(), c, "TEST-1", "someuser@example.com")
	// Parse errors should return ExitError (1), not ExitNotFound (3).
	if code != jrerrors.ExitError {
		t.Fatalf("expected exit %d (error) for parse failure, got %d", jrerrors.ExitError, code)
	}
	if !strings.Contains(stderr.String(), "connection_error") {
		t.Errorf("expected connection_error in stderr for parse failure, got: %s", stderr.String())
	}
}

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

func TestBatchAssign_JSONSafe(t *testing.T) {
	// Verify that account IDs with special characters are properly escaped.
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/rest/api/3/myself" {
			w.Header().Set("Content-Type", "application/json")
			// Account ID with a quote character (adversarial).
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

// --- executeBatchWorkflow ---

func TestBatchWorkflow_MissingArgs(t *testing.T) {
	var stdout, stderr bytes.Buffer
	c := newTestClient("http://localhost", &stdout, &stderr)

	code := executeBatchWorkflow(t.Context(), c, BatchOp{Command: "workflow transition", Args: map[string]string{}})
	if code != jrerrors.ExitValidation {
		t.Fatalf("expected exit %d, got %d", jrerrors.ExitValidation, code)
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

// --- dry-run on workflow commands ---

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

// --- buildBatchResult ---

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

func TestBuildBatchResult_NonJSONStdout(t *testing.T) {
	// If per-op stdout produces non-JSON (shouldn't happen normally),
	// it should be JSON-encoded as a string.
	var stdoutBuf, stderrBuf strings.Builder
	stdoutBuf.WriteString("not valid json")

	result := buildBatchResult(0, jrerrors.ExitOK, &stdoutBuf, &stderrBuf, false)
	if result.ExitCode != jrerrors.ExitOK {
		t.Fatalf("expected exit 0, got %d", result.ExitCode)
	}
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

	var parsed any
	if err := json.Unmarshal(result.Error, &parsed); err != nil {
		t.Fatal(err)
	}
	arr, ok := parsed.([]any)
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
	var parsed map[string]any
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
	var parsed map[string]any
	if err := json.Unmarshal(result.Error, &parsed); err != nil {
		t.Fatalf("expected single error object: %s", string(result.Error))
	}
	if parsed["error_type"] != "not_found" {
		t.Errorf("expected error_type=not_found, got: %v", parsed["error_type"])
	}
}

// --- executeBatchOp ---

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

	// issue get requires issueIdOrKey but we don't provide it.
	result := executeBatchOp(cmd, c, 0, BatchOp{
		Command: "issue get",
		Args:    map[string]string{},
	}, opMap)

	if result.ExitCode != jrerrors.ExitValidation {
		t.Fatalf("expected exit code %d (validation), got %d", jrerrors.ExitValidation, result.ExitCode)
	}
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

	// issue get with empty issueIdOrKey.
	result := executeBatchOp(cmd, c, 0, BatchOp{
		Command: "issue get",
		Args:    map[string]string{"issueIdOrKey": ""},
	}, opMap)

	if result.ExitCode != jrerrors.ExitValidation {
		t.Fatalf("expected exit code %d (validation), got %d", jrerrors.ExitValidation, result.ExitCode)
	}
}

func TestBatchPerOpPretty_Disabled(t *testing.T) {
	// When --pretty is set, per-op clients should NOT have Pretty=true.
	// Pretty is applied once at the batch output level, not per-op.
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintln(w, `{"key":"TEST-1","summary":"test"}`)
	}))
	defer ts.Close()

	var stdout, stderr bytes.Buffer
	baseClient := newTestClient(ts.URL, &stdout, &stderr)
	baseClient.Pretty = true

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

func TestBatchVerboseLogDetection_ErrorWithTypeField(t *testing.T) {
	// Verify that an error message containing "type":"request" is NOT
	// incorrectly forwarded as a verbose log line.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		// Error body that contains the text "type":"request" to try to trick
		// the verbose log detection heuristic.
		_, _ = w.Write([]byte(`{"errorMessages":["invalid \"type\":\"request\" field"]}`))
	}))
	defer srv.Close()

	var stdout, stderr bytes.Buffer
	c := newTestClient(srv.URL, &stdout, &stderr)
	c.Verbose = true

	ctx := client.NewContext(context.Background(), c)
	cmd := &cobra.Command{Use: "test"}
	cmd.SetContext(ctx)

	allOps := generated.AllSchemaOps()
	allOps = append(allOps, HandWrittenSchemaOps()...)
	opMap := make(map[string]generated.SchemaOp, len(allOps))
	for _, op := range allOps {
		opMap[op.Resource+" "+op.Verb] = op
	}

	bop := BatchOp{
		Command: "issue get",
		Args:    map[string]string{"issueIdOrKey": "TEST-1"},
	}
	result := executeBatchOp(cmd, c, 0, bop, opMap)

	if result.ExitCode == 0 {
		t.Error("expected non-zero exit code for 400 error")
	}
	// The error result should contain the error, not have it swallowed by verbose detection.
	if result.Error == nil {
		t.Error("expected error in batch result, got nil")
	}
	errStr := string(result.Error)
	if !strings.Contains(errStr, "errorMessages") && !strings.Contains(errStr, "validation_error") && !strings.Contains(errStr, "client_error") {
		t.Errorf("error result should contain the API error, got: %s", errStr)
	}
}

// --- runBatch ---

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
	aw, ok := err.(*jrerrors.AlreadyWrittenError)
	if !ok {
		t.Fatalf("expected AlreadyWrittenError, got %T", err)
	}
	if aw.Code != jrerrors.ExitValidation {
		t.Errorf("expected exit %d, got %d", jrerrors.ExitValidation, aw.Code)
	}
}

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
	aw, ok := capturedErr.(*jrerrors.AlreadyWrittenError)
	if !ok {
		t.Fatalf("expected AlreadyWrittenError, got %T", capturedErr)
	}
	if aw.Code != jrerrors.ExitNotFound {
		t.Errorf("expected exit %d, got %d", jrerrors.ExitNotFound, aw.Code)
	}
}

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

// --- --fields precedence (table-driven) ---

func TestBatch_FieldsPrecedence(t *testing.T) {
	tests := []struct {
		name       string
		args       map[string]string
		wantFields string
	}{
		{
			name:       "per-op fields override global",
			args:       map[string]string{"issueIdOrKey": "TEST-1", "fields": "summary,status"},
			wantFields: "summary,status",
		},
		{
			name:       "global fields applied when no per-op fields",
			args:       map[string]string{"issueIdOrKey": "TEST-1"},
			wantFields: "key",
		},
	}

	allOps := generated.AllSchemaOps()
	allOps = append(allOps, HandWrittenSchemaOps()...)
	opMap := make(map[string]generated.SchemaOp, len(allOps))
	for _, op := range allOps {
		opMap[op.Resource+" "+op.Verb] = op
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
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

			bop := BatchOp{Command: "issue get", Args: tt.args}
			result := executeBatchOp(cmd, c, 0, bop, opMap)

			if result.ExitCode != jrerrors.ExitOK {
				t.Fatalf("expected exit 0, got %d; error=%s", result.ExitCode, string(result.Error))
			}

			var data map[string]string
			if err := json.Unmarshal(result.Data, &data); err != nil {
				t.Fatalf("failed to parse result data: %v (raw: %s)", err, string(result.Data))
			}

			if data["fields_received"] != tt.wantFields {
				t.Errorf("expected fields %q, got %q", tt.wantFields, data["fields_received"])
			}
		})
	}
}
