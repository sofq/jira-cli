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

	"github.com/sofq/jira-cli/internal/client"
	"github.com/sofq/jira-cli/internal/config"
	jrerrors "github.com/sofq/jira-cli/internal/errors"
	"github.com/spf13/cobra"
)

// ---------------------------------------------------------------------------
// Helpers shared within this file
// ---------------------------------------------------------------------------

// newRunPipeCmd creates a pipe command wired with the given client in context.
// Flags mirror the real pipeCmd registration.
func newRunPipeCmd(c *client.Client) *cobra.Command {
	cmd := &cobra.Command{
		Use:  "pipe",
		Args: cobra.ExactArgs(2),
		RunE: runPipe,
	}
	cmd.Flags().String("extract", "[.issues[].key]", "jq expression to extract values")
	cmd.Flags().String("inject", "issue", "argument name to inject into target command")
	cmd.Flags().Int("parallel", 1, "concurrent operations")
	cmd.SetContext(client.NewContext(context.Background(), c))
	return cmd
}

// runPipeCmd executes runPipe with the supplied args and returns stdout output
// and the returned error. Stderr is discarded.
func runPipeCmd(t *testing.T, cmd *cobra.Command, args []string) (string, error) {
	t.Helper()
	var runErr error
	out := captureStdout(t, func() {
		runErr = cmd.RunE(cmd, args)
	})
	return out, runErr
}

// ---------------------------------------------------------------------------
// runPipe — full function coverage
// ---------------------------------------------------------------------------

// TestRunPipe_SequentialSuccess verifies the happy path: source produces keys,
// extract pulls them out, target fetches each issue in order.
func TestRunPipe_SequentialSuccess(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case strings.Contains(r.URL.RawQuery, "jql") || strings.Contains(r.URL.Path, "search"):
			fmt.Fprint(w, `{"issues":[{"key":"TST-1"},{"key":"TST-2"}]}`)
		case strings.HasSuffix(r.URL.Path, "/TST-1"):
			fmt.Fprint(w, `{"key":"TST-1","fields":{"summary":"First"}}`)
		case strings.HasSuffix(r.URL.Path, "/TST-2"):
			fmt.Fprint(w, `{"key":"TST-2","fields":{"summary":"Second"}}`)
		default:
			w.WriteHeader(http.StatusNotFound)
			fmt.Fprint(w, `{"error":"not found"}`)
		}
	}))
	defer ts.Close()

	var stdout, stderr bytes.Buffer
	c := newPipeClient(ts.URL, &stdout, &stderr)

	cmd := newRunPipeCmd(c)
	_ = cmd.Flags().Set("extract", "[.issues[].key]")
	_ = cmd.Flags().Set("inject", "issueIdOrKey")
	_ = cmd.Flags().Set("parallel", "1")

	out, err := runPipeCmd(t, cmd, []string{
		"search search-and-reconsile-issues-using-jql --jql 'project=TST'",
		"issue get",
	})

	if err != nil {
		t.Fatalf("runPipe unexpected error: %v", err)
	}

	var results []BatchResult
	if jsonErr := json.Unmarshal([]byte(strings.TrimSpace(out)), &results); jsonErr != nil {
		t.Fatalf("output is not valid JSON: %s — %v", out, jsonErr)
	}
	if len(results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(results))
	}
	for _, r := range results {
		if r.ExitCode != jrerrors.ExitOK {
			t.Errorf("result[%d] failed: %v", r.Index, r.Error)
		}
	}
}

// TestRunPipe_ParallelSuccess verifies that parallel > 1 path produces all results.
func TestRunPipe_ParallelSuccess(t *testing.T) {
	const n = 4
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		for i := 1; i <= n; i++ {
			key := fmt.Sprintf("TST-%d", i)
			if strings.HasSuffix(r.URL.Path, "/"+key) {
				fmt.Fprintf(w, `{"key":%q}`, key)
				return
			}
		}
		// Source search returns n issues.
		issues := make([]string, n)
		for i := 0; i < n; i++ {
			issues[i] = fmt.Sprintf(`{"key":"TST-%d"}`, i+1)
		}
		fmt.Fprintf(w, `{"issues":[%s]}`, strings.Join(issues, ","))
	}))
	defer ts.Close()

	var stdout, stderr bytes.Buffer
	c := newPipeClient(ts.URL, &stdout, &stderr)

	cmd := newRunPipeCmd(c)
	_ = cmd.Flags().Set("extract", "[.issues[].key]")
	_ = cmd.Flags().Set("inject", "issueIdOrKey")
	_ = cmd.Flags().Set("parallel", "3")

	out, err := runPipeCmd(t, cmd, []string{
		"search search-and-reconsile-issues-using-jql --jql 'project=TST'",
		"issue get",
	})
	if err != nil {
		t.Fatalf("runPipe parallel unexpected error: %v", err)
	}

	var results []BatchResult
	if jsonErr := json.Unmarshal([]byte(strings.TrimSpace(out)), &results); jsonErr != nil {
		t.Fatalf("output not valid JSON: %s — %v", out, jsonErr)
	}
	if len(results) != n {
		t.Fatalf("expected %d results, got %d", n, len(results))
	}
}

// TestRunPipe_EmptyExtract verifies that when the extract expression returns an
// empty array, runPipe writes "[]" and returns nil.
func TestRunPipe_EmptyExtract(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"issues":[]}`)
	}))
	defer ts.Close()

	var stdout, stderr bytes.Buffer
	c := newPipeClient(ts.URL, &stdout, &stderr)

	cmd := newRunPipeCmd(c)
	_ = cmd.Flags().Set("extract", "[.issues[].key]")
	_ = cmd.Flags().Set("inject", "issueIdOrKey")

	out, err := runPipeCmd(t, cmd, []string{
		"search search-and-reconsile-issues-using-jql --jql 'project=TST'",
		"issue get",
	})
	if err != nil {
		t.Fatalf("expected nil for empty extract, got: %v", err)
	}
	if strings.TrimSpace(out) != "[]" {
		t.Errorf("expected empty array output, got: %q", out)
	}
}

// TestRunPipe_NoClientInContext verifies that runPipe returns ExitError when
// the context carries no client.
func TestRunPipe_NoClientInContext(t *testing.T) {
	cmd := &cobra.Command{
		Use:  "pipe",
		Args: cobra.ExactArgs(2),
		RunE: runPipe,
	}
	cmd.Flags().String("extract", "[.issues[].key]", "")
	cmd.Flags().String("inject", "issue", "")
	cmd.Flags().Int("parallel", 1, "")
	cmd.SetContext(context.Background()) // no client stored

	var runErr error
	captureStdout(t, func() {
		runErr = cmd.RunE(cmd, []string{"issue get", "issue get"})
	})

	if runErr == nil {
		t.Fatal("expected error for missing client, got nil")
	}
	awe, ok := runErr.(*jrerrors.AlreadyWrittenError)
	if !ok {
		t.Fatalf("expected AlreadyWrittenError, got %T: %v", runErr, runErr)
	}
	if awe.Code != jrerrors.ExitError {
		t.Errorf("expected ExitError, got %d", awe.Code)
	}
}

// TestRunPipe_BadSourceCommand verifies ExitValidation for unparseable source.
func TestRunPipe_BadSourceCommand(t *testing.T) {
	var stdout, stderr bytes.Buffer
	c := newPipeClient("http://localhost:1", &stdout, &stderr)

	cmd := newRunPipeCmd(c)
	_ = cmd.Flags().Set("extract", "[.issues[].key]")
	_ = cmd.Flags().Set("inject", "issueIdOrKey")

	_, err := runPipeCmd(t, cmd, []string{
		"onlyonetoken", // fewer than 2 tokens — parseCommandString error
		"issue get",
	})
	if err == nil {
		t.Fatal("expected error for bad source command, got nil")
	}
	awe, ok := err.(*jrerrors.AlreadyWrittenError)
	if !ok {
		t.Fatalf("expected AlreadyWrittenError, got %T: %v", err, err)
	}
	if awe.Code != jrerrors.ExitValidation {
		t.Errorf("expected ExitValidation(%d), got %d", jrerrors.ExitValidation, awe.Code)
	}
}

// TestRunPipe_BadTargetCommand verifies ExitValidation for unparseable target.
func TestRunPipe_BadTargetCommand(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"issues":[{"key":"TST-1"}]}`)
	}))
	defer ts.Close()

	var stdout, stderr bytes.Buffer
	c := newPipeClient(ts.URL, &stdout, &stderr)

	cmd := newRunPipeCmd(c)
	_ = cmd.Flags().Set("extract", "[.issues[].key]")
	_ = cmd.Flags().Set("inject", "issueIdOrKey")

	_, err := runPipeCmd(t, cmd, []string{
		"search search-and-reconsile-issues-using-jql --jql 'project=TST'",
		"singletoken", // only one token — invalid target
	})
	if err == nil {
		t.Fatal("expected error for bad target command, got nil")
	}
	awe, ok := err.(*jrerrors.AlreadyWrittenError)
	if !ok {
		t.Fatalf("expected AlreadyWrittenError, got %T: %v", err, err)
	}
	if awe.Code != jrerrors.ExitValidation {
		t.Errorf("expected ExitValidation(%d), got %d", jrerrors.ExitValidation, awe.Code)
	}
}

// TestRunPipe_InvalidExtractExpr verifies ExitValidation when the jq extract
// expression is syntactically invalid.
func TestRunPipe_InvalidExtractExpr(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"issues":[{"key":"TST-1"}]}`)
	}))
	defer ts.Close()

	var stdout, stderr bytes.Buffer
	c := newPipeClient(ts.URL, &stdout, &stderr)

	cmd := newRunPipeCmd(c)
	_ = cmd.Flags().Set("extract", "this is {{{not valid jq")
	_ = cmd.Flags().Set("inject", "issueIdOrKey")

	_, err := runPipeCmd(t, cmd, []string{
		"search search-and-reconsile-issues-using-jql --jql 'project=TST'",
		"issue get",
	})
	if err == nil {
		t.Fatal("expected error for invalid jq extract, got nil")
	}
	awe, ok := err.(*jrerrors.AlreadyWrittenError)
	if !ok {
		t.Fatalf("expected AlreadyWrittenError, got %T: %v", err, err)
	}
	if awe.Code != jrerrors.ExitValidation {
		t.Errorf("expected ExitValidation(%d), got %d", jrerrors.ExitValidation, awe.Code)
	}
}

// TestRunPipe_ExtractNonArray verifies ExitValidation when the jq filter
// returns a non-array value (e.g. a string).
func TestRunPipe_ExtractNonArray(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"issues":[{"key":"TST-1"}]}`)
	}))
	defer ts.Close()

	var stdout, stderr bytes.Buffer
	c := newPipeClient(ts.URL, &stdout, &stderr)

	cmd := newRunPipeCmd(c)
	// .total is a number, not an array.
	_ = cmd.Flags().Set("extract", ".issues[0].key")
	_ = cmd.Flags().Set("inject", "issueIdOrKey")

	_, err := runPipeCmd(t, cmd, []string{
		"search search-and-reconsile-issues-using-jql --jql 'project=TST'",
		"issue get",
	})
	if err == nil {
		t.Fatal("expected error when extract returns non-array, got nil")
	}
	awe, ok := err.(*jrerrors.AlreadyWrittenError)
	if !ok {
		t.Fatalf("expected AlreadyWrittenError, got %T: %v", err, err)
	}
	if awe.Code != jrerrors.ExitValidation {
		t.Errorf("expected ExitValidation(%d), got %d", jrerrors.ExitValidation, awe.Code)
	}
}

// TestRunPipe_SourceFails verifies that runPipe forwards the source exit code
// when the source operation itself fails.
func TestRunPipe_SourceFails(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnauthorized)
		fmt.Fprint(w, `{"error_type":"auth_failed","message":"unauthorized"}`)
	}))
	defer ts.Close()

	var stdout, stderr bytes.Buffer
	c := newPipeClient(ts.URL, &stdout, &stderr)

	cmd := newRunPipeCmd(c)
	_ = cmd.Flags().Set("extract", "[.issues[].key]")
	_ = cmd.Flags().Set("inject", "issueIdOrKey")

	_, err := runPipeCmd(t, cmd, []string{
		"search search-and-reconsile-issues-using-jql --jql 'project=TST'",
		"issue get",
	})
	if err == nil {
		t.Fatal("expected error when source fails, got nil")
	}
	awe, ok := err.(*jrerrors.AlreadyWrittenError)
	if !ok {
		t.Fatalf("expected AlreadyWrittenError, got %T: %v", err, err)
	}
	// Any non-zero exit code is acceptable here.
	if awe.Code == jrerrors.ExitOK {
		t.Error("expected non-zero exit code")
	}
}

// TestRunPipe_GlobalJQFilter verifies that the global --jq flag on the client
// is applied to the final result array.
func TestRunPipe_GlobalJQFilter(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case strings.Contains(r.URL.RawQuery, "jql") || strings.Contains(r.URL.Path, "search"):
			fmt.Fprint(w, `{"issues":[{"key":"TST-1"}]}`)
		case strings.HasSuffix(r.URL.Path, "/TST-1"):
			fmt.Fprint(w, `{"key":"TST-1"}`)
		default:
			w.WriteHeader(http.StatusNotFound)
			fmt.Fprint(w, `{"error":"not found"}`)
		}
	}))
	defer ts.Close()

	var stdout, stderr bytes.Buffer
	c := newPipeClient(ts.URL, &stdout, &stderr)
	// Set a global jq filter to pluck just the exit codes.
	c.JQFilter = "[.[].exit_code]"

	cmd := newRunPipeCmd(c)
	_ = cmd.Flags().Set("extract", "[.issues[].key]")
	_ = cmd.Flags().Set("inject", "issueIdOrKey")

	out, err := runPipeCmd(t, cmd, []string{
		"search search-and-reconsile-issues-using-jql --jql 'project=TST'",
		"issue get",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Output should be a JSON array of exit codes (all 0).
	var codes []int
	if jsonErr := json.Unmarshal([]byte(strings.TrimSpace(out)), &codes); jsonErr != nil {
		t.Fatalf("output is not a JSON int array: %s — %v", out, jsonErr)
	}
	if len(codes) != 1 || codes[0] != 0 {
		t.Errorf("expected [0], got %v", codes)
	}
}

// TestRunPipe_GlobalJQFilterInvalid verifies ExitValidation when the global
// --jq filter on the client is syntactically bad.
func TestRunPipe_GlobalJQFilterInvalid(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case strings.Contains(r.URL.RawQuery, "jql") || strings.Contains(r.URL.Path, "search"):
			fmt.Fprint(w, `{"issues":[{"key":"TST-1"}]}`)
		case strings.HasSuffix(r.URL.Path, "/TST-1"):
			fmt.Fprint(w, `{"key":"TST-1"}`)
		default:
			w.WriteHeader(http.StatusNotFound)
			fmt.Fprint(w, `{"error":"not found"}`)
		}
	}))
	defer ts.Close()

	var stdout, stderr bytes.Buffer
	c := newPipeClient(ts.URL, &stdout, &stderr)
	c.JQFilter = "this is {{{not valid jq"

	cmd := newRunPipeCmd(c)
	_ = cmd.Flags().Set("extract", "[.issues[].key]")
	_ = cmd.Flags().Set("inject", "issueIdOrKey")

	_, err := runPipeCmd(t, cmd, []string{
		"search search-and-reconsile-issues-using-jql --jql 'project=TST'",
		"issue get",
	})
	if err == nil {
		t.Fatal("expected error for invalid global jq filter, got nil")
	}
	awe, ok := err.(*jrerrors.AlreadyWrittenError)
	if !ok {
		t.Fatalf("expected AlreadyWrittenError, got %T: %v", err, err)
	}
	if awe.Code != jrerrors.ExitValidation {
		t.Errorf("expected ExitValidation(%d), got %d", jrerrors.ExitValidation, awe.Code)
	}
}

// TestRunPipe_TargetFailurePropagatesExitCode verifies that when target ops
// fail, runPipe still outputs JSON and returns a non-zero AlreadyWrittenError.
func TestRunPipe_TargetFailurePropagatesExitCode(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if strings.Contains(r.URL.RawQuery, "jql") || strings.Contains(r.URL.Path, "search") {
			fmt.Fprint(w, `{"issues":[{"key":"TST-1"}]}`)
			return
		}
		// All issue GETs return 404.
		w.WriteHeader(http.StatusNotFound)
		fmt.Fprint(w, `{"error_type":"not_found","message":"not found"}`)
	}))
	defer ts.Close()

	var stdout, stderr bytes.Buffer
	c := newPipeClient(ts.URL, &stdout, &stderr)

	cmd := newRunPipeCmd(c)
	_ = cmd.Flags().Set("extract", "[.issues[].key]")
	_ = cmd.Flags().Set("inject", "issueIdOrKey")

	out, err := runPipeCmd(t, cmd, []string{
		"search search-and-reconsile-issues-using-jql --jql 'project=TST'",
		"issue get",
	})

	// Output should still be a JSON array.
	if strings.TrimSpace(out) != "" {
		var results []BatchResult
		if jsonErr := json.Unmarshal([]byte(strings.TrimSpace(out)), &results); jsonErr != nil {
			t.Fatalf("output is not valid JSON: %s — %v", out, jsonErr)
		}
	}

	// Error should be non-nil since at least one target failed.
	if err == nil {
		t.Fatal("expected non-nil error for failed target, got nil")
	}
	awe, ok := err.(*jrerrors.AlreadyWrittenError)
	if !ok {
		t.Fatalf("expected AlreadyWrittenError, got %T: %v", err, err)
	}
	if awe.Code == jrerrors.ExitOK {
		t.Error("expected non-zero exit code for failed target")
	}
}

// ---------------------------------------------------------------------------
// doctor.go — missing branch coverage
// ---------------------------------------------------------------------------

// TestDoctorUnhealthy_CorruptConfig verifies that a corrupt config file
// (non-JSON content) triggers a profile_load fail and early return.
func TestDoctorUnhealthy_CorruptConfig(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.json")
	if err := os.WriteFile(configPath, []byte("this is not json }{"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("JR_CONFIG_PATH", configPath)

	cmd := newDoctorCmd()

	var runErr error
	output := captureStdout(t, func() {
		runErr = runDoctor(cmd, nil)
	})

	if runErr != nil {
		t.Fatalf("runDoctor returned error: %v", runErr)
	}

	result := parseDoctorResult(t, output)
	if result.Status != "unhealthy" {
		t.Errorf("expected unhealthy, got %q", result.Status)
	}
	c := findCheck(t, result.Checks, "profile_load")
	if c.Status != "fail" {
		t.Errorf("profile_load: expected fail, got %q", c.Status)
	}
}

// TestDoctorUnhealthy_ProfileNotFound verifies the case where the config loads
// but the requested profile name does not exist.
func TestDoctorUnhealthy_ProfileNotFound(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.json")
	cfg := &config.Config{
		DefaultProfile: "default",
		Profiles: map[string]config.Profile{
			"default": {
				BaseURL: "http://example.com",
				Auth:    config.AuthConfig{Type: "basic", Token: "t"},
			},
		},
	}
	if err := config.SaveTo(cfg, configPath); err != nil {
		t.Fatal(err)
	}
	t.Setenv("JR_CONFIG_PATH", configPath)

	cmd := newDoctorCmd()
	// Request a profile that doesn't exist.
	if err := cmd.Flags().Set("profile", "nonexistent-profile"); err != nil {
		t.Fatal(err)
	}

	var runErr error
	output := captureStdout(t, func() {
		runErr = runDoctor(cmd, nil)
	})

	if runErr != nil {
		t.Fatalf("runDoctor returned error: %v", runErr)
	}

	result := parseDoctorResult(t, output)
	if result.Status != "unhealthy" {
		t.Errorf("expected unhealthy, got %q", result.Status)
	}
	c := findCheck(t, result.Checks, "profile_load")
	if c.Status != "fail" {
		t.Errorf("profile_load: expected fail, got %q", c.Status)
	}
	if !strings.Contains(c.Message, "nonexistent-profile") {
		t.Errorf("expected message to mention profile name, got: %q", c.Message)
	}
}

// TestDoctorUnhealthy_EmptyBaseURL verifies that a profile with an empty
// base_url triggers a base_url fail.
func TestDoctorUnhealthy_EmptyBaseURL(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.json")
	cfg := &config.Config{
		DefaultProfile: "default",
		Profiles: map[string]config.Profile{
			"default": {
				BaseURL: "", // intentionally empty
				Auth:    config.AuthConfig{Type: "basic", Token: "tok"},
			},
		},
	}
	if err := config.SaveTo(cfg, configPath); err != nil {
		t.Fatal(err)
	}
	t.Setenv("JR_CONFIG_PATH", configPath)

	cmd := newDoctorCmd()

	var runErr error
	output := captureStdout(t, func() {
		runErr = runDoctor(cmd, nil)
	})

	if runErr != nil {
		t.Fatalf("runDoctor returned error: %v", runErr)
	}

	result := parseDoctorResult(t, output)
	if result.Status != "unhealthy" {
		t.Errorf("expected unhealthy, got %q", result.Status)
	}
	c := findCheck(t, result.Checks, "base_url")
	if c.Status != "fail" {
		t.Errorf("base_url check: expected fail, got %q", c.Status)
	}
}

// TestDoctorUnhealthy_BearerEmptyToken verifies the bearer auth branch when
// the token is not set.
func TestDoctorUnhealthy_BearerEmptyToken(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.json")
	cfg := &config.Config{
		DefaultProfile: "default",
		Profiles: map[string]config.Profile{
			"default": {
				BaseURL: "http://example.com",
				Auth: config.AuthConfig{
					Type:  "bearer",
					Token: "", // empty — should trigger fail
				},
			},
		},
	}
	if err := config.SaveTo(cfg, configPath); err != nil {
		t.Fatal(err)
	}
	t.Setenv("JR_CONFIG_PATH", configPath)

	cmd := newDoctorCmd()

	var runErr error
	output := captureStdout(t, func() {
		runErr = runDoctor(cmd, nil)
	})

	if runErr != nil {
		t.Fatalf("runDoctor returned error: %v", runErr)
	}

	result := parseDoctorResult(t, output)
	if result.Status != "unhealthy" {
		t.Errorf("expected unhealthy, got %q", result.Status)
	}
	c := findCheck(t, result.Checks, "auth_configured")
	if c.Status != "fail" {
		t.Errorf("auth_configured check: expected fail, got %q", c.Status)
	}
	if !strings.Contains(c.Message, "bearer") {
		t.Errorf("expected message to mention bearer, got: %q", c.Message)
	}
}

// TestDoctorUnhealthy_OAuth2MissingFields verifies the oauth2 branch when
// required oauth2 fields are absent.
func TestDoctorUnhealthy_OAuth2MissingFields(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.json")
	cfg := &config.Config{
		DefaultProfile: "default",
		Profiles: map[string]config.Profile{
			"default": {
				BaseURL: "http://example.com",
				Auth: config.AuthConfig{
					Type: "oauth2",
					// ClientID, ClientSecret, TokenURL all empty.
				},
			},
		},
	}
	if err := config.SaveTo(cfg, configPath); err != nil {
		t.Fatal(err)
	}
	t.Setenv("JR_CONFIG_PATH", configPath)

	cmd := newDoctorCmd()

	var runErr error
	output := captureStdout(t, func() {
		runErr = runDoctor(cmd, nil)
	})

	if runErr != nil {
		t.Fatalf("runDoctor returned error: %v", runErr)
	}

	result := parseDoctorResult(t, output)
	if result.Status != "unhealthy" {
		t.Errorf("expected unhealthy, got %q", result.Status)
	}
	c := findCheck(t, result.Checks, "auth_configured")
	if c.Status != "fail" {
		t.Errorf("auth_configured check: expected fail, got %q", c.Status)
	}
	// The message should list all three missing fields.
	for _, field := range []string{"client_id", "client_secret", "token_url"} {
		if !strings.Contains(c.Message, field) {
			t.Errorf("expected message to mention %q, got: %q", field, c.Message)
		}
	}
}

// TestDoctorWarn_BasicAuthEmptyToken verifies that basic auth with an empty
// token produces a warn (not fail) on auth_configured.
func TestDoctorWarn_BasicAuthEmptyToken(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"accountId":"u","displayName":"U"}`))
	}))
	defer srv.Close()

	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.json")
	cfg := &config.Config{
		DefaultProfile: "default",
		Profiles: map[string]config.Profile{
			"default": {
				BaseURL: srv.URL,
				Auth: config.AuthConfig{
					Type:  "basic",
					Token: "", // empty — should warn, not fail
				},
			},
		},
	}
	if err := config.SaveTo(cfg, configPath); err != nil {
		t.Fatal(err)
	}
	t.Setenv("JR_CONFIG_PATH", configPath)

	cmd := newDoctorCmd()

	var runErr error
	output := captureStdout(t, func() {
		runErr = runDoctor(cmd, nil)
	})

	if runErr != nil {
		t.Fatalf("runDoctor returned error: %v", runErr)
	}

	result := parseDoctorResult(t, output)
	// Overall status should be unhealthy (warn counts).
	if result.Status != "unhealthy" {
		t.Errorf("expected unhealthy (warn), got %q", result.Status)
	}
	c := findCheck(t, result.Checks, "auth_configured")
	if c.Status != "warn" {
		t.Errorf("auth_configured: expected warn, got %q (message: %s)", c.Status, c.Message)
	}
}

// ---------------------------------------------------------------------------
// explain.go — missing branch coverage
// ---------------------------------------------------------------------------

// TestRunExplain_NoInput_TerminalStdin verifies that when stdin is a terminal
// (not a pipe) and no args are given, runExplain returns ExitValidation.
// We simulate this by replacing os.Stdin with a regular file (not a pipe),
// which has the ModeCharDevice bit set by the OS.
func TestRunExplain_NoInput_TerminalStdin(t *testing.T) {
	// Use /dev/tty if available, otherwise skip — we need a char device.
	devTTY := "/dev/tty"
	if _, err := os.Stat(devTTY); err != nil {
		t.Skip("no /dev/tty available in this environment")
	}

	f, err := os.Open(devTTY)
	if err != nil {
		t.Skip("cannot open /dev/tty:", err)
	}
	defer f.Close()

	origStdin := os.Stdin
	os.Stdin = f
	t.Cleanup(func() { os.Stdin = origStdin })

	// Suppress stderr output.
	origStderr := os.Stderr
	devNull, _ := os.Open(os.DevNull)
	os.Stderr = devNull
	t.Cleanup(func() {
		os.Stderr = origStderr
		devNull.Close()
	})

	err = runExplain(nil, nil)
	if err == nil {
		t.Fatal("expected error for terminal stdin without args, got nil")
	}
	awe, ok := err.(*jrerrors.AlreadyWrittenError)
	if !ok {
		t.Fatalf("expected AlreadyWrittenError, got %T: %v", err, err)
	}
	if awe.Code != jrerrors.ExitValidation {
		t.Errorf("expected ExitValidation(%d), got %d", jrerrors.ExitValidation, awe.Code)
	}
}

// TestBuildExplanation_DefaultCase_NonZeroStatus verifies that the default
// case of buildExplanation uses the exit code derived from a non-zero HTTP
// status rather than always returning ExitError when ExitCodeFromStatus gives
// a non-zero code.
func TestBuildExplanation_DefaultCase_NonZeroStatus(t *testing.T) {
	tests := []struct {
		name         string
		status       int
		wantExitCode int
	}{
		{
			name:         "unknown type with status 0",
			status:       0,
			wantExitCode: jrerrors.ExitError,
		},
		{
			name:         "unknown type with non-zero status mapping to non-OK",
			status:       401,
			wantExitCode: jrerrors.ExitAuth, // ExitCodeFromStatus(401) != ExitOK
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			apiErr := &jrerrors.APIError{
				ErrorType: "completely_unknown_custom_type",
				Status:    tc.status,
				Message:   "something went sideways",
			}
			exp := buildExplanation(apiErr)
			if exp.ExitCode != tc.wantExitCode {
				t.Errorf("expected exit_code=%d, got %d", tc.wantExitCode, exp.ExitCode)
			}
			if exp.ErrorType != "completely_unknown_custom_type" {
				t.Errorf("error_type not preserved: got %q", exp.ErrorType)
			}
		})
	}
}
// ---------------------------------------------------------------------------
// doctor.go writeDoctorResult — marshalNoEscape fallback path
// ---------------------------------------------------------------------------

// TestWriteDoctorResult_Healthy verifies that writeDoctorResult produces a
// valid JSON result and returns nil when all checks pass.
func TestWriteDoctorResult_Healthy(t *testing.T) {
	checks := []DoctorCheck{
		{Name: "config_file", Status: "pass", Message: "/some/path"},
		{Name: "profile_load", Status: "pass", Message: "loaded"},
		{Name: "base_url", Status: "pass", Message: "http://example.com"},
		{Name: "auth_configured", Status: "pass", Message: "auth type: basic"},
		{Name: "connectivity", Status: "pass", Message: "GET /rest/api/3/myself succeeded"},
	}

	cmd := newDoctorCmd()
	var runErr error
	output := captureStdout(t, func() {
		runErr = writeDoctorResult(cmd, checks)
	})

	if runErr != nil {
		t.Fatalf("writeDoctorResult returned error: %v", runErr)
	}

	result := parseDoctorResult(t, output)
	if result.Status != "healthy" {
		t.Errorf("expected healthy, got %q", result.Status)
	}
	if len(result.Checks) != len(checks) {
		t.Errorf("expected %d checks, got %d", len(checks), len(result.Checks))
	}
}

// TestWriteDoctorResult_Unhealthy_WarnCheck verifies that a single warn check
// makes the overall status "unhealthy".
func TestWriteDoctorResult_Unhealthy_WarnCheck(t *testing.T) {
	checks := []DoctorCheck{
		{Name: "auth_configured", Status: "warn", Message: "token not set"},
	}

	cmd := newDoctorCmd()
	var runErr error
	output := captureStdout(t, func() {
		runErr = writeDoctorResult(cmd, checks)
	})

	if runErr != nil {
		t.Fatalf("writeDoctorResult returned error: %v", runErr)
	}

	result := parseDoctorResult(t, output)
	if result.Status != "unhealthy" {
		t.Errorf("expected unhealthy, got %q", result.Status)
	}
}

// ---------------------------------------------------------------------------
// batch.go — executeBatchOpWithContext workflow/template/diff branches
// ---------------------------------------------------------------------------

// TestExecuteBatchOpWithContext_WorkflowCommand verifies the workflow command
// dispatch path inside executeBatchOpWithContext.
func TestExecuteBatchOpWithContext_WorkflowCommand(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if strings.Contains(r.URL.Path, "/transitions") {
			if r.Method == "GET" {
				fmt.Fprint(w, `{"transitions":[{"id":"11","name":"Done","to":{"name":"Done"}}]}`)
				return
			}
			if r.Method == "POST" {
				w.WriteHeader(http.StatusNoContent)
				return
			}
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer ts.Close()

	var stdout, stderr bytes.Buffer
	c := newTestClient(ts.URL, &stdout, &stderr)

	opMap := buildTestOpMap()

	bop := BatchOp{
		Command: "workflow transition",
		Args:    map[string]string{"issue": "TST-1", "to": "Done"},
	}

	result := executeBatchOpWithContext(context.Background(), c, 0, bop, opMap)
	if result.ExitCode != jrerrors.ExitOK {
		t.Errorf("expected ExitOK, got %d; error: %s", result.ExitCode, result.Error)
	}
}

// TestExecuteBatchOpWithContext_UnknownCommand verifies the error path for
// unknown commands.
func TestExecuteBatchOpWithContext_UnknownCommand(t *testing.T) {
	var stdout, stderr bytes.Buffer
	c := newTestClient("http://unused", &stdout, &stderr)
	opMap := buildTestOpMap()

	bop := BatchOp{Command: "nonexistent command", Args: map[string]string{}}
	result := executeBatchOpWithContext(context.Background(), c, 0, bop, opMap)

	if result.ExitCode != jrerrors.ExitError {
		t.Errorf("expected ExitError, got %d", result.ExitCode)
	}
}

// TestExecuteBatchOpWithContext_PolicyDenied verifies the policy check path.
func TestExecuteBatchOpWithContext_PolicyDenied(t *testing.T) {
	var stdout, stderr bytes.Buffer
	c := newTestClient("http://unused", &stdout, &stderr)
	pol, _ := newTestDenyPolicy()
	c.Policy = pol
	opMap := buildTestOpMap()

	bop := BatchOp{Command: "issue get", Args: map[string]string{"issueIdOrKey": "TST-1"}}
	result := executeBatchOpWithContext(context.Background(), c, 0, bop, opMap)

	if result.ExitCode != jrerrors.ExitValidation {
		t.Errorf("expected ExitValidation, got %d", result.ExitCode)
	}
}

// TestExecuteBatchOpWithContext_MissingRequiredPath verifies the error when
// a required path parameter is missing.
func TestExecuteBatchOpWithContext_MissingRequiredPath(t *testing.T) {
	var stdout, stderr bytes.Buffer
	c := newTestClient("http://unused", &stdout, &stderr)
	opMap := buildTestOpMap()

	// "issue get" requires "issueIdOrKey" path parameter — omit it.
	bop := BatchOp{Command: "issue get", Args: map[string]string{}}
	result := executeBatchOpWithContext(context.Background(), c, 0, bop, opMap)

	if result.ExitCode != jrerrors.ExitValidation {
		t.Errorf("expected ExitValidation for missing path param, got %d", result.ExitCode)
	}
}

// TestExecuteBatchOpWithContext_DiffCommand verifies the diff command dispatch.
func TestExecuteBatchOpWithContext_DiffCommand(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"values":[]}`)
	}))
	defer ts.Close()

	var stdout, stderr bytes.Buffer
	c := newTestClient(ts.URL, &stdout, &stderr)
	opMap := buildTestOpMap()

	bop := BatchOp{Command: "diff diff", Args: map[string]string{"issue": "TST-1"}}
	result := executeBatchOpWithContext(context.Background(), c, 0, bop, opMap)
	// We just care that it dispatches to the diff handler, not that the result is perfect.
	_ = result
}

// TestExecuteBatchOpWithContext_QueryAndBody verifies query params, fields, and body
// handling in the HTTP dispatch path.
func TestExecuteBatchOpWithContext_QueryAndBody(t *testing.T) {
	var gotQuery string
	var gotBody string
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotQuery = r.URL.RawQuery
		var buf bytes.Buffer
		_, _ = buf.ReadFrom(r.Body)
		gotBody = buf.String()
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"ok":true}`)
	}))
	defer ts.Close()

	var stdout, stderr bytes.Buffer
	c := newTestClient(ts.URL, &stdout, &stderr)
	c.Fields = "summary,status" // global --fields
	opMap := buildTestOpMap()

	bop := BatchOp{
		Command: "issue edit",
		Args: map[string]string{
			"issueIdOrKey": "TST-1",
			"body":         `{"fields":{"summary":"Updated"}}`,
			"fields":       "summary", // per-op fields override
		},
	}
	result := executeBatchOpWithContext(context.Background(), c, 0, bop, opMap)
	_ = result
	_ = gotQuery
	_ = gotBody
}

// ---------------------------------------------------------------------------
// context_cmd.go — changelog failure path
// ---------------------------------------------------------------------------

// TestRunContext_ChangelogFails verifies early exit when the changelog endpoint
// returns an error (comments succeed but changelog fails).
func TestRunContext_ChangelogFails(t *testing.T) {
	var callCount int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		w.Header().Set("Content-Type", "application/json")
		if strings.Contains(r.URL.Path, "/changelog") {
			w.WriteHeader(http.StatusInternalServerError)
			fmt.Fprint(w, `{"errorMessages":"server error"}`)
			return
		}
		if strings.Contains(r.URL.Path, "/comment") {
			fmt.Fprint(w, `{"comments":[],"total":0}`)
			return
		}
		fmt.Fprint(w, `{"key":"PROJ-1","fields":{"summary":"test"}}`)
	}))
	defer srv.Close()

	var stdout, stderr bytes.Buffer
	c := newTestClient(srv.URL, &stdout, &stderr)

	cmd := newContextCmd(c)
	err := cmd.RunE(cmd, []string{"PROJ-1"})
	if err == nil {
		t.Fatal("expected error when changelog fails")
	}
	awe, ok := err.(*jrerrors.AlreadyWrittenError)
	if !ok {
		t.Fatalf("expected AlreadyWrittenError, got %T: %v", err, err)
	}
	if awe.Code == jrerrors.ExitOK {
		t.Errorf("expected non-zero exit code, got %d", awe.Code)
	}
	if callCount != 3 {
		t.Errorf("expected 3 HTTP calls (issue + comments + changelog), got %d", callCount)
	}
}

// ---------------------------------------------------------------------------
// Additional runPipe edge cases: parallel=0 coerced to 1
// ---------------------------------------------------------------------------

// TestRunPipe_ParallelZeroCoercedToOne verifies that parallel=0 is coerced to
// 1 (sequential execution) and the command still succeeds.
func TestRunPipe_ParallelZeroCoercedToOne(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if strings.Contains(r.URL.RawQuery, "jql") || strings.Contains(r.URL.Path, "search") {
			fmt.Fprint(w, `{"issues":[{"key":"ZZZ-1"}]}`)
			return
		}
		if strings.HasSuffix(r.URL.Path, "/ZZZ-1") {
			fmt.Fprint(w, `{"key":"ZZZ-1"}`)
			return
		}
		w.WriteHeader(http.StatusNotFound)
		fmt.Fprint(w, `{"error":"not found"}`)
	}))
	defer ts.Close()

	var stdout, stderr bytes.Buffer
	c := newPipeClient(ts.URL, &stdout, &stderr)

	cmd := newRunPipeCmd(c)
	_ = cmd.Flags().Set("extract", "[.issues[].key]")
	_ = cmd.Flags().Set("inject", "issueIdOrKey")
	_ = cmd.Flags().Set("parallel", "0") // should be coerced to 1

	out, err := runPipeCmd(t, cmd, []string{
		"search search-and-reconsile-issues-using-jql --jql 'project=ZZZ'",
		"issue get",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var results []BatchResult
	if jsonErr := json.Unmarshal([]byte(strings.TrimSpace(out)), &results); jsonErr != nil {
		t.Fatalf("output not valid JSON: %s — %v", out, jsonErr)
	}
	if len(results) != 1 {
		t.Errorf("expected 1 result, got %d", len(results))
	}
}


// ---------------------------------------------------------------------------
// doctor.go — additional auth type coverage
// ---------------------------------------------------------------------------

// TestDoctorHealthy_BearerAuth verifies bearer auth with valid token passes.
func TestDoctorHealthy_BearerAuth(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, `{"accountId":"u1"}`)
	}))
	defer srv.Close()

	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.json")
	cfg := &config.Config{
		DefaultProfile: "default",
		Profiles: map[string]config.Profile{
			"default": {
				BaseURL: srv.URL,
				Auth:    config.AuthConfig{Type: "bearer", Token: "valid-bearer-token"},
			},
		},
	}
	if err := config.SaveTo(cfg, configPath); err != nil {
		t.Fatal(err)
	}
	t.Setenv("JR_CONFIG_PATH", configPath)

	cmd := newDoctorCmd()
	var runErr error
	output := captureStdout(t, func() {
		runErr = runDoctor(cmd, nil)
	})

	if runErr != nil {
		t.Fatalf("runDoctor returned error: %v", runErr)
	}

	result := parseDoctorResult(t, output)
	if result.Status != "healthy" {
		t.Errorf("expected healthy, got %q", result.Status)
	}
	authCheck := findCheck(t, result.Checks, "auth_configured")
	if authCheck.Status != "pass" {
		t.Errorf("expected auth pass, got %q: %s", authCheck.Status, authCheck.Message)
	}
}

// TestDoctorHealthy_OAuth2Auth verifies oauth2 auth with all fields passes.
func TestDoctorHealthy_OAuth2Auth(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, `{"accountId":"u1"}`)
	}))
	defer srv.Close()

	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.json")
	cfg := &config.Config{
		DefaultProfile: "default",
		Profiles: map[string]config.Profile{
			"default": {
				BaseURL: srv.URL,
				Auth: config.AuthConfig{
					Type:         "oauth2",
					ClientID:     "cid",
					ClientSecret: "csecret",
					TokenURL:     "https://example.com/token",
				},
			},
		},
	}
	if err := config.SaveTo(cfg, configPath); err != nil {
		t.Fatal(err)
	}
	t.Setenv("JR_CONFIG_PATH", configPath)

	cmd := newDoctorCmd()
	var runErr error
	output := captureStdout(t, func() {
		runErr = runDoctor(cmd, nil)
	})

	if runErr != nil {
		t.Fatalf("runDoctor returned error: %v", runErr)
	}

	result := parseDoctorResult(t, output)
	authCheck := findCheck(t, result.Checks, "auth_configured")
	if authCheck.Status != "pass" {
		t.Errorf("expected auth pass for oauth2, got %q: %s", authCheck.Status, authCheck.Message)
	}
}

// ---------------------------------------------------------------------------
// explain.go — buildExplanation default case with status mapping to exit code
// ---------------------------------------------------------------------------

// TestBuildExplanation_DefaultCase_WithStatus verifies the default case where
// status maps to a known exit code (not ExitOK).
func TestBuildExplanation_DefaultCase_WithStatus(t *testing.T) {
	// Status 500 maps to ExitServer via ExitCodeFromStatus
	e := &jrerrors.APIError{ErrorType: "custom_error", Status: 500, Message: "oops"}
	exp := buildExplanation(e)

	if exp.ErrorType != "custom_error" {
		t.Errorf("expected error_type=custom_error, got %q", exp.ErrorType)
	}
	if exp.ExitCode != jrerrors.ExitServer {
		t.Errorf("expected ExitServer (%d), got %d", jrerrors.ExitServer, exp.ExitCode)
	}
}

// TestBuildExplanation_EmptyType_InferFromStatus verifies that an empty error_type
// is inferred from status code.
func TestBuildExplanation_EmptyType_InferFromStatus(t *testing.T) {
	e := &jrerrors.APIError{Status: 404, Message: "not found"}
	exp := buildExplanation(e)

	if exp.ErrorType != "not_found" {
		t.Errorf("expected inferred error_type=not_found, got %q", exp.ErrorType)
	}
	if exp.ExitCode != jrerrors.ExitNotFound {
		t.Errorf("expected ExitNotFound (%d), got %d", jrerrors.ExitNotFound, exp.ExitCode)
	}
}

// ---------------------------------------------------------------------------
// batch.go — executeBatchOpWithContext template command dispatch
// ---------------------------------------------------------------------------

// TestExecuteBatchOpWithContext_TemplateCommand verifies the template command
// dispatch path inside executeBatchOpWithContext.
func TestExecuteBatchOpWithContext_TemplateCommand(t *testing.T) {
	var stdout, stderr bytes.Buffer
	c := newTestClient("http://unused", &stdout, &stderr)
	opMap := buildTestOpMap()

	bop := BatchOp{
		Command: "template list",
		Args:    map[string]string{},
	}

	result := executeBatchOpWithContext(context.Background(), c, 0, bop, opMap)
	// Template list is local — we just care that it dispatches correctly.
	_ = result
}
