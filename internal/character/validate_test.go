package character

import (
	"testing"
)

func validCharacter() *Character {
	return &Character{
		Version: "1",
		Name:    "hero",
		Source:  SourceManual,
		StyleGuide: StyleGuide{
			Writing:     "Write well.",
			Workflow:    "Work well.",
			Interaction: "Play well.",
		},
	}
}

func TestValidate_Valid(t *testing.T) {
	if err := Validate(validCharacter()); err != nil {
		t.Fatalf("expected no error for valid character, got: %v", err)
	}
}

func TestValidate_EmptySource(t *testing.T) {
	c := validCharacter()
	c.Source = ""
	if err := Validate(c); err != nil {
		t.Fatalf("empty source should be valid (optional), got: %v", err)
	}
}

func TestValidate_MissingVersion(t *testing.T) {
	c := validCharacter()
	c.Version = ""
	if err := Validate(c); err == nil {
		t.Fatal("expected error for missing version, got nil")
	}
}

func TestValidate_MissingName(t *testing.T) {
	c := validCharacter()
	c.Name = ""
	if err := Validate(c); err == nil {
		t.Fatal("expected error for missing name, got nil")
	}
}

func TestValidate_MissingWriting(t *testing.T) {
	c := validCharacter()
	c.StyleGuide.Writing = ""
	if err := Validate(c); err == nil {
		t.Fatal("expected error for missing style_guide.writing, got nil")
	}
}

func TestValidate_MissingWorkflow(t *testing.T) {
	c := validCharacter()
	c.StyleGuide.Workflow = ""
	if err := Validate(c); err == nil {
		t.Fatal("expected error for missing style_guide.workflow, got nil")
	}
}

func TestValidate_MissingInteraction(t *testing.T) {
	c := validCharacter()
	c.StyleGuide.Interaction = ""
	if err := Validate(c); err == nil {
		t.Fatal("expected error for missing style_guide.interaction, got nil")
	}
}

func TestValidate_InvalidSource(t *testing.T) {
	c := validCharacter()
	c.Source = "unknown-source"
	if err := Validate(c); err == nil {
		t.Fatal("expected error for invalid source, got nil")
	}
}

func TestValidate_AllValidSources(t *testing.T) {
	sources := []string{SourceClone, SourceManual, SourceTemplate, SourcePersona}
	for _, src := range sources {
		c := validCharacter()
		c.Source = src
		if err := Validate(c); err != nil {
			t.Errorf("source %q should be valid, got: %v", src, err)
		}
	}
}
