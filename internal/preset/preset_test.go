package preset

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestLookup_BuiltinPresets(t *testing.T) {
	tests := []struct {
		name      string
		wantFound bool
	}{
		{"agent", true},
		{"detail", true},
		{"triage", true},
		{"board", true},
		{"nonexistent", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p, ok, err := Lookup(tt.name)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if ok != tt.wantFound {
				t.Errorf("Lookup(%q) found = %v, want %v", tt.name, ok, tt.wantFound)
			}
			if ok && p.Fields == "" {
				t.Errorf("Lookup(%q) returned empty Fields", tt.name)
			}
		})
	}
}

func TestLookup_AgentPresetFields(t *testing.T) {
	p, ok, err := Lookup("agent")
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatal("agent preset not found")
	}
	if p.Fields != "key,summary,status,assignee,issuetype,priority" {
		t.Errorf("unexpected agent fields: %s", p.Fields)
	}
}

func TestLookup_UserPresetsOverrideBuiltin(t *testing.T) {
	tmpDir := t.TempDir()
	presetsFile := filepath.Join(tmpDir, "presets.json")

	userPresets := map[string]Preset{
		"agent":  {Fields: "key,summary", JQ: ".key"},
		"custom": {Fields: "key,status", JQ: ""},
	}
	data, _ := json.Marshal(userPresets)
	if err := os.WriteFile(presetsFile, data, 0o644); err != nil {
		t.Fatal(err)
	}

	orig := userPresetsPath
	userPresetsPath = func() string { return presetsFile }
	t.Cleanup(func() { userPresetsPath = orig })

	// User "agent" should override built-in.
	p, ok, err := Lookup("agent")
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatal("agent preset not found")
	}
	if p.Fields != "key,summary" {
		t.Errorf("expected user override fields, got: %s", p.Fields)
	}
	if p.JQ != ".key" {
		t.Errorf("expected user override jq, got: %s", p.JQ)
	}

	// User "custom" should be available.
	p, ok, err = Lookup("custom")
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatal("custom preset not found")
	}
	if p.Fields != "key,status" {
		t.Errorf("expected custom fields, got: %s", p.Fields)
	}

	// Built-in "triage" should still be available.
	_, ok, err = Lookup("triage")
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatal("triage preset not found (should fall through to built-in)")
	}
}

func TestLookup_MalformedUserPresets(t *testing.T) {
	tmpDir := t.TempDir()
	presetsFile := filepath.Join(tmpDir, "presets.json")
	if err := os.WriteFile(presetsFile, []byte("{bad json"), 0o644); err != nil {
		t.Fatal(err)
	}

	orig := userPresetsPath
	userPresetsPath = func() string { return presetsFile }
	t.Cleanup(func() { userPresetsPath = orig })

	_, _, err := Lookup("agent")
	if err == nil {
		t.Error("expected error for malformed presets file, got nil")
	}
}

func TestLookup_EmptyUserPresetsFile(t *testing.T) {
	tmpDir := t.TempDir()
	presetsFile := filepath.Join(tmpDir, "presets.json")
	if err := os.WriteFile(presetsFile, []byte("{}"), 0o644); err != nil {
		t.Fatal(err)
	}

	orig := userPresetsPath
	userPresetsPath = func() string { return presetsFile }
	t.Cleanup(func() { userPresetsPath = orig })

	// Built-in presets should still work with an empty user file.
	p, ok, err := Lookup("agent")
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatal("agent preset not found with empty user presets file")
	}
	if p.Fields == "" {
		t.Error("agent preset has empty fields")
	}
}

func TestList_ReturnsAllBuiltinPresets(t *testing.T) {
	data, err := List()
	if err != nil {
		t.Fatal(err)
	}

	var entries []struct {
		Name   string `json:"name"`
		Fields string `json:"fields"`
		Source string `json:"source"`
	}
	if err := json.Unmarshal(data, &entries); err != nil {
		t.Fatalf("failed to parse list output: %v", err)
	}

	if len(entries) < 4 {
		t.Errorf("expected at least 4 presets, got %d", len(entries))
	}

	// Verify sorted order.
	for i := 1; i < len(entries); i++ {
		if entries[i].Name < entries[i-1].Name {
			t.Errorf("presets not sorted: %q comes after %q", entries[i].Name, entries[i-1].Name)
		}
	}
}

func TestList_IncludesUserPresets(t *testing.T) {
	tmpDir := t.TempDir()
	presetsFile := filepath.Join(tmpDir, "presets.json")

	userPresets := map[string]Preset{
		"mypreset": {Fields: "key,summary", JQ: ".key"},
	}
	data, _ := json.Marshal(userPresets)
	if err := os.WriteFile(presetsFile, data, 0o644); err != nil {
		t.Fatal(err)
	}

	orig := userPresetsPath
	userPresetsPath = func() string { return presetsFile }
	t.Cleanup(func() { userPresetsPath = orig })

	listData, err := List()
	if err != nil {
		t.Fatal(err)
	}

	var entries []struct {
		Name   string `json:"name"`
		Source string `json:"source"`
	}
	if err := json.Unmarshal(listData, &entries); err != nil {
		t.Fatal(err)
	}

	found := false
	for _, e := range entries {
		if e.Name == "mypreset" && e.Source == "user" {
			found = true
		}
	}
	if !found {
		t.Error("user preset 'mypreset' not found in list output")
	}
}

func TestList_MalformedUserPresets(t *testing.T) {
	tmpDir := t.TempDir()
	presetsFile := filepath.Join(tmpDir, "presets.json")
	if err := os.WriteFile(presetsFile, []byte("{bad json"), 0o644); err != nil {
		t.Fatal(err)
	}

	orig := userPresetsPath
	userPresetsPath = func() string { return presetsFile }
	t.Cleanup(func() { userPresetsPath = orig })

	_, err := List()
	if err == nil {
		t.Error("expected error for malformed presets file, got nil")
	}
}

// TestLoadUserPresets_NonExistentError verifies that loadUserPresets (via
// Lookup) propagates errors that are not os.ErrNotExist. Pointing
// userPresetsPath at a directory causes os.ReadFile to fail with "is a
// directory", which is not ErrNotExist and must be returned as an error.
func TestLoadUserPresets_NonExistentError(t *testing.T) {
	tmpDir := t.TempDir()
	// tmpDir itself is a directory — ReadFile on a directory returns an error
	// that is NOT os.ErrNotExist on all major platforms.
	orig := userPresetsPath
	userPresetsPath = func() string { return tmpDir }
	t.Cleanup(func() { userPresetsPath = orig })

	_, _, err := Lookup("agent")
	if err == nil {
		t.Fatal("expected error when presets path points to a directory, got nil")
	}
}

// TestUserPresetsPath_Default exercises the default userPresetsPath function
// closure to ensure it returns a reasonable path.
func TestUserPresetsPath_Default(t *testing.T) {
	p := userPresetsPath()
	if p == "" {
		t.Error("default userPresetsPath() returned empty string")
	}
	if !filepath.IsAbs(p) {
		t.Errorf("expected absolute path, got %q", p)
	}
	if filepath.Base(p) != "presets.json" {
		t.Errorf("expected filename 'presets.json', got %q", filepath.Base(p))
	}
}

// TestUserPresetsPath_FallbackOnConfigDirError exercises the fallback path
// in userPresetsPath when os.UserConfigDir fails.
func TestUserPresetsPath_FallbackOnConfigDirError(t *testing.T) {
	t.Setenv("HOME", "")
	t.Setenv("XDG_CONFIG_HOME", "")

	p := userPresetsPath()
	if p == "" {
		t.Error("userPresetsPath() returned empty even with HOME unset")
	}
	if filepath.Base(p) != "presets.json" {
		t.Errorf("expected filename 'presets.json', got %q", filepath.Base(p))
	}
}
