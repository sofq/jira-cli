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

