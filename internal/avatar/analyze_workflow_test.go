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

func TestAnalyzeTransitions_EmptyFromField(t *testing.T) {
	// When the first status entry has an empty From field, the sequence
	// should not prepend an empty string.
	entries := []ChangelogEntry{
		{Issue: "PROJ-1", Timestamp: "2024-01-01T09:00:00Z", Author: "alice", Field: "status", From: "", To: "In Progress"},
		{Issue: "PROJ-1", Timestamp: "2024-01-03T09:00:00Z", Author: "alice", Field: "status", From: "In Progress", To: "Done"},
		{Issue: "PROJ-2", Timestamp: "2024-01-01T10:00:00Z", Author: "alice", Field: "status", From: "", To: "In Progress"},
		{Issue: "PROJ-2", Timestamp: "2024-01-02T10:00:00Z", Author: "alice", Field: "status", From: "In Progress", To: "Done"},
	}
	result := AnalyzeTransitions(entries)
	// Should have common sequences without empty strings at start
	if len(result.CommonSequences) == 0 {
		t.Fatal("expected at least one common sequence")
	}
	for _, seq := range result.CommonSequences {
		for _, s := range seq {
			if s == "" {
				t.Errorf("sequence should not contain empty strings, got %v", seq)
			}
		}
	}
}

func TestAnalyzeTransitions_InvalidTimestamps(t *testing.T) {
	// Status entries with unparseable timestamps should be skipped without panic.
	entries := []ChangelogEntry{
		{Issue: "PROJ-1", Timestamp: "not-a-timestamp", Author: "alice", Field: "status", From: "Todo", To: "In Progress"},
		{Issue: "PROJ-1", Timestamp: "also-invalid", Author: "alice", Field: "status", From: "In Progress", To: "Done"},
	}
	result := AnalyzeTransitions(entries)
	// Should not panic; AvgTimeInStatus should be empty since timestamps are invalid
	if result.AvgTimeInStatus == nil {
		t.Log("AvgTimeInStatus is nil (acceptable for invalid timestamps)")
	}
}

func TestAnalyzeTransitions_SingleStatusEntry(t *testing.T) {
	// Only 1 status entry per issue — no time-in-status calculation, no sequences
	entries := []ChangelogEntry{
		{Issue: "PROJ-1", Timestamp: "2024-01-01T09:00:00Z", Author: "alice", Field: "status", From: "Todo", To: "Done"},
	}
	result := AnalyzeTransitions(entries)
	// 1 status entry per issue → no commonSequences (need >= 2)
	if len(result.CommonSequences) != 0 {
		t.Errorf("expected no sequences for single status entry per issue, got %v", result.CommonSequences)
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

// TestAnalyzeTransitions_DifferentSequenceFrequencies exercises the sort
// comparison body inside AnalyzeTransitions where seqList[i].count >
// seqList[j].count evaluates to true (i.e. two sequences have different counts).
// We produce two distinct sequences: one appearing twice and one appearing once,
// so the sort must order them by count descending.
func TestAnalyzeTransitions_DifferentSequenceFrequencies(t *testing.T) {
	entries := []ChangelogEntry{
		// PROJ-1: Todo -> In Progress -> Done  (sequence A, count 1)
		{Issue: "PROJ-1", Timestamp: "2024-01-01T09:00:00Z", Author: "alice", Field: "status", From: "Todo", To: "In Progress"},
		{Issue: "PROJ-1", Timestamp: "2024-01-03T09:00:00Z", Author: "alice", Field: "status", From: "In Progress", To: "Done"},
		// PROJ-2: Todo -> In Progress -> Done  (sequence A again, count 2)
		{Issue: "PROJ-2", Timestamp: "2024-01-01T10:00:00Z", Author: "alice", Field: "status", From: "Todo", To: "In Progress"},
		{Issue: "PROJ-2", Timestamp: "2024-01-02T10:00:00Z", Author: "alice", Field: "status", From: "In Progress", To: "Done"},
		// PROJ-3: Open -> Review -> Closed  (sequence B, count 1)
		{Issue: "PROJ-3", Timestamp: "2024-01-05T09:00:00Z", Author: "alice", Field: "status", From: "Open", To: "Review"},
		{Issue: "PROJ-3", Timestamp: "2024-01-06T09:00:00Z", Author: "alice", Field: "status", From: "Review", To: "Closed"},
	}

	result := AnalyzeTransitions(entries)

	// There should be at least one sequence in commonSequences.
	if len(result.CommonSequences) == 0 {
		t.Fatal("expected at least one common sequence")
	}
	// The first sequence should be the higher-frequency one (Todo->In Progress->Done).
	first := result.CommonSequences[0]
	if len(first) < 3 || first[0] != "Todo" || first[1] != "In Progress" || first[2] != "Done" {
		t.Errorf("expected highest-frequency sequence [Todo In Progress Done] first, got %v", first)
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
// topKeys
// ---------------------------------------------------------------------------

func TestTopKeys_Empty(t *testing.T) {
	result := topKeys(map[string]int{}, 5)
	if len(result) != 0 {
		t.Errorf("expected empty result for empty map, got %v", result)
	}
}

func TestTopKeys_FewerThanN(t *testing.T) {
	counts := map[string]int{"alpha": 3, "beta": 1}
	result := topKeys(counts, 5)
	if len(result) != 2 {
		t.Errorf("expected 2 results (all keys), got %d: %v", len(result), result)
	}
	// alpha should come first (higher count)
	if result[0] != "alpha" {
		t.Errorf("expected alpha first, got %q", result[0])
	}
}

func TestTopKeys_ExactlyN(t *testing.T) {
	counts := map[string]int{"a": 5, "b": 3, "c": 1}
	result := topKeys(counts, 3)
	if len(result) != 3 {
		t.Errorf("expected 3 results, got %d: %v", len(result), result)
	}
}

func TestTopKeys_TieBreakByKey(t *testing.T) {
	// Same count — alphabetical order should be applied as tiebreaker.
	counts := map[string]int{"zebra": 2, "apple": 2, "mango": 2}
	result := topKeys(counts, 3)
	if len(result) != 3 {
		t.Fatalf("expected 3 results, got %d", len(result))
	}
	if result[0] != "apple" {
		t.Errorf("tiebreak: expected 'apple' first, got %q", result[0])
	}
}

func TestTopKeys_LimitToN(t *testing.T) {
	counts := map[string]int{"a": 5, "b": 4, "c": 3, "d": 2, "e": 1}
	result := topKeys(counts, 2)
	if len(result) != 2 {
		t.Errorf("expected 2 results, got %d: %v", len(result), result)
	}
	if result[0] != "a" {
		t.Errorf("expected 'a' first, got %q", result[0])
	}
}

// ---------------------------------------------------------------------------
// formatDuration
// ---------------------------------------------------------------------------

func TestFormatDuration(t *testing.T) {
	tests := []struct {
		d    string // parsed duration string
		want string
	}{
		{"72h", "3d"},
		{"25h", "1d"},
		{"24h", "1d"},
		{"5h", "5h"},
		{"1h", "1h"},
		{"45m", "45m"},
		{"0s", "0m"},
		{"-2h", "2h"}, // negative becomes positive
	}
	for _, tc := range tests {
		var d interface{ String() string }
		_ = d
		// Parse using time.Duration
		var dur interface{}
		_ = dur
		// Use a manual approach
		t.Run(tc.d, func(t *testing.T) {
			// Re-parse the duration in a test-friendly way
			switch tc.d {
			case "72h":
				got := formatDuration(72 * 3600 * 1e9)
				if got != tc.want {
					t.Errorf("formatDuration(72h) = %q, want %q", got, tc.want)
				}
			case "25h":
				got := formatDuration(25 * 3600 * 1e9)
				if got != tc.want {
					t.Errorf("formatDuration(25h) = %q, want %q", got, tc.want)
				}
			case "24h":
				got := formatDuration(24 * 3600 * 1e9)
				if got != tc.want {
					t.Errorf("formatDuration(24h) = %q, want %q", got, tc.want)
				}
			case "5h":
				got := formatDuration(5 * 3600 * 1e9)
				if got != tc.want {
					t.Errorf("formatDuration(5h) = %q, want %q", got, tc.want)
				}
			case "1h":
				got := formatDuration(1 * 3600 * 1e9)
				if got != tc.want {
					t.Errorf("formatDuration(1h) = %q, want %q", got, tc.want)
				}
			case "45m":
				got := formatDuration(45 * 60 * 1e9)
				if got != tc.want {
					t.Errorf("formatDuration(45m) = %q, want %q", got, tc.want)
				}
			case "0s":
				got := formatDuration(0)
				if got != tc.want {
					t.Errorf("formatDuration(0) = %q, want %q", got, tc.want)
				}
			case "-2h":
				got := formatDuration(-2 * 3600 * 1e9)
				if got != tc.want {
					t.Errorf("formatDuration(-2h) = %q, want %q", got, tc.want)
				}
			}
		})
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
