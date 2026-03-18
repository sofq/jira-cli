package avatar

import (
	"fmt"
	"sort"
	"time"
)

// AnalyzeTransitions analyzes status change entries and returns transition patterns.
func AnalyzeTransitions(entries []ChangelogEntry) TransitionPatterns {
	if len(entries) == 0 {
		return TransitionPatterns{}
	}

	// Filter to status changes only
	var statusEntries []ChangelogEntry
	for _, e := range entries {
		if e.Field == "status" {
			statusEntries = append(statusEntries, e)
		}
	}

	// Detect assignsBeforeTransition: check if any "assignee" change appears
	// immediately before a "status" change for the same issue.
	assignsBeforeTransition := false
	// Build a map of issue -> sorted entries (all fields)
	issueAllEntries := map[string][]ChangelogEntry{}
	for _, e := range entries {
		issueAllEntries[e.Issue] = append(issueAllEntries[e.Issue], e)
	}
	for _, issueEntries := range issueAllEntries {
		// Sort by timestamp
		sort.Slice(issueEntries, func(i, j int) bool {
			ti, _ := time.Parse(time.RFC3339, issueEntries[i].Timestamp)
			tj, _ := time.Parse(time.RFC3339, issueEntries[j].Timestamp)
			return ti.Before(tj)
		})
		for i := 1; i < len(issueEntries); i++ {
			if issueEntries[i].Field == "status" && issueEntries[i-1].Field == "assignee" {
				assignsBeforeTransition = true
			}
		}
	}

	// Group status entries per issue, sorted by timestamp
	issueStatusEntries := map[string][]ChangelogEntry{}
	for _, e := range statusEntries {
		issueStatusEntries[e.Issue] = append(issueStatusEntries[e.Issue], e)
	}
	for issue := range issueStatusEntries {
		sort.Slice(issueStatusEntries[issue], func(i, j int) bool {
			ti, _ := time.Parse(time.RFC3339, issueStatusEntries[issue][i].Timestamp)
			tj, _ := time.Parse(time.RFC3339, issueStatusEntries[issue][j].Timestamp)
			return ti.Before(tj)
		})
	}

	// Compute avg time in each status (time between entering a status and leaving it)
	// status -> list of durations
	statusDurations := map[string][]time.Duration{}
	for _, issueEntryList := range issueStatusEntries {
		for i := 1; i < len(issueEntryList); i++ {
			prev := issueEntryList[i-1]
			curr := issueEntryList[i]
			tPrev, errPrev := time.Parse(time.RFC3339, prev.Timestamp)
			tCurr, errCurr := time.Parse(time.RFC3339, curr.Timestamp)
			if errPrev != nil || errCurr != nil {
				continue
			}
			duration := tCurr.Sub(tPrev)
			statusDurations[prev.To] = append(statusDurations[prev.To], duration)
		}
	}

	avgTimeInStatus := map[string]string{}
	for status, durations := range statusDurations {
		var total time.Duration
		for _, d := range durations {
			total += d
		}
		avg := total / time.Duration(len(durations))
		avgTimeInStatus[status] = formatDuration(avg)
	}

	// Find common sequences (top 3)
	seqCounts := map[string]int{}
	seqMap := map[string][]string{}
	for _, issueEntryList := range issueStatusEntries {
		if len(issueEntryList) < 2 {
			continue
		}
		seq := make([]string, 0, len(issueEntryList)+1)
		// Start from the "From" of first entry
		if issueEntryList[0].From != "" {
			seq = append(seq, issueEntryList[0].From)
		}
		for _, e := range issueEntryList {
			seq = append(seq, e.To)
		}
		// Build a key from the sequence
		key := fmt.Sprintf("%v", seq)
		seqCounts[key]++
		seqMap[key] = seq
	}

	type seqEntry struct {
		seq   []string
		count int
	}
	var seqList []seqEntry
	for k, count := range seqCounts {
		seqList = append(seqList, seqEntry{seq: seqMap[k], count: count})
	}
	sort.Slice(seqList, func(i, j int) bool {
		return seqList[i].count > seqList[j].count
	})

	var commonSequences [][]string
	for i := 0; i < len(seqList) && i < 3; i++ {
		commonSequences = append(commonSequences, seqList[i].seq)
	}

	return TransitionPatterns{
		AvgTimeInStatus:         avgTimeInStatus,
		CommonSequences:         commonSequences,
		AssignsBeforeTransition: assignsBeforeTransition,
	}
}

// formatDuration formats a duration in a human-readable form (e.g., "2d", "4h", "30m").
func formatDuration(d time.Duration) string {
	if d < 0 {
		d = -d
	}
	days := int(d.Hours()) / 24
	if days > 0 {
		return fmt.Sprintf("%dd", days)
	}
	hours := int(d.Hours())
	if hours > 0 {
		return fmt.Sprintf("%dh", hours)
	}
	minutes := int(d.Minutes())
	return fmt.Sprintf("%dm", minutes)
}

// AnalyzeFieldPreferences analyzes what fields users set across issues.
func AnalyzeFieldPreferences(issues []IssueFields) FieldPreferences {
	if len(issues) == 0 {
		return FieldPreferences{}
	}

	total := len(issues)
	fieldCounts := map[string]int{
		"priority":    0,
		"labels":      0,
		"components":  0,
		"fix_version": 0,
	}

	priorityCount := map[string]int{}
	labelCount := map[string]int{}
	componentCount := map[string]int{}

	for _, issue := range issues {
		if issue.Priority != "" {
			fieldCounts["priority"]++
			priorityCount[issue.Priority]++
		}
		if len(issue.Labels) > 0 {
			fieldCounts["labels"]++
			for _, l := range issue.Labels {
				if l != "" {
					labelCount[l]++
				}
			}
		}
		if len(issue.Components) > 0 {
			fieldCounts["components"]++
			for _, c := range issue.Components {
				if c != "" {
					componentCount[c]++
				}
			}
		}
		if issue.FixVersion != "" {
			fieldCounts["fix_version"]++
		}
	}

	var alwaysSets []string
	var rarelySets []string
	for field, count := range fieldCounts {
		ratio := float64(count) / float64(total)
		if ratio > 0.8 {
			alwaysSets = append(alwaysSets, field)
		} else if ratio < 0.2 {
			rarelySets = append(rarelySets, field)
		}
	}
	sort.Strings(alwaysSets)
	sort.Strings(rarelySets)

	// Find mode priority
	defaultPriority := ""
	maxCount := 0
	for p, c := range priorityCount {
		if c > maxCount {
			maxCount = c
			defaultPriority = p
		}
	}

	// Top labels
	commonLabels := topKeys(labelCount, 5)
	// Top components
	commonComponents := topKeys(componentCount, 5)

	return FieldPreferences{
		AlwaysSets:       alwaysSets,
		RarelySets:       rarelySets,
		DefaultPriority:  defaultPriority,
		CommonLabels:     commonLabels,
		CommonComponents: commonComponents,
	}
}

// topKeys returns the top-N keys from a frequency map, sorted by count descending.
func topKeys(counts map[string]int, n int) []string {
	type kv struct {
		key   string
		count int
	}
	var pairs []kv
	for k, v := range counts {
		pairs = append(pairs, kv{k, v})
	}
	sort.Slice(pairs, func(i, j int) bool {
		if pairs[i].count != pairs[j].count {
			return pairs[i].count > pairs[j].count
		}
		return pairs[i].key < pairs[j].key
	})
	var result []string
	for i := 0; i < len(pairs) && i < n; i++ {
		result = append(result, pairs[i].key)
	}
	return result
}

// AnalyzeIssueCreation analyzes issue type patterns from created issues.
func AnalyzeIssueCreation(issues []CreatedIssue) IssueCreation {
	if len(issues) == 0 {
		return IssueCreation{}
	}

	total := len(issues)
	typeCounts := map[string]int{}
	var storyCount int
	var totalSubtasks int

	for _, issue := range issues {
		typeCounts[issue.Type]++
		if issue.Type == "Story" {
			storyCount++
			totalSubtasks += issue.SubtaskCount
		}
	}

	typesCreated := map[string]float64{}
	for t, count := range typeCounts {
		typesCreated[t] = float64(count) / float64(total)
	}

	var avgSubtasksPerStory float64
	if storyCount > 0 {
		avgSubtasksPerStory = float64(totalSubtasks) / float64(storyCount)
	}

	return IssueCreation{
		TypesCreated:        typesCreated,
		AvgSubtasksPerStory: avgSubtasksPerStory,
	}
}
