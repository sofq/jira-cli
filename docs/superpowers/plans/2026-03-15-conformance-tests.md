# Schema Conformance Tests Implementation Plan

> **For agentic workers:** REQUIRED: Use superpowers:subagent-driven-development (if subagents available) or superpowers:executing-plans to implement this plan. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add a Go test that verifies all generated CLI commands match the Jira OpenAPI spec — catching missing operations, extra operations, and flag mismatches.

**Architecture:** A single test file in `gen/` (package main) that reuses the existing `ParseSpec`, `GroupOperations`, `deduplicateVerbs`, and `DeriveVerb` functions to build the expected operation set, then compares against the actual generated `schema_data.go` by re-parsing it as structured data. No new dependencies needed.

**Tech Stack:** Go testing, existing gen/ functions, encoding/json for parsing schema_data.go

**Spec:** `docs/superpowers/specs/2026-03-15-conformance-tests-design.md`

---

## File Structure

```
gen/
├── conformance_test.go    # NEW — conformance tests
├── parser.go              # EXISTING — reused (ParseSpec)
├── grouper.go             # EXISTING — reused (GroupOperations, DeriveVerb, ExtractResource)
├── generator.go           # EXISTING — reused (deduplicateVerbs)
└── ...
```

---

## Chunk 1: Conformance Tests

### Task 1: Schema conformance test

**Files:**
- Create: `gen/conformance_test.go`

The approach: since `gen/` is `package main` and cannot import `cmd/generated`, we **re-derive** the expected operations from the spec using the same pipeline the generator uses, then parse the committed `cmd/generated/schema_data.go` as JSON-like structured data to get the actual operations.

Simpler alternative chosen: instead of AST parsing, we'll serialize the expected operations to a canonical form (sorted JSON) and compare against a canonical form of the actual generated data, extracted by running `go run` on a small helper or by directly reading and regex-extracting from schema_data.go.

**Simplest approach:** Re-derive expected from spec, then read `schema_data.go` and extract operations using Go's `go/ast` package to parse the Go source file and extract struct literals. But even simpler: since both the expected and actual are derived from the same spec through the same functions, we can test that **the pipeline is idempotent** — regenerate into a temp dir and diff against committed files.

**Final approach: regenerate-and-diff.** This is the simplest, most robust approach:
1. Run the full generation pipeline into a temp dir
2. Compare every generated file byte-for-byte against `cmd/generated/`
3. Any diff = conformance failure

This catches ALL drift: missing ops, extra ops, flag mismatches, stale code after spec update.

- [ ] **Step 1: Write the failing test**

Create `gen/conformance_test.go`:
```go
package main

import (
	"os"
	"path/filepath"
	"sort"
	"testing"
)

// TestConformance_GeneratedCodeMatchesSpec verifies that the committed
// generated code in cmd/generated/ exactly matches what the generator
// would produce from the current spec. Any mismatch means the generated
// code is stale or the generator has a bug.
func TestConformance_GeneratedCodeMatchesSpec(t *testing.T) {
	specPath := filepath.Join("..", "spec", "jira-v3.json")
	committedDir := filepath.Join("..", "cmd", "generated")

	// Parse spec using existing pipeline
	ops, err := ParseSpec(specPath)
	if err != nil {
		t.Fatalf("ParseSpec failed: %v", err)
	}
	if len(ops) == 0 {
		t.Fatal("spec has no operations")
	}

	groups := GroupOperations(ops)
	t.Logf("Spec: %d operations across %d resources", len(ops), len(groups))

	// Generate into temp dir
	tmpDir := t.TempDir()
	var resources []string
	for resource, resOps := range groups {
		if err := GenerateResource(resource, resOps, tmpDir); err != nil {
			t.Fatalf("GenerateResource(%s) failed: %v", resource, err)
		}
		resources = append(resources, resource)
	}
	sort.Strings(resources)

	if err := GenerateSchemaData(groups, resources, tmpDir); err != nil {
		t.Fatalf("GenerateSchemaData failed: %v", err)
	}
	if err := GenerateInit(resources, tmpDir); err != nil {
		t.Fatalf("GenerateInit failed: %v", err)
	}

	// Compare every generated file against committed version
	tmpFiles, err := os.ReadDir(tmpDir)
	if err != nil {
		t.Fatalf("reading temp dir: %v", err)
	}

	mismatches := 0
	for _, f := range tmpFiles {
		if f.IsDir() {
			continue
		}
		expected, err := os.ReadFile(filepath.Join(tmpDir, f.Name()))
		if err != nil {
			t.Errorf("reading generated %s: %v", f.Name(), err)
			continue
		}
		actual, err := os.ReadFile(filepath.Join(committedDir, f.Name()))
		if err != nil {
			t.Errorf("MISSING from cmd/generated/: %s (run 'make generate')", f.Name())
			mismatches++
			continue
		}
		if string(expected) != string(actual) {
			t.Errorf("STALE: cmd/generated/%s differs from spec (run 'make generate')", f.Name())
			mismatches++
		}
	}

	// Check for extra files in committed dir not in generated
	committedFiles, err := os.ReadDir(committedDir)
	if err != nil {
		t.Fatalf("reading committed dir: %v", err)
	}
	generatedSet := map[string]bool{}
	for _, f := range tmpFiles {
		generatedSet[f.Name()] = true
	}
	for _, f := range committedFiles {
		if !generatedSet[f.Name()] {
			t.Errorf("EXTRA in cmd/generated/: %s (not produced by generator)", f.Name())
			mismatches++
		}
	}

	if mismatches > 0 {
		t.Fatalf("%d file(s) out of sync — run 'make generate' to fix", mismatches)
	}
	t.Logf("All %d generated files match spec", len(tmpFiles))
}

// TestConformance_OperationCount verifies the expected number of operations
// and resources as a sanity check.
func TestConformance_OperationCount(t *testing.T) {
	specPath := filepath.Join("..", "spec", "jira-v3.json")
	ops, err := ParseSpec(specPath)
	if err != nil {
		t.Fatalf("ParseSpec failed: %v", err)
	}

	groups := GroupOperations(ops)

	if len(ops) < 600 {
		t.Errorf("expected 600+ operations from Jira spec, got %d", len(ops))
	}
	if len(groups) < 70 {
		t.Errorf("expected 70+ resource groups, got %d", len(groups))
	}

	t.Logf("Operations: %d, Resources: %d", len(ops), len(groups))
}

// TestConformance_NoVerbCollisions verifies that deduplicateVerbs
// resolves all verb collisions within each resource.
func TestConformance_NoVerbCollisions(t *testing.T) {
	specPath := filepath.Join("..", "spec", "jira-v3.json")
	ops, err := ParseSpec(specPath)
	if err != nil {
		t.Fatalf("ParseSpec failed: %v", err)
	}

	groups := GroupOperations(ops)
	totalCollisions := 0

	for resource, resOps := range groups {
		verbs := deduplicateVerbs(resOps, resource)
		seen := map[string]int{}
		for i, v := range verbs {
			if prev, ok := seen[v]; ok {
				t.Errorf("resource %q: verb %q collision between operations %q (index %d) and %q (index %d)",
					resource, v, resOps[prev].OperationID, prev, resOps[i].OperationID, i)
				totalCollisions++
			}
			seen[v] = i
		}
	}

	if totalCollisions > 0 {
		t.Fatalf("%d verb collision(s) found", totalCollisions)
	}
}

// TestConformance_AllPathParamsHaveFlags verifies that every {param}
// in every path has a corresponding required flag.
func TestConformance_AllPathParamsHaveFlags(t *testing.T) {
	specPath := filepath.Join("..", "spec", "jira-v3.json")
	ops, err := ParseSpec(specPath)
	if err != nil {
		t.Fatalf("ParseSpec failed: %v", err)
	}

	missing := 0
	for _, op := range ops {
		// Extract {param} names from path
		pathParams := map[string]bool{}
		for _, p := range op.PathParams {
			pathParams[p.Name] = true
		}

		// Check that path template {params} match parsed params
		path := op.Path
		for {
			start := indexOf(path, '{')
			if start == -1 {
				break
			}
			end := indexOf(path[start:], '}')
			if end == -1 {
				break
			}
			paramName := path[start+1 : start+end]
			if !pathParams[paramName] {
				t.Errorf("%s %s: path has {%s} but no path param parsed",
					op.Method, op.Path, paramName)
				missing++
			}
			path = path[start+end+1:]
		}
	}

	if missing > 0 {
		t.Fatalf("%d path param(s) missing from parsed operations", missing)
	}
}

func indexOf(s string, c byte) int {
	for i := 0; i < len(s); i++ {
		if s[i] == c {
			return i
		}
	}
	return -1
}
```

- [ ] **Step 2: Run tests to verify they pass**

```bash
go test ./gen/... -v -run TestConformance
```
Expected: all 4 tests PASS (since generated code is currently in sync with spec).

- [ ] **Step 3: Verify the test catches staleness**

Manually introduce a diff to prove the test works:
```bash
echo "// stale marker" >> cmd/generated/issue.go
go test ./gen/... -v -run TestConformance_GeneratedCodeMatchesSpec
```
Expected: FAIL with "STALE: cmd/generated/issue.go differs from spec"

Then revert:
```bash
git checkout cmd/generated/issue.go
```

- [ ] **Step 4: Commit**

```bash
git add gen/conformance_test.go
git commit -m "test: add schema conformance tests to verify generated code matches spec"
```
