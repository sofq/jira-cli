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
		{0, jrerrors.ExitError},
		{418, jrerrors.ExitError},
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
		{0, "connection_error"},
		{418, "connection_error"},
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
