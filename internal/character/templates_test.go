package character

import (
	"sort"
	"testing"
)

var expectedTemplateNames = []string{"casual", "concise", "detailed", "formal"}

func TestTemplateNames(t *testing.T) {
	names := TemplateNames()

	if len(names) != len(expectedTemplateNames) {
		t.Fatalf("TemplateNames: got %v, want %v", names, expectedTemplateNames)
	}

	// Must be sorted.
	sorted := make([]string, len(names))
	copy(sorted, names)
	sort.Strings(sorted)
	for i, n := range sorted {
		if names[i] != n {
			t.Errorf("TemplateNames not sorted: got %v", names)
			break
		}
	}

	for i, n := range names {
		if n != expectedTemplateNames[i] {
			t.Errorf("TemplateNames[%d]: got %q, want %q", i, n, expectedTemplateNames[i])
		}
	}
}

func TestTemplateReturnsKnownNames(t *testing.T) {
	for _, name := range expectedTemplateNames {
		t.Run(name, func(t *testing.T) {
			c, err := Template(name)
			if err != nil {
				t.Fatalf("Template(%q): %v", name, err)
			}
			if c == nil {
				t.Fatalf("Template(%q): returned nil", name)
			}
		})
	}
}

func TestTemplateUnknownReturnsError(t *testing.T) {
	_, err := Template("nonexistent")
	if err == nil {
		t.Error("expected error for unknown template, got nil")
	}
}

func TestTemplateFields(t *testing.T) {
	for _, name := range expectedTemplateNames {
		t.Run(name, func(t *testing.T) {
			c, err := Template(name)
			if err != nil {
				t.Fatalf("Template(%q): %v", name, err)
			}

			if c.Version != "1" {
				t.Errorf("Version: got %q, want 1", c.Version)
			}
			if c.Source != SourceTemplate {
				t.Errorf("Source: got %q, want %q", c.Source, SourceTemplate)
			}
			if c.Name != name {
				t.Errorf("Name: got %q, want %q", c.Name, name)
			}
			if c.StyleGuide.Writing == "" {
				t.Error("StyleGuide.Writing is empty")
			}
			if c.StyleGuide.Workflow == "" {
				t.Error("StyleGuide.Workflow is empty")
			}
			if c.StyleGuide.Interaction == "" {
				t.Error("StyleGuide.Interaction is empty")
			}
		})
	}
}

func TestTemplateReturnsCopy(t *testing.T) {
	c1, err := Template("concise")
	if err != nil {
		t.Fatalf("Template: %v", err)
	}
	c2, err := Template("concise")
	if err != nil {
		t.Fatalf("Template: %v", err)
	}

	// Mutating c1 must not affect c2.
	original := c2.StyleGuide.Writing
	c1.StyleGuide.Writing = "mutated"

	if c2.StyleGuide.Writing != original {
		t.Error("Template must return independent copies; mutation affected second call")
	}
	if c1 == c2 {
		t.Error("Template must return different pointers")
	}
}

func TestTemplateNamesAreConsistent(t *testing.T) {
	// Every name returned by TemplateNames must resolve via Template.
	for _, name := range TemplateNames() {
		if _, err := Template(name); err != nil {
			t.Errorf("Template(%q) failed: %v", name, err)
		}
	}
}
