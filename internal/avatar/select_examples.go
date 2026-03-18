package avatar

import (
	"sort"
	"strings"
)

// classifyComment classifies a comment text into a context category using
// keyword matching. Priority: blocker_report > review_feedback > question > status_update.
func classifyComment(text string) string {
	lower := strings.ToLower(text)

	// blocker_report: check first (highest priority)
	if strings.Contains(lower, "block") ||
		strings.Contains(lower, "waiting on") ||
		strings.Contains(lower, "waiting for") ||
		strings.Contains(lower, "need input") {
		return "blocker_report"
	}

	// review_feedback
	if strings.Contains(lower, "looks good") ||
		strings.Contains(lower, "lgtm") ||
		strings.Contains(lower, "nit") ||
		strings.Contains(lower, "approve") ||
		strings.Contains(lower, "review") {
		return "review_feedback"
	}

	// question: ends with "?" or high question density
	trimmed := strings.TrimSpace(text)
	if strings.HasSuffix(trimmed, "?") {
		return "question"
	}
	sentences := strings.Split(trimmed, ".")
	questionCount := strings.Count(trimmed, "?")
	if len(sentences) > 0 && questionCount > 0 {
		// high density: more than 1 question per 3 sentences
		if float64(questionCount)/float64(len(sentences)) > 0.33 {
			return "question"
		}
	}

	return "status_update"
}

// wordCount returns the number of whitespace-separated words in s.
func selectWordCount(s string) int {
	return len(strings.Fields(s))
}

// medianWordCount returns the median word count for a slice of strings.
func medianWordCount(texts []string) int {
	if len(texts) == 0 {
		return 0
	}
	counts := make([]int, len(texts))
	for i, t := range texts {
		counts[i] = selectWordCount(t)
	}
	sort.Ints(counts)
	mid := len(counts) / 2
	if len(counts)%2 == 0 && mid > 0 {
		return (counts[mid-1] + counts[mid]) / 2
	}
	return counts[mid]
}

// abs returns the absolute value of an int.
func absInt(x int) int {
	if x < 0 {
		return -x
	}
	return x
}

// SelectCommentExamples selects up to maxExamples diverse, representative
// comments from the input. It classifies each comment by context, groups by
// context, picks the comment closest to the group's median word count, then
// distributes selections across contexts (round-robin from largest groups).
func SelectCommentExamples(comments []RawComment, maxExamples int) []CommentExample {
	if len(comments) == 0 {
		return nil
	}

	// Group comments by context
	groups := map[string][]RawComment{}
	for _, c := range comments {
		ctx := classifyComment(c.Text)
		groups[ctx] = append(groups[ctx], c)
	}

	// For each group, find the best representative (closest to median word count)
	type groupEntry struct {
		context     string
		best        RawComment
		groupSize   int
	}

	var entries []groupEntry
	for ctx, grp := range groups {
		texts := make([]string, len(grp))
		for i, c := range grp {
			texts[i] = c.Text
		}
		med := medianWordCount(texts)

		best := grp[0]
		bestDist := absInt(selectWordCount(grp[0].Text) - med)
		for _, c := range grp[1:] {
			d := absInt(selectWordCount(c.Text) - med)
			if d < bestDist {
				bestDist = d
				best = c
			}
		}

		entries = append(entries, groupEntry{
			context:   ctx,
			best:      best,
			groupSize: len(grp),
		})
	}

	// Sort by group size descending for round-robin fairness
	sort.Slice(entries, func(i, j int) bool {
		if entries[i].groupSize != entries[j].groupSize {
			return entries[i].groupSize > entries[j].groupSize
		}
		return entries[i].context < entries[j].context
	})

	// Distribute: round-robin from largest groups, up to maxExamples
	var result []CommentExample
	for _, e := range entries {
		if len(result) >= maxExamples {
			break
		}
		c := e.best
		result = append(result, CommentExample{
			Issue:   c.Issue,
			Date:    c.Date,
			Text:    c.Text,
			Context: e.context,
		})
	}

	return result
}

// SelectDescriptionExamples selects up to maxExamples representative issue
// descriptions. It groups by issue type, picks the description closest to the
// group's median word count, and returns one per type up to maxExamples.
func SelectDescriptionExamples(descriptions []CreatedIssue, maxExamples int) []DescriptionExample {
	if len(descriptions) == 0 {
		return nil
	}

	// Group by type
	groups := map[string][]CreatedIssue{}
	for _, d := range descriptions {
		groups[d.Type] = append(groups[d.Type], d)
	}

	type groupEntry struct {
		issueType string
		best      CreatedIssue
		groupSize int
	}

	var entries []groupEntry
	for typ, grp := range groups {
		texts := make([]string, len(grp))
		for i, d := range grp {
			texts[i] = d.Description
		}
		med := medianWordCount(texts)

		best := grp[0]
		bestDist := absInt(selectWordCount(grp[0].Description) - med)
		for _, d := range grp[1:] {
			dist := absInt(selectWordCount(d.Description) - med)
			if dist < bestDist {
				bestDist = dist
				best = d
			}
		}

		entries = append(entries, groupEntry{
			issueType: typ,
			best:      best,
			groupSize: len(grp),
		})
	}

	// Sort by group size descending for determinism
	sort.Slice(entries, func(i, j int) bool {
		if entries[i].groupSize != entries[j].groupSize {
			return entries[i].groupSize > entries[j].groupSize
		}
		return entries[i].issueType < entries[j].issueType
	})

	var result []DescriptionExample
	for _, e := range entries {
		if len(result) >= maxExamples {
			break
		}
		d := e.best
		result = append(result, DescriptionExample{
			Issue: d.Key,
			Type:  d.Type,
			Text:  d.Description,
		})
	}

	return result
}
