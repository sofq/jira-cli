package jq_test

import (
	"encoding/json"
	"testing"

	"github.com/sofq/jira-cli/internal/jq"
)

var sampleJSON = []byte(`{
	"key": "PROJ-123",
	"fields": {
		"summary": "Fix the bug",
		"status": {
			"name": "In Progress"
		}
	}
}`)

var issuesJSON = []byte(`{
	"issues": [
		{"key": "PROJ-1"},
		{"key": "PROJ-2"},
		{"key": "PROJ-3"}
	]
}`)

func TestApply_EmptyFilter(t *testing.T) {
	out, err := jq.Apply(sampleJSON, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if string(out) != string(sampleJSON) {
		t.Errorf("expected input unchanged, got %s", out)
	}
}

func TestApply_SimpleFieldAccess(t *testing.T) {
	out, err := jq.Apply(sampleJSON, ".fields.summary")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var result string
	if err := json.Unmarshal(out, &result); err != nil {
		t.Fatalf("failed to unmarshal result: %v", err)
	}
	if result != "Fix the bug" {
		t.Errorf("expected %q, got %q", "Fix the bug", result)
	}
}

func TestApply_ObjectConstruction(t *testing.T) {
	out, err := jq.Apply(sampleJSON, "{key: .key, status: .fields.status.name}")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var result map[string]interface{}
	if err := json.Unmarshal(out, &result); err != nil {
		t.Fatalf("failed to unmarshal result: %v", err)
	}
	if result["key"] != "PROJ-123" {
		t.Errorf("expected key %q, got %q", "PROJ-123", result["key"])
	}
	if result["status"] != "In Progress" {
		t.Errorf("expected status %q, got %q", "In Progress", result["status"])
	}
}

func TestApply_ArrayFilter(t *testing.T) {
	out, err := jq.Apply(issuesJSON, "[.issues[].key]")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var result []string
	if err := json.Unmarshal(out, &result); err != nil {
		t.Fatalf("failed to unmarshal result: %v", err)
	}
	expected := []string{"PROJ-1", "PROJ-2", "PROJ-3"}
	if len(result) != len(expected) {
		t.Fatalf("expected %d items, got %d", len(expected), len(result))
	}
	for i, v := range expected {
		if result[i] != v {
			t.Errorf("index %d: expected %q, got %q", i, v, result[i])
		}
	}
}

func TestApply_InvalidFilter(t *testing.T) {
	_, err := jq.Apply(sampleJSON, "!!!invalid!!!")
	if err == nil {
		t.Fatal("expected error for invalid filter, got nil")
	}
}

func TestApply_InvalidJSON(t *testing.T) {
	_, err := jq.Apply([]byte(`not json`), ".foo")
	if err == nil {
		t.Fatal("expected error for invalid JSON, got nil")
	}
}

func TestApply_MultipleResults(t *testing.T) {
	out, err := jq.Apply(issuesJSON, ".issues[].key")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	var result []string
	if err := json.Unmarshal(out, &result); err != nil {
		t.Fatalf("expected JSON array, got %s; err: %v", out, err)
	}
	expected := []string{"PROJ-1", "PROJ-2", "PROJ-3"}
	if len(result) != len(expected) {
		t.Fatalf("expected %d items, got %d", len(expected), len(result))
	}
	for i, v := range expected {
		if result[i] != v {
			t.Errorf("index %d: expected %q, got %q", i, v, result[i])
		}
	}
}

func TestApply_NoResults(t *testing.T) {
	// A filter that produces no output (empty stream) — use `empty`
	out, err := jq.Apply(sampleJSON, "empty")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if string(out) != "null" {
		t.Errorf("expected null for no results, got %s", out)
	}
}
