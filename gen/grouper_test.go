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
		resource    string
		want        string
	}{
		{"getIssue", "issue", "get"},
		{"createIssue", "issue", "create"},
		{"getIssueTransitions", "issue", "get-transitions"},
		{"deleteIssue", "issue", "delete"},
		{"getAllProjects", "project", "get-all"},
	}

	for _, tc := range cases {
		t.Run(tc.operationID, func(t *testing.T) {
			got := DeriveVerb(tc.operationID, "GET", "", tc.resource)
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
