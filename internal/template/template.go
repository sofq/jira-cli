package template

import (
	"bytes"
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"text/template"

	"gopkg.in/yaml.v3"
)

// validTemplateName matches safe template names: alphanumeric, hyphens, underscores.
var validTemplateName = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9_-]*$`)

//go:embed builtin/*.yaml
var builtinFS embed.FS

// Variable defines a template variable with metadata.
type Variable struct {
	Name        string `yaml:"name" json:"name"`
	Required    bool   `yaml:"required" json:"required"`
	Default     string `yaml:"default,omitempty" json:"default,omitempty"`
	Description string `yaml:"description,omitempty" json:"description,omitempty"`
}

// Template defines an issue creation template.
type Template struct {
	Name        string            `yaml:"name" json:"name"`
	IssueType   string            `yaml:"issuetype" json:"issuetype"`
	Description string            `yaml:"description" json:"description"`
	Variables   []Variable        `yaml:"variables" json:"variables"`
	Fields      map[string]string `yaml:"fields" json:"fields"`
}

// userTemplatesDir returns the path to user-defined templates.
// It is a var so tests can override it.
var userTemplatesDir = func() string {
	dir, err := os.UserConfigDir()
	if err != nil {
		home, _ := os.UserHomeDir()
		return filepath.Join(home, ".config", "jr", "templates")
	}
	return filepath.Join(dir, "jr", "templates")
}

// loadBuiltinTemplates reads all built-in templates from the embedded FS.
func loadBuiltinTemplates() (map[string]*Template, error) {
	entries, err := builtinFS.ReadDir("builtin")
	if err != nil {
		return nil, fmt.Errorf("reading builtin templates: %w", err)
	}

	templates := make(map[string]*Template, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".yaml") {
			continue
		}
		data, err := builtinFS.ReadFile("builtin/" + entry.Name())
		if err != nil {
			return nil, fmt.Errorf("reading builtin/%s: %w", entry.Name(), err)
		}
		var tmpl Template
		if err := yaml.Unmarshal(data, &tmpl); err != nil {
			return nil, fmt.Errorf("parsing builtin/%s: %w", entry.Name(), err)
		}
		templates[tmpl.Name] = &tmpl
	}
	return templates, nil
}

// loadUserTemplates reads all user-defined templates from disk.
// Returns (nil, nil) if the directory doesn't exist.
func loadUserTemplates() (map[string]*Template, error) {
	dir := userTemplatesDir()
	entries, err := os.ReadDir(dir)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, err
	}

	templates := make(map[string]*Template, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".yaml") {
			continue
		}
		data, err := os.ReadFile(filepath.Join(dir, entry.Name()))
		if err != nil {
			return nil, fmt.Errorf("reading %s: %w", entry.Name(), err)
		}
		var tmpl Template
		if err := yaml.Unmarshal(data, &tmpl); err != nil {
			return nil, fmt.Errorf("parsing %s: %w", entry.Name(), err)
		}
		templates[tmpl.Name] = &tmpl
	}
	return templates, nil
}

// Lookup resolves a template name. User-defined templates override built-in ones.
func Lookup(name string) (*Template, bool, error) {
	user, err := loadUserTemplates()
	if err != nil {
		return nil, false, err
	}
	if t, ok := user[name]; ok {
		return t, true, nil
	}

	builtin, err := loadBuiltinTemplates()
	if err != nil {
		return nil, false, err
	}
	t, ok := builtin[name]
	return t, ok, nil
}

// templateEntry is used for listing templates with their source.
type templateEntry struct {
	Name        string     `json:"name"`
	IssueType   string     `json:"issuetype"`
	Description string     `json:"description"`
	Variables   []Variable `json:"variables"`
	Source      string     `json:"source"`
}

// List returns all available templates (builtin + user) as JSON bytes.
func List() ([]byte, error) {
	merged := make(map[string]templateEntry)

	builtin, err := loadBuiltinTemplates()
	if err != nil {
		return nil, err
	}
	for name, t := range builtin {
		merged[name] = templateEntry{
			Name:        t.Name,
			IssueType:   t.IssueType,
			Description: t.Description,
			Variables:   t.Variables,
			Source:      "builtin",
		}
	}

	user, err := loadUserTemplates()
	if err != nil {
		return nil, err
	}
	for name, t := range user {
		merged[name] = templateEntry{
			Name:        t.Name,
			IssueType:   t.IssueType,
			Description: t.Description,
			Variables:   t.Variables,
			Source:      "user",
		}
	}

	names := make([]string, 0, len(merged))
	for name := range merged {
		names = append(names, name)
	}
	sort.Strings(names)

	result := make([]templateEntry, 0, len(names))
	for _, name := range names {
		result = append(result, merged[name])
	}

	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(result); err != nil {
		return nil, err
	}
	return bytes.TrimRight(buf.Bytes(), "\n"), nil
}

// RenderFields renders a template's fields with the given variable values.
// Returns a map of field name -> rendered value. Fields with empty rendered
// values are omitted.
func RenderFields(tmpl *Template, vars map[string]string) (map[string]string, error) {
	// Check required variables.
	var missing []string
	for _, v := range tmpl.Variables {
		val, ok := vars[v.Name]
		if v.Required && (!ok || val == "") {
			if v.Default == "" {
				missing = append(missing, v.Name)
			}
		}
	}
	if len(missing) > 0 {
		return nil, fmt.Errorf("missing required variables: %s", strings.Join(missing, ", "))
	}

	// Build template data: merge defaults, then overrides.
	data := make(map[string]string)
	for _, v := range tmpl.Variables {
		if v.Default != "" {
			data[v.Name] = v.Default
		}
	}
	for k, v := range vars {
		data[k] = v
	}

	// Render each field.
	rendered := make(map[string]string, len(tmpl.Fields))
	for field, pattern := range tmpl.Fields {
		t, err := template.New(field).Option("missingkey=zero").Parse(pattern)
		if err != nil {
			return nil, fmt.Errorf("parsing template for field %q: %w", field, err)
		}
		var buf strings.Builder
		if err := t.Execute(&buf, data); err != nil {
			return nil, fmt.Errorf("rendering field %q: %w", field, err)
		}
		val := strings.TrimSpace(buf.String())
		if val != "" {
			rendered[field] = val
		}
	}

	return rendered, nil
}

// ValidateName checks that a template name is safe for use as a filename.
func ValidateName(name string) error {
	if !validTemplateName.MatchString(name) {
		return fmt.Errorf("invalid template name %q: must match [a-zA-Z0-9][a-zA-Z0-9_-]*", name)
	}
	return nil
}

// Save writes a template to the user templates directory.
// Returns an error if a template with the same name already exists
// and overwrite is false.
func Save(tmpl *Template, overwrite bool) (string, error) {
	if err := ValidateName(tmpl.Name); err != nil {
		return "", err
	}

	dir := userTemplatesDir()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", fmt.Errorf("creating templates directory: %w", err)
	}

	path := filepath.Join(dir, tmpl.Name+".yaml")

	if !overwrite {
		if _, err := os.Stat(path); err == nil {
			return "", fmt.Errorf("template %q already exists at %s; use --overwrite to replace", tmpl.Name, path)
		}
	}

	data, err := yaml.Marshal(tmpl)
	if err != nil {
		return "", fmt.Errorf("marshaling template: %w", err)
	}

	if err := os.WriteFile(path, data, 0o644); err != nil {
		return "", fmt.Errorf("writing template: %w", err)
	}
	return path, nil
}
