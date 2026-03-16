package main

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
)

// run is the main logic, extracted for testability.
func run(specPath, outDir string) error {
	// 1. Parse the OpenAPI spec.
	fmt.Printf("Parsing spec: %s\n", specPath)
	ops, err := ParseSpec(specPath)
	if err != nil {
		return fmt.Errorf("error parsing spec: %w", err)
	}
	fmt.Printf("  Found %d operations\n", len(ops))

	// 2. Group operations by resource.
	groups := GroupOperations(ops)

	// 3. Sort resources for deterministic output.
	resources := make([]string, 0, len(groups))
	for r := range groups {
		resources = append(resources, r)
	}
	sort.Strings(resources)
	fmt.Printf("  Found %d resource groups\n", len(resources))

	// 4. Clean and recreate the output directory.
	fmt.Printf("Cleaning output directory: %s\n", outDir)
	if err := os.RemoveAll(outDir); err != nil {
		return fmt.Errorf("error cleaning output dir: %w", err)
	}
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		return fmt.Errorf("error creating output dir: %w", err)
	}

	// 5. Generate one file per resource.
	fmt.Println("Generating resource files...")
	for _, resource := range resources {
		if err := GenerateResource(resource, groups[resource], outDir); err != nil {
			return fmt.Errorf("error generating resource %q: %w", resource, err)
		}
		fmt.Printf("  %s (%d ops)\n", resource, len(groups[resource]))
	}

	// 6. Generate schema data.
	fmt.Println("Generating schema_data.go...")
	if err := GenerateSchemaData(groups, resources, outDir); err != nil {
		return fmt.Errorf("error generating schema data: %w", err)
	}

	// 7. Generate init.go (excluding resources with hand-written commands).
	fmt.Println("Generating init.go...")
	if err := GenerateInit(resources, outDir); err != nil {
		return fmt.Errorf("error generating init: %w", err)
	}

	// 8. Summary.
	fmt.Printf("\nDone! Generated %d resource files + schema_data.go + init.go in %s\n",
		len(resources), outDir)
	return nil
}

// exitFn is the function used to exit. Overridable in tests.
var exitFn = os.Exit

func main() {
	specPath := filepath.Join("spec", "jira-v3.json")
	outDir := filepath.Join("cmd", "generated")
	if err := run(specPath, outDir); err != nil {
		fmt.Fprintln(os.Stderr, err)
		exitFn(1)
	}
}
