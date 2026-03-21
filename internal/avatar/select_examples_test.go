package avatar

import (
	"testing"
)

// TestClassifyComment verifies the keyword-based context classification.
func TestClassifyComment(t *testing.T) {
	cases := []struct {
		text string
		want string
	}{
		{"Fixed the bug. Deployed to staging.", "status_update"},
		{"Blocked by PROJ-100. Waiting on approval.", "blocker_report"},
		{"Looks good. Minor nit on line 42.", "review_feedback"},
		{"What's the expected behavior here?", "question"},
		{"Updated the config file.", "status_update"},
	}
	for _, tc := range cases {
		got := classifyComment(tc.text)
		if got != tc.want {
			t.Errorf("classifyComment(%q) = %q, want %q", tc.text, got, tc.want)
		}
	}
}

// TestClassifyComment_ExtendedCases verifies additional classification paths.
func TestClassifyComment_ExtendedCases(t *testing.T) {
	cases := []struct {
		text string
		want string
	}{
		// review_feedback via lgtm
		{"LGTM, looks ready to merge.", "review_feedback"},
		// review_feedback via nit
		{"nit: rename this variable", "review_feedback"},
		// review_feedback via approve
		{"Approved this PR", "review_feedback"},
		// review_feedback via review keyword
		{"I reviewed this and it seems fine", "review_feedback"},
		// blocker_report via waiting for
		{"Waiting for QA sign-off.", "blocker_report"},
		// blocker_report via need input
		{"Need input from backend team.", "blocker_report"},
		// question: ends with ?
		{"Is this the correct approach?", "question"},
		// question: high density — does NOT end with "?", has 2 questions across 3 sentences → 2/3=0.67 > 0.33
		{"Why? What? Okay.", "question"},
		// question: low density — 1 question in 4 sentences → 1/4=0.25, below 0.33 threshold → status_update
		{"Done. Is this right? Confirmed. Merged.", "status_update"},
		// status_update: no keywords
		{"Deployed to production.", "status_update"},
	}
	for _, tc := range cases {
		got := classifyComment(tc.text)
		if got != tc.want {
			t.Errorf("classifyComment(%q) = %q, want %q", tc.text, got, tc.want)
		}
	}
}

// TestMedianWordCount verifies the unexported medianWordCount helper.
func TestMedianWordCount(t *testing.T) {
	tests := []struct {
		texts []string
		want  int
	}{
		{nil, 0},
		{[]string{}, 0},
		{[]string{"hello world"}, 2},
		{[]string{"one", "two three", "four five six"}, 2}, // sorted [1,2,3] → median=2
		{[]string{"one two", "three four"}, 2},             // even: (2+2)/2 = 2
		{[]string{"a", "b c", "d e f", "g h i j"}, 2},     // sorted [1,2,3,4], even: (2+3)/2=2
	}
	for _, tc := range tests {
		got := medianWordCount(tc.texts)
		if got != tc.want {
			t.Errorf("medianWordCount(%v) = %d, want %d", tc.texts, got, tc.want)
		}
	}
}

// TestSelectCommentExamples_MaxLessThanGroups verifies that maxExamples < number of groups is respected.
func TestSelectCommentExamples_MaxLessThanGroups(t *testing.T) {
	comments := []RawComment{
		{Issue: "PROJ-1", Text: "Fixed the bug."},
		{Issue: "PROJ-2", Text: "Blocked by infra."},
		{Issue: "PROJ-3", Text: "Looks good, LGTM."},
		{Issue: "PROJ-4", Text: "What is the deadline?"},
	}
	got := SelectCommentExamples(comments, 1)
	if len(got) > 1 {
		t.Errorf("expected at most 1 example, got %d", len(got))
	}
}

// TestSelectDescriptionExamples_MaxExamplesExceeded verifies the maxExamples cap.
func TestSelectDescriptionExamples_MaxExamplesExceeded(t *testing.T) {
	// 4 different types but maxExamples=2 — should return exactly 2
	issues := []CreatedIssue{
		{Key: "PROJ-1", Type: "Bug", Description: "Bug description here."},
		{Key: "PROJ-2", Type: "Story", Description: "Story description here."},
		{Key: "PROJ-3", Type: "Task", Description: "Task description here."},
		{Key: "PROJ-4", Type: "Epic", Description: "Epic description here."},
	}
	got := SelectDescriptionExamples(issues, 2)
	if len(got) > 2 {
		t.Errorf("expected at most 2 examples with maxExamples=2, got %d", len(got))
	}
}

// TestSelectDescriptionExamples_BestRepresentative verifies that the description closest to median is selected.
func TestSelectDescriptionExamples_BestRepresentative(t *testing.T) {
	// 3 Bug issues with different word counts; median=5 words; issue with 5 words should win
	issues := []CreatedIssue{
		{Key: "PROJ-1", Type: "Bug", Description: "one two three four five"},                // 5 words (exactly median)
		{Key: "PROJ-2", Type: "Bug", Description: "one word"},                               // 2 words
		{Key: "PROJ-3", Type: "Bug", Description: "one two three four five six seven eight"}, // 8 words
	}
	got := SelectDescriptionExamples(issues, 3)
	if len(got) != 1 {
		t.Fatalf("expected 1 result for single type, got %d", len(got))
	}
	if got[0].Issue != "PROJ-1" {
		t.Errorf("expected PROJ-1 (closest to median 5 words), got %q", got[0].Issue)
	}
}

// TestSelectDescriptionExamples_SingleType verifies single-type group selection.
func TestSelectDescriptionExamples_SingleType(t *testing.T) {
	issues := []CreatedIssue{
		{Key: "PROJ-1", Type: "Bug", Description: "Short bug description."},
		{Key: "PROJ-2", Type: "Bug", Description: "Another bug with more detail in the description here."},
		{Key: "PROJ-3", Type: "Bug", Description: "Medium length bug description here."},
	}
	got := SelectDescriptionExamples(issues, 3)
	if len(got) != 1 {
		t.Errorf("expected 1 result for single type (1 per type), got %d", len(got))
	}
	if got[0].Type != "Bug" {
		t.Errorf("expected Type=Bug, got %q", got[0].Type)
	}
}

// TestSelectDescriptionExamples_Empty verifies empty input.
func TestSelectDescriptionExamples_Empty(t *testing.T) {
	got := SelectDescriptionExamples(nil, 5)
	if len(got) != 0 {
		t.Errorf("expected empty result for nil input, got %d items", len(got))
	}

	got2 := SelectDescriptionExamples([]CreatedIssue{}, 5)
	if len(got2) != 0 {
		t.Errorf("expected empty result for empty input, got %d items", len(got2))
	}
}

// TestSelectCommentExamples feeds 5 diverse comments and asserts len <= maxExamples
// and multiple contexts represented.
func TestSelectCommentExamples(t *testing.T) {
	comments := []RawComment{
		{Issue: "PROJ-1", Date: "2025-01-01", Text: "Fixed the bug. Deployed to staging.", Author: "alice"},
		{Issue: "PROJ-2", Date: "2025-01-02", Text: "Blocked by PROJ-100. Waiting on approval.", Author: "alice"},
		{Issue: "PROJ-3", Date: "2025-01-03", Text: "Looks good. Minor nit on line 42.", Author: "alice"},
		{Issue: "PROJ-4", Date: "2025-01-04", Text: "What's the expected behavior here?", Author: "alice"},
		{Issue: "PROJ-5", Date: "2025-01-05", Text: "Updated the config file.", Author: "alice"},
	}

	maxExamples := 4
	got := SelectCommentExamples(comments, maxExamples)

	if len(got) > maxExamples {
		t.Errorf("len(got) = %d, want <= %d", len(got), maxExamples)
	}

	// Expect multiple contexts
	contexts := map[string]bool{}
	for _, ex := range got {
		contexts[ex.Context] = true
	}
	if len(contexts) < 2 {
		t.Errorf("expected at least 2 distinct contexts, got %d: %v", len(contexts), contexts)
	}

	// Ensure fields are populated
	for i, ex := range got {
		if ex.Issue == "" {
			t.Errorf("example[%d].Issue is empty", i)
		}
		if ex.Text == "" {
			t.Errorf("example[%d].Text is empty", i)
		}
		if ex.Context == "" {
			t.Errorf("example[%d].Context is empty", i)
		}
	}
}

// TestSelectCommentExamples_Empty verifies that nil input returns nil/empty.
func TestSelectCommentExamples_Empty(t *testing.T) {
	got := SelectCommentExamples(nil, 5)
	if len(got) != 0 {
		t.Errorf("expected empty result for nil input, got %d items", len(got))
	}

	got2 := SelectCommentExamples([]RawComment{}, 5)
	if len(got2) != 0 {
		t.Errorf("expected empty result for empty input, got %d items", len(got2))
	}
}

// TestSelectDescriptionExamples feeds 3 CreatedIssues with different types and
// asserts diversity of types in the output.
func TestSelectDescriptionExamples(t *testing.T) {
	issues := []CreatedIssue{
		{Key: "PROJ-1", Type: "Bug", SubtaskCount: 0, Description: "Steps to reproduce: click login button and observe error message on screen."},
		{Key: "PROJ-2", Type: "Story", SubtaskCount: 2, Description: "As a user I want to reset my password so that I can regain access to my account."},
		{Key: "PROJ-3", Type: "Task", SubtaskCount: 0, Description: "Update the deployment pipeline to use the new Docker image version."},
	}

	maxExamples := 3
	got := SelectDescriptionExamples(issues, maxExamples)

	if len(got) > maxExamples {
		t.Errorf("len(got) = %d, want <= %d", len(got), maxExamples)
	}

	// Expect multiple types (diversity)
	types := map[string]bool{}
	for _, ex := range got {
		types[ex.Type] = true
	}
	if len(types) < 2 {
		t.Errorf("expected at least 2 distinct types, got %d: %v", len(types), types)
	}

	// Ensure fields are populated
	for i, ex := range got {
		if ex.Issue == "" {
			t.Errorf("example[%d].Issue is empty", i)
		}
		if ex.Text == "" {
			t.Errorf("example[%d].Text is empty", i)
		}
		if ex.Type == "" {
			t.Errorf("example[%d].Type is empty", i)
		}
	}
}

// TestClassifyComment_DecisionPath covers the "decision" classification.
func TestClassifyComment_DecisionPath(t *testing.T) {
	cases := []struct {
		text string
		want string
	}{
		{"We decided to use Postgres.", "decision"},
		{"Going with option A.", "decision"},
		{"Let's go with the new design.", "decision"},
		{"We'll use Redis for caching.", "decision"},
		{"The plan is to migrate next week.", "decision"},
	}
	for _, tc := range cases {
		got := classifyComment(tc.text)
		if got != tc.want {
			t.Errorf("classifyComment(%q) = %q, want %q", tc.text, got, tc.want)
		}
	}
}

// TestClassifyComment_RequestPath covers the "request" classification.
func TestClassifyComment_RequestPath(t *testing.T) {
	cases := []struct {
		text string
		want string
	}{
		{"Can you update the config?", "request"},
		{"Could you check the logs?", "request"},
		{"Please deploy to staging.", "request"},
		{"Would you review this PR?", "request"},
	}
	for _, tc := range cases {
		got := classifyComment(tc.text)
		if got != tc.want {
			t.Errorf("classifyComment(%q) = %q, want %q", tc.text, got, tc.want)
		}
	}
}

// TestClassifyComment_PushbackPath covers the "pushback" classification.
func TestClassifyComment_PushbackPath(t *testing.T) {
	cases := []struct {
		text string
		want string
	}{
		{"I disagree with this approach.", "pushback"},
		{"I have a concern about the timeline.", "pushback"},
		{"Not sure about this implementation.", "pushback"},
		{"I don't think this will work.", "pushback"},
		{"We could use Redis instead.", "pushback"},
		{"Alternatively, we could try Kafka.", "pushback"},
	}
	for _, tc := range cases {
		got := classifyComment(tc.text)
		if got != tc.want {
			t.Errorf("classifyComment(%q) = %q, want %q", tc.text, got, tc.want)
		}
	}
}

// TestClassifyComment_AcknowledgmentPath covers the "acknowledgment" classification.
func TestClassifyComment_AcknowledgmentPath(t *testing.T) {
	cases := []struct {
		text string
		want string
	}{
		{"Thanks for the update.", "acknowledgment"},
		{"Got it, will proceed.", "acknowledgment"},
		{"Noted.", "acknowledgment"},
		{"Sounds good to me.", "acknowledgment"},
		{"Will do.", "acknowledgment"},
		{"On it.", "acknowledgment"},
	}
	for _, tc := range cases {
		got := classifyComment(tc.text)
		if got != tc.want {
			t.Errorf("classifyComment(%q) = %q, want %q", tc.text, got, tc.want)
		}
	}
}

// TestClassifyComment_TechnicalPath covers the "technical" classification.
func TestClassifyComment_TechnicalPath(t *testing.T) {
	cases := []struct {
		text string
		want string
	}{
		{"Found the root cause of the crash.", "technical"},
		{"The stack trace shows a null pointer.", "technical"},
		{"We need to refactor the auth module.", "technical"},
		{"The API endpoint returns 404.", "technical"},
		{"Database migration failed on staging.", "technical"},
	}
	for _, tc := range cases {
		got := classifyComment(tc.text)
		if got != tc.want {
			t.Errorf("classifyComment(%q) = %q, want %q", tc.text, got, tc.want)
		}
	}
}
