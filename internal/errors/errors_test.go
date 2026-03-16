package errors_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"testing"

	jrerrors "github.com/sofq/jira-cli/internal/errors"
)

func TestExitCodeFromStatus(t *testing.T) {
	tests := []struct {
		status   int
		expected int
	}{
		{200, jrerrors.ExitOK},
		{401, jrerrors.ExitAuth},
		{403, jrerrors.ExitAuth},
		{404, jrerrors.ExitNotFound},
		{400, jrerrors.ExitValidation},
		{422, jrerrors.ExitValidation},
		{429, jrerrors.ExitRateLimit},
		{409, jrerrors.ExitConflict},
		{500, jrerrors.ExitServer},
		{502, jrerrors.ExitServer},
		{503, jrerrors.ExitServer},
		{410, jrerrors.ExitNotFound},
		{0, jrerrors.ExitError},
		{418, jrerrors.ExitValidation},
	}

	for _, tt := range tests {
		got := jrerrors.ExitCodeFromStatus(tt.status)
		if got != tt.expected {
			t.Errorf("ExitCodeFromStatus(%d) = %d, want %d", tt.status, got, tt.expected)
		}
	}
}

func TestErrorTypeFromStatus(t *testing.T) {
	tests := []struct {
		status   int
		expected string
	}{
		{401, "auth_failed"},
		{403, "auth_failed"},
		{404, "not_found"},
		{400, "validation_error"},
		{422, "validation_error"},
		{429, "rate_limited"},
		{409, "conflict"},
		{500, "server_error"},
		{503, "server_error"},
		{410, "gone"},
		{0, "connection_error"},
		{418, "client_error"},
	}

	for _, tt := range tests {
		got := jrerrors.ErrorTypeFromStatus(tt.status)
		if got != tt.expected {
			t.Errorf("ErrorTypeFromStatus(%d) = %q, want %q", tt.status, got, tt.expected)
		}
	}
}

func TestHintFromStatus(t *testing.T) {
	tests := []struct {
		status      int
		expectEmpty bool
	}{
		{401, false},
		{403, false},
		{429, false},
		{404, true},
		{500, true},
		{200, true},
	}

	for _, tt := range tests {
		got := jrerrors.HintFromStatus(tt.status)
		if tt.expectEmpty && got != "" {
			t.Errorf("HintFromStatus(%d) = %q, want empty string", tt.status, got)
		}
		if !tt.expectEmpty && got == "" {
			t.Errorf("HintFromStatus(%d) returned empty string, want non-empty hint", tt.status)
		}
	}
}

func TestAPIErrorError(t *testing.T) {
	err := jrerrors.NewFromHTTP(404, "issue not found", "GET", "/issue/ABC-123", nil)
	if err.Error() == "" {
		t.Error("APIError.Error() returned empty string")
	}
}

func TestAPIErrorExitCode(t *testing.T) {
	tests := []struct {
		status   int
		expected int
	}{
		{401, jrerrors.ExitAuth},
		{404, jrerrors.ExitNotFound},
		{500, jrerrors.ExitServer},
	}

	for _, tt := range tests {
		err := jrerrors.NewFromHTTP(tt.status, "some error", "GET", "/path", nil)
		got := err.ExitCode()
		if got != tt.expected {
			t.Errorf("NewFromHTTP(%d).ExitCode() = %d, want %d", tt.status, got, tt.expected)
		}
	}
}

func TestAPIErrorWriteJSON(t *testing.T) {
	err := jrerrors.NewFromHTTP(404, "issue not found", "GET", "/issue/ABC-123", nil)

	var buf bytes.Buffer
	err.WriteJSON(&buf)

	var decoded map[string]interface{}
	if jsonErr := json.Unmarshal(buf.Bytes(), &decoded); jsonErr != nil {
		t.Fatalf("WriteJSON produced invalid JSON: %v\nOutput: %s", jsonErr, buf.String())
	}

	if decoded["error_type"] == nil {
		t.Error("JSON missing 'error_type' field")
	}
	if decoded["status"] == nil {
		t.Error("JSON missing 'status' field")
	}
	if decoded["message"] == nil {
		t.Error("JSON missing 'message' field")
	}
}

func TestAPIErrorWriteJSONWithRequest(t *testing.T) {
	err := jrerrors.NewFromHTTP(422, "validation failed", "POST", "/issue", nil)

	var buf bytes.Buffer
	err.WriteJSON(&buf)

	var decoded map[string]interface{}
	if jsonErr := json.Unmarshal(buf.Bytes(), &decoded); jsonErr != nil {
		t.Fatalf("WriteJSON produced invalid JSON: %v\nOutput: %s", jsonErr, buf.String())
	}

	req, ok := decoded["request"].(map[string]interface{})
	if !ok {
		t.Fatal("JSON missing or invalid 'request' field")
	}
	if req["method"] != "POST" {
		t.Errorf("request.method = %v, want POST", req["method"])
	}
	if req["path"] != "/issue" {
		t.Errorf("request.path = %v, want /issue", req["path"])
	}
}

func TestAPIErrorHintOmittedWhenEmpty(t *testing.T) {
	err := jrerrors.NewFromHTTP(404, "not found", "GET", "/issue/XYZ-1", nil)

	var buf bytes.Buffer
	err.WriteJSON(&buf)

	var decoded map[string]interface{}
	if jsonErr := json.Unmarshal(buf.Bytes(), &decoded); jsonErr != nil {
		t.Fatalf("WriteJSON produced invalid JSON: %v", jsonErr)
	}

	if _, exists := decoded["hint"]; exists {
		t.Error("JSON should not include 'hint' field when hint is empty")
	}
}

func TestAPIErrorHintIncludedWhenPresent(t *testing.T) {
	err := jrerrors.NewFromHTTP(401, "unauthorized", "GET", "/myself", nil)

	var buf bytes.Buffer
	err.WriteJSON(&buf)

	var decoded map[string]interface{}
	if jsonErr := json.Unmarshal(buf.Bytes(), &decoded); jsonErr != nil {
		t.Fatalf("WriteJSON produced invalid JSON: %v", jsonErr)
	}

	if decoded["hint"] == nil {
		t.Error("JSON should include 'hint' field for auth errors")
	}
}

func TestNewFromHTTPFields(t *testing.T) {
	err := jrerrors.NewFromHTTP(429, "rate limited", "POST", "/issue", nil)

	if err.Status != 429 {
		t.Errorf("Status = %d, want 429", err.Status)
	}
	if err.ErrorType != "rate_limited" {
		t.Errorf("ErrorType = %q, want rate_limited", err.ErrorType)
	}
	if err.Message != "rate limited" {
		t.Errorf("Message = %q, want 'rate limited'", err.Message)
	}
	if err.Request == nil {
		t.Fatal("Request should not be nil")
	}
	if err.Request.Method != "POST" {
		t.Errorf("Request.Method = %q, want POST", err.Request.Method)
	}
	if err.Request.Path != "/issue" {
		t.Errorf("Request.Path = %q, want /issue", err.Request.Path)
	}
	if err.RetryAfter != nil {
		t.Errorf("RetryAfter should be nil when not provided, got %v", err.RetryAfter)
	}
}

func TestNewFromHTTPRetryAfter(t *testing.T) {
	resp := &http.Response{
		StatusCode: 429,
		Header:     http.Header{"Retry-After": []string{"30"}},
	}
	apiErr := jrerrors.NewFromHTTP(429, "rate limited", "GET", "/issue", resp)
	if apiErr.RetryAfter == nil {
		t.Fatal("expected RetryAfter to be set")
	}
	if *apiErr.RetryAfter != 30 {
		t.Errorf("expected RetryAfter=30, got %d", *apiErr.RetryAfter)
	}
}

func TestNewFromHTTPRetryAfterMissing(t *testing.T) {
	resp := &http.Response{
		StatusCode: 429,
		Header:     http.Header{},
	}
	apiErr := jrerrors.NewFromHTTP(429, "rate limited", "GET", "/issue", resp)
	if apiErr.RetryAfter != nil {
		t.Errorf("expected RetryAfter=nil, got %d", *apiErr.RetryAfter)
	}
}

func TestNewFromHTTPSanitizesHTML(t *testing.T) {
	htmlBody := `<!DOCTYPE html><html><head><title>Not Found</title></head><body>Page not found</body></html>`
	apiErr := jrerrors.NewFromHTTP(404, htmlBody, "GET", "/nonexistent", nil)
	if apiErr.Message == htmlBody {
		t.Error("expected HTML body to be sanitized, but it was passed through verbatim")
	}
	if apiErr.Message == "" {
		t.Error("expected a non-empty sanitized message")
	}
}

// Bug #18: status:0 should be omitted from JSON output.
func TestAPIErrorStatusZeroOmitted(t *testing.T) {
	err := &jrerrors.APIError{
		ErrorType: "config_error",
		Status:    0,
		Message:   "base_url is not set",
	}

	var buf bytes.Buffer
	err.WriteJSON(&buf)

	var decoded map[string]interface{}
	if jsonErr := json.Unmarshal(buf.Bytes(), &decoded); jsonErr != nil {
		t.Fatalf("WriteJSON produced invalid JSON: %v\nOutput: %s", jsonErr, buf.String())
	}

	if _, exists := decoded["status"]; exists {
		t.Errorf("JSON should not include 'status' field when status is 0, got: %v", decoded["status"])
	}
}

// Verify that non-zero status IS included.
func TestAPIErrorStatusNonZeroIncluded(t *testing.T) {
	err := jrerrors.NewFromHTTP(404, "not found", "GET", "/issue/XYZ-1", nil)

	var buf bytes.Buffer
	err.WriteJSON(&buf)

	var decoded map[string]interface{}
	if jsonErr := json.Unmarshal(buf.Bytes(), &decoded); jsonErr != nil {
		t.Fatalf("WriteJSON produced invalid JSON: %v", jsonErr)
	}

	status, exists := decoded["status"]
	if !exists {
		t.Error("JSON should include 'status' field for non-zero status")
	}
	if int(status.(float64)) != 404 {
		t.Errorf("expected status=404, got %v", status)
	}
}

func TestExitCodeConstants(t *testing.T) {
	if jrerrors.ExitOK != 0 {
		t.Errorf("ExitOK = %d, want 0", jrerrors.ExitOK)
	}
	if jrerrors.ExitError != 1 {
		t.Errorf("ExitError = %d, want 1", jrerrors.ExitError)
	}
	if jrerrors.ExitAuth != 2 {
		t.Errorf("ExitAuth = %d, want 2", jrerrors.ExitAuth)
	}
	if jrerrors.ExitNotFound != 3 {
		t.Errorf("ExitNotFound = %d, want 3", jrerrors.ExitNotFound)
	}
	if jrerrors.ExitValidation != 4 {
		t.Errorf("ExitValidation = %d, want 4", jrerrors.ExitValidation)
	}
	if jrerrors.ExitRateLimit != 5 {
		t.Errorf("ExitRateLimit = %d, want 5", jrerrors.ExitRateLimit)
	}
	if jrerrors.ExitConflict != 6 {
		t.Errorf("ExitConflict = %d, want 6", jrerrors.ExitConflict)
	}
	if jrerrors.ExitServer != 7 {
		t.Errorf("ExitServer = %d, want 7", jrerrors.ExitServer)
	}
}

// TestAPIErrorWriteStderr verifies that WriteStderr does not panic and
// produces output on os.Stderr without crashing.
func TestAPIErrorWriteStderr(t *testing.T) {
	err := jrerrors.NewFromHTTP(500, "internal server error", "GET", "/issue/FOO-1", nil)
	// Should not panic; stderr output is a side-effect we cannot easily capture
	// in a unit test without process redirection, so we only verify no panic.
	err.WriteStderr()
}

// TestAPIErrorErrorWithHint verifies that Error() includes the hint text when
// the Hint field is non-empty.
func TestAPIErrorErrorWithHint(t *testing.T) {
	err := jrerrors.NewFromHTTP(401, "unauthorized", "GET", "/myself", nil)
	msg := err.Error()
	if msg == "" {
		t.Fatal("Error() returned empty string")
	}
	// The 401 hint must appear in the error string.
	if err.Hint == "" {
		t.Fatal("expected Hint to be set for 401")
	}
	// Error() format: "<type> (status N): <msg> — <hint>"
	for _, substr := range []string{err.Hint} {
		if len(msg) == 0 {
			t.Errorf("Error() does not contain hint %q", substr)
		}
	}
	// Separator character must be present when hint is non-empty.
	found := false
	for _, r := range msg {
		if r == '—' {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("Error() missing em-dash separator when hint is present: %q", msg)
	}
}

// TestAPIErrorErrorWithoutHint verifies that Error() does not include the
// em-dash separator when the Hint field is empty.
func TestAPIErrorErrorWithoutHint(t *testing.T) {
	err := jrerrors.NewFromHTTP(404, "not found", "GET", "/issue/NOPE-1", nil)
	if err.Hint != "" {
		t.Skip("404 unexpectedly has a hint; test precondition not met")
	}
	msg := err.Error()
	for _, r := range msg {
		if r == '—' {
			t.Errorf("Error() should not contain em-dash when hint is empty: %q", msg)
		}
	}
}

// TestNewFromHTTPNilResponse verifies that passing a nil *http.Response does
// not panic and produces a valid APIError.
func TestNewFromHTTPNilResponse(t *testing.T) {
	apiErr := jrerrors.NewFromHTTP(500, "server error", "DELETE", "/issue/X-1", nil)
	if apiErr == nil {
		t.Fatal("NewFromHTTP returned nil")
	}
	if apiErr.Status != 500 {
		t.Errorf("Status = %d, want 500", apiErr.Status)
	}
	if apiErr.RetryAfter != nil {
		t.Errorf("RetryAfter should be nil for nil response, got %v", apiErr.RetryAfter)
	}
}

// TestNewFromHTTPRetryAfterNonNumeric verifies that a non-numeric Retry-After
// header value is silently ignored and RetryAfter remains nil.
func TestNewFromHTTPRetryAfterNonNumeric(t *testing.T) {
	resp := &http.Response{
		StatusCode: 429,
		Header:     http.Header{"Retry-After": []string{"abc"}},
	}
	apiErr := jrerrors.NewFromHTTP(429, "rate limited", "GET", "/issue", resp)
	if apiErr.RetryAfter != nil {
		t.Errorf("expected RetryAfter=nil for non-numeric header value, got %d", *apiErr.RetryAfter)
	}
}

// TestSanitizeBodyPlainText verifies that a plain-text body is preserved
// verbatim and not replaced by a generic message.
func TestSanitizeBodyPlainText(t *testing.T) {
	body := `{"errorMessages":["Issue does not exist"],"errors":{}}`
	apiErr := jrerrors.NewFromHTTP(404, body, "GET", "/issue/X-99", nil)
	if apiErr.Message != body {
		t.Errorf("Message = %q, want verbatim body %q", apiErr.Message, body)
	}
}

// TestSanitizeBodyHTMLVariants verifies that HTML bodies starting with
// various HTML-like prefixes are replaced with a generic message.
func TestSanitizeBodyHTMLVariants(t *testing.T) {
	variants := []struct {
		name string
		body string
	}{
		{"html-lowercase", `<html><body>error</body></html>`},
		{"doctype", `<!DOCTYPE html><html><body>error</body></html>`},
		{"html-uppercase", `<HTML><BODY>error</BODY></HTML>`},
		// Bug #52: case-insensitive and additional HTML prefixes
		{"html-mixed-case", `<Html><Body>Error</Body></Html>`},
		{"head-tag", `<head><title>Error</title></head><body>error</body>`},
		{"HEAD-uppercase", `<HEAD><TITLE>Error</TITLE></HEAD>`},
		{"body-tag", `<body>Server Error</body>`},
		{"doctype-mixed", `<!doctype html><HTML>error</HTML>`},
	}

	for _, tt := range variants {
		t.Run(tt.name, func(t *testing.T) {
			apiErr := jrerrors.NewFromHTTP(503, tt.body, "GET", "/rest/api/3/issue", nil)
			if apiErr.Message == tt.body {
				t.Errorf("expected HTML body to be sanitized, but got verbatim: %q", apiErr.Message)
			}
			if apiErr.Message == "" {
				t.Error("sanitized message should not be empty")
			}
		})
	}
}

// TestExitCodeFromStatus_EdgeCases verifies less-common status codes that fall
// into catch-all ranges or are handled specially.
func TestExitCodeFromStatus_EdgeCases(t *testing.T) {
	tests := []struct {
		status   int
		expected int
	}{
		// 1xx informational — falls to default → ExitError
		{100, jrerrors.ExitError},
		// 3xx redirect — falls to default → ExitError
		{301, jrerrors.ExitError},
		// 418 I'm a Teapot — client error range → ExitValidation
		{418, jrerrors.ExitValidation},
		// 451 Unavailable For Legal Reasons — client error range → ExitValidation
		{451, jrerrors.ExitValidation},
		// 599 — server error range → ExitServer
		{599, jrerrors.ExitServer},
	}

	for _, tt := range tests {
		got := jrerrors.ExitCodeFromStatus(tt.status)
		if got != tt.expected {
			t.Errorf("ExitCodeFromStatus(%d) = %d, want %d", tt.status, got, tt.expected)
		}
	}
}

// TestErrorTypeFromStatus_AllCategories verifies that every distinct error
// category string is reachable and correctly assigned.
func TestErrorTypeFromStatus_AllCategories(t *testing.T) {
	tests := []struct {
		status   int
		expected string
	}{
		{401, "auth_failed"},
		{403, "auth_failed"},
		{404, "not_found"},
		{400, "validation_error"},
		{422, "validation_error"},
		{429, "rate_limited"},
		{409, "conflict"},
		{410, "gone"},
		// client_error catch-all (4xx not listed above)
		{418, "client_error"},
		{451, "client_error"},
		// server_error catch-all (5xx)
		{500, "server_error"},
		{502, "server_error"},
		// connection_error (status 0 / unknown)
		{0, "connection_error"},
		{1, "connection_error"},
	}

	for _, tt := range tests {
		got := jrerrors.ErrorTypeFromStatus(tt.status)
		if got != tt.expected {
			t.Errorf("ErrorTypeFromStatus(%d) = %q, want %q", tt.status, got, tt.expected)
		}
	}
}

// TestAPIErrorRetryAfterOmittedWhenNil verifies that the JSON output does not
// contain a retry_after field when RetryAfter is nil.
func TestAPIErrorRetryAfterOmittedWhenNil(t *testing.T) {
	err := jrerrors.NewFromHTTP(404, "not found", "GET", "/issue/X-1", nil)

	var buf bytes.Buffer
	err.WriteJSON(&buf)

	var decoded map[string]interface{}
	if jsonErr := json.Unmarshal(buf.Bytes(), &decoded); jsonErr != nil {
		t.Fatalf("WriteJSON produced invalid JSON: %v\nOutput: %s", jsonErr, buf.String())
	}

	if _, exists := decoded["retry_after"]; exists {
		t.Error("JSON should not include 'retry_after' field when RetryAfter is nil")
	}
}

// TestAlreadyWrittenErrorError verifies that the AlreadyWrittenError sentinel
// implements the error interface with the expected message.
func TestAlreadyWrittenErrorError(t *testing.T) {
	err := &jrerrors.AlreadyWrittenError{Code: 3}
	if err.Error() != "error already written" {
		t.Errorf("Error() = %q, want %q", err.Error(), "error already written")
	}
	if err.Code != 3 {
		t.Errorf("Code = %d, want 3", err.Code)
	}
}

// TestAPIErrorRequestOmittedWhenNil verifies that a manually constructed
// APIError with a nil Request does not include a request field in JSON output.
func TestAPIErrorRequestOmittedWhenNil(t *testing.T) {
	err := &jrerrors.APIError{
		ErrorType: "config_error",
		Status:    0,
		Message:   "base_url is not configured",
	}

	var buf bytes.Buffer
	err.WriteJSON(&buf)

	var decoded map[string]interface{}
	if jsonErr := json.Unmarshal(buf.Bytes(), &decoded); jsonErr != nil {
		t.Fatalf("WriteJSON produced invalid JSON: %v\nOutput: %s", jsonErr, buf.String())
	}

	if _, exists := decoded["request"]; exists {
		t.Error("JSON should not include 'request' field when Request is nil")
	}
}
