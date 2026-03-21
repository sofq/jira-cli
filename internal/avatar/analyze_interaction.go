package avatar

import (
	"fmt"
	"regexp"
	"sort"
	"strings"
	"time"
)

// WorklogEntry represents a single worklog entry for analysis.
type WorklogEntry struct {
	Date            string `json:"date"`
	DurationSeconds int    `json:"duration_seconds"`
}

// mentionRe matches @username patterns.
var mentionRe = regexp.MustCompile(`@([A-Za-z0-9_.-]+)`)

// AnalyzeResponsePatterns analyses comment records for a given user and returns
// response pattern statistics.
func AnalyzeResponsePatterns(comments []CommentRecord, userID string) ResponsePatterns {
	if len(comments) == 0 {
		return ResponsePatterns{}
	}

	var replyDurations []time.Duration
	ownCount := 0
	othersCount := 0

	// group by issue owner to compute thread depth
	threadCounts := map[string]int{}

	for _, c := range comments {
		if c.ParentTimestamp != "" && c.Timestamp != "" {
			ts, err1 := time.Parse(time.RFC3339, c.Timestamp)
			parent, err2 := time.Parse(time.RFC3339, c.ParentTimestamp)
			if err1 == nil && err2 == nil && ts.After(parent) {
				replyDurations = append(replyDurations, ts.Sub(parent))
			}
		}

		if c.IssueOwner == userID {
			ownCount++
		} else {
			othersCount++
		}

		threadCounts[c.IssueOwner]++
	}

	total := float64(ownCount + othersCount)
	ownPct, othersPct := 0.0, 0.0
	if total > 0 {
		ownPct = float64(ownCount) / total
		othersPct = float64(othersCount) / total
	}

	medianDur := medianDuration(replyDurations)

	avgDepth := 0.0
	if len(threadCounts) > 0 {
		sum := 0
		for _, v := range threadCounts {
			sum += v
		}
		avgDepth = float64(sum) / float64(len(threadCounts))
	}

	return ResponsePatterns{
		MedianReplyTime:       formatDuration(medianDur),
		RepliesToOwnIssuesPct: ownPct,
		RepliesToOthersPct:    othersPct,
		AvgThreadDepth:        avgDepth,
	}
}

// medianDuration returns the median of a slice of durations.
func medianDuration(durations []time.Duration) time.Duration {
	if len(durations) == 0 {
		return 0
	}
	cp := make([]time.Duration, len(durations))
	copy(cp, durations)
	sort.Slice(cp, func(i, j int) bool { return cp[i] < cp[j] })
	n := len(cp)
	if n%2 == 0 {
		return (cp[n/2-1] + cp[n/2]) / 2
	}
	return cp[n/2]
}

// AnalyzeMentionHabits analyses comments for @mention patterns.
func AnalyzeMentionHabits(comments []string) MentionHabits {
	freq := map[string]int{}
	for _, c := range comments {
		matches := mentionRe.FindAllStringSubmatch(c, -1)
		seen := map[string]bool{}
		for _, m := range matches {
			if len(m) > 1 && !seen[m[1]] {
				freq[m[1]]++
				seen[m[1]] = true
			}
		}
	}

	type kv struct {
		key   string
		count int
	}
	var sorted []kv
	for k, v := range freq {
		sorted = append(sorted, kv{k, v})
	}
	sort.Slice(sorted, func(i, j int) bool {
		if sorted[i].count != sorted[j].count {
			return sorted[i].count > sorted[j].count
		}
		return sorted[i].key < sorted[j].key
	})

	topN := 5
	if len(sorted) < topN {
		topN = len(sorted)
	}
	frequently := make([]string, topN)
	for i := 0; i < topN; i++ {
		frequently[i] = sorted[i].key
	}

	allText := strings.ToLower(strings.Join(comments, " "))
	hasReview := strings.Contains(allText, "review") || strings.Contains(allText, "pr") || strings.Contains(allText, "looks")
	hasBlocker := strings.Contains(allText, "block") || strings.Contains(allText, "wait") || strings.Contains(allText, "need")

	var contexts []string
	switch {
	case hasReview && hasBlocker:
		contexts = []string{"blockers_and_reviews"}
	case hasReview:
		contexts = []string{"reviews"}
	case hasBlocker:
		contexts = []string{"blockers"}
	}

	return MentionHabits{
		FrequentlyMentions: frequently,
		MentionContext:     contexts,
	}
}

// AnalyzeEscalation analyses comments and changelog entries for escalation signals.
func AnalyzeEscalation(comments []CommentRecord, changelog []ChangelogEntry, userID string) EscalationSignals {
	blockerPhrases := []string{"blocked", "waiting on", "need input from"}
	foundKeywords := map[string]bool{}

	for _, c := range comments {
		lower := strings.ToLower(c.Text)
		for _, phrase := range blockerPhrases {
			if strings.Contains(lower, phrase) {
				foundKeywords[phrase] = true
			}
		}
	}

	var keywords []string
	for _, phrase := range blockerPhrases {
		if foundKeywords[phrase] {
			keywords = append(keywords, phrase)
		}
	}

	escalationCount := 0
	for _, entry := range changelog {
		if entry.Field == "status" && strings.EqualFold(entry.To, "Blocked") {
			escalationCount++
		}
		if entry.Field == "assignee" {
			escalationCount++
		}
	}

	avgCommentsBefore := 0.0
	if escalationCount > 0 {
		avgCommentsBefore = float64(len(comments)) / float64(escalationCount)
	}

	pattern := "comments_then_escalate"
	if len(keywords) == 0 {
		pattern = "no_escalation_detected"
	}

	return EscalationSignals{
		BlockerKeywords:             keywords,
		EscalationPattern:           pattern,
		AvgCommentsBeforeEscalation: avgCommentsBefore,
	}
}

// AnalyzeCollaboration analyses timestamps to determine active hours and peak days.
func AnalyzeCollaboration(timestamps []string) Collaboration {
	type parsedTS struct {
		t time.Time
	}

	var parsed []parsedTS
	for _, ts := range timestamps {
		t, err := time.Parse(time.RFC3339, ts)
		if err == nil {
			parsed = append(parsed, parsedTS{t: t})
		}
	}

	if len(parsed) == 0 {
		return Collaboration{}
	}

	minHour := 23
	maxHour := 0
	for _, p := range parsed {
		h := p.t.Hour()
		if h < minHour {
			minHour = h
		}
		if h > maxHour {
			maxHour = h
		}
	}

	tz := parsed[0].t.Location().String()
	if tz == "" {
		tz = "UTC"
	}

	dayFreq := map[string]int{}
	for _, p := range parsed {
		day := p.t.Weekday().String()
		dayFreq[day]++
	}

	type dayCount struct {
		day   string
		count int
	}
	var days []dayCount
	for d, c := range dayFreq {
		days = append(days, dayCount{d, c})
	}
	sort.Slice(days, func(i, j int) bool {
		if days[i].count != days[j].count {
			return days[i].count > days[j].count
		}
		return days[i].day < days[j].day
	})

	topDays := 3
	if len(days) < topDays {
		topDays = len(days)
	}
	peakDays := make([]string, topDays)
	for i := 0; i < topDays; i++ {
		peakDays[i] = days[i].day
	}

	return Collaboration{
		ActiveHours: ActiveHours{
			Start:    fmt.Sprintf("%02d:00", minHour),
			End:      fmt.Sprintf("%02d:00", maxHour),
			Timezone: tz,
		},
		PeakActivityDays: peakDays,
	}
}

// AnalyzeWorklogs analyses worklog entries and returns worklog habits.
func AnalyzeWorklogs(entries []WorklogEntry) WorklogHabits {
	if len(entries) == 0 {
		return WorklogHabits{}
	}

	type weekKey struct {
		year int
		week int
	}
	weekDays := map[weekKey]map[string]bool{}
	weekCounts := map[weekKey]int{}

	for _, e := range entries {
		t, err := time.Parse("2006-01-02", e.Date)
		if err != nil {
			continue
		}
		year, week := t.ISOWeek()
		key := weekKey{year, week}
		if weekDays[key] == nil {
			weekDays[key] = map[string]bool{}
		}
		weekDays[key][e.Date] = true
		weekCounts[key]++
	}

	if len(weekCounts) == 0 {
		return WorklogHabits{}
	}

	totalEntries := 0
	for _, c := range weekCounts {
		totalEntries += c
	}
	avgEntriesPerWeek := float64(totalEntries) / float64(len(weekCounts))

	logsDaily := false
	for _, days := range weekDays {
		if len(days) >= 4 {
			logsDaily = true
			break
		}
	}

	return WorklogHabits{
		LogsDaily:         logsDaily,
		AvgEntriesPerWeek: avgEntriesPerWeek,
	}
}
