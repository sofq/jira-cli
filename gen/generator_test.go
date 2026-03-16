package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// withLoadTemplate temporarily overrides the template loader for testing.
func withLoadTemplate(t *testing.T, fn func(string) (string, error)) {
	t.Helper()
	old := loadTemplateFn
	loadTemplateFn = fn
	t.Cleanup(func() { loadTemplateFn = old })
}

func TestBuildPathTemplateMalformedBrace(t *testing.T) {
	// Path with { but no closing } — should pass through literally.
	tmpl, args := buildPathTemplate("/rest/api/{broken")
	if !strings.Contains(tmpl, "{") {
		t.Errorf("expected malformed brace to pass through, got tmpl=%q", tmpl)
	}
	if args != "" {
		t.Errorf("expected no args for malformed path, got %q", args)
	}
}

func TestLoadTemplateNotFound(t *testing.T) {
	_, err := loadTemplate("nonexistent_template.tmpl")
	if err == nil {
		t.Fatal("expected error for nonexistent template, got nil")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("error should mention 'not found', got: %v", err)
	}
}

func TestRenderTemplateLoadError(t *testing.T) {
	_, err := renderTemplate("nonexistent.tmpl", nil)
	if err == nil {
		t.Fatal("expected error from renderTemplate with bad template name")
	}
}

func TestGenerateResourceRenderError(t *testing.T) {
	// Use a resource name but corrupt the template search path won't help here
	// since templates exist. Instead, test with an outDir that can't be written to.
	dir := t.TempDir()
	blocker := filepath.Join(dir, "blocker")
	if err := os.WriteFile(blocker, []byte("x"), 0o444); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	ops := []Operation{
		{OperationID: "getTest", Method: "GET", Path: "/test/{id}",
			PathParams: []Param{{Name: "id", In: "path", Required: true, Type: "string"}}},
	}
	// blocker is a file, not a dir — WriteFile will fail.
	err := GenerateResource("test", ops, blocker)
	if err == nil {
		t.Fatal("expected error writing to non-directory, got nil")
	}
}

func TestGenerateSchemaDataWriteError(t *testing.T) {
	dir := t.TempDir()
	blocker := filepath.Join(dir, "blocker")
	if err := os.WriteFile(blocker, []byte("x"), 0o444); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	groups := map[string][]Operation{
		"test": {{OperationID: "getTest", Method: "GET", Path: "/test"}},
	}
	err := GenerateSchemaData(groups, []string{"test"}, blocker)
	if err == nil {
		t.Fatal("expected error writing to non-directory, got nil")
	}
}

func TestGenerateInitWriteError(t *testing.T) {
	dir := t.TempDir()
	blocker := filepath.Join(dir, "blocker")
	if err := os.WriteFile(blocker, []byte("x"), 0o444); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	err := GenerateInit([]string{"test"}, blocker)
	if err == nil {
		t.Fatal("expected error writing to non-directory, got nil")
	}
}

func TestRenderTemplateParseError(t *testing.T) {
	withLoadTemplate(t, func(name string) (string, error) {
		return "{{ invalid template", nil
	})
	_, err := renderTemplate("bad.tmpl", nil)
	if err == nil {
		t.Fatal("expected parse error")
	}
}

func TestRenderTemplateExecuteError(t *testing.T) {
	withLoadTemplate(t, func(name string) (string, error) {
		return "{{ .MissingMethod }}", nil
	})
	_, err := renderTemplate("exec.tmpl", "a string, not a struct")
	if err == nil {
		t.Fatal("expected execute error")
	}
}

func TestRenderTemplateFormatError(t *testing.T) {
	withLoadTemplate(t, func(name string) (string, error) {
		// Valid template that produces invalid Go code.
		return "not valid go code {{ .X }}", nil
	})
	out, err := renderTemplate("fmt.tmpl", struct{ X string }{X: "val"})
	if err == nil {
		t.Fatal("expected format error")
	}
	// Should still return the unformatted output.
	if len(out) == 0 {
		t.Error("expected unformatted output to be returned on format error")
	}
}

func TestGenerateResourceRenderTemplateError(t *testing.T) {
	withLoadTemplate(t, func(name string) (string, error) {
		return "", fmt.Errorf("injected load error")
	})
	ops := []Operation{
		{OperationID: "getTest", Method: "GET", Path: "/test"},
	}
	err := GenerateResource("test", ops, t.TempDir())
	if err == nil {
		t.Fatal("expected error from renderTemplate failure")
	}
}

func TestGenerateSchemaDataRenderError(t *testing.T) {
	withLoadTemplate(t, func(name string) (string, error) {
		return "", fmt.Errorf("injected load error")
	})
	groups := map[string][]Operation{
		"test": {{OperationID: "getTest", Method: "GET", Path: "/test"}},
	}
	err := GenerateSchemaData(groups, []string{"test"}, t.TempDir())
	if err == nil {
		t.Fatal("expected error from renderTemplate failure")
	}
}

func TestGenerateInitRenderError(t *testing.T) {
	withLoadTemplate(t, func(name string) (string, error) {
		return "", fmt.Errorf("injected load error")
	})
	err := GenerateInit([]string{"test"}, t.TempDir())
	if err == nil {
		t.Fatal("expected error from renderTemplate failure")
	}
}

func TestLoadTemplateCwdFallback(t *testing.T) {
	// loadTemplateDefault with a name that doesn't exist exercises all fallback paths.
	_, err := loadTemplateDefault("nonexistent_xyz.tmpl")
	if err == nil {
		t.Fatal("expected error for nonexistent template")
	}
}

func TestLoadTemplateDefaultCwdFallbackSuccess(t *testing.T) {
	// Create a temp dir with a templates/ subdir containing a unique template.
	// runtime.Caller will look in gen/templates/ (relative to source) — won't find it.
	// Cwd fallback will check templates/<name> relative to tmpDir — will find it.
	tmpDir := t.TempDir()
	tmplDir := filepath.Join(tmpDir, "templates")
	if err := os.MkdirAll(tmplDir, 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	uniqueName := "cwd_fallback_test_only.tmpl"
	if err := os.WriteFile(filepath.Join(tmplDir, uniqueName), []byte("hello_cwd"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	origDir, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd: %v", err)
	}
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatalf("Chdir: %v", err)
	}
	t.Cleanup(func() { os.Chdir(origDir) })

	content, err := loadTemplateDefault(uniqueName)
	if err != nil {
		t.Fatalf("expected cwd fallback to find template: %v", err)
	}
	if content != "hello_cwd" {
		t.Errorf("got %q, want %q", content, "hello_cwd")
	}
}

func TestGenerateSchemaData(t *testing.T) {
	dir := t.TempDir()
	groups := map[string][]Operation{
		"issue": {
			{OperationID: "getIssue", Method: "GET", Path: "/rest/api/3/issue/{id}",
				PathParams:  []Param{{Name: "id", In: "path", Required: true, Type: "string"}},
				QueryParams: []Param{{Name: "fields", In: "query", Required: false, Type: "string", Description: "fields"}},
				HasBody:     false},
			{OperationID: "createIssue", Method: "POST", Path: "/rest/api/3/issue", HasBody: true},
		},
	}

	if err := GenerateSchemaData(groups, []string{"issue"}, dir); err != nil {
		t.Fatalf("GenerateSchemaData: %v", err)
	}

	content, err := os.ReadFile(filepath.Join(dir, "schema_data.go"))
	if err != nil {
		t.Fatalf("reading schema_data.go: %v", err)
	}
	src := string(content)
	for _, want := range []string{"package generated", "get", "create", "body"} {
		if !strings.Contains(src, want) {
			t.Errorf("schema_data.go missing %q", want)
		}
	}
}

func TestGenerateResource(t *testing.T) {
	ops := []Operation{
		{
			OperationID: "getIssue",
			Method:      "GET",
			Path:        "/rest/api/3/issue/{issueIdOrKey}",
			Summary:     "Get issue",
			Description: "Returns an issue.",
			PathParams: []Param{
				{Name: "issueIdOrKey", In: "path", Required: true, Type: "string", Description: "The issue ID or key"},
			},
			QueryParams: []Param{
				{Name: "fields", In: "query", Required: false, Type: "string", Description: "Fields to include"},
				{Name: "expand", In: "query", Required: false, Type: "string", Description: "Fields to expand"},
			},
			HasBody: false,
		},
		{
			OperationID: "createIssue",
			Method:      "POST",
			Path:        "/rest/api/3/issue",
			Summary:     "Create issue",
			Description: "Creates an issue.",
			PathParams:  nil,
			QueryParams: nil,
			HasBody:     true,
		},
	}

	dir := t.TempDir()
	if err := GenerateResource("issue", ops, dir); err != nil {
		t.Fatalf("GenerateResource: %v", err)
	}

	content, err := os.ReadFile(filepath.Join(dir, "issue.go"))
	if err != nil {
		t.Fatalf("reading generated file: %v", err)
	}

	src := string(content)

	checks := []string{
		"package generated",
		"DO NOT EDIT",
		`Use:   "issue"`,
		`Use:   "get"`,
		`Use:   "create"`,
		`issueIdOrKey`,
		`"fields"`,
		`"expand"`,
		`bodyReader`,
		`"body"`,
		`os.Exit`,
	}
	for _, want := range checks {
		if !strings.Contains(src, want) {
			t.Errorf("generated issue.go missing %q", want)
		}
	}
}

func TestGenerateInit(t *testing.T) {
	dir := t.TempDir()
	resources := []string{"issue", "project"}

	if err := GenerateInit(resources, dir); err != nil {
		t.Fatalf("GenerateInit: %v", err)
	}

	content, err := os.ReadFile(filepath.Join(dir, "init.go"))
	if err != nil {
		t.Fatalf("reading generated file: %v", err)
	}

	src := string(content)

	checks := []string{
		"package generated",
		"DO NOT EDIT",
		"issueCmd",
		"projectCmd",
		"RegisterAll",
	}
	for _, want := range checks {
		if !strings.Contains(src, want) {
			t.Errorf("generated init.go missing %q", want)
		}
	}
}
