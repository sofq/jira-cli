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
// resolves all verb collisions within each resource group.
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
				t.Errorf("resource %q: verb %q collision between %q (index %d) and %q (index %d)",
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
// in every path has a corresponding parsed path parameter.
func TestConformance_AllPathParamsHaveFlags(t *testing.T) {
	specPath := filepath.Join("..", "spec", "jira-v3.json")
	ops, err := ParseSpec(specPath)
	if err != nil {
		t.Fatalf("ParseSpec failed: %v", err)
	}

	missing := 0
	for _, op := range ops {
		pathParams := map[string]bool{}
		for _, p := range op.PathParams {
			pathParams[p.Name] = true
		}

		path := op.Path
		for {
			start := indexOfByte(path, '{')
			if start == -1 {
				break
			}
			end := indexOfByte(path[start:], '}')
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

func indexOfByte(s string, c byte) int {
	for i := 0; i < len(s); i++ {
		if s[i] == c {
			return i
		}
	}
	return -1
}
