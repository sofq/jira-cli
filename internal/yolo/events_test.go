package yolo

import (
	"testing"
)

func TestAllEventTypes(t *testing.T) {
	all := AllEventTypes()
	if len(all) != 11 {
		t.Errorf("AllEventTypes() returned %d events, want 11", len(all))
	}

	expected := []string{
		EventAssigned,
		EventUnassigned,
		EventCommentAdded,
		EventCommentMention,
		EventStatusChange,
		EventPriorityChange,
		EventFieldChange,
		EventIssueCreated,
		EventReviewRequested,
		EventBlocked,
		EventSprintChange,
	}

	got := make(map[string]bool)
	for _, e := range all {
		got[e] = true
	}

	for _, want := range expected {
		if !got[want] {
			t.Errorf("AllEventTypes() missing %q", want)
		}
	}
}

func TestEventTypeConstants(t *testing.T) {
	cases := []struct {
		name string
		val  string
	}{
		{"EventAssigned", EventAssigned},
		{"EventUnassigned", EventUnassigned},
		{"EventCommentAdded", EventCommentAdded},
		{"EventCommentMention", EventCommentMention},
		{"EventStatusChange", EventStatusChange},
		{"EventPriorityChange", EventPriorityChange},
		{"EventFieldChange", EventFieldChange},
		{"EventIssueCreated", EventIssueCreated},
		{"EventReviewRequested", EventReviewRequested},
		{"EventBlocked", EventBlocked},
		{"EventSprintChange", EventSprintChange},
	}

	expected := map[string]string{
		"EventAssigned":       "assigned",
		"EventUnassigned":     "unassigned",
		"EventCommentAdded":   "comment_added",
		"EventCommentMention": "comment_mention",
		"EventStatusChange":   "status_change",
		"EventPriorityChange": "priority_change",
		"EventFieldChange":    "field_change",
		"EventIssueCreated":   "issue_created",
		"EventReviewRequested": "review_requested",
		"EventBlocked":        "blocked",
		"EventSprintChange":   "sprint_change",
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			want := expected[tc.name]
			if tc.val != want {
				t.Errorf("%s = %q, want %q", tc.name, tc.val, want)
			}
		})
	}
}

func TestValidEventType(t *testing.T) {
	cases := []struct {
		event string
		valid bool
	}{
		{"assigned", true},
		{"unassigned", true},
		{"comment_added", true},
		{"comment_mention", true},
		{"status_change", true},
		{"priority_change", true},
		{"field_change", true},
		{"issue_created", true},
		{"review_requested", true},
		{"blocked", true},
		{"sprint_change", true},
		{"", false},
		{"unknown", false},
		{"ASSIGNED", false},
		{"comment-added", false},
	}

	for _, tc := range cases {
		t.Run(tc.event, func(t *testing.T) {
			got := ValidEventType(tc.event)
			if got != tc.valid {
				t.Errorf("ValidEventType(%q) = %v, want %v", tc.event, got, tc.valid)
			}
		})
	}
}

func TestReactionKey(t *testing.T) {
	cases := []struct {
		eventType string
		want      string
	}{
		{"assigned", "on_assigned"},
		{"comment_added", "on_comment_added"},
		{"blocked", "on_blocked"},
		{"sprint_change", "on_sprint_change"},
		{"", "on_"},
	}

	for _, tc := range cases {
		t.Run(tc.eventType, func(t *testing.T) {
			got := ReactionKey(tc.eventType)
			if got != tc.want {
				t.Errorf("ReactionKey(%q) = %q, want %q", tc.eventType, got, tc.want)
			}
		})
	}
}

func TestAllEventTypesNoDuplicates(t *testing.T) {
	all := AllEventTypes()
	seen := make(map[string]int)
	for _, e := range all {
		seen[e]++
	}
	for event, count := range seen {
		if count > 1 {
			t.Errorf("AllEventTypes() contains %q %d times (want 1)", event, count)
		}
	}
}
