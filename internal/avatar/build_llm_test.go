package avatar_test

import (
	"os"
	"path/filepath"
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
