package main

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
)

func main() {
	specPath := filepath.Join("spec", "jira-v3.json")
	outDir := filepath.Join("cmd", "generated")

	// 1. Parse the OpenAPI spec.
	fmt.Printf("Parsing spec: %s\n", specPath)
	ops, err := ParseSpec(specPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error parsing spec: %v\n", err)
		os.Exit(1)
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
		fmt.Fprintf(os.Stderr, "error cleaning output dir: %v\n", err)
		os.Exit(1)
	}
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		fmt.Fprintf(os.Stderr, "error creating output dir: %v\n", err)
		os.Exit(1)
	}

	// 5. Generate one file per resource.
	fmt.Println("Generating resource files...")
	for _, resource := range resources {
		if err := GenerateResource(resource, groups[resource], outDir); err != nil {
			fmt.Fprintf(os.Stderr, "error generating resource %q: %v\n", resource, err)
			os.Exit(1)
		}
		fmt.Printf("  %s (%d ops)\n", resource, len(groups[resource]))
	}

	// 6. Generate schema data.
	fmt.Println("Generating schema_data.go...")
	if err := GenerateSchemaData(groups, resources, outDir); err != nil {
		fmt.Fprintf(os.Stderr, "error generating schema data: %v\n", err)
		os.Exit(1)
	}

	// 7. Generate init.go.
	fmt.Println("Generating init.go...")
	if err := GenerateInit(resources, outDir); err != nil {
		fmt.Fprintf(os.Stderr, "error generating init: %v\n", err)
		os.Exit(1)
	}

	// 8. Summary.
	fmt.Printf("\nDone! Generated %d resource files + schema_data.go + init.go in %s\n",
		len(resources), outDir)
}
