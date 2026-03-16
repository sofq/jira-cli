package main

import (
	"testing"
)

func TestGroupOperations(t *testing.T) {
	ops := []Operation{
		{OperationID: "getIssue", Method: "GET", Path: "/rest/api/3/issue/{issueIdOrKey}"},
		{OperationID: "createIssue", Method: "POST", Path: "/rest/api/3/issue"},
		{OperationID: "deleteIssue", Method: "DELETE", Path: "/rest/api/3/issue/{issueIdOrKey}"},
		{OperationID: "getAllProjects", Method: "GET", Path: "/rest/api/3/project"},
	}

	groups := GroupOperations(ops)

	if len(groups["issue"]) != 3 {
		t.Errorf("expected 3 issue ops, got %d", len(groups["issue"]))
	}
	if len(groups["project"]) != 1 {
		t.Errorf("expected 1 project op, got %d", len(groups["project"]))
	}
}

func TestDeriveVerb(t *testing.T) {
	cases := []struct {
		operationID string
		method      string
		resource    string
		want        string
	}{
		// Case 1: rest == resource (singular match)
		{"getIssue", "GET", "issue", "get"},
		{"createIssue", "POST", "issue", "create"},
		{"deleteIssue", "DELETE", "issue", "delete"},
		// Case 1: rest == resource (plural match via restLower == resourceLower)
		{"getProjects", "GET", "projects", "get"},
		// Case 2: rest ENDS with resource → strip it, keep prefix
		{"getAllProjects", "GET", "project", "get-all"},
		// Case 3: rest STARTS with resource → strip it, keep suffix
		{"getIssueTransitions", "GET", "issue", "get-transitions"},
		// Fallback: no resource match → verb + kebab rest
		{"getRemoteData", "GET", "issue", "get-remote-data"},
		// Empty operationID → fallback to method
		{"", "POST", "issue", "post"},
		// Single word operationID → just verb
		{"list", "GET", "issue", "list"},
		// "ies" plural: resource=category, rest=["Categories"] → singular match
		{"getCategories", "GET", "category", "get"},
		// Case 2: rest ends with resource, prefix is empty after strip → just verb
		{"getIssues", "GET", "issue", "get"},
		// Case 3: rest starts with resource (exact lowercase), suffix empty → just verb
		{"getIssue", "GET", "issues", "get"},
		// "Categoriess" ends in "ss" so singularize does not strip it; no resource match → fallback
		{"getCategoriess", "GET", "category", "get-categoriess"},
		// Case 3: double-singularize hits (rest starts with resource after double singularize)
		{"getCategoriess", "GET", "categoriess", "get"},
	}

	for _, tc := range cases {
		t.Run(tc.operationID+"_"+tc.resource, func(t *testing.T) {
			got := DeriveVerb(tc.operationID, tc.method, "", tc.resource)
			if got != tc.want {
				t.Errorf("DeriveVerb(%q, resource=%q): got %q, want %q", tc.operationID, tc.resource, got, tc.want)
			}
		})
	}
}

func TestExtractResource(t *testing.T) {
	cases := []struct {
		path string
		want string
	}{
		{"/rest/api/3/issue/{issueIdOrKey}", "issue"},
		{"/rest/api/3/project", "project"},
		{"/rest/atlassian-connect/1/addons/{addonKey}", "atlassian-connect"},
		{"/rest/api/3/issue/{issueIdOrKey}/transitions", "issue"},
		// Fallback: first non-empty segment
		{"/foo/bar", "foo"},
		// Edge: empty path returns path itself
		{"", ""},
	}

	for _, tc := range cases {
		t.Run(tc.path, func(t *testing.T) {
			got := ExtractResource(tc.path)
			if got != tc.want {
				t.Errorf("ExtractResource(%q): got %q, want %q", tc.path, got, tc.want)
			}
		})
	}
}

func TestSplitCamelCase(t *testing.T) {
	cases := []struct {
		input string
		want  []string
	}{
		{"", nil},
		{"get", []string{"get"}},
		{"getIssue", []string{"get", "Issue"}},
		{"HTMLParser", []string{"HTML", "Parser"}},
		{"getV2Issue", []string{"get", "V2", "Issue"}},
		{"ABC", []string{"ABC"}},
	}

	for _, tc := range cases {
		t.Run(tc.input, func(t *testing.T) {
			got := splitCamelCase(tc.input)
			if len(got) != len(tc.want) {
				t.Fatalf("splitCamelCase(%q): got %v, want %v", tc.input, got, tc.want)
			}
			for i := range got {
				if got[i] != tc.want[i] {
					t.Errorf("splitCamelCase(%q)[%d]: got %q, want %q", tc.input, i, got[i], tc.want[i])
				}
			}
		})
	}
}

func TestSingularizeEdgeCases(t *testing.T) {
	tests := []struct {
		input, want string
	}{
		{"status", "status"},
		{"statuses", "status"},
		{"class", "class"},
		{"classes", "class"},
		{"address", "address"},
		{"addresses", "address"},
		{"bus", "bus"},
		{"buses", "bus"},
		{"issues", "issue"},
		{"categories", "category"},
		{"values", "value"},
		{"project", "project"},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			if got := singularize(tt.input); got != tt.want {
				t.Errorf("singularize(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestSingularize(t *testing.T) {
	cases := []struct {
		input string
		want  string
	}{
		{"issues", "issue"},
		{"categories", "category"},
		{"s", "s"},
		{"issue", "issue"},
		{"", ""},
	}

	for _, tc := range cases {
		t.Run(tc.input, func(t *testing.T) {
			got := singularize(tc.input)
			if got != tc.want {
				t.Errorf("singularize(%q): got %q, want %q", tc.input, got, tc.want)
			}
		})
	}
}
