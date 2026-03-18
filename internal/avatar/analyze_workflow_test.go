package avatar

import (
	"testing"
)

// ---------------------------------------------------------------------------
// TestAnalyzeTransitions
// ---------------------------------------------------------------------------

func TestAnalyzeTransitions(t *testing.T) {
	entries := []ChangelogEntry{
		// Issue PROJ-1: Todo -> In Progress -> Done
		{Issue: "PROJ-1", Timestamp: "2024-01-01T09:00:00Z", Author: "alice", Field: "status", From: "Todo", To: "In Progress"},
		{Issue: "PROJ-1", Timestamp: "2024-01-03T09:00:00Z", Author: "alice", Field: "status", From: "In Progress", To: "Done"},
		// Issue PROJ-2: Todo -> In Progress -> Done
		{Issue: "PROJ-2", Timestamp: "2024-01-01T10:00:00Z", Author: "alice", Field: "status", From: "Todo", To: "In Progress"},
		{Issue: "PROJ-2", Timestamp: "2024-01-02T10:00:00Z", Author: "alice", Field: "status", From: "In Progress", To: "Done"},
		// Non-status entry
		{Issue: "PROJ-1", Timestamp: "2024-01-01T08:55:00Z", Author: "alice", Field: "summary", From: "", To: "Updated title"},
	}

	result := AnalyzeTransitions(entries)

	// Should have avg time in status for "In Progress"
	if result.AvgTimeInStatus == nil {
		t.Fatal("expected AvgTimeInStatus to be non-nil")
	}
	if _, ok := result.AvgTimeInStatus["In Progress"]; !ok {
		t.Errorf("expected AvgTimeInStatus to contain 'In Progress', got %v", result.AvgTimeInStatus)
	}

	// Both issues have sequence [Todo In Progress Done] — should appear in common sequences
	if len(result.CommonSequences) == 0 {
		t.Fatal("expected at least one common sequence")
	}
	foundSeq := false
	for _, seq := range result.CommonSequences {
		if len(seq) == 3 && seq[0] == "Todo" && seq[1] == "In Progress" && seq[2] == "Done" {
			foundSeq = true
		}
	}
	if !foundSeq {
		t.Errorf("expected sequence [Todo In Progress Done], got %v", result.CommonSequences)
	}
}

func TestAnalyzeTransitions_AssignsBeforeTransition(t *testing.T) {
	entries := []ChangelogEntry{
		{Issue: "PROJ-1", Timestamp: "2024-01-01T09:00:00Z", Author: "alice", Field: "assignee", From: "", To: "alice"},
		{Issue: "PROJ-1", Timestamp: "2024-01-01T09:01:00Z", Author: "alice", Field: "status", From: "Todo", To: "In Progress"},
	}
	result := AnalyzeTransitions(entries)
	if !result.AssignsBeforeTransition {
		t.Error("expected AssignsBeforeTransition to be true when assignee change precedes status change")
	}
}

func TestAnalyzeTransitions_Empty(t *testing.T) {
	result := AnalyzeTransitions(nil)
	if result.AvgTimeInStatus != nil {
		t.Errorf("expected nil AvgTimeInStatus for empty input, got %v", result.AvgTimeInStatus)
	}
	if result.CommonSequences != nil {
		t.Errorf("expected nil CommonSequences for empty input, got %v", result.CommonSequences)
	}
	if result.AssignsBeforeTransition {
		t.Error("expected AssignsBeforeTransition to be false for empty input")
	}
}

// ---------------------------------------------------------------------------
// TestAnalyzeFieldPreferences
// ---------------------------------------------------------------------------

func TestAnalyzeFieldPreferences(t *testing.T) {
	issues := []IssueFields{
		// Issue 1: all fields set
		{Priority: "High", Labels: []string{"bug"}, Components: []string{"auth"}, FixVersion: "v1.0"},
		// Issue 2: priority + labels set, no components or fix_version
		{Priority: "High", Labels: []string{"bug"}, Components: nil, FixVersion: ""},
		// Issue 3: only priority set
		{Priority: "Medium", Labels: nil, Components: nil, FixVersion: ""},
	}

	result := AnalyzeFieldPreferences(issues)

	// Priority is set in all 3 (100%) → always_sets
	if !contains(result.AlwaysSets, "priority") {
		t.Errorf("expected 'priority' in AlwaysSets, got %v", result.AlwaysSets)
	}

	// fix_version set in only 1/3 (33%) → neither
	if contains(result.AlwaysSets, "fix_version") {
		t.Errorf("did not expect 'fix_version' in AlwaysSets")
	}

	// components set in only 1/3 (33%) → neither (not <20%)
	// fix_version set in 1/3 = 33% → neither
	// Let's check rarely_sets: nothing should be there for these ratios
	// (all are >=20% or >80%)

	// DefaultPriority should be "High" (appears 2 times vs Medium 1 time)
	if result.DefaultPriority != "High" {
		t.Errorf("expected DefaultPriority to be 'High', got %q", result.DefaultPriority)
	}

	// CommonLabels should include "bug"
	if !contains(result.CommonLabels, "bug") {
		t.Errorf("expected 'bug' in CommonLabels, got %v", result.CommonLabels)
	}
}

func TestAnalyzeFieldPreferences_RarelySets(t *testing.T) {
	// 6 issues, fix_version set in only 1 → ratio = 1/6 = 16.7% → rarely_sets
	issues := make([]IssueFields, 6)
	for i := range issues {
		issues[i] = IssueFields{Priority: "High"}
	}
	issues[0].FixVersion = "v1.0"

	result := AnalyzeFieldPreferences(issues)
	if !contains(result.RarelySets, "fix_version") {
		t.Errorf("expected 'fix_version' in RarelySets, got %v", result.RarelySets)
	}
}

func TestAnalyzeFieldPreferences_Empty(t *testing.T) {
	result := AnalyzeFieldPreferences(nil)
	if result.AlwaysSets != nil || result.RarelySets != nil {
		t.Errorf("expected nil slices for empty input, got AlwaysSets=%v RarelySets=%v", result.AlwaysSets, result.RarelySets)
	}
	if result.DefaultPriority != "" {
		t.Errorf("expected empty DefaultPriority for empty input, got %q", result.DefaultPriority)
	}
}

// ---------------------------------------------------------------------------
// TestAnalyzeIssueCreation
// ---------------------------------------------------------------------------

func TestAnalyzeIssueCreation(t *testing.T) {
	issues := []CreatedIssue{
		{Key: "PROJ-1", Type: "Bug", SubtaskCount: 0},
		{Key: "PROJ-2", Type: "Bug", SubtaskCount: 0},
		{Key: "PROJ-3", Type: "Story", SubtaskCount: 3},
		{Key: "PROJ-4", Type: "Task", SubtaskCount: 0},
	}

	result := AnalyzeIssueCreation(issues)

	// Bug ratio should be 0.5 (2/4)
	if result.TypesCreated == nil {
		t.Fatal("expected TypesCreated to be non-nil")
	}
	bugRatio, ok := result.TypesCreated["Bug"]
	if !ok {
		t.Fatal("expected 'Bug' in TypesCreated")
	}
	if bugRatio != 0.5 {
		t.Errorf("expected Bug ratio 0.5, got %v", bugRatio)
	}

	// Story ratio should be 0.25
	storyRatio := result.TypesCreated["Story"]
	if storyRatio != 0.25 {
		t.Errorf("expected Story ratio 0.25, got %v", storyRatio)
	}

	// Task ratio should be 0.25
	taskRatio := result.TypesCreated["Task"]
	if taskRatio != 0.25 {
		t.Errorf("expected Task ratio 0.25, got %v", taskRatio)
	}

	// Avg subtasks per story: only 1 story with 3 subtasks → 3.0
	if result.AvgSubtasksPerStory != 3.0 {
		t.Errorf("expected AvgSubtasksPerStory 3.0, got %v", result.AvgSubtasksPerStory)
	}
}

func TestAnalyzeIssueCreation_Empty(t *testing.T) {
	result := AnalyzeIssueCreation(nil)
	if result.TypesCreated != nil {
		t.Errorf("expected nil TypesCreated for empty input, got %v", result.TypesCreated)
	}
	if result.AvgSubtasksPerStory != 0 {
		t.Errorf("expected AvgSubtasksPerStory 0 for empty input, got %v", result.AvgSubtasksPerStory)
	}
}

// ---------------------------------------------------------------------------
// helpers
// ---------------------------------------------------------------------------

func contains(slice []string, s string) bool {
	for _, v := range slice {
		if v == s {
			return true
		}
	}
	return false
}
