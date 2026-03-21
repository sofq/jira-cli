package avatar_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sofq/jira-cli/internal/avatar"
)

// validProfileYAML is a well-formed profile YAML used in multiple tests.
const validProfileYAML = `version: 1
user: "test@co.com"
display_name: "Test User"
generated_at: "2026-03-18T10:00:00Z"
engine: "llm"
style_guide:
  writing: "Writes concise comments."
  workflow: "Sets priority always."
  interaction: "Responds quickly."
defaults:
  priority: "Medium"
examples: []
`

// makeMockScript writes a shell script to dir/mock_llm.sh that echoes the
// given YAML on stdout and returns its absolute path.
func makeMockScript(t *testing.T, dir, yamlOutput string) string {
	t.Helper()
	script := filepath.Join(dir, "mock_llm.sh")
	content := "#!/bin/sh\ncat <<'YAML'\n" + yamlOutput + "YAML\n"
	if err := os.WriteFile(script, []byte(content), 0o755); err != nil {
		t.Fatalf("write mock script: %v", err)
	}
	return script
}

// minimalExtraction returns a non-nil Extraction suitable for marshalling.
func minimalExtraction() *avatar.Extraction {
	return &avatar.Extraction{Version: "1"}
}

// TestBuildLLM_MockCommand verifies that BuildLLM correctly parses the YAML
// output of the mock script and sets Engine to "llm".
func TestBuildLLM_MockCommand(t *testing.T) {
	dir := t.TempDir()
	script := filepath.Join(dir, "mock_llm.sh")
	if err := os.WriteFile(script, []byte(`#!/bin/sh
cat <<'YAML'
version: 1
user: "test@co.com"
display_name: "Test User"
generated_at: "2026-03-18T10:00:00Z"
engine: "llm"
style_guide:
  writing: "Writes concise comments."
  workflow: "Sets priority always."
  interaction: "Responds quickly."
defaults:
  priority: "Medium"
examples: []
YAML
`), 0o755); err != nil {
		t.Fatalf("write script: %v", err)
	}

	profile, err := avatar.BuildLLM(minimalExtraction(), script, nil)
	if err != nil {
		t.Fatalf("BuildLLM returned error: %v", err)
	}
	if profile.Engine != "llm" {
		t.Errorf("Engine = %q, want \"llm\"", profile.Engine)
	}
	if profile.User != "test@co.com" {
		t.Errorf("User = %q, want \"test@co.com\"", profile.User)
	}
	if profile.DisplayName != "Test User" {
		t.Errorf("DisplayName = %q, want \"Test User\"", profile.DisplayName)
	}
	if profile.StyleGuide.Writing != "Writes concise comments." {
		t.Errorf("StyleGuide.Writing = %q, unexpected", profile.StyleGuide.Writing)
	}
	if profile.Defaults.Priority != "Medium" {
		t.Errorf("Defaults.Priority = %q, want \"Medium\"", profile.Defaults.Priority)
	}
}

// TestBuildLLM_InvalidOutput verifies that BuildLLM returns an error when the
// command writes invalid or incomplete YAML (e.g. missing required sections).
func TestBuildLLM_InvalidOutput(t *testing.T) {
	dir := t.TempDir()
	// Script outputs something that is syntactically invalid YAML.
	script := filepath.Join(dir, "mock_llm_invalid.sh")
	if err := os.WriteFile(script, []byte("#!/bin/sh\necho 'not: valid: yaml: ::'\n"), 0o755); err != nil {
		t.Fatalf("write script: %v", err)
	}

	_, err := avatar.BuildLLM(minimalExtraction(), script, nil)
	if err == nil {
		t.Error("expected error for invalid YAML output, got nil")
	}
}

// TestBuildLLM_CommandFailure verifies that BuildLLM returns an error when the
// command does not exist.
func TestBuildLLM_CommandFailure(t *testing.T) {
	_, err := avatar.BuildLLM(minimalExtraction(), "/nonexistent/llm_command_xyz", nil)
	if err == nil {
		t.Error("expected error for non-existent command, got nil")
	}
}

// TestBuildLLM_EmptyCmd verifies that an empty llmCmd returns an error.
func TestBuildLLM_EmptyCmd(t *testing.T) {
	_, err := avatar.BuildLLM(minimalExtraction(), "", nil)
	if err == nil {
		t.Error("expected error for empty llmCmd, got nil")
	}
}

// TestBuildLLM_InvalidVersionZero verifies that profile version "0" fails validation.
func TestBuildLLM_InvalidVersionZero(t *testing.T) {
	dir := t.TempDir()
	// Profile with version=0 should fail: version must be >= 1
	badVersionYAML := `version: 0
user: "u"
display_name: "U"
generated_at: "2026-01-01T00:00:00Z"
engine: "llm"
style_guide:
  writing: "Something."
  workflow: "Something."
  interaction: "Something."
`
	script := makeMockScript(t, dir, badVersionYAML)
	_, err := avatar.BuildLLM(minimalExtraction(), script, nil)
	if err == nil {
		t.Error("expected error for version=0, got nil")
	}
}

// TestBuildLLM_InvalidVersionNonNumeric verifies that a non-numeric profile version fails.
func TestBuildLLM_InvalidVersionNonNumeric(t *testing.T) {
	dir := t.TempDir()
	badVersionYAML := `version: "abc"
user: "u"
display_name: "U"
generated_at: "2026-01-01T00:00:00Z"
engine: "llm"
style_guide:
  writing: "Something."
  workflow: "Something."
  interaction: "Something."
`
	script := makeMockScript(t, dir, badVersionYAML)
	_, err := avatar.BuildLLM(minimalExtraction(), script, nil)
	if err == nil {
		t.Error("expected error for non-numeric version, got nil")
	}
}

// TestBuildLLM_EmptyWriting verifies that missing style_guide.writing fails validation.
func TestBuildLLM_EmptyWriting(t *testing.T) {
	dir := t.TempDir()
	missingWritingYAML := `version: 1
user: "u"
display_name: "U"
generated_at: "2026-01-01T00:00:00Z"
engine: "llm"
style_guide:
  writing: ""
  workflow: "Something."
  interaction: "Something."
`
	script := makeMockScript(t, dir, missingWritingYAML)
	_, err := avatar.BuildLLM(minimalExtraction(), script, nil)
	if err == nil {
		t.Error("expected error for empty style_guide.writing, got nil")
	}
}

// TestBuildLLM_EmptyWorkflow verifies that missing style_guide.workflow fails validation.
func TestBuildLLM_EmptyWorkflow(t *testing.T) {
	dir := t.TempDir()
	missingWorkflowYAML := `version: 1
user: "u"
display_name: "U"
generated_at: "2026-01-01T00:00:00Z"
engine: "llm"
style_guide:
  writing: "Something."
  workflow: ""
  interaction: "Something."
`
	script := makeMockScript(t, dir, missingWorkflowYAML)
	_, err := avatar.BuildLLM(minimalExtraction(), script, nil)
	if err == nil {
		t.Error("expected error for empty style_guide.workflow, got nil")
	}
}

// TestBuildLLM_EmptyInteraction verifies that missing style_guide.interaction fails validation.
func TestBuildLLM_EmptyInteraction(t *testing.T) {
	dir := t.TempDir()
	missingInteractionYAML := `version: 1
user: "u"
display_name: "U"
generated_at: "2026-01-01T00:00:00Z"
engine: "llm"
style_guide:
  writing: "Something."
  workflow: "Something."
  interaction: ""
`
	script := makeMockScript(t, dir, missingInteractionYAML)
	_, err := avatar.BuildLLM(minimalExtraction(), script, nil)
	if err == nil {
		t.Error("expected error for empty style_guide.interaction, got nil")
	}
}

// TestBuildLLM_InvalidOverride verifies that an override without '=' returns an error.
func TestBuildLLM_InvalidOverride(t *testing.T) {
	dir := t.TempDir()
	script := makeMockScript(t, dir, validProfileYAML)
	_, err := avatar.BuildLLM(minimalExtraction(), script, []string{"no-equals-sign"})
	if err == nil {
		t.Error("expected error for override without '=', got nil")
	}
}

// TestBuildLLM_CommandKilled verifies that BuildLLM returns an error (not a
// timeout error) when the command exits with a non-zero status due to a signal.
// This exercises the c.Run() error path when ctx.Err() is NOT DeadlineExceeded.
//
// Note: the context.DeadlineExceeded branch inside BuildLLM (the "timed out
// after 5m" message) cannot be reached in a unit test without waiting 5 minutes
// or refactoring BuildLLM to accept an injectable context. The two observable
// error paths are: (a) command not found, and (b) command exits with error —
// both of which flow through the non-deadline branch tested here.
func TestBuildLLM_CommandKilled(t *testing.T) {
	dir := t.TempDir()
	// Script that exits with a non-zero status to simulate a runtime failure.
	script := filepath.Join(dir, "fail.sh")
	if err := os.WriteFile(script, []byte("#!/bin/sh\nexit 1\n"), 0o755); err != nil {
		t.Fatalf("write script: %v", err)
	}

	_, err := avatar.BuildLLM(minimalExtraction(), script, nil)
	if err == nil {
		t.Fatal("expected error for script that exits 1, got nil")
	}
	if !strings.Contains(err.Error(), "llm command failed") {
		t.Errorf("expected error to contain 'llm command failed', got: %v", err)
	}
}

// TestBuildLLM_UnclosedQuoteInCmd verifies that BuildLLM returns an error
// when the llmCmd string has an unclosed quote (shellSplit error branch).
func TestBuildLLM_UnclosedQuoteInCmd(t *testing.T) {
	_, err := avatar.BuildLLM(minimalExtraction(), `cmd "unclosed`, nil)
	if err == nil {
		t.Fatal("expected error for unclosed quote in llmCmd, got nil")
	}
	if !strings.Contains(err.Error(), "invalid llmCmd") {
		t.Errorf("expected 'invalid llmCmd' in error, got: %v", err)
	}
}

// TestBuildLLM_OverridesApplied verifies that key=value overrides are stored
// in profile.Overrides after a successful build.
func TestBuildLLM_OverridesApplied(t *testing.T) {
	dir := t.TempDir()
	script := makeMockScript(t, dir, validProfileYAML)

	overrides := []string{"tone=formal", "greeting=Hello"}
	profile, err := avatar.BuildLLM(minimalExtraction(), script, overrides)
	if err != nil {
		t.Fatalf("BuildLLM returned error: %v", err)
	}
	if profile.Overrides == nil {
		t.Fatal("Overrides map is nil")
	}
	if profile.Overrides["tone"] != "formal" {
		t.Errorf("Overrides[tone] = %q, want \"formal\"", profile.Overrides["tone"])
	}
	if profile.Overrides["greeting"] != "Hello" {
		t.Errorf("Overrides[greeting] = %q, want \"Hello\"", profile.Overrides["greeting"])
	}
	// Engine must still be "llm".
	if profile.Engine != "llm" {
		t.Errorf("Engine = %q, want \"llm\"", profile.Engine)
	}
}
