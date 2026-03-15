package client_test

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
	"github.com/sofq/jira-cli/internal/config"
	"github.com/spf13/cobra"
)

// newTestClient creates a Client wired to the given test server URL and captures stdout/stderr.
func newTestClient(serverURL string, stdout, stderr io.Writer) *client.Client {
	return &client.Client{
		BaseURL:    serverURL,
		Auth:       config.AuthConfig{},
		HTTPClient: &http.Client{},
		Stdout:     stdout,
		Stderr:     stderr,
	}
}

// Test 1: Do GET returns JSON to stdout.
func TestDo_GetReturnsJSON(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintln(w, `{"key":"value"}`)
	}))
	defer ts.Close()

	var stdout, stderr bytes.Buffer
	c := newTestClient(ts.URL, &stdout, &stderr)

	code := c.Do(context.Background(), "GET", "/rest/api/3/test", nil, nil)
	if code != 0 {
		t.Fatalf("expected exit code 0, got %d; stderr=%s", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), `"key"`) {
		t.Errorf("expected JSON in stdout, got: %s", stdout.String())
	}
}

// Test 2: Basic auth header is set correctly.
func TestDo_BasicAuth(t *testing.T) {
	var gotAuth string
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintln(w, `{}`)
	}))
	defer ts.Close()

	var stdout, stderr bytes.Buffer
	c := newTestClient(ts.URL, &stdout, &stderr)
	c.Auth = config.AuthConfig{Type: "basic", Username: "user", Token: "secret"}

	code := c.Do(context.Background(), "GET", "/path", nil, nil)
	if code != 0 {
		t.Fatalf("unexpected exit code %d; stderr=%s", code, stderr.String())
	}
	if !strings.HasPrefix(gotAuth, "Basic ") {
		t.Errorf("expected Basic auth header, got: %q", gotAuth)
	}
}

// Test 3: Bearer auth header is set correctly.
func TestDo_BearerAuth(t *testing.T) {
	var gotAuth string
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintln(w, `{}`)
	}))
	defer ts.Close()

	var stdout, stderr bytes.Buffer
	c := newTestClient(ts.URL, &stdout, &stderr)
	c.Auth = config.AuthConfig{Type: "bearer", Token: "mytoken"}

	code := c.Do(context.Background(), "GET", "/path", nil, nil)
	if code != 0 {
		t.Fatalf("unexpected exit code %d; stderr=%s", code, stderr.String())
	}
	if gotAuth != "Bearer mytoken" {
		t.Errorf("expected 'Bearer mytoken', got: %q", gotAuth)
	}
}

// Test 4: HTTP 404 → structured JSON error on stderr, exit code 3.
func TestDo_404ReturnsStructuredError(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "not found", http.StatusNotFound)
	}))
	defer ts.Close()

	var stdout, stderr bytes.Buffer
	c := newTestClient(ts.URL, &stdout, &stderr)

	code := c.Do(context.Background(), "GET", "/missing", nil, nil)
	if code != 3 {
		t.Fatalf("expected exit code 3 (not found), got %d", code)
	}
	var errObj map[string]interface{}
	if err := json.Unmarshal(stderr.Bytes(), &errObj); err != nil {
		t.Fatalf("stderr is not valid JSON: %s; err=%v", stderr.String(), err)
	}
	if errObj["error_type"] != "not_found" {
		t.Errorf("expected error_type 'not_found', got: %v", errObj["error_type"])
	}
	if int(errObj["status"].(float64)) != 404 {
		t.Errorf("expected status 404, got: %v", errObj["status"])
	}
}

// Test 5: JQ filter applied to response.
func TestDo_JQFilter(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintln(w, `{"name":"jira","version":3}`)
	}))
	defer ts.Close()

	var stdout, stderr bytes.Buffer
	c := newTestClient(ts.URL, &stdout, &stderr)
	c.JQFilter = ".name"

	code := c.Do(context.Background(), "GET", "/path", nil, nil)
	if code != 0 {
		t.Fatalf("unexpected exit code %d; stderr=%s", code, stderr.String())
	}
	out := strings.TrimSpace(stdout.String())
	if out != `"jira"` {
		t.Errorf("expected jq result %q, got %q", `"jira"`, out)
	}
}

// Test 6: Pagination — 2 pages merged.
func TestDo_Pagination(t *testing.T) {
	callCount := 0
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		callCount++
		startAt := r.URL.Query().Get("startAt")
		if startAt == "" || startAt == "0" {
			fmt.Fprintln(w, `{"startAt":0,"maxResults":2,"total":4,"values":[{"id":1},{"id":2}]}`)
		} else {
			fmt.Fprintln(w, `{"startAt":2,"maxResults":2,"total":4,"values":[{"id":3},{"id":4}]}`)
		}
	}))
	defer ts.Close()

	var stdout, stderr bytes.Buffer
	c := newTestClient(ts.URL, &stdout, &stderr)
	c.Paginate = true

	code := c.Do(context.Background(), "GET", "/path", nil, nil)
	if code != 0 {
		t.Fatalf("unexpected exit code %d; stderr=%s", code, stderr.String())
	}
	if callCount != 2 {
		t.Errorf("expected 2 HTTP calls for pagination, got %d", callCount)
	}

	var result []map[string]interface{}
	if err := json.Unmarshal(stdout.Bytes(), &result); err != nil {
		t.Fatalf("stdout is not valid JSON array: %s; err=%v", stdout.String(), err)
	}
	if len(result) != 4 {
		t.Errorf("expected 4 merged items, got %d", len(result))
	}
}

// Test 7: DryRun outputs request JSON to stdout.
func TestDo_DryRun(t *testing.T) {
	var stdout, stderr bytes.Buffer
	c := &client.Client{
		BaseURL:    "https://example.atlassian.net",
		HTTPClient: &http.Client{},
		Stdout:     &stdout,
		Stderr:     &stderr,
		DryRun:     true,
	}

	code := c.Do(context.Background(), "GET", "/rest/api/3/issue/TEST-1", nil, nil)
	if code != 0 {
		t.Fatalf("expected exit code 0 for dry run, got %d; stderr=%s", code, stderr.String())
	}
	var obj map[string]string
	if err := json.Unmarshal(stdout.Bytes(), &obj); err != nil {
		t.Fatalf("stdout is not valid JSON: %s; err=%v", stdout.String(), err)
	}
	if obj["method"] != "GET" {
		t.Errorf("expected method GET, got %q", obj["method"])
	}
	if !strings.Contains(obj["url"], "/rest/api/3/issue/TEST-1") {
		t.Errorf("expected url to contain path, got %q", obj["url"])
	}
}

// Test 8: POST with body sends correct content-type and body.
func TestDo_PostWithBody(t *testing.T) {
	var gotContentType, gotBody string
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotContentType = r.Header.Get("Content-Type")
		b, _ := io.ReadAll(r.Body)
		gotBody = string(b)
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintln(w, `{"id":"NEW-1"}`)
	}))
	defer ts.Close()

	var stdout, stderr bytes.Buffer
	c := newTestClient(ts.URL, &stdout, &stderr)

	body := strings.NewReader(`{"summary":"test issue"}`)
	code := c.Do(context.Background(), "POST", "/rest/api/3/issue", nil, body)
	if code != 0 {
		t.Fatalf("unexpected exit code %d; stderr=%s", code, stderr.String())
	}
	if gotContentType != "application/json" {
		t.Errorf("expected Content-Type application/json, got %q", gotContentType)
	}
	if !strings.Contains(gotBody, "summary") {
		t.Errorf("expected body to contain 'summary', got %q", gotBody)
	}
}

// Test 9: QueryFromFlags only includes changed flags.
func TestQueryFromFlags(t *testing.T) {
	cmd := &cobra.Command{}
	cmd.Flags().String("project", "", "project key")
	cmd.Flags().Int("max-results", 50, "max results")
	cmd.Flags().String("status", "", "status filter")

	// Simulate only "project" being set by the user.
	if err := cmd.Flags().Set("project", "MYPROJ"); err != nil {
		t.Fatal(err)
	}

	q := client.QueryFromFlags(cmd, "project", "max-results", "status")
	if q.Get("project") != "MYPROJ" {
		t.Errorf("expected project=MYPROJ, got %q", q.Get("project"))
	}
	if q.Has("max-results") {
		t.Error("max-results should NOT be in query (not changed by user)")
	}
	if q.Has("status") {
		t.Error("status should NOT be in query (not changed by user)")
	}
}

// Test 10: FromContext and NewContext roundtrip.
func TestContextRoundtrip(t *testing.T) {
	c := &client.Client{BaseURL: "https://test.atlassian.net"}
	ctx := client.NewContext(context.Background(), c)

	got, err := client.FromContext(ctx)
	if err != nil {
		t.Fatalf("FromContext returned error: %v", err)
	}
	if got != c {
		t.Error("FromContext returned a different client pointer")
	}
}

// Test 10b: FromContext on empty context returns error.
func TestFromContext_Missing(t *testing.T) {
	_, err := client.FromContext(context.Background())
	if err == nil {
		t.Error("expected error when client is not in context, got nil")
	}
}
