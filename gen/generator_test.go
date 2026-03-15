package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestGenerateResource(t *testing.T) {
	ops := []Operation{
		{
			OperationID: "getIssue",
			Method:      "GET",
			Path:        "/rest/api/3/issue/{issueIdOrKey}",
			Summary:     "Get issue",
			Description: "Returns an issue.",
			PathParams: []Param{
				{Name: "issueIdOrKey", In: "path", Required: true, Type: "string", Description: "The issue ID or key"},
			},
			QueryParams: []Param{
				{Name: "fields", In: "query", Required: false, Type: "string", Description: "Fields to include"},
				{Name: "expand", In: "query", Required: false, Type: "string", Description: "Fields to expand"},
			},
			HasBody: false,
		},
		{
			OperationID: "createIssue",
			Method:      "POST",
			Path:        "/rest/api/3/issue",
			Summary:     "Create issue",
			Description: "Creates an issue.",
			PathParams:  nil,
			QueryParams: nil,
			HasBody:     true,
		},
	}

	dir := t.TempDir()
	if err := GenerateResource("issue", ops, dir); err != nil {
		t.Fatalf("GenerateResource: %v", err)
	}

	content, err := os.ReadFile(filepath.Join(dir, "issue.go"))
	if err != nil {
		t.Fatalf("reading generated file: %v", err)
	}

	src := string(content)

	checks := []string{
		"package generated",
		"DO NOT EDIT",
		`Use:   "issue"`,
		`Use:   "get"`,
		`Use:   "create"`,
		`issueIdOrKey`,
		`"fields"`,
		`"expand"`,
		`bodyReader`,
		`"body"`,
		`os.Exit`,
	}
	for _, want := range checks {
		if !strings.Contains(src, want) {
			t.Errorf("generated issue.go missing %q", want)
		}
	}
}

func TestGenerateInit(t *testing.T) {
	dir := t.TempDir()
	resources := []string{"issue", "project"}

	if err := GenerateInit(resources, dir); err != nil {
		t.Fatalf("GenerateInit: %v", err)
	}

	content, err := os.ReadFile(filepath.Join(dir, "init.go"))
	if err != nil {
		t.Fatalf("reading generated file: %v", err)
	}

	src := string(content)

	checks := []string{
		"package generated",
		"DO NOT EDIT",
		"issueCmd",
		"projectCmd",
		"RegisterAll",
	}
	for _, want := range checks {
		if !strings.Contains(src, want) {
			t.Errorf("generated init.go missing %q", want)
		}
	}
}
