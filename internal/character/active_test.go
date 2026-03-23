package character

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// writeCharFile writes a minimal character YAML file into dir and returns the path.
func writeCharFile(t *testing.T, dir string, c *Character) {
	t.Helper()
	if err := Save(dir, c); err != nil {
		t.Fatalf("writeCharFile: %v", err)
	}
}

// writeStateFile writes a JSON state file and returns its path.
func writeStateFile(t *testing.T, dir, profile, base string, compose map[string]string) string {
	t.Helper()
	path := filepath.Join(dir, "state.json")
	state := map[string]stateEntry{
		profile: {Base: base, Compose: compose},
	}
	data, _ := json.Marshal(state)
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatalf("writeStateFile: %v", err)
	}
	return path
}

func TestResolveActive_FlagCharacter(t *testing.T) {
	dir := t.TempDir()
	c := &Character{
		Version: "1", Name: "flag-char", Source: SourceManual,
		StyleGuide: StyleGuide{Writing: "w", Workflow: "wf", Interaction: "i"},
	}
	writeCharFile(t, dir, c)

	got, err := ResolveActive(ResolveOptions{
		FlagCharacter: "flag-char",
		CharDir:       dir,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got == nil {
		t.Fatal("expected non-nil result")
	}
	if got.Name != "flag-char" {
		t.Errorf("expected name 'flag-char', got %q", got.Name)
	}
}

func TestResolveActive_FlagCharacterNotFound(t *testing.T) {
	dir := t.TempDir()
	_, err := ResolveActive(ResolveOptions{
		FlagCharacter: "missing",
		CharDir:       dir,
	})
	if err == nil {
		t.Fatal("expected error for missing flag character, got nil")
	}
}

func TestResolveActive_ConfigChar(t *testing.T) {
	dir := t.TempDir()
	c := &Character{
		Version: "1", Name: "config-char", Source: SourceManual,
		StyleGuide: StyleGuide{Writing: "w", Workflow: "wf", Interaction: "i"},
	}
	writeCharFile(t, dir, c)

	got, err := ResolveActive(ResolveOptions{
		ConfigChar: "config-char",
		CharDir:    dir,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got == nil || got.Name != "config-char" {
		t.Errorf("expected 'config-char', got %v", got)
	}
}

func TestResolveActive_FlagOverridesConfig(t *testing.T) {
	dir := t.TempDir()
	for _, name := range []string{"flag-char", "config-char"} {
		writeCharFile(t, dir, &Character{
			Version: "1", Name: name, Source: SourceManual,
			StyleGuide: StyleGuide{Writing: "w", Workflow: "wf", Interaction: "i"},
		})
	}

	got, err := ResolveActive(ResolveOptions{
		FlagCharacter: "flag-char",
		ConfigChar:    "config-char",
		CharDir:       dir,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.Name != "flag-char" {
		t.Errorf("flag should take priority, got %q", got.Name)
	}
}

func TestResolveActive_StateFile(t *testing.T) {
	dir := t.TempDir()
	c := &Character{
		Version: "1", Name: "state-char", Source: SourceManual,
		StyleGuide: StyleGuide{Writing: "w", Workflow: "wf", Interaction: "i"},
	}
	writeCharFile(t, dir, c)
	stateFile := writeStateFile(t, dir, "default", "state-char", nil)

	got, err := ResolveActive(ResolveOptions{
		CharDir:     dir,
		ProfileName: "default",
		StateFile:   stateFile,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got == nil || got.Name != "state-char" {
		t.Errorf("expected 'state-char' from state file, got %v", got)
	}
}

func TestResolveActive_StateFileWrongProfile(t *testing.T) {
	dir := t.TempDir()
	c := &Character{
		Version: "1", Name: "state-char", Source: SourceManual,
		StyleGuide: StyleGuide{Writing: "w", Workflow: "wf", Interaction: "i"},
	}
	writeCharFile(t, dir, c)
	stateFile := writeStateFile(t, dir, "other-profile", "state-char", nil)

	got, err := ResolveActive(ResolveOptions{
		CharDir:     dir,
		ProfileName: "default",
		StateFile:   stateFile,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// state file has no entry for "default" — should fall through to nil
	if got != nil {
		t.Errorf("expected nil when no state entry for profile, got %v", got)
	}
}

func TestResolveActive_NilWhenNoneAvailable(t *testing.T) {
	dir := t.TempDir()

	got, err := ResolveActive(ResolveOptions{
		CharDir: dir,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != nil {
		t.Errorf("expected nil when no characters available, got %+v", got)
	}
}

func TestResolveActive_FlagCompose(t *testing.T) {
	dir := t.TempDir()
	base := &Character{
		Version: "1", Name: "base", Source: SourceManual,
		StyleGuide: StyleGuide{Writing: "base writing", Workflow: "base wf", Interaction: "base i"},
	}
	override := &Character{
		Version: "1", Name: "over", Source: SourceManual,
		StyleGuide: StyleGuide{Writing: "override writing", Workflow: "wf2", Interaction: "i2"},
	}
	writeCharFile(t, dir, base)
	writeCharFile(t, dir, override)

	got, err := ResolveActive(ResolveOptions{
		FlagCharacter: "base",
		FlagCompose:   map[string]string{"writing": "over"},
		CharDir:       dir,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.StyleGuide.Writing != "override writing" {
		t.Errorf("expected composed writing, got %q", got.StyleGuide.Writing)
	}
}

func TestSaveState(t *testing.T) {
	dir := t.TempDir()
	stateFile := filepath.Join(dir, "state.json")

	if err := SaveState(stateFile, "default", "my-char", map[string]string{"writing": "other"}); err != nil {
		t.Fatalf("SaveState: %v", err)
	}

	data, err := os.ReadFile(stateFile)
	if err != nil {
		t.Fatalf("read state file: %v", err)
	}
	var state map[string]stateEntry
	if err := json.Unmarshal(data, &state); err != nil {
		t.Fatalf("unmarshal state: %v", err)
	}
	entry, ok := state["default"]
	if !ok {
		t.Fatal("expected 'default' profile entry")
	}
	if entry.Base != "my-char" {
		t.Errorf("expected base 'my-char', got %q", entry.Base)
	}
	if entry.Compose["writing"] != "other" {
		t.Errorf("expected compose writing 'other', got %q", entry.Compose["writing"])
	}
}

func TestSaveState_UpdatesExisting(t *testing.T) {
	dir := t.TempDir()
	stateFile := filepath.Join(dir, "state.json")

	// Write initial state.
	if err := SaveState(stateFile, "p1", "char-a", nil); err != nil {
		t.Fatalf("first SaveState: %v", err)
	}
	// Update a different profile — p1 should be preserved.
	if err := SaveState(stateFile, "p2", "char-b", nil); err != nil {
		t.Fatalf("second SaveState: %v", err)
	}

	data, _ := os.ReadFile(stateFile)
	var state map[string]stateEntry
	json.Unmarshal(data, &state) //nolint:errcheck
	if state["p1"].Base != "char-a" {
		t.Errorf("p1 should still be 'char-a', got %q", state["p1"].Base)
	}
	if state["p2"].Base != "char-b" {
		t.Errorf("p2 should be 'char-b', got %q", state["p2"].Base)
	}
}
