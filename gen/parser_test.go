package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/pb33f/libopenapi/datamodel/high/base"
)

func TestParseSpec_ReturnsOperations(t *testing.T) {
	ops, err := ParseSpec("../spec/jira-v3.json")
	if err != nil {
		t.Fatalf("ParseSpec error: %v", err)
	}
	if len(ops) == 0 {
		t.Fatal("expected operations, got none")
	}
	t.Logf("parsed %d operations", len(ops))
}

func TestParseSpec_GetIssue(t *testing.T) {
	ops, err := ParseSpec("../spec/jira-v3.json")
	if err != nil {
		t.Fatalf("ParseSpec error: %v", err)
	}

	var getIssue *Operation
	for i := range ops {
		if ops[i].OperationID == "getIssue" {
			getIssue = &ops[i]
			break
		}
	}

	if getIssue == nil {
		t.Fatal("getIssue operation not found")
	}

	if getIssue.Method != "GET" {
		t.Errorf("expected method GET, got %s", getIssue.Method)
	}

	if len(getIssue.PathParams) == 0 {
		t.Error("expected path params for getIssue, got none")
	}
}

func TestParseSpec_FileNotFound(t *testing.T) {
	_, err := ParseSpec("nonexistent.json")
	if err == nil {
		t.Fatal("expected error for missing file, got nil")
	}
}

func TestParseSpec_InvalidJSON(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "bad.json")
	if err := os.WriteFile(path, []byte("not json at all"), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	_, err := ParseSpec(path)
	if err == nil {
		t.Fatal("expected error for invalid JSON, got nil")
	}
}

func TestParseSpec_NoPaths(t *testing.T) {
	dir := t.TempDir()
	// Minimal valid OpenAPI 3.0 spec with no paths.
	spec := `{"openapi":"3.0.0","info":{"title":"empty","version":"1.0"}}`
	path := filepath.Join(dir, "empty.json")
	if err := os.WriteFile(path, []byte(spec), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	_, err := ParseSpec(path)
	if err == nil {
		t.Fatal("expected error for spec with no paths, got nil")
	}
}

func TestSchemaType(t *testing.T) {
	// nil schema proxy returns "string"
	got := schemaType(nil)
	if got != "string" {
		t.Errorf("schemaType(nil) = %q, want %q", got, "string")
	}
}

func TestSchemaTypeEmptyType(t *testing.T) {
	// SchemaProxy wrapping a Schema with no Type slice falls back to "string".
	proxy := base.CreateSchemaProxy(&base.Schema{})
	got := schemaType(proxy)
	if got != "string" {
		t.Errorf("schemaType(empty schema) = %q, want %q", got, "string")
	}
}

func TestParseSpec_BuildModelError(t *testing.T) {
	dir := t.TempDir()
	// Swagger 2.0 spec — libopenapi.NewDocument succeeds but BuildV3Model fails
	// with "supplied spec is a different version (oas2)".
	spec := `{"swagger":"2.0","info":{"title":"bad","version":"1.0"},"paths":{}}`
	path := filepath.Join(dir, "swagger2.json")
	if err := os.WriteFile(path, []byte(spec), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	_, err := ParseSpec(path)
	if err == nil {
		t.Fatal("expected error for swagger 2.0 spec, got nil")
	}
}

func TestParseSpec_WithPathLevelParams(t *testing.T) {
	dir := t.TempDir()
	// Valid OpenAPI structure but with broken schema refs to force BuildV3Model errors.
	spec := `{
		"openapi": "3.0.0",
		"info": {"title": "test", "version": "1.0"},
		"paths": {
			"/test": {
				"parameters": [
					{"name": "shared", "in": "query", "schema": {"type": "string"}}
				],
				"get": {
					"operationId": "testOp",
					"summary": "test",
					"parameters": [
						{"name": "id", "in": "path", "required": true, "schema": {"type": "string"}}
					],
					"responses": {"200": {"description": "ok"}}
				},
				"post": {
					"operationId": "testPost",
					"summary": "post test",
					"requestBody": {"content": {"application/json": {"schema": {"type": "object"}}}},
					"responses": {"200": {"description": "ok"}}
				}
			}
		}
	}`
	path := filepath.Join(dir, "test.json")
	if err := os.WriteFile(path, []byte(spec), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	ops, err := ParseSpec(path)
	if err != nil {
		t.Fatalf("ParseSpec: %v", err)
	}
	// Should parse both GET and POST operations.
	if len(ops) < 2 {
		t.Fatalf("expected at least 2 operations, got %d", len(ops))
	}
	// Verify path-level parameter was merged.
	var getOp *Operation
	for i := range ops {
		if ops[i].OperationID == "testOp" {
			getOp = &ops[i]
		}
	}
	if getOp == nil {
		t.Fatal("testOp not found")
	}
	if len(getOp.QueryParams) == 0 {
		t.Error("expected path-level query param to be merged into GET operation")
	}
	// Verify POST has body.
	var postOp *Operation
	for i := range ops {
		if ops[i].OperationID == "testPost" {
			postOp = &ops[i]
		}
	}
	if postOp == nil {
		t.Fatal("testPost not found")
	}
	if !postOp.HasBody {
		t.Error("expected POST operation to have body")
	}
}
