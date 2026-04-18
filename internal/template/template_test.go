package template

import (
	"encoding/json"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"
	"time"

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
	// Sandbox user templates to a clean empty dir so List() returns only
	// builtin templates, regardless of whatever the runner's real HOME contains.
	origDir := userTemplatesDir
	userTemplatesDir = func() string { return t.TempDir() }
	defer func() { userTemplatesDir = origDir }()

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

// TestRenderFields_EmptyOverrideKeepsDefault verifies that explicitly passing
// an empty string for a required variable with a default does not silently
// omit the field — the default value is preserved.
func TestRenderFields_EmptyOverrideKeepsDefault(t *testing.T) {
	tmpl := &Template{
		Variables: []Variable{
			{Name: "summary", Required: true, Default: "fallback"},
		},
		Fields: map[string]string{
			"summary": "{{.summary}}",
		},
	}

	rendered, err := RenderFields(tmpl, map[string]string{"summary": ""})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if rendered["summary"] != "fallback" {
		t.Errorf("expected default 'fallback' when empty string override provided, got %q", rendered["summary"])
	}
}

// TestRenderFields_RequiredEmptyStringNoDefault verifies that explicitly passing
// an empty string for a required variable that has NO default is treated as
// missing. This exercises the else-if branch at template.go:203-205.
func TestRenderFields_RequiredEmptyStringNoDefault(t *testing.T) {
	tmpl := &Template{
		Variables: []Variable{
			{Name: "summary", Required: true}, // no default
		},
		Fields: map[string]string{
			"summary": "{{.summary}}",
		},
	}

	_, err := RenderFields(tmpl, map[string]string{"summary": ""})
	if err == nil {
		t.Fatal("expected error when required variable is explicitly set to empty string with no default")
	}
	if !strings.Contains(err.Error(), "missing required variables") {
		t.Errorf("expected 'missing required variables' in error, got: %v", err)
	}
	if !strings.Contains(err.Error(), "summary") {
		t.Errorf("expected 'summary' in error, got: %v", err)
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

// TestLoadUserTemplates_MissingDir verifies that loadUserTemplates returns
// (nil, nil) when the user templates directory does not exist — this is the
// expected fast-path when a user has never created any templates.
func TestLoadUserTemplates_MissingDir(t *testing.T) {
	tmpDir := t.TempDir()
	missing := filepath.Join(tmpDir, "does-not-exist")

	origDir := userTemplatesDir
	userTemplatesDir = func() string { return missing }
	defer func() { userTemplatesDir = origDir }()

	// Lookup of a builtin should succeed (user dir missing is not an error).
	_, found, err := Lookup("bug-report")
	if err != nil {
		t.Fatalf("unexpected error when user dir missing: %v", err)
	}
	if !found {
		t.Fatal("expected builtin 'bug-report' to be found")
	}
}

// TestLoadUserTemplates_UnreadableDir verifies that loadUserTemplates (via
// Lookup) returns an error when the templates directory path exists but is not
// a directory (e.g. it is a regular file). os.ReadDir on a file fails with an
// error that is NOT os.ErrNotExist, so the error must be propagated.
func TestLoadUserTemplates_UnreadableDir(t *testing.T) {
	tmpDir := t.TempDir()
	// Create a plain file at the path that userTemplatesDir will return.
	// os.ReadDir on a plain file returns a syscall error, not ErrNotExist.
	filePath := filepath.Join(tmpDir, "not-a-dir")
	if err := os.WriteFile(filePath, []byte("I am a file"), 0o644); err != nil {
		t.Fatalf("setup: %v", err)
	}

	origDir := userTemplatesDir
	userTemplatesDir = func() string { return filePath }
	defer func() { userTemplatesDir = origDir }()

	_, _, err := Lookup("anything")
	if err == nil {
		t.Fatal("expected error when userTemplatesDir points to a file, got nil")
	}
}

// TestList_UnreadableUserTemplatesDir verifies that List propagates errors
// returned by loadUserTemplates when the path is unreadable (not ErrNotExist).
func TestList_UnreadableUserTemplatesDir(t *testing.T) {
	tmpDir := t.TempDir()
	filePath := filepath.Join(tmpDir, "not-a-dir")
	if err := os.WriteFile(filePath, []byte("I am a file"), 0o644); err != nil {
		t.Fatalf("setup: %v", err)
	}

	origDir := userTemplatesDir
	userTemplatesDir = func() string { return filePath }
	defer func() { userTemplatesDir = origDir }()

	_, err := List()
	if err == nil {
		t.Fatal("expected error from List when userTemplatesDir points to a file, got nil")
	}
}

// TestSave_InvalidName verifies that Save rejects template names containing
// characters that fail ValidateName (e.g. spaces).
func TestSave_InvalidName(t *testing.T) {
	tmpDir := t.TempDir()
	origDir := userTemplatesDir
	userTemplatesDir = func() string { return tmpDir }
	defer func() { userTemplatesDir = origDir }()

	tmpl := &Template{
		Name:      "has space",
		IssueType: "Task",
		Fields:    map[string]string{"summary": "{{.summary}}"},
	}

	_, err := Save(tmpl, false)
	if err == nil {
		t.Fatal("expected error for template name with spaces, got nil")
	}
	if !strings.Contains(err.Error(), "invalid template name") {
		t.Errorf("expected 'invalid template name' in error, got: %v", err)
	}
}

// TestSave_WriteError verifies that Save propagates errors from os.WriteFile
// when the templates directory is read-only.
func TestSave_WriteError(t *testing.T) {
	tmpDir := t.TempDir()
	readonlyDir := filepath.Join(tmpDir, "readonly")
	if err := os.MkdirAll(readonlyDir, 0o755); err != nil {
		t.Fatalf("setup MkdirAll: %v", err)
	}
	if err := os.Chmod(readonlyDir, 0o444); err != nil {
		t.Fatalf("setup Chmod: %v", err)
	}
	t.Cleanup(func() { _ = os.Chmod(readonlyDir, 0o755) })

	origDir := userTemplatesDir
	userTemplatesDir = func() string { return readonlyDir }
	defer func() { userTemplatesDir = origDir }()

	tmpl := &Template{
		Name:      "test",
		IssueType: "Task",
		Fields:    map[string]string{"summary": "{{.summary}}"},
	}
	_, err := Save(tmpl, false)
	if err == nil {
		t.Fatal("expected error writing to read-only directory, got nil")
	}
}

// TestSave_MkdirError verifies that Save propagates errors from os.MkdirAll
// when the templates directory cannot be created (e.g. a regular file blocks
// the path).
func TestSave_MkdirError(t *testing.T) {
	tmpDir := t.TempDir()
	// Place a regular file at the path that MkdirAll would need to traverse.
	blockingFile := filepath.Join(tmpDir, "blocker")
	if err := os.WriteFile(blockingFile, []byte("x"), 0o644); err != nil {
		t.Fatalf("setup: %v", err)
	}

	origDir := userTemplatesDir
	// Point userTemplatesDir to a path whose parent is the blocking file,
	// so os.MkdirAll fails because "blocker" is not a directory.
	userTemplatesDir = func() string { return filepath.Join(blockingFile, "templates") }
	defer func() { userTemplatesDir = origDir }()

	tmpl := &Template{
		Name:      "test",
		IssueType: "Task",
		Fields:    map[string]string{"summary": "{{.summary}}"},
	}
	_, err := Save(tmpl, false)
	if err == nil {
		t.Fatal("expected error when MkdirAll fails due to file blocking path, got nil")
	}
}

// TestRenderFields_BadTemplateExecution verifies that RenderFields returns an
// error when a field template parses successfully but fails at execution time.
// Using {{call .summary}} where "summary" is a string (not a function) causes
// text/template to return an error during Execute.
func TestRenderFields_BadTemplateExecution(t *testing.T) {
	tmpl := &Template{
		Variables: []Variable{
			{Name: "summary", Required: true},
		},
		Fields: map[string]string{
			// Parses fine, but Execute will fail: "summary" is a string, not callable.
			"summary": `{{call .summary}}`,
		},
	}

	_, err := RenderFields(tmpl, map[string]string{"summary": "hello"})
	if err == nil {
		t.Fatal("expected error for bad template execution, got nil")
	}
	if !strings.Contains(err.Error(), "rendering field") {
		t.Errorf("expected 'rendering field' in error, got: %v", err)
	}
}

// TestUserTemplatesDir_Default exercises the default userTemplatesDir closure.
func TestUserTemplatesDir_Default(t *testing.T) {
	p := userTemplatesDir()
	if p == "" {
		t.Error("default userTemplatesDir() returned empty string")
	}
	if !filepath.IsAbs(p) {
		t.Errorf("expected absolute path, got %q", p)
	}
	if filepath.Base(p) != "templates" {
		t.Errorf("expected dirname 'templates', got %q", filepath.Base(p))
	}
}

// TestUserTemplatesDir_FallbackOnConfigDirError exercises the fallback path
// when os.UserConfigDir fails.
func TestUserTemplatesDir_FallbackOnConfigDirError(t *testing.T) {
	t.Setenv("HOME", "")
	t.Setenv("XDG_CONFIG_HOME", "")

	p := userTemplatesDir()
	if p == "" {
		t.Error("userTemplatesDir() returned empty even with HOME unset")
	}
	if filepath.Base(p) != "templates" {
		t.Errorf("expected dirname 'templates', got %q", filepath.Base(p))
	}
}

// errFS is a filesystem that always returns an error from Open.
type errFS struct{}

func (errFS) Open(name string) (fs.File, error) {
	return nil, &fs.PathError{Op: "open", Path: name, Err: fs.ErrNotExist}
}

// TestLoadBuiltinTemplates_ReadDirError verifies that loadBuiltinTemplates
// returns an error when the embedded FS fails to read the builtin directory.
func TestLoadBuiltinTemplates_ReadDirError(t *testing.T) {
	origFS := builtinFS
	builtinFS = errFS{}
	defer func() { builtinFS = origFS }()

	_, err := loadBuiltinTemplates()
	if err == nil {
		t.Fatal("expected error when builtinFS fails ReadDir, got nil")
	}
	if !strings.Contains(err.Error(), "reading builtin templates") {
		t.Errorf("expected 'reading builtin templates' in error, got: %v", err)
	}
}

// TestLoadBuiltinTemplates_ReadFileError verifies that loadBuiltinTemplates
// returns an error when a builtin YAML file cannot be read.
func TestLoadBuiltinTemplates_ReadFileError(t *testing.T) {
	// Create a MapFS that has a directory entry but the file read fails
	// by making the file content trigger an error through a broken file.
	origFS := builtinFS
	builtinFS = fstest.MapFS{
		"builtin/good.yaml": {
			Data: []byte("name: good\nissuetype: Task\ndescription: test\nfields:\n  summary: \"{{.summary}}\""),
		},
		"builtin/bad.yaml": {
			// Invalid: MapFS files always succeed reads, so test invalid YAML instead.
			Data: []byte("invalid: yaml: [unclosed"),
		},
	}
	defer func() { builtinFS = origFS }()

	_, err := loadBuiltinTemplates()
	if err == nil {
		t.Fatal("expected error when builtin YAML is invalid, got nil")
	}
	if !strings.Contains(err.Error(), "parsing builtin/bad.yaml") {
		t.Errorf("expected 'parsing builtin/bad.yaml' in error, got: %v", err)
	}
}

// TestLoadBuiltinTemplates_SkipsNonYAMLAndDirs verifies that non-.yaml files
// and subdirectories in the builtin FS are skipped.
func TestLoadBuiltinTemplates_SkipsNonYAMLAndDirs(t *testing.T) {
	origFS := builtinFS
	builtinFS = fstest.MapFS{
		"builtin/valid.yaml": {
			Data: []byte("name: valid\nissuetype: Task\ndescription: test\nfields:\n  summary: \"{{.summary}}\""),
		},
		"builtin/readme.txt": {
			Data: []byte("not a template"),
		},
		"builtin/subdir/nested.yaml": {
			Data: []byte("name: nested\nissuetype: Task"),
		},
	}
	defer func() { builtinFS = origFS }()

	templates, err := loadBuiltinTemplates()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(templates) != 1 {
		t.Errorf("expected 1 template, got %d", len(templates))
	}
	if _, ok := templates["valid"]; !ok {
		t.Error("expected 'valid' template to be loaded")
	}
}

// TestLookup_BuiltinFSError verifies that Lookup propagates errors from
// loadBuiltinTemplates when builtinFS is broken and no user template matches.
func TestLookup_BuiltinFSError(t *testing.T) {
	origFS := builtinFS
	builtinFS = errFS{}
	defer func() { builtinFS = origFS }()

	// Point to empty user dir so user templates load fine but don't match.
	tmpDir := t.TempDir()
	origDir := userTemplatesDir
	userTemplatesDir = func() string { return tmpDir }
	defer func() { userTemplatesDir = origDir }()

	_, _, err := Lookup("anything")
	if err == nil {
		t.Fatal("expected error when builtinFS fails, got nil")
	}
}

// TestList_BuiltinFSError verifies that List propagates errors from
// loadBuiltinTemplates when builtinFS is broken.
func TestList_BuiltinFSError(t *testing.T) {
	origFS := builtinFS
	builtinFS = errFS{}
	defer func() { builtinFS = origFS }()

	_, err := List()
	if err == nil {
		t.Fatal("expected error when builtinFS fails, got nil")
	}
}

// TestLoadUserTemplates_UnreadableFile verifies that loadUserTemplates returns
// an error when a .yaml file in the templates directory cannot be read.
func TestLoadUserTemplates_UnreadableFile(t *testing.T) {
	tmpDir := t.TempDir()
	origDir := userTemplatesDir
	userTemplatesDir = func() string { return tmpDir }
	defer func() { userTemplatesDir = origDir }()

	// Create a .yaml file with no read permissions.
	filePath := filepath.Join(tmpDir, "noperm.yaml")
	if err := os.WriteFile(filePath, []byte("name: test"), 0o644); err != nil {
		t.Fatalf("setup WriteFile: %v", err)
	}
	if err := os.Chmod(filePath, 0o000); err != nil {
		t.Fatalf("setup Chmod: %v", err)
	}
	t.Cleanup(func() { _ = os.Chmod(filePath, 0o644) })

	_, err := loadUserTemplates()
	if err == nil {
		t.Fatal("expected error reading unreadable .yaml file")
	}
	if !strings.Contains(err.Error(), "reading noperm.yaml") {
		t.Errorf("expected 'reading noperm.yaml' in error, got: %v", err)
	}
}

// readDirButFailReadFS is a filesystem that successfully lists directory entries
// via ReadDir but fails when attempting to read (Open) specific files.
type readDirButFailReadFS struct {
	entries []string
}

func (f readDirButFailReadFS) Open(name string) (fs.File, error) {
	// Allow opening the directory itself for ReadDir to work.
	if name == "builtin" {
		return &fakeDirFile{entries: f.entries, pos: 0}, nil
	}
	return nil, &fs.PathError{Op: "open", Path: name, Err: fs.ErrPermission}
}

// fakeDirFile implements fs.File and fs.ReadDirFile for a synthetic directory.
type fakeDirFile struct {
	entries []string
	pos     int
}

func (d *fakeDirFile) Stat() (fs.FileInfo, error) {
	return &fakeDirInfo{}, nil
}

func (d *fakeDirFile) Read([]byte) (int, error) {
	return 0, &fs.PathError{Op: "read", Path: "builtin", Err: fs.ErrInvalid}
}

func (d *fakeDirFile) Close() error { return nil }

func (d *fakeDirFile) ReadDir(n int) ([]fs.DirEntry, error) {
	var result []fs.DirEntry
	for _, name := range d.entries[d.pos:] {
		result = append(result, &fakeDirEntry{name: name})
	}
	d.pos = len(d.entries)
	return result, nil
}

type fakeDirInfo struct{}

func (fakeDirInfo) Name() string      { return "builtin" }
func (fakeDirInfo) Size() int64       { return 0 }
func (fakeDirInfo) Mode() fs.FileMode { return fs.ModeDir | 0o755 }
func (fakeDirInfo) IsDir() bool       { return true }
func (fakeDirInfo) Sys() any          { return nil }
func (fakeDirInfo) ModTime() (t time.Time) { return t }

type fakeDirEntry struct {
	name string
}

func (e *fakeDirEntry) Name() string               { return e.name }
func (e *fakeDirEntry) IsDir() bool                { return false }
func (e *fakeDirEntry) Type() fs.FileMode          { return 0 }
func (e *fakeDirEntry) Info() (fs.FileInfo, error)  { return nil, nil }

// TestLoadBuiltinTemplates_ReadFileErrorFromFS verifies that loadBuiltinTemplates
// returns an error when a builtin .yaml file is listed in the directory but
// cannot be read (fs.ReadFile fails).
func TestLoadBuiltinTemplates_ReadFileErrorFromFS(t *testing.T) {
	origFS := builtinFS
	builtinFS = readDirButFailReadFS{entries: []string{"broken.yaml"}}
	defer func() { builtinFS = origFS }()

	_, err := loadBuiltinTemplates()
	if err == nil {
		t.Fatal("expected error when ReadFile fails for builtin template, got nil")
	}
	if !strings.Contains(err.Error(), "reading builtin/broken.yaml") {
		t.Errorf("expected 'reading builtin/broken.yaml' in error, got: %v", err)
	}
}
