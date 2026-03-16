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
	"github.com/sofq/jira-cli/internal/config"
	jrerrors "github.com/sofq/jira-cli/internal/errors"
)

func TestFetchJSONWithBody_SingleOutput(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	defer ts.Close()

	var stdout, stderr bytes.Buffer
	c := newTestClient(ts.URL, &stdout, &stderr)

	body, code := c.Fetch(t.Context(), "POST", "/test", strings.NewReader(`{}`))
	if code != jrerrors.ExitOK {
		t.Fatalf("expected exit 0, got %d; stderr=%s", code, stderr.String())
	}
	// c.Fetch should NOT write anything to stdout.
	if stdout.String() != "" {
		t.Errorf("c.Fetch should not write to stdout, got: %s", stdout.String())
	}
	// Body should be empty for 204.
	if len(body) != 0 {
		t.Errorf("expected empty body for 204, got: %s", string(body))
	}
}

func TestFetchJSON_VerboseLogging(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintln(w, `{"ok":true}`)
	}))
	defer ts.Close()

	var stdout, stderr bytes.Buffer
	c := newTestClient(ts.URL, &stdout, &stderr)
	c.Verbose = true

	_, code := c.Fetch(t.Context(), "GET", "/test", nil)
	if code != jrerrors.ExitOK {
		t.Fatalf("expected exit 0, got %d", code)
	}

	// Verbose mode should have logged request and response to stderr.
	if !strings.Contains(stderr.String(), `"type":"request"`) {
		t.Error("expected verbose request log in stderr")
	}
	if !strings.Contains(stderr.String(), `"type":"response"`) {
		t.Error("expected verbose response log in stderr")
	}
}

func TestFetchJSON_NoVerboseByDefault(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintln(w, `{"ok":true}`)
	}))
	defer ts.Close()

	var stdout, stderr bytes.Buffer
	c := newTestClient(ts.URL, &stdout, &stderr)

	_, code := c.Fetch(t.Context(), "GET", "/test", nil)
	if code != jrerrors.ExitOK {
		t.Fatalf("expected exit 0, got %d", code)
	}

	if stderr.String() != "" {
		t.Errorf("expected no verbose output by default, got: %s", stderr.String())
	}
}

func TestClientDo_PathWithoutLeadingSlash(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/rest/api/3/test" {
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprintln(w, `{"ok":true}`)
			return
		}
		t.Errorf("unexpected path: %s", r.URL.Path)
		w.WriteHeader(http.StatusNotFound)
	}))
	defer ts.Close()

	var stdout, stderr bytes.Buffer
	c := newTestClient(ts.URL, &stdout, &stderr)

	// Path without leading /.
	code := c.Do(t.Context(), "GET", "rest/api/3/test", nil, nil)
	if code != jrerrors.ExitOK {
		t.Fatalf("expected exit 0, got %d; stderr=%s", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), `"ok"`) {
		t.Errorf("expected ok in response, got: %s", stdout.String())
	}
}

func TestClientDo_PathWithLeadingSlash(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/rest/api/3/test" {
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprintln(w, `{"ok":true}`)
			return
		}
		t.Errorf("unexpected path: %s", r.URL.Path)
		w.WriteHeader(http.StatusNotFound)
	}))
	defer ts.Close()

	var stdout, stderr bytes.Buffer
	c := newTestClient(ts.URL, &stdout, &stderr)

	// Path with leading / should still work.
	code := c.Do(t.Context(), "GET", "/rest/api/3/test", nil, nil)
	if code != jrerrors.ExitOK {
		t.Fatalf("expected exit 0, got %d; stderr=%s", code, stderr.String())
	}
}

func TestClientDo_PathWithQueryParams(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify URL has both inline and additional params merged with &.
		if !strings.Contains(r.URL.RawQuery, "jql=project") {
			t.Errorf("missing inline param jql, got query: %s", r.URL.RawQuery)
		}
		if !strings.Contains(r.URL.RawQuery, "maxResults=1") {
			t.Errorf("missing added param maxResults, got query: %s", r.URL.RawQuery)
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintln(w, `{"ok":true}`)
	}))
	defer ts.Close()

	var stdout, stderr bytes.Buffer
	c := newTestClient(ts.URL, &stdout, &stderr)
	c.Paginate = false

	query := make(map[string][]string)
	query["maxResults"] = []string{"1"}
	// Path already has ?jql=... inline.
	code := c.Do(t.Context(), "GET", "/rest/api/3/search?jql=project%3DKAN", query, nil)
	if code != 0 {
		t.Fatalf("expected exit 0, got %d; stderr=%s", code, stderr.String())
	}
}

func TestFetchJSONWithBody_Error(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		fmt.Fprintln(w, `{"errorMessages":["forbidden"]}`)
	}))
	defer ts.Close()

	var stdout, stderr bytes.Buffer
	c := newTestClient(ts.URL, &stdout, &stderr)

	_, code := c.Fetch(t.Context(), "POST", "/test", strings.NewReader(`{}`))
	if code == jrerrors.ExitOK {
		t.Fatal("expected non-zero exit code for 403 response")
	}
	if !strings.Contains(stderr.String(), "forbidden") {
		t.Errorf("expected error message in stderr, got: %s", stderr.String())
	}
}

func TestFetchJSONWithBody_BadURL_WritesError(t *testing.T) {
	var stdout, stderr bytes.Buffer
	c := &client.Client{
		BaseURL:    "://invalid-url", // will cause http.NewRequestWithContext to fail
		Auth:       config.AuthConfig{Type: "basic", Username: "u", Token: "t"},
		HTTPClient: &http.Client{},
		Stdout:     &stdout,
		Stderr:     &stderr,
	}

	// Call batchTransition which uses fetchJSON -> fetchJSONWithBody.
	code := BatchTransitionForTest(context.Background(), c, "TEST-1", "Done")

	if code == 0 {
		t.Error("expected non-zero exit code for invalid URL")
	}

	errOutput := stderr.String()
	if errOutput == "" {
		t.Error("expected error JSON on stderr when request creation fails, got empty stderr")
	}
	if !strings.Contains(errOutput, "connection_error") && !strings.Contains(errOutput, "failed to create request") {
		t.Errorf("expected structured error on stderr, got: %s", errOutput)
	}
}

func TestFetchJSONWithBody_ReadError(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Hijack the connection to force a body read error by closing early.
		hj, ok := w.(http.Hijacker)
		if !ok {
			// Can't hijack — write a normal response and let the test verify
			// fetchJSONWithBody works without panicking.
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"ok":true}`))
			return
		}
		conn, bufrw, _ := hj.Hijack()
		// Write HTTP headers but then close immediately (incomplete body).
		_, _ = bufrw.WriteString("HTTP/1.1 200 OK\r\nContent-Length: 1000\r\n\r\npartial")
		_ = bufrw.Flush()
		_ = conn.Close()
	}))
	defer ts.Close()

	var stdout, stderr bytes.Buffer
	c := newTestClient(ts.URL, &stdout, &stderr)

	body, exitCode := c.Fetch(t.Context(), "GET", "/test", nil)

	// We expect either:
	// 1. A connection_error (if the read fails), or
	// 2. A success with partial body (if the client managed to read what was sent).
	// A read error should not silently return empty body with ExitOK.
	if exitCode == jrerrors.ExitOK && len(body) == 0 {
		t.Error("expected either non-zero exit code or non-empty body; got exit=0 body=empty")
	}
}

func TestDryRunPostNilBodyDoesNotHang(t *testing.T) {
	// Simulate what happens when a generated POST command runs in dry-run
	// mode without a body (e.g. non-terminal context where stdin is not available).
	var stdout, stderr bytes.Buffer
	c := &client.Client{
		BaseURL:    "https://example.atlassian.net",
		Auth:       config.AuthConfig{Type: "basic", Username: "u", Token: "t"},
		HTTPClient: &http.Client{},
		Stdout:     &stdout,
		Stderr:     &stderr,
		DryRun:     true,
	}

	// Call Do with nil body — this is what generated commands do in dry-run
	// when stdin is not available.
	code := c.Do(t.Context(), "POST", "/rest/api/3/issue", nil, nil)
	if code != 0 {
		t.Errorf("expected exit code 0, got %d; stderr: %s", code, stderr.String())
	}

	var result map[string]any
	if err := json.Unmarshal(stdout.Bytes(), &result); err != nil {
		t.Fatalf("invalid JSON output: %v; raw: %s", err, stdout.String())
	}
	if result["method"] != "POST" {
		t.Errorf("expected method POST in dry-run, got %v", result["method"])
	}
}

func TestOAuth2Failure_SingleError_ExitAuth(t *testing.T) {
	tokenServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		fmt.Fprintln(w, `{"error":"invalid_grant"}`)
	}))
	defer tokenServer.Close()

	apiHit := false
	apiServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		apiHit = true
		w.WriteHeader(http.StatusUnauthorized)
		fmt.Fprintln(w, `{"error":"not_authenticated"}`)
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

	code := c.Do(context.Background(), "GET", "/rest/api/3/myself", nil, nil)

	if code != jrerrors.ExitAuth {
		t.Errorf("expected exit code %d (auth), got %d", jrerrors.ExitAuth, code)
	}
	if apiHit {
		t.Error("API server should NOT have been hit when OAuth2 token fetch failed")
	}

	// Stderr should contain exactly one error (auth_error), not two.
	lines := strings.Split(strings.TrimSpace(stderr.String()), "\n")
	if len(lines) != 1 {
		t.Errorf("expected 1 error line on stderr, got %d: %s", len(lines), stderr.String())
	}
	if !strings.Contains(lines[0], "auth_error") {
		t.Errorf("expected auth_error in stderr, got: %s", lines[0])
	}
}

func TestApplyAuthDefaultsToBasicForUnknownType(t *testing.T) {
	// Even if an unknown type slips through (e.g. manually edited config),
	// ApplyAuth should default to basic auth rather than silently skipping.
	c := &client.Client{
		Auth: config.AuthConfig{Type: "unknown", Username: "user", Token: "tok"},
	}
	req, _ := http.NewRequest("GET", "http://example.com", nil)
	if err := c.ApplyAuth(req); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	auth := req.Header.Get("Authorization")
	if auth == "" {
		t.Fatal("expected Authorization header for unknown auth type (should default to basic), got empty")
	}
	if !strings.HasPrefix(auth, "Basic ") {
		t.Errorf("expected Basic auth header, got: %s", auth)
	}
}

func TestApplyAuth_OAuth2Error_ReturnsError(t *testing.T) {
	tokenServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		fmt.Fprintln(w, `{"error":"invalid_client"}`)
	}))
	defer tokenServer.Close()

	c := &client.Client{
		HTTPClient: &http.Client{},
		Auth: config.AuthConfig{
			Type:     "oauth2",
			TokenURL: tokenServer.URL,
		},
	}
	req, _ := http.NewRequest("GET", "http://example.com", nil)
	err := c.ApplyAuth(req)
	if err == nil {
		t.Fatal("expected error from ApplyAuth when OAuth2 token fetch fails")
	}
	if !strings.Contains(err.Error(), "oauth2 authentication failed") {
		t.Errorf("expected 'oauth2 authentication failed' in error, got: %s", err.Error())
	}
	// Authorization header must NOT be set.
	if req.Header.Get("Authorization") != "" {
		t.Error("Authorization header should not be set after OAuth2 failure")
	}
}

func TestApplyAuth_OAuth2EmptyToken_ReturnsError(t *testing.T) {
	tokenServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintln(w, `{"access_token":""}`)
	}))
	defer tokenServer.Close()

	c := &client.Client{
		HTTPClient: &http.Client{},
		Auth: config.AuthConfig{
			Type:         "oauth2",
			TokenURL:     tokenServer.URL,
			ClientID:     "id",
			ClientSecret: "secret",
		},
	}
	req, _ := http.NewRequest("GET", "http://example.com", nil)
	err := c.ApplyAuth(req)
	if err == nil {
		t.Fatal("expected error from ApplyAuth when OAuth2 returns empty token")
	}
	if !strings.Contains(err.Error(), "empty access_token") {
		t.Errorf("expected 'empty access_token' in error, got: %s", err.Error())
	}
}

func TestApplyAuth_OAuth2Success_SetsBearer(t *testing.T) {
	tokenServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintln(w, `{"access_token":"good-token-123"}`)
	}))
	defer tokenServer.Close()

	c := &client.Client{
		HTTPClient: &http.Client{},
		Auth: config.AuthConfig{
			Type:         "oauth2",
			TokenURL:     tokenServer.URL,
			ClientID:     "id",
			ClientSecret: "secret",
		},
	}
	req, _ := http.NewRequest("GET", "http://example.com", nil)
	if err := c.ApplyAuth(req); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	auth := req.Header.Get("Authorization")
	if auth != "Bearer good-token-123" {
		t.Errorf("expected 'Bearer good-token-123', got: %s", auth)
	}
}

func TestApplyAuth_BasicAndBearer_NoError(t *testing.T) {
	tests := []struct {
		authType string
		wantAuth string
	}{
		{"basic", "Basic"},
		{"bearer", "Bearer tok"},
	}
	for _, tc := range tests {
		t.Run(tc.authType, func(t *testing.T) {
			c := &client.Client{
				Auth: config.AuthConfig{Type: tc.authType, Username: "u", Token: "tok"},
			}
			req, _ := http.NewRequest("GET", "http://example.com", nil)
			if err := c.ApplyAuth(req); err != nil {
				t.Fatalf("unexpected error for %s: %v", tc.authType, err)
			}
			auth := req.Header.Get("Authorization")
			if !strings.HasPrefix(auth, tc.wantAuth) {
				t.Errorf("expected Authorization starting with %q, got %q", tc.wantAuth, auth)
			}
		})
	}
}

func TestFetchOAuth2Token_HTTPError(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		fmt.Fprintln(w, `{"error":"invalid_client"}`)
	}))
	defer ts.Close()

	c := &client.Client{
		Auth: config.AuthConfig{
			Type:         "oauth2",
			ClientID:     "id",
			ClientSecret: "secret",
			TokenURL:     ts.URL + "/token",
		},
		HTTPClient: &http.Client{},
	}

	req, _ := http.NewRequest("GET", "http://example.com", nil)
	err := c.ApplyAuth(req)
	if err == nil {
		t.Fatal("expected error for OAuth2 token fetch failure")
	}
	if !strings.Contains(err.Error(), "oauth2") {
		t.Errorf("expected oauth2 error, got: %v", err)
	}
}

func TestFetchOAuth2Token_InvalidJSON(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintln(w, `not valid json`)
	}))
	defer ts.Close()

	c := &client.Client{
		Auth: config.AuthConfig{
			Type:         "oauth2",
			ClientID:     "id",
			ClientSecret: "secret",
			TokenURL:     ts.URL + "/token",
		},
		HTTPClient: &http.Client{},
	}

	req, _ := http.NewRequest("GET", "http://example.com", nil)
	err := c.ApplyAuth(req)
	if err == nil {
		t.Fatal("expected error for invalid OAuth2 token JSON")
	}
	if !strings.Contains(err.Error(), "decode") {
		t.Errorf("expected decode error, got: %v", err)
	}
}

func TestClientDo_DeleteMethod(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "DELETE" {
			t.Errorf("expected DELETE, got %s", r.Method)
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer ts.Close()

	var stdout, stderr bytes.Buffer
	c := newTestClient(ts.URL, &stdout, &stderr)

	code := c.Do(t.Context(), "DELETE", "/test", nil, nil)
	if code != jrerrors.ExitOK {
		t.Fatalf("expected exit 0, got %d; stderr=%s", code, stderr.String())
	}
	// 204 No Content should produce {}.
	if !strings.Contains(stdout.String(), "{}") {
		t.Errorf("expected {} for 204, got: %s", stdout.String())
	}
}

func TestClientDo_EmptyPath(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintln(w, `{"ok":true}`)
	}))
	defer ts.Close()

	var stdout, stderr bytes.Buffer
	c := newTestClient(ts.URL, &stdout, &stderr)

	code := c.Do(t.Context(), "GET", "", nil, nil)
	if code != jrerrors.ExitOK {
		t.Fatalf("expected exit 0, got %d; stderr=%s", code, stderr.String())
	}
}

func TestClientDo_DryRunWithBody(t *testing.T) {
	var stdout, stderr bytes.Buffer
	c := newTestClient("http://should-not-be-called", &stdout, &stderr)
	c.DryRun = true

	code := c.Do(t.Context(), "POST", "/test", nil, strings.NewReader(`{"hello":"world"}`))
	if code != jrerrors.ExitOK {
		t.Fatalf("expected exit 0, got %d", code)
	}
	// Dry-run output should include the parsed body.
	output := stdout.String()
	if !strings.Contains(output, "hello") {
		t.Errorf("expected body in dry-run output, got: %s", output)
	}
}

func TestClientDo_DryRunNilBody(t *testing.T) {
	var stdout, stderr bytes.Buffer
	c := newTestClient("http://should-not-be-called", &stdout, &stderr)
	c.DryRun = true

	code := c.Do(t.Context(), "GET", "/test", nil, nil)
	if code != jrerrors.ExitOK {
		t.Fatalf("expected exit 0, got %d", code)
	}
	output := stdout.String()
	if !strings.Contains(output, `"method":"GET"`) {
		t.Errorf("expected method in dry-run output, got: %s", output)
	}
}

func TestPagination_TotalZeroWithValues(t *testing.T) {
	// Server returns total=0, no isLast field, but has values.
	// Without the fix, pagination would loop infinitely.
	callCount := 0
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		w.Header().Set("Content-Type", "application/json")
		if callCount == 1 {
			fmt.Fprintln(w, `{"startAt":0,"maxResults":2,"total":0,"values":[{"id":"1"},{"id":"2"}]}`)
		} else {
			// Should never be called — pagination should stop after first page.
			fmt.Fprintln(w, `{"startAt":2,"maxResults":2,"total":0,"values":[]}`)
		}
	}))
	defer ts.Close()

	var stdout, stderr bytes.Buffer
	c := newTestClient(ts.URL, &stdout, &stderr)

	code := c.Do(t.Context(), "GET", "/test", nil, nil)
	if code != jrerrors.ExitOK {
		t.Fatalf("expected exit 0, got %d; stderr=%s", code, stderr.String())
	}

	// Should only have fetched one page.
	if callCount != 1 {
		t.Errorf("expected 1 HTTP call (single page), got %d — infinite pagination loop!", callCount)
	}

	// Output should include the two values from the first (only) page.
	output := stdout.String()
	if !strings.Contains(output, `"id":"1"`) || !strings.Contains(output, `"id":"2"`) {
		t.Errorf("expected values in output, got: %s", output)
	}
}

func TestPagination_TotalZeroEmptyValues(t *testing.T) {
	// Server returns total=0, values=[] — should produce an empty result.
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintln(w, `{"startAt":0,"maxResults":50,"total":0,"values":[]}`)
	}))
	defer ts.Close()

	var stdout, stderr bytes.Buffer
	c := newTestClient(ts.URL, &stdout, &stderr)

	code := c.Do(t.Context(), "GET", "/test", nil, nil)
	if code != jrerrors.ExitOK {
		t.Fatalf("expected exit 0, got %d; stderr=%s", code, stderr.String())
	}
}

func TestFetchJSONWithBody_TrimSpaceOnError(t *testing.T) {
	// Server returns an error with trailing whitespace.
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		fmt.Fprintln(w, `{"errorMessages":["Issue not found"]}  `)
	}))
	defer ts.Close()

	var stdout, stderr bytes.Buffer
	c := newTestClient(ts.URL, &stdout, &stderr)

	_, code := FetchJSONWithBody(c, t.Context(), "GET", "/rest/api/3/issue/NOPE-1", nil)
	if code != jrerrors.ExitNotFound {
		t.Errorf("expected exit %d, got %d", jrerrors.ExitNotFound, code)
	}

	// The error JSON on stderr should be well-formed.
	errOutput := strings.TrimSpace(stderr.String())
	if !json.Valid([]byte(errOutput)) {
		t.Errorf("stderr not valid JSON: %s", errOutput)
	}
}

func TestFetchOAuth2Token_EmptyAccessToken(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintln(w, `{"access_token":""}`)
	}))
	defer ts.Close()

	c := &client.Client{
		Auth: config.AuthConfig{
			Type:         "oauth2",
			ClientID:     "id",
			ClientSecret: "secret",
			TokenURL:     ts.URL + "/token",
		},
		HTTPClient: &http.Client{},
	}

	req, _ := http.NewRequest("GET", "http://example.com", nil)
	err := c.ApplyAuth(req)
	if err == nil {
		t.Fatal("expected error for empty access_token")
	}
	if !strings.Contains(err.Error(), "empty") {
		t.Errorf("expected 'empty' in error, got: %v", err)
	}
}
