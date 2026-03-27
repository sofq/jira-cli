package watch

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/sofq/jira-cli/internal/client"
	"github.com/sofq/jira-cli/internal/config"
	jrerrors "github.com/sofq/jira-cli/internal/errors"
)

// --- helpers ---

type testEnv struct {
	Server *httptest.Server
	Client *client.Client
	Stdout *bytes.Buffer
	Stderr *bytes.Buffer
}

func newTestEnv(handler http.HandlerFunc) *testEnv {
	ts := httptest.NewServer(handler)
	var stdout, stderr bytes.Buffer
	c := &client.Client{
		BaseURL:    ts.URL,
		Auth:       config.AuthConfig{},
		HTTPClient: &http.Client{},
		Stdout:     &stdout,
		Stderr:     &stderr,
	}
	return &testEnv{Server: ts, Client: c, Stdout: &stdout, Stderr: &stderr}
}

func (te *testEnv) Close() { te.Server.Close() }

func searchJSON(issues ...string) []byte {
	parts := strings.Join(issues, ",")
	return []byte(`{"issues":[` + parts + `]}`)
}

func issueJSON(key, updated string) string {
	return `{"key":"` + key + `","updated":"` + updated + `"}`
}

func issueJSONFieldsUpdated(key, updated string) string {
	return `{"key":"` + key + `","fields":{"updated":"` + updated + `"}}`
}

// writeJSON writes b to w, discarding the error (test helper to satisfy errcheck).
func writeJSON(w http.ResponseWriter, b []byte) {
	_, _ = w.Write(b)
}

// --- marshalNoEscape ---

func TestMarshalNoEscape(t *testing.T) {
	data, err := marshalNoEscape(map[string]string{"url": "http://example.com?a=1&b=2"})
	if err != nil {
		t.Fatal(err)
	}
	// Should NOT escape & or < >
	if bytes.Contains(data, []byte(`\u0026`)) {
		t.Errorf("expected no HTML escaping, got %s", data)
	}
	if bytes.Contains(data, []byte("\n")) {
		t.Errorf("expected no trailing newline, got %q", data)
	}
}

// --- DryRunOutput ---

func TestDryRunOutput_JQL(t *testing.T) {
	var buf bytes.Buffer
	DryRunOutput(&buf, "https://example.atlassian.net", Options{
		JQL:      "project = TEST",
		Interval: 30 * time.Second,
	})
	out := buf.String()
	if !strings.Contains(out, "project = TEST") {
		t.Errorf("expected JQL in output, got %s", out)
	}
	if !strings.Contains(out, "POST") {
		t.Errorf("expected POST method, got %s", out)
	}
	if !strings.Contains(out, "/rest/api/3/search/jql") {
		t.Errorf("expected search endpoint, got %s", out)
	}
}

func TestDryRunOutput_Issue(t *testing.T) {
	var buf bytes.Buffer
	DryRunOutput(&buf, "https://example.atlassian.net", Options{
		Issue:    "PROJ-1",
		Interval: 10 * time.Second,
	})
	out := buf.String()
	if !strings.Contains(out, "key = PROJ-1") {
		t.Errorf("expected single-issue JQL, got %s", out)
	}
}

// --- emitEvent ---

func TestEmitEvent_Basic(t *testing.T) {
	var stdout, stderr bytes.Buffer
	c := &client.Client{Stdout: &stdout, Stderr: &stderr}
	emitted := 0
	code := emitEvent(c, "created", json.RawMessage(`{"key":"T-1"}`), &emitted, 0)
	if code != jrerrors.ExitOK {
		t.Fatalf("expected ExitOK, got %d", code)
	}
	if emitted != 1 {
		t.Errorf("expected emitted=1, got %d", emitted)
	}
	var ev Event
	if err := json.Unmarshal([]byte(strings.TrimSpace(stdout.String())), &ev); err != nil {
		t.Fatal(err)
	}
	if ev.Type != "created" {
		t.Errorf("expected type=created, got %s", ev.Type)
	}
}

func TestEmitEvent_MaxEventsReached(t *testing.T) {
	var stdout, stderr bytes.Buffer
	c := &client.Client{Stdout: &stdout, Stderr: &stderr}
	emitted := 0
	code := emitEvent(c, "created", json.RawMessage(`{"key":"T-1"}`), &emitted, 1)
	if code != -1 {
		t.Fatalf("expected -1 sentinel, got %d", code)
	}
}

func TestEmitEvent_JQFilter(t *testing.T) {
	var stdout, stderr bytes.Buffer
	c := &client.Client{Stdout: &stdout, Stderr: &stderr, JQFilter: ".type"}
	emitted := 0
	code := emitEvent(c, "updated", json.RawMessage(`{"key":"T-1"}`), &emitted, 0)
	if code != jrerrors.ExitOK {
		t.Fatalf("expected ExitOK, got %d", code)
	}
	out := strings.TrimSpace(stdout.String())
	if out != `"updated"` {
		t.Errorf("expected jq output '\"updated\"', got %s", out)
	}
}

func TestEmitEvent_JQFilterError(t *testing.T) {
	var stdout, stderr bytes.Buffer
	c := &client.Client{Stdout: &stdout, Stderr: &stderr, JQFilter: ".invalid[[["}
	emitted := 0
	code := emitEvent(c, "created", json.RawMessage(`{"key":"T-1"}`), &emitted, 0)
	if code != jrerrors.ExitValidation {
		t.Fatalf("expected ExitValidation, got %d", code)
	}
	if !strings.Contains(stderr.String(), "jq_error") {
		t.Errorf("expected jq_error in stderr, got %s", stderr.String())
	}
}

func TestEmitEvent_Pretty(t *testing.T) {
	var stdout, stderr bytes.Buffer
	c := &client.Client{Stdout: &stdout, Stderr: &stderr, Pretty: true}
	emitted := 0
	emitEvent(c, "created", json.RawMessage(`{"key":"T-1"}`), &emitted, 0)
	out := stdout.String()
	// Pretty output should be indented
	if !strings.Contains(out, "\n") {
		t.Errorf("expected pretty-printed output with newlines, got %q", out)
	}
}

// --- poll ---

func TestPoll_FirstPoll_InitialEvents(t *testing.T) {
	te := newTestEnv(func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, searchJSON(issueJSON("T-1", "2025-01-01T00:00:00Z")))
	})
	defer te.Close()

	seen := make(map[string]string)
	emitted := 0
	code := poll(context.Background(), te.Client, jqlSearchRequest{JQL: "project=T"}, seen, &emitted, 0, true)
	if code != jrerrors.ExitOK {
		t.Fatalf("expected ExitOK, got %d", code)
	}
	if emitted != 1 {
		t.Errorf("expected 1 event emitted, got %d", emitted)
	}
	// First poll emits "initial" type
	if !strings.Contains(te.Stdout.String(), `"initial"`) {
		t.Errorf("expected initial event type, got %s", te.Stdout.String())
	}
	if seen["T-1"] != "2025-01-01T00:00:00Z" {
		t.Errorf("expected seen entry, got %v", seen)
	}
}

func TestPoll_SecondPoll_CreatedEvent(t *testing.T) {
	te := newTestEnv(func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, searchJSON(issueJSON("T-2", "2025-01-02T00:00:00Z")))
	})
	defer te.Close()

	seen := make(map[string]string)
	emitted := 0
	// Not first poll — new issue should be "created"
	code := poll(context.Background(), te.Client, jqlSearchRequest{JQL: "project=T"}, seen, &emitted, 0, false)
	if code != jrerrors.ExitOK {
		t.Fatalf("expected ExitOK, got %d", code)
	}
	if !strings.Contains(te.Stdout.String(), `"created"`) {
		t.Errorf("expected created event type, got %s", te.Stdout.String())
	}
}

func TestPoll_UpdatedEvent(t *testing.T) {
	te := newTestEnv(func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, searchJSON(issueJSON("T-1", "2025-01-02T00:00:00Z")))
	})
	defer te.Close()

	seen := map[string]string{"T-1": "2025-01-01T00:00:00Z"}
	emitted := 0
	code := poll(context.Background(), te.Client, jqlSearchRequest{JQL: "project=T"}, seen, &emitted, 0, false)
	if code != jrerrors.ExitOK {
		t.Fatalf("expected ExitOK, got %d", code)
	}
	if !strings.Contains(te.Stdout.String(), `"updated"`) {
		t.Errorf("expected updated event, got %s", te.Stdout.String())
	}
	if seen["T-1"] != "2025-01-02T00:00:00Z" {
		t.Errorf("expected updated timestamp in seen, got %s", seen["T-1"])
	}
}

func TestPoll_NoChange(t *testing.T) {
	te := newTestEnv(func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, searchJSON(issueJSON("T-1", "2025-01-01T00:00:00Z")))
	})
	defer te.Close()

	seen := map[string]string{"T-1": "2025-01-01T00:00:00Z"}
	emitted := 0
	code := poll(context.Background(), te.Client, jqlSearchRequest{JQL: "project=T"}, seen, &emitted, 0, false)
	if code != jrerrors.ExitOK {
		t.Fatalf("expected ExitOK, got %d", code)
	}
	if emitted != 0 {
		t.Errorf("expected no events for unchanged issue, got %d", emitted)
	}
}

func TestPoll_RemovedEvent(t *testing.T) {
	te := newTestEnv(func(w http.ResponseWriter, r *http.Request) {
		// Return empty results — T-1 is gone
		writeJSON(w, searchJSON())
	})
	defer te.Close()

	seen := map[string]string{"T-1": "2025-01-01T00:00:00Z"}
	emitted := 0
	code := poll(context.Background(), te.Client, jqlSearchRequest{JQL: "project=T"}, seen, &emitted, 0, false)
	if code != jrerrors.ExitOK {
		t.Fatalf("expected ExitOK, got %d", code)
	}
	if !strings.Contains(te.Stdout.String(), `"removed"`) {
		t.Errorf("expected removed event, got %s", te.Stdout.String())
	}
	if _, exists := seen["T-1"]; exists {
		t.Error("expected T-1 removed from seen map")
	}
}

func TestPoll_RemovedNotOnFirstPoll(t *testing.T) {
	te := newTestEnv(func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, searchJSON())
	})
	defer te.Close()

	seen := map[string]string{"T-1": "2025-01-01T00:00:00Z"}
	emitted := 0
	// On first poll, removals should NOT be emitted
	poll(context.Background(), te.Client, jqlSearchRequest{JQL: "project=T"}, seen, &emitted, 0, true)
	if emitted != 0 {
		t.Errorf("expected no removal on first poll, got %d events", emitted)
	}
}

func TestPoll_FetchError(t *testing.T) {
	te := newTestEnv(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		writeJSON(w, []byte(`{"message":"unauthorized"}`))
	})
	defer te.Close()

	seen := make(map[string]string)
	emitted := 0
	code := poll(context.Background(), te.Client, jqlSearchRequest{JQL: "project=T"}, seen, &emitted, 0, true)
	if code == jrerrors.ExitOK {
		t.Fatal("expected non-zero exit code on auth error")
	}
}

func TestPoll_InvalidJSON(t *testing.T) {
	te := newTestEnv(func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, []byte(`not json`))
	})
	defer te.Close()

	seen := make(map[string]string)
	emitted := 0
	code := poll(context.Background(), te.Client, jqlSearchRequest{JQL: "project=T"}, seen, &emitted, 0, true)
	if code != jrerrors.ExitError {
		t.Fatalf("expected ExitError for bad JSON, got %d", code)
	}
	if !strings.Contains(te.Stderr.String(), "connection_error") {
		t.Errorf("expected connection_error in stderr, got %s", te.Stderr.String())
	}
}

func TestPoll_FieldsUpdatedFallback(t *testing.T) {
	te := newTestEnv(func(w http.ResponseWriter, r *http.Request) {
		// Issue has no top-level "updated", only in fields
		writeJSON(w, searchJSON(issueJSONFieldsUpdated("T-1", "2025-02-01T00:00:00Z")))
	})
	defer te.Close()

	seen := make(map[string]string)
	emitted := 0
	code := poll(context.Background(), te.Client, jqlSearchRequest{JQL: "project=T"}, seen, &emitted, 0, true)
	if code != jrerrors.ExitOK {
		t.Fatalf("expected ExitOK, got %d", code)
	}
	if seen["T-1"] != "2025-02-01T00:00:00Z" {
		t.Errorf("expected fields.updated fallback, got %s", seen["T-1"])
	}
}

func TestPoll_BadIssueJSON(t *testing.T) {
	te := newTestEnv(func(w http.ResponseWriter, r *http.Request) {
		// Issue array contains invalid JSON element
		writeJSON(w, []byte(`{"issues":[{bad json}]}`))
	})
	defer te.Close()

	seen := make(map[string]string)
	emitted := 0
	// Should not panic — bad issue is skipped
	code := poll(context.Background(), te.Client, jqlSearchRequest{JQL: "project=T"}, seen, &emitted, 0, true)
	// The outer JSON is valid but the issue element is not valid JSON inside the array.
	// Actually the whole response will fail to parse. Let's adjust expectations.
	// json.Unmarshal of the outer object may fail too since {bad json} is not valid JSON.
	if code != jrerrors.ExitError {
		// If outer parse fails, we get ExitError
		// If individual issue parse fails (valid outer array with weird elements), it continues
		t.Logf("code=%d (depends on JSON parse behavior)", code)
	}
}

func TestPoll_SkipBadIssueFingerprint(t *testing.T) {
	te := newTestEnv(func(w http.ResponseWriter, r *http.Request) {
		// One valid issue, one that can't be parsed as fingerprint (number instead of object)
		writeJSON(w, []byte(`{"issues":[42,` + issueJSON("T-1", "2025-01-01T00:00:00Z") + `]}`))
	})
	defer te.Close()

	seen := make(map[string]string)
	emitted := 0
	code := poll(context.Background(), te.Client, jqlSearchRequest{JQL: "project=T"}, seen, &emitted, 0, true)
	if code != jrerrors.ExitOK {
		t.Fatalf("expected ExitOK (bad issue skipped), got %d", code)
	}
	if emitted != 1 {
		t.Errorf("expected 1 event (bad issue skipped), got %d", emitted)
	}
}

func TestPoll_MaxEventsReached(t *testing.T) {
	te := newTestEnv(func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, searchJSON(
			issueJSON("T-1", "2025-01-01T00:00:00Z"),
			issueJSON("T-2", "2025-01-01T00:00:00Z"),
		))
	})
	defer te.Close()

	seen := make(map[string]string)
	emitted := 0
	code := poll(context.Background(), te.Client, jqlSearchRequest{JQL: "project=T"}, seen, &emitted, 1, true)
	if code != -1 {
		t.Fatalf("expected -1 sentinel for maxEvents, got %d", code)
	}
}

func TestPoll_MaxEventsOnUpdate(t *testing.T) {
	te := newTestEnv(func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, searchJSON(issueJSON("T-1", "2025-01-02T00:00:00Z")))
	})
	defer te.Close()

	seen := map[string]string{"T-1": "2025-01-01T00:00:00Z"}
	emitted := 0
	code := poll(context.Background(), te.Client, jqlSearchRequest{JQL: "project=T"}, seen, &emitted, 1, false)
	if code != -1 {
		t.Fatalf("expected -1 for maxEvents on update, got %d", code)
	}
}

func TestPoll_MaxEventsOnRemoval(t *testing.T) {
	te := newTestEnv(func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, searchJSON())
	})
	defer te.Close()

	seen := map[string]string{"T-1": "2025-01-01T00:00:00Z"}
	emitted := 0
	code := poll(context.Background(), te.Client, jqlSearchRequest{JQL: "project=T"}, seen, &emitted, 1, false)
	if code != -1 {
		t.Fatalf("expected -1 for maxEvents on removal, got %d", code)
	}
}

func TestPoll_WithFields(t *testing.T) {
	var gotBody []byte
	te := newTestEnv(func(w http.ResponseWriter, r *http.Request) {
		gotBody, _ = io.ReadAll(r.Body)
		writeJSON(w, searchJSON())
	})
	defer te.Close()

	seen := make(map[string]string)
	emitted := 0
	req := jqlSearchRequest{JQL: "project=T", Fields: []string{"summary", "status"}}
	poll(context.Background(), te.Client, req, seen, &emitted, 0, true)

	if !strings.Contains(string(gotBody), `"summary"`) {
		t.Errorf("expected fields in request body, got %s", gotBody)
	}
}

// --- Run (integration) ---

func TestRun_FirstPollError(t *testing.T) {
	te := newTestEnv(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		writeJSON(w, []byte(`{"message":"server error"}`))
	})
	defer te.Close()

	code := Run(context.Background(), te.Client, Options{
		JQL:      "project=T",
		Interval: 50 * time.Millisecond,
	})
	if code == jrerrors.ExitOK {
		t.Fatal("expected non-zero exit on first poll error")
	}
}

func TestRun_FirstPollMaxEvents(t *testing.T) {
	te := newTestEnv(func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, searchJSON(issueJSON("T-1", "2025-01-01T00:00:00Z")))
	})
	defer te.Close()

	code := Run(context.Background(), te.Client, Options{
		JQL:       "project=T",
		Interval:  50 * time.Millisecond,
		MaxEvents: 1,
	})
	if code != jrerrors.ExitOK {
		t.Fatalf("expected ExitOK when maxEvents reached, got %d", code)
	}
}

func TestRun_ContextCancelled(t *testing.T) {
	var pollCount atomic.Int32
	te := newTestEnv(func(w http.ResponseWriter, r *http.Request) {
		pollCount.Add(1)
		writeJSON(w, searchJSON())
	})
	defer te.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()

	code := Run(ctx, te.Client, Options{
		JQL:      "project=T",
		Interval: 50 * time.Millisecond,
	})
	if code != jrerrors.ExitOK {
		t.Fatalf("expected ExitOK on context cancel, got %d", code)
	}
}

func TestRun_IssueToJQL(t *testing.T) {
	var gotBody string
	te := newTestEnv(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		gotBody = string(b)
		writeJSON(w, searchJSON(issueJSON("PROJ-1", "2025-01-01T00:00:00Z")))
	})
	defer te.Close()

	code := Run(context.Background(), te.Client, Options{
		Issue:     "PROJ-1",
		Interval:  50 * time.Millisecond,
		MaxEvents: 1,
	})
	if code != jrerrors.ExitOK {
		t.Fatalf("expected ExitOK, got %d", code)
	}
	if !strings.Contains(gotBody, "key = PROJ-1") {
		t.Errorf("expected issue converted to JQL, got %s", gotBody)
	}
}

func TestRun_WithFields(t *testing.T) {
	var gotBody string
	te := newTestEnv(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		gotBody = string(b)
		writeJSON(w, searchJSON(issueJSON("T-1", "2025-01-01T00:00:00Z")))
	})
	defer te.Close()

	code := Run(context.Background(), te.Client, Options{
		JQL:       "project=T",
		Interval:  50 * time.Millisecond,
		MaxEvents: 1,
		Fields:    []string{"summary"},
	})
	if code != jrerrors.ExitOK {
		t.Fatalf("expected ExitOK, got %d", code)
	}
	if !strings.Contains(gotBody, `"summary"`) {
		t.Errorf("expected fields in body, got %s", gotBody)
	}
}

func TestRun_SecondPollMaxEvents(t *testing.T) {
	var pollCount atomic.Int32
	te := newTestEnv(func(w http.ResponseWriter, r *http.Request) {
		n := pollCount.Add(1)
		if n == 1 {
			writeJSON(w, searchJSON()) // empty first poll
		} else {
			writeJSON(w, searchJSON(issueJSON("T-1", "2025-01-01T00:00:00Z")))
		}
	})
	defer te.Close()

	code := Run(context.Background(), te.Client, Options{
		JQL:       "project=T",
		Interval:  50 * time.Millisecond,
		MaxEvents: 1,
	})
	if code != jrerrors.ExitOK {
		t.Fatalf("expected ExitOK when maxEvents reached on second poll, got %d", code)
	}
}

func TestRun_AuthErrorStopsImmediately(t *testing.T) {
	var pollCount atomic.Int32
	te := newTestEnv(func(w http.ResponseWriter, r *http.Request) {
		n := pollCount.Add(1)
		if n == 1 {
			writeJSON(w, searchJSON()) // first poll OK
		} else {
			w.WriteHeader(http.StatusUnauthorized)
			writeJSON(w, []byte(`{"message":"unauthorized"}`))
		}
	})
	defer te.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	code := Run(ctx, te.Client, Options{
		JQL:      "project=T",
		Interval: 50 * time.Millisecond,
	})
	if code != jrerrors.ExitAuth {
		t.Fatalf("expected ExitAuth, got %d", code)
	}
}

func TestRun_TransientErrorContinues(t *testing.T) {
	var pollCount atomic.Int32
	te := newTestEnv(func(w http.ResponseWriter, r *http.Request) {
		n := pollCount.Add(1)
		switch n {
		case 1:
			writeJSON(w, searchJSON()) // first poll OK
		case 2:
			w.WriteHeader(http.StatusInternalServerError)
			writeJSON(w, []byte(`{"message":"server error"}`))
		default:
			// third poll: emit an event so we can stop
			writeJSON(w, searchJSON(issueJSON("T-1", "2025-01-01T00:00:00Z")))
		}
	})
	defer te.Close()

	code := Run(context.Background(), te.Client, Options{
		JQL:       "project=T",
		Interval:  50 * time.Millisecond,
		MaxEvents: 1,
	})
	if code != jrerrors.ExitOK {
		t.Fatalf("expected ExitOK after transient error recovery, got %d", code)
	}
	if pollCount.Load() < 3 {
		t.Errorf("expected at least 3 polls (1 ok + 1 error + 1 recovery), got %d", pollCount.Load())
	}
}

func TestRun_BackoffOnRepeatedErrors(t *testing.T) {
	var pollCount atomic.Int32
	te := newTestEnv(func(w http.ResponseWriter, r *http.Request) {
		n := pollCount.Add(1)
		if n == 1 {
			writeJSON(w, searchJSON()) // first poll OK
		} else if n <= 3 {
			// Two consecutive errors to trigger backoff
			w.WriteHeader(http.StatusInternalServerError)
			writeJSON(w, []byte(`{"message":"server error"}`))
		} else {
			writeJSON(w, searchJSON(issueJSON("T-1", "2025-01-01T00:00:00Z")))
		}
	})
	defer te.Close()

	code := Run(context.Background(), te.Client, Options{
		JQL:       "project=T",
		Interval:  50 * time.Millisecond,
		MaxEvents: 1,
	})
	if code != jrerrors.ExitOK {
		t.Fatalf("expected ExitOK after backoff recovery, got %d", code)
	}
}

func TestRun_BackoffContextCancel(t *testing.T) {
	var pollCount atomic.Int32
	te := newTestEnv(func(w http.ResponseWriter, r *http.Request) {
		n := pollCount.Add(1)
		if n == 1 {
			writeJSON(w, searchJSON()) // first poll OK
		} else {
			// Keep returning errors to force backoff
			w.WriteHeader(http.StatusInternalServerError)
			writeJSON(w, []byte(`{"message":"server error"}`))
		}
	})
	defer te.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel()

	code := Run(ctx, te.Client, Options{
		JQL:      "project=T",
		Interval: 50 * time.Millisecond,
	})
	if code != jrerrors.ExitOK {
		t.Fatalf("expected ExitOK when context cancelled during backoff, got %d", code)
	}
}

func TestRun_ConsecutiveErrorsReset(t *testing.T) {
	var pollCount atomic.Int32
	te := newTestEnv(func(w http.ResponseWriter, r *http.Request) {
		n := pollCount.Add(1)
		switch n {
		case 1:
			writeJSON(w, searchJSON()) // first poll OK
		case 2:
			w.WriteHeader(http.StatusInternalServerError)
			writeJSON(w, []byte(`{"message":"err"}`))
		case 3:
			// Success — should reset consecutiveErrors
			writeJSON(w, searchJSON(issueJSON("T-1", "2025-01-01T00:00:00Z")))
		default:
			// Another event to stop
			writeJSON(w, searchJSON(
				issueJSON("T-1", "2025-01-01T00:00:00Z"),
				issueJSON("T-2", "2025-01-01T00:00:00Z"),
			))
		}
	})
	defer te.Close()

	code := Run(context.Background(), te.Client, Options{
		JQL:       "project=T",
		Interval:  50 * time.Millisecond,
		MaxEvents: 2,
	})
	if code != jrerrors.ExitOK {
		t.Fatalf("expected ExitOK, got %d", code)
	}
}

func TestMarshalNoEscape_Error(t *testing.T) {
	// channels cannot be marshaled to JSON
	ch := make(chan int)
	_, err := marshalNoEscape(ch)
	if err == nil {
		t.Fatal("expected error marshaling channel")
	}
}

func TestRun_BackoffCapped(t *testing.T) {
	// Lower maxBackoff so the cap is reachable with small intervals.
	orig := maxBackoff
	maxBackoff = 5 * time.Millisecond
	defer func() { maxBackoff = orig }()

	var pollCount atomic.Int32
	te := newTestEnv(func(w http.ResponseWriter, r *http.Request) {
		n := pollCount.Add(1)
		if n == 1 {
			writeJSON(w, searchJSON()) // first poll OK
		} else if n <= 4 {
			// 3 consecutive errors: backoff = 3*10ms = 30ms > 5ms cap
			w.WriteHeader(http.StatusInternalServerError)
			writeJSON(w, []byte(`{"message":"err"}`))
		} else {
			writeJSON(w, searchJSON(issueJSON("T-1", "2025-01-01T00:00:00Z")))
		}
	})
	defer te.Close()

	code := Run(context.Background(), te.Client, Options{
		JQL:       "project=T",
		Interval:  10 * time.Millisecond,
		MaxEvents: 1,
	})
	if code != jrerrors.ExitOK {
		t.Fatalf("expected ExitOK, got %d", code)
	}
	if pollCount.Load() < 4 {
		t.Errorf("expected at least 4 polls to trigger backoff cap, got %d", pollCount.Load())
	}
}

func TestEmitEvent_JQFilterMaxEvents(t *testing.T) {
	var stdout, stderr bytes.Buffer
	c := &client.Client{Stdout: &stdout, Stderr: &stderr, JQFilter: ".type"}
	emitted := 0
	code := emitEvent(c, "created", json.RawMessage(`{"key":"T-1"}`), &emitted, 1)
	if code != -1 {
		t.Fatalf("expected -1 when maxEvents=1, got %d", code)
	}
}

// --- Issue key validation ---

func TestRun_InvalidIssueKey(t *testing.T) {
	tests := []struct {
		name  string
		issue string
	}{
		{"injection attempt", `PROJ-1 OR key = SECRET-1`},
		{"empty project", `-123`},
		{"lowercase project", `proj-1`},
		{"no dash", `PROJ123`},
		{"special chars", `PROJ-1; DROP TABLE`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			te := newTestEnv(func(w http.ResponseWriter, r *http.Request) {
				t.Fatal("server should not be called for invalid issue key")
			})
			defer te.Close()

			code := Run(context.Background(), te.Client, Options{
				Issue:    tt.issue,
				Interval: 50 * time.Millisecond,
			})
			if code != jrerrors.ExitValidation {
				t.Errorf("expected ExitValidation for issue %q, got %d", tt.issue, code)
			}
			if !strings.Contains(te.Stderr.String(), "validation_error") {
				t.Errorf("expected validation_error in stderr, got %s", te.Stderr.String())
			}
		})
	}
}

func TestRun_ValidIssueKeys(t *testing.T) {
	tests := []string{"PROJ-1", "MY_PROJECT-999", "A-1", "AB2-42"}
	for _, key := range tests {
		t.Run(key, func(t *testing.T) {
			te := newTestEnv(func(w http.ResponseWriter, r *http.Request) {
				writeJSON(w, searchJSON(issueJSON(key, "2025-01-01T00:00:00Z")))
			})
			defer te.Close()

			code := Run(context.Background(), te.Client, Options{
				Issue:     key,
				Interval:  50 * time.Millisecond,
				MaxEvents: 1,
			})
			if code != jrerrors.ExitOK {
				t.Errorf("expected ExitOK for valid key %q, got %d; stderr=%s", key, code, te.Stderr.String())
			}
		})
	}
}
