package adf

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestFromText_SingleLine(t *testing.T) {
	result := FromText("Hello world")
	data, err := json.Marshal(result)
	if err != nil {
		t.Fatal(err)
	}
	s := string(data)
	if !strings.Contains(s, `"type":"doc"`) {
		t.Errorf("expected doc type, got: %s", s)
	}
	if !strings.Contains(s, `"version":1`) {
		t.Errorf("expected version 1, got: %s", s)
	}
	if !strings.Contains(s, `"Hello world"`) {
		t.Errorf("expected text content, got: %s", s)
	}
}

func TestFromText_MultiLine(t *testing.T) {
	result := FromText("line1\nline2\nline3")
	if len(result.Content) != 3 {
		t.Errorf("expected 3 paragraphs, got %d", len(result.Content))
	}
}

func TestFromText_Empty(t *testing.T) {
	result := FromText("")
	if len(result.Content) != 1 {
		t.Errorf("expected 1 empty paragraph, got %d", len(result.Content))
	}
}

func TestFromText_TrailingNewline(t *testing.T) {
	// Bug fix: trailing newline should not create an empty paragraph
	// with an empty text node (invalid ADF).
	result := FromText("Hello\nWorld\n")
	if len(result.Content) != 2 {
		t.Errorf("expected 2 paragraphs (trailing newline trimmed), got %d", len(result.Content))
	}
	if result.Content[0].Content[0].Text != "Hello" {
		t.Errorf("expected first paragraph text 'Hello', got %q", result.Content[0].Content[0].Text)
	}
	if result.Content[1].Content[0].Text != "World" {
		t.Errorf("expected second paragraph text 'World', got %q", result.Content[1].Content[0].Text)
	}
}

func TestFromText_MultipleTrailingNewlines(t *testing.T) {
	result := FromText("Hello\n\n\n")
	if len(result.Content) != 1 {
		t.Errorf("expected 1 paragraph (trailing newlines trimmed), got %d", len(result.Content))
	}
}

func TestFromText_OnlyNewlines(t *testing.T) {
	// A string of only newlines should produce the empty doc (same as "").
	result := FromText("\n\n")
	if len(result.Content) != 1 {
		t.Errorf("expected 1 empty paragraph, got %d", len(result.Content))
	}
}

func TestFromText_RoundTrip(t *testing.T) {
	result := FromText("test")
	data, err := json.Marshal(result)
	if err != nil {
		t.Fatal(err)
	}
	if !json.Valid(data) {
		t.Error("output is not valid JSON")
	}
}

