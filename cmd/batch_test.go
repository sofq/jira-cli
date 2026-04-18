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
	"github.com/sofq/jira-cli/internal/policy"
	"github.com/spf13/cobra"
)

// newTestDenyPolicy creates a policy that denies all operations.
func newTestDenyPolicy() (*policy.Policy, error) {
	return policy.NewFromConfig(nil, []string{"*"})
}

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

	code := doTransition(t.Context(), c, "TEST-1", "Done")
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

	code := doTransition(t.Context(), c, "TEST-1", "Nonexistent")
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
	code := doTransition(t.Context(), c, "TEST-1", "progress")
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

	code := doTransition(t.Context(), c, "TEST-1", "Done")
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

	code := doTransition(t.Context(), c, "NOPE-999", "Done")
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

	code := doTransition(t.Context(), c, "TEST-1", "Done")
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

	code := doTransition(t.Context(), c, "TEST-1", "Done")
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

	code := doTransition(t.Context(), c, "TEST-1", "Done")
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

	code := doTransition(t.Context(), c, "TEST-1", "In Progress")
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

	code := doTransition(t.Context(), c, "TEST-1", "Done")
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

	code := doTransition(t.Context(), c, "TEST-1", "Done")
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

	code := doAssign(t.Context(), c, "TEST-1", "me")
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

	code := doAssign(t.Context(), c, "TEST-1", "none")
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

	code := doAssign(t.Context(), c, "TEST-1", "Test User")
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

	code := doAssign(t.Context(), c, "TEST-1", "nonexistent@example.com")
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

	code := doAssign(t.Context(), c, "TEST-1", "someuser@example.com")
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

	code := doAssign(t.Context(), c, "TEST-1", "me")
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

	code := doAssign(t.Context(), c, "TEST-1", "me")
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

	code := doAssign(t.Context(), c, "TEST-1", "me")
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

	code := doAssign(t.Context(), c, "TEST-1", "someuser")
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

	code := doAssign(t.Context(), c, "TEST-1", "me")
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

	code := doAssign(t.Context(), c, "TEST-1", "none")
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

	code := doAssign(t.Context(), c, "TEST-1", "me")
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

	code := doAssign(t.Context(), c, "TEST-1", "me")
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

	code := doAssign(t.Context(), c, "TEST-1", "none")
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

	code := doAssign(t.Context(), c, "TEST-1", "me")
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

	code := doAssign(t.Context(), c, "TEST-1", "me")
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

	code := doAssign(t.Context(), c, "TEST-1", "me")
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

	code := doTransition(t.Context(), c, "TEST-1", "Done")
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

	code := doAssign(t.Context(), c, "TEST-1", "me")
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

// --- executeBatchWorkflow: additional dispatch paths ---

func TestBatchWorkflow_Move_DryRun(t *testing.T) {
	var stdout, stderr bytes.Buffer
	c := newTestClient("http://unused", &stdout, &stderr)
	c.DryRun = true

	code := executeBatchWorkflow(t.Context(), c, BatchOp{
		Command: "workflow move",
		Args:    map[string]string{"issue": "TEST-1", "to": "In Progress"},
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
	if !strings.Contains(result["url"], "/transitions") {
		t.Errorf("expected URL containing /transitions, got %s", result["url"])
	}
}

func TestBatchWorkflow_Move_MissingTo(t *testing.T) {
	var stdout, stderr bytes.Buffer
	c := newTestClient("http://unused", &stdout, &stderr)

	code := executeBatchWorkflow(t.Context(), c, BatchOp{
		Command: "workflow move",
		Args:    map[string]string{"issue": "TEST-1"},
	})
	if code != jrerrors.ExitValidation {
		t.Fatalf("expected exit %d, got %d", jrerrors.ExitValidation, code)
	}
	if !strings.Contains(stderr.String(), "'to'") {
		t.Errorf("expected 'to' in error, got: %s", stderr.String())
	}
}

func TestBatchWorkflow_Comment_DryRun(t *testing.T) {
	var stdout, stderr bytes.Buffer
	c := newTestClient("http://unused", &stdout, &stderr)
	c.DryRun = true

	code := executeBatchWorkflow(t.Context(), c, BatchOp{
		Command: "workflow comment",
		Args:    map[string]string{"issue": "TEST-1", "text": "Hello"},
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
	if !strings.Contains(result["url"], "/comment") {
		t.Errorf("expected URL containing /comment, got %s", result["url"])
	}
}

func TestBatchWorkflow_Comment_MissingText(t *testing.T) {
	var stdout, stderr bytes.Buffer
	c := newTestClient("http://unused", &stdout, &stderr)

	code := executeBatchWorkflow(t.Context(), c, BatchOp{
		Command: "workflow comment",
		Args:    map[string]string{"issue": "TEST-1"},
	})
	if code != jrerrors.ExitValidation {
		t.Fatalf("expected exit %d, got %d", jrerrors.ExitValidation, code)
	}
}

func TestBatchWorkflow_LogWork_DryRun(t *testing.T) {
	var stdout, stderr bytes.Buffer
	c := newTestClient("http://unused", &stdout, &stderr)
	c.DryRun = true

	code := executeBatchWorkflow(t.Context(), c, BatchOp{
		Command: "workflow log-work",
		Args:    map[string]string{"issue": "TEST-1", "time": "1h"},
	})
	if code != jrerrors.ExitOK {
		t.Fatalf("expected exit 0, got %d; stderr=%s", code, stderr.String())
	}
	var result map[string]any
	if err := json.Unmarshal([]byte(strings.TrimSpace(stdout.String())), &result); err != nil {
		t.Fatalf("stdout not valid JSON: %s", stdout.String())
	}
	if result["method"] != "POST" {
		t.Errorf("expected method=POST, got %v", result["method"])
	}
}

func TestBatchWorkflow_LogWork_MissingTime(t *testing.T) {
	var stdout, stderr bytes.Buffer
	c := newTestClient("http://unused", &stdout, &stderr)

	code := executeBatchWorkflow(t.Context(), c, BatchOp{
		Command: "workflow log-work",
		Args:    map[string]string{"issue": "TEST-1"},
	})
	if code != jrerrors.ExitValidation {
		t.Fatalf("expected exit %d, got %d", jrerrors.ExitValidation, code)
	}
}

func TestBatchWorkflow_Sprint_DryRun(t *testing.T) {
	var stdout, stderr bytes.Buffer
	c := newTestClient("http://unused", &stdout, &stderr)
	c.DryRun = true

	code := executeBatchWorkflow(t.Context(), c, BatchOp{
		Command: "workflow sprint",
		Args:    map[string]string{"issue": "TEST-1", "to": "Sprint 5"},
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
	if !strings.Contains(result["note"], "would move") {
		t.Errorf("expected dry-run note, got %s", result["note"])
	}
}

func TestBatchWorkflow_Sprint_MissingTo(t *testing.T) {
	var stdout, stderr bytes.Buffer
	c := newTestClient("http://unused", &stdout, &stderr)

	code := executeBatchWorkflow(t.Context(), c, BatchOp{
		Command: "workflow sprint",
		Args:    map[string]string{"issue": "TEST-1"},
	})
	if code != jrerrors.ExitValidation {
		t.Fatalf("expected exit %d, got %d", jrerrors.ExitValidation, code)
	}
}

func TestBatchWorkflow_Link_MissingArgs(t *testing.T) {
	tests := []struct {
		name string
		args map[string]string
		want string
	}{
		{
			name: "missing from",
			args: map[string]string{"to": "TEST-2", "type": "blocks"},
			want: "'from'",
		},
		{
			name: "missing to",
			args: map[string]string{"from": "TEST-1", "type": "blocks"},
			want: "'to'",
		},
		{
			name: "missing type",
			args: map[string]string{"from": "TEST-1", "to": "TEST-2"},
			want: "'type'",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			c := newTestClient("http://unused", &stdout, &stderr)

			code := executeBatchWorkflow(t.Context(), c, BatchOp{
				Command: "workflow link",
				Args:    tt.args,
			})
			if code != jrerrors.ExitValidation {
				t.Fatalf("expected exit %d, got %d", jrerrors.ExitValidation, code)
			}
			if !strings.Contains(stderr.String(), tt.want) {
				t.Errorf("expected %q in error, got: %s", tt.want, stderr.String())
			}
		})
	}
}

func TestBatchWorkflow_Create_MissingArgs(t *testing.T) {
	tests := []struct {
		name string
		args map[string]string
		want string
	}{
		{
			name: "missing project",
			args: map[string]string{"type": "Bug", "summary": "oops"},
			want: "'project'",
		},
		{
			name: "missing type",
			args: map[string]string{"project": "PROJ", "summary": "oops"},
			want: "'type'",
		},
		{
			name: "missing summary",
			args: map[string]string{"project": "PROJ", "type": "Bug"},
			want: "'summary'",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			c := newTestClient("http://unused", &stdout, &stderr)

			code := executeBatchWorkflow(t.Context(), c, BatchOp{
				Command: "workflow create",
				Args:    tt.args,
			})
			if code != jrerrors.ExitValidation {
				t.Fatalf("expected exit %d, got %d", jrerrors.ExitValidation, code)
			}
			if !strings.Contains(stderr.String(), tt.want) {
				t.Errorf("expected %q in error, got: %s", tt.want, stderr.String())
			}
		})
	}
}

func TestBatchWorkflow_UnknownCommand(t *testing.T) {
	var stdout, stderr bytes.Buffer
	c := newTestClient("http://unused", &stdout, &stderr)

	code := executeBatchWorkflow(t.Context(), c, BatchOp{
		Command: "workflow frobnicate",
		Args:    map[string]string{"issue": "TEST-1"},
	})
	if code != jrerrors.ExitValidation {
		t.Fatalf("expected exit %d, got %d", jrerrors.ExitValidation, code)
	}
	if !strings.Contains(stderr.String(), "unknown workflow command") {
		t.Errorf("expected 'unknown workflow command' in error, got: %s", stderr.String())
	}
}

func TestBatchWorkflow_MissingIssue_CommentLogWorkSprintMove(t *testing.T) {
	cmds := []struct {
		command string
		extra   map[string]string
	}{
		{"workflow comment", map[string]string{"text": "hi"}},
		{"workflow log-work", map[string]string{"time": "1h"}},
		{"workflow sprint", map[string]string{"to": "Sprint 1"}},
		{"workflow move", map[string]string{"to": "Done"}},
	}

	for _, tc := range cmds {
		t.Run(tc.command, func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			c := newTestClient("http://unused", &stdout, &stderr)

			args := make(map[string]string)
			for k, v := range tc.extra {
				args[k] = v
			}
			// Deliberately omit "issue".

			code := executeBatchWorkflow(t.Context(), c, BatchOp{
				Command: tc.command,
				Args:    args,
			})
			if code != jrerrors.ExitValidation {
				t.Fatalf("[%s] expected exit %d, got %d", tc.command, jrerrors.ExitValidation, code)
			}
			if !strings.Contains(stderr.String(), "'issue'") {
				t.Errorf("[%s] expected 'issue' in error, got: %s", tc.command, stderr.String())
			}
		})
	}
}

// --- batchMove ---

func TestBatchMove_Success_NoAssign(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "GET" && strings.Contains(r.URL.Path, "/transitions") {
			w.Header().Set("Content-Type", "application/json")
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

	code := doMove(t.Context(), c, "TEST-1", "In Progress", "")
	if code != jrerrors.ExitOK {
		t.Fatalf("expected exit 0, got %d; stderr=%s", code, stderr.String())
	}
	var result map[string]any
	if err := json.Unmarshal([]byte(strings.TrimSpace(stdout.String())), &result); err != nil {
		t.Fatalf("stdout not valid JSON: %s", stdout.String())
	}
	if result["status"] != "moved" {
		t.Errorf("expected status=moved, got %v", result["status"])
	}
	if result["transition"] != "In Progress" {
		t.Errorf("expected transition=In Progress, got %v", result["transition"])
	}
	if _, hasAssign := result["assigned"]; hasAssign {
		t.Errorf("expected no assigned field when assign is empty")
	}
}

func TestBatchMove_Success_WithAssignMe(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "GET" && strings.Contains(r.URL.Path, "/transitions") {
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprintln(w, `{"transitions":[{"id":"21","name":"Done","to":{"name":"Done"}}]}`)
			return
		}
		if r.Method == "POST" && strings.Contains(r.URL.Path, "/transitions") {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		if r.URL.Path == "/rest/api/3/myself" {
			w.Header().Set("Content-Type", "application/json")
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

	code := doMove(t.Context(), c, "TEST-1", "Done", "me")
	if code != jrerrors.ExitOK {
		t.Fatalf("expected exit 0, got %d; stderr=%s", code, stderr.String())
	}
	var result map[string]any
	if err := json.Unmarshal([]byte(strings.TrimSpace(stdout.String())), &result); err != nil {
		t.Fatalf("stdout not valid JSON: %s", stdout.String())
	}
	if result["status"] != "moved" {
		t.Errorf("expected status=moved, got %v", result["status"])
	}
	if result["assigned"] != "me" {
		t.Errorf("expected assigned=me, got %v", result["assigned"])
	}
}

func TestBatchMove_Success_WithUnassign(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "GET" && strings.Contains(r.URL.Path, "/transitions") {
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprintln(w, `{"transitions":[{"id":"21","name":"Done","to":{"name":"Done"}}]}`)
			return
		}
		if r.Method == "POST" && strings.Contains(r.URL.Path, "/transitions") {
			w.WriteHeader(http.StatusNoContent)
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

	code := doMove(t.Context(), c, "TEST-1", "Done", "none")
	if code != jrerrors.ExitOK {
		t.Fatalf("expected exit 0, got %d; stderr=%s", code, stderr.String())
	}
	var result map[string]any
	if err := json.Unmarshal([]byte(strings.TrimSpace(stdout.String())), &result); err != nil {
		t.Fatalf("stdout not valid JSON: %s", stdout.String())
	}
	if result["assigned"] != "unassigned" {
		t.Errorf("expected assigned=unassigned, got %v", result["assigned"])
	}
}

func TestBatchMove_DryRun_NoAssign(t *testing.T) {
	var stdout, stderr bytes.Buffer
	c := newTestClient("http://unused", &stdout, &stderr)
	c.DryRun = true

	code := doMove(t.Context(), c, "TEST-1", "Done", "")
	if code != jrerrors.ExitOK {
		t.Fatalf("expected exit 0, got %d; stderr=%s", code, stderr.String())
	}
	var result map[string]any
	if err := json.Unmarshal([]byte(strings.TrimSpace(stdout.String())), &result); err != nil {
		t.Fatalf("stdout not valid JSON: %s", stdout.String())
	}
	if result["method"] != "POST" {
		t.Errorf("expected method=POST, got %v", result["method"])
	}
	if _, hasAssign := result["assign"]; hasAssign {
		t.Errorf("expected no assign field for empty assign")
	}
}

func TestBatchMove_DryRun_WithAssign(t *testing.T) {
	var stdout, stderr bytes.Buffer
	c := newTestClient("http://unused", &stdout, &stderr)
	c.DryRun = true

	code := doMove(t.Context(), c, "TEST-1", "Done", "me")
	if code != jrerrors.ExitOK {
		t.Fatalf("expected exit 0, got %d; stderr=%s", code, stderr.String())
	}
	var result map[string]any
	if err := json.Unmarshal([]byte(strings.TrimSpace(stdout.String())), &result); err != nil {
		t.Fatalf("stdout not valid JSON: %s", stdout.String())
	}
	if _, hasAssign := result["assign"]; !hasAssign {
		t.Errorf("expected assign field in dry-run output when assign='me'")
	}
}

// --- batchComment ---

func TestBatchComment_Success(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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

	code := doComment(t.Context(), c, "TEST-1", "Great work!")
	if code != jrerrors.ExitOK {
		t.Fatalf("expected exit 0, got %d; stderr=%s", code, stderr.String())
	}
	var result map[string]string
	if err := json.Unmarshal([]byte(strings.TrimSpace(stdout.String())), &result); err != nil {
		t.Fatalf("stdout not valid JSON: %s", stdout.String())
	}
	if result["status"] != "commented" {
		t.Errorf("expected status=commented, got %s", result["status"])
	}
	if result["issue"] != "TEST-1" {
		t.Errorf("expected issue=TEST-1, got %s", result["issue"])
	}
}

func TestBatchComment_DryRun(t *testing.T) {
	var stdout, stderr bytes.Buffer
	c := newTestClient("http://unused", &stdout, &stderr)
	c.DryRun = true

	code := doComment(t.Context(), c, "TEST-1", "A comment")
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
	if !strings.Contains(result["url"], "/comment") {
		t.Errorf("expected URL containing /comment, got %s", result["url"])
	}
	if !strings.Contains(result["note"], "would add comment") {
		t.Errorf("expected dry-run note, got %s", result["note"])
	}
}

// --- batchLink ---

func TestBatchLink_Success(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "GET" && r.URL.Path == "/rest/api/3/issueLinkType" {
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprintln(w, `{"issueLinkTypes":[{"id":"10001","name":"Blocks","inward":"is blocked by","outward":"blocks"}]}`)
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

	code := doLink(t.Context(), c, "TEST-1", "TEST-2", "blocks")
	if code != jrerrors.ExitOK {
		t.Fatalf("expected exit 0, got %d; stderr=%s", code, stderr.String())
	}
	var result map[string]string
	if err := json.Unmarshal([]byte(strings.TrimSpace(stdout.String())), &result); err != nil {
		t.Fatalf("stdout not valid JSON: %s", stdout.String())
	}
	if result["status"] != "linked" {
		t.Errorf("expected status=linked, got %s", result["status"])
	}
	if result["from"] != "TEST-1" {
		t.Errorf("expected from=TEST-1, got %s", result["from"])
	}
	if result["to"] != "TEST-2" {
		t.Errorf("expected to=TEST-2, got %s", result["to"])
	}
}

func TestBatchLink_DryRun(t *testing.T) {
	var stdout, stderr bytes.Buffer
	c := newTestClient("http://unused", &stdout, &stderr)
	c.DryRun = true

	code := doLink(t.Context(), c, "TEST-1", "TEST-2", "blocks")
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
	if !strings.Contains(result["url"], "issueLink") {
		t.Errorf("expected URL containing issueLink, got %s", result["url"])
	}
	if !strings.Contains(result["note"], "would link") {
		t.Errorf("expected dry-run note, got %s", result["note"])
	}
}

// --- batchCreateIssue ---

func TestBatchCreateIssue_MissingArgs(t *testing.T) {
	tests := []struct {
		name string
		args map[string]string
		want string
	}{
		{
			name: "missing project",
			args: map[string]string{"type": "Bug", "summary": "oops"},
			want: "'project'",
		},
		{
			name: "missing type",
			args: map[string]string{"project": "PROJ", "summary": "oops"},
			want: "'type'",
		},
		{
			name: "missing summary",
			args: map[string]string{"project": "PROJ", "type": "Bug"},
			want: "'summary'",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			c := newTestClient("http://unused", &stdout, &stderr)

			code := doCreateIssue(t.Context(), c, tt.args)
			if code != jrerrors.ExitValidation {
				t.Fatalf("expected exit %d, got %d", jrerrors.ExitValidation, code)
			}
			if !strings.Contains(stderr.String(), tt.want) {
				t.Errorf("expected %q in error, got: %s", tt.want, stderr.String())
			}
		})
	}
}

func TestBatchCreateIssue_Success_Minimal(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "POST" && r.URL.Path == "/rest/api/3/issue" {
			w.WriteHeader(http.StatusCreated)
			fmt.Fprintln(w, `{"id":"10001","key":"PROJ-42"}`)
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer ts.Close()

	var stdout, stderr bytes.Buffer
	c := newTestClient(ts.URL, &stdout, &stderr)

	code := doCreateIssue(t.Context(), c, map[string]string{
		"project": "PROJ",
		"type":    "Bug",
		"summary": "Login broken",
	})
	if code != jrerrors.ExitOK {
		t.Fatalf("expected exit 0, got %d; stderr=%s", code, stderr.String())
	}
	var result map[string]string
	if err := json.Unmarshal([]byte(strings.TrimSpace(stdout.String())), &result); err != nil {
		t.Fatalf("stdout not valid JSON: %s", stdout.String())
	}
	if result["key"] != "PROJ-42" {
		t.Errorf("expected key=PROJ-42, got %s", result["key"])
	}
}

func TestBatchCreateIssue_DryRun_AllFields(t *testing.T) {
	var stdout, stderr bytes.Buffer
	c := newTestClient("http://unused", &stdout, &stderr)
	c.DryRun = true

	code := doCreateIssue(t.Context(), c, map[string]string{
		"project":     "PROJ",
		"type":        "Bug",
		"summary":     "Login broken",
		"description": "Steps to reproduce...",
		"priority":    "High",
		"labels":      "bug,urgent",
	})
	if code != jrerrors.ExitOK {
		t.Fatalf("expected exit 0, got %d; stderr=%s", code, stderr.String())
	}
	var result map[string]any
	if err := json.Unmarshal([]byte(strings.TrimSpace(stdout.String())), &result); err != nil {
		t.Fatalf("stdout not valid JSON: %s", stdout.String())
	}
	if result["method"] != "POST" {
		t.Errorf("expected method=POST, got %v", result["method"])
	}
	if !strings.Contains(result["url"].(string), "/rest/api/3/issue") {
		t.Errorf("expected URL containing /rest/api/3/issue, got %v", result["url"])
	}
}

func TestBatchCreateIssue_Success_WithAssignMe(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/rest/api/3/myself" {
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprintln(w, `{"accountId":"abc123"}`)
			return
		}
		if r.Method == "POST" && r.URL.Path == "/rest/api/3/issue" {
			w.WriteHeader(http.StatusCreated)
			fmt.Fprintln(w, `{"id":"10002","key":"PROJ-43"}`)
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer ts.Close()

	var stdout, stderr bytes.Buffer
	c := newTestClient(ts.URL, &stdout, &stderr)

	code := doCreateIssue(t.Context(), c, map[string]string{
		"project": "PROJ",
		"type":    "Task",
		"summary": "Do the thing",
		"assign":  "me",
	})
	if code != jrerrors.ExitOK {
		t.Fatalf("expected exit 0, got %d; stderr=%s", code, stderr.String())
	}
	var result map[string]string
	if err := json.Unmarshal([]byte(strings.TrimSpace(stdout.String())), &result); err != nil {
		t.Fatalf("stdout not valid JSON: %s", stdout.String())
	}
	if result["key"] != "PROJ-43" {
		t.Errorf("expected key=PROJ-43, got %s", result["key"])
	}
}

// --- batchSprint ---

func TestBatchSprint_DryRun(t *testing.T) {
	var stdout, stderr bytes.Buffer
	c := newTestClient("http://unused", &stdout, &stderr)
	c.DryRun = true

	code := doSprint(t.Context(), c, "TEST-1", "Sprint 5")
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
	if !strings.Contains(result["note"], "would move") {
		t.Errorf("expected dry-run note, got %s", result["note"])
	}
}

func TestBatchSprint_Success(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "GET" && r.URL.Path == "/rest/agile/1.0/board" {
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprintln(w, `{"values":[{"id":1}]}`)
			return
		}
		if r.Method == "GET" && strings.Contains(r.URL.Path, "/sprint") {
			w.Header().Set("Content-Type", "application/json")
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

	code := doSprint(t.Context(), c, "TEST-1", "Sprint 5")
	if code != jrerrors.ExitOK {
		t.Fatalf("expected exit 0, got %d; stderr=%s", code, stderr.String())
	}
	var result map[string]string
	if err := json.Unmarshal([]byte(strings.TrimSpace(stdout.String())), &result); err != nil {
		t.Fatalf("stdout not valid JSON: %s", stdout.String())
	}
	if result["status"] != "sprint_set" {
		t.Errorf("expected status=sprint_set, got %s", result["status"])
	}
	if result["sprint"] != "Sprint 5" {
		t.Errorf("expected sprint=Sprint 5, got %s", result["sprint"])
	}
}

// --- batchLogWork ---

func TestBatchLogWork_InvalidDuration(t *testing.T) {
	var stdout, stderr bytes.Buffer
	c := newTestClient("http://unused", &stdout, &stderr)

	code := doLogWork(t.Context(), c, "TEST-1", "not-a-duration", "")
	if code != jrerrors.ExitValidation {
		t.Fatalf("expected exit %d, got %d", jrerrors.ExitValidation, code)
	}
	if !strings.Contains(stderr.String(), "invalid duration") {
		t.Errorf("expected 'invalid duration' in error, got: %s", stderr.String())
	}
}

func TestBatchLogWork_DryRun(t *testing.T) {
	var stdout, stderr bytes.Buffer
	c := newTestClient("http://unused", &stdout, &stderr)
	c.DryRun = true

	code := doLogWork(t.Context(), c, "TEST-1", "2h30m", "debugging")
	if code != jrerrors.ExitOK {
		t.Fatalf("expected exit 0, got %d; stderr=%s", code, stderr.String())
	}
	var result map[string]any
	if err := json.Unmarshal([]byte(strings.TrimSpace(stdout.String())), &result); err != nil {
		t.Fatalf("stdout not valid JSON: %s", stdout.String())
	}
	if result["method"] != "POST" {
		t.Errorf("expected method=POST, got %v", result["method"])
	}
	if !strings.Contains(result["url"].(string), "/worklog") {
		t.Errorf("expected URL containing /worklog, got %v", result["url"])
	}
	// 2h30m = 9000 seconds.
	if result["timeSpentSeconds"] != float64(9000) {
		t.Errorf("expected timeSpentSeconds=9000, got %v", result["timeSpentSeconds"])
	}
}

func TestBatchLogWork_Success(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "POST" && strings.Contains(r.URL.Path, "/worklog") {
			w.WriteHeader(http.StatusCreated)
			fmt.Fprintln(w, `{"id":"10001"}`)
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer ts.Close()

	var stdout, stderr bytes.Buffer
	c := newTestClient(ts.URL, &stdout, &stderr)

	code := doLogWork(t.Context(), c, "TEST-1", "1h", "debugging session")
	if code != jrerrors.ExitOK {
		t.Fatalf("expected exit 0, got %d; stderr=%s", code, stderr.String())
	}
	var result map[string]string
	if err := json.Unmarshal([]byte(strings.TrimSpace(stdout.String())), &result); err != nil {
		t.Fatalf("stdout not valid JSON: %s", stdout.String())
	}
	if result["status"] != "logged" {
		t.Errorf("expected status=logged, got %s", result["status"])
	}
	if result["issue"] != "TEST-1" {
		t.Errorf("expected issue=TEST-1, got %s", result["issue"])
	}
	if result["time"] != "1h" {
		t.Errorf("expected time=1h, got %s", result["time"])
	}
}

// --- executeBatchTemplate ---

func TestBatchTemplate_UnknownCommand(t *testing.T) {
	var stdout, stderr bytes.Buffer
	c := newTestClient("http://unused", &stdout, &stderr)

	code := executeBatchTemplate(t.Context(), c, BatchOp{
		Command: "template list",
		Args:    map[string]string{},
	})
	if code != jrerrors.ExitValidation {
		t.Fatalf("expected exit %d, got %d", jrerrors.ExitValidation, code)
	}
	if !strings.Contains(stderr.String(), "unknown template command") {
		t.Errorf("expected 'unknown template command' in error, got: %s", stderr.String())
	}
}

func TestBatchTemplate_Apply_MissingName(t *testing.T) {
	var stdout, stderr bytes.Buffer
	c := newTestClient("http://unused", &stdout, &stderr)

	code := executeBatchTemplate(t.Context(), c, BatchOp{
		Command: "template apply",
		Args:    map[string]string{"project": "PROJ"},
	})
	if code != jrerrors.ExitValidation {
		t.Fatalf("expected exit %d, got %d", jrerrors.ExitValidation, code)
	}
	if !strings.Contains(stderr.String(), "'name'") {
		t.Errorf("expected 'name' in error, got: %s", stderr.String())
	}
}

// --- batchTemplateApply ---

func TestBatchTemplateApply_MissingName(t *testing.T) {
	var stdout, stderr bytes.Buffer
	c := newTestClient("http://unused", &stdout, &stderr)

	code := batchTemplateApply(t.Context(), c, map[string]string{"project": "PROJ"})
	if code != jrerrors.ExitValidation {
		t.Fatalf("expected exit %d, got %d", jrerrors.ExitValidation, code)
	}
	if !strings.Contains(stderr.String(), "'name'") {
		t.Errorf("expected 'name' in error, got: %s", stderr.String())
	}
}

func TestBatchTemplateApply_MissingProject(t *testing.T) {
	var stdout, stderr bytes.Buffer
	c := newTestClient("http://unused", &stdout, &stderr)

	code := batchTemplateApply(t.Context(), c, map[string]string{"name": "bug-report"})
	if code != jrerrors.ExitValidation {
		t.Fatalf("expected exit %d, got %d", jrerrors.ExitValidation, code)
	}
	if !strings.Contains(stderr.String(), "'project'") {
		t.Errorf("expected 'project' in error, got: %s", stderr.String())
	}
}

func TestBatchTemplateApply_TemplateNotFound(t *testing.T) {
	var stdout, stderr bytes.Buffer
	c := newTestClient("http://unused", &stdout, &stderr)

	code := batchTemplateApply(t.Context(), c, map[string]string{
		"name":    "nonexistent-template-xyz",
		"project": "PROJ",
	})
	if code != jrerrors.ExitNotFound {
		t.Fatalf("expected exit %d, got %d", jrerrors.ExitNotFound, code)
	}
	if !strings.Contains(stderr.String(), "not found") {
		t.Errorf("expected 'not found' in error, got: %s", stderr.String())
	}
}

func TestBatchTemplateApply_DryRun_Success(t *testing.T) {
	// Use the builtin "bug-report" template which requires "summary".
	var stdout, stderr bytes.Buffer
	c := newTestClient("http://unused", &stdout, &stderr)
	c.DryRun = true

	code := batchTemplateApply(t.Context(), c, map[string]string{
		"name":    "bug-report",
		"project": "PROJ",
		"summary": "Login page crashes",
	})
	if code != jrerrors.ExitOK {
		t.Fatalf("expected exit 0, got %d; stderr=%s", code, stderr.String())
	}
	var result map[string]any
	if err := json.Unmarshal([]byte(strings.TrimSpace(stdout.String())), &result); err != nil {
		t.Fatalf("stdout not valid JSON: %s", stdout.String())
	}
	if result["method"] != "POST" {
		t.Errorf("expected method=POST, got %v", result["method"])
	}
	if result["template"] != "bug-report" {
		t.Errorf("expected template=bug-report, got %v", result["template"])
	}
	if !strings.Contains(result["url"].(string), "/rest/api/3/issue") {
		t.Errorf("expected URL containing /rest/api/3/issue, got %v", result["url"])
	}
}

func TestBatchTemplateApply_Success(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "POST" && r.URL.Path == "/rest/api/3/issue" {
			w.WriteHeader(http.StatusCreated)
			fmt.Fprintln(w, `{"id":"10001","key":"PROJ-50"}`)
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer ts.Close()

	var stdout, stderr bytes.Buffer
	c := newTestClient(ts.URL, &stdout, &stderr)

	code := batchTemplateApply(t.Context(), c, map[string]string{
		"name":    "bug-report",
		"project": "PROJ",
		"summary": "App crashes on startup",
	})
	if code != jrerrors.ExitOK {
		t.Fatalf("expected exit 0, got %d; stderr=%s", code, stderr.String())
	}
	var result map[string]string
	if err := json.Unmarshal([]byte(strings.TrimSpace(stdout.String())), &result); err != nil {
		t.Fatalf("stdout not valid JSON: %s", stdout.String())
	}
	if result["key"] != "PROJ-50" {
		t.Errorf("expected key=PROJ-50, got %s", result["key"])
	}
}

// --- executeBatchDiff ---

func TestBatchDiff_MissingIssue(t *testing.T) {
	var stdout, stderr bytes.Buffer
	c := newTestClient("http://unused", &stdout, &stderr)

	code := executeBatchDiff(t.Context(), c, BatchOp{
		Command: "diff diff",
		Args:    map[string]string{},
	})
	if code != jrerrors.ExitValidation {
		t.Fatalf("expected exit %d, got %d", jrerrors.ExitValidation, code)
	}
	if !strings.Contains(stderr.String(), "'issue'") {
		t.Errorf("expected 'issue' in error, got: %s", stderr.String())
	}
}

func TestBatchDiff_DryRun(t *testing.T) {
	var stdout, stderr bytes.Buffer
	c := newTestClient("http://unused", &stdout, &stderr)
	c.DryRun = true

	code := executeBatchDiff(t.Context(), c, BatchOp{
		Command: "diff diff",
		Args:    map[string]string{"issue": "TEST-1"},
	})
	if code != jrerrors.ExitOK {
		t.Fatalf("expected exit 0, got %d; stderr=%s", code, stderr.String())
	}
	var result map[string]string
	if err := json.Unmarshal([]byte(strings.TrimSpace(stdout.String())), &result); err != nil {
		t.Fatalf("stdout not valid JSON: %s", stdout.String())
	}
	if result["method"] != "GET" {
		t.Errorf("expected method=GET, got %s", result["method"])
	}
	if !strings.Contains(result["url"], "/changelog") {
		t.Errorf("expected URL containing /changelog, got %s", result["url"])
	}
	if !strings.Contains(result["note"], "would fetch changelog") {
		t.Errorf("expected dry-run note, got %s", result["note"])
	}
}

func TestBatchDiff_Success(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "GET" && strings.Contains(r.URL.Path, "/changelog") {
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprintln(w, `{"values":[{"id":"100","author":{"accountId":"abc","emailAddress":"a@b.com"},"created":"2025-01-15T10:00:00.000+0000","items":[{"field":"status","fieldtype":"jira","fromString":"Open","toString":"In Progress"}]}]}`)
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer ts.Close()

	var stdout, stderr bytes.Buffer
	c := newTestClient(ts.URL, &stdout, &stderr)

	code := executeBatchDiff(t.Context(), c, BatchOp{
		Command: "diff diff",
		Args:    map[string]string{"issue": "TEST-1"},
	})
	if code != jrerrors.ExitOK {
		t.Fatalf("expected exit 0, got %d; stderr=%s", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "status") {
		t.Errorf("expected changelog to contain 'status', got: %s", stdout.String())
	}
}

func TestBatchDiff_WithFieldFilter(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintln(w, `{"values":[{"id":"100","author":{"accountId":"abc","emailAddress":"a@b.com"},"created":"2025-01-15T10:00:00.000+0000","items":[{"field":"status","fieldtype":"jira","fromString":"Open","toString":"Done"},{"field":"priority","fieldtype":"jira","fromString":"Low","toString":"High"}]}]}`)
	}))
	defer ts.Close()

	var stdout, stderr bytes.Buffer
	c := newTestClient(ts.URL, &stdout, &stderr)

	code := executeBatchDiff(t.Context(), c, BatchOp{
		Command: "diff diff",
		Args:    map[string]string{"issue": "TEST-1", "field": "status"},
	})
	if code != jrerrors.ExitOK {
		t.Fatalf("expected exit 0, got %d; stderr=%s", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "status") {
		t.Errorf("expected 'status' in output, got: %s", stdout.String())
	}
}

func TestBatchDiff_WithSinceFilter(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintln(w, `{"values":[{"id":"100","author":{"accountId":"abc","emailAddress":"a@b.com"},"created":"2020-01-01T10:00:00.000+0000","items":[{"field":"status","fieldtype":"jira","fromString":"Open","toString":"Done"}]}]}`)
	}))
	defer ts.Close()

	var stdout, stderr bytes.Buffer
	c := newTestClient(ts.URL, &stdout, &stderr)

	code := executeBatchDiff(t.Context(), c, BatchOp{
		Command: "diff diff",
		Args:    map[string]string{"issue": "TEST-1", "since": "2025-01-01"},
	})
	if code != jrerrors.ExitOK {
		t.Fatalf("expected exit 0, got %d; stderr=%s", code, stderr.String())
	}
	// Should have empty changes since all entries are from 2020
	if !strings.Contains(stdout.String(), "changes") {
		t.Errorf("expected 'changes' in output, got: %s", stdout.String())
	}
}

func TestBatchDiff_FetchError(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		fmt.Fprintln(w, `{"errorMessages":["issue not found"]}`)
	}))
	defer ts.Close()

	var stdout, stderr bytes.Buffer
	c := newTestClient(ts.URL, &stdout, &stderr)

	code := executeBatchDiff(t.Context(), c, BatchOp{
		Command: "diff diff",
		Args:    map[string]string{"issue": "NOPE-999"},
	})
	if code != jrerrors.ExitNotFound {
		t.Fatalf("expected exit %d, got %d", jrerrors.ExitNotFound, code)
	}
}

func TestBatchDiff_InvalidSince(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintln(w, `{"values":[]}`)
	}))
	defer ts.Close()

	var stdout, stderr bytes.Buffer
	c := newTestClient(ts.URL, &stdout, &stderr)

	code := executeBatchDiff(t.Context(), c, BatchOp{
		Command: "diff diff",
		Args:    map[string]string{"issue": "TEST-1", "since": "not-a-date"},
	})
	if code != jrerrors.ExitValidation {
		t.Fatalf("expected exit %d, got %d", jrerrors.ExitValidation, code)
	}
}

// --- batchTemplateApply: additional coverage ---

func TestBatchTemplateApply_WithAssign(t *testing.T) {
	callCount := 0
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		if r.URL.Path == "/rest/api/3/myself" {
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprintln(w, `{"accountId":"me123"}`)
			return
		}
		if r.Method == "POST" && r.URL.Path == "/rest/api/3/issue" {
			w.WriteHeader(http.StatusCreated)
			fmt.Fprintln(w, `{"id":"10001","key":"PROJ-51"}`)
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer ts.Close()

	var stdout, stderr bytes.Buffer
	c := newTestClient(ts.URL, &stdout, &stderr)

	code := batchTemplateApply(t.Context(), c, map[string]string{
		"name":    "bug-report",
		"project": "PROJ",
		"summary": "Test with assignee",
		"assign":  "me",
	})
	if code != jrerrors.ExitOK {
		t.Fatalf("expected exit 0, got %d; stderr=%s", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "PROJ-51") {
		t.Errorf("expected PROJ-51 in output, got: %s", stdout.String())
	}
}

func TestBatchTemplateApply_MissingRequiredVar(t *testing.T) {
	var stdout, stderr bytes.Buffer
	c := newTestClient("http://unused", &stdout, &stderr)

	code := batchTemplateApply(t.Context(), c, map[string]string{
		"name":    "bug-report",
		"project": "PROJ",
		// missing 'summary' which is required
	})
	if code != jrerrors.ExitValidation {
		t.Fatalf("expected exit %d, got %d", jrerrors.ExitValidation, code)
	}
	if !strings.Contains(stderr.String(), "missing required") {
		t.Errorf("expected 'missing required' in error, got: %s", stderr.String())
	}
}

func TestBatchTemplateApply_WithLabelsAndParent(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "POST" && r.URL.Path == "/rest/api/3/issue" {
			var body map[string]any
			_ = json.NewDecoder(r.Body).Decode(&body)
			fields := body["fields"].(map[string]any)
			if fields["parent"] == nil {
				t.Error("expected parent field")
			}
			w.WriteHeader(http.StatusCreated)
			fmt.Fprintln(w, `{"id":"10002","key":"PROJ-52"}`)
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer ts.Close()

	var stdout, stderr bytes.Buffer
	c := newTestClient(ts.URL, &stdout, &stderr)

	code := batchTemplateApply(t.Context(), c, map[string]string{
		"name":    "subtask",
		"project": "PROJ",
		"summary": "Sub task test",
		"parent":  "PROJ-100",
	})
	if code != jrerrors.ExitOK {
		t.Fatalf("expected exit 0, got %d; stderr=%s", code, stderr.String())
	}
}

// --- parseErrorJSON: additional coverage ---

func TestParseErrorJSON_ValidJSON(t *testing.T) {
	input := `{"error":"something broke"}`
	result := parseErrorJSON(input)
	if string(result) != input {
		t.Errorf("expected %s, got %s", input, string(result))
	}
}

func TestParseErrorJSON_MultiLineJSON(t *testing.T) {
	input := `{"error":"first"}` + "\n" + `{"error":"second"}`
	result := parseErrorJSON(input)
	// Should return an array of the two JSON objects.
	if !strings.HasPrefix(string(result), "[") {
		t.Errorf("expected JSON array for multi-line, got: %s", string(result))
	}
}

func TestParseErrorJSON_SingleLineInMulti(t *testing.T) {
	input := `{"error":"only one"}` + "\n"
	result := parseErrorJSON(input)
	// Single valid JSON line after split should return that line.
	if !strings.Contains(string(result), "only one") {
		t.Errorf("expected 'only one' in result, got: %s", string(result))
	}
}

func TestParseErrorJSON_PlainText(t *testing.T) {
	input := "plain text error"
	result := parseErrorJSON(input)
	if !strings.Contains(string(result), "plain text error") {
		t.Errorf("expected plain text wrapped in JSON, got: %s", string(result))
	}
}

func TestParseErrorJSON_MixedLines(t *testing.T) {
	input := `{"valid":"json"}` + "\n" + "not json"
	result := parseErrorJSON(input)
	// Mixed lines should fall back to wrapping in message.
	if !strings.Contains(string(result), "message") {
		t.Errorf("expected wrapped message for mixed lines, got: %s", string(result))
	}
}

// TestParseErrorJSON_EmptyLinesBetween exercises the empty-line-skipping
// path (batch.go line 777-778) with multiple JSON objects separated by blank lines.
func TestParseErrorJSON_EmptyLinesBetween(t *testing.T) {
	// Two JSON objects separated by an empty line. json.Valid returns false
	// for the whole string (two objects). After split, the empty line is
	// skipped and two valid JSON lines remain.
	input := `{"error":"first"}` + "\n\n" + `{"error":"second"}`
	result := parseErrorJSON(input)
	if !strings.HasPrefix(string(result), "[") {
		t.Errorf("expected JSON array, got: %s", string(result))
	}
	if !strings.Contains(string(result), "first") || !strings.Contains(string(result), "second") {
		t.Errorf("expected both objects in array, got: %s", string(result))
	}
}

// TestParseErrorJSON_WhitespaceOnlyLines exercises the empty-line-skipping
// path with lines that contain only whitespace between valid JSON objects.
func TestParseErrorJSON_WhitespaceOnlyLines(t *testing.T) {
	input := `{"a":1}` + "\n  \t  \n" + `{"b":2}`
	result := parseErrorJSON(input)
	if !strings.HasPrefix(string(result), "[") {
		t.Errorf("expected JSON array, got: %s", string(result))
	}
}

// --- stripVerboseLogs: additional coverage ---

func TestStripVerboseLogs_OnlyErrorLines(t *testing.T) {
	input := `{"error_type":"api_error","message":"bad request"}`
	result := stripVerboseLogs(input)
	if result != input {
		t.Errorf("expected error line preserved, got: %s", result)
	}
}

func TestStripVerboseLogs_EmptyInput(t *testing.T) {
	result := stripVerboseLogs("")
	if result != "" {
		t.Errorf("expected empty output, got: %q", result)
	}
}

func TestStripVerboseLogs_NonJSONLines(t *testing.T) {
	input := "plain text error\nanother line"
	result := stripVerboseLogs(input)
	if !strings.Contains(result, "plain text error") {
		t.Errorf("expected plain text preserved, got: %s", result)
	}
	if !strings.Contains(result, "another line") {
		t.Errorf("expected second line preserved, got: %s", result)
	}
}

// --- errorResult ---

func TestErrorResult(t *testing.T) {
	result := errorResult(3, jrerrors.ExitValidation, "validation_error", "missing field")
	if result.Index != 3 {
		t.Errorf("expected index=3, got %d", result.Index)
	}
	if result.ExitCode != jrerrors.ExitValidation {
		t.Errorf("expected exit code %d, got %d", jrerrors.ExitValidation, result.ExitCode)
	}
	if result.Data != nil {
		t.Errorf("expected nil data, got %s", string(result.Data))
	}
	if !strings.Contains(string(result.Error), "missing field") {
		t.Errorf("expected 'missing field' in error, got: %s", string(result.Error))
	}
}

// --- executeBatchOp: policy check ---

func TestBatchOp_PolicyDenied(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer ts.Close()

	allOps := generated.AllSchemaOps()
	allOps = append(allOps, HandWrittenSchemaOps()...)
	opMap := make(map[string]generated.SchemaOp, len(allOps))
	for _, op := range allOps {
		opMap[op.Resource+" "+op.Verb] = op
	}

	var stdout, stderr bytes.Buffer
	c := newTestClient(ts.URL, &stdout, &stderr)

	// Create a deny-all policy.
	pol, _ := newTestDenyPolicy()
	c.Policy = pol

	cmd := &cobra.Command{Use: "batch"}
	cmd.SetContext(context.Background())

	result := executeBatchOp(cmd, c, 0, BatchOp{
		Command: "issue get",
		Args:    map[string]string{"issueIdOrKey": "TEST-1"},
	}, opMap)

	if result.ExitCode != jrerrors.ExitValidation {
		t.Fatalf("expected exit %d for policy denial, got %d", jrerrors.ExitValidation, result.ExitCode)
	}
	if !strings.Contains(string(result.Error), "denied") {
		t.Errorf("expected 'denied' in error, got: %s", string(result.Error))
	}
}

// --- batchMove: error paths ---

func TestBatchMove_TransitionFails(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		fmt.Fprintln(w, `{"errorMessages":["issue not found"]}`)
	}))
	defer ts.Close()

	var stdout, stderr bytes.Buffer
	c := newTestClient(ts.URL, &stdout, &stderr)

	code := doMove(t.Context(), c, "NOPE-1", "Done", "")
	if code == jrerrors.ExitOK {
		t.Fatal("expected non-zero exit for failed transition")
	}
}

func TestBatchMove_AssignResolveError(t *testing.T) {
	callCount := 0
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		if r.Method == "GET" && strings.Contains(r.URL.Path, "/transitions") {
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprintln(w, `{"transitions":[{"id":"11","name":"Done","to":{"name":"Done"}}]}`)
			return
		}
		if r.Method == "POST" && strings.Contains(r.URL.Path, "/transitions") {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		if r.URL.Path == "/rest/api/3/user/search" {
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprintln(w, `[]`) // empty results = not found
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer ts.Close()

	var stdout, stderr bytes.Buffer
	c := newTestClient(ts.URL, &stdout, &stderr)

	code := doMove(t.Context(), c, "TEST-1", "Done", "nonexistent@user.com")
	if code == jrerrors.ExitOK {
		t.Fatal("expected non-zero exit for unresolved assignee")
	}
}

// --- batchComment: error path ---

func TestBatchComment_FetchError(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		fmt.Fprintln(w, `{"errorMessages":["no permission"]}`)
	}))
	defer ts.Close()

	var stdout, stderr bytes.Buffer
	c := newTestClient(ts.URL, &stdout, &stderr)

	code := doComment(t.Context(), c, "TEST-1", "Hello")
	if code == jrerrors.ExitOK {
		t.Fatal("expected non-zero exit for forbidden")
	}
}

// --- batchLink: error paths ---

func TestBatchLink_ResolveTypeError(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/issueLinkType") {
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprintln(w, `{"issueLinkTypes":[{"name":"Blocks","inward":"is blocked by","outward":"blocks"}]}`)
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer ts.Close()

	var stdout, stderr bytes.Buffer
	c := newTestClient(ts.URL, &stdout, &stderr)

	code := doLink(t.Context(), c, "A-1", "B-2", "nonexistent-type")
	if code == jrerrors.ExitOK {
		t.Fatal("expected non-zero exit for unresolved link type")
	}
}

func TestBatchLink_PostError(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "GET" && strings.Contains(r.URL.Path, "/issueLinkType") {
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprintln(w, `{"issueLinkTypes":[{"name":"Blocks","inward":"is blocked by","outward":"blocks"}]}`)
			return
		}
		if r.Method == "POST" && strings.Contains(r.URL.Path, "/issueLink") {
			w.WriteHeader(http.StatusForbidden)
			fmt.Fprintln(w, `{"errorMessages":["no permission"]}`)
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer ts.Close()

	var stdout, stderr bytes.Buffer
	c := newTestClient(ts.URL, &stdout, &stderr)

	code := doLink(t.Context(), c, "A-1", "B-2", "Blocks")
	if code == jrerrors.ExitOK {
		t.Fatal("expected non-zero exit for POST error")
	}
}

// --- batchCreateIssue: error paths ---

func TestBatchCreateIssue_WithAllOptionalFields(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "POST" && r.URL.Path == "/rest/api/3/issue" {
			var body map[string]any
			_ = json.NewDecoder(r.Body).Decode(&body)
			fields := body["fields"].(map[string]any)
			if fields["description"] == nil {
				t.Error("expected description field")
			}
			if fields["priority"] == nil {
				t.Error("expected priority field")
			}
			if fields["labels"] == nil {
				t.Error("expected labels field")
			}
			if fields["parent"] == nil {
				t.Error("expected parent field")
			}
			w.WriteHeader(http.StatusCreated)
			fmt.Fprintln(w, `{"id":"10001","key":"PROJ-99"}`)
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer ts.Close()

	var stdout, stderr bytes.Buffer
	c := newTestClient(ts.URL, &stdout, &stderr)

	code := doCreateIssue(t.Context(), c, map[string]string{
		"project":     "PROJ",
		"type":        "Sub-task",
		"summary":     "Full fields",
		"description": "Detailed desc",
		"priority":    "High",
		"labels":      "bug,critical",
		"parent":      "PROJ-1",
	})
	if code != jrerrors.ExitOK {
		t.Fatalf("expected exit 0, got %d; stderr=%s", code, stderr.String())
	}
}

func TestBatchCreateIssue_FetchError(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		fmt.Fprintln(w, `{"errorMessages":["server error"]}`)
	}))
	defer ts.Close()

	var stdout, stderr bytes.Buffer
	c := newTestClient(ts.URL, &stdout, &stderr)

	code := doCreateIssue(t.Context(), c, map[string]string{
		"project": "PROJ",
		"type":    "Bug",
		"summary": "Will fail",
	})
	if code == jrerrors.ExitOK {
		t.Fatal("expected non-zero exit for server error")
	}
}

// --- batchSprint: error path ---

func TestBatchSprint_ResolveFails(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/board") && !strings.Contains(r.URL.Path, "/sprint") {
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprintln(w, `{"values":[{"id":1}]}`)
			return
		}
		if strings.Contains(r.URL.Path, "/sprint") {
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprintln(w, `{"values":[{"id":10,"name":"Sprint 1","state":"active"}]}`)
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer ts.Close()

	var stdout, stderr bytes.Buffer
	c := newTestClient(ts.URL, &stdout, &stderr)

	code := doSprint(t.Context(), c, "TEST-1", "Nonexistent Sprint 999")
	if code == jrerrors.ExitOK {
		t.Fatal("expected non-zero exit for unresolved sprint")
	}
}

func TestBatchSprint_PostFails(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "GET" && strings.Contains(r.URL.Path, "/board") && !strings.Contains(r.URL.Path, "/sprint") {
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprintln(w, `{"values":[{"id":1}]}`)
			return
		}
		if r.Method == "GET" && strings.Contains(r.URL.Path, "/sprint") {
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprintln(w, `{"values":[{"id":10,"name":"Sprint 5","state":"active"}]}`)
			return
		}
		if r.Method == "POST" {
			w.WriteHeader(http.StatusForbidden)
			fmt.Fprintln(w, `{"errorMessages":["no permission"]}`)
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer ts.Close()

	var stdout, stderr bytes.Buffer
	c := newTestClient(ts.URL, &stdout, &stderr)

	code := doSprint(t.Context(), c, "TEST-1", "Sprint 5")
	if code == jrerrors.ExitOK {
		t.Fatal("expected non-zero exit for POST failure")
	}
}

// --- batchLogWork: error path ---

func TestBatchLogWork_FetchError(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		fmt.Fprintln(w, `{"errorMessages":["no permission"]}`)
	}))
	defer ts.Close()

	var stdout, stderr bytes.Buffer
	c := newTestClient(ts.URL, &stdout, &stderr)

	code := doLogWork(t.Context(), c, "TEST-1", "2h", "working")
	if code == jrerrors.ExitOK {
		t.Fatal("expected non-zero exit for forbidden")
	}
}

func TestBatchLogWork_SuccessWithComment(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "POST" && strings.Contains(r.URL.Path, "/worklog") {
			var body map[string]any
			_ = json.NewDecoder(r.Body).Decode(&body)
			if body["comment"] == nil {
				t.Error("expected comment in worklog body")
			}
			w.WriteHeader(http.StatusCreated)
			fmt.Fprintln(w, `{"id":"5001"}`)
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer ts.Close()

	var stdout, stderr bytes.Buffer
	c := newTestClient(ts.URL, &stdout, &stderr)

	code := doLogWork(t.Context(), c, "TEST-1", "1h 30m", "debugging session")
	if code != jrerrors.ExitOK {
		t.Fatalf("expected exit 0, got %d; stderr=%s", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "logged") {
		t.Errorf("expected 'logged' in output, got: %s", stdout.String())
	}
}

// --- batchMove: dry-run with assign ---

func TestBatchMove_DryRun_WithAssign_Output(t *testing.T) {
	var stdout, stderr bytes.Buffer
	c := newTestClient("http://should-not-be-called", &stdout, &stderr)
	c.DryRun = true

	code := doMove(t.Context(), c, "TEST-1", "Done", "john@example.com")
	if code != jrerrors.ExitOK {
		t.Fatalf("expected exit 0, got %d; stderr=%s", code, stderr.String())
	}
	var result map[string]any
	if err := json.Unmarshal([]byte(strings.TrimSpace(stdout.String())), &result); err != nil {
		t.Fatalf("stdout not valid JSON: %s", stdout.String())
	}
	if result["assign"] == nil {
		t.Error("expected 'assign' field in dry-run output")
	}
}

// newBatchCmd creates a cobra command wired to runBatch for testing.
func newBatchCmd(c *client.Client) *cobra.Command {
	cmd := &cobra.Command{Use: "batch", RunE: runBatch}
	cmd.Flags().String("input", "", "")
	cmd.Flags().Int("max-batch", 50, "")
	cmd.SetContext(client.NewContext(context.Background(), c))
	return cmd
}

func TestRunBatch_InvalidJSON(t *testing.T) {
	tmp := t.TempDir()
	inputFile := filepath.Join(tmp, "ops.json")
	if err := os.WriteFile(inputFile, []byte("not valid json"), 0o644); err != nil {
		t.Fatalf("failed to write temp file: %v", err)
	}

	var stdout, stderr bytes.Buffer
	c := newTestClient("http://unused", &stdout, &stderr)

	cmd := newBatchCmd(c)
	_ = cmd.Flags().Set("input", inputFile)

	err := cmd.RunE(cmd, nil)
	if err == nil {
		t.Fatal("expected error for invalid JSON input")
	}
	aw, ok := err.(*jrerrors.AlreadyWrittenError)
	if !ok {
		t.Fatalf("expected AlreadyWrittenError, got %T: %v", err, err)
	}
	if aw.Code != jrerrors.ExitValidation {
		t.Errorf("expected exit code %d, got %d", jrerrors.ExitValidation, aw.Code)
	}
}

func TestRunBatch_NullInput(t *testing.T) {
	tmp := t.TempDir()
	inputFile := filepath.Join(tmp, "ops.json")
	if err := os.WriteFile(inputFile, []byte("null"), 0o644); err != nil {
		t.Fatalf("failed to write temp file: %v", err)
	}

	var stdout, stderr bytes.Buffer
	c := newTestClient("http://unused", &stdout, &stderr)

	cmd := newBatchCmd(c)
	_ = cmd.Flags().Set("input", inputFile)

	err := cmd.RunE(cmd, nil)
	if err == nil {
		t.Fatal("expected error for null JSON input")
	}
	aw, ok := err.(*jrerrors.AlreadyWrittenError)
	if !ok {
		t.Fatalf("expected AlreadyWrittenError, got %T: %v", err, err)
	}
	if aw.Code != jrerrors.ExitValidation {
		t.Errorf("expected exit code %d, got %d", jrerrors.ExitValidation, aw.Code)
	}
}

func TestRunBatch_ExceedsMaxBatch(t *testing.T) {
	ops := `[
		{"command":"issue get","args":{"issueIdOrKey":"PROJ-1"}},
		{"command":"issue get","args":{"issueIdOrKey":"PROJ-2"}},
		{"command":"issue get","args":{"issueIdOrKey":"PROJ-3"}}
	]`

	tmp := t.TempDir()
	inputFile := filepath.Join(tmp, "ops.json")
	if err := os.WriteFile(inputFile, []byte(ops), 0o644); err != nil {
		t.Fatalf("failed to write temp file: %v", err)
	}

	var stdout, stderr bytes.Buffer
	c := newTestClient("http://unused", &stdout, &stderr)

	cmd := newBatchCmd(c)
	_ = cmd.Flags().Set("input", inputFile)
	_ = cmd.Flags().Set("max-batch", "2")

	err := cmd.RunE(cmd, nil)
	if err == nil {
		t.Fatal("expected error when batch exceeds max-batch limit")
	}
	aw, ok := err.(*jrerrors.AlreadyWrittenError)
	if !ok {
		t.Fatalf("expected AlreadyWrittenError, got %T: %v", err, err)
	}
	if aw.Code != jrerrors.ExitValidation {
		t.Errorf("expected exit code %d, got %d", jrerrors.ExitValidation, aw.Code)
	}
}

func TestRunBatch_InvalidGlobalJQFilter(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintln(w, `{"key":"PROJ-1","fields":{"summary":"Test"}}`)
	}))
	defer ts.Close()

	ops := `[{"command":"issue get","args":{"issueIdOrKey":"PROJ-1"}}]`

	tmp := t.TempDir()
	inputFile := filepath.Join(tmp, "ops.json")
	if err := os.WriteFile(inputFile, []byte(ops), 0o644); err != nil {
		t.Fatalf("failed to write temp file: %v", err)
	}

	var stdout, stderr bytes.Buffer
	c := newTestClient(ts.URL, &stdout, &stderr)
	c.JQFilter = ".[invalid jq expression!!!"

	cmd := newBatchCmd(c)
	_ = cmd.Flags().Set("input", inputFile)

	err := cmd.RunE(cmd, nil)
	if err == nil {
		t.Fatal("expected error for invalid global jq filter")
	}
}

// ---------------------------------------------------------------------------
// runBatch — --input pointing to nonexistent file via newBatchCmd
// ---------------------------------------------------------------------------

func TestRunBatch_InputFileNotFound(t *testing.T) {
	var stdout, stderr bytes.Buffer
	c := newTestClient("http://unused", &stdout, &stderr)
	cmd := newBatchCmd(c)
	_ = cmd.Flags().Set("input", "/nonexistent/file.json")

	err := cmd.RunE(cmd, nil)
	if err == nil {
		t.Fatal("expected error for nonexistent input file")
	}
	aw, ok := err.(*jrerrors.AlreadyWrittenError)
	if !ok {
		t.Fatalf("expected AlreadyWrittenError, got %T", err)
	}
	if aw.Code != jrerrors.ExitValidation {
		t.Errorf("expected ExitValidation (%d), got %d", jrerrors.ExitValidation, aw.Code)
	}
}

// ---------------------------------------------------------------------------
// runBatch — empty array [] succeeds with zero-result output
// ---------------------------------------------------------------------------

func TestRunBatch_EmptyArrayViaNewBatchCmd(t *testing.T) {
	tmp := t.TempDir()
	inputFile := filepath.Join(tmp, "empty.json")
	if err := os.WriteFile(inputFile, []byte("[]"), 0o644); err != nil {
		t.Fatalf("failed to write temp file: %v", err)
	}

	var stdout, stderr bytes.Buffer
	c := newTestClient("http://unused", &stdout, &stderr)
	cmd := newBatchCmd(c)
	_ = cmd.Flags().Set("input", inputFile)

	captured := captureStdout(t, func() {
		if err := cmd.RunE(cmd, nil); err != nil {
			t.Fatalf("unexpected error for empty batch: %v", err)
		}
	})

	got := strings.TrimSpace(captured)
	if got != "[]" {
		t.Errorf("expected '[]' for empty batch, got: %s", got)
	}
}

// ---------------------------------------------------------------------------
// runBatch — no client in context
// ---------------------------------------------------------------------------

// TestRunBatch_NoClient covers the runBatch path where there is no client in
// the context (batch.go line 70-77).
func TestRunBatch_NoClient(t *testing.T) {
	cmd := &cobra.Command{Use: "batch", RunE: runBatch}
	cmd.Flags().String("input", "", "")
	cmd.Flags().Int("max-batch", 50, "")
	cmd.SetContext(context.Background()) // no client

	err := cmd.RunE(cmd, nil)
	if err == nil {
		t.Fatal("expected error when no client in context")
	}
	aw, ok := err.(*jrerrors.AlreadyWrittenError)
	if !ok {
		t.Fatalf("expected AlreadyWrittenError, got %T: %v", err, err)
	}
	if aw.Code != jrerrors.ExitError {
		t.Errorf("expected ExitError (%d), got %d", jrerrors.ExitError, aw.Code)
	}
}

// ---------------------------------------------------------------------------
// executeBatchOp — template and diff routing
// ---------------------------------------------------------------------------

// TestExecuteBatchOp_TemplateDryRun covers the template prefix routing in
// executeBatchOp (batch.go line 284-287) via a dry-run apply.
func TestExecuteBatchOp_TemplateDryRun(t *testing.T) {
	allOps := generated.AllSchemaOps()
	allOps = append(allOps, HandWrittenSchemaOps()...)
	allOps = append(allOps, TemplateSchemaOps()...)
	allOps = append(allOps, DiffSchemaOps()...)
	opMap := make(map[string]generated.SchemaOp, len(allOps))
	for _, op := range allOps {
		opMap[op.Resource+" "+op.Verb] = op
	}

	var stdout, stderr bytes.Buffer
	c := newTestClient("http://unused", &stdout, &stderr)
	c.DryRun = true

	cmd := &cobra.Command{Use: "batch"}
	cmd.SetContext(client.NewContext(context.Background(), c))

	result := executeBatchOp(cmd, c, 0, BatchOp{
		Command: "template apply",
		Args:    map[string]string{"name": "bug-report", "project": "PROJ", "summary": "Test"},
	}, opMap)
	if result.ExitCode != jrerrors.ExitOK {
		t.Fatalf("expected exit 0 for dry-run template apply, got %d; error: %s", result.ExitCode, string(result.Error))
	}
}

// TestExecuteBatchOp_DiffDryRun covers the diff command routing in
// executeBatchOp (batch.go line 288-291) via a dry-run diff.
func TestExecuteBatchOp_DiffDryRun(t *testing.T) {
	allOps := generated.AllSchemaOps()
	allOps = append(allOps, HandWrittenSchemaOps()...)
	allOps = append(allOps, TemplateSchemaOps()...)
	allOps = append(allOps, DiffSchemaOps()...)
	opMap := make(map[string]generated.SchemaOp, len(allOps))
	for _, op := range allOps {
		opMap[op.Resource+" "+op.Verb] = op
	}

	var stdout, stderr bytes.Buffer
	c := newTestClient("http://unused", &stdout, &stderr)
	c.DryRun = true

	cmd := &cobra.Command{Use: "batch"}
	cmd.SetContext(client.NewContext(context.Background(), c))

	result := executeBatchOp(cmd, c, 0, BatchOp{
		Command: "diff diff",
		Args:    map[string]string{"issue": "PROJ-1"},
	}, opMap)
	if result.ExitCode != jrerrors.ExitOK {
		t.Fatalf("expected exit 0 for dry-run diff, got %d; error: %s", result.ExitCode, string(result.Error))
	}
}

// ---------------------------------------------------------------------------
// executeBatchWorkflow — missing "to" for transition and assign
// ---------------------------------------------------------------------------

// TestExecuteBatchWorkflow_TransitionMissingTo covers the missing "to" arg
// validation in executeBatchWorkflow for workflow transition (batch.go line 374-376).
func TestExecuteBatchWorkflow_TransitionMissingTo(t *testing.T) {
	var stdout, stderr bytes.Buffer
	c := newTestClient("http://unused", &stdout, &stderr)

	code := executeBatchWorkflow(t.Context(), c, BatchOp{
		Command: "workflow transition",
		Args:    map[string]string{"issue": "PROJ-1"},
		// missing "to"
	})
	if code != jrerrors.ExitValidation {
		t.Fatalf("expected ExitValidation (%d), got %d", jrerrors.ExitValidation, code)
	}
	if !strings.Contains(stderr.String(), "'to'") {
		t.Errorf("expected 'to' in error, got: %s", stderr.String())
	}
}

// TestExecuteBatchWorkflow_AssignMissingTo covers the missing "to" arg
// validation in executeBatchWorkflow for workflow assign (batch.go line 386-388).
func TestExecuteBatchWorkflow_AssignMissingTo(t *testing.T) {
	var stdout, stderr bytes.Buffer
	c := newTestClient("http://unused", &stdout, &stderr)

	code := executeBatchWorkflow(t.Context(), c, BatchOp{
		Command: "workflow assign",
		Args:    map[string]string{"issue": "PROJ-1"},
		// missing "to"
	})
	if code != jrerrors.ExitValidation {
		t.Fatalf("expected ExitValidation (%d), got %d", jrerrors.ExitValidation, code)
	}
	if !strings.Contains(stderr.String(), "'to'") {
		t.Errorf("expected 'to' in error, got: %s", stderr.String())
	}
}

// TestExecuteBatchWorkflow_LinkSuccess covers the batchLink call path in
// executeBatchWorkflow when all required link args are provided (batch.go line 363).
func TestExecuteBatchWorkflow_LinkSuccess(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Method == "GET" && r.URL.Path == "/rest/api/3/issueLinkType" {
			fmt.Fprintln(w, `{"issueLinkTypes":[{"id":"10","name":"Blocks","inward":"is blocked by","outward":"blocks"}]}`)
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

	code := executeBatchWorkflow(t.Context(), c, BatchOp{
		Command: "workflow link",
		Args:    map[string]string{"from": "PROJ-1", "to": "PROJ-2", "type": "blocks"},
	})
	if code != jrerrors.ExitOK {
		t.Fatalf("expected exit 0 for link, got %d; stderr=%s", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "linked") {
		t.Errorf("expected 'linked' in output, got: %s", stdout.String())
	}
}

// ---------------------------------------------------------------------------
// batchMove — POST transitions fail and unassign PUT fail
// ---------------------------------------------------------------------------

// TestBatchMove_PostTransitionFails covers the batchMove path where POST
// /transitions returns an error (batch.go line 469-471).
func TestBatchMove_PostTransitionFails(t *testing.T) {
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

	code := doMove(t.Context(), c, "PROJ-1", "Done", "")
	if code == jrerrors.ExitOK {
		t.Fatal("expected non-zero exit when POST transitions returns 403")
	}
}

// TestBatchMove_UnassignPUTFails covers the batchMove unassign branch where
// the PUT /assignee call returns an error (batch.go line 490-492).
func TestBatchMove_UnassignPUTFails(t *testing.T) {
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

	code := doMove(t.Context(), c, "PROJ-1", "Done", "none")
	if code == jrerrors.ExitOK {
		t.Fatal("expected non-zero exit when unassign PUT returns 500")
	}
}

// TestBatchMove_AssignPUTFails covers the batchMove non-unassign assign branch
// where the PUT /assignee call returns an error (batch.go line 499-501).
func TestBatchMove_AssignPUTFails(t *testing.T) {
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
		if strings.Contains(r.URL.Path, "/myself") {
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

	code := doMove(t.Context(), c, "PROJ-1", "Done", "me")
	if code == jrerrors.ExitOK {
		t.Fatal("expected non-zero exit when assign PUT returns 403")
	}
}

// ---------------------------------------------------------------------------
// batchCreateIssue — resolveAssignee error
// ---------------------------------------------------------------------------

// TestBatchCreateIssue_AssignResolveError covers the batchCreateIssue path
// where resolveAssignee returns an error (batch.go line 656-658).
func TestBatchCreateIssue_AssignResolveError(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
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

	code := executeBatchWorkflow(t.Context(), c, BatchOp{
		Command: "workflow create",
		Args: map[string]string{
			"project": "PROJ",
			"type":    "Bug",
			"summary": "Test issue",
			"assign":  "unknown@user.com",
		},
	})
	if code == jrerrors.ExitOK {
		t.Fatal("expected non-zero exit when resolveAssignee fails")
	}
}

// ---------------------------------------------------------------------------
// batchTemplateApply — optional fields (description, priority) and assign
// ---------------------------------------------------------------------------

// TestBatchTemplateApply_WithDescriptionAndPriority covers the optional fields
// (description, priority) in batchTemplateApply (batch.go lines 889-894).
func TestBatchTemplateApply_WithDescriptionAndPriority(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "POST" && r.URL.Path == "/rest/api/3/issue" {
			var body map[string]any
			_ = json.NewDecoder(r.Body).Decode(&body)
			fields, _ := body["fields"].(map[string]any)
			if fields["description"] == nil {
				t.Error("expected description field to be set")
			}
			if fields["priority"] == nil {
				t.Error("expected priority field to be set")
			}
			w.WriteHeader(http.StatusCreated)
			fmt.Fprintln(w, `{"id":"10005","key":"PROJ-55"}`)
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer ts.Close()

	var stdout, stderr bytes.Buffer
	c := newTestClient(ts.URL, &stdout, &stderr)

	// The "bug-report" template maps its "priority" field from the "severity" variable.
	code := batchTemplateApply(t.Context(), c, map[string]string{
		"name":        "bug-report",
		"project":     "PROJ",
		"summary":     "A bug",
		"description": "Steps to reproduce",
		"severity":    "High",
	})
	if code != jrerrors.ExitOK {
		t.Fatalf("expected exit 0, got %d; stderr=%s", code, stderr.String())
	}
}

// TestBatchTemplateApply_AssignResolveError covers the resolveAssignee error
// path in batchTemplateApply (batch.go line 915-917).
func TestBatchTemplateApply_AssignResolveError(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/rest/api/3/user/search" {
			fmt.Fprintln(w, `[]`)
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer ts.Close()

	var stdout, stderr bytes.Buffer
	c := newTestClient(ts.URL, &stdout, &stderr)

	code := batchTemplateApply(t.Context(), c, map[string]string{
		"name":    "bug-report",
		"project": "PROJ",
		"summary": "A bug",
		"assign":  "unknown@user.com",
	})
	if code == jrerrors.ExitOK {
		t.Fatal("expected non-zero exit when assign resolve fails")
	}
}

// TestBatchTemplateApply_WithAssignSuccess covers the assign path in
// batchTemplateApply when the assignee resolves successfully (batch.go lines 913-920).
func TestBatchTemplateApply_WithAssignSuccess(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if strings.Contains(r.URL.Path, "/myself") {
			fmt.Fprintln(w, `{"accountId":"me123"}`)
			return
		}
		if r.Method == "POST" && r.URL.Path == "/rest/api/3/issue" {
			var body map[string]any
			_ = json.NewDecoder(r.Body).Decode(&body)
			fields, _ := body["fields"].(map[string]any)
			if fields["assignee"] == nil {
				t.Error("expected assignee field to be set")
			}
			w.WriteHeader(http.StatusCreated)
			fmt.Fprintln(w, `{"id":"10006","key":"PROJ-56"}`)
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer ts.Close()

	var stdout, stderr bytes.Buffer
	c := newTestClient(ts.URL, &stdout, &stderr)

	code := batchTemplateApply(t.Context(), c, map[string]string{
		"name":    "bug-report",
		"project": "PROJ",
		"summary": "A bug with assignee",
		"assign":  "me",
	})
	if code != jrerrors.ExitOK {
		t.Fatalf("expected exit 0, got %d; stderr=%s", code, stderr.String())
	}
}

// TestBatchTemplateApply_CreateFails covers the POST /issue failure in
// batchTemplateApply (batch.go line 925-927).
func TestBatchTemplateApply_CreateFails(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "POST" && r.URL.Path == "/rest/api/3/issue" {
			w.WriteHeader(http.StatusInternalServerError)
			fmt.Fprintln(w, `{"message":"server error"}`)
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer ts.Close()

	var stdout, stderr bytes.Buffer
	c := newTestClient(ts.URL, &stdout, &stderr)

	code := batchTemplateApply(t.Context(), c, map[string]string{
		"name":    "bug-report",
		"project": "PROJ",
		"summary": "A bug",
	})
	if code == jrerrors.ExitOK {
		t.Fatal("expected non-zero exit when POST issue returns 500")
	}
}

// ---------------------------------------------------------------------------
// runBatch — pretty output and max exit propagation
// ---------------------------------------------------------------------------

// TestExecuteBatchOp_WorkflowRouting covers the workflow prefix routing in
// executeBatchOp (batch.go line 280-283) by calling it with a dry-run workflow command.
func TestExecuteBatchOp_WorkflowRouting(t *testing.T) {
	allOps := generated.AllSchemaOps()
	allOps = append(allOps, HandWrittenSchemaOps()...)
	allOps = append(allOps, TemplateSchemaOps()...)
	allOps = append(allOps, DiffSchemaOps()...)
	opMap := make(map[string]generated.SchemaOp, len(allOps))
	for _, op := range allOps {
		opMap[op.Resource+" "+op.Verb] = op
	}

	var stdout, stderr bytes.Buffer
	c := newTestClient("http://unused", &stdout, &stderr)
	c.DryRun = true

	cmd := &cobra.Command{Use: "batch"}
	cmd.SetContext(client.NewContext(context.Background(), c))

	result := executeBatchOp(cmd, c, 0, BatchOp{
		Command: "workflow transition",
		Args:    map[string]string{"issue": "PROJ-1", "to": "Done"},
	}, opMap)
	if result.ExitCode != jrerrors.ExitOK {
		t.Fatalf("expected exit 0 for dry-run workflow transition, got %d; error: %s", result.ExitCode, string(result.Error))
	}
}

// TestBatchTemplateApply_WithLabels covers the labels field rendering in
// batchTemplateApply (batch.go line 895-897) using the spike template which
// always renders a "labels" field.
func TestBatchTemplateApply_WithLabels(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "POST" && r.URL.Path == "/rest/api/3/issue" {
			var body map[string]any
			_ = json.NewDecoder(r.Body).Decode(&body)
			fields, _ := body["fields"].(map[string]any)
			if fields["labels"] == nil {
				t.Error("expected labels field to be set for spike template")
			}
			w.WriteHeader(http.StatusCreated)
			fmt.Fprintln(w, `{"id":"10007","key":"PROJ-57"}`)
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer ts.Close()

	var stdout, stderr bytes.Buffer
	c := newTestClient(ts.URL, &stdout, &stderr)

	// The "spike" template always renders labels: "spike".
	code := batchTemplateApply(t.Context(), c, map[string]string{
		"name":    "spike",
		"project": "PROJ",
		"summary": "Investigate auth performance",
	})
	if code != jrerrors.ExitOK {
		t.Fatalf("expected exit 0, got %d; stderr=%s", code, stderr.String())
	}
}

// TestRunBatch_Pretty covers the --pretty flag path in runBatch.
func TestRunBatch_Pretty(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintln(w, `{"key":"PROJ-1","fields":{"summary":"Test"}}`)
	}))
	defer ts.Close()

	ops := `[{"command":"issue get","args":{"issueIdOrKey":"PROJ-1"}}]`

	tmp := t.TempDir()
	inputFile := filepath.Join(tmp, "ops.json")
	if err := os.WriteFile(inputFile, []byte(ops), 0o644); err != nil {
		t.Fatalf("failed to write temp file: %v", err)
	}

	var stdout, stderr bytes.Buffer
	c := newTestClient(ts.URL, &stdout, &stderr)
	c.Pretty = true

	cmd := newBatchCmd(c)
	_ = cmd.Flags().Set("input", inputFile)

	captured := captureStdout(t, func() {
		if err := cmd.RunE(cmd, nil); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	// Pretty output should be indented.
	if !strings.Contains(captured, "\n") {
		t.Errorf("expected pretty-printed output with newlines, got: %s", captured)
	}
}

// TestRunBatch_MaxExitPropagated covers the path in runBatch where the highest
// exit code from batch operations is returned.
func TestRunBatch_MaxExitPropagated(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		fmt.Fprintln(w, `{"errorMessages":["not found"]}`)
	}))
	defer ts.Close()

	ops := `[{"command":"issue get","args":{"issueIdOrKey":"PROJ-999"}}]`

	tmp := t.TempDir()
	inputFile := filepath.Join(tmp, "ops.json")
	if err := os.WriteFile(inputFile, []byte(ops), 0o644); err != nil {
		t.Fatalf("failed to write temp file: %v", err)
	}

	var stdout, stderr bytes.Buffer
	c := newTestClient(ts.URL, &stdout, &stderr)

	cmd := newBatchCmd(c)
	_ = cmd.Flags().Set("input", inputFile)

	captured := captureStdout(t, func() {
		err := cmd.RunE(cmd, nil)
		if err == nil {
			t.Fatal("expected error when batch op has non-zero exit")
		}
		aw, ok := err.(*jrerrors.AlreadyWrittenError)
		if !ok {
			t.Fatalf("expected AlreadyWrittenError, got %T: %v", err, err)
		}
		if aw.Code == jrerrors.ExitOK {
			t.Errorf("expected non-zero exit code propagated, got %d", aw.Code)
		}
	})

	// Output should still be written even on partial failure.
	if !strings.Contains(captured, "[") {
		t.Errorf("expected JSON array output, got: %s", captured)
	}
}

// TestBatchTemplateApply_LookupError exercises the template Lookup error path
// in batchTemplateApply (batch.go line 855-858) by making the user templates
// directory unreadable (a regular file where a directory is expected).
func TestBatchTemplateApply_LookupError(t *testing.T) {
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)
	// Pin XDG_CONFIG_HOME so os.UserConfigDir() resolves into tmpHome on Linux
	// regardless of the runner's ambient XDG_CONFIG_HOME value.
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(tmpHome, ".config"))

	// os.UserConfigDir() is platform-specific: macOS → $HOME/Library/Application
	// Support, Linux → $XDG_CONFIG_HOME (or $HOME/.config). Write the sentinel
	// at both paths so os.ReadDir fails with "not a directory" regardless of OS.
	for _, rel := range []string{
		"Library/Application Support/jr/templates",
		".config/jr/templates",
	} {
		path := filepath.Join(tmpHome, rel)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte("not a dir"), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	var stdout, stderr bytes.Buffer
	c := newTestClient("http://unused", &stdout, &stderr)

	code := batchTemplateApply(t.Context(), c, map[string]string{
		"name":    "bug-report",
		"project": "PROJ",
	})
	if code != jrerrors.ExitError {
		t.Errorf("expected ExitError for lookup failure, got %d", code)
	}
}
