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
