package avatar

import (
	"testing"
	"time"
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

	// 2 out of 3 replies are to own issues → ~0.667
	if got.RepliesToOwnIssuesPct <= 0 {
		t.Errorf("RepliesToOwnIssuesPct = %f, want > 0", got.RepliesToOwnIssuesPct)
	}
	// 1 out of 3 replies are to others' issues → ~0.333
	if got.RepliesToOthersPct <= 0 {
		t.Errorf("RepliesToOthersPct = %f, want > 0", got.RepliesToOthersPct)
	}
	// sum should be 1.0 (fractions)
	total := got.RepliesToOwnIssuesPct + got.RepliesToOthersPct
	if total < 0.999 || total > 1.001 {
		t.Errorf("pct total = %f, want ~1.0", total)
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

// ---------------------------------------------------------------------------
// TestMedianDuration
// ---------------------------------------------------------------------------

func TestMedianDuration_Empty(t *testing.T) {
	result := medianDuration(nil)
	if result != 0 {
		t.Errorf("medianDuration(nil) = %v, want 0", result)
	}

	result2 := medianDuration([]time.Duration{})
	if result2 != 0 {
		t.Errorf("medianDuration([]) = %v, want 0", result2)
	}
}

func TestMedianDuration_Single(t *testing.T) {
	d := 5 * time.Hour
	result := medianDuration([]time.Duration{d})
	if result != d {
		t.Errorf("medianDuration([5h]) = %v, want %v", result, d)
	}
}

func TestMedianDuration_Even(t *testing.T) {
	// Even count: average of middle two values
	// [1h, 2h, 3h, 4h] → median = (2h + 3h) / 2 = 2.5h
	durations := []time.Duration{4 * time.Hour, 1 * time.Hour, 3 * time.Hour, 2 * time.Hour}
	result := medianDuration(durations)
	expected := (2*time.Hour + 3*time.Hour) / 2
	if result != expected {
		t.Errorf("medianDuration([1h,2h,3h,4h]) = %v, want %v", result, expected)
	}
}

func TestMedianDuration_Odd(t *testing.T) {
	// Odd count: middle value
	// [1h, 2h, 3h] → median = 2h
	durations := []time.Duration{3 * time.Hour, 1 * time.Hour, 2 * time.Hour}
	result := medianDuration(durations)
	if result != 2*time.Hour {
		t.Errorf("medianDuration([1h,2h,3h]) = %v, want 2h", result)
	}
}

// ---------------------------------------------------------------------------
// TestAnalyzeResponsePatterns — edge cases
// ---------------------------------------------------------------------------

func TestAnalyzeResponsePatterns_Empty(t *testing.T) {
	result := AnalyzeResponsePatterns(nil, "alice")
	if result.MedianReplyTime != "" {
		t.Errorf("expected empty MedianReplyTime for nil input, got %q", result.MedianReplyTime)
	}
	if result.RepliesToOwnIssuesPct != 0 {
		t.Errorf("expected 0 RepliesToOwnIssuesPct for nil input, got %f", result.RepliesToOwnIssuesPct)
	}
}

func TestAnalyzeResponsePatterns_NoParentTimestamp(t *testing.T) {
	// Comments with no parent timestamp should not contribute to replyDurations.
	comments := []CommentRecord{
		{IssueOwner: "alice", Author: "alice", Timestamp: "2024-01-10T10:00:00Z", ParentTimestamp: "", Text: "no parent"},
	}
	result := AnalyzeResponsePatterns(comments, "alice")
	// MedianReplyTime should be "0m" (no durations collected)
	if result.MedianReplyTime == "" {
		t.Error("MedianReplyTime should not be empty even with zero duration")
	}
}

func TestAnalyzeResponsePatterns_TimestampBeforeParent(t *testing.T) {
	// Comment timestamp before parent should not contribute a reply duration.
	comments := []CommentRecord{
		{
			IssueOwner:      "alice",
			Author:          "alice",
			Timestamp:       "2024-01-10T08:00:00Z",
			ParentTimestamp: "2024-01-10T10:00:00Z", // parent is AFTER comment — invalid
			Text:            "bad order",
		},
	}
	result := AnalyzeResponsePatterns(comments, "alice")
	// Should still return something without panicking
	if result.RepliesToOwnIssuesPct == 0 && result.RepliesToOthersPct == 0 {
		// Only one comment on own issue
		if result.RepliesToOwnIssuesPct != 1.0 {
			t.Errorf("expected RepliesToOwnIssuesPct=1.0 for single own comment, got %f", result.RepliesToOwnIssuesPct)
		}
	}
}

// ---------------------------------------------------------------------------
// TestAnalyzeMentionHabits — edge cases
// ---------------------------------------------------------------------------

func TestAnalyzeMentionHabits_Empty(t *testing.T) {
	result := AnalyzeMentionHabits(nil)
	if len(result.FrequentlyMentions) != 0 {
		t.Errorf("expected empty FrequentlyMentions for nil input, got %v", result.FrequentlyMentions)
	}
}

func TestAnalyzeMentionHabits_NoMentions(t *testing.T) {
	comments := []string{"no mentions here", "just plain text"}
	result := AnalyzeMentionHabits(comments)
	if len(result.FrequentlyMentions) != 0 {
		t.Errorf("expected empty FrequentlyMentions, got %v", result.FrequentlyMentions)
	}
	if len(result.MentionContext) != 0 {
		t.Errorf("expected empty MentionContext for no-keyword text, got %v", result.MentionContext)
	}
}

func TestAnalyzeMentionHabits_DuplicateMentionsInComment(t *testing.T) {
	// Same user @alice mentioned twice in same comment — should count once per comment.
	comments := []string{
		"@alice please review and @alice approve this.",
		"@alice LGTM",
		"@alice can you check this?",
	}
	result := AnalyzeMentionHabits(comments)
	// alice should be present
	found := false
	for _, u := range result.FrequentlyMentions {
		if u == "alice" {
			found = true
		}
	}
	if !found {
		t.Errorf("expected 'alice' in FrequentlyMentions, got %v", result.FrequentlyMentions)
	}
	// alice count should be 3 (once per comment, not 4 for the doubled mention in first comment)
}

func TestAnalyzeMentionHabits_ManyUsers(t *testing.T) {
	// 6 unique mentions — tests the topN = 5 path (len(sorted) >= topN)
	comments := []string{
		"@alice and @bob reviewed.",
		"@charlie approved it.",
		"@dave and @eve are on standby.",
		"@frank needs to be notified.",
		"@alice is assigned now.",
	}
	result := AnalyzeMentionHabits(comments)
	// Should cap at 5 even though there are 6 unique mentions
	if len(result.FrequentlyMentions) > 5 {
		t.Errorf("expected at most 5 mentions, got %d: %v", len(result.FrequentlyMentions), result.FrequentlyMentions)
	}
	if len(result.FrequentlyMentions) == 0 {
		t.Error("expected non-empty FrequentlyMentions")
	}
}

func TestAnalyzeMentionHabits_BlockerContext(t *testing.T) {
	comments := []string{"blocked by @alice", "waiting on @bob"}
	result := AnalyzeMentionHabits(comments)
	found := false
	for _, ctx := range result.MentionContext {
		if ctx == "blockers" {
			found = true
		}
	}
	if !found {
		t.Errorf("expected 'blockers' context, got %v", result.MentionContext)
	}
}

func TestAnalyzeMentionHabits_BothContexts(t *testing.T) {
	comments := []string{
		"review this pr @alice",
		"blocked by @bob, need input",
	}
	result := AnalyzeMentionHabits(comments)
	found := false
	for _, ctx := range result.MentionContext {
		if ctx == "blockers_and_reviews" {
			found = true
		}
	}
	if !found {
		t.Errorf("expected 'blockers_and_reviews' context, got %v", result.MentionContext)
	}
}

// ---------------------------------------------------------------------------
// TestAnalyzeEscalation — no escalation detected
// ---------------------------------------------------------------------------

func TestAnalyzeEscalation_NoEscalation(t *testing.T) {
	comments := []CommentRecord{
		{IssueOwner: "alice", Author: "alice", Timestamp: "2024-01-09T09:00:00Z", Text: "All good!"},
	}
	result := AnalyzeEscalation(comments, nil, "alice")
	if result.EscalationPattern != "no_escalation_detected" {
		t.Errorf("expected 'no_escalation_detected', got %q", result.EscalationPattern)
	}
	if result.AvgCommentsBeforeEscalation != 0 {
		t.Errorf("expected 0 AvgCommentsBeforeEscalation, got %f", result.AvgCommentsBeforeEscalation)
	}
}

func TestAnalyzeEscalation_AssigneeChange(t *testing.T) {
	comments := []CommentRecord{
		{IssueOwner: "alice", Author: "alice", Timestamp: "2024-01-09T09:00:00Z", Text: "blocked by infrastructure"},
	}
	changelog := []ChangelogEntry{
		{Issue: "PROJ-1", Timestamp: "2024-01-09T10:00:00Z", Author: "alice", Field: "assignee", From: "alice", To: "bob"},
	}
	result := AnalyzeEscalation(comments, changelog, "alice")
	if result.AvgCommentsBeforeEscalation == 0 {
		t.Error("expected non-zero AvgCommentsBeforeEscalation when escalationCount > 0")
	}
}

// ---------------------------------------------------------------------------
// TestAnalyzeCollaboration — edge cases
// ---------------------------------------------------------------------------

func TestAnalyzeCollaboration_Empty(t *testing.T) {
	result := AnalyzeCollaboration(nil)
	if result.ActiveHours.Start != "" {
		t.Errorf("expected empty ActiveHours.Start for nil input, got %q", result.ActiveHours.Start)
	}
}

func TestAnalyzeCollaboration_InvalidTimestamps(t *testing.T) {
	result := AnalyzeCollaboration([]string{"not-a-timestamp", "also-invalid"})
	if result.ActiveHours.Start != "" {
		t.Errorf("expected empty result for all-invalid timestamps, got %+v", result.ActiveHours)
	}
}

func TestAnalyzeCollaboration_NonUTCTimezone(t *testing.T) {
	// Timestamps with numeric offsets (+05:30) — location.String() returns "" → tz fallback to "UTC"
	timestamps := []string{
		"2024-01-09T09:00:00+05:30",
		"2024-01-10T10:00:00+05:30",
	}
	result := AnalyzeCollaboration(timestamps)
	if result.ActiveHours.Timezone != "UTC" {
		t.Errorf("expected Timezone=UTC for numeric offset, got %q", result.ActiveHours.Timezone)
	}
}

func TestAnalyzeCollaboration_FewDays(t *testing.T) {
	// Only 1 unique day — topDays capped at len(days)
	timestamps := []string{
		"2024-01-09T09:00:00Z", // Tuesday
		"2024-01-09T10:00:00Z", // Tuesday
	}
	result := AnalyzeCollaboration(timestamps)
	if len(result.PeakActivityDays) != 1 {
		t.Errorf("expected 1 peak day, got %d: %v", len(result.PeakActivityDays), result.PeakActivityDays)
	}
}

// ---------------------------------------------------------------------------
// TestAnalyzeWorklogs — edge cases
// ---------------------------------------------------------------------------

func TestAnalyzeWorklogs_Empty(t *testing.T) {
	result := AnalyzeWorklogs(nil)
	if result.LogsDaily {
		t.Error("expected LogsDaily=false for nil input")
	}
	if result.AvgEntriesPerWeek != 0 {
		t.Errorf("expected AvgEntriesPerWeek=0 for nil input, got %f", result.AvgEntriesPerWeek)
	}
}

func TestAnalyzeWorklogs_InvalidDates(t *testing.T) {
	entries := []WorklogEntry{
		{Date: "not-a-date", DurationSeconds: 3600},
		{Date: "also-bad", DurationSeconds: 3600},
	}
	result := AnalyzeWorklogs(entries)
	// All invalid dates → weekCounts is empty → returns WorklogHabits{}
	if result.LogsDaily {
		t.Error("expected LogsDaily=false for all-invalid dates")
	}
	if result.AvgEntriesPerWeek != 0 {
		t.Errorf("expected AvgEntriesPerWeek=0 for invalid dates, got %f", result.AvgEntriesPerWeek)
	}
}

func TestAnalyzeWorklogs_LessThan4DaysPerWeek(t *testing.T) {
	// 3 days in a single week → LogsDaily should be false
	entries := []WorklogEntry{
		{Date: "2024-01-08", DurationSeconds: 3600}, // Mon
		{Date: "2024-01-09", DurationSeconds: 3600}, // Tue
		{Date: "2024-01-10", DurationSeconds: 3600}, // Wed
	}
	result := AnalyzeWorklogs(entries)
	if result.LogsDaily {
		t.Error("expected LogsDaily=false for 3 days/week")
	}
	if result.AvgEntriesPerWeek != 3.0 {
		t.Errorf("expected AvgEntriesPerWeek=3.0, got %f", result.AvgEntriesPerWeek)
	}
}

// ---------------------------------------------------------------------------
// TestAnalyzeWorklogs checks avg entries per week calculation.
// ---------------------------------------------------------------------------

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
