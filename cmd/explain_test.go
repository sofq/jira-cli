package cmd

import (
	"bytes"
	"encoding/json"
	"io"
	"os"
	"strings"
	"testing"

	jrerrors "github.com/sofq/jira-cli/internal/errors"
)

// runExplainWithArgs invokes runExplain with the given positional args,
// capturing stdout and stderr. It returns the captured stdout string and
// the error returned by runExplain.
func runExplainWithArgs(t *testing.T, args []string) (string, error) {
	t.Helper()

	// Capture stdout.
	oldStdout := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("failed to create pipe: %v", err)
	}
	os.Stdout = w

	// Suppress stderr writes during the call.
	oldStderr := os.Stderr
	devNull, _ := os.Open(os.DevNull)
	os.Stderr = devNull

	runErr := runExplain(nil, args)

	w.Close()
	os.Stdout = oldStdout
	os.Stderr = oldStderr
	devNull.Close()

	var buf bytes.Buffer
	_, _ = io.Copy(&buf, r)
	r.Close()

	return buf.String(), runErr
}

// runExplainWithStdin invokes runExplain with no positional args and the
// given string as stdin content.
func runExplainWithStdin(t *testing.T, input string) (string, error) {
	t.Helper()

	// Replace stdin with a pipe carrying our input.
	oldStdin := os.Stdin
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("failed to create stdin pipe: %v", err)
	}
	os.Stdin = r

	_, _ = w.WriteString(input)
	w.Close()

	// Capture stdout.
	oldStdout := os.Stdout
	outR, outW, err := os.Pipe()
	if err != nil {
		t.Fatalf("failed to create stdout pipe: %v", err)
	}
	os.Stdout = outW

	// Suppress stderr.
	oldStderr := os.Stderr
	devNull, _ := os.Open(os.DevNull)
	os.Stderr = devNull

	runErr := runExplain(nil, nil)

	outW.Close()
	os.Stdout = oldStdout
	os.Stdin = oldStdin
	os.Stderr = oldStderr
	devNull.Close()

	var buf bytes.Buffer
	_, _ = io.Copy(&buf, outR)
	outR.Close()

	return buf.String(), runErr
}

func TestExplain_AuthFailed(t *testing.T) {
	input := `{"error_type":"auth_failed","status":401,"message":"Unauthorized"}`

	out, err := runExplainWithArgs(t, []string{input})
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}

	var exp ErrorExplanation
	if jsonErr := json.Unmarshal([]byte(strings.TrimSpace(out)), &exp); jsonErr != nil {
		t.Fatalf("stdout is not valid JSON: %s", out)
	}

	if exp.ErrorType != "auth_failed" {
		t.Errorf("expected error_type=auth_failed, got %q", exp.ErrorType)
	}
	if exp.Retryable {
		t.Error("auth_failed should not be retryable")
	}
	if exp.ExitCode != jrerrors.ExitAuth {
		t.Errorf("expected exit_code=%d, got %d", jrerrors.ExitAuth, exp.ExitCode)
	}
	if len(exp.Causes) == 0 {
		t.Error("expected at least one cause")
	}
	if len(exp.Fixes) == 0 {
		t.Error("expected at least one fix")
	}
	// Fixes should mention jr configure.
	found := false
	for _, f := range exp.Fixes {
		if strings.Contains(f, "jr configure") {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected a fix mentioning `jr configure`")
	}
}

func TestExplain_AuthFailed_403(t *testing.T) {
	input := `{"error_type":"auth_failed","status":403,"message":"Forbidden"}`

	out, err := runExplainWithArgs(t, []string{input})
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}

	var exp ErrorExplanation
	if jsonErr := json.Unmarshal([]byte(strings.TrimSpace(out)), &exp); jsonErr != nil {
		t.Fatalf("stdout is not valid JSON: %s", out)
	}

	if exp.ErrorType != "auth_failed" {
		t.Errorf("expected error_type=auth_failed, got %q", exp.ErrorType)
	}
	// For 403, an additional permission fix should be mentioned.
	found := false
	for _, f := range exp.Fixes {
		if strings.Contains(f, "403") || strings.Contains(f, "permission") || strings.Contains(f, "Forbidden") {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected a fix mentioning 403/permission for 403 status, got fixes: %v", exp.Fixes)
	}
}

func TestExplain_RateLimited_WithRetryAfter(t *testing.T) {
	ra := 30
	input, _ := json.Marshal(map[string]interface{}{
		"error_type":  "rate_limited",
		"status":      429,
		"message":     "Too Many Requests",
		"retry_after": ra,
	})

	out, err := runExplainWithArgs(t, []string{string(input)})
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}

	var exp ErrorExplanation
	if jsonErr := json.Unmarshal([]byte(strings.TrimSpace(out)), &exp); jsonErr != nil {
		t.Fatalf("stdout is not valid JSON: %s", out)
	}

	if exp.ErrorType != "rate_limited" {
		t.Errorf("expected error_type=rate_limited, got %q", exp.ErrorType)
	}
	if !exp.Retryable {
		t.Error("rate_limited should be retryable")
	}
	if exp.ExitCode != jrerrors.ExitRateLimit {
		t.Errorf("expected exit_code=%d, got %d", jrerrors.ExitRateLimit, exp.ExitCode)
	}
	// Summary should mention the retry_after value.
	if !strings.Contains(exp.Summary, "30") {
		t.Errorf("expected summary to mention retry delay (30s), got: %q", exp.Summary)
	}
	// First fix should be the retry-after-specific one.
	if len(exp.Fixes) == 0 || !strings.Contains(exp.Fixes[0], "30") {
		t.Errorf("expected first fix to mention 30 seconds, got fixes: %v", exp.Fixes)
	}
}

func TestExplain_RateLimited_NoRetryAfter(t *testing.T) {
	input := `{"error_type":"rate_limited","status":429,"message":"Too Many Requests"}`

	out, err := runExplainWithArgs(t, []string{input})
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}

	var exp ErrorExplanation
	if jsonErr := json.Unmarshal([]byte(strings.TrimSpace(out)), &exp); jsonErr != nil {
		t.Fatalf("stdout is not valid JSON: %s", out)
	}

	if !exp.Retryable {
		t.Error("rate_limited should be retryable")
	}
}

func TestExplain_NotFound(t *testing.T) {
	input := `{"error_type":"not_found","status":404,"message":"Issue Does Not Exist"}`

	out, err := runExplainWithArgs(t, []string{input})
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}

	var exp ErrorExplanation
	if jsonErr := json.Unmarshal([]byte(strings.TrimSpace(out)), &exp); jsonErr != nil {
		t.Fatalf("stdout is not valid JSON: %s", out)
	}

	if exp.ErrorType != "not_found" {
		t.Errorf("expected error_type=not_found, got %q", exp.ErrorType)
	}
	if exp.Retryable {
		t.Error("not_found should not be retryable")
	}
	if exp.ExitCode != jrerrors.ExitNotFound {
		t.Errorf("expected exit_code=%d, got %d", jrerrors.ExitNotFound, exp.ExitCode)
	}
}

func TestExplain_ValidationError(t *testing.T) {
	input := `{"error_type":"validation_error","status":400,"message":"Summary is required"}`

	out, err := runExplainWithArgs(t, []string{input})
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}

	var exp ErrorExplanation
	if jsonErr := json.Unmarshal([]byte(strings.TrimSpace(out)), &exp); jsonErr != nil {
		t.Fatalf("stdout is not valid JSON: %s", out)
	}

	if exp.ErrorType != "validation_error" {
		t.Errorf("expected error_type=validation_error, got %q", exp.ErrorType)
	}
	if exp.Retryable {
		t.Error("validation_error should not be retryable")
	}
	if exp.ExitCode != jrerrors.ExitValidation {
		t.Errorf("expected exit_code=%d, got %d", jrerrors.ExitValidation, exp.ExitCode)
	}
}

func TestExplain_Conflict(t *testing.T) {
	input := `{"error_type":"conflict","status":409,"message":"Edit conflict"}`

	out, err := runExplainWithArgs(t, []string{input})
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}

	var exp ErrorExplanation
	if jsonErr := json.Unmarshal([]byte(strings.TrimSpace(out)), &exp); jsonErr != nil {
		t.Fatalf("stdout is not valid JSON: %s", out)
	}

	if exp.ErrorType != "conflict" {
		t.Errorf("expected error_type=conflict, got %q", exp.ErrorType)
	}
	if !exp.Retryable {
		t.Error("conflict should be retryable")
	}
	if exp.ExitCode != jrerrors.ExitConflict {
		t.Errorf("expected exit_code=%d, got %d", jrerrors.ExitConflict, exp.ExitCode)
	}
}

func TestExplain_ServerError(t *testing.T) {
	input := `{"error_type":"server_error","status":500,"message":"Internal Server Error"}`

	out, err := runExplainWithArgs(t, []string{input})
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}

	var exp ErrorExplanation
	if jsonErr := json.Unmarshal([]byte(strings.TrimSpace(out)), &exp); jsonErr != nil {
		t.Fatalf("stdout is not valid JSON: %s", out)
	}

	if exp.ErrorType != "server_error" {
		t.Errorf("expected error_type=server_error, got %q", exp.ErrorType)
	}
	if !exp.Retryable {
		t.Error("server_error should be retryable")
	}
	if exp.ExitCode != jrerrors.ExitServer {
		t.Errorf("expected exit_code=%d, got %d", jrerrors.ExitServer, exp.ExitCode)
	}
}

func TestExplain_ConnectionError(t *testing.T) {
	input := `{"error_type":"connection_error","status":0,"message":"dial tcp: connection refused"}`

	out, err := runExplainWithArgs(t, []string{input})
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}

	var exp ErrorExplanation
	if jsonErr := json.Unmarshal([]byte(strings.TrimSpace(out)), &exp); jsonErr != nil {
		t.Fatalf("stdout is not valid JSON: %s", out)
	}

	if exp.ErrorType != "connection_error" {
		t.Errorf("expected error_type=connection_error, got %q", exp.ErrorType)
	}
	if !exp.Retryable {
		t.Error("connection_error should be retryable")
	}
}

func TestExplain_UnknownErrorType_Fallback(t *testing.T) {
	input := `{"error_type":"some_unknown_type","status":0,"message":"something went wrong"}`

	out, err := runExplainWithArgs(t, []string{input})
	if err != nil {
		t.Fatalf("expected no error for unknown type, got: %v", err)
	}

	var exp ErrorExplanation
	if jsonErr := json.Unmarshal([]byte(strings.TrimSpace(out)), &exp); jsonErr != nil {
		t.Fatalf("stdout is not valid JSON: %s", out)
	}

	if exp.ErrorType != "some_unknown_type" {
		t.Errorf("expected error_type to be preserved, got %q", exp.ErrorType)
	}
	if len(exp.Fixes) == 0 {
		t.Error("expected at least one fix in fallback case")
	}
}

func TestExplain_FromStdin(t *testing.T) {
	input := `{"error_type":"auth_failed","status":401,"message":"Unauthorized"}`

	out, err := runExplainWithStdin(t, input)
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}

	var exp ErrorExplanation
	if jsonErr := json.Unmarshal([]byte(strings.TrimSpace(out)), &exp); jsonErr != nil {
		t.Fatalf("stdout is not valid JSON: %s", out)
	}

	if exp.ErrorType != "auth_failed" {
		t.Errorf("expected error_type=auth_failed, got %q", exp.ErrorType)
	}
}

func TestExplain_InvalidJSON(t *testing.T) {
	input := `this is not json at all`

	_, err := runExplainWithArgs(t, []string{input})
	if err == nil {
		t.Fatal("expected error for invalid JSON, got nil")
	}
	aw, ok := err.(*jrerrors.AlreadyWrittenError)
	if !ok {
		t.Fatalf("expected AlreadyWrittenError, got %T: %v", err, err)
	}
	if aw.Code != jrerrors.ExitValidation {
		t.Errorf("expected exit code %d, got %d", jrerrors.ExitValidation, aw.Code)
	}
}

func TestExplain_EmptyObject(t *testing.T) {
	// Valid JSON but missing both error_type and message — should be rejected.
	input := `{}`

	_, err := runExplainWithArgs(t, []string{input})
	if err == nil {
		t.Fatal("expected error for empty JSON object, got nil")
	}
	aw, ok := err.(*jrerrors.AlreadyWrittenError)
	if !ok {
		t.Fatalf("expected AlreadyWrittenError, got %T: %v", err, err)
	}
	if aw.Code != jrerrors.ExitValidation {
		t.Errorf("expected exit code %d, got %d", jrerrors.ExitValidation, aw.Code)
	}
}

func TestExplain_StatusOnlyNormalisesErrorType(t *testing.T) {
	// Error with status but no explicit error_type — type should be inferred.
	input := `{"status":401,"message":"Unauthorized"}`

	out, err := runExplainWithArgs(t, []string{input})
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}

	var exp ErrorExplanation
	if jsonErr := json.Unmarshal([]byte(strings.TrimSpace(out)), &exp); jsonErr != nil {
		t.Fatalf("stdout is not valid JSON: %s", out)
	}

	if exp.ErrorType != "auth_failed" {
		t.Errorf("expected error_type inferred from status 401 to be auth_failed, got %q", exp.ErrorType)
	}
}
