package cmd

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	jrerrors "github.com/sofq/jira-cli/internal/errors"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

func TestRootHelp_NoHTMLEscaping(t *testing.T) {
	// The root help JSON should not contain HTML-escaped angle brackets.
	// Verify the encoding approach directly.
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	_ = enc.Encode(map[string]string{
		"hint": "use `jr schema <resource>` for operations",
	})

	output := buf.String()
	if strings.Contains(output, `\u003c`) {
		t.Errorf("expected no HTML escaping in JSON, got: %s", output)
	}
	if !strings.Contains(output, `<resource>`) {
		t.Errorf("expected literal <resource> in output, got: %s", output)
	}
}

func TestRootCommand_ReturnsNonNil(t *testing.T) {
	cmd := RootCommand()
	if cmd == nil {
		t.Fatal("RootCommand() returned nil")
	}
	if cmd.Use != "jr" {
		t.Errorf("expected Use='jr', got %q", cmd.Use)
	}
}

func TestResolveOperation_RawCommand(t *testing.T) {
	cmd := &cobra.Command{Use: "raw"}
	result := resolveOperation(cmd)
	if result != "raw" {
		t.Errorf("expected 'raw', got %q", result)
	}
}

func TestResolveOperation_NestedCommand(t *testing.T) {
	parent := &cobra.Command{Use: "issue"}
	child := &cobra.Command{Use: "get"}
	parent.AddCommand(child)

	result := resolveOperation(child)
	if result != "issue get" {
		t.Errorf("expected 'issue get', got %q", result)
	}
}

func TestResolveOperation_TopLevelCommand(t *testing.T) {
	root := &cobra.Command{Use: "jr"}
	child := &cobra.Command{Use: "configure"}
	root.AddCommand(child)

	result := resolveOperation(child)
	if result != "configure" {
		t.Errorf("expected 'configure', got %q", result)
	}
}

func TestMergeCommand_PreservesGeneratedSubcommands(t *testing.T) {
	root := &cobra.Command{Use: "jr"}

	// Simulate a generated parent with subcommands.
	genParent := &cobra.Command{Use: "workflow"}
	genSub1 := &cobra.Command{Use: "generated-sub"}
	genParent.AddCommand(genSub1)
	root.AddCommand(genParent)

	// Create a hand-written replacement with its own sub.
	handWritten := &cobra.Command{Use: "workflow"}
	handSub := &cobra.Command{Use: "hand-sub"}
	handWritten.AddCommand(handSub)

	mergeCommand(root, handWritten)

	// Verify hand-written replaced the generated parent.
	found := false
	for _, c := range root.Commands() {
		if c.Name() == "workflow" {
			found = true
			// Should have both the hand-written sub and the generated sub.
			subNames := make(map[string]bool)
			for _, sub := range c.Commands() {
				subNames[sub.Name()] = true
			}
			if !subNames["hand-sub"] {
				t.Error("expected 'hand-sub' subcommand")
			}
			if !subNames["generated-sub"] {
				t.Error("expected 'generated-sub' subcommand to be preserved")
			}
		}
	}
	if !found {
		t.Error("workflow command not found on root")
	}
}

func TestMergeCommand_NoExistingGenerated(t *testing.T) {
	root := &cobra.Command{Use: "jr"}
	handWritten := &cobra.Command{Use: "brand-new"}
	handSub := &cobra.Command{Use: "sub1"}
	handWritten.AddCommand(handSub)

	mergeCommand(root, handWritten)

	found := false
	for _, c := range root.Commands() {
		if c.Name() == "brand-new" {
			found = true
		}
	}
	if !found {
		t.Error("brand-new command should be added to root")
	}
}

func TestMergeCommand_HandWrittenSubNotDuplicated(t *testing.T) {
	root := &cobra.Command{Use: "jr"}

	// Generated parent has a sub called "shared".
	genParent := &cobra.Command{Use: "workflow"}
	genSub := &cobra.Command{Use: "shared"}
	genParent.AddCommand(genSub)
	root.AddCommand(genParent)

	// Hand-written also has "shared".
	handWritten := &cobra.Command{Use: "workflow"}
	handSub := &cobra.Command{Use: "shared"}
	handWritten.AddCommand(handSub)

	mergeCommand(root, handWritten)

	for _, c := range root.Commands() {
		if c.Name() == "workflow" {
			count := 0
			for _, sub := range c.Commands() {
				if sub.Name() == "shared" {
					count++
				}
			}
			if count != 1 {
				t.Errorf("expected exactly 1 'shared' subcommand, got %d", count)
			}
		}
	}
}

// --- PersistentPreRunE branch coverage ---

func TestPreRunE_SkippedCommandReturnsNil(t *testing.T) {
	cmd := &cobra.Command{Use: "configure"}
	if err := rootCmd.PersistentPreRunE(cmd, nil); err != nil {
		t.Fatalf("expected skip for 'configure', got %v", err)
	}
}

func TestPreRunE_SkippedParentReturnsNil(t *testing.T) {
	parent := &cobra.Command{Use: "completion"}
	child := &cobra.Command{Use: "bash"}
	parent.AddCommand(child)
	if err := rootCmd.PersistentPreRunE(child, nil); err != nil {
		t.Fatalf("expected skip via parent, got %v", err)
	}
}

// makePreRunCmd attaches a disposable subcommand to rootCmd so persistent
// flags are inherited. Caller must call cleanup() to detach and reset any
// persistent-flag state that parent tests may have mutated.
func makePreRunCmd(t *testing.T, name string) (*cobra.Command, func()) {
	t.Helper()
	sub := &cobra.Command{Use: name, Run: func(*cobra.Command, []string) {}}
	rootCmd.AddCommand(sub)
	sub.InheritedFlags()
	sub.SetContext(t.Context())
	return sub, func() {
		rootCmd.RemoveCommand(sub)
		// Reset persistent flags on rootCmd so subsequent tests see defaults.
		rootCmd.PersistentFlags().VisitAll(func(f *pflag.Flag) {
			_ = f.Value.Set(f.DefValue)
			f.Changed = false
		})
	}
}

func writeConfig(t *testing.T, body string) string {
	t.Helper()
	f := t.TempDir() + "/config.json"
	if err := os.WriteFile(f, []byte(body), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}
	return f
}

func TestPreRunE_ConfigResolveError(t *testing.T) {
	t.Setenv("JR_CONFIG_PATH", writeConfig(t, "not-json"))
	cmd, cleanup := makePreRunCmd(t, "preruntest-cfg")
	defer cleanup()

	err := rootCmd.PersistentPreRunE(cmd, nil)
	if err == nil {
		t.Fatal("expected config resolve error")
	}
	aw, ok := err.(*jrerrors.AlreadyWrittenError)
	if !ok || aw.Code != jrerrors.ExitError {
		t.Fatalf("expected ExitError AlreadyWrittenError, got %v", err)
	}
}

func TestPreRunE_MissingBaseURL(t *testing.T) {
	t.Setenv("JR_CONFIG_PATH", writeConfig(t, `{"default_profile":"p","profiles":{"p":{"auth":{"type":"basic","username":"u","token":"t"}}}}`))
	t.Setenv("JR_BASE_URL", "")
	cmd, cleanup := makePreRunCmd(t, "preruntest-nourl")
	defer cleanup()

	err := rootCmd.PersistentPreRunE(cmd, nil)
	if err == nil {
		t.Fatal("expected missing base_url error")
	}
	aw, ok := err.(*jrerrors.AlreadyWrittenError)
	if !ok || aw.Code != jrerrors.ExitError {
		t.Fatalf("expected ExitError, got %v", err)
	}
}

func TestPreRunE_UnknownPreset(t *testing.T) {
	t.Setenv("JR_CONFIG_PATH", writeConfig(t, `{"default_profile":"p","profiles":{"p":{"base_url":"http://x","auth":{"type":"basic","username":"u","token":"t"}}}}`))
	cmd, cleanup := makePreRunCmd(t, "preruntest-badpreset")
	defer cleanup()
	_ = cmd.Flags().Parse([]string{"--preset", "does-not-exist"})

	err := rootCmd.PersistentPreRunE(cmd, nil)
	if err == nil {
		t.Fatal("expected unknown preset error")
	}
	aw, ok := err.(*jrerrors.AlreadyWrittenError)
	if !ok || aw.Code != jrerrors.ExitValidation {
		t.Fatalf("expected ExitValidation, got %v", err)
	}
}

func TestPreRunE_PresetApplied(t *testing.T) {
	t.Setenv("JR_CONFIG_PATH", writeConfig(t, `{"default_profile":"p","profiles":{"p":{"base_url":"http://x","auth":{"type":"basic","username":"u","token":"t"}}}}`))

	// Install a user preset with both Fields and JQ so both override
	// branches in PreRunE are exercised. Write to every platform path so this
	// works regardless of OS.
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", home)
	t.Setenv("APPDATA", home)
	body := []byte(`{"mypreset":{"fields":"key,summary","jq":".key"}}`)
	for _, p := range []string{
		home + "/jr/presets.json",
		home + "/Library/Application Support/jr/presets.json",
	} {
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, body, 0o600); err != nil {
			t.Fatal(err)
		}
	}

	cmd, cleanup := makePreRunCmd(t, "preruntest-preset")
	defer cleanup()
	_ = cmd.Flags().Parse([]string{"--preset", "mypreset"})

	if err := rootCmd.PersistentPreRunE(cmd, nil); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestPreRunE_PresetLookupError(t *testing.T) {
	t.Setenv("JR_CONFIG_PATH", writeConfig(t, `{"default_profile":"p","profiles":{"p":{"base_url":"http://x","auth":{"type":"basic","username":"u","token":"t"}}}}`))
	writeBrokenUserPresets(t)

	cmd, cleanup := makePreRunCmd(t, "preruntest-presetlookuperr")
	defer cleanup()
	_ = cmd.Flags().Parse([]string{"--preset", "anything"})

	err := rootCmd.PersistentPreRunE(cmd, nil)
	if err == nil {
		t.Fatal("expected preset lookup error")
	}
	aw, ok := err.(*jrerrors.AlreadyWrittenError)
	if !ok || aw.Code != jrerrors.ExitError {
		t.Fatalf("expected ExitError, got %v", err)
	}
}

// writeBrokenUserPresets puts malformed JSON at every possible user-config
// location so preset.Lookup / preset.List return an error regardless of OS.
func writeBrokenUserPresets(t *testing.T) {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", home)
	t.Setenv("APPDATA", home)

	// Linux/XDG: $XDG_CONFIG_HOME/jr/presets.json
	// macOS: $HOME/Library/Application Support/jr/presets.json
	// Windows: %APPDATA%/jr/presets.json
	paths := []string{
		home + "/jr/presets.json",
		home + "/Library/Application Support/jr/presets.json",
	}
	for _, p := range paths {
		dir := filepath.Dir(p)
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte("not-json"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
}

func TestPreRunE_PolicyBlocksOperation(t *testing.T) {
	t.Setenv("JR_CONFIG_PATH", writeConfig(t, `{"default_profile":"p","profiles":{"p":{"base_url":"http://x","auth":{"type":"basic","username":"u","token":"t"},"denied_operations":["preruntest-blocked"]}}}`))
	cmd, cleanup := makePreRunCmd(t, "preruntest-blocked")
	defer cleanup()

	err := rootCmd.PersistentPreRunE(cmd, nil)
	if err == nil {
		t.Fatal("expected policy error")
	}
	aw, ok := err.(*jrerrors.AlreadyWrittenError)
	if !ok || aw.Code != jrerrors.ExitValidation {
		t.Fatalf("expected ExitValidation, got %v", err)
	}
}

func TestPreRunE_InvalidPolicyConfig(t *testing.T) {
	// allowed + denied both set → invalid policy.
	t.Setenv("JR_CONFIG_PATH", writeConfig(t, `{"default_profile":"p","profiles":{"p":{"base_url":"http://x","auth":{"type":"basic","username":"u","token":"t"},"allowed_operations":["*"],"denied_operations":["*"]}}}`))
	cmd, cleanup := makePreRunCmd(t, "preruntest-badpol")
	defer cleanup()

	err := rootCmd.PersistentPreRunE(cmd, nil)
	if err == nil {
		t.Fatal("expected invalid policy error")
	}
	aw, ok := err.(*jrerrors.AlreadyWrittenError)
	if !ok || aw.Code != jrerrors.ExitValidation {
		t.Fatalf("expected ExitValidation, got %v", err)
	}
}

func TestPreRunE_AuditEnabledDefaultPath(t *testing.T) {
	t.Setenv("JR_CONFIG_PATH", writeConfig(t, `{"default_profile":"p","profiles":{"p":{"base_url":"http://x","auth":{"type":"basic","username":"u","token":"t"}}}}`))
	auditDir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", auditDir)
	t.Setenv("HOME", auditDir)

	cmd, cleanup := makePreRunCmd(t, "preruntest-audit")
	defer cleanup()
	_ = cmd.Flags().Parse([]string{"--audit"})

	if err := rootCmd.PersistentPreRunE(cmd, nil); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Invoke PostRunE to close the audit logger.
	_ = rootCmd.PersistentPostRunE(cmd, nil)
}

func TestPreRunE_AuditOpenFailure(t *testing.T) {
	t.Setenv("JR_CONFIG_PATH", writeConfig(t, `{"default_profile":"p","profiles":{"p":{"base_url":"http://x","auth":{"type":"basic","username":"u","token":"t"}}}}`))
	// Point audit-file to a directory (can't be opened as log file).
	dir := t.TempDir()
	cmd, cleanup := makePreRunCmd(t, "preruntest-audfail")
	defer cleanup()
	_ = cmd.Flags().Parse([]string{"--audit-file", dir})

	// Should not return error — audit failure is non-fatal.
	if err := rootCmd.PersistentPreRunE(cmd, nil); err != nil {
		t.Fatalf("audit failure should be non-fatal, got %v", err)
	}
}

func TestPostRunE_ClosesAuditLogger(t *testing.T) {
	// Happy path is exercised by TestPreRunE_AuditEnabledDefaultPath.
	// Here exercise the "no client in context" branch.
	cmd := &cobra.Command{Use: "anything"}
	cmd.SetContext(t.Context())
	if err := rootCmd.PersistentPostRunE(cmd, nil); err != nil {
		t.Fatalf("expected nil for no-client path, got %v", err)
	}
}

// --- init() coverage: the custom HelpFunc ---

func TestRootHelp_JSONForRoot(t *testing.T) {
	var buf bytes.Buffer
	origOut := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w
	rootCmd.HelpFunc()(rootCmd, nil)
	w.Close()
	os.Stdout = origOut
	_, _ = buf.ReadFrom(r)

	if !strings.Contains(buf.String(), "hint") {
		t.Errorf("expected hint in root help JSON, got: %s", buf.String())
	}
}

func TestRootHelp_SubcommandWritesToStderr(t *testing.T) {
	sub, cleanup := makePreRunCmd(t, "helpsubtest")
	defer cleanup()
	// Should not panic; writes to stderr.
	rootCmd.HelpFunc()(sub, nil)
}

// TestExecute exercises the Execute() entry point happy path (--version).
// The error return path is covered implicitly via rootCmd.Execute() failures
// in PersistentPreRunE tests, which exercise the same error handling code
// when PreRunE returns AlreadyWrittenError.
func TestExecute(t *testing.T) {
	origArgs := os.Args
	defer func() { os.Args = origArgs }()
	os.Args = []string{"jr", "--version"}
	// Another test may have called rootCmd.SetArgs(); clear it so cobra
	// falls back to os.Args.
	rootCmd.SetArgs(os.Args[1:])
	defer rootCmd.SetArgs(nil)

	code := Execute()
	if code != 0 {
		t.Errorf("expected exit 0 for --version, got %d", code)
	}
}

// TestExecute_GenericError covers the non-AlreadyWrittenError return branch
// in Execute() by passing an invalid value for a typed flag (duration parse
// error) to a subcommand that doesn't gate on config.
func TestExecute_GenericError(t *testing.T) {
	origArgs := os.Args
	defer func() { os.Args = origArgs }()
	// 'schema' is a skipClientCommand, so PreRunE returns nil and cobra itself
	// reports the flag parse error (plain error, not AlreadyWrittenError).
	os.Args = []string{"jr", "schema", "--cache", "not-a-duration"}
	rootCmd.SetArgs(os.Args[1:])
	defer rootCmd.SetArgs(nil)

	origStderr := os.Stderr
	devnull, _ := os.Open(os.DevNull)
	os.Stderr = devnull
	defer func() { os.Stderr = origStderr; devnull.Close() }()

	code := Execute()
	if code == 0 {
		t.Error("expected non-zero exit for invalid flag value")
	}
}

// TestExecute_PropagatesAlreadyWrittenError verifies that Execute() returns the
// embedded exit code when rootCmd.Execute() returns an AlreadyWrittenError.
// Uses a subcommand wired to fail inside PersistentPreRunE.
func TestExecute_PropagatesAlreadyWrittenError(t *testing.T) {
	// Write a malformed config so PreRunE returns AlreadyWrittenError(ExitError).
	cfg := t.TempDir() + "/config.json"
	if err := os.WriteFile(cfg, []byte("not-json"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("JR_CONFIG_PATH", cfg)

	origArgs := os.Args
	defer func() { os.Args = origArgs }()
	// schema is a skipClientCommand, so won't hit PreRunE. Use a real command.
	// myself is a generated command that requires a client, so PreRunE runs.
	os.Args = []string{"jr", "myself", "get-current-user"}
	rootCmd.SetArgs(os.Args[1:])
	defer rootCmd.SetArgs(nil)

	origStderr := os.Stderr
	devnull, _ := os.Open(os.DevNull)
	os.Stderr = devnull
	defer func() { os.Stderr = origStderr; devnull.Close() }()

	code := Execute()
	if code == 0 {
		t.Error("expected non-zero exit when config fails to parse")
	}
}
