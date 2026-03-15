package errors

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
)

// Exit code constants for structured error handling.
const (
	ExitOK         = 0
	ExitError      = 1
	ExitAuth       = 2
	ExitNotFound   = 3
	ExitValidation = 4
	ExitRateLimit  = 5
	ExitConflict   = 6
	ExitServer     = 7
)

// RequestInfo holds metadata about the HTTP request that triggered the error.
type RequestInfo struct {
	Method string `json:"method"`
	Path   string `json:"path"`
}

// APIError is a structured error with an HTTP status, typed error category,
// human-readable message, optional hint, and optional request context.
type APIError struct {
	ErrorType  string       `json:"error_type"`
	Status     int          `json:"status"`
	Message    string       `json:"message"`
	Hint       string       `json:"hint,omitempty"`
	Request    *RequestInfo `json:"request,omitempty"`
	RetryAfter *int         `json:"retry_after,omitempty"`
}

// Error implements the error interface.
func (e *APIError) Error() string {
	if e.Hint != "" {
		return fmt.Sprintf("%s (status %d): %s — %s", e.ErrorType, e.Status, e.Message, e.Hint)
	}
	return fmt.Sprintf("%s (status %d): %s", e.ErrorType, e.Status, e.Message)
}

// ExitCode returns the process exit code appropriate for this error.
func (e *APIError) ExitCode() int {
	return ExitCodeFromStatus(e.Status)
}

// WriteJSON encodes the error as a JSON object and writes it to w.
func (e *APIError) WriteJSON(w io.Writer) {
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(false)
	_ = enc.Encode(e)
}

// WriteStderr writes the JSON-encoded error to os.Stderr.
func (e *APIError) WriteStderr() {
	e.WriteJSON(os.Stderr)
}

// ExitCodeFromStatus maps an HTTP status code to a structured exit code.
func ExitCodeFromStatus(status int) int {
	switch {
	case status == 401 || status == 403:
		return ExitAuth
	case status == 404:
		return ExitNotFound
	case status == 400 || status == 422:
		return ExitValidation
	case status == 429:
		return ExitRateLimit
	case status == 409:
		return ExitConflict
	case status >= 500 && status < 600:
		return ExitServer
	case status >= 200 && status < 300:
		return ExitOK
	default:
		return ExitError
	}
}

// ErrorTypeFromStatus maps an HTTP status code to a string error type.
func ErrorTypeFromStatus(status int) string {
	switch {
	case status == 401 || status == 403:
		return "auth_failed"
	case status == 404:
		return "not_found"
	case status == 400 || status == 422:
		return "validation_error"
	case status == 429:
		return "rate_limited"
	case status == 409:
		return "conflict"
	case status >= 500 && status < 600:
		return "server_error"
	default:
		return "connection_error"
	}
}

// HintFromStatus returns a helpful hint string for well-known error statuses.
// Returns an empty string for statuses that do not have a standard hint.
func HintFromStatus(status int) string {
	switch status {
	case 401:
		return "Run `jr auth login` to authenticate."
	case 403:
		return "You do not have permission to perform this action."
	case 429:
		return "You are being rate limited. Wait before retrying."
	default:
		return ""
	}
}

// NewFromHTTP constructs an APIError from an HTTP response status, body text,
// and the originating request method and path.
func NewFromHTTP(status int, body string, method, path string) *APIError {
	return &APIError{
		ErrorType: ErrorTypeFromStatus(status),
		Status:    status,
		Message:   body,
		Hint:      HintFromStatus(status),
		Request: &RequestInfo{
			Method: method,
			Path:   path,
		},
	}
}
