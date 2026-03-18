package avatar

import (
	"testing"
)

// TestAnalyzeResponsePatterns checks reply ratios for 3 comment records:
// 2 on own issues, 1 on another's.
func TestAnalyzeResponsePatterns(t *testing.T) {
	userID := "alice"
	comments := []CommentRecord{
		{
			IssueOwner:      "alice",
			Author:          "alice",
			Timestamp:       "2024-01-10T10:30:00Z",
			ParentTimestamp: "2024-01-10T08:00:00Z",
			Text:            "Working on it",
		},
		{
			IssueOwner:      "alice",
			Author:          "alice",
			Timestamp:       "2024-01-11T09:00:00Z",
			ParentTimestamp: "2024-01-11T07:00:00Z",
			Text:            "Done",
		},
		{
			IssueOwner:      "bob",
			Author:          "alice",
			Timestamp:       "2024-01-12T14:00:00Z",
			ParentTimestamp: "2024-01-12T12:00:00Z",
			Text:            "LGTM",
		},
	}

	got := AnalyzeResponsePatterns(comments, userID)

	// 2 out of 3 replies are to own issues → ~66.67%
	if got.RepliesToOwnIssuesPct <= 0 {
		t.Errorf("RepliesToOwnIssuesPct = %f, want > 0", got.RepliesToOwnIssuesPct)
	}
	// 1 out of 3 replies are to others' issues → ~33.33%
	if got.RepliesToOthersPct <= 0 {
		t.Errorf("RepliesToOthersPct = %f, want > 0", got.RepliesToOthersPct)
	}
	// sum should be 100
	total := got.RepliesToOwnIssuesPct + got.RepliesToOthersPct
	if total < 99.9 || total > 100.1 {
		t.Errorf("pct total = %f, want ~100", total)
	}
	// median reply time should be non-empty
	if got.MedianReplyTime == "" {
		t.Error("MedianReplyTime should be non-empty")
	}
}

// TestAnalyzeMentionHabits checks that @mention extraction and frequency work.
func TestAnalyzeMentionHabits(t *testing.T) {
	comments := []string{
		"Hey @charlie, can you review this PR?",
		"@charlie looks good, please merge.",
		"Blocked by @dave, need input from them.",
		"@charlie, waiting on your approval.",
	}

	got := AnalyzeMentionHabits(comments)

	if len(got.FrequentlyMentions) == 0 {
		t.Fatal("FrequentlyMentions should not be empty")
	}
	// charlie is mentioned 3 times, should be top
	found := false
	for _, u := range got.FrequentlyMentions {
		if u == "charlie" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected 'charlie' in FrequentlyMentions, got %v", got.FrequentlyMentions)
	}
	if len(got.MentionContext) == 0 {
		t.Error("MentionContext should not be empty")
	}
}

// TestAnalyzeCollaboration checks active hours and peak days from 4 timestamps.
func TestAnalyzeCollaboration(t *testing.T) {
	// Tue=2024-01-09, Wed=2024-01-10, Thu=2024-01-11
	timestamps := []string{
		"2024-01-09T09:00:00Z", // Tuesday
		"2024-01-10T10:00:00Z", // Wednesday
		"2024-01-10T11:00:00Z", // Wednesday
		"2024-01-11T09:30:00Z", // Thursday
	}

	got := AnalyzeCollaboration(timestamps)

	if got.ActiveHours.Start == "" {
		t.Error("ActiveHours.Start should be non-empty")
	}
	if got.ActiveHours.End == "" {
		t.Error("ActiveHours.End should be non-empty")
	}
	if len(got.PeakActivityDays) == 0 {
		t.Error("PeakActivityDays should not be empty")
	}
	// Wednesday appears twice, should be a peak day
	found := false
	for _, d := range got.PeakActivityDays {
		if d == "Wednesday" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected 'Wednesday' in PeakActivityDays, got %v", got.PeakActivityDays)
	}
}

// TestAnalyzeEscalation checks blocker keyword detection in comments.
func TestAnalyzeEscalation(t *testing.T) {
	userID := "alice"
	comments := []CommentRecord{
		{
			IssueOwner: "alice",
			Author:     "alice",
			Timestamp:  "2024-01-09T09:00:00Z",
			Text:       "This is blocked by the infra team.",
		},
		{
			IssueOwner: "alice",
			Author:     "alice",
			Timestamp:  "2024-01-09T11:00:00Z",
			Text:       "Waiting on deployment approval.",
		},
		{
			IssueOwner: "alice",
			Author:     "alice",
			Timestamp:  "2024-01-09T13:00:00Z",
			Text:       "Need input from security team.",
		},
		{
			IssueOwner: "alice",
			Author:     "alice",
			Timestamp:  "2024-01-09T15:00:00Z",
			Text:       "Regular update, all good.",
		},
	}
	changelog := []ChangelogEntry{
		{
			Issue:     "PROJ-1",
			Timestamp: "2024-01-09T16:00:00Z",
			Author:    "alice",
			Field:     "status",
			From:      "In Progress",
			To:        "Blocked",
		},
	}

	got := AnalyzeEscalation(comments, changelog, userID)

	if len(got.BlockerKeywords) == 0 {
		t.Error("BlockerKeywords should not be empty")
	}
	// should detect at least one of the known blocker keywords
	foundBlocker := false
	for _, kw := range got.BlockerKeywords {
		if kw == "blocked" || kw == "waiting on" || kw == "need input from" {
			foundBlocker = true
			break
		}
	}
	if !foundBlocker {
		t.Errorf("expected blocker keyword in %v", got.BlockerKeywords)
	}
	if got.EscalationPattern == "" {
		t.Error("EscalationPattern should not be empty")
	}
}

// TestAnalyzeWorklogs checks avg entries per week calculation.
func TestAnalyzeWorklogs(t *testing.T) {
	// 5 entries in week 1, 3 entries in week 2 → avg ~4.0
	entries := []WorklogEntry{
		{Date: "2024-01-08", DurationSeconds: 3600}, // Mon week1
		{Date: "2024-01-09", DurationSeconds: 3600}, // Tue week1
		{Date: "2024-01-10", DurationSeconds: 3600}, // Wed week1
		{Date: "2024-01-11", DurationSeconds: 3600}, // Thu week1
		{Date: "2024-01-12", DurationSeconds: 3600}, // Fri week1
		{Date: "2024-01-15", DurationSeconds: 3600}, // Mon week2
		{Date: "2024-01-16", DurationSeconds: 3600}, // Tue week2
		{Date: "2024-01-17", DurationSeconds: 3600}, // Wed week2
	}

	got := AnalyzeWorklogs(entries)

	if got.AvgEntriesPerWeek <= 0 {
		t.Errorf("AvgEntriesPerWeek = %f, want > 0", got.AvgEntriesPerWeek)
	}
	// 5 days in week 1 >= 4, logs_daily should be true
	if !got.LogsDaily {
		t.Error("LogsDaily should be true (>= 4 days in first week)")
	}
}
