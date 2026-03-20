package format_test

import (
	"strings"
	"testing"

	"github.com/sofq/jira-cli/internal/format"
)

// ---- Table tests ----

func TestTable_ArrayOfObjects(t *testing.T) {
	input := []byte(`[{"name":"Alice","age":30},{"name":"Bob","age":25}]`)
	out, err := format.Table(input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	got := string(out)

	// Headers must be sorted alphabetically (age before name).
	lines := strings.Split(got, "\n")
	if len(lines) < 4 {
		t.Fatalf("expected at least 4 lines (header, sep, 2 rows), got %d:\n%s", len(lines), got)
	}
	if !strings.HasPrefix(lines[0], "age") {
		t.Errorf("expected first column 'age', got header line: %q", lines[0])
	}
	if !strings.Contains(lines[0], "name") {
		t.Errorf("expected 'name' column in header, got: %q", lines[0])
	}
	// Data rows must contain values.
	if !strings.Contains(got, "Alice") {
		t.Errorf("expected 'Alice' in output:\n%s", got)
	}
	if !strings.Contains(got, "Bob") {
		t.Errorf("expected 'Bob' in output:\n%s", got)
	}
	if !strings.Contains(got, "30") {
		t.Errorf("expected '30' in output:\n%s", got)
	}
}

func TestTable_ColumnWidthAlignment(t *testing.T) {
	input := []byte(`[{"id":1,"summary":"Short"},{"id":2,"summary":"A much longer summary text"}]`)
	out, err := format.Table(input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	lines := strings.Split(string(out), "\n")
	// All data lines should be consistent.
	if len(lines) < 3 {
		t.Fatalf("expected at least 3 lines, got %d", len(lines))
	}
	// The separator line should contain dashes.
	if !strings.Contains(lines[1], "---") {
		t.Errorf("expected separator dashes in line 2, got: %q", lines[1])
	}
}

func TestTable_EmptyArray(t *testing.T) {
	input := []byte(`[]`)
	out, err := format.Table(input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(out) != 0 {
		t.Errorf("expected empty output for empty array, got: %q", out)
	}
}

func TestTable_NonArrayJSON_ReturnsAsIs(t *testing.T) {
	input := []byte(`{"key":"PROJ-1","summary":"A bug"}`)
	out, err := format.Table(input)
	if err != nil {
		t.Fatalf("expected no error for non-array JSON, got: %v", err)
	}
	if string(out) != string(input) {
		t.Errorf("expected input returned unchanged:\ngot:  %s\nwant: %s", out, input)
	}
}

func TestTable_MissingFieldsInSomeRows(t *testing.T) {
	// Second row is missing the "age" field.
	input := []byte(`[{"name":"Alice","age":30},{"name":"Bob"}]`)
	out, err := format.Table(input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	got := string(out)
	if !strings.Contains(got, "Alice") {
		t.Errorf("expected 'Alice' in output:\n%s", got)
	}
	if !strings.Contains(got, "Bob") {
		t.Errorf("expected 'Bob' in output:\n%s", got)
	}
}

func TestTable_HeadersSortedAlphabetically(t *testing.T) {
	// Keys in input are in reverse alphabetical order.
	input := []byte(`[{"z":1,"m":2,"a":3}]`)
	out, err := format.Table(input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	lines := strings.Split(string(out), "\n")
	headerLine := lines[0]
	aIdx := strings.Index(headerLine, "a")
	mIdx := strings.Index(headerLine, "m")
	zIdx := strings.Index(headerLine, "z")
	if aIdx >= mIdx || mIdx >= zIdx {
		t.Errorf("headers not sorted alphabetically, header line: %q", headerLine)
	}
}

// ---- CSV tests ----

func TestCSV_ArrayOfObjects(t *testing.T) {
	input := []byte(`[{"name":"Alice","age":30},{"name":"Bob","age":25}]`)
	out, err := format.CSV(input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	got := string(out)
	lines := strings.Split(got, "\n")
	// Header + 2 data rows = 3 lines.
	if len(lines) != 3 {
		t.Fatalf("expected 3 lines, got %d:\n%s", len(lines), got)
	}
	// Headers sorted: age, name.
	if lines[0] != "age,name" {
		t.Errorf("expected header 'age,name', got: %q", lines[0])
	}
	if !strings.Contains(lines[1], "30") || !strings.Contains(lines[1], "Alice") {
		t.Errorf("unexpected row 1: %q", lines[1])
	}
	if !strings.Contains(lines[2], "25") || !strings.Contains(lines[2], "Bob") {
		t.Errorf("unexpected row 2: %q", lines[2])
	}
}

func TestCSV_EmptyArray(t *testing.T) {
	input := []byte(`[]`)
	out, err := format.CSV(input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(out) != 0 {
		t.Errorf("expected empty output for empty array, got: %q", out)
	}
}

func TestCSV_NonArrayJSON_ReturnsError(t *testing.T) {
	input := []byte(`{"key":"PROJ-1"}`)
	_, err := format.CSV(input)
	if err == nil {
		t.Fatal("expected error for non-array JSON, got nil")
	}
}

func TestCSV_NestedObjects_Stringified(t *testing.T) {
	input := []byte(`[{"key":"PROJ-1","status":{"name":"Open"},"labels":["bug","urgent"]}]`)
	out, err := format.CSV(input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	got := string(out)
	lines := strings.Split(got, "\n")
	if len(lines) != 2 {
		t.Fatalf("expected 2 lines (header + 1 row), got %d:\n%s", len(lines), got)
	}
	// Nested object and array should appear as their JSON representation.
	if !strings.Contains(lines[1], "PROJ-1") {
		t.Errorf("expected key in data row, got: %q", lines[1])
	}
	if !strings.Contains(got, "Open") {
		t.Errorf("expected nested status name 'Open' stringified, got:\n%s", got)
	}
	if !strings.Contains(got, "bug") {
		t.Errorf("expected label 'bug' stringified, got:\n%s", got)
	}
}

func TestCSV_MissingFieldsInSomeRows(t *testing.T) {
	// Second row is missing "age".
	input := []byte(`[{"name":"Alice","age":30},{"name":"Bob"}]`)
	out, err := format.CSV(input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	lines := strings.Split(string(out), "\n")
	if len(lines) != 3 {
		t.Fatalf("expected 3 lines, got %d:\n%s", len(lines), string(out))
	}
	// Bob's age cell should be empty — row ends with comma or just missing value.
	if !strings.Contains(lines[2], "Bob") {
		t.Errorf("expected 'Bob' in row 2, got: %q", lines[2])
	}
}

func TestCSV_HeadersSortedAlphabetically(t *testing.T) {
	input := []byte(`[{"z":1,"m":2,"a":3}]`)
	out, err := format.CSV(input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	lines := strings.Split(string(out), "\n")
	if lines[0] != "a,m,z" {
		t.Errorf("expected header 'a,m,z', got: %q", lines[0])
	}
}
