package main

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sort"
)

// run is the main logic, extracted for testability.
func run(specPath, outDir string) error {
	// 1. Parse the OpenAPI spec.
	log.Printf("Parsing spec: %s", specPath)
	ops, err := ParseSpec(specPath)
	if err != nil {
		return fmt.Errorf("error parsing spec: %w", err)
	}
	log.Printf("  Found %d operations", len(ops))

	// 2. Group operations by resource.
	groups := GroupOperations(ops)

	// 3. Sort resources for deterministic output.
	resources := make([]string, 0, len(groups))
	for r := range groups {
		resources = append(resources, r)
	}
	sort.Strings(resources)
	log.Printf("  Found %d resource groups", len(resources))

	// 4. Clean and recreate the output directory.
	log.Printf("Cleaning output directory: %s", outDir)
	if err := os.RemoveAll(outDir); err != nil {
		return fmt.Errorf("error cleaning output dir: %w", err)
	}
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		return fmt.Errorf("error creating output dir: %w", err)
	}

	// 5. Generate one file per resource.
	log.Println("Generating resource files...")
	for _, resource := range resources {
		if err := GenerateResource(resource, groups[resource], outDir); err != nil {
			return fmt.Errorf("error generating resource %q: %w", resource, err)
		}
		log.Printf("  %s (%d ops)", resource, len(groups[resource]))
	}

	// 6. Generate schema data.
	log.Println("Generating schema_data.go...")
	if err := GenerateSchemaData(groups, resources, outDir); err != nil {
		return fmt.Errorf("error generating schema data: %w", err)
	}

	// 7. Generate init.go (excluding resources with hand-written commands).
	log.Println("Generating init.go...")
	if err := GenerateInit(resources, outDir); err != nil {
		return fmt.Errorf("error generating init: %w", err)
	}

	// 8. Summary.
	log.Printf("Done! Generated %d resource files + schema_data.go + init.go in %s",
		len(resources), outDir)
	return nil
}

// exitFn is the function used to exit. Overridable in tests.
var exitFn = os.Exit

func main() {
	specPath := filepath.Join("spec", "jira-v3.json")
	outDir := filepath.Join("cmd", "generated")
	if err := run(specPath, outDir); err != nil {
		log.Println(err)
		exitFn(1)
	}
}
