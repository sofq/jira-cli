package main

import (
	"testing"
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
