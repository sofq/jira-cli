package avatar

import (
	"sort"
	"strings"
)

// classifyComment classifies a comment text into a context category using
// keyword matching. Priority order: blocker_report > decision > request >
// pushback > review_feedback > acknowledgment > technical > question > status_update.
func classifyComment(text string) string {
	lower := strings.ToLower(text)

	// blocker_report: highest priority
	if strings.Contains(lower, "block") ||
		strings.Contains(lower, "waiting on") ||
		strings.Contains(lower, "waiting for") ||
		strings.Contains(lower, "need input") ||
		strings.Contains(lower, "stuck") {
		return "blocker_report"
	}

	// decision: explicit choices or conclusions
	if strings.Contains(lower, "decided") ||
		strings.Contains(lower, "going with") ||
		strings.Contains(lower, "let's go with") ||
		strings.Contains(lower, "we'll use") ||
		strings.Contains(lower, "the plan is") {
		return "decision"
	}

	// request: asking someone to do something
	if strings.Contains(lower, "can you") ||
		strings.Contains(lower, "could you") ||
		strings.Contains(lower, "please") ||
		strings.Contains(lower, "would you") {
		return "request"
	}

	// pushback: disagreement or concerns
	if strings.Contains(lower, "disagree") ||
		strings.Contains(lower, "concern") ||
		strings.Contains(lower, "not sure about") ||
		strings.Contains(lower, "i don't think") ||
		strings.Contains(lower, "instead") ||
		strings.Contains(lower, "alternatively") {
		return "pushback"
	}

	// review_feedback
	if strings.Contains(lower, "looks good") ||
		strings.Contains(lower, "lgtm") ||
		strings.Contains(lower, "nit") ||
		strings.Contains(lower, "approve") ||
		strings.Contains(lower, "review") {
		return "review_feedback"
	}

	// acknowledgment: short confirmations
	if strings.Contains(lower, "thanks") ||
		strings.Contains(lower, "got it") ||
		strings.Contains(lower, "noted") ||
		strings.Contains(lower, "sounds good") ||
		strings.Contains(lower, "will do") ||
		strings.Contains(lower, "on it") {
		return "acknowledgment"
	}

	// technical: code/architecture discussion
	if strings.Contains(lower, "stack trace") ||
		strings.Contains(lower, "error") ||
		strings.Contains(lower, "exception") ||
		strings.Contains(lower, "root cause") ||
		strings.Contains(lower, "refactor") ||
		strings.Contains(lower, "migration") ||
		strings.Contains(lower, "endpoint") ||
		strings.Contains(lower, "database") ||
		strings.Contains(lower, "api") {
		return "technical"
	}

	// question: ends with "?" or high question density
	trimmed := strings.TrimSpace(text)
	if strings.HasSuffix(trimmed, "?") {
		return "question"
	}
	sentences := strings.Split(trimmed, ".")
	questionCount := strings.Count(trimmed, "?")
	if len(sentences) > 0 && questionCount > 0 {
		if float64(questionCount)/float64(len(sentences)) > 0.33 {
			return "question"
		}
	}

	return "status_update"
}

// medianWordCount returns the median word count for a slice of strings.
func medianWordCount(texts []string) int {
	if len(texts) == 0 {
		return 0
	}
	counts := make([]int, len(texts))
	for i, t := range texts {
		counts[i] = wordCount(t)
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
// context, and picks up to 2 examples per category (closest to median word
// count), distributed round-robin across contexts from largest groups.
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

	// For each group, find up to 2 best representatives
	type groupEntry struct {
		context   string
		examples  []RawComment
		groupSize int
	}

	var entries []groupEntry
	for ctx, grp := range groups {
		texts := make([]string, len(grp))
		for i, c := range grp {
			texts[i] = c.Text
		}
		med := medianWordCount(texts)

		// Sort by distance from median
		type scored struct {
			comment RawComment
			dist    int
		}
		var scoredComments []scored
		for _, c := range grp {
			scoredComments = append(scoredComments, scored{c, absInt(wordCount(c.Text) - med)})
		}
		sort.Slice(scoredComments, func(i, j int) bool {
			return scoredComments[i].dist < scoredComments[j].dist
		})

		// Take up to 2 per category
		maxPerCategory := 2
		if len(scoredComments) < maxPerCategory {
			maxPerCategory = len(scoredComments)
		}
		var examples []RawComment
		for i := 0; i < maxPerCategory; i++ {
			examples = append(examples, scoredComments[i].comment)
		}

		entries = append(entries, groupEntry{
			context:   ctx,
			examples:  examples,
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

	// Round-robin: first pass takes 1 per group, second pass takes the second
	var result []CommentExample
	for pass := 0; pass < 2; pass++ {
		for _, e := range entries {
			if len(result) >= maxExamples {
				break
			}
			if pass < len(e.examples) {
				c := e.examples[pass]
				result = append(result, CommentExample{
					Issue:   c.Issue,
					Date:    c.Date,
					Text:    c.Text,
					Context: e.context,
				})
			}
		}
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
		bestDist := absInt(wordCount(grp[0].Description) - med)
		for _, d := range grp[1:] {
			dist := absInt(wordCount(d.Description) - med)
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
