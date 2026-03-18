package cmd

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/sofq/jira-cli/internal/avatar"
	"github.com/sofq/jira-cli/internal/config"
	jrerrors "github.com/sofq/jira-cli/internal/errors"
	"github.com/spf13/cobra"
)

// newAvatarStatusCmd creates a standalone avatar status command for testing.
func newAvatarStatusCmd() *cobra.Command {
	cmd := &cobra.Command{Use: "status", RunE: runAvatarStatus}
	cmd.SetContext(context.Background())
	return cmd
}

// newAvatarShowCmd creates a standalone avatar show command for testing.
func newAvatarShowCmd() *cobra.Command {
	cmd := &cobra.Command{Use: "show", RunE: runAvatarShow}
	cmd.SetContext(context.Background())
	return cmd
}

// setTempAvatarBase points resolveAvatarDirFromDisk at a temp directory via
// the JR_AVATAR_BASE env var.
func setTempAvatarBase(t *testing.T, base string) {
	t.Helper()
	orig := os.Getenv("JR_AVATAR_BASE")
	t.Cleanup(func() { os.Setenv("JR_AVATAR_BASE", orig) })
	os.Setenv("JR_AVATAR_BASE", base)
}

// TestAvatarSubcommands_RegisteredWithMerge verifies that the hand-written
// avatar subcommands (extract, build, etc.) are accessible after merging with
// the generated avatar command. This was a bug where the generated avatarCmd
// from the OpenAPI spec shadowed the hand-written one, making custom
// subcommands like "extract" unreachable.
func TestAvatarSubcommands_RegisteredWithMerge(t *testing.T) {
	// The root command is initialized via init(), so avatarCmd should already
	// have both hand-written and generated subcommands merged.
	wantSubs := []string{"extract", "build", "prompt", "show", "edit", "refresh", "status"}
	gotSubs := make(map[string]bool)
	for _, sub := range avatarCmd.Commands() {
		gotSubs[sub.Name()] = true
	}
	for _, name := range wantSubs {
		if !gotSubs[name] {
			t.Errorf("avatar subcommand %q not found — mergeCommand may not be wiring custom subcommands", name)
		}
	}

	// Also verify the generated subcommand is preserved.
	if !gotSubs["get-all-system"] {
		t.Error("generated subcommand \"get-all-system\" should be preserved after merge")
	}
}

// TestAvatarStatusCmd_NoProfile verifies that `jr avatar status` outputs
// {"exists":false} when no avatar data is present (empty avatars directory).
func TestAvatarStatusCmd_NoProfile(t *testing.T) {
	base := t.TempDir()
	setTempAvatarBase(t, base)
	// base is empty — no user subdirectories.

	r, w, _ := os.Pipe()
	origStdout := os.Stdout
	os.Stdout = w

	cmd := newAvatarStatusCmd()
	err := cmd.RunE(cmd, nil)

	w.Close()
	os.Stdout = origStdout

	var buf bytes.Buffer
	_, _ = buf.ReadFrom(r)
	output := strings.TrimSpace(buf.String())

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var result map[string]any
	if jsonErr := json.Unmarshal([]byte(output), &result); jsonErr != nil {
		t.Fatalf("output is not valid JSON: %q — error: %v", output, jsonErr)
	}

	if exists := result["exists"]; exists != false {
		t.Errorf("expected {\"exists\":false}, got: %s", output)
	}
}

// TestAvatarShowCmd_NoProfile verifies that `jr avatar show` returns a
// not_found error (exit code 3) when no profile exists.
func TestAvatarShowCmd_NoProfile(t *testing.T) {
	base := t.TempDir()
	setTempAvatarBase(t, base)

	var stderr bytes.Buffer

	cmd := newAvatarShowCmd()
	cmd.SetErr(&stderr)

	err := cmd.RunE(cmd, nil)
	if err == nil {
		t.Fatal("expected error but got nil")
	}

	awe, ok := err.(*jrerrors.AlreadyWrittenError)
	if !ok {
		t.Fatalf("expected AlreadyWrittenError, got %T: %v", err, err)
	}
	if awe.Code != jrerrors.ExitNotFound {
		t.Errorf("expected exit code %d (not_found), got %d", jrerrors.ExitNotFound, awe.Code)
	}
}

// TestAvatarStatusCmd_WithProfile verifies that status returns profile info
// when a valid profile.yaml exists.
func TestAvatarStatusCmd_WithProfile(t *testing.T) {
	base := t.TempDir()
	setTempAvatarBase(t, base)

	avatarDir := filepath.Join(base, "abc123def456789a")
	if err := os.MkdirAll(avatarDir, 0o700); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}

	p := &avatar.Profile{
		Version:     "1",
		User:        "account:123",
		DisplayName: "Test User",
		GeneratedAt: "2026-01-01T00:00:00Z",
		Engine:      "local",
	}
	if err := avatar.SaveProfile(avatarDir, p); err != nil {
		t.Fatalf("SaveProfile: %v", err)
	}

	r, w, _ := os.Pipe()
	origStdout := os.Stdout
	os.Stdout = w

	cmd := newAvatarStatusCmd()
	err := cmd.RunE(cmd, nil)

	w.Close()
	os.Stdout = origStdout

	var buf bytes.Buffer
	_, _ = buf.ReadFrom(r)
	output := strings.TrimSpace(buf.String())

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var result map[string]any
	if jsonErr := json.Unmarshal([]byte(output), &result); jsonErr != nil {
		t.Fatalf("output is not valid JSON: %q — error: %v", output, jsonErr)
	}

	if exists := result["exists"]; exists != true {
		t.Errorf("expected exists=true, got: %s", output)
	}
	if user := result["user"]; user != "account:123" {
		t.Errorf("expected user=account:123, got: %v", user)
	}
	if engine := result["engine"]; engine != "local" {
		t.Errorf("expected engine=local, got: %v", engine)
	}
}

// ---------------------------------------------------------------------------
// Helper constructors for test commands
// ---------------------------------------------------------------------------

// newAvatarPromptCmd creates a standalone avatar prompt command for testing.
func newAvatarPromptCmd() *cobra.Command {
	cmd := &cobra.Command{Use: "prompt", RunE: runAvatarPrompt}
	cmd.Flags().String("format", "prose", "output format")
	cmd.Flags().StringSlice("section", nil, "sections to include")
	cmd.Flags().Bool("redact", false, "redact PII")
	cmd.SetContext(context.Background())
	return cmd
}

// newAvatarEditCmd creates a standalone avatar edit command for testing.
func newAvatarEditCmd() *cobra.Command {
	cmd := &cobra.Command{Use: "edit", RunE: runAvatarEdit}
	cmd.SetContext(context.Background())
	return cmd
}

// newAvatarExtractCmd creates a standalone avatar extract command for testing
// (no client in context — exercises the client.FromContext error path).
func newAvatarExtractCmd() *cobra.Command {
	cmd := &cobra.Command{Use: "extract", RunE: runAvatarExtract}
	cmd.Flags().String("user", "", "")
	cmd.Flags().Int("min-comments", 50, "")
	cmd.Flags().Int("min-updates", 30, "")
	cmd.Flags().String("max-window", "6m", "")
	cmd.SetContext(context.Background())
	return cmd
}

// newAvatarBuildCmd creates a standalone avatar build command for testing
// (no client in context — exercises the client.FromContext error path).
func newAvatarBuildCmd() *cobra.Command {
	cmd := &cobra.Command{Use: "build", RunE: runAvatarBuild}
	cmd.Flags().String("user", "", "")
	cmd.Flags().String("engine", "", "")
	cmd.Flags().String("llm-cmd", "", "")
	cmd.Flags().Bool("yes", false, "")
	cmd.SetContext(context.Background())
	return cmd
}

// newAvatarRefreshCmd creates a standalone avatar refresh command for testing
// (no client in context — exercises the client.FromContext error path).
func newAvatarRefreshCmd() *cobra.Command {
	cmd := &cobra.Command{Use: "refresh", RunE: runAvatarRefresh}
	cmd.Flags().String("user", "", "")
	cmd.Flags().Int("min-comments", 50, "")
	cmd.Flags().Int("min-updates", 30, "")
	cmd.Flags().String("max-window", "6m", "")
	cmd.Flags().String("engine", "", "")
	cmd.Flags().String("llm-cmd", "", "")
	cmd.Flags().Bool("yes", false, "")
	cmd.SetContext(context.Background())
	return cmd
}

// captureStdoutTrimmed wraps the shared captureStdout and trims surrounding
// whitespace from the output.
func captureStdoutTrimmed(t *testing.T, f func()) string {
	t.Helper()
	return strings.TrimSpace(captureStdout(t, f))
}

// makeProfile creates a profile.yaml inside dir and returns the profile.
func makeProfile(t *testing.T, dir, user, displayName, engine string) *avatar.Profile {
	t.Helper()
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	p := &avatar.Profile{
		Version:     "1",
		User:        user,
		DisplayName: displayName,
		GeneratedAt: "2099-01-01T00:00:00Z",
		Engine:      engine,
		StyleGuide: avatar.StyleGuide{
			Writing:     "Be concise.",
			Workflow:    "Use small PRs.",
			Interaction: "Respond quickly.",
		},
	}
	if err := avatar.SaveProfile(dir, p); err != nil {
		t.Fatalf("SaveProfile: %v", err)
	}
	return p
}

// makeExtraction creates a minimal extraction.json inside dir.
func makeExtraction(t *testing.T, dir string, comments, issuesCreated, transitions int) {
	t.Helper()
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	ext := &avatar.Extraction{
		Version: "1",
		Meta: avatar.ExtractionMeta{
			User:        "account:abc",
			DisplayName: "Test User",
			DataPoints: avatar.DataPoints{
				Comments:      comments,
				IssuesCreated: issuesCreated,
				Transitions:   transitions,
			},
		},
	}
	if err := avatar.SaveExtraction(dir, ext); err != nil {
		t.Fatalf("SaveExtraction: %v", err)
	}
}

// ---------------------------------------------------------------------------
// resolveAvatarDirFromDisk tests
// ---------------------------------------------------------------------------

// TestResolveAvatarDirFromDisk_EnvOverride verifies that JR_AVATAR_BASE is
// honoured and a single user dir is returned directly.
func TestResolveAvatarDirFromDisk_EnvOverride(t *testing.T) {
	base := t.TempDir()
	setTempAvatarBase(t, base)

	userDir := filepath.Join(base, "abc123def456789a")
	if err := os.MkdirAll(userDir, 0o700); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}

	got, err := resolveAvatarDirFromDisk()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != userDir {
		t.Errorf("got %q, want %q", got, userDir)
	}
}

// TestResolveAvatarDirFromDisk_NoEntries verifies the error when the base
// directory contains no sub-directories.
func TestResolveAvatarDirFromDisk_NoEntries(t *testing.T) {
	base := t.TempDir()
	setTempAvatarBase(t, base)

	_, err := resolveAvatarDirFromDisk()
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "no avatar data found") {
		t.Errorf("unexpected error message: %v", err)
	}
}

// TestResolveAvatarDirFromDisk_BaseNotExist verifies the not-exist error path.
func TestResolveAvatarDirFromDisk_BaseNotExist(t *testing.T) {
	// Point JR_AVATAR_BASE at a path that does not exist.
	setTempAvatarBase(t, filepath.Join(t.TempDir(), "does_not_exist"))

	_, err := resolveAvatarDirFromDisk()
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "no avatar data found") {
		t.Errorf("unexpected error message: %v", err)
	}
}

// TestResolveAvatarDirFromDisk_MultipleUsers verifies that, when several user
// directories exist, the one with the most recently modified profile.yaml is
// selected.
func TestResolveAvatarDirFromDisk_MultipleUsers(t *testing.T) {
	base := t.TempDir()
	setTempAvatarBase(t, base)

	older := filepath.Join(base, "user_older")
	newer := filepath.Join(base, "user_newer")
	for _, d := range []string{older, newer} {
		if err := os.MkdirAll(d, 0o700); err != nil {
			t.Fatalf("MkdirAll: %v", err)
		}
	}

	// Write older profile first, then newer to produce distinct mod-times.
	makeProfile(t, older, "account:old", "Old User", "local")
	// Bump the newer profile's mtime by backdating the older one.
	olderTime := filepath.Join(older, "profile.yaml")
	past := time.Now().Add(-2 * time.Second)
	if err := os.Chtimes(olderTime, past, past); err != nil {
		t.Fatalf("Chtimes: %v", err)
	}
	makeProfile(t, newer, "account:new", "New User", "local")

	got, err := resolveAvatarDirFromDisk()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != newer {
		t.Errorf("expected newer dir %q, got %q", newer, got)
	}
}

// TestResolveAvatarDirFromDisk_MultipleUsersNoProfile verifies the fallback
// when multiple dirs exist but none contains a profile.yaml: the first dir is
// returned without error.
func TestResolveAvatarDirFromDisk_MultipleUsersNoProfile(t *testing.T) {
	base := t.TempDir()
	setTempAvatarBase(t, base)

	dirA := filepath.Join(base, "aaaa")
	dirB := filepath.Join(base, "bbbb")
	for _, d := range []string{dirA, dirB} {
		if err := os.MkdirAll(d, 0o700); err != nil {
			t.Fatalf("MkdirAll: %v", err)
		}
	}

	got, err := resolveAvatarDirFromDisk()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != dirA && got != dirB {
		t.Errorf("unexpected dir %q", got)
	}
}

// ---------------------------------------------------------------------------
// runAvatarPrompt tests
// ---------------------------------------------------------------------------

// TestAvatarPromptCmd_NoProfile verifies that prompt returns not_found when no
// avatar directory exists.
func TestAvatarPromptCmd_NoProfile(t *testing.T) {
	base := t.TempDir()
	setTempAvatarBase(t, base)

	var stderr bytes.Buffer
	cmd := newAvatarPromptCmd()
	cmd.SetErr(&stderr)

	err := cmd.RunE(cmd, nil)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	awe, ok := err.(*jrerrors.AlreadyWrittenError)
	if !ok {
		t.Fatalf("expected AlreadyWrittenError, got %T: %v", err, err)
	}
	if awe.Code != jrerrors.ExitNotFound {
		t.Errorf("expected ExitNotFound (%d), got %d", jrerrors.ExitNotFound, awe.Code)
	}
}

// TestAvatarPromptCmd_NoProfileYAML verifies that prompt returns not_found
// when the user directory exists but contains no profile.yaml.
func TestAvatarPromptCmd_NoProfileYAML(t *testing.T) {
	base := t.TempDir()
	setTempAvatarBase(t, base)

	if err := os.MkdirAll(filepath.Join(base, "abc123"), 0o700); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}

	var stderr bytes.Buffer
	cmd := newAvatarPromptCmd()
	cmd.SetErr(&stderr)

	err := cmd.RunE(cmd, nil)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	awe, ok := err.(*jrerrors.AlreadyWrittenError)
	if !ok {
		t.Fatalf("expected AlreadyWrittenError, got %T: %v", err, err)
	}
	if awe.Code != jrerrors.ExitNotFound {
		t.Errorf("expected ExitNotFound (%d), got %d", jrerrors.ExitNotFound, awe.Code)
	}
}

// TestAvatarPromptCmd_ValidProfile verifies that prompt outputs prose text
// when a valid profile exists.
func TestAvatarPromptCmd_ValidProfile(t *testing.T) {
	base := t.TempDir()
	setTempAvatarBase(t, base)

	avatarDir := filepath.Join(base, "abc123def456789a")
	makeProfile(t, avatarDir, "account:123", "Prose User", "local")

	output := captureStdoutTrimmed(t, func() {
		cmd := newAvatarPromptCmd()
		if err := cmd.RunE(cmd, nil); err != nil {
			t.Errorf("unexpected error: %v", err)
		}
	})

	if !strings.Contains(output, "Style Profile") {
		t.Errorf("expected prose output with 'Style Profile', got: %q", output)
	}
	if !strings.Contains(output, "Prose User") {
		t.Errorf("expected display name in output, got: %q", output)
	}
}

// TestAvatarPromptCmd_JSONFormat verifies that --format=json produces JSON
// output.
func TestAvatarPromptCmd_JSONFormat(t *testing.T) {
	base := t.TempDir()
	setTempAvatarBase(t, base)

	avatarDir := filepath.Join(base, "abc123def456789a")
	makeProfile(t, avatarDir, "account:123", "JSON User", "local")

	output := captureStdoutTrimmed(t, func() {
		cmd := newAvatarPromptCmd()
		if err := cmd.Flags().Set("format", "json"); err != nil {
			t.Fatalf("Set flag: %v", err)
		}
		if err := cmd.RunE(cmd, nil); err != nil {
			t.Errorf("unexpected error: %v", err)
		}
	})

	var result map[string]any
	if err := json.Unmarshal([]byte(output), &result); err != nil {
		t.Fatalf("expected JSON output, got: %q — error: %v", output, err)
	}
	if result["user"] != "account:123" {
		t.Errorf("expected user=account:123, got: %v", result["user"])
	}
}

// TestAvatarPromptCmd_BothFormat verifies that --format=both contains prose
// and JSON separated by a horizontal rule.
func TestAvatarPromptCmd_BothFormat(t *testing.T) {
	base := t.TempDir()
	setTempAvatarBase(t, base)

	avatarDir := filepath.Join(base, "abc123def456789a")
	makeProfile(t, avatarDir, "account:123", "Both User", "local")

	output := captureStdoutTrimmed(t, func() {
		cmd := newAvatarPromptCmd()
		if err := cmd.Flags().Set("format", "both"); err != nil {
			t.Fatalf("Set flag: %v", err)
		}
		if err := cmd.RunE(cmd, nil); err != nil {
			t.Errorf("unexpected error: %v", err)
		}
	})

	if !strings.Contains(output, "Style Profile") {
		t.Errorf("expected prose section in 'both' output, got: %q", output)
	}
	if !strings.Contains(output, "---") {
		t.Errorf("expected separator '---' in 'both' output, got: %q", output)
	}
}

// TestAvatarPromptCmd_Redact verifies that --redact replaces PII in the output.
func TestAvatarPromptCmd_Redact(t *testing.T) {
	base := t.TempDir()
	setTempAvatarBase(t, base)

	avatarDir := filepath.Join(base, "abc123def456789a")
	if err := os.MkdirAll(avatarDir, 0o700); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	p := &avatar.Profile{
		Version:     "1",
		User:        "account:123",
		DisplayName: "Redact User",
		GeneratedAt: "2099-01-01T00:00:00Z",
		Engine:      "local",
		StyleGuide: avatar.StyleGuide{
			Writing: "Contact user@example.com for PROJ-1 tickets.",
		},
	}
	if err := avatar.SaveProfile(avatarDir, p); err != nil {
		t.Fatalf("SaveProfile: %v", err)
	}

	output := captureStdoutTrimmed(t, func() {
		cmd := newAvatarPromptCmd()
		if err := cmd.Flags().Set("redact", "true"); err != nil {
			t.Fatalf("Set flag: %v", err)
		}
		if err := cmd.RunE(cmd, nil); err != nil {
			t.Errorf("unexpected error: %v", err)
		}
	})

	if strings.Contains(output, "user@example.com") {
		t.Errorf("expected email to be redacted, got: %q", output)
	}
	if strings.Contains(output, "PROJ-1") {
		t.Errorf("expected issue key to be redacted, got: %q", output)
	}
}

// ---------------------------------------------------------------------------
// runAvatarShow tests
// ---------------------------------------------------------------------------

// TestAvatarShowCmd_ValidProfile verifies that show outputs indented JSON for
// a valid profile.
func TestAvatarShowCmd_ValidProfile(t *testing.T) {
	base := t.TempDir()
	setTempAvatarBase(t, base)

	avatarDir := filepath.Join(base, "abc123def456789a")
	makeProfile(t, avatarDir, "account:xyz", "Show User", "local")

	output := captureStdoutTrimmed(t, func() {
		cmd := newAvatarShowCmd()
		if err := cmd.RunE(cmd, nil); err != nil {
			t.Errorf("unexpected error: %v", err)
		}
	})

	var result map[string]any
	if err := json.Unmarshal([]byte(output), &result); err != nil {
		t.Fatalf("expected valid JSON, got: %q — error: %v", output, err)
	}
	if result["user"] != "account:xyz" {
		t.Errorf("expected user=account:xyz, got: %v", result["user"])
	}
	if result["display_name"] != "Show User" {
		t.Errorf("expected display_name=Show User, got: %v", result["display_name"])
	}
}

// TestAvatarShowCmd_NoUserDir verifies the not_found path when the base dir
// has no user subdirectories.
func TestAvatarShowCmd_NoUserDir(t *testing.T) {
	base := t.TempDir()
	setTempAvatarBase(t, base)

	var stderr bytes.Buffer
	cmd := newAvatarShowCmd()
	cmd.SetErr(&stderr)

	err := cmd.RunE(cmd, nil)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	awe, ok := err.(*jrerrors.AlreadyWrittenError)
	if !ok {
		t.Fatalf("expected AlreadyWrittenError, got %T: %v", err, err)
	}
	if awe.Code != jrerrors.ExitNotFound {
		t.Errorf("expected ExitNotFound (%d), got %d", jrerrors.ExitNotFound, awe.Code)
	}
}

// ---------------------------------------------------------------------------
// runAvatarEdit tests
// ---------------------------------------------------------------------------

// TestAvatarEditCmd_NoProfile verifies that edit returns not_found when no
// avatar directory is set up.
func TestAvatarEditCmd_NoProfile(t *testing.T) {
	base := t.TempDir()
	setTempAvatarBase(t, base)

	var stderr bytes.Buffer
	cmd := newAvatarEditCmd()
	cmd.SetErr(&stderr)

	err := cmd.RunE(cmd, nil)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	awe, ok := err.(*jrerrors.AlreadyWrittenError)
	if !ok {
		t.Fatalf("expected AlreadyWrittenError, got %T: %v", err, err)
	}
	if awe.Code != jrerrors.ExitNotFound {
		t.Errorf("expected ExitNotFound (%d), got %d", jrerrors.ExitNotFound, awe.Code)
	}
}

// TestAvatarEditCmd_NoProfileYAML verifies that edit returns not_found when the
// user dir exists but has no profile.yaml.
func TestAvatarEditCmd_NoProfileYAML(t *testing.T) {
	base := t.TempDir()
	setTempAvatarBase(t, base)

	if err := os.MkdirAll(filepath.Join(base, "abc123"), 0o700); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}

	var stderr bytes.Buffer
	cmd := newAvatarEditCmd()
	cmd.SetErr(&stderr)

	err := cmd.RunE(cmd, nil)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	awe, ok := err.(*jrerrors.AlreadyWrittenError)
	if !ok {
		t.Fatalf("expected AlreadyWrittenError, got %T: %v", err, err)
	}
	if awe.Code != jrerrors.ExitNotFound {
		t.Errorf("expected ExitNotFound (%d), got %d", jrerrors.ExitNotFound, awe.Code)
	}
}

// TestAvatarEditCmd_EditorSuccess verifies that edit outputs {"status":"saved"}
// when the editor exits cleanly. EDITOR is set to "true" (a shell no-op).
func TestAvatarEditCmd_EditorSuccess(t *testing.T) {
	base := t.TempDir()
	setTempAvatarBase(t, base)

	avatarDir := filepath.Join(base, "abc123def456789a")
	makeProfile(t, avatarDir, "account:123", "Edit User", "local")

	orig := os.Getenv("EDITOR")
	t.Cleanup(func() { os.Setenv("EDITOR", orig) })
	os.Setenv("EDITOR", "true")

	output := captureStdoutTrimmed(t, func() {
		cmd := newAvatarEditCmd()
		if err := cmd.RunE(cmd, nil); err != nil {
			t.Errorf("unexpected error: %v", err)
		}
	})

	var result map[string]any
	if err := json.Unmarshal([]byte(output), &result); err != nil {
		t.Fatalf("expected JSON output, got: %q — error: %v", output, err)
	}
	if result["status"] != "saved" {
		t.Errorf("expected status=saved, got: %v", result["status"])
	}
}

// TestAvatarEditCmd_EditorFailure verifies that edit returns an ExitError when
// the editor exits non-zero. EDITOR is set to "false".
func TestAvatarEditCmd_EditorFailure(t *testing.T) {
	base := t.TempDir()
	setTempAvatarBase(t, base)

	avatarDir := filepath.Join(base, "abc123def456789a")
	makeProfile(t, avatarDir, "account:123", "Edit User", "local")

	orig := os.Getenv("EDITOR")
	t.Cleanup(func() { os.Setenv("EDITOR", orig) })
	os.Setenv("EDITOR", "false")

	var stderr bytes.Buffer
	cmd := newAvatarEditCmd()
	cmd.SetErr(&stderr)

	err := cmd.RunE(cmd, nil)
	if err == nil {
		t.Fatal("expected error from failing editor, got nil")
	}
	awe, ok := err.(*jrerrors.AlreadyWrittenError)
	if !ok {
		t.Fatalf("expected AlreadyWrittenError, got %T: %v", err, err)
	}
	if awe.Code != jrerrors.ExitError {
		t.Errorf("expected ExitError (%d), got %d", jrerrors.ExitError, awe.Code)
	}
}

// ---------------------------------------------------------------------------
// runAvatarStatus additional tests
// ---------------------------------------------------------------------------

// TestAvatarStatusCmd_WithExtraction verifies that status includes data_points
// when an extraction file is present alongside the profile.
func TestAvatarStatusCmd_WithExtraction(t *testing.T) {
	base := t.TempDir()
	setTempAvatarBase(t, base)

	avatarDir := filepath.Join(base, "abc123def456789a")
	makeProfile(t, avatarDir, "account:123", "Data User", "local")
	makeExtraction(t, avatarDir, 75, 10, 20)

	output := captureStdoutTrimmed(t, func() {
		cmd := newAvatarStatusCmd()
		if err := cmd.RunE(cmd, nil); err != nil {
			t.Errorf("unexpected error: %v", err)
		}
	})

	var result map[string]any
	if err := json.Unmarshal([]byte(output), &result); err != nil {
		t.Fatalf("expected valid JSON, got: %q — error: %v", output, err)
	}
	if result["exists"] != true {
		t.Errorf("expected exists=true, got: %v", result["exists"])
	}
	dp, ok := result["data_points"].(map[string]any)
	if !ok {
		t.Fatalf("expected data_points object, got: %T", result["data_points"])
	}
	if dp["comments"] != float64(75) {
		t.Errorf("expected data_points.comments=75, got: %v", dp["comments"])
	}
}

// TestAvatarStatusCmd_StaleProfile verifies that a profile older than 30 days
// reports stale=true.
func TestAvatarStatusCmd_StaleProfile(t *testing.T) {
	base := t.TempDir()
	setTempAvatarBase(t, base)

	avatarDir := filepath.Join(base, "abc123def456789a")
	if err := os.MkdirAll(avatarDir, 0o700); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}

	// Profile generated 40 days ago.
	staleTime := time.Now().UTC().Add(-40 * 24 * time.Hour)
	p := &avatar.Profile{
		Version:     "1",
		User:        "account:123",
		DisplayName: "Stale User",
		GeneratedAt: staleTime.Format(time.RFC3339),
		Engine:      "local",
	}
	if err := avatar.SaveProfile(avatarDir, p); err != nil {
		t.Fatalf("SaveProfile: %v", err)
	}

	output := captureStdoutTrimmed(t, func() {
		cmd := newAvatarStatusCmd()
		if err := cmd.RunE(cmd, nil); err != nil {
			t.Errorf("unexpected error: %v", err)
		}
	})

	var result map[string]any
	if err := json.Unmarshal([]byte(output), &result); err != nil {
		t.Fatalf("expected valid JSON, got: %q — error: %v", output, err)
	}
	if result["stale"] != true {
		t.Errorf("expected stale=true for 40-day-old profile, got: %v", result["stale"])
	}
}

// TestAvatarStatusCmd_FreshProfile verifies that a recently generated profile
// reports stale=false.
func TestAvatarStatusCmd_FreshProfile(t *testing.T) {
	base := t.TempDir()
	setTempAvatarBase(t, base)

	avatarDir := filepath.Join(base, "abc123def456789a")
	makeProfile(t, avatarDir, "account:123", "Fresh User", "local")

	output := captureStdoutTrimmed(t, func() {
		cmd := newAvatarStatusCmd()
		if err := cmd.RunE(cmd, nil); err != nil {
			t.Errorf("unexpected error: %v", err)
		}
	})

	var result map[string]any
	if err := json.Unmarshal([]byte(output), &result); err != nil {
		t.Fatalf("expected valid JSON, got: %q — error: %v", output, err)
	}
	if result["stale"] != false {
		t.Errorf("expected stale=false for fresh profile, got: %v", result["stale"])
	}
}

// ---------------------------------------------------------------------------
// loadAvatarConfig tests
// ---------------------------------------------------------------------------

// TestLoadAvatarConfig_NoConfigFile verifies that loadAvatarConfig succeeds
// and returns nil when no config file exists.
func TestLoadAvatarConfig_NoConfigFile(t *testing.T) {
	tmp := t.TempDir()
	orig := os.Getenv("JR_CONFIG_PATH")
	t.Cleanup(func() { os.Setenv("JR_CONFIG_PATH", orig) })
	os.Setenv("JR_CONFIG_PATH", filepath.Join(tmp, "no_such_config.json"))

	got, err := loadAvatarConfig("")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != nil {
		t.Errorf("expected nil AvatarConfig when no profile exists, got: %+v", got)
	}
}

// TestLoadAvatarConfig_WithAvatarSection verifies that the avatar section is
// parsed correctly from a real config file.
func TestLoadAvatarConfig_WithAvatarSection(t *testing.T) {
	tmp := t.TempDir()
	cfgPath := filepath.Join(tmp, "config.json")

	cfg := &config.Config{
		DefaultProfile: "default",
		Profiles: map[string]config.Profile{
			"default": {
				BaseURL: "https://example.atlassian.net",
				Auth:    config.AuthConfig{Type: "basic", Token: "tok"},
				Avatar: &config.AvatarConfig{
					Engine:      "local",
					MinComments: 100,
					MaxWindow:   "3m",
				},
			},
		},
	}
	if err := config.SaveTo(cfg, cfgPath); err != nil {
		t.Fatalf("SaveTo: %v", err)
	}

	orig := os.Getenv("JR_CONFIG_PATH")
	t.Cleanup(func() { os.Setenv("JR_CONFIG_PATH", orig) })
	os.Setenv("JR_CONFIG_PATH", cfgPath)

	got, err := loadAvatarConfig("")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got == nil {
		t.Fatal("expected non-nil AvatarConfig, got nil")
	}
	if got.Engine != "local" {
		t.Errorf("expected engine=local, got: %q", got.Engine)
	}
	if got.MinComments != 100 {
		t.Errorf("expected min_comments=100, got: %d", got.MinComments)
	}
}

// TestLoadAvatarConfig_NoAvatarSection verifies that loadAvatarConfig returns
// nil when the profile exists but has no avatar config section.
func TestLoadAvatarConfig_NoAvatarSection(t *testing.T) {
	tmp := t.TempDir()
	cfgPath := filepath.Join(tmp, "config.json")

	cfg := &config.Config{
		DefaultProfile: "default",
		Profiles: map[string]config.Profile{
			"default": {
				BaseURL: "https://example.atlassian.net",
				Auth:    config.AuthConfig{Type: "basic", Token: "tok"},
			},
		},
	}
	if err := config.SaveTo(cfg, cfgPath); err != nil {
		t.Fatalf("SaveTo: %v", err)
	}

	orig := os.Getenv("JR_CONFIG_PATH")
	t.Cleanup(func() { os.Setenv("JR_CONFIG_PATH", orig) })
	os.Setenv("JR_CONFIG_PATH", cfgPath)

	got, err := loadAvatarConfig("")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != nil {
		t.Errorf("expected nil AvatarConfig for profile without avatar section, got: %+v", got)
	}
}

// TestLoadAvatarConfig_NamedProfile verifies that a named profile is selected
// when profileName is non-empty.
func TestLoadAvatarConfig_NamedProfile(t *testing.T) {
	tmp := t.TempDir()
	cfgPath := filepath.Join(tmp, "config.json")

	cfg := &config.Config{
		Profiles: map[string]config.Profile{
			"ci": {
				BaseURL: "https://ci.atlassian.net",
				Auth:    config.AuthConfig{Type: "basic", Token: "tok"},
				Avatar:  &config.AvatarConfig{Engine: "llm"},
			},
		},
	}
	if err := config.SaveTo(cfg, cfgPath); err != nil {
		t.Fatalf("SaveTo: %v", err)
	}

	orig := os.Getenv("JR_CONFIG_PATH")
	t.Cleanup(func() { os.Setenv("JR_CONFIG_PATH", orig) })
	os.Setenv("JR_CONFIG_PATH", cfgPath)

	got, err := loadAvatarConfig("ci")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got == nil {
		t.Fatal("expected non-nil AvatarConfig for named profile, got nil")
	}
	if got.Engine != "llm" {
		t.Errorf("expected engine=llm, got: %q", got.Engine)
	}
}

// ---------------------------------------------------------------------------
// Client-required commands — no-client error path
// ---------------------------------------------------------------------------

// TestAvatarExtractCmd_NoClient verifies that extract returns an ExitError
// when no Jira client is available in the command context.
func TestAvatarExtractCmd_NoClient(t *testing.T) {
	base := t.TempDir()
	setTempAvatarBase(t, base)

	var stderr bytes.Buffer
	cmd := newAvatarExtractCmd()
	cmd.SetErr(&stderr)

	err := cmd.RunE(cmd, nil)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	awe, ok := err.(*jrerrors.AlreadyWrittenError)
	if !ok {
		t.Fatalf("expected AlreadyWrittenError, got %T: %v", err, err)
	}
	if awe.Code != jrerrors.ExitError {
		t.Errorf("expected ExitError (%d), got %d", jrerrors.ExitError, awe.Code)
	}
}

// TestAvatarBuildCmd_NoClient verifies that build returns an ExitError when no
// Jira client is available in the command context.
func TestAvatarBuildCmd_NoClient(t *testing.T) {
	base := t.TempDir()
	setTempAvatarBase(t, base)

	var stderr bytes.Buffer
	cmd := newAvatarBuildCmd()
	cmd.SetErr(&stderr)

	err := cmd.RunE(cmd, nil)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	awe, ok := err.(*jrerrors.AlreadyWrittenError)
	if !ok {
		t.Fatalf("expected AlreadyWrittenError, got %T: %v", err, err)
	}
	if awe.Code != jrerrors.ExitError {
		t.Errorf("expected ExitError (%d), got %d", jrerrors.ExitError, awe.Code)
	}
}

// TestAvatarRefreshCmd_NoClient verifies that refresh returns an ExitError
// when no Jira client is available. refresh delegates to extract first, which
// fails immediately.
func TestAvatarRefreshCmd_NoClient(t *testing.T) {
	base := t.TempDir()
	setTempAvatarBase(t, base)

	var stderr bytes.Buffer
	cmd := newAvatarRefreshCmd()
	cmd.SetErr(&stderr)

	err := cmd.RunE(cmd, nil)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	awe, ok := err.(*jrerrors.AlreadyWrittenError)
	if !ok {
		t.Fatalf("expected AlreadyWrittenError, got %T: %v", err, err)
	}
	if awe.Code != jrerrors.ExitError {
		t.Errorf("expected ExitError (%d), got %d", jrerrors.ExitError, awe.Code)
	}
}
