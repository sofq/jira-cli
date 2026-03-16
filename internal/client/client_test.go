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
	"time"

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

	var envelope struct {
		StartAt int                      `json:"startAt"`
		Total   int                      `json:"total"`
		IsLast  bool                     `json:"isLast"`
		Values  []map[string]interface{} `json:"values"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &envelope); err != nil {
		t.Fatalf("stdout is not valid JSON: %s; err=%v", stdout.String(), err)
	}
	if len(envelope.Values) != 4 {
		t.Errorf("expected 4 merged items, got %d", len(envelope.Values))
	}
	if envelope.Total != 4 {
		t.Errorf("expected total=4, got %d", envelope.Total)
	}
	if !envelope.IsLast {
		t.Error("expected isLast=true after merging all pages")
	}
}

// Test 6b: Pagination passthrough — non-paginated GET responses are returned as-is.
func TestDo_Pagination_NonPaginatedPassthrough(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintln(w, `{"displayName":"Agent","active":true}`)
	}))
	defer ts.Close()

	var stdout, stderr bytes.Buffer
	c := newTestClient(ts.URL, &stdout, &stderr)
	c.Paginate = true

	code := c.Do(context.Background(), "GET", "/rest/api/3/myself", nil, nil)
	if code != 0 {
		t.Fatalf("unexpected exit code %d; stderr=%s", code, stderr.String())
	}

	var result map[string]interface{}
	if err := json.Unmarshal(stdout.Bytes(), &result); err != nil {
		t.Fatalf("stdout is not valid JSON: %s; err=%v", stdout.String(), err)
	}
	if result["displayName"] != "Agent" {
		t.Errorf("expected displayName=Agent, got %v", result["displayName"])
	}
}

// Test 6c: Pagination + jq filter on envelope — .values[].id works on paginated response.
func TestDo_Pagination_JQFilter(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintln(w, `{"startAt":0,"maxResults":2,"total":2,"values":[{"id":1},{"id":2}]}`)
	}))
	defer ts.Close()

	var stdout, stderr bytes.Buffer
	c := newTestClient(ts.URL, &stdout, &stderr)
	c.Paginate = true
	c.JQFilter = "[.values[].id]"

	code := c.Do(context.Background(), "GET", "/path", nil, nil)
	if code != 0 {
		t.Fatalf("unexpected exit code %d; stderr=%s", code, stderr.String())
	}
	out := strings.TrimSpace(stdout.String())
	if out != "[1,2]" {
		t.Errorf("expected [1,2], got %q", out)
	}
}

// Test 6d: Pagination with isLast field instead of total.
func TestDo_Pagination_IsLast(t *testing.T) {
	callCount := 0
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		callCount++
		startAt := r.URL.Query().Get("startAt")
		if startAt == "" || startAt == "0" {
			fmt.Fprintln(w, `{"startAt":0,"maxResults":2,"isLast":false,"values":[{"id":1},{"id":2}]}`)
		} else {
			fmt.Fprintln(w, `{"startAt":2,"maxResults":2,"isLast":true,"values":[{"id":3}]}`)
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
		t.Errorf("expected 2 HTTP calls, got %d", callCount)
	}

	var envelope struct {
		Values []map[string]interface{} `json:"values"`
		IsLast bool                     `json:"isLast"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &envelope); err != nil {
		t.Fatalf("stdout is not valid JSON: %s; err=%v", stdout.String(), err)
	}
	if len(envelope.Values) != 3 {
		t.Errorf("expected 3 merged items, got %d", len(envelope.Values))
	}
	if !envelope.IsLast {
		t.Error("expected isLast=true after merging all pages")
	}
}

// Test 6e: Single-page paginated response (no extra requests).
func TestDo_Pagination_SinglePage(t *testing.T) {
	callCount := 0
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		callCount++
		fmt.Fprintln(w, `{"startAt":0,"maxResults":50,"total":2,"values":[{"id":1},{"id":2}]}`)
	}))
	defer ts.Close()

	var stdout, stderr bytes.Buffer
	c := newTestClient(ts.URL, &stdout, &stderr)
	c.Paginate = true

	code := c.Do(context.Background(), "GET", "/path", nil, nil)
	if code != 0 {
		t.Fatalf("unexpected exit code %d; stderr=%s", code, stderr.String())
	}
	if callCount != 1 {
		t.Errorf("expected 1 HTTP call for single page, got %d", callCount)
	}

	var envelope struct {
		Values []map[string]interface{} `json:"values"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &envelope); err != nil {
		t.Fatalf("stdout is not valid JSON: %s; err=%v", stdout.String(), err)
	}
	if len(envelope.Values) != 2 {
		t.Errorf("expected 2 items, got %d", len(envelope.Values))
	}
}

// Test 6f: Pagination with HTTP error on second page.
func TestDo_Pagination_ErrorOnPage2(t *testing.T) {
	callCount := 0
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		callCount++
		if callCount == 1 {
			fmt.Fprintln(w, `{"startAt":0,"maxResults":2,"total":4,"values":[{"id":1},{"id":2}]}`)
		} else {
			w.WriteHeader(http.StatusInternalServerError)
			fmt.Fprintln(w, `{"errorMessages":["internal error"]}`)
		}
	}))
	defer ts.Close()

	var stdout, stderr bytes.Buffer
	c := newTestClient(ts.URL, &stdout, &stderr)
	c.Paginate = true

	code := c.Do(context.Background(), "GET", "/path", nil, nil)
	if code == 0 {
		t.Fatal("expected non-zero exit code on page 2 error")
	}

	var errObj map[string]interface{}
	if err := json.Unmarshal(stderr.Bytes(), &errObj); err != nil {
		t.Fatalf("stderr is not valid JSON: %s; err=%v", stderr.String(), err)
	}
	if errObj["error_type"] != "server_error" {
		t.Errorf("expected error_type 'server_error', got: %v", errObj["error_type"])
	}
}

// Test 6g: jq filter on non-paginated response with pagination enabled.
func TestDo_Pagination_JQOnNonPaginated(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintln(w, `{"displayName":"Agent","emailAddress":"agent@test.com"}`)
	}))
	defer ts.Close()

	var stdout, stderr bytes.Buffer
	c := newTestClient(ts.URL, &stdout, &stderr)
	c.Paginate = true
	c.JQFilter = ".displayName"

	code := c.Do(context.Background(), "GET", "/path", nil, nil)
	if code != 0 {
		t.Fatalf("unexpected exit code %d; stderr=%s", code, stderr.String())
	}
	out := strings.TrimSpace(stdout.String())
	if out != `"Agent"` {
		t.Errorf("expected %q, got %q", `"Agent"`, out)
	}
}

// Test 6h: Pagination with pretty print preserves envelope.
func TestDo_Pagination_Pretty(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintln(w, `{"startAt":0,"maxResults":50,"total":1,"values":[{"id":1}]}`)
	}))
	defer ts.Close()

	var stdout, stderr bytes.Buffer
	c := newTestClient(ts.URL, &stdout, &stderr)
	c.Paginate = true
	c.Pretty = true

	code := c.Do(context.Background(), "GET", "/path", nil, nil)
	if code != 0 {
		t.Fatalf("unexpected exit code %d; stderr=%s", code, stderr.String())
	}

	out := stdout.String()
	// Pretty output should have newlines and indentation.
	if !strings.Contains(out, "\n") {
		t.Error("expected pretty-printed output with newlines")
	}
	// Should still be valid JSON with envelope.
	var envelope struct {
		Values []map[string]interface{} `json:"values"`
	}
	if err := json.Unmarshal([]byte(out), &envelope); err != nil {
		t.Fatalf("pretty output is not valid JSON: err=%v", err)
	}
	if len(envelope.Values) != 1 {
		t.Errorf("expected 1 value, got %d", len(envelope.Values))
	}
}

// Test 6i: Pagination with empty values array (0 results).
func TestDo_Pagination_EmptyValues(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintln(w, `{"startAt":0,"maxResults":50,"total":0,"values":[]}`)
	}))
	defer ts.Close()

	var stdout, stderr bytes.Buffer
	c := newTestClient(ts.URL, &stdout, &stderr)
	c.Paginate = true

	code := c.Do(context.Background(), "GET", "/path", nil, nil)
	if code != 0 {
		t.Fatalf("unexpected exit code %d; stderr=%s", code, stderr.String())
	}

	var envelope struct {
		Values []interface{} `json:"values"`
		Total  int           `json:"total"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &envelope); err != nil {
		t.Fatalf("stdout is not valid JSON: %s; err=%v", stdout.String(), err)
	}
	if len(envelope.Values) != 0 {
		t.Errorf("expected 0 values, got %d", len(envelope.Values))
	}
}

// Test 6j: Pagination disabled — paginated response returned as-is.
func TestDo_NoPaginate_ReturnsRawResponse(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintln(w, `{"startAt":0,"maxResults":2,"total":4,"values":[{"id":1},{"id":2}]}`)
	}))
	defer ts.Close()

	var stdout, stderr bytes.Buffer
	c := newTestClient(ts.URL, &stdout, &stderr)
	c.Paginate = false

	code := c.Do(context.Background(), "GET", "/path", nil, nil)
	if code != 0 {
		t.Fatalf("unexpected exit code %d; stderr=%s", code, stderr.String())
	}

	var envelope struct {
		Total      int `json:"total"`
		MaxResults int `json:"maxResults"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &envelope); err != nil {
		t.Fatalf("stdout is not valid JSON: %s; err=%v", stdout.String(), err)
	}
	if envelope.Total != 4 {
		t.Errorf("expected total=4 in raw response, got %d", envelope.Total)
	}
	if envelope.MaxResults != 2 {
		t.Errorf("expected maxResults=2, got %d", envelope.MaxResults)
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

// Test 11: HTTP 429 with Retry-After header populates retry_after in error JSON.
func TestDo_429RetryAfter(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Retry-After", "60")
		w.WriteHeader(429)
		fmt.Fprintln(w, `{"message":"rate limited"}`)
	}))
	defer ts.Close()

	var stdout, stderr bytes.Buffer
	c := newTestClient(ts.URL, &stdout, &stderr)

	code := c.Do(context.Background(), "GET", "/path", nil, nil)
	if code != 5 {
		t.Fatalf("expected exit code 5 (rate limit), got %d", code)
	}

	var errObj map[string]interface{}
	if err := json.Unmarshal(stderr.Bytes(), &errObj); err != nil {
		t.Fatalf("stderr not valid JSON: %s", stderr.String())
	}
	if errObj["retry_after"] != float64(60) {
		t.Errorf("expected retry_after=60, got %v", errObj["retry_after"])
	}
}

// Test 12: Verbose output is structured JSON.
func TestDo_VerboseStructuredJSON(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintln(w, `{"ok":true}`)
	}))
	defer ts.Close()

	var stdout, stderr bytes.Buffer
	c := newTestClient(ts.URL, &stdout, &stderr)
	c.Verbose = true

	code := c.Do(context.Background(), "GET", "/rest/api/3/test", nil, nil)
	if code != 0 {
		t.Fatalf("expected exit code 0, got %d", code)
	}

	lines := strings.Split(strings.TrimSpace(stderr.String()), "\n")
	if len(lines) < 2 {
		t.Fatalf("expected at least 2 lines in stderr, got %d: %s", len(lines), stderr.String())
	}

	var reqLog map[string]interface{}
	if err := json.Unmarshal([]byte(lines[0]), &reqLog); err != nil {
		t.Fatalf("request log line not valid JSON: %s", lines[0])
	}
	if reqLog["type"] != "request" {
		t.Errorf("expected type=request, got %v", reqLog["type"])
	}

	var respLog map[string]interface{}
	if err := json.Unmarshal([]byte(lines[1]), &respLog); err != nil {
		t.Fatalf("response log line not valid JSON: %s", lines[1])
	}
	if respLog["type"] != "response" {
		t.Errorf("expected type=response, got %v", respLog["type"])
	}
}

// Test 12b: OAuth2 client credentials flow.
func TestDo_OAuth2Auth(t *testing.T) {
	tokenServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintln(w, `{"access_token":"oauth-token-123","token_type":"Bearer","expires_in":3600}`)
	}))
	defer tokenServer.Close()

	var gotAuth string
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintln(w, `{}`)
	}))
	defer ts.Close()

	var stdout, stderr bytes.Buffer
	c := newTestClient(ts.URL, &stdout, &stderr)
	c.Auth = config.AuthConfig{
		Type:         "oauth2",
		ClientID:     "my-client-id",
		ClientSecret: "my-client-secret",
		TokenURL:     tokenServer.URL,
	}

	code := c.Do(context.Background(), "GET", "/path", nil, nil)
	if code != 0 {
		t.Fatalf("unexpected exit code %d; stderr=%s", code, stderr.String())
	}
	if gotAuth != "Bearer oauth-token-123" {
		t.Errorf("expected 'Bearer oauth-token-123', got: %q", gotAuth)
	}
}

// Test 13: --fields adds fields query param to GET requests.
func TestDo_FieldsQueryParam(t *testing.T) {
	var gotFields string
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotFields = r.URL.Query().Get("fields")
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintln(w, `{"key":"TEST-1"}`)
	}))
	defer ts.Close()

	var stdout, stderr bytes.Buffer
	c := newTestClient(ts.URL, &stdout, &stderr)
	c.Fields = "key,summary,status"

	code := c.Do(context.Background(), "GET", "/rest/api/3/issue/TEST-1", nil, nil)
	if code != 0 {
		t.Fatalf("expected exit code 0, got %d; stderr=%s", code, stderr.String())
	}
	if gotFields != "key,summary,status" {
		t.Errorf("expected fields=key,summary,status, got %q", gotFields)
	}
}

// Test 13b: --fields not applied to POST requests.
func TestDo_FieldsNotAppliedToPost(t *testing.T) {
	var gotQuery string
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotQuery = r.URL.RawQuery
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintln(w, `{"id":"1"}`)
	}))
	defer ts.Close()

	var stdout, stderr bytes.Buffer
	c := newTestClient(ts.URL, &stdout, &stderr)
	c.Fields = "key,summary"

	code := c.Do(context.Background(), "POST", "/rest/api/3/issue", nil, strings.NewReader(`{}`))
	if code != 0 {
		t.Fatalf("expected exit code 0, got %d", code)
	}
	if strings.Contains(gotQuery, "fields=") {
		t.Errorf("fields should not be applied to POST, got query: %s", gotQuery)
	}
}

// Test 14: Cache hit skips server call.
func TestDo_CacheHit(t *testing.T) {
	callCount := 0
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintln(w, `{"data":"fresh"}`)
	}))
	defer ts.Close()

	var stdout1, stderr1 bytes.Buffer
	c := newTestClient(ts.URL, &stdout1, &stderr1)
	c.CacheTTL = 5 * time.Minute

	code := c.Do(context.Background(), "GET", "/rest/api/3/cached-test", nil, nil)
	if code != 0 {
		t.Fatalf("expected exit code 0, got %d", code)
	}
	if callCount != 1 {
		t.Fatalf("expected 1 server call, got %d", callCount)
	}

	var stdout2, stderr2 bytes.Buffer
	c.Stdout = &stdout2
	c.Stderr = &stderr2
	code = c.Do(context.Background(), "GET", "/rest/api/3/cached-test", nil, nil)
	if code != 0 {
		t.Fatalf("expected exit code 0, got %d", code)
	}
	if callCount != 1 {
		t.Errorf("expected still 1 server call (cache hit), got %d", callCount)
	}
}

// Bug #4: HTTP 204 No Content returns {} instead of empty body.
func TestDo_204ReturnsEmptyJSON(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	defer ts.Close()

	var stdout, stderr bytes.Buffer
	c := newTestClient(ts.URL, &stdout, &stderr)

	code := c.Do(context.Background(), "PUT", "/rest/api/3/issue/TEST-1", nil, strings.NewReader(`{}`))
	if code != 0 {
		t.Fatalf("expected exit code 0, got %d; stderr=%s", code, stderr.String())
	}

	out := strings.TrimSpace(stdout.String())
	if out != "{}" {
		t.Errorf("expected '{}' for 204 response, got %q", out)
	}
}

// Bug #5: --jq on 204 response should work (operates on {}).
func TestDo_204WithJQFilter(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	defer ts.Close()

	var stdout, stderr bytes.Buffer
	c := newTestClient(ts.URL, &stdout, &stderr)
	c.JQFilter = `.status // "ok"`

	code := c.Do(context.Background(), "PUT", "/path", nil, strings.NewReader(`{}`))
	if code != 0 {
		t.Fatalf("expected exit code 0, got %d; stderr=%s", code, stderr.String())
	}

	out := strings.TrimSpace(stdout.String())
	if out != `"ok"` {
		t.Errorf("expected jq result %q, got %q", `"ok"`, out)
	}
}

// Bug #4: Empty body (not 204 status) also returns {}.
func TestDo_EmptyBodyReturnsEmptyJSON(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		// Write nothing.
	}))
	defer ts.Close()

	var stdout, stderr bytes.Buffer
	c := newTestClient(ts.URL, &stdout, &stderr)

	code := c.Do(context.Background(), "DELETE", "/path", nil, nil)
	if code != 0 {
		t.Fatalf("expected exit code 0, got %d; stderr=%s", code, stderr.String())
	}

	out := strings.TrimSpace(stdout.String())
	if out != "{}" {
		t.Errorf("expected '{}' for empty response, got %q", out)
	}
}

// Bug #8: Cache works with pagination enabled.
func TestDo_CacheWithPagination(t *testing.T) {
	callCount := 0
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintln(w, `{"startAt":0,"maxResults":50,"total":1,"values":[{"id":1}]}`)
	}))
	defer ts.Close()

	// First call: should hit server.
	var stdout1, stderr1 bytes.Buffer
	c := newTestClient(ts.URL, &stdout1, &stderr1)
	c.Paginate = true
	c.CacheTTL = 5 * time.Minute

	code := c.Do(context.Background(), "GET", "/rest/api/3/cache-page-test", nil, nil)
	if code != 0 {
		t.Fatalf("expected exit code 0, got %d", code)
	}
	if callCount != 1 {
		t.Fatalf("expected 1 server call, got %d", callCount)
	}

	// Second call: should be cached.
	var stdout2, stderr2 bytes.Buffer
	c.Stdout = &stdout2
	c.Stderr = &stderr2
	code = c.Do(context.Background(), "GET", "/rest/api/3/cache-page-test", nil, nil)
	if code != 0 {
		t.Fatalf("expected exit code 0, got %d", code)
	}
	if callCount != 1 {
		t.Errorf("expected still 1 server call (cache hit), got %d", callCount)
	}

	// Verify both calls return same data.
	if stdout1.String() != stdout2.String() {
		t.Errorf("cached output differs:\n  first:  %s\n  second: %s", stdout1.String(), stdout2.String())
	}
}

// Bug #11: Paginated output should not HTML-escape characters like &.
func TestDo_PaginationNoHTMLEscape(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintln(w, `{"startAt":0,"maxResults":50,"total":1,"values":[{"url":"http://example.com/a?b=1&c=2"}]}`)
	}))
	defer ts.Close()

	var stdout, stderr bytes.Buffer
	c := newTestClient(ts.URL, &stdout, &stderr)
	c.Paginate = true

	code := c.Do(context.Background(), "GET", "/path", nil, nil)
	if code != 0 {
		t.Fatalf("expected exit code 0, got %d; stderr=%s", code, stderr.String())
	}

	out := stdout.String()
	if strings.Contains(out, `\u0026`) {
		t.Errorf("paginated output should not HTML-escape '&' as '\\u0026':\n%s", out)
	}
	if !strings.Contains(out, `&c=2`) {
		t.Errorf("expected literal '&' in output:\n%s", out)
	}
}

// Test: DELETE request returns JSON response on stdout.
func TestDo_DeleteReturnsJSON(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodDelete {
			t.Errorf("expected DELETE method, got %s", r.Method)
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintln(w, `{"deleted":true,"id":"PROJ-42"}`)
	}))
	defer ts.Close()

	var stdout, stderr bytes.Buffer
	c := newTestClient(ts.URL, &stdout, &stderr)

	code := c.Do(context.Background(), "DELETE", "/rest/api/3/issue/PROJ-42", nil, nil)
	if code != 0 {
		t.Fatalf("expected exit code 0, got %d; stderr=%s", code, stderr.String())
	}

	var result map[string]interface{}
	if err := json.Unmarshal(stdout.Bytes(), &result); err != nil {
		t.Fatalf("stdout is not valid JSON: %s; err=%v", stdout.String(), err)
	}
	if result["deleted"] != true {
		t.Errorf("expected deleted=true, got %v", result["deleted"])
	}
	if result["id"] != "PROJ-42" {
		t.Errorf("expected id=PROJ-42, got %v", result["id"])
	}
}

// Test: PATCH with body sends correct content-type header.
func TestDo_PatchWithBody(t *testing.T) {
	var gotContentType, gotMethod string
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		gotContentType = r.Header.Get("Content-Type")
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintln(w, `{"updated":true}`)
	}))
	defer ts.Close()

	var stdout, stderr bytes.Buffer
	c := newTestClient(ts.URL, &stdout, &stderr)

	body := strings.NewReader(`{"fields":{"summary":"Updated summary"}}`)
	code := c.Do(context.Background(), "PATCH", "/rest/api/3/issue/PROJ-1", nil, body)
	if code != 0 {
		t.Fatalf("expected exit code 0, got %d; stderr=%s", code, stderr.String())
	}
	if gotMethod != "PATCH" {
		t.Errorf("expected method PATCH, got %q", gotMethod)
	}
	if gotContentType != "application/json" {
		t.Errorf("expected Content-Type application/json, got %q", gotContentType)
	}
}

// Test: HEAD request succeeds with no body and no error.
func TestDo_HeadRequest(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodHead {
			t.Errorf("expected HEAD method, got %s", r.Method)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		// HEAD responses must not include a body per HTTP spec.
	}))
	defer ts.Close()

	var stdout, stderr bytes.Buffer
	c := newTestClient(ts.URL, &stdout, &stderr)

	code := c.Do(context.Background(), "HEAD", "/rest/api/3/issue/PROJ-1", nil, nil)
	if code != 0 {
		t.Fatalf("expected exit code 0 for HEAD request, got %d; stderr=%s", code, stderr.String())
	}
	if stderr.Len() != 0 {
		t.Errorf("expected empty stderr for HEAD request, got: %s", stderr.String())
	}
}

// Test: Invalid URL produces connection_error on stderr with exit code 1.
func TestDo_ConnectionError(t *testing.T) {
	var stdout, stderr bytes.Buffer
	c := newTestClient("http://127.0.0.1:0", &stdout, &stderr)

	code := c.Do(context.Background(), "GET", "/rest/api/3/test", nil, nil)
	if code != 1 {
		t.Fatalf("expected exit code 1 (error), got %d", code)
	}

	var errObj map[string]interface{}
	if err := json.Unmarshal(stderr.Bytes(), &errObj); err != nil {
		t.Fatalf("stderr is not valid JSON: %s; err=%v", stderr.String(), err)
	}
	if errObj["error_type"] != "connection_error" {
		t.Errorf("expected error_type 'connection_error', got: %v", errObj["error_type"])
	}
}

// Test: Pretty=true outputs indented JSON with newlines.
func TestDo_PrettyPrint(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintln(w, `{"key":"PROJ-1","summary":"Test issue"}`)
	}))
	defer ts.Close()

	var stdout, stderr bytes.Buffer
	c := newTestClient(ts.URL, &stdout, &stderr)
	c.Pretty = true

	code := c.Do(context.Background(), "GET", "/rest/api/3/issue/PROJ-1", nil, nil)
	if code != 0 {
		t.Fatalf("expected exit code 0, got %d; stderr=%s", code, stderr.String())
	}

	out := stdout.String()
	if !strings.Contains(out, "\n") {
		t.Error("expected pretty-printed output with newlines")
	}
	// Indented JSON typically has leading spaces before field names.
	if !strings.Contains(out, "  ") {
		t.Error("expected indentation in pretty-printed output")
	}
	// Must still be valid JSON.
	var result map[string]interface{}
	if err := json.Unmarshal([]byte(strings.TrimSpace(out)), &result); err != nil {
		t.Fatalf("pretty output is not valid JSON: err=%v\noutput: %s", err, out)
	}
	if result["key"] != "PROJ-1" {
		t.Errorf("expected key=PROJ-1, got %v", result["key"])
	}
}

// Test: Verbose=true logs both request and response lines to stderr as JSON.
func TestDo_VerboseRequestAndResponse(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintln(w, `{"ok":true}`)
	}))
	defer ts.Close()

	var stdout, stderr bytes.Buffer
	c := newTestClient(ts.URL, &stdout, &stderr)
	c.Verbose = true

	code := c.Do(context.Background(), "GET", "/rest/api/3/test", nil, nil)
	if code != 0 {
		t.Fatalf("expected exit code 0, got %d", code)
	}

	lines := strings.Split(strings.TrimSpace(stderr.String()), "\n")
	if len(lines) < 2 {
		t.Fatalf("expected at least 2 lines in stderr (request + response), got %d: %s", len(lines), stderr.String())
	}

	var reqLog map[string]interface{}
	if err := json.Unmarshal([]byte(lines[0]), &reqLog); err != nil {
		t.Fatalf("request log line not valid JSON: %s", lines[0])
	}
	if reqLog["type"] != "request" {
		t.Errorf("expected first log type=request, got %v", reqLog["type"])
	}
	if reqLog["method"] != "GET" {
		t.Errorf("expected method=GET in request log, got %v", reqLog["method"])
	}

	var respLog map[string]interface{}
	if err := json.Unmarshal([]byte(lines[1]), &respLog); err != nil {
		t.Fatalf("response log line not valid JSON: %s", lines[1])
	}
	if respLog["type"] != "response" {
		t.Errorf("expected second log type=response, got %v", respLog["type"])
	}
	if int(respLog["status"].(float64)) != 200 {
		t.Errorf("expected status=200 in response log, got %v", respLog["status"])
	}
}

// Test: Invalid jq expression returns jq_error with exit code 4.
func TestDo_JQInvalidFilter(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintln(w, `{"key":"PROJ-1"}`)
	}))
	defer ts.Close()

	var stdout, stderr bytes.Buffer
	c := newTestClient(ts.URL, &stdout, &stderr)
	c.JQFilter = ".[[[invalid jq"

	code := c.Do(context.Background(), "GET", "/path", nil, nil)
	if code != 4 {
		t.Fatalf("expected exit code 4 (validation/jq_error), got %d; stderr=%s", code, stderr.String())
	}

	var errObj map[string]interface{}
	if err := json.Unmarshal(stderr.Bytes(), &errObj); err != nil {
		t.Fatalf("stderr is not valid JSON: %s; err=%v", stderr.String(), err)
	}
	if errObj["error_type"] != "jq_error" {
		t.Errorf("expected error_type 'jq_error', got: %v", errObj["error_type"])
	}
}

// Test: jq filter producing multiple results returns each on its own line.
func TestDo_JQMultipleResults(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintln(w, `{"items":[{"id":1},{"id":2},{"id":3}]}`)
	}))
	defer ts.Close()

	var stdout, stderr bytes.Buffer
	c := newTestClient(ts.URL, &stdout, &stderr)
	c.JQFilter = "[.items[].id]"

	code := c.Do(context.Background(), "GET", "/path", nil, nil)
	if code != 0 {
		t.Fatalf("expected exit code 0, got %d; stderr=%s", code, stderr.String())
	}

	out := strings.TrimSpace(stdout.String())
	var ids []interface{}
	if err := json.Unmarshal([]byte(out), &ids); err != nil {
		t.Fatalf("stdout is not valid JSON array: %s; err=%v", out, err)
	}
	if len(ids) != 3 {
		t.Errorf("expected 3 results, got %d", len(ids))
	}
}

// Test: jq object construction {key: .key, name: .name} works correctly.
func TestDo_JQObjectConstruction(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintln(w, `{"key":"PROJ-7","name":"Test Project","extra":"ignored"}`)
	}))
	defer ts.Close()

	var stdout, stderr bytes.Buffer
	c := newTestClient(ts.URL, &stdout, &stderr)
	c.JQFilter = `{key: .key, name: .name}`

	code := c.Do(context.Background(), "GET", "/path", nil, nil)
	if code != 0 {
		t.Fatalf("expected exit code 0, got %d; stderr=%s", code, stderr.String())
	}

	var result map[string]interface{}
	if err := json.Unmarshal(stdout.Bytes(), &result); err != nil {
		t.Fatalf("stdout is not valid JSON: %s; err=%v", stdout.String(), err)
	}
	if result["key"] != "PROJ-7" {
		t.Errorf("expected key=PROJ-7, got %v", result["key"])
	}
	if result["name"] != "Test Project" {
		t.Errorf("expected name=Test Project, got %v", result["name"])
	}
	if _, hasExtra := result["extra"]; hasExtra {
		t.Error("jq object construction should not include 'extra' field")
	}
}

// Test: Fields="key,summary" is NOT added as query param for POST requests.
func TestDo_FieldsNotAddedToPost(t *testing.T) {
	var gotRawQuery string
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotRawQuery = r.URL.RawQuery
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintln(w, `{"id":"PROJ-99"}`)
	}))
	defer ts.Close()

	var stdout, stderr bytes.Buffer
	c := newTestClient(ts.URL, &stdout, &stderr)
	c.Fields = "key,summary"

	code := c.Do(context.Background(), "POST", "/rest/api/3/issue", nil, strings.NewReader(`{"fields":{}}`))
	if code != 0 {
		t.Fatalf("expected exit code 0, got %d; stderr=%s", code, stderr.String())
	}
	if strings.Contains(gotRawQuery, "fields=") {
		t.Errorf("fields query param must not be added to POST requests, got query: %q", gotRawQuery)
	}
}

// Test: Fields="key,summary" IS added as ?fields=key,summary for GET requests.
func TestDo_FieldsAddedToGet(t *testing.T) {
	var gotFields string
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotFields = r.URL.Query().Get("fields")
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintln(w, `{"key":"PROJ-1","summary":"Hello"}`)
	}))
	defer ts.Close()

	var stdout, stderr bytes.Buffer
	c := newTestClient(ts.URL, &stdout, &stderr)
	c.Fields = "key,summary"

	code := c.Do(context.Background(), "GET", "/rest/api/3/issue/PROJ-1", nil, nil)
	if code != 0 {
		t.Fatalf("expected exit code 0, got %d; stderr=%s", code, stderr.String())
	}
	if gotFields != "key,summary" {
		t.Errorf("expected fields query param=key,summary, got %q", gotFields)
	}
}

// Test: DryRun with query params includes them in the output URL.
func TestDo_DryRunIncludesQuery(t *testing.T) {
	var stdout, stderr bytes.Buffer
	c := &client.Client{
		BaseURL:    "https://example.atlassian.net",
		HTTPClient: &http.Client{},
		Stdout:     &stdout,
		Stderr:     &stderr,
		DryRun:     true,
	}

	q := map[string][]string{"jql": {"project = PROJ"}, "maxResults": {"10"}}
	code := c.Do(context.Background(), "GET", "/rest/api/3/search", q, nil)
	if code != 0 {
		t.Fatalf("expected exit code 0 for dry run, got %d; stderr=%s", code, stderr.String())
	}

	var obj map[string]string
	if err := json.Unmarshal(stdout.Bytes(), &obj); err != nil {
		t.Fatalf("stdout is not valid JSON: %s; err=%v", stdout.String(), err)
	}
	if !strings.Contains(obj["url"], "jql=") {
		t.Errorf("expected url to contain jql query param, got %q", obj["url"])
	}
	if !strings.Contains(obj["url"], "maxResults=") {
		t.Errorf("expected url to contain maxResults query param, got %q", obj["url"])
	}
}

// Test: DryRun with POST body still outputs request JSON without executing.
func TestDo_DryRunWithBody(t *testing.T) {
	// Use a server that would record if it got called.
	called := false
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusCreated)
	}))
	defer ts.Close()

	var stdout, stderr bytes.Buffer
	c := &client.Client{
		BaseURL:    ts.URL,
		HTTPClient: &http.Client{},
		Stdout:     &stdout,
		Stderr:     &stderr,
		DryRun:     true,
	}

	body := strings.NewReader(`{"fields":{"summary":"Dry run issue"}}`)
	code := c.Do(context.Background(), "POST", "/rest/api/3/issue", nil, body)
	if code != 0 {
		t.Fatalf("expected exit code 0 for dry run, got %d; stderr=%s", code, stderr.String())
	}
	if called {
		t.Error("dry run must not execute the actual HTTP request")
	}

	var obj map[string]interface{}
	if err := json.Unmarshal(stdout.Bytes(), &obj); err != nil {
		t.Fatalf("stdout is not valid JSON: %s; err=%v", stdout.String(), err)
	}
	if obj["method"] != "POST" {
		t.Errorf("expected method=POST in dry run output, got %q", obj["method"])
	}
	urlStr, _ := obj["url"].(string)
	if !strings.Contains(urlStr, "/rest/api/3/issue") {
		t.Errorf("expected url to contain path, got %q", urlStr)
	}
	// Verify body is included in dry-run output.
	bodyObj, ok := obj["body"]
	if !ok {
		t.Error("expected body field in dry-run output for POST request")
	} else {
		bodyMap, ok := bodyObj.(map[string]interface{})
		if !ok {
			t.Errorf("expected body to be a JSON object, got %T", bodyObj)
		} else if bodyMap["fields"] == nil {
			t.Error("expected body.fields in dry-run output")
		}
	}
}

// Test: POST request does NOT cache even when CacheTTL is set.
func TestDo_CacheOnlyForGet(t *testing.T) {
	callCount := 0
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintln(w, `{"created":true}`)
	}))
	defer ts.Close()

	var stdout1, stderr1 bytes.Buffer
	c := newTestClient(ts.URL, &stdout1, &stderr1)
	c.CacheTTL = 5 * time.Minute

	code := c.Do(context.Background(), "POST", "/rest/api/3/issue", nil, strings.NewReader(`{}`))
	if code != 0 {
		t.Fatalf("expected exit code 0, got %d", code)
	}
	if callCount != 1 {
		t.Fatalf("expected 1 server call, got %d", callCount)
	}

	// Second POST to same path — must not be served from cache.
	var stdout2, stderr2 bytes.Buffer
	c.Stdout = &stdout2
	c.Stderr = &stderr2
	code = c.Do(context.Background(), "POST", "/rest/api/3/issue", nil, strings.NewReader(`{}`))
	if code != 0 {
		t.Fatalf("expected exit code 0 on second call, got %d", code)
	}
	if callCount != 2 {
		t.Errorf("expected 2 server calls (POST never cached), got %d", callCount)
	}
}

// Test: Expired cache entry triggers a new server call.
func TestDo_CacheExpired(t *testing.T) {
	callCount := 0
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, `{"call":%d}`, callCount)
	}))
	defer ts.Close()

	var stdout1, stderr1 bytes.Buffer
	c := newTestClient(ts.URL, &stdout1, &stderr1)
	// Use a tiny TTL so it expires almost immediately.
	c.CacheTTL = 1 * time.Millisecond

	code := c.Do(context.Background(), "GET", "/rest/api/3/expire-test", nil, nil)
	if code != 0 {
		t.Fatalf("expected exit code 0, got %d", code)
	}
	if callCount != 1 {
		t.Fatalf("expected 1 server call, got %d", callCount)
	}

	// Wait for the cache to expire.
	time.Sleep(10 * time.Millisecond)

	var stdout2, stderr2 bytes.Buffer
	c.Stdout = &stdout2
	c.Stderr = &stderr2
	code = c.Do(context.Background(), "GET", "/rest/api/3/expire-test", nil, nil)
	if code != 0 {
		t.Fatalf("expected exit code 0 on second call, got %d", code)
	}
	if callCount != 2 {
		t.Errorf("expected 2 server calls after cache expiry, got %d", callCount)
	}
}

// Test: 3 pages merged correctly — total=6, values contain ids 1-6.
func TestDo_PaginationMultiplePages(t *testing.T) {
	callCount := 0
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		w.Header().Set("Content-Type", "application/json")
		startAt := r.URL.Query().Get("startAt")
		switch startAt {
		case "", "0":
			fmt.Fprintln(w, `{"startAt":0,"maxResults":2,"total":6,"values":[{"id":1},{"id":2}]}`)
		case "2":
			fmt.Fprintln(w, `{"startAt":2,"maxResults":2,"total":6,"values":[{"id":3},{"id":4}]}`)
		default:
			fmt.Fprintln(w, `{"startAt":4,"maxResults":2,"total":6,"values":[{"id":5},{"id":6}]}`)
		}
	}))
	defer ts.Close()

	var stdout, stderr bytes.Buffer
	c := newTestClient(ts.URL, &stdout, &stderr)
	c.Paginate = true

	code := c.Do(context.Background(), "GET", "/path", nil, nil)
	if code != 0 {
		t.Fatalf("expected exit code 0, got %d; stderr=%s", code, stderr.String())
	}
	if callCount != 3 {
		t.Errorf("expected 3 HTTP calls for 3 pages, got %d", callCount)
	}

	var envelope struct {
		Total  int                      `json:"total"`
		Values []map[string]interface{} `json:"values"`
		IsLast bool                     `json:"isLast"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &envelope); err != nil {
		t.Fatalf("stdout is not valid JSON: %s; err=%v", stdout.String(), err)
	}
	if len(envelope.Values) != 6 {
		t.Errorf("expected 6 merged values, got %d", len(envelope.Values))
	}
	if envelope.Total != 6 {
		t.Errorf("expected total=6, got %d", envelope.Total)
	}
	if !envelope.IsLast {
		t.Error("expected isLast=true after all pages merged")
	}
	// Verify ids are 1-6 in order.
	for i, v := range envelope.Values {
		expectedID := float64(i + 1)
		if v["id"] != expectedID {
			t.Errorf("expected values[%d].id=%v, got %v", i, expectedID, v["id"])
		}
	}
}

// Test: POST with Paginate=true still goes through doOnce (no pagination for non-GET).
func TestDo_PaginationDoesNotApplyToPost(t *testing.T) {
	callCount := 0
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		if r.Method != http.MethodPost {
			t.Errorf("expected POST, got %s", r.Method)
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintln(w, `{"startAt":0,"maxResults":2,"total":4,"values":[{"id":1},{"id":2}]}`)
	}))
	defer ts.Close()

	var stdout, stderr bytes.Buffer
	c := newTestClient(ts.URL, &stdout, &stderr)
	c.Paginate = true

	code := c.Do(context.Background(), "POST", "/rest/api/3/search", nil, strings.NewReader(`{"jql":"project=PROJ"}`))
	if code != 0 {
		t.Fatalf("expected exit code 0, got %d; stderr=%s", code, stderr.String())
	}
	// POST + Paginate=true must only make one server call.
	if callCount != 1 {
		t.Errorf("expected exactly 1 server call for POST (no pagination), got %d", callCount)
	}
	// Response should be the raw first (and only) page, not a merged envelope.
	var envelope struct {
		Total int `json:"total"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &envelope); err != nil {
		t.Fatalf("stdout is not valid JSON: %s; err=%v", stdout.String(), err)
	}
	if envelope.Total != 4 {
		t.Errorf("expected total=4 in raw response, got %d", envelope.Total)
	}
}

// Test: HTTP 401 returns exit code 2 with error_type=auth_failed and a hint.
func TestDo_401StructuredError(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		fmt.Fprintln(w, `{"message":"Unauthorized"}`)
	}))
	defer ts.Close()

	var stdout, stderr bytes.Buffer
	c := newTestClient(ts.URL, &stdout, &stderr)

	code := c.Do(context.Background(), "GET", "/rest/api/3/myself", nil, nil)
	if code != 2 {
		t.Fatalf("expected exit code 2 (auth), got %d; stderr=%s", code, stderr.String())
	}

	var errObj map[string]interface{}
	if err := json.Unmarshal(stderr.Bytes(), &errObj); err != nil {
		t.Fatalf("stderr is not valid JSON: %s; err=%v", stderr.String(), err)
	}
	if errObj["error_type"] != "auth_failed" {
		t.Errorf("expected error_type 'auth_failed', got: %v", errObj["error_type"])
	}
	if int(errObj["status"].(float64)) != 401 {
		t.Errorf("expected status=401, got: %v", errObj["status"])
	}
	hint, _ := errObj["hint"].(string)
	if hint == "" {
		t.Error("expected non-empty hint for 401 error")
	}
}

// Test: HTTP 403 returns exit code 2 with error_type=auth_failed and a hint.
func TestDo_403StructuredError(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		fmt.Fprintln(w, `{"message":"Forbidden"}`)
	}))
	defer ts.Close()

	var stdout, stderr bytes.Buffer
	c := newTestClient(ts.URL, &stdout, &stderr)

	code := c.Do(context.Background(), "GET", "/rest/api/3/issue/PROJ-1", nil, nil)
	if code != 2 {
		t.Fatalf("expected exit code 2 (auth), got %d; stderr=%s", code, stderr.String())
	}

	var errObj map[string]interface{}
	if err := json.Unmarshal(stderr.Bytes(), &errObj); err != nil {
		t.Fatalf("stderr is not valid JSON: %s; err=%v", stderr.String(), err)
	}
	if errObj["error_type"] != "auth_failed" {
		t.Errorf("expected error_type 'auth_failed', got: %v", errObj["error_type"])
	}
	if int(errObj["status"].(float64)) != 403 {
		t.Errorf("expected status=403, got: %v", errObj["status"])
	}
	hint, _ := errObj["hint"].(string)
	if hint == "" {
		t.Error("expected non-empty hint for 403 error")
	}
}

// Test: HTTP 409 returns exit code 6 with error_type=conflict.
func TestDo_409ConflictError(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusConflict)
		fmt.Fprintln(w, `{"errorMessages":["Issue already exists"]}`)
	}))
	defer ts.Close()

	var stdout, stderr bytes.Buffer
	c := newTestClient(ts.URL, &stdout, &stderr)

	code := c.Do(context.Background(), "POST", "/rest/api/3/issue", nil, strings.NewReader(`{}`))
	if code != 6 {
		t.Fatalf("expected exit code 6 (conflict), got %d; stderr=%s", code, stderr.String())
	}

	var errObj map[string]interface{}
	if err := json.Unmarshal(stderr.Bytes(), &errObj); err != nil {
		t.Fatalf("stderr is not valid JSON: %s; err=%v", stderr.String(), err)
	}
	if errObj["error_type"] != "conflict" {
		t.Errorf("expected error_type 'conflict', got: %v", errObj["error_type"])
	}
	if int(errObj["status"].(float64)) != 409 {
		t.Errorf("expected status=409, got: %v", errObj["status"])
	}
}

// Test: HTTP 500 returns exit code 7 with error_type=server_error.
func TestDo_500ServerError(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		fmt.Fprintln(w, `{"errorMessages":["Internal Server Error"]}`)
	}))
	defer ts.Close()

	var stdout, stderr bytes.Buffer
	c := newTestClient(ts.URL, &stdout, &stderr)

	code := c.Do(context.Background(), "GET", "/rest/api/3/issue/PROJ-1", nil, nil)
	if code != 7 {
		t.Fatalf("expected exit code 7 (server_error), got %d; stderr=%s", code, stderr.String())
	}

	var errObj map[string]interface{}
	if err := json.Unmarshal(stderr.Bytes(), &errObj); err != nil {
		t.Fatalf("stderr is not valid JSON: %s; err=%v", stderr.String(), err)
	}
	if errObj["error_type"] != "server_error" {
		t.Errorf("expected error_type 'server_error', got: %v", errObj["error_type"])
	}
	if int(errObj["status"].(float64)) != 500 {
		t.Errorf("expected status=500, got: %v", errObj["status"])
	}
}

// Test: Server returning HTML error body gets a sanitized message (not raw HTML).
func TestDo_HTMLErrorSanitized(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		w.WriteHeader(http.StatusServiceUnavailable)
		fmt.Fprintln(w, `<!DOCTYPE html><html><body><h1>Service Unavailable</h1></body></html>`)
	}))
	defer ts.Close()

	var stdout, stderr bytes.Buffer
	c := newTestClient(ts.URL, &stdout, &stderr)

	code := c.Do(context.Background(), "GET", "/rest/api/3/test", nil, nil)
	if code == 0 {
		t.Fatal("expected non-zero exit code for 503 response")
	}

	var errObj map[string]interface{}
	if err := json.Unmarshal(stderr.Bytes(), &errObj); err != nil {
		t.Fatalf("stderr is not valid JSON: %s; err=%v", stderr.String(), err)
	}
	msg, _ := errObj["message"].(string)
	if strings.Contains(msg, "<html") || strings.Contains(msg, "<!DOCTYPE") {
		t.Errorf("expected sanitized message (no raw HTML), got: %q", msg)
	}
	if !strings.Contains(msg, "503") && !strings.Contains(strings.ToLower(msg), "html") {
		t.Errorf("expected message to reference status or 'html', got: %q", msg)
	}
}

// Test: 100KB response body is handled correctly.
func TestDo_LargeResponse(t *testing.T) {
	// Build a JSON object with a 100KB string value.
	const targetSize = 100 * 1024
	largeValue := strings.Repeat("x", targetSize)
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, `{"data":"%s"}`, largeValue)
	}))
	defer ts.Close()

	var stdout, stderr bytes.Buffer
	c := newTestClient(ts.URL, &stdout, &stderr)

	code := c.Do(context.Background(), "GET", "/rest/api/3/large", nil, nil)
	if code != 0 {
		t.Fatalf("expected exit code 0, got %d; stderr=%s", code, stderr.String())
	}

	var result map[string]interface{}
	if err := json.Unmarshal(stdout.Bytes(), &result); err != nil {
		t.Fatalf("stdout is not valid JSON for large response; err=%v", err)
	}
	data, _ := result["data"].(string)
	if len(data) != targetSize {
		t.Errorf("expected data length %d, got %d", targetSize, len(data))
	}
}

// Test: Empty JQFilter ("") passes response through unchanged.
func TestDo_EmptyJQFilterPassthrough(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintln(w, `{"key":"PROJ-1","summary":"No filter applied"}`)
	}))
	defer ts.Close()

	var stdout, stderr bytes.Buffer
	c := newTestClient(ts.URL, &stdout, &stderr)
	c.JQFilter = "" // Explicitly empty — must behave as passthrough.

	code := c.Do(context.Background(), "GET", "/rest/api/3/issue/PROJ-1", nil, nil)
	if code != 0 {
		t.Fatalf("expected exit code 0, got %d; stderr=%s", code, stderr.String())
	}
	if stderr.Len() != 0 {
		t.Errorf("expected empty stderr, got: %s", stderr.String())
	}

	var result map[string]interface{}
	if err := json.Unmarshal(stdout.Bytes(), &result); err != nil {
		t.Fatalf("stdout is not valid JSON: %s; err=%v", stdout.String(), err)
	}
	if result["key"] != "PROJ-1" {
		t.Errorf("expected key=PROJ-1 in passthrough response, got %v", result["key"])
	}
	if result["summary"] != "No filter applied" {
		t.Errorf("expected summary intact, got %v", result["summary"])
	}
}

// Test: Paginated GET with Paginate=true and CacheTTL set caches the merged result.
// Uses a proper paginated response (with a "values" key) so the pagination code
// path runs and caches the merged envelope on the first call.
func TestDo_PaginationCachePassthrough(t *testing.T) {
	callCount := 0
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		w.Header().Set("Content-Type", "application/json")
		// Single-page paginated response (total == len(values)) so only one fetch.
		fmt.Fprintln(w, `{"startAt":0,"maxResults":50,"total":1,"values":[{"displayName":"Agent Smith"}]}`)
	}))
	defer ts.Close()

	var stdout1, stderr1 bytes.Buffer
	c := newTestClient(ts.URL, &stdout1, &stderr1)
	c.Paginate = true
	c.CacheTTL = 5 * time.Minute

	code := c.Do(context.Background(), "GET", "/rest/api/3/paginationcache-passthrough-test", nil, nil)
	if code != 0 {
		t.Fatalf("expected exit code 0, got %d", code)
	}
	if callCount != 1 {
		t.Fatalf("expected 1 server call, got %d", callCount)
	}

	// Second call must be served from cache (no additional server calls).
	var stdout2, stderr2 bytes.Buffer
	c.Stdout = &stdout2
	c.Stderr = &stderr2
	code = c.Do(context.Background(), "GET", "/rest/api/3/paginationcache-passthrough-test", nil, nil)
	if code != 0 {
		t.Fatalf("expected exit code 0 on cached call, got %d", code)
	}
	if callCount != 1 {
		t.Errorf("expected still 1 server call (cache hit), got %d", callCount)
	}

	// Both outputs must be identical valid JSON containing the merged envelope.
	if stdout1.String() != stdout2.String() {
		t.Errorf("cached output differs:\n  first:  %s\n  second: %s", stdout1.String(), stdout2.String())
	}
	var envelope struct {
		Values []map[string]interface{} `json:"values"`
		Total  int                      `json:"total"`
	}
	if err := json.Unmarshal(stdout2.Bytes(), &envelope); err != nil {
		t.Fatalf("cached stdout is not valid JSON: %s; err=%v", stdout2.String(), err)
	}
	if len(envelope.Values) != 1 {
		t.Errorf("expected 1 value in cached response, got %d", len(envelope.Values))
	}
	if envelope.Values[0]["displayName"] != "Agent Smith" {
		t.Errorf("expected displayName=Agent Smith, got %v", envelope.Values[0]["displayName"])
	}
}

// TestDo_TokenPagination tests auto-pagination for nextPageToken-based responses
// (e.g. the new /search/jql endpoint).
func TestDo_TokenPagination(t *testing.T) {
	page := 0
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch page {
		case 0:
			page++
			fmt.Fprintln(w, `{"issues":[{"key":"A-1"},{"key":"A-2"}],"nextPageToken":"tok1","isLast":false}`)
		case 1:
			page++
			if r.URL.Query().Get("nextPageToken") != "tok1" {
				t.Errorf("expected nextPageToken=tok1, got %q", r.URL.Query().Get("nextPageToken"))
			}
			fmt.Fprintln(w, `{"issues":[{"key":"A-3"}],"isLast":true}`)
		default:
			t.Error("unexpected extra page fetch")
			fmt.Fprintln(w, `{"issues":[],"isLast":true}`)
		}
	}))
	defer ts.Close()

	var stdout, stderr bytes.Buffer
	c := newTestClient(ts.URL, &stdout, &stderr)
	c.Paginate = true

	code := c.Do(context.Background(), "GET", "/rest/api/3/search/jql", nil, nil)
	if code != 0 {
		t.Fatalf("expected exit 0, got %d; stderr=%s", code, stderr.String())
	}

	var resp struct {
		Issues []map[string]string `json:"issues"`
		IsLast bool                `json:"isLast"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &resp); err != nil {
		t.Fatalf("invalid JSON: %s; err=%v", stdout.String(), err)
	}
	if len(resp.Issues) != 3 {
		t.Errorf("expected 3 issues, got %d", len(resp.Issues))
	}
	if !resp.IsLast {
		t.Error("expected isLast=true in merged response")
	}
	// nextPageToken should be removed from merged response
	var raw map[string]json.RawMessage
	_ = json.Unmarshal(stdout.Bytes(), &raw)
	if _, ok := raw["nextPageToken"]; ok {
		t.Error("nextPageToken should be removed from merged response")
	}
}

// TestDo_TokenPagination_SinglePage tests that a single-page token response passes through.
func TestDo_TokenPagination_SinglePage(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintln(w, `{"issues":[{"key":"B-1"}],"isLast":true}`)
	}))
	defer ts.Close()

	var stdout, stderr bytes.Buffer
	c := newTestClient(ts.URL, &stdout, &stderr)
	c.Paginate = true

	code := c.Do(context.Background(), "GET", "/rest/api/3/search/jql", nil, nil)
	if code != 0 {
		t.Fatalf("expected exit 0, got %d; stderr=%s", code, stderr.String())
	}

	var resp struct {
		Issues []map[string]string `json:"issues"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &resp); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if len(resp.Issues) != 1 {
		t.Errorf("expected 1 issue, got %d", len(resp.Issues))
	}
}

// TestDo_TokenPagination_ErrorOnPage2 tests error handling during token pagination.
func TestDo_TokenPagination_ErrorOnPage2(t *testing.T) {
	page := 0
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch page {
		case 0:
			page++
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprintln(w, `{"issues":[{"key":"C-1"}],"nextPageToken":"tok1","isLast":false}`)
		default:
			w.WriteHeader(500)
			fmt.Fprintln(w, `{"errorMessages":["server error"]}`)
		}
	}))
	defer ts.Close()

	var stdout, stderr bytes.Buffer
	c := newTestClient(ts.URL, &stdout, &stderr)
	c.Paginate = true

	code := c.Do(context.Background(), "GET", "/rest/api/3/search/jql", nil, nil)
	if code == 0 {
		t.Error("expected non-zero exit code on page 2 error")
	}
}

// TestDo_TokenPagination_WithJQ tests JQ filtering on token-paginated responses.
func TestDo_TokenPagination_WithJQ(t *testing.T) {
	page := 0
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch page {
		case 0:
			page++
			fmt.Fprintln(w, `{"issues":[{"key":"D-1"},{"key":"D-2"}],"nextPageToken":"tok1","isLast":false}`)
		default:
			fmt.Fprintln(w, `{"issues":[{"key":"D-3"}],"isLast":true}`)
		}
	}))
	defer ts.Close()

	var stdout, stderr bytes.Buffer
	c := newTestClient(ts.URL, &stdout, &stderr)
	c.Paginate = true
	c.JQFilter = "[.issues[] | .key]"

	code := c.Do(context.Background(), "GET", "/rest/api/3/search/jql", nil, nil)
	if code != 0 {
		t.Fatalf("expected exit 0, got %d; stderr=%s", code, stderr.String())
	}

	var keys []string
	if err := json.Unmarshal(stdout.Bytes(), &keys); err != nil {
		t.Fatalf("invalid JSON: %v; stdout=%s", err, stdout.String())
	}
	if len(keys) != 3 {
		t.Errorf("expected 3 keys, got %d: %v", len(keys), keys)
	}
}

// TestDo_FieldsPropagation tests that the Fields option applies to GET requests.
func TestDo_FieldsPropagation(t *testing.T) {
	var gotFields string
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotFields = r.URL.Query().Get("fields")
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintln(w, `{"key":"PROJ-1"}`)
	}))
	defer ts.Close()

	var stdout, stderr bytes.Buffer
	c := newTestClient(ts.URL, &stdout, &stderr)
	c.Fields = "key,summary,status"

	code := c.Do(context.Background(), "GET", "/rest/api/3/issue/PROJ-1", nil, nil)
	if code != 0 {
		t.Fatalf("expected exit 0, got %d", code)
	}
	if gotFields != "key,summary,status" {
		t.Errorf("expected fields=key,summary,status, got %q", gotFields)
	}
}

// TestDo_FieldsIgnoredOnPost tests that Fields are not applied to POST requests.
func TestDo_FieldsIgnoredOnPost(t *testing.T) {
	var gotFields string
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotFields = r.URL.Query().Get("fields")
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintln(w, `{"id":"123"}`)
	}))
	defer ts.Close()

	var stdout, stderr bytes.Buffer
	c := newTestClient(ts.URL, &stdout, &stderr)
	c.Fields = "key,summary"

	code := c.Do(context.Background(), "POST", "/rest/api/3/issue", nil, strings.NewReader(`{"fields":{}}`))
	if code != 0 {
		t.Fatalf("expected exit 0, got %d", code)
	}
	if gotFields != "" {
		t.Errorf("expected no fields query param on POST, got %q", gotFields)
	}
}

// Test: Token pagination terminates when a subsequent page returns empty issues
// (regression test for BUG-1: stale len(firstPage.Issues) check).
func TestTokenPagination_EmptySubsequentPage(t *testing.T) {
	callCount := 0
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		w.Header().Set("Content-Type", "application/json")
		switch callCount {
		case 1:
			// First page: 2 issues, has next token
			fmt.Fprintln(w, `{"issues":[{"id":"1"},{"id":"2"}],"nextPageToken":"tok1"}`)
		case 2:
			// Second page: 1 issue, has next token
			fmt.Fprintln(w, `{"issues":[{"id":"3"}],"nextPageToken":"tok2"}`)
		case 3:
			// Third page: empty issues with token still present — should stop
			fmt.Fprintln(w, `{"issues":[],"nextPageToken":"tok3"}`)
		default:
			// Should never reach here
			t.Errorf("unexpected request #%d — pagination should have stopped", callCount)
			fmt.Fprintln(w, `{"issues":[]}`)
		}
	}))
	defer ts.Close()

	var stdout, stderr bytes.Buffer
	c := newTestClient(ts.URL, &stdout, &stderr)
	c.Paginate = true

	code := c.Do(context.Background(), "GET", "/test", nil, nil)
	if code != 0 {
		t.Fatalf("expected exit 0, got %d; stderr=%s", code, stderr.String())
	}

	// Parse result and verify all 3 issues are collected.
	var result struct {
		Issues []json.RawMessage `json:"issues"`
	}
	if err := json.Unmarshal([]byte(strings.TrimSpace(stdout.String())), &result); err != nil {
		t.Fatalf("failed to parse output: %v", err)
	}
	if len(result.Issues) != 3 {
		t.Errorf("expected 3 issues, got %d", len(result.Issues))
	}
	// Must not make more than 3 requests
	if callCount > 3 {
		t.Errorf("expected at most 3 requests, made %d (infinite loop bug)", callCount)
	}
}

// Test: Token pagination terminates correctly when server sends non-empty
// nextPageToken alongside empty issues array (potential infinite loop scenario).
func TestTokenPagination_NonEmptyTokenEmptyIssues(t *testing.T) {
	callCount := 0
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		w.Header().Set("Content-Type", "application/json")
		if callCount == 1 {
			// First page: has issues
			fmt.Fprintln(w, `{"issues":[{"id":"1"}],"nextPageToken":"tok1"}`)
		} else {
			// Subsequent page: empty issues but non-empty token — MUST stop
			fmt.Fprintln(w, `{"issues":[],"nextPageToken":"infinite-loop-trap"}`)
		}
	}))
	defer ts.Close()

	var stdout, stderr bytes.Buffer
	c := newTestClient(ts.URL, &stdout, &stderr)
	c.Paginate = true

	code := c.Do(context.Background(), "GET", "/test", nil, nil)
	if code != 0 {
		t.Fatalf("expected exit 0, got %d; stderr=%s", code, stderr.String())
	}

	// Should have made exactly 2 requests (first page + one empty page)
	if callCount > 2 {
		t.Fatalf("expected at most 2 requests, made %d (infinite loop detected!)", callCount)
	}
}

// TestDo_CacheNonPaginatedGET verifies that non-paginated GET responses
// are cached when --cache is set and Paginate is true (the default path).
func TestDo_CacheNonPaginatedGET(t *testing.T) {
	callCount := 0
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintln(w, `{"key":"PROJ-1","fields":{"summary":"Test"}}`)
	}))
	defer ts.Close()

	var stdout1, stderr1 bytes.Buffer
	c := newTestClient(ts.URL, &stdout1, &stderr1)
	c.Paginate = true
	c.CacheTTL = 5 * time.Minute

	// First call should hit the server.
	code := c.Do(context.Background(), "GET", "/rest/api/3/issue/PROJ-1", nil, nil)
	if code != 0 {
		t.Fatalf("first call: expected exit 0, got %d; stderr=%s", code, stderr1.String())
	}
	if callCount != 1 {
		t.Fatalf("expected 1 server call after first request, got %d", callCount)
	}

	// Second call with same URL should be served from cache.
	var stdout2, stderr2 bytes.Buffer
	c.Stdout = &stdout2
	c.Stderr = &stderr2

	code = c.Do(context.Background(), "GET", "/rest/api/3/issue/PROJ-1", nil, nil)
	if code != 0 {
		t.Fatalf("second call: expected exit 0, got %d; stderr=%s", code, stderr2.String())
	}
	if callCount != 1 {
		t.Fatalf("expected still 1 server call after cached request, got %d", callCount)
	}

	// Both calls should return the same data.
	if strings.TrimSpace(stdout1.String()) != strings.TrimSpace(stdout2.String()) {
		t.Errorf("cached response differs: %q vs %q", stdout1.String(), stdout2.String())
	}
}

// --- Bug 25: OAuth2 token request should check HTTP status code ---

func TestOAuth2_ErrorStatusCode(t *testing.T) {
	// OAuth2 token endpoint returns 401 with an error body.
	tokenServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		fmt.Fprintln(w, `{"error":"invalid_client","error_description":"bad credentials"}`)
	}))
	defer tokenServer.Close()

	apiServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// If the OAuth2 token fetch failed, the Authorization header should be empty.
		authHeader := r.Header.Get("Authorization")
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, `{"auth_header":%q}`, authHeader)
	}))
	defer apiServer.Close()

	var stdout, stderr bytes.Buffer
	c := newTestClient(apiServer.URL, &stdout, &stderr)
	c.Auth = config.AuthConfig{
		Type:         "oauth2",
		TokenURL:     tokenServer.URL,
		ClientID:     "bad-id",
		ClientSecret: "bad-secret",
	}

	code := c.Do(context.Background(), "GET", "/rest/api/3/test", nil, nil)
	// ApplyAuth now propagates OAuth2 errors: the request should fail
	// with ExitAuth (2) and never reach the API server.
	if code != 2 {
		t.Fatalf("expected exit 2 (auth), got %d; stderr=%s", code, stderr.String())
	}

	stderrStr := stderr.String()
	if !strings.Contains(stderrStr, "auth_error") {
		t.Errorf("expected auth_error in stderr, got: %s", stderrStr)
	}
	// Stdout should be empty — no API request was made.
	if stdout.Len() != 0 {
		t.Errorf("expected empty stdout, got: %s", stdout.String())
	}
}

func TestOAuth2_HTMLErrorResponse(t *testing.T) {
	// OAuth2 token endpoint returns 500 with HTML body (not JSON).
	tokenServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		w.WriteHeader(http.StatusInternalServerError)
		fmt.Fprintln(w, `<html><body>Internal Server Error</body></html>`)
	}))
	defer tokenServer.Close()

	apiServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		authHeader := r.Header.Get("Authorization")
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, `{"auth_header":%q}`, authHeader)
	}))
	defer apiServer.Close()

	var stdout, stderr bytes.Buffer
	c := newTestClient(apiServer.URL, &stdout, &stderr)
	c.Auth = config.AuthConfig{
		Type:         "oauth2",
		TokenURL:     tokenServer.URL,
		ClientID:     "id",
		ClientSecret: "secret",
	}

	code := c.Do(context.Background(), "GET", "/rest/api/3/test", nil, nil)
	// ApplyAuth now propagates OAuth2 errors: the request should fail
	// with ExitAuth (2) and never reach the API server.
	if code != 2 {
		t.Fatalf("expected exit 2 (auth), got %d; stderr=%s", code, stderr.String())
	}

	stderrStr := stderr.String()
	if !strings.Contains(stderrStr, "auth_error") {
		t.Errorf("expected auth_error in stderr, got: %s", stderrStr)
	}
	if stdout.Len() != 0 {
		t.Errorf("expected empty stdout, got: %s", stdout.String())
	}
}

// Bug #45: OAuth2 token fetch errors must be reported to stderr, not silently swallowed.
func TestOAuth2_ErrorReportedToStderr(t *testing.T) {
	// OAuth2 token endpoint returns an error.
	tokenServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		fmt.Fprintln(w, `{"error":"invalid_client"}`)
	}))
	defer tokenServer.Close()

	apiServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintln(w, `{"ok":true}`)
	}))
	defer apiServer.Close()

	var stdout, stderr bytes.Buffer
	c := newTestClient(apiServer.URL, &stdout, &stderr)
	c.Auth = config.AuthConfig{
		Type:         "oauth2",
		TokenURL:     tokenServer.URL,
		ClientID:     "bad-id",
		ClientSecret: "bad-secret",
	}

	_ = c.Do(context.Background(), "GET", "/rest/api/3/test", nil, nil)

	// Before the fix, stderr would be empty (error silently swallowed).
	// After the fix, stderr should contain the auth error.
	stderrStr := stderr.String()
	if !strings.Contains(stderrStr, "auth_error") {
		t.Errorf("expected auth_error in stderr, got: %q", stderrStr)
	}
	if !strings.Contains(stderrStr, "oauth2") {
		t.Errorf("expected 'oauth2' mention in stderr error, got: %q", stderrStr)
	}
}

// Bug #45: OAuth2 with empty TokenURL must report a clear error.
func TestOAuth2_EmptyTokenURLReportsError(t *testing.T) {
	apiServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintln(w, `{"ok":true}`)
	}))
	defer apiServer.Close()

	var stdout, stderr bytes.Buffer
	c := newTestClient(apiServer.URL, &stdout, &stderr)
	c.Auth = config.AuthConfig{
		Type: "oauth2",
		// Deliberately empty ClientID, ClientSecret, TokenURL
	}

	_ = c.Do(context.Background(), "GET", "/rest/api/3/test", nil, nil)

	stderrStr := stderr.String()
	if stderrStr == "" {
		t.Error("expected stderr output for oauth2 with empty config, got empty string")
	}
	if !strings.Contains(stderrStr, "auth_error") {
		t.Errorf("expected auth_error type in stderr, got: %q", stderrStr)
	}
}

// Test: DryRun respects --jq filter.
func TestDo_DryRunRespectsJQ(t *testing.T) {
	var stdout, stderr bytes.Buffer
	c := &client.Client{
		BaseURL:    "https://example.atlassian.net",
		HTTPClient: &http.Client{},
		Stdout:     &stdout,
		Stderr:     &stderr,
		DryRun:     true,
		JQFilter:   ".method",
	}

	code := c.Do(context.Background(), "GET", "/rest/api/3/issue/TEST-1", nil, nil)
	if code != 0 {
		t.Fatalf("expected exit code 0, got %d; stderr=%s", code, stderr.String())
	}
	got := strings.TrimSpace(stdout.String())
	if got != `"GET"` {
		t.Errorf("expected jq-filtered output %q, got %q", `"GET"`, got)
	}
}

// Test: DryRun respects --pretty flag.
func TestDo_DryRunRespectsPretty(t *testing.T) {
	var stdout, stderr bytes.Buffer
	c := &client.Client{
		BaseURL:    "https://example.atlassian.net",
		HTTPClient: &http.Client{},
		Stdout:     &stdout,
		Stderr:     &stderr,
		DryRun:     true,
		Pretty:     true,
	}

	code := c.Do(context.Background(), "GET", "/rest/api/3/issue/TEST-1", nil, nil)
	if code != 0 {
		t.Fatalf("expected exit code 0, got %d; stderr=%s", code, stderr.String())
	}
	got := stdout.String()
	if !strings.Contains(got, "\n") || !strings.Contains(got, "  ") {
		t.Errorf("expected pretty-printed output with indentation, got: %s", got)
	}
}

// Test: DryRun with POST body respects --jq.
func TestDo_DryRunWithBodyRespectsJQ(t *testing.T) {
	var stdout, stderr bytes.Buffer
	c := &client.Client{
		BaseURL:    "https://example.atlassian.net",
		HTTPClient: &http.Client{},
		Stdout:     &stdout,
		Stderr:     &stderr,
		DryRun:     true,
		JQFilter:   ".body.fields.summary",
	}

	body := strings.NewReader(`{"fields":{"summary":"Test issue"}}`)
	code := c.Do(context.Background(), "POST", "/rest/api/3/issue", nil, body)
	if code != 0 {
		t.Fatalf("expected exit code 0, got %d; stderr=%s", code, stderr.String())
	}
	got := strings.TrimSpace(stdout.String())
	if got != `"Test issue"` {
		t.Errorf("expected jq-filtered body output %q, got %q", `"Test issue"`, got)
	}
}

// --- Bug #55: Pagination startAt offset should account for initial startAt ---

func TestPagination_StartAtOffset_NonZeroFirstPage(t *testing.T) {
	// Server returns paginated results where the first page starts at offset 2.
	// This simulates a user passing --startAt 2. The bug was that subsequent
	// pages would use len(allValues) instead of firstPage.StartAt + len(allValues),
	// causing duplicate fetches.
	requestCount := 0
	requestedStartAts := []string{}
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestCount++
		startAt := r.URL.Query().Get("startAt")
		requestedStartAts = append(requestedStartAts, startAt)
		w.Header().Set("Content-Type", "application/json")

		switch startAt {
		case "", "0":
			// Should not be requested when first page starts at 2.
			_, _ = w.Write([]byte(`{"startAt":0,"maxResults":2,"total":6,"isLast":false,"values":[{"id":"A"},{"id":"B"}]}`))
		case "2":
			_, _ = w.Write([]byte(`{"startAt":2,"maxResults":2,"total":6,"isLast":false,"values":[{"id":"C"},{"id":"D"}]}`))
		case "4":
			_, _ = w.Write([]byte(`{"startAt":4,"maxResults":2,"total":6,"isLast":true,"values":[{"id":"E"},{"id":"F"}]}`))
		default:
			t.Errorf("unexpected startAt=%q", startAt)
			_, _ = w.Write([]byte(`{"startAt":0,"maxResults":2,"total":6,"isLast":true,"values":[]}`))
		}
	}))
	defer ts.Close()

	var stdout, stderr bytes.Buffer
	c := &client.Client{
		BaseURL:    ts.URL,
		Auth:       config.AuthConfig{},
		HTTPClient: &http.Client{},
		Stdout:     &stdout,
		Stderr:     &stderr,
		Paginate:   true,
	}

	// Simulate requesting with startAt=2 in the query params.
	q := make(map[string][]string)
	q["startAt"] = []string{"2"}
	code := c.Do(context.Background(), "GET", "/rest/api/3/test", q, nil)
	if code != 0 {
		t.Fatalf("expected exit code 0, got %d; stderr=%s", code, stderr.String())
	}

	// Parse the merged result.
	var envelope struct {
		Values []map[string]string `json:"values"`
		Total  json.RawMessage     `json:"total"`
		IsLast bool                `json:"isLast"`
	}
	if err := json.Unmarshal([]byte(strings.TrimSpace(stdout.String())), &envelope); err != nil {
		t.Fatalf("failed to parse output: %v\nstdout: %s", err, stdout.String())
	}

	// Should have fetched pages starting at 2 and 4.
	if len(envelope.Values) != 4 {
		t.Errorf("expected 4 merged values (from startAt=2 and startAt=4), got %d", len(envelope.Values))
	}

	// Verify the IDs are correct (no duplicates from re-fetching startAt=2).
	ids := make([]string, len(envelope.Values))
	for i, v := range envelope.Values {
		ids[i] = v["id"]
	}
	expectedIDs := []string{"C", "D", "E", "F"}
	for i, expected := range expectedIDs {
		if i >= len(ids) || ids[i] != expected {
			t.Errorf("value[%d]: expected id=%q, got ids=%v", i, expected, ids)
			break
		}
	}

	// The server should have been called exactly twice (startAt=2, startAt=4).
	if requestCount != 2 {
		t.Errorf("expected 2 server requests, got %d (startAts: %v)", requestCount, requestedStartAts)
	}
}

func TestPagination_StartAtZero_StillWorks(t *testing.T) {
	// Verify that normal pagination (starting at 0) is not broken.
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		startAt := r.URL.Query().Get("startAt")
		if startAt == "" || startAt == "0" {
			_, _ = w.Write([]byte(`{"startAt":0,"maxResults":2,"total":3,"isLast":false,"values":[{"id":"1"},{"id":"2"}]}`))
		} else {
			_, _ = w.Write([]byte(`{"startAt":2,"maxResults":2,"total":3,"isLast":true,"values":[{"id":"3"}]}`))
		}
	}))
	defer ts.Close()

	var stdout, stderr bytes.Buffer
	c := &client.Client{
		BaseURL:    ts.URL,
		Auth:       config.AuthConfig{},
		HTTPClient: &http.Client{},
		Stdout:     &stdout,
		Stderr:     &stderr,
		Paginate:   true,
	}

	code := c.Do(context.Background(), "GET", "/rest/api/3/test", nil, nil)
	if code != 0 {
		t.Fatalf("expected exit code 0, got %d; stderr=%s", code, stderr.String())
	}

	var envelope struct {
		Values []map[string]string `json:"values"`
	}
	if err := json.Unmarshal([]byte(strings.TrimSpace(stdout.String())), &envelope); err != nil {
		t.Fatalf("failed to parse output: %v", err)
	}
	if len(envelope.Values) != 3 {
		t.Errorf("expected 3 merged values, got %d", len(envelope.Values))
	}
}
