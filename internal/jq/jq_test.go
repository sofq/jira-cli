package jq_test

import (
	"encoding/json"
	"strings"
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

// TestApplyNullInput verifies that applying a filter to the JSON value null
// returns the expected result without error.
func TestApplyNullInput(t *testing.T) {
	out, err := jq.Apply([]byte(`null`), ".")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if string(out) != "null" {
		t.Errorf("expected null, got %s", out)
	}
}

// TestApplyNestedAccess verifies that deep nested field access works correctly.
func TestApplyNestedAccess(t *testing.T) {
	input := []byte(`{"a":{"b":{"c":{"d":"deep-value"}}}}`)
	out, err := jq.Apply(input, ".a.b.c.d")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var result string
	if err := json.Unmarshal(out, &result); err != nil {
		t.Fatalf("failed to unmarshal result: %v", err)
	}
	if result != "deep-value" {
		t.Errorf("expected %q, got %q", "deep-value", result)
	}
}

// TestApplyArrayIndex verifies that .[0] returns the first element of an array.
func TestApplyArrayIndex(t *testing.T) {
	input := []byte(`["first","second","third"]`)
	out, err := jq.Apply(input, ".[0]")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var result string
	if err := json.Unmarshal(out, &result); err != nil {
		t.Fatalf("failed to unmarshal result: %v", err)
	}
	if result != "first" {
		t.Errorf("expected %q, got %q", "first", result)
	}
}

// TestApplyPipe verifies that a piped expression works as expected.
func TestApplyPipe(t *testing.T) {
	input := []byte(`{"key":"PROJ-123"}`)
	out, err := jq.Apply(input, ".key | length")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var result float64
	if err := json.Unmarshal(out, &result); err != nil {
		t.Fatalf("failed to unmarshal result: %v", err)
	}
	// "PROJ-123" has 8 characters.
	if result != 8 {
		t.Errorf("expected length 8, got %v", result)
	}
}

// TestApplyConditional verifies that if/then/else expressions work correctly.
func TestApplyConditional(t *testing.T) {
	input := []byte(`{"x":true}`)
	out, err := jq.Apply(input, `if .x then "yes" else "no" end`)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var result string
	if err := json.Unmarshal(out, &result); err != nil {
		t.Fatalf("failed to unmarshal result: %v", err)
	}
	if result != "yes" {
		t.Errorf("expected %q, got %q", "yes", result)
	}
}

// TestApplyStringInterpolation verifies that jq string interpolation with \()
// produces the expected concatenated string.
func TestApplyStringInterpolation(t *testing.T) {
	input := []byte(`{"name":"World"}`)
	out, err := jq.Apply(input, `"Hello \(.name)"`)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var result string
	if err := json.Unmarshal(out, &result); err != nil {
		t.Fatalf("failed to unmarshal result: %v", err)
	}
	if result != "Hello World" {
		t.Errorf("expected %q, got %q", "Hello World", result)
	}
}

// TestApplySelect verifies that select() correctly filters array elements.
func TestApplySelect(t *testing.T) {
	input := []byte(`[{"name":"a","active":true},{"name":"b","active":false},{"name":"c","active":true}]`)
	out, err := jq.Apply(input, `[.[] | select(.active)]`)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var result []map[string]interface{}
	if err := json.Unmarshal(out, &result); err != nil {
		t.Fatalf("failed to unmarshal result: %v", err)
	}
	if len(result) != 2 {
		t.Fatalf("expected 2 active items, got %d", len(result))
	}
	if result[0]["name"] != "a" {
		t.Errorf("expected first active item name %q, got %q", "a", result[0]["name"])
	}
	if result[1]["name"] != "c" {
		t.Errorf("expected second active item name %q, got %q", "c", result[1]["name"])
	}
}

// TestApplyMathOperations verifies that arithmetic expressions work on numeric
// fields.
func TestApplyMathOperations(t *testing.T) {
	input := []byte(`{"a":7,"b":3}`)
	out, err := jq.Apply(input, ".a + .b")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var result float64
	if err := json.Unmarshal(out, &result); err != nil {
		t.Fatalf("failed to unmarshal result: %v", err)
	}
	if result != 10 {
		t.Errorf("expected 10, got %v", result)
	}
}

// TestApplyNullCoalescing verifies that the // operator returns the alternative
// when the left-hand side is null or missing.
func TestApplyNullCoalescing(t *testing.T) {
	input := []byte(`{"present":"value"}`)
	out, err := jq.Apply(input, `.missing // "default"`)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var result string
	if err := json.Unmarshal(out, &result); err != nil {
		t.Fatalf("failed to unmarshal result: %v", err)
	}
	if result != "default" {
		t.Errorf("expected %q, got %q", "default", result)
	}
}

// TestApplyKeys verifies that the keys builtin returns a sorted array of
// object key names.
func TestApplyKeys(t *testing.T) {
	input := []byte(`{"z":1,"a":2,"m":3}`)
	out, err := jq.Apply(input, "keys")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var result []string
	if err := json.Unmarshal(out, &result); err != nil {
		t.Fatalf("failed to unmarshal result: %v", err)
	}
	expected := []string{"a", "m", "z"}
	if len(result) != len(expected) {
		t.Fatalf("expected %d keys, got %d: %v", len(expected), len(result), result)
	}
	for i, k := range expected {
		if result[i] != k {
			t.Errorf("keys[%d] = %q, want %q", i, result[i], k)
		}
	}
}

// TestApplyEmptyArrayResult verifies that a select filter matching no elements
// returns an empty JSON array when wrapped with [.[]|select(...)].
func TestApplyEmptyArrayResult(t *testing.T) {
	input := []byte(`[{"active":false},{"active":false}]`)
	out, err := jq.Apply(input, `[.[] | select(.active)]`)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var result []interface{}
	if err := json.Unmarshal(out, &result); err != nil {
		t.Fatalf("failed to unmarshal result: %v", err)
	}
	if len(result) != 0 {
		t.Errorf("expected empty array, got %d items", len(result))
	}
}

// TestApplyNoHTMLEscaping verifies that Apply does not HTML-escape &, <, > characters.
func TestApplyNoHTMLEscaping(t *testing.T) {
	input := []byte(`{"summary":"Fix <bug> & deploy","url":"https://example.com?a=1&b=2"}`)

	tests := []struct {
		name   string
		filter string
		want   string
	}{
		{"ampersand in string", ".summary", `"Fix <bug> & deploy"`},
		{"url with ampersand", ".url", `"https://example.com?a=1&b=2"`},
		{"object with special chars", "{s: .summary}", `{"s":"Fix <bug> & deploy"}`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			out, err := jq.Apply(input, tt.filter)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			got := string(out)
			if got != tt.want {
				t.Errorf("got %s, want %s", got, tt.want)
			}
			// Ensure no HTML-escaped sequences
			if strings.Contains(got, `\u0026`) || strings.Contains(got, `\u003c`) || strings.Contains(got, `\u003e`) {
				t.Errorf("output contains HTML-escaped characters: %s", got)
			}
		})
	}
}

// TestApplyNoHTMLEscapingMultipleResults verifies no HTML escaping for multiple results.
func TestApplyNoHTMLEscapingMultipleResults(t *testing.T) {
	input := []byte(`[{"name":"A & B"},{"name":"C < D"}]`)
	out, err := jq.Apply(input, ".[].name")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	got := string(out)
	if strings.Contains(got, `\u0026`) || strings.Contains(got, `\u003c`) {
		t.Errorf("multiple results contain HTML-escaped characters: %s", got)
	}
}

// TestApplyLargeInput verifies that Apply handles a ~50 KB JSON document with
// a complex filter without error and returns a non-empty result.
func TestApplyLargeInput(t *testing.T) {
	// Build a JSON array of 500 issue objects (~50 KB).
	var sb strings.Builder
	sb.WriteString(`{"issues":[`)
	for i := 0; i < 500; i++ {
		if i > 0 {
			sb.WriteByte(',')
		}
		sb.WriteString(`{"key":"PROJ-`)
		sb.WriteString(strings.Repeat("0", 3)) // padding
		sb.WriteString(`","fields":{"summary":"Issue summary text here","status":{"name":"Open"},"priority":{"name":"Medium"}}}`)
	}
	sb.WriteString(`]}`)

	input := []byte(sb.String())
	// Verify the payload is at least 40 KB.
	if len(input) < 40*1024 {
		t.Logf("payload size: %d bytes", len(input))
	}

	out, err := jq.Apply(input, `[.issues[] | {key: .key, summary: .fields.summary}]`)
	if err != nil {
		t.Fatalf("unexpected error on large input: %v", err)
	}
	if len(out) == 0 {
		t.Error("expected non-empty output for large input")
	}

	var result []map[string]interface{}
	if err := json.Unmarshal(out, &result); err != nil {
		t.Fatalf("failed to unmarshal large-input result: %v", err)
	}
	if len(result) != 500 {
		t.Errorf("expected 500 items, got %d", len(result))
	}
}
