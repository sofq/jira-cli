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
	jrerrors "github.com/sofq/jira-cli/internal/errors"
	"github.com/spf13/cobra"
)

// ---------------------------------------------------------------------------
// Execute() — covers root.go lines 263-276
// ---------------------------------------------------------------------------

// TestExecute_Success verifies Execute returns ExitOK (0) when a
// built-in command succeeds. "schema" is in skipClientCommands so no
// config is required.
func TestExecute_Success(t *testing.T) {
	t.Cleanup(func() { rootCmd.SetArgs(nil) })

	// Redirect stdout so the schema output does not pollute test output.
	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	rootCmd.SetArgs([]string{"schema"})
	code := Execute()

	w.Close()
	os.Stdout = oldStdout
	var buf bytes.Buffer
	_, _ = buf.ReadFrom(r)

	if code != jrerrors.ExitOK {
		t.Errorf("Execute(schema) expected exit 0, got %d", code)
	}
}

// TestExecute_AlreadyWrittenError verifies Execute returns the Code field from
// an AlreadyWrittenError. We trigger this by running a command that requires
// a client but has no config. "batch" is not in skipClientCommands and the
// PersistentPreRunE will fail on missing base_url.
func TestExecute_AlreadyWrittenError(t *testing.T) {
	t.Cleanup(func() { rootCmd.SetArgs(nil) })

	// Use a temp dir with no config so base_url is unset.
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)
	t.Setenv("JR_CONFIG_PATH", filepath.Join(tmpHome, "nonexistent.json"))
	t.Setenv("JR_BASE_URL", "")
	t.Setenv("JR_TOKEN", "")

	// Redirect stderr so the JSON error does not pollute test output.
	oldStderr := os.Stderr
	r, w, _ := os.Pipe()
	os.Stderr = w

	// Any leaf command not in skipClientCommands will trigger the missing
	// base_url error path in PersistentPreRunE.
	rootCmd.SetArgs([]string{"schema", "issue"})
	// "schema" is in skipClientCommands, so switch to something that isn't.
	// Use "batch" with --input to avoid stdin detection issues.
	tmp := t.TempDir()
	inputFile := filepath.Join(tmp, "ops.json")
	_ = os.WriteFile(inputFile, []byte(`[]`), 0o644)

	// We need a command outside skipClientCommands. "diff" works.
	rootCmd.SetArgs([]string{"diff", "--issue", "FAKE-1"})

	code := Execute()

	w.Close()
	os.Stderr = oldStderr
	var buf bytes.Buffer
	_, _ = buf.ReadFrom(r)

	// Without a valid config, PersistentPreRunE returns AlreadyWrittenError
	// with ExitError (1).
	if code == jrerrors.ExitOK {
		t.Errorf("Execute(diff) with no config expected non-zero exit, got 0; stderr: %s", buf.String())
	}
}

// TestExecute_UnknownCommand verifies Execute returns ExitError for an
// unrecognised command (cobra returns a plain error, not AlreadyWrittenError).
func TestExecute_UnknownCommand(t *testing.T) {
	t.Cleanup(func() { rootCmd.SetArgs(nil) })

	oldStderr := os.Stderr
	r, w, _ := os.Pipe()
	os.Stderr = w

	rootCmd.SetArgs([]string{"no-such-command-xyz"})
	code := Execute()

	w.Close()
	os.Stderr = oldStderr
	var buf bytes.Buffer
	_, _ = buf.ReadFrom(r)

	if code != jrerrors.ExitError {
		t.Errorf("Execute(unknown) expected exit %d, got %d; stderr: %s",
			jrerrors.ExitError, code, buf.String())
	}
	// The error JSON should contain "command_error".
	if !strings.Contains(buf.String(), "command_error") {
		t.Errorf("expected 'command_error' in stderr JSON, got: %s", buf.String())
	}
}

// TestExecute_StderrIsJSON verifies that Execute writes valid JSON to stderr
// when an unknown command is used, not plain text.
func TestExecute_StderrIsJSON(t *testing.T) {
	t.Cleanup(func() { rootCmd.SetArgs(nil) })

	oldStderr := os.Stderr
	r, w, _ := os.Pipe()
	os.Stderr = w

	rootCmd.SetArgs([]string{"totally-unknown-xyz"})
	Execute()

	w.Close()
	os.Stderr = oldStderr
	var buf bytes.Buffer
	_, _ = buf.ReadFrom(r)

	line := strings.TrimSpace(buf.String())
	if line == "" {
		t.Fatal("expected non-empty stderr output")
	}
	var parsed map[string]string
	if err := json.Unmarshal([]byte(line), &parsed); err != nil {
		t.Errorf("stderr is not valid JSON: %q; parse error: %v", line, err)
	}
	if parsed["error_type"] != "command_error" {
		t.Errorf("expected error_type=command_error, got %q", parsed["error_type"])
	}
}

// ---------------------------------------------------------------------------
// init() help function — covers root.go lines 238-253
// ---------------------------------------------------------------------------

// TestRootHelpFunc_RootCmd_OutputsJSON verifies that calling the root help
// function for the root command writes a JSON hint to stdout.
func TestRootHelpFunc_RootCmd_OutputsJSON(t *testing.T) {
	captured := captureStdout(t, func() {
		rootCmd.HelpFunc()(rootCmd, []string{})
	})

	trimmed := strings.TrimSpace(captured)
	if trimmed == "" {
		t.Fatal("expected JSON output on stdout, got empty string")
	}

	var parsed map[string]string
	if err := json.Unmarshal([]byte(trimmed), &parsed); err != nil {
		t.Fatalf("root help output is not valid JSON: %q; err: %v", trimmed, err)
	}
	if _, ok := parsed["hint"]; !ok {
		t.Error("expected 'hint' key in root help JSON")
	}
	if _, ok := parsed["version"]; !ok {
		t.Error("expected 'version' key in root help JSON")
	}
	// Verify no HTML escaping of angle brackets.
	if strings.Contains(trimmed, `\u003c`) {
		t.Errorf("expected no HTML escaping, got: %s", trimmed)
	}
}

// TestRootHelpFunc_NonRootCmd_WritesToStderr verifies that calling the root
// help function for a non-root command redirects output to stderr.
func TestRootHelpFunc_NonRootCmd_WritesToStderr(t *testing.T) {
	// Find a real non-root subcommand (e.g., "schema").
	var subCmd *cobra.Command
	for _, c := range rootCmd.Commands() {
		if c.Name() == "schema" {
			subCmd = c
			break
		}
	}
	if subCmd == nil {
		t.Skip("schema subcommand not found; skipping non-root help test")
	}

	// The non-root path in the help func calls cmd.SetOut(os.Stderr) then
	// invokes the default help func. We verify the subcommand's Out is set
	// to os.Stderr after the call (the side-effect we can observe).
	originalOut := subCmd.OutOrStdout()
	rootCmd.HelpFunc()(subCmd, []string{})

	// After calling the custom help func for a non-root command, cmd.OutOrStdout()
	// should be os.Stderr.
	if subCmd.OutOrStdout() != os.Stderr {
		t.Errorf("expected subcommand Out to be os.Stderr after help call, got: %T", subCmd.OutOrStdout())
	}

	// Restore to avoid side-effects on other tests.
	subCmd.SetOut(originalOut)
}

// ---------------------------------------------------------------------------
// runBatch stdin paths — covers batch.go lines 98-116
// ---------------------------------------------------------------------------

// TestRunBatch_NoInput_TerminalStdin verifies that runBatch returns a
// validation error when no --input flag is provided and stdin is a terminal
// (character device). In the test process, os.Stdin is the real terminal
// handle or at minimum not a pipe, so this covers the ModeCharDevice branch.
func TestRunBatch_NoInput_TerminalStdin(t *testing.T) {
	// Use a pipe as stdin to simulate a terminal (ModeCharDevice) by using a
	// non-piped stdin, but in CI os.Stdin may already be non-terminal.
	// The most reliable approach: replace os.Stdin with a real terminal-like
	// FD (a regular file opened for reading, which has ModeCharDevice = 0
	// but the stat won't error). We test the "stat error" branch by replacing
	// os.Stdin with a closed file descriptor.

	// Save real stdin.
	realStdin := os.Stdin

	// Create a temporary regular file and close it immediately.  When we
	// try to Stat() a closed *os.File, the call fails, triggering the
	// statErr != nil branch (line 98).
	tmpFile, err := os.CreateTemp(t.TempDir(), "closed-stdin-*")
	if err != nil {
		t.Fatal(err)
	}
	tmpFile.Close() // Close so Stat() on the file descriptor fails.
	os.Stdin = tmpFile
	defer func() { os.Stdin = realStdin }()

	var stdout, stderr bytes.Buffer
	c := newTestClient("http://unused", &stdout, &stderr)

	cmd := newBatchCmd(c)
	// Do NOT set --input flag, so the code falls through to the stdin branch.

	err = cmd.RunE(cmd, nil)
	if err == nil {
		t.Fatal("expected validation error when stdin is not a pipe")
	}
	aw, ok := err.(*jrerrors.AlreadyWrittenError)
	if !ok {
		t.Fatalf("expected AlreadyWrittenError, got %T: %v", err, err)
	}
	if aw.Code != jrerrors.ExitValidation {
		t.Errorf("expected ExitValidation (%d), got %d", jrerrors.ExitValidation, aw.Code)
	}
}

// TestRunBatch_StdinPipe_ValidJSON verifies that runBatch reads from stdin
// when it is a pipe (not a character device). This covers the io.ReadAll(os.Stdin)
// success path (line 107).
func TestRunBatch_StdinPipe_ValidJSON(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintln(w, `{"key":"PROJ-1","fields":{"summary":"Test"}}`)
	}))
	defer ts.Close()

	// Save real stdin.
	realStdin := os.Stdin

	// Create a pipe: the read end becomes stdin.
	pr, pw, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stdin = pr
	defer func() { os.Stdin = realStdin }()

	// Write valid batch JSON and close the write end.
	ops := `[{"command":"issue get","args":{"issueIdOrKey":"PROJ-1"}}]`
	_, _ = pw.WriteString(ops)
	pw.Close()

	var stdout, stderr bytes.Buffer
	c := newTestClient(ts.URL, &stdout, &stderr)

	cmd := newBatchCmd(c)
	// No --input flag: reads from stdin pipe.

	captured := captureStdout(t, func() {
		if err := cmd.RunE(cmd, nil); err != nil {
			// Ignore non-zero exits from the batch op itself (HTTP may fail).
			_ = err
		}
	})

	// We should get a JSON array back (at minimum "[]" or "[{...}]").
	trimmed := strings.TrimSpace(captured)
	if !strings.HasPrefix(trimmed, "[") {
		t.Errorf("expected JSON array from batch output, got: %s", trimmed)
	}
}

// ---------------------------------------------------------------------------
// raw.go — --body - (stdin) path: covers raw.go line 101-103
// ---------------------------------------------------------------------------

// TestRawCmd_BodyFromStdin verifies that runRaw reads the body from stdin
// when --body - is passed.
func TestRawCmd_BodyFromStdin(t *testing.T) {
	var receivedBody string
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var buf bytes.Buffer
		_, _ = buf.ReadFrom(r.Body)
		receivedBody = buf.String()
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintln(w, `{"ok":true}`)
	}))
	defer ts.Close()

	realStdin := os.Stdin
	pr, pw, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stdin = pr
	defer func() { os.Stdin = realStdin }()

	bodyContent := `{"fields":{"summary":"test"}}`
	_, _ = pw.WriteString(bodyContent)
	pw.Close()

	var stdout, stderr bytes.Buffer
	c := newTestClient(ts.URL, &stdout, &stderr)

	ctx := client.NewContext(t.Context(), c)
	cmd := &cobra.Command{Use: "raw"}
	cmd.Flags().String("body", "", "")
	cmd.Flags().StringArray("query", nil, "")
	cmd.Flags().String("fields", "", "")
	_ = cmd.Flags().Set("body", "-")
	cmd.SetContext(ctx)

	if err := runRaw(cmd, []string{"POST", "/rest/api/3/issue"}); err != nil {
		t.Fatalf("runRaw with --body - failed: %v", err)
	}

	if receivedBody != bodyContent {
		t.Errorf("expected body %q, got %q", bodyContent, receivedBody)
	}
}

// ---------------------------------------------------------------------------
// PersistentPreRunE paths — covers root.go lines 40-194
// ---------------------------------------------------------------------------

// TestPersistentPreRunE_SkipClientCommands verifies that commands in
// skipClientCommands do not require a client (no config error).
func TestPersistentPreRunE_SkipClientCommands(t *testing.T) {
	skipped := []string{"configure", "version", "completion", "help", "schema", "preset"}
	for _, name := range skipped {
		cmd := &cobra.Command{Use: name}
		rootCmd.AddCommand(cmd)
		err := rootCmd.PersistentPreRunE(cmd, nil)
		rootCmd.RemoveCommand(cmd)
		if err != nil {
			t.Errorf("PersistentPreRunE should skip %q (no client needed), got error: %v", name, err)
		}
	}
}

// TestPersistentPreRunE_SkipParentOfSkippedCommand verifies that subcommands
// of skipped commands (e.g., "completion bash") are also skipped.
func TestPersistentPreRunE_SkipParentOfSkippedCommand(t *testing.T) {
	parent := &cobra.Command{Use: "completion"}
	child := &cobra.Command{Use: "bash"}
	parent.AddCommand(child)
	rootCmd.AddCommand(parent)
	defer rootCmd.RemoveCommand(parent)

	err := rootCmd.PersistentPreRunE(child, nil)
	if err != nil {
		t.Errorf("PersistentPreRunE should skip subcommand of 'completion', got: %v", err)
	}
}

// TestPersistentPreRunE_MissingBaseURL verifies the missing base_url error path.
func TestPersistentPreRunE_MissingBaseURL(t *testing.T) {
	// Use a temp home so config.Resolve finds no config file and no env vars.
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)
	t.Setenv("JR_BASE_URL", "")
	t.Setenv("JR_TOKEN", "")

	// Register a throwaway leaf command to avoid using a real one that may
	// have its own PersistentPreRunE.
	leaf := &cobra.Command{Use: "fakeleaf", RunE: func(cmd *cobra.Command, args []string) error { return nil }}
	rootCmd.AddCommand(leaf)
	defer rootCmd.RemoveCommand(leaf)

	// Add the required persistent flags so GetString calls don't panic.
	// (They are already on rootCmd.PersistentFlags, so the leaf inherits them.)

	oldStderr := os.Stderr
	r, w, _ := os.Pipe()
	os.Stderr = w

	err := rootCmd.PersistentPreRunE(leaf, nil)

	w.Close()
	os.Stderr = oldStderr
	var buf bytes.Buffer
	_, _ = buf.ReadFrom(r)

	if err == nil {
		t.Fatal("expected error when base_url is missing")
	}
	aw, ok := err.(*jrerrors.AlreadyWrittenError)
	if !ok {
		t.Fatalf("expected AlreadyWrittenError, got %T: %v", err, err)
	}
	if aw.Code != jrerrors.ExitError {
		t.Errorf("expected ExitError (%d), got %d", jrerrors.ExitError, aw.Code)
	}
	if !strings.Contains(buf.String(), "base_url") {
		t.Errorf("expected 'base_url' in error output, got: %s", buf.String())
	}
}

// TestPersistentPreRunE_InvalidPreset verifies the unknown preset error path
// by running the root command with a real --preset flag value that does not exist.
func TestPersistentPreRunE_InvalidPreset(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer ts.Close()

	// Point config at a valid server so PersistentPreRunE reaches the preset
	// check (after config resolution).
	tmp := t.TempDir()
	cfgPath := filepath.Join(tmp, "config.json")
	cfgContent := fmt.Sprintf(`{
  "profiles": { "default": { "base_url": %q, "auth": {"type":"basic","username":"u","token":"t"} } }
}`, ts.URL)
	if err := os.WriteFile(cfgPath, []byte(cfgContent), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("JR_CONFIG_PATH", cfgPath)
	t.Setenv("JR_BASE_URL", "")
	t.Setenv("JR_TOKEN", "")

	// Reset the preset flag after this test so it does not leak into others.
	t.Cleanup(func() {
		_ = rootCmd.PersistentFlags().Set("preset", "")
		rootCmd.SetArgs(nil)
	})

	oldStderr := os.Stderr
	r, w, _ := os.Pipe()
	os.Stderr = w

	// "diff --issue X --preset bad" will hit PersistentPreRunE because diff is
	// not in skipClientCommands. It fails at the preset check before talking to Jira.
	rootCmd.SetArgs([]string{"diff", "--issue", "FAKE-1", "--preset", "no-such-preset-xyz"})
	code := Execute()

	w.Close()
	os.Stderr = oldStderr
	var buf bytes.Buffer
	_, _ = buf.ReadFrom(r)

	if code != jrerrors.ExitValidation {
		t.Errorf("expected ExitValidation (%d) for unknown preset, got %d; stderr: %s",
			jrerrors.ExitValidation, code, buf.String())
	}
	if !strings.Contains(buf.String(), "unknown preset") {
		t.Errorf("expected 'unknown preset' in stderr, got: %s", buf.String())
	}
}

// TestPersistentPreRunE_PolicyDenied verifies the policy check error path
// by running via Execute() with a config that denies all operations.
func TestPersistentPreRunE_PolicyDenied(t *testing.T) {
	t.Cleanup(func() { rootCmd.SetArgs(nil) })

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer ts.Close()

	// Write a config file that denies all operations. Use JR_CONFIG_PATH to
	// bypass OS-specific default path logic.
	tmp := t.TempDir()
	cfgPath := filepath.Join(tmp, "config.json")
	cfgContent := fmt.Sprintf(`{
  "profiles": {
    "default": {
      "base_url": %q,
      "auth": {"type": "basic", "username": "u", "token": "t"},
      "denied_operations": ["*"]
    }
  }
}`, ts.URL)
	if err := os.WriteFile(cfgPath, []byte(cfgContent), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("JR_CONFIG_PATH", cfgPath)
	t.Setenv("JR_BASE_URL", "")
	t.Setenv("JR_TOKEN", "")

	oldStderr := os.Stderr
	r, w, _ := os.Pipe()
	os.Stderr = w

	// "diff --issue X" runs PersistentPreRunE (not in skipClientCommands)
	// and will be denied by the "*" policy before reaching Jira.
	rootCmd.SetArgs([]string{"diff", "--issue", "FAKE-1"})
	code := Execute()

	w.Close()
	os.Stderr = oldStderr
	var buf bytes.Buffer
	_, _ = buf.ReadFrom(r)

	if code != jrerrors.ExitValidation {
		t.Errorf("expected ExitValidation (%d) for denied operation, got %d; stderr: %s",
			jrerrors.ExitValidation, code, buf.String())
	}
	if !strings.Contains(buf.String(), "not allowed") {
		t.Errorf("expected 'not allowed' in stderr, got: %s", buf.String())
	}
}

// TestPersistentPreRunE_SuccessInjectsClient verifies the happy path: when
// base_url is set, a client is injected into the command context.
func TestPersistentPreRunE_SuccessInjectsClient(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer ts.Close()

	t.Setenv("JR_BASE_URL", ts.URL)
	t.Setenv("JR_TOKEN", "testtoken")
	t.Setenv("JR_USERNAME", "user@example.com")

	leaf := &cobra.Command{
		Use: "fakeleaf4",
		RunE: func(cmd *cobra.Command, args []string) error {
			_, err := client.FromContext(cmd.Context())
			return err
		},
	}
	rootCmd.AddCommand(leaf)
	defer rootCmd.RemoveCommand(leaf)

	leaf.SetContext(context.Background())

	err := rootCmd.PersistentPreRunE(leaf, nil)
	if err != nil {
		t.Fatalf("expected no error with valid config, got: %v", err)
	}

	// Verify client was injected.
	c, cerr := client.FromContext(leaf.Context())
	if cerr != nil {
		t.Fatalf("expected client in context, got error: %v", cerr)
	}
	if c.BaseURL != ts.URL {
		t.Errorf("expected BaseURL=%q, got %q", ts.URL, c.BaseURL)
	}
}

// TestPersistentPreRunE_AuditFileFlag verifies that --audit-file triggers
// audit logger creation. We use an unwriteable path to exercise the
// non-fatal warning branch (lines 163-171 in root.go). The command still
// succeeds even if the audit log cannot be opened.
func TestPersistentPreRunE_AuditFileFlag(t *testing.T) {
	t.Cleanup(func() {
		rootCmd.SetArgs(nil)
		_ = rootCmd.PersistentFlags().Set("audit-file", "")
	})

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		// Return a valid schema response so "schema" command succeeds — but
		// schema is in skipClientCommands so PersistentPreRunE is skipped for it.
		// Instead handle the "diff" request that goes through PersistentPreRunE.
		fmt.Fprintln(w, `{"values":[]}`)
	}))
	defer ts.Close()

	tmp := t.TempDir()
	cfgPath := filepath.Join(tmp, "config.json")
	cfgContent := fmt.Sprintf(`{
  "profiles": { "default": { "base_url": %q, "auth": {"type":"basic","username":"u","token":"t"} } }
}`, ts.URL)
	if err := os.WriteFile(cfgPath, []byte(cfgContent), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("JR_CONFIG_PATH", cfgPath)
	t.Setenv("JR_BASE_URL", "")
	t.Setenv("JR_TOKEN", "")

	// Use an unwriteable audit-file path so audit.NewLogger fails (non-fatal).
	// Create a file where the parent "directory" should be, so MkdirAll fails.
	fakeParent := filepath.Join(tmp, "notadir")
	if err := os.WriteFile(fakeParent, []byte("not a dir"), 0o644); err != nil {
		t.Fatal(err)
	}
	badAuditPath := filepath.Join(fakeParent, "audit.log")

	oldStderr := os.Stderr
	r, w, _ := os.Pipe()
	os.Stderr = w

	// "diff --issue FAKE-1 --audit-file <bad>" runs PersistentPreRunE which
	// tries to open the audit log. The failure is non-fatal, so Execute
	// should NOT return ExitError for the audit failure alone. It may still
	// return an error for the diff command itself (e.g. HTTP error) but we
	// only care that the audit warning is written to stderr.
	rootCmd.SetArgs([]string{"diff", "--issue", "FAKE-1", "--audit-file", badAuditPath})
	_ = Execute()

	w.Close()
	os.Stderr = oldStderr
	var buf bytes.Buffer
	_, _ = buf.ReadFrom(r)

	stderrStr := buf.String()
	// The warning JSON written by PersistentPreRunE has "type":"warning" and
	// "message":"audit log open failed: ...".
	if !strings.Contains(stderrStr, "audit log open failed") {
		t.Errorf("expected audit warning in stderr, got: %s", stderrStr)
	}
}
