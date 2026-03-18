package adf

import (
	"encoding/json"
	"testing"
)

func TestFromText_SingleLine(t *testing.T) {
	result := FromText("Hello world")
	data, err := json.Marshal(result)
	if err != nil {
		t.Fatal(err)
	}
	s := string(data)
	if !contains(s, `"type":"doc"`) {
		t.Errorf("expected doc type, got: %s", s)
	}
	if !contains(s, `"version":1`) {
		t.Errorf("expected version 1, got: %s", s)
	}
	if !contains(s, `"Hello world"`) {
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

func contains(s, substr string) bool {
	return len(s) >= len(substr) && containsHelper(s, substr)
}

func containsHelper(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
