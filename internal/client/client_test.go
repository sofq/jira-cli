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

// testEnv bundles the common test fixtures: a test server, client, and captured stdout/stderr.
type testEnv struct {
	Server *httptest.Server
	Client *client.Client
	Stdout *bytes.Buffer
	Stderr *bytes.Buffer
}

// Close shuts down the test server.
func (te *testEnv) Close() { te.Server.Close() }

// ResetBuffers replaces stdout/stderr with fresh buffers (useful for multi-call cache tests).
func (te *testEnv) ResetBuffers() {
	te.Stdout = &bytes.Buffer{}
	te.Stderr = &bytes.Buffer{}
	te.Client.Stdout = te.Stdout
	te.Client.Stderr = te.Stderr
}

// newTestEnv creates a testEnv wired to the given handler.
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

// jsonHandler returns a handler that serves a static JSON body with 200 OK.
func jsonHandler(body string) http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintln(w, body)
	}
}

// --- Basic request/response ---

func TestDo_GetReturnsJSON(t *testing.T) {
	te := newTestEnv(jsonHandler(`{"key":"value"}`))
	defer te.Close()

	code := te.Client.Do(context.Background(), "GET", "/rest/api/3/test", nil, nil)
	if code != 0 {
		t.Fatalf("expected exit code 0, got %d; stderr=%s", code, te.Stderr.String())
	}
	if !strings.Contains(te.Stdout.String(), `"key"`) {
		t.Errorf("expected JSON in stdout, got: %s", te.Stdout.String())
	}
}

func TestDo_PostWithBody(t *testing.T) {
	var gotContentType, gotBody string
	te := newTestEnv(func(w http.ResponseWriter, r *http.Request) {
		gotContentType = r.Header.Get("Content-Type")
		b, _ := io.ReadAll(r.Body)
		gotBody = string(b)
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintln(w, `{"id":"NEW-1"}`)
	})
	defer te.Close()

	body := strings.NewReader(`{"summary":"test issue"}`)
	code := te.Client.Do(context.Background(), "POST", "/rest/api/3/issue", nil, body)
	if code != 0 {
		t.Fatalf("unexpected exit code %d; stderr=%s", code, te.Stderr.String())
	}
	if gotContentType != "application/json" {
		t.Errorf("expected Content-Type application/json, got %q", gotContentType)
	}
	if !strings.Contains(gotBody, "summary") {
		t.Errorf("expected body to contain 'summary', got %q", gotBody)
	}
}

func TestDo_PatchWithBody(t *testing.T) {
	var gotMethod, gotContentType string
	te := newTestEnv(func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		gotContentType = r.Header.Get("Content-Type")
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintln(w, `{"updated":true}`)
	})
	defer te.Close()

	body := strings.NewReader(`{"fields":{"summary":"Updated summary"}}`)
	code := te.Client.Do(context.Background(), "PATCH", "/rest/api/3/issue/PROJ-1", nil, body)
	if code != 0 {
		t.Fatalf("expected exit code 0, got %d; stderr=%s", code, te.Stderr.String())
	}
	if gotMethod != "PATCH" {
		t.Errorf("expected method PATCH, got %q", gotMethod)
	}
	if gotContentType != "application/json" {
		t.Errorf("expected Content-Type application/json, got %q", gotContentType)
	}
}

func TestDo_DeleteReturnsJSON(t *testing.T) {
	te := newTestEnv(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodDelete {
			t.Errorf("expected DELETE method, got %s", r.Method)
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintln(w, `{"deleted":true,"id":"PROJ-42"}`)
	})
	defer te.Close()

	code := te.Client.Do(context.Background(), "DELETE", "/rest/api/3/issue/PROJ-42", nil, nil)
	if code != 0 {
		t.Fatalf("expected exit code 0, got %d; stderr=%s", code, te.Stderr.String())
	}
	var result map[string]any
	if err := json.Unmarshal(te.Stdout.Bytes(), &result); err != nil {
		t.Fatalf("stdout is not valid JSON: %s; err=%v", te.Stdout.String(), err)
	}
	if result["deleted"] != true {
		t.Errorf("expected deleted=true, got %v", result["deleted"])
	}
}

func TestDo_HeadRequest(t *testing.T) {
	te := newTestEnv(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodHead {
			t.Errorf("expected HEAD method, got %s", r.Method)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
	})
	defer te.Close()

	code := te.Client.Do(context.Background(), "HEAD", "/rest/api/3/issue/PROJ-1", nil, nil)
	if code != 0 {
		t.Fatalf("expected exit code 0 for HEAD request, got %d; stderr=%s", code, te.Stderr.String())
	}
	if te.Stderr.Len() != 0 {
		t.Errorf("expected empty stderr for HEAD request, got: %s", te.Stderr.String())
	}
}

// --- Auth ---

func TestDo_BasicAuth(t *testing.T) {
	var gotAuth string
	te := newTestEnv(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintln(w, `{}`)
	})
	defer te.Close()
	te.Client.Auth = config.AuthConfig{Type: "basic", Username: "user", Token: "secret"}

	code := te.Client.Do(context.Background(), "GET", "/path", nil, nil)
	if code != 0 {
		t.Fatalf("unexpected exit code %d; stderr=%s", code, te.Stderr.String())
	}
	if !strings.HasPrefix(gotAuth, "Basic ") {
		t.Errorf("expected Basic auth header, got: %q", gotAuth)
	}
}

func TestDo_BearerAuth(t *testing.T) {
	var gotAuth string
	te := newTestEnv(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintln(w, `{}`)
	})
	defer te.Close()
	te.Client.Auth = config.AuthConfig{Type: "bearer", Token: "mytoken"}

	code := te.Client.Do(context.Background(), "GET", "/path", nil, nil)
	if code != 0 {
		t.Fatalf("unexpected exit code %d; stderr=%s", code, te.Stderr.String())
	}
	if gotAuth != "Bearer mytoken" {
		t.Errorf("expected 'Bearer mytoken', got: %q", gotAuth)
	}
}

func TestDo_OAuth2Auth(t *testing.T) {
	tokenServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintln(w, `{"access_token":"oauth-token-123","token_type":"Bearer","expires_in":3600}`)
	}))
	defer tokenServer.Close()

	var gotAuth string
	te := newTestEnv(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintln(w, `{}`)
	})
	defer te.Close()
	te.Client.Auth = config.AuthConfig{
		Type:         "oauth2",
		ClientID:     "my-client-id",
		ClientSecret: "my-client-secret",
		TokenURL:     tokenServer.URL,
	}

	code := te.Client.Do(context.Background(), "GET", "/path", nil, nil)
	if code != 0 {
		t.Fatalf("unexpected exit code %d; stderr=%s", code, te.Stderr.String())
	}
	if gotAuth != "Bearer oauth-token-123" {
		t.Errorf("expected 'Bearer oauth-token-123', got: %q", gotAuth)
	}
}

// --- OAuth2 error handling ---

func TestOAuth2_Errors(t *testing.T) {
	tests := []struct {
		name       string
		tokenCode  int
		tokenCT    string
		tokenBody  string
		authCfg    config.AuthConfig
		wantStderr string
	}{
		{
			name:      "401 from token endpoint",
			tokenCode: http.StatusUnauthorized,
			tokenCT:   "application/json",
			tokenBody: `{"error":"invalid_client","error_description":"bad credentials"}`,
			authCfg: config.AuthConfig{
				Type: "oauth2", ClientID: "bad-id", ClientSecret: "bad-secret",
			},
			wantStderr: "auth_error",
		},
		{
			name:      "500 HTML from token endpoint",
			tokenCode: http.StatusInternalServerError,
			tokenCT:   "text/html",
			tokenBody: `<html><body>Internal Server Error</body></html>`,
			authCfg: config.AuthConfig{
				Type: "oauth2", ClientID: "id", ClientSecret: "secret",
			},
			wantStderr: "auth_error",
		},
		{
			name:      "empty TokenURL",
			tokenCode: 0, // not used
			authCfg: config.AuthConfig{
				Type: "oauth2",
			},
			wantStderr: "auth_error",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.tokenCode != 0 {
				tokenServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
					w.Header().Set("Content-Type", tt.tokenCT)
					w.WriteHeader(tt.tokenCode)
					fmt.Fprintln(w, tt.tokenBody)
				}))
				defer tokenServer.Close()
				tt.authCfg.TokenURL = tokenServer.URL
			}

			apiServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				fmt.Fprintln(w, `{"ok":true}`)
			}))
			defer apiServer.Close()

			te := newTestEnv(jsonHandler(`{"ok":true}`))
			defer te.Close()
			te.Client.BaseURL = apiServer.URL
			te.Client.Auth = tt.authCfg

			code := te.Client.Do(context.Background(), "GET", "/rest/api/3/test", nil, nil)
			if code != 2 {
				t.Fatalf("expected exit 2 (auth), got %d; stderr=%s", code, te.Stderr.String())
			}
			if !strings.Contains(te.Stderr.String(), tt.wantStderr) {
				t.Errorf("expected %q in stderr, got: %s", tt.wantStderr, te.Stderr.String())
			}
			if te.Stdout.Len() != 0 {
				t.Errorf("expected empty stdout, got: %s", te.Stdout.String())
			}
		})
	}
}

// --- HTTP error codes (table-driven) ---

func TestDo_HTTPErrors(t *testing.T) {
	tests := []struct {
		name      string
		status    int
		body      string
		wantCode  int
		wantType  string
		wantHint  bool
		extraBody map[string]any // extra fields to check in stderr JSON
	}{
		{
			name: "401 Unauthorized", status: http.StatusUnauthorized,
			body: `{"message":"Unauthorized"}`, wantCode: 2, wantType: "auth_failed", wantHint: true,
		},
		{
			name: "403 Forbidden", status: http.StatusForbidden,
			body: `{"message":"Forbidden"}`, wantCode: 2, wantType: "auth_failed", wantHint: true,
		},
		{
			name: "404 Not Found", status: http.StatusNotFound,
			body: "not found", wantCode: 3, wantType: "not_found",
		},
		{
			name: "409 Conflict", status: http.StatusConflict,
			body: `{"errorMessages":["Issue already exists"]}`, wantCode: 6, wantType: "conflict",
		},
		{
			name: "429 Rate Limited", status: 429,
			body: `{"message":"rate limited"}`, wantCode: 5, wantType: "rate_limited",
			extraBody: map[string]any{"retry_after": float64(60)},
		},
		{
			name: "500 Server Error", status: http.StatusInternalServerError,
			body: `{"errorMessages":["Internal Server Error"]}`, wantCode: 7, wantType: "server_error",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			te := newTestEnv(func(w http.ResponseWriter, _ *http.Request) {
				if tt.status == 429 {
					w.Header().Set("Retry-After", "60")
				}
				w.WriteHeader(tt.status)
				fmt.Fprintln(w, tt.body)
			})
			defer te.Close()

			code := te.Client.Do(context.Background(), "GET", "/path", nil, nil)
			if code != tt.wantCode {
				t.Fatalf("expected exit code %d, got %d; stderr=%s", tt.wantCode, code, te.Stderr.String())
			}

			var errObj map[string]any
			if err := json.Unmarshal(te.Stderr.Bytes(), &errObj); err != nil {
				t.Fatalf("stderr is not valid JSON: %s; err=%v", te.Stderr.String(), err)
			}
			if errObj["error_type"] != tt.wantType {
				t.Errorf("expected error_type %q, got: %v", tt.wantType, errObj["error_type"])
			}
			if int(errObj["status"].(float64)) != tt.status {
				t.Errorf("expected status %d, got: %v", tt.status, errObj["status"])
			}
			if tt.wantHint {
				if hint, _ := errObj["hint"].(string); hint == "" {
					t.Error("expected non-empty hint")
				}
			}
			for k, v := range tt.extraBody {
				if errObj[k] != v {
					t.Errorf("expected %s=%v, got %v", k, v, errObj[k])
				}
			}
		})
	}
}

func TestDo_HTMLErrorSanitized(t *testing.T) {
	te := newTestEnv(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		w.WriteHeader(http.StatusServiceUnavailable)
		fmt.Fprintln(w, `<!DOCTYPE html><html><body><h1>Service Unavailable</h1></body></html>`)
	})
	defer te.Close()

	code := te.Client.Do(context.Background(), "GET", "/rest/api/3/test", nil, nil)
	if code == 0 {
		t.Fatal("expected non-zero exit code for 503 response")
	}

	var errObj map[string]any
	if err := json.Unmarshal(te.Stderr.Bytes(), &errObj); err != nil {
		t.Fatalf("stderr is not valid JSON: %s; err=%v", te.Stderr.String(), err)
	}
	msg, _ := errObj["message"].(string)
	if strings.Contains(msg, "<html") || strings.Contains(msg, "<!DOCTYPE") {
		t.Errorf("expected sanitized message (no raw HTML), got: %q", msg)
	}
}

func TestDo_ConnectionError(t *testing.T) {
	var stdout, stderr bytes.Buffer
	c := &client.Client{
		BaseURL:    "http://127.0.0.1:0",
		Auth:       config.AuthConfig{},
		HTTPClient: &http.Client{},
		Stdout:     &stdout,
		Stderr:     &stderr,
	}

	code := c.Do(context.Background(), "GET", "/rest/api/3/test", nil, nil)
	if code != 1 {
		t.Fatalf("expected exit code 1 (error), got %d", code)
	}
	var errObj map[string]any
	if err := json.Unmarshal(stderr.Bytes(), &errObj); err != nil {
		t.Fatalf("stderr is not valid JSON: %s; err=%v", stderr.String(), err)
	}
	if errObj["error_type"] != "connection_error" {
		t.Errorf("expected error_type 'connection_error', got: %v", errObj["error_type"])
	}
}

// --- Empty / 204 responses ---

func TestDo_204ReturnsEmptyJSON(t *testing.T) {
	te := newTestEnv(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})
	defer te.Close()

	code := te.Client.Do(context.Background(), "PUT", "/rest/api/3/issue/TEST-1", nil, strings.NewReader(`{}`))
	if code != 0 {
		t.Fatalf("expected exit code 0, got %d; stderr=%s", code, te.Stderr.String())
	}
	if out := strings.TrimSpace(te.Stdout.String()); out != "{}" {
		t.Errorf("expected '{}' for 204 response, got %q", out)
	}
}

func TestDo_204WithJQFilter(t *testing.T) {
	te := newTestEnv(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})
	defer te.Close()
	te.Client.JQFilter = `.status // "ok"`

	code := te.Client.Do(context.Background(), "PUT", "/path", nil, strings.NewReader(`{}`))
	if code != 0 {
		t.Fatalf("expected exit code 0, got %d; stderr=%s", code, te.Stderr.String())
	}
	if out := strings.TrimSpace(te.Stdout.String()); out != `"ok"` {
		t.Errorf("expected jq result %q, got %q", `"ok"`, out)
	}
}

func TestDo_EmptyBodyReturnsEmptyJSON(t *testing.T) {
	te := newTestEnv(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	defer te.Close()

	code := te.Client.Do(context.Background(), "DELETE", "/path", nil, nil)
	if code != 0 {
		t.Fatalf("expected exit code 0, got %d; stderr=%s", code, te.Stderr.String())
	}
	if out := strings.TrimSpace(te.Stdout.String()); out != "{}" {
		t.Errorf("expected '{}' for empty response, got %q", out)
	}
}

// --- JQ filter ---

func TestDo_JQFilter(t *testing.T) {
	tests := []struct {
		name     string
		response string
		filter   string
		want     string
	}{
		{
			name: "simple field", response: `{"name":"jira","version":3}`,
			filter: ".name", want: `"jira"`,
		},
		{
			name: "array construction", response: `{"items":[{"id":1},{"id":2},{"id":3}]}`,
			filter: "[.items[].id]", want: "[1,2,3]",
		},
		{
			name: "object construction", response: `{"key":"PROJ-7","name":"Test Project","extra":"ignored"}`,
			filter: `{key: .key, name: .name}`, want: `{"key":"PROJ-7","name":"Test Project"}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			te := newTestEnv(jsonHandler(tt.response))
			defer te.Close()
			te.Client.JQFilter = tt.filter

			code := te.Client.Do(context.Background(), "GET", "/path", nil, nil)
			if code != 0 {
				t.Fatalf("unexpected exit code %d; stderr=%s", code, te.Stderr.String())
			}

			got := strings.TrimSpace(te.Stdout.String())
			// Normalize for comparison: compact both expected and actual JSON.
			var wantBuf, gotBuf bytes.Buffer
			if err := json.Compact(&wantBuf, []byte(tt.want)); err == nil {
				if err := json.Compact(&gotBuf, []byte(got)); err == nil {
					if wantBuf.String() != gotBuf.String() {
						t.Errorf("expected %s, got %s", wantBuf.String(), gotBuf.String())
					}
					return
				}
			}
			// Fallback: raw string comparison.
			if got != tt.want {
				t.Errorf("expected %q, got %q", tt.want, got)
			}
		})
	}
}

func TestDo_JQInvalidFilter(t *testing.T) {
	te := newTestEnv(jsonHandler(`{"key":"PROJ-1"}`))
	defer te.Close()
	te.Client.JQFilter = ".[[[invalid jq"

	code := te.Client.Do(context.Background(), "GET", "/path", nil, nil)
	if code != 4 {
		t.Fatalf("expected exit code 4 (jq_error), got %d; stderr=%s", code, te.Stderr.String())
	}
	var errObj map[string]any
	if err := json.Unmarshal(te.Stderr.Bytes(), &errObj); err != nil {
		t.Fatalf("stderr is not valid JSON: %s; err=%v", te.Stderr.String(), err)
	}
	if errObj["error_type"] != "jq_error" {
		t.Errorf("expected error_type 'jq_error', got: %v", errObj["error_type"])
	}
}

func TestDo_EmptyJQFilterPassthrough(t *testing.T) {
	te := newTestEnv(jsonHandler(`{"key":"PROJ-1","summary":"No filter applied"}`))
	defer te.Close()
	te.Client.JQFilter = ""

	code := te.Client.Do(context.Background(), "GET", "/rest/api/3/issue/PROJ-1", nil, nil)
	if code != 0 {
		t.Fatalf("expected exit code 0, got %d; stderr=%s", code, te.Stderr.String())
	}
	if te.Stderr.Len() != 0 {
		t.Errorf("expected empty stderr, got: %s", te.Stderr.String())
	}
	var result map[string]any
	if err := json.Unmarshal(te.Stdout.Bytes(), &result); err != nil {
		t.Fatalf("stdout is not valid JSON: %s; err=%v", te.Stdout.String(), err)
	}
	if result["key"] != "PROJ-1" {
		t.Errorf("expected key=PROJ-1, got %v", result["key"])
	}
}

// --- Fields query param ---

func TestDo_FieldsOnGet(t *testing.T) {
	var gotFields string
	te := newTestEnv(func(w http.ResponseWriter, r *http.Request) {
		gotFields = r.URL.Query().Get("fields")
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintln(w, `{"key":"PROJ-1"}`)
	})
	defer te.Close()
	te.Client.Fields = "key,summary,status"

	code := te.Client.Do(context.Background(), "GET", "/rest/api/3/issue/PROJ-1", nil, nil)
	if code != 0 {
		t.Fatalf("expected exit code 0, got %d; stderr=%s", code, te.Stderr.String())
	}
	if gotFields != "key,summary,status" {
		t.Errorf("expected fields=key,summary,status, got %q", gotFields)
	}
}

func TestDo_FieldsIgnoredOnPost(t *testing.T) {
	var gotQuery string
	te := newTestEnv(func(w http.ResponseWriter, r *http.Request) {
		gotQuery = r.URL.RawQuery
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintln(w, `{"id":"1"}`)
	})
	defer te.Close()
	te.Client.Fields = "key,summary"

	code := te.Client.Do(context.Background(), "POST", "/rest/api/3/issue", nil, strings.NewReader(`{}`))
	if code != 0 {
		t.Fatalf("expected exit code 0, got %d", code)
	}
	if strings.Contains(gotQuery, "fields=") {
		t.Errorf("fields should not be applied to POST, got query: %s", gotQuery)
	}
}

// --- Pretty print ---

func TestDo_PrettyPrint(t *testing.T) {
	te := newTestEnv(jsonHandler(`{"key":"PROJ-1","summary":"Test issue"}`))
	defer te.Close()
	te.Client.Pretty = true

	code := te.Client.Do(context.Background(), "GET", "/rest/api/3/issue/PROJ-1", nil, nil)
	if code != 0 {
		t.Fatalf("expected exit code 0, got %d; stderr=%s", code, te.Stderr.String())
	}
	out := te.Stdout.String()
	if !strings.Contains(out, "\n") {
		t.Error("expected pretty-printed output with newlines")
	}
	if !strings.Contains(out, "  ") {
		t.Error("expected indentation in pretty-printed output")
	}
	var result map[string]any
	if err := json.Unmarshal([]byte(strings.TrimSpace(out)), &result); err != nil {
		t.Fatalf("pretty output is not valid JSON: err=%v\noutput: %s", err, out)
	}
	if result["key"] != "PROJ-1" {
		t.Errorf("expected key=PROJ-1, got %v", result["key"])
	}
}

// --- Verbose ---

func TestDo_Verbose(t *testing.T) {
	te := newTestEnv(jsonHandler(`{"ok":true}`))
	defer te.Close()
	te.Client.Verbose = true

	code := te.Client.Do(context.Background(), "GET", "/rest/api/3/test", nil, nil)
	if code != 0 {
		t.Fatalf("expected exit code 0, got %d", code)
	}

	lines := strings.Split(strings.TrimSpace(te.Stderr.String()), "\n")
	if len(lines) < 2 {
		t.Fatalf("expected at least 2 lines in stderr (request + response), got %d: %s", len(lines), te.Stderr.String())
	}

	var reqLog map[string]any
	if err := json.Unmarshal([]byte(lines[0]), &reqLog); err != nil {
		t.Fatalf("request log line not valid JSON: %s", lines[0])
	}
	if reqLog["type"] != "request" {
		t.Errorf("expected first log type=request, got %v", reqLog["type"])
	}
	if reqLog["method"] != "GET" {
		t.Errorf("expected method=GET in request log, got %v", reqLog["method"])
	}

	var respLog map[string]any
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

// --- DryRun ---

func TestDo_DryRun(t *testing.T) {
	t.Run("GET outputs request JSON without executing", func(t *testing.T) {
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
			t.Fatalf("expected exit code 0, got %d; stderr=%s", code, stderr.String())
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
	})

	t.Run("includes query params", func(t *testing.T) {
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
			t.Fatalf("expected exit code 0, got %d; stderr=%s", code, stderr.String())
		}
		var obj map[string]string
		if err := json.Unmarshal(stdout.Bytes(), &obj); err != nil {
			t.Fatalf("stdout is not valid JSON: %s; err=%v", stdout.String(), err)
		}
		if !strings.Contains(obj["url"], "jql=") {
			t.Errorf("expected url to contain jql query param, got %q", obj["url"])
		}
	})

	t.Run("POST body included, server not called", func(t *testing.T) {
		called := false
		ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
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
			t.Fatalf("expected exit code 0, got %d; stderr=%s", code, stderr.String())
		}
		if called {
			t.Error("dry run must not execute the actual HTTP request")
		}
		var obj map[string]any
		if err := json.Unmarshal(stdout.Bytes(), &obj); err != nil {
			t.Fatalf("stdout is not valid JSON: %s; err=%v", stdout.String(), err)
		}
		if obj["method"] != "POST" {
			t.Errorf("expected method=POST, got %q", obj["method"])
		}
		if bodyObj, ok := obj["body"].(map[string]any); !ok {
			t.Error("expected body field as JSON object in dry-run output")
		} else if bodyObj["fields"] == nil {
			t.Error("expected body.fields in dry-run output")
		}
	})

	t.Run("respects --jq", func(t *testing.T) {
		var stdout, stderr bytes.Buffer
		c := &client.Client{
			BaseURL: "https://example.atlassian.net", HTTPClient: &http.Client{},
			Stdout: &stdout, Stderr: &stderr,
			DryRun: true, JQFilter: ".method",
		}

		code := c.Do(context.Background(), "GET", "/rest/api/3/issue/TEST-1", nil, nil)
		if code != 0 {
			t.Fatalf("expected exit code 0, got %d; stderr=%s", code, stderr.String())
		}
		if got := strings.TrimSpace(stdout.String()); got != `"GET"` {
			t.Errorf("expected %q, got %q", `"GET"`, got)
		}
	})

	t.Run("respects --pretty", func(t *testing.T) {
		var stdout, stderr bytes.Buffer
		c := &client.Client{
			BaseURL: "https://example.atlassian.net", HTTPClient: &http.Client{},
			Stdout: &stdout, Stderr: &stderr,
			DryRun: true, Pretty: true,
		}

		code := c.Do(context.Background(), "GET", "/rest/api/3/issue/TEST-1", nil, nil)
		if code != 0 {
			t.Fatalf("expected exit code 0, got %d; stderr=%s", code, stderr.String())
		}
		if got := stdout.String(); !strings.Contains(got, "\n") || !strings.Contains(got, "  ") {
			t.Errorf("expected pretty-printed output, got: %s", got)
		}
	})

	t.Run("POST body with --jq", func(t *testing.T) {
		var stdout, stderr bytes.Buffer
		c := &client.Client{
			BaseURL: "https://example.atlassian.net", HTTPClient: &http.Client{},
			Stdout: &stdout, Stderr: &stderr,
			DryRun: true, JQFilter: ".body.fields.summary",
		}

		body := strings.NewReader(`{"fields":{"summary":"Test issue"}}`)
		code := c.Do(context.Background(), "POST", "/rest/api/3/issue", nil, body)
		if code != 0 {
			t.Fatalf("expected exit code 0, got %d; stderr=%s", code, stderr.String())
		}
		if got := strings.TrimSpace(stdout.String()); got != `"Test issue"` {
			t.Errorf("expected %q, got %q", `"Test issue"`, got)
		}
	})
}

// --- Cache ---

func TestDo_Cache(t *testing.T) {
	t.Run("GET cache hit skips server call", func(t *testing.T) {
		callCount := 0
		te := newTestEnv(func(w http.ResponseWriter, _ *http.Request) {
			callCount++
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprintln(w, `{"data":"fresh"}`)
		})
		defer te.Close()
		te.Client.CacheTTL = 5 * time.Minute

		code := te.Client.Do(context.Background(), "GET", "/rest/api/3/cached-test", nil, nil)
		if code != 0 {
			t.Fatalf("expected exit code 0, got %d", code)
		}
		if callCount != 1 {
			t.Fatalf("expected 1 server call, got %d", callCount)
		}

		te.ResetBuffers()
		code = te.Client.Do(context.Background(), "GET", "/rest/api/3/cached-test", nil, nil)
		if code != 0 {
			t.Fatalf("expected exit code 0, got %d", code)
		}
		if callCount != 1 {
			t.Errorf("expected still 1 server call (cache hit), got %d", callCount)
		}
	})

	t.Run("POST never cached", func(t *testing.T) {
		callCount := 0
		te := newTestEnv(func(w http.ResponseWriter, _ *http.Request) {
			callCount++
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprintln(w, `{"created":true}`)
		})
		defer te.Close()
		te.Client.CacheTTL = 5 * time.Minute

		te.Client.Do(context.Background(), "POST", "/rest/api/3/issue", nil, strings.NewReader(`{}`))
		te.ResetBuffers()
		te.Client.Do(context.Background(), "POST", "/rest/api/3/issue", nil, strings.NewReader(`{}`))

		if callCount != 2 {
			t.Errorf("expected 2 server calls (POST never cached), got %d", callCount)
		}
	})

	t.Run("expired cache triggers new call", func(t *testing.T) {
		callCount := 0
		te := newTestEnv(func(w http.ResponseWriter, _ *http.Request) {
			callCount++
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprintf(w, `{"call":%d}`, callCount)
		})
		defer te.Close()
		te.Client.CacheTTL = 1 * time.Millisecond

		te.Client.Do(context.Background(), "GET", "/rest/api/3/expire-test", nil, nil)
		time.Sleep(10 * time.Millisecond)
		te.ResetBuffers()
		te.Client.Do(context.Background(), "GET", "/rest/api/3/expire-test", nil, nil)

		if callCount != 2 {
			t.Errorf("expected 2 server calls after cache expiry, got %d", callCount)
		}
	})

	t.Run("cache works with pagination", func(t *testing.T) {
		callCount := 0
		te := newTestEnv(func(w http.ResponseWriter, _ *http.Request) {
			callCount++
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprintln(w, `{"startAt":0,"maxResults":50,"total":1,"values":[{"id":1}]}`)
		})
		defer te.Close()
		te.Client.Paginate = true
		te.Client.CacheTTL = 5 * time.Minute

		te.Client.Do(context.Background(), "GET", "/rest/api/3/cache-page-test", nil, nil)
		first := te.Stdout.String()
		te.ResetBuffers()
		te.Client.Do(context.Background(), "GET", "/rest/api/3/cache-page-test", nil, nil)

		if callCount != 1 {
			t.Errorf("expected 1 server call (cache hit), got %d", callCount)
		}
		if te.Stdout.String() != first {
			t.Errorf("cached output differs:\n  first:  %s\n  second: %s", first, te.Stdout.String())
		}
	})

	t.Run("cache works with non-paginated GET when Paginate=true", func(t *testing.T) {
		callCount := 0
		te := newTestEnv(func(w http.ResponseWriter, _ *http.Request) {
			callCount++
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprintln(w, `{"key":"PROJ-1","fields":{"summary":"Test"}}`)
		})
		defer te.Close()
		te.Client.Paginate = true
		te.Client.CacheTTL = 5 * time.Minute

		te.Client.Do(context.Background(), "GET", "/rest/api/3/issue/PROJ-1", nil, nil)
		te.ResetBuffers()
		te.Client.Do(context.Background(), "GET", "/rest/api/3/issue/PROJ-1", nil, nil)

		if callCount != 1 {
			t.Fatalf("expected 1 server call after cached request, got %d", callCount)
		}
	})
}

// --- Offset-based pagination ---

func TestDo_OffsetPagination(t *testing.T) {
	t.Run("2 pages merged", func(t *testing.T) {
		callCount := 0
		te := newTestEnv(func(w http.ResponseWriter, r *http.Request) {
			callCount++
			w.Header().Set("Content-Type", "application/json")
			startAt := r.URL.Query().Get("startAt")
			if startAt == "" || startAt == "0" {
				fmt.Fprintln(w, `{"startAt":0,"maxResults":2,"total":4,"values":[{"id":1},{"id":2}]}`)
			} else {
				fmt.Fprintln(w, `{"startAt":2,"maxResults":2,"total":4,"values":[{"id":3},{"id":4}]}`)
			}
		})
		defer te.Close()
		te.Client.Paginate = true

		code := te.Client.Do(context.Background(), "GET", "/path", nil, nil)
		if code != 0 {
			t.Fatalf("unexpected exit code %d; stderr=%s", code, te.Stderr.String())
		}
		if callCount != 2 {
			t.Errorf("expected 2 HTTP calls, got %d", callCount)
		}

		var envelope struct {
			StartAt int                      `json:"startAt"`
			Total   int                      `json:"total"`
			IsLast  bool                     `json:"isLast"`
			Values  []map[string]any `json:"values"`
		}
		if err := json.Unmarshal(te.Stdout.Bytes(), &envelope); err != nil {
			t.Fatalf("stdout is not valid JSON: %s; err=%v", te.Stdout.String(), err)
		}
		if len(envelope.Values) != 4 {
			t.Errorf("expected 4 merged items, got %d", len(envelope.Values))
		}
		if !envelope.IsLast {
			t.Error("expected isLast=true after merging all pages")
		}
	})

	t.Run("3 pages with correct order", func(t *testing.T) {
		callCount := 0
		te := newTestEnv(func(w http.ResponseWriter, r *http.Request) {
			callCount++
			w.Header().Set("Content-Type", "application/json")
			switch r.URL.Query().Get("startAt") {
			case "", "0":
				fmt.Fprintln(w, `{"startAt":0,"maxResults":2,"total":6,"values":[{"id":1},{"id":2}]}`)
			case "2":
				fmt.Fprintln(w, `{"startAt":2,"maxResults":2,"total":6,"values":[{"id":3},{"id":4}]}`)
			default:
				fmt.Fprintln(w, `{"startAt":4,"maxResults":2,"total":6,"values":[{"id":5},{"id":6}]}`)
			}
		})
		defer te.Close()
		te.Client.Paginate = true

		code := te.Client.Do(context.Background(), "GET", "/path", nil, nil)
		if code != 0 {
			t.Fatalf("expected exit code 0, got %d", code)
		}
		if callCount != 3 {
			t.Errorf("expected 3 HTTP calls, got %d", callCount)
		}

		var envelope struct {
			Total  int                      `json:"total"`
			Values []map[string]any `json:"values"`
		}
		if err := json.Unmarshal(te.Stdout.Bytes(), &envelope); err != nil {
			t.Fatalf("stdout is not valid JSON: %v", err)
		}
		if len(envelope.Values) != 6 {
			t.Errorf("expected 6 merged values, got %d", len(envelope.Values))
		}
		for i, v := range envelope.Values {
			if v["id"] != float64(i+1) {
				t.Errorf("expected values[%d].id=%d, got %v", i, i+1, v["id"])
			}
		}
	})

	t.Run("isLast field instead of total", func(t *testing.T) {
		callCount := 0
		te := newTestEnv(func(w http.ResponseWriter, r *http.Request) {
			callCount++
			w.Header().Set("Content-Type", "application/json")
			startAt := r.URL.Query().Get("startAt")
			if startAt == "" || startAt == "0" {
				fmt.Fprintln(w, `{"startAt":0,"maxResults":2,"isLast":false,"values":[{"id":1},{"id":2}]}`)
			} else {
				fmt.Fprintln(w, `{"startAt":2,"maxResults":2,"isLast":true,"values":[{"id":3}]}`)
			}
		})
		defer te.Close()
		te.Client.Paginate = true

		code := te.Client.Do(context.Background(), "GET", "/path", nil, nil)
		if code != 0 {
			t.Fatalf("unexpected exit code %d", code)
		}
		if callCount != 2 {
			t.Errorf("expected 2 HTTP calls, got %d", callCount)
		}
		var envelope struct {
			Values []map[string]any `json:"values"`
			IsLast bool                     `json:"isLast"`
		}
		if err := json.Unmarshal(te.Stdout.Bytes(), &envelope); err != nil {
			t.Fatalf("not valid JSON: %v", err)
		}
		if len(envelope.Values) != 3 {
			t.Errorf("expected 3 merged items, got %d", len(envelope.Values))
		}
		if !envelope.IsLast {
			t.Error("expected isLast=true")
		}
	})

	t.Run("single page no extra requests", func(t *testing.T) {
		callCount := 0
		te := newTestEnv(func(w http.ResponseWriter, _ *http.Request) {
			callCount++
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprintln(w, `{"startAt":0,"maxResults":50,"total":2,"values":[{"id":1},{"id":2}]}`)
		})
		defer te.Close()
		te.Client.Paginate = true

		te.Client.Do(context.Background(), "GET", "/path", nil, nil)
		if callCount != 1 {
			t.Errorf("expected 1 HTTP call, got %d", callCount)
		}
	})

	t.Run("error on page 2", func(t *testing.T) {
		callCount := 0
		te := newTestEnv(func(w http.ResponseWriter, _ *http.Request) {
			callCount++
			if callCount == 1 {
				w.Header().Set("Content-Type", "application/json")
				fmt.Fprintln(w, `{"startAt":0,"maxResults":2,"total":4,"values":[{"id":1},{"id":2}]}`)
			} else {
				w.WriteHeader(http.StatusInternalServerError)
				fmt.Fprintln(w, `{"errorMessages":["internal error"]}`)
			}
		})
		defer te.Close()
		te.Client.Paginate = true

		code := te.Client.Do(context.Background(), "GET", "/path", nil, nil)
		if code == 0 {
			t.Fatal("expected non-zero exit code on page 2 error")
		}
	})

	t.Run("non-paginated response passthrough", func(t *testing.T) {
		te := newTestEnv(jsonHandler(`{"displayName":"Agent","active":true}`))
		defer te.Close()
		te.Client.Paginate = true

		code := te.Client.Do(context.Background(), "GET", "/rest/api/3/myself", nil, nil)
		if code != 0 {
			t.Fatalf("unexpected exit code %d", code)
		}
		var result map[string]any
		if err := json.Unmarshal(te.Stdout.Bytes(), &result); err != nil {
			t.Fatalf("not valid JSON: %v", err)
		}
		if result["displayName"] != "Agent" {
			t.Errorf("expected displayName=Agent, got %v", result["displayName"])
		}
	})

	t.Run("jq filter on paginated response", func(t *testing.T) {
		te := newTestEnv(jsonHandler(`{"startAt":0,"maxResults":2,"total":2,"values":[{"id":1},{"id":2}]}`))
		defer te.Close()
		te.Client.Paginate = true
		te.Client.JQFilter = "[.values[].id]"

		code := te.Client.Do(context.Background(), "GET", "/path", nil, nil)
		if code != 0 {
			t.Fatalf("unexpected exit code %d", code)
		}
		if out := strings.TrimSpace(te.Stdout.String()); out != "[1,2]" {
			t.Errorf("expected [1,2], got %q", out)
		}
	})

	t.Run("jq on non-paginated with pagination enabled", func(t *testing.T) {
		te := newTestEnv(jsonHandler(`{"displayName":"Agent","emailAddress":"agent@test.com"}`))
		defer te.Close()
		te.Client.Paginate = true
		te.Client.JQFilter = ".displayName"

		code := te.Client.Do(context.Background(), "GET", "/path", nil, nil)
		if code != 0 {
			t.Fatalf("unexpected exit code %d", code)
		}
		if out := strings.TrimSpace(te.Stdout.String()); out != `"Agent"` {
			t.Errorf("expected %q, got %q", `"Agent"`, out)
		}
	})

	t.Run("pretty print preserves envelope", func(t *testing.T) {
		te := newTestEnv(jsonHandler(`{"startAt":0,"maxResults":50,"total":1,"values":[{"id":1}]}`))
		defer te.Close()
		te.Client.Paginate = true
		te.Client.Pretty = true

		code := te.Client.Do(context.Background(), "GET", "/path", nil, nil)
		if code != 0 {
			t.Fatalf("unexpected exit code %d", code)
		}
		out := te.Stdout.String()
		if !strings.Contains(out, "\n") {
			t.Error("expected pretty-printed output with newlines")
		}
		var envelope struct {
			Values []map[string]any `json:"values"`
		}
		if err := json.Unmarshal([]byte(out), &envelope); err != nil {
			t.Fatalf("not valid JSON: err=%v", err)
		}
		if len(envelope.Values) != 1 {
			t.Errorf("expected 1 value, got %d", len(envelope.Values))
		}
	})

	t.Run("empty values array", func(t *testing.T) {
		te := newTestEnv(jsonHandler(`{"startAt":0,"maxResults":50,"total":0,"values":[]}`))
		defer te.Close()
		te.Client.Paginate = true

		te.Client.Do(context.Background(), "GET", "/path", nil, nil)

		var envelope struct {
			Values []any `json:"values"`
			Total  int           `json:"total"`
		}
		if err := json.Unmarshal(te.Stdout.Bytes(), &envelope); err != nil {
			t.Fatalf("not valid JSON: %v", err)
		}
		if len(envelope.Values) != 0 {
			t.Errorf("expected 0 values, got %d", len(envelope.Values))
		}
	})

	t.Run("pagination disabled returns raw response", func(t *testing.T) {
		te := newTestEnv(jsonHandler(`{"startAt":0,"maxResults":2,"total":4,"values":[{"id":1},{"id":2}]}`))
		defer te.Close()
		te.Client.Paginate = false

		te.Client.Do(context.Background(), "GET", "/path", nil, nil)

		var envelope struct {
			Total      int `json:"total"`
			MaxResults int `json:"maxResults"`
		}
		if err := json.Unmarshal(te.Stdout.Bytes(), &envelope); err != nil {
			t.Fatalf("not valid JSON: %v", err)
		}
		if envelope.Total != 4 || envelope.MaxResults != 2 {
			t.Errorf("expected total=4, maxResults=2, got total=%d, maxResults=%d", envelope.Total, envelope.MaxResults)
		}
	})

	t.Run("POST with Paginate=true does not paginate", func(t *testing.T) {
		callCount := 0
		te := newTestEnv(func(w http.ResponseWriter, r *http.Request) {
			callCount++
			if r.Method != http.MethodPost {
				t.Errorf("expected POST, got %s", r.Method)
			}
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprintln(w, `{"startAt":0,"maxResults":2,"total":4,"values":[{"id":1},{"id":2}]}`)
		})
		defer te.Close()
		te.Client.Paginate = true

		te.Client.Do(context.Background(), "POST", "/rest/api/3/search", nil, strings.NewReader(`{"jql":"project=PROJ"}`))
		if callCount != 1 {
			t.Errorf("expected 1 server call for POST, got %d", callCount)
		}
	})

	t.Run("no HTML escaping", func(t *testing.T) {
		te := newTestEnv(jsonHandler(`{"startAt":0,"maxResults":50,"total":1,"values":[{"url":"http://example.com/a?b=1&c=2"}]}`))
		defer te.Close()
		te.Client.Paginate = true

		te.Client.Do(context.Background(), "GET", "/path", nil, nil)

		out := te.Stdout.String()
		if strings.Contains(out, `\u0026`) {
			t.Errorf("paginated output should not HTML-escape '&':\n%s", out)
		}
		if !strings.Contains(out, `&c=2`) {
			t.Errorf("expected literal '&' in output:\n%s", out)
		}
	})

	t.Run("non-zero startAt offset", func(t *testing.T) {
		requestCount := 0
		requestedStartAts := []string{}
		te := newTestEnv(func(w http.ResponseWriter, r *http.Request) {
			requestCount++
			startAt := r.URL.Query().Get("startAt")
			requestedStartAts = append(requestedStartAts, startAt)
			w.Header().Set("Content-Type", "application/json")

			switch startAt {
			case "", "0":
				_, _ = w.Write([]byte(`{"startAt":0,"maxResults":2,"total":6,"isLast":false,"values":[{"id":"A"},{"id":"B"}]}`))
			case "2":
				_, _ = w.Write([]byte(`{"startAt":2,"maxResults":2,"total":6,"isLast":false,"values":[{"id":"C"},{"id":"D"}]}`))
			case "4":
				_, _ = w.Write([]byte(`{"startAt":4,"maxResults":2,"total":6,"isLast":true,"values":[{"id":"E"},{"id":"F"}]}`))
			default:
				t.Errorf("unexpected startAt=%q", startAt)
				_, _ = w.Write([]byte(`{"startAt":0,"maxResults":2,"total":6,"isLast":true,"values":[]}`))
			}
		})
		defer te.Close()
		te.Client.Paginate = true

		q := make(map[string][]string)
		q["startAt"] = []string{"2"}
		code := te.Client.Do(context.Background(), "GET", "/rest/api/3/test", q, nil)
		if code != 0 {
			t.Fatalf("expected exit code 0, got %d", code)
		}

		var envelope struct {
			Values []map[string]string `json:"values"`
		}
		if err := json.Unmarshal([]byte(strings.TrimSpace(te.Stdout.String())), &envelope); err != nil {
			t.Fatalf("failed to parse output: %v", err)
		}
		if len(envelope.Values) != 4 {
			t.Errorf("expected 4 merged values, got %d", len(envelope.Values))
		}
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
		if requestCount != 2 {
			t.Errorf("expected 2 server requests, got %d (startAts: %v)", requestCount, requestedStartAts)
		}
	})
}

// --- Token-based pagination ---

func TestDo_TokenPagination(t *testing.T) {
	t.Run("multi-page merge", func(t *testing.T) {
		page := 0
		te := newTestEnv(func(w http.ResponseWriter, r *http.Request) {
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
		})
		defer te.Close()
		te.Client.Paginate = true

		code := te.Client.Do(context.Background(), "GET", "/rest/api/3/search/jql", nil, nil)
		if code != 0 {
			t.Fatalf("expected exit 0, got %d; stderr=%s", code, te.Stderr.String())
		}

		var resp struct {
			Issues []map[string]string `json:"issues"`
			IsLast bool                `json:"isLast"`
		}
		if err := json.Unmarshal(te.Stdout.Bytes(), &resp); err != nil {
			t.Fatalf("invalid JSON: %v", err)
		}
		if len(resp.Issues) != 3 {
			t.Errorf("expected 3 issues, got %d", len(resp.Issues))
		}
		if !resp.IsLast {
			t.Error("expected isLast=true in merged response")
		}
		var raw map[string]json.RawMessage
		_ = json.Unmarshal(te.Stdout.Bytes(), &raw)
		if _, ok := raw["nextPageToken"]; ok {
			t.Error("nextPageToken should be removed from merged response")
		}
	})

	t.Run("single page", func(t *testing.T) {
		te := newTestEnv(jsonHandler(`{"issues":[{"key":"B-1"}],"isLast":true}`))
		defer te.Close()
		te.Client.Paginate = true

		code := te.Client.Do(context.Background(), "GET", "/rest/api/3/search/jql", nil, nil)
		if code != 0 {
			t.Fatalf("expected exit 0, got %d", code)
		}
		var resp struct {
			Issues []map[string]string `json:"issues"`
		}
		if err := json.Unmarshal(te.Stdout.Bytes(), &resp); err != nil {
			t.Fatalf("invalid JSON: %v", err)
		}
		if len(resp.Issues) != 1 {
			t.Errorf("expected 1 issue, got %d", len(resp.Issues))
		}
	})

	t.Run("error on page 2", func(t *testing.T) {
		page := 0
		te := newTestEnv(func(w http.ResponseWriter, _ *http.Request) {
			switch page {
			case 0:
				page++
				w.Header().Set("Content-Type", "application/json")
				fmt.Fprintln(w, `{"issues":[{"key":"C-1"}],"nextPageToken":"tok1","isLast":false}`)
			default:
				w.WriteHeader(500)
				fmt.Fprintln(w, `{"errorMessages":["server error"]}`)
			}
		})
		defer te.Close()
		te.Client.Paginate = true

		code := te.Client.Do(context.Background(), "GET", "/rest/api/3/search/jql", nil, nil)
		if code == 0 {
			t.Error("expected non-zero exit code on page 2 error")
		}
	})

	t.Run("with JQ filter", func(t *testing.T) {
		page := 0
		te := newTestEnv(func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			switch page {
			case 0:
				page++
				fmt.Fprintln(w, `{"issues":[{"key":"D-1"},{"key":"D-2"}],"nextPageToken":"tok1","isLast":false}`)
			default:
				fmt.Fprintln(w, `{"issues":[{"key":"D-3"}],"isLast":true}`)
			}
		})
		defer te.Close()
		te.Client.Paginate = true
		te.Client.JQFilter = "[.issues[] | .key]"

		code := te.Client.Do(context.Background(), "GET", "/rest/api/3/search/jql", nil, nil)
		if code != 0 {
			t.Fatalf("expected exit 0, got %d", code)
		}
		var keys []string
		if err := json.Unmarshal(te.Stdout.Bytes(), &keys); err != nil {
			t.Fatalf("invalid JSON: %v; stdout=%s", err, te.Stdout.String())
		}
		if len(keys) != 3 {
			t.Errorf("expected 3 keys, got %d: %v", len(keys), keys)
		}
	})

	t.Run("empty subsequent page stops pagination", func(t *testing.T) {
		callCount := 0
		te := newTestEnv(func(w http.ResponseWriter, _ *http.Request) {
			callCount++
			w.Header().Set("Content-Type", "application/json")
			switch callCount {
			case 1:
				fmt.Fprintln(w, `{"issues":[{"id":"1"},{"id":"2"}],"nextPageToken":"tok1"}`)
			case 2:
				fmt.Fprintln(w, `{"issues":[{"id":"3"}],"nextPageToken":"tok2"}`)
			case 3:
				fmt.Fprintln(w, `{"issues":[],"nextPageToken":"tok3"}`)
			default:
				t.Errorf("unexpected request #%d", callCount)
				fmt.Fprintln(w, `{"issues":[]}`)
			}
		})
		defer te.Close()
		te.Client.Paginate = true

		code := te.Client.Do(context.Background(), "GET", "/test", nil, nil)
		if code != 0 {
			t.Fatalf("expected exit 0, got %d", code)
		}
		var result struct {
			Issues []json.RawMessage `json:"issues"`
		}
		if err := json.Unmarshal([]byte(strings.TrimSpace(te.Stdout.String())), &result); err != nil {
			t.Fatalf("failed to parse output: %v", err)
		}
		if len(result.Issues) != 3 {
			t.Errorf("expected 3 issues, got %d", len(result.Issues))
		}
		if callCount > 3 {
			t.Errorf("expected at most 3 requests, made %d (infinite loop bug)", callCount)
		}
	})

	t.Run("non-empty token with empty issues stops", func(t *testing.T) {
		callCount := 0
		te := newTestEnv(func(w http.ResponseWriter, _ *http.Request) {
			callCount++
			w.Header().Set("Content-Type", "application/json")
			if callCount == 1 {
				fmt.Fprintln(w, `{"issues":[{"id":"1"}],"nextPageToken":"tok1"}`)
			} else {
				fmt.Fprintln(w, `{"issues":[],"nextPageToken":"infinite-loop-trap"}`)
			}
		})
		defer te.Close()
		te.Client.Paginate = true

		te.Client.Do(context.Background(), "GET", "/test", nil, nil)
		if callCount > 2 {
			t.Fatalf("expected at most 2 requests, made %d (infinite loop detected!)", callCount)
		}
	})
}

// --- QueryFromFlags ---

func TestQueryFromFlags(t *testing.T) {
	cmd := &cobra.Command{}
	cmd.Flags().String("project", "", "project key")
	cmd.Flags().Int("max-results", 50, "max results")
	cmd.Flags().String("status", "", "status filter")

	if err := cmd.Flags().Set("project", "MYPROJ"); err != nil {
		t.Fatal(err)
	}

	q := client.QueryFromFlags(cmd, "project", "max-results", "status")
	if q.Get("project") != "MYPROJ" {
		t.Errorf("expected project=MYPROJ, got %q", q.Get("project"))
	}
	if q.Has("max-results") {
		t.Error("max-results should NOT be in query (not changed)")
	}
	if q.Has("status") {
		t.Error("status should NOT be in query (not changed)")
	}
}

// --- Context roundtrip ---

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

func TestFromContext_Missing(t *testing.T) {
	_, err := client.FromContext(context.Background())
	if err == nil {
		t.Error("expected error when client is not in context, got nil")
	}
}

// --- Large response ---

func TestDo_LargeResponse(t *testing.T) {
	const targetSize = 100 * 1024
	largeValue := strings.Repeat("x", targetSize)
	te := newTestEnv(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, `{"data":"%s"}`, largeValue)
	})
	defer te.Close()

	code := te.Client.Do(context.Background(), "GET", "/rest/api/3/large", nil, nil)
	if code != 0 {
		t.Fatalf("expected exit code 0, got %d", code)
	}
	var result map[string]any
	if err := json.Unmarshal(te.Stdout.Bytes(), &result); err != nil {
		t.Fatalf("stdout is not valid JSON: err=%v", err)
	}
	data, _ := result["data"].(string)
	if len(data) != targetSize {
		t.Errorf("expected data length %d, got %d", targetSize, len(data))
	}
}

func TestFetch(t *testing.T) {
	tests := []struct {
		name     string
		status   int
		body     string
		method   string
		wantCode int
		wantBody string
	}{
		{"success GET", 200, `{"ok":true}`, "GET", 0, `{"ok":true}`},
		{"success POST", 200, `{"id":1}`, "POST", 0, `{"id":1}`},
		{"not found", 404, `{"error":"nope"}`, "GET", 3, ""},
		{"server error", 500, `{"error":"boom"}`, "GET", 7, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			te := newTestEnv(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tt.status)
				w.Write([]byte(tt.body))
			})
			defer te.Close()
			body, code := te.Client.Fetch(context.Background(), tt.method, "/test", nil)
			if code != tt.wantCode {
				t.Errorf("code = %d, want %d", code, tt.wantCode)
			}
			if tt.wantCode == 0 && string(body) != tt.wantBody {
				t.Errorf("body = %q, want %q", body, tt.wantBody)
			}
		})
	}
}
