package template

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

func TestLoadBuiltinTemplates(t *testing.T) {
	templates, err := loadBuiltinTemplates()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	expected := []string{"bug-report", "story", "task", "epic", "subtask", "spike"}
	for _, name := range expected {
		if _, ok := templates[name]; !ok {
			t.Errorf("expected builtin template %q to exist", name)
		}
	}

	if len(templates) != len(expected) {
		t.Errorf("expected %d templates, got %d", len(expected), len(templates))
	}
}

func TestLookupBuiltin(t *testing.T) {
	tmpl, ok, err := Lookup("bug-report")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !ok {
		t.Fatal("expected to find bug-report template")
	}
	if tmpl.Name != "bug-report" {
		t.Errorf("expected name 'bug-report', got %q", tmpl.Name)
	}
	if tmpl.IssueType != "Bug" {
		t.Errorf("expected issuetype 'Bug', got %q", tmpl.IssueType)
	}
}

func TestLookupNotFound(t *testing.T) {
	_, ok, err := Lookup("nonexistent")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ok {
		t.Error("expected nonexistent template to not be found")
	}
}

func TestLookupUserOverride(t *testing.T) {
	tmpDir := t.TempDir()
	origDir := userTemplatesDir
	userTemplatesDir = func() string { return tmpDir }
	defer func() { userTemplatesDir = origDir }()

	// Write a user template that overrides bug-report.
	userTmpl := Template{
		Name:        "bug-report",
		IssueType:   "CustomBug",
		Description: "User override",
		Variables:   []Variable{{Name: "summary", Required: true}},
		Fields:      map[string]string{"summary": "{{.summary}}"},
	}
	data, _ := yaml.Marshal(userTmpl)
	os.WriteFile(filepath.Join(tmpDir, "bug-report.yaml"), data, 0o644)

	tmpl, ok, err := Lookup("bug-report")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !ok {
		t.Fatal("expected to find bug-report template")
	}
	if tmpl.IssueType != "CustomBug" {
		t.Errorf("expected user override issuetype 'CustomBug', got %q", tmpl.IssueType)
	}
}

func TestRenderFields(t *testing.T) {
	tmpl := &Template{
		Variables: []Variable{
			{Name: "summary", Required: true},
			{Name: "description"},
			{Name: "severity", Default: "Medium"},
		},
		Fields: map[string]string{
			"summary":     "{{.summary}}",
			"description": "{{.description}}",
			"priority":    "{{.severity}}",
		},
	}

	rendered, err := RenderFields(tmpl, map[string]string{
		"summary":     "Login broken",
		"description": "Cannot log in",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if rendered["summary"] != "Login broken" {
		t.Errorf("expected summary 'Login broken', got %q", rendered["summary"])
	}
	if rendered["description"] != "Cannot log in" {
		t.Errorf("expected description 'Cannot log in', got %q", rendered["description"])
	}
	if rendered["priority"] != "Medium" {
		t.Errorf("expected priority 'Medium' (default), got %q", rendered["priority"])
	}
}

func TestRenderFieldsMissingRequired(t *testing.T) {
	tmpl := &Template{
		Variables: []Variable{
			{Name: "summary", Required: true},
		},
		Fields: map[string]string{
			"summary": "{{.summary}}",
		},
	}

	_, err := RenderFields(tmpl, map[string]string{})
	if err == nil {
		t.Fatal("expected error for missing required variable")
	}
}

func TestRenderFieldsEmptyOmitted(t *testing.T) {
	tmpl := &Template{
		Variables: []Variable{
			{Name: "summary", Required: true},
			{Name: "description"},
		},
		Fields: map[string]string{
			"summary":     "{{.summary}}",
			"description": "{{.description}}",
		},
	}

	rendered, err := RenderFields(tmpl, map[string]string{
		"summary": "Test",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if _, ok := rendered["description"]; ok {
		t.Error("expected empty description to be omitted")
	}
}

func TestList(t *testing.T) {
	data, err := List()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var result []templateEntry
	if err := json.Unmarshal(data, &result); err != nil {
		t.Fatalf("output is not valid JSON: %v", err)
	}

	if len(result) != 6 {
		t.Errorf("expected 6 templates, got %d", len(result))
	}

	// Should be sorted by name.
	for i := 1; i < len(result); i++ {
		if result[i].Name < result[i-1].Name {
			t.Errorf("templates not sorted: %q comes after %q", result[i].Name, result[i-1].Name)
		}
	}

	// All should be builtin source.
	for _, r := range result {
		if r.Source != "builtin" {
			t.Errorf("expected source 'builtin', got %q for %s", r.Source, r.Name)
		}
	}
}

func TestSave(t *testing.T) {
	tmpDir := t.TempDir()
	origDir := userTemplatesDir
	userTemplatesDir = func() string { return tmpDir }
	defer func() { userTemplatesDir = origDir }()

	tmpl := &Template{
		Name:        "test-tmpl",
		IssueType:   "Task",
		Description: "Test template",
		Variables:   []Variable{{Name: "summary", Required: true}},
		Fields:      map[string]string{"summary": "{{.summary}}"},
	}

	path, err := Save(tmpl, false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if filepath.Base(path) != "test-tmpl.yaml" {
		t.Errorf("expected filename 'test-tmpl.yaml', got %q", filepath.Base(path))
	}

	// Verify file contents.
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("failed to read saved file: %v", err)
	}

	var loaded Template
	if err := yaml.Unmarshal(data, &loaded); err != nil {
		t.Fatalf("failed to parse saved YAML: %v", err)
	}
	if loaded.Name != "test-tmpl" {
		t.Errorf("expected name 'test-tmpl', got %q", loaded.Name)
	}
}

func TestSaveOverwriteProtection(t *testing.T) {
	tmpDir := t.TempDir()
	origDir := userTemplatesDir
	userTemplatesDir = func() string { return tmpDir }
	defer func() { userTemplatesDir = origDir }()

	tmpl := &Template{
		Name:      "dup",
		IssueType: "Task",
		Fields:    map[string]string{"summary": "{{.summary}}"},
	}

	// First save succeeds.
	_, err := Save(tmpl, false)
	if err != nil {
		t.Fatalf("first save failed: %v", err)
	}

	// Second save without overwrite fails.
	_, err = Save(tmpl, false)
	if err == nil {
		t.Fatal("expected error for duplicate save without overwrite")
	}
	if !strings.Contains(err.Error(), "already exists") {
		t.Errorf("expected 'already exists' error, got: %v", err)
	}

	// Third save with overwrite succeeds.
	_, err = Save(tmpl, true)
	if err != nil {
		t.Fatalf("overwrite save failed: %v", err)
	}
}

func TestValidateName(t *testing.T) {
	valid := []string{"bug-report", "my_template", "task1", "a"}
	for _, name := range valid {
		if err := ValidateName(name); err != nil {
			t.Errorf("ValidateName(%q) unexpected error: %v", name, err)
		}
	}

	invalid := []string{"", "../evil", "foo/bar", ".hidden", "-starts-with-dash", "has space", "a@b"}
	for _, name := range invalid {
		if err := ValidateName(name); err == nil {
			t.Errorf("ValidateName(%q) expected error", name)
		}
	}
}

func TestLoadUserTemplates_InvalidYAML(t *testing.T) {
	tmpDir := t.TempDir()
	origDir := userTemplatesDir
	userTemplatesDir = func() string { return tmpDir }
	defer func() { userTemplatesDir = origDir }()

	os.WriteFile(filepath.Join(tmpDir, "bad.yaml"), []byte("invalid: yaml: [unclosed"), 0o644)

	_, _, err := Lookup("anything")
	if err == nil {
		t.Fatal("expected error for invalid YAML in user templates")
	}
}

func TestLoadUserTemplates_SkipsNonYAMLAndDirs(t *testing.T) {
	tmpDir := t.TempDir()
	origDir := userTemplatesDir
	userTemplatesDir = func() string { return tmpDir }
	defer func() { userTemplatesDir = origDir }()

	// Write a non-YAML file and a subdirectory.
	os.WriteFile(filepath.Join(tmpDir, "readme.txt"), []byte("not a template"), 0o644)
	_ = os.Mkdir(filepath.Join(tmpDir, "subdir"), 0o755)

	// Write a valid user template.
	tmpl := Template{
		Name:      "custom",
		IssueType: "Task",
		Fields:    map[string]string{"summary": "{{.summary}}"},
	}
	data, _ := yaml.Marshal(tmpl)
	os.WriteFile(filepath.Join(tmpDir, "custom.yaml"), data, 0o644)

	list, err := List()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var result []templateEntry
	if err := json.Unmarshal(list, &result); err != nil {
		t.Fatalf("output not valid JSON: %v", err)
	}

	// Should have builtin templates + 1 user template, no .txt or subdir.
	foundCustom := false
	for _, r := range result {
		if r.Name == "custom" {
			foundCustom = true
			if r.Source != "user" {
				t.Errorf("expected source 'user' for custom, got %q", r.Source)
			}
		}
		if r.Name == "readme" || r.Name == "subdir" {
			t.Errorf("unexpected entry %q should have been skipped", r.Name)
		}
	}
	if !foundCustom {
		t.Error("expected to find user template 'custom' in list")
	}
}

func TestRenderFields_RequiredWithDefault(t *testing.T) {
	tmpl := &Template{
		Variables: []Variable{
			{Name: "summary", Required: true, Default: "fallback value"},
		},
		Fields: map[string]string{
			"summary": "{{.summary}}",
		},
	}

	rendered, err := RenderFields(tmpl, map[string]string{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if rendered["summary"] != "fallback value" {
		t.Errorf("expected 'fallback value', got %q", rendered["summary"])
	}
}

func TestRenderFields_BadTemplateSyntax(t *testing.T) {
	tmpl := &Template{
		Variables: []Variable{
			{Name: "summary", Required: true},
		},
		Fields: map[string]string{
			"summary": "{{.unclosed",
		},
	}

	_, err := RenderFields(tmpl, map[string]string{"summary": "test"})
	if err == nil {
		t.Fatal("expected error for bad template syntax")
	}
	if !strings.Contains(err.Error(), "parsing template") {
		t.Errorf("expected 'parsing template' in error, got: %v", err)
	}
}

func TestList_WithUserTemplate(t *testing.T) {
	tmpDir := t.TempDir()
	origDir := userTemplatesDir
	userTemplatesDir = func() string { return tmpDir }
	defer func() { userTemplatesDir = origDir }()

	userTmpl := Template{
		Name:        "my-custom",
		IssueType:   "Story",
		Description: "A user template",
		Variables:   []Variable{{Name: "summary", Required: true}},
		Fields:      map[string]string{"summary": "{{.summary}}"},
	}
	data, _ := yaml.Marshal(userTmpl)
	os.WriteFile(filepath.Join(tmpDir, "my-custom.yaml"), data, 0o644)

	list, err := List()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var result []templateEntry
	if err := json.Unmarshal(list, &result); err != nil {
		t.Fatalf("output not valid JSON: %v", err)
	}

	// Should have builtins (6) + 1 user = 7.
	if len(result) != 7 {
		t.Errorf("expected 7 templates, got %d", len(result))
	}

	found := false
	for _, r := range result {
		if r.Name == "my-custom" {
			found = true
			if r.Source != "user" {
				t.Errorf("expected source 'user', got %q", r.Source)
			}
		}
	}
	if !found {
		t.Error("expected to find 'my-custom' in list")
	}
}
