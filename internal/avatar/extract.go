package avatar

import (
	"fmt"
	"strconv"
	"time"

	"github.com/sofq/jira-cli/internal/client"
)

// ---------------------------------------------------------------------------
// Public types
// ---------------------------------------------------------------------------

// ExtractOptions controls the behaviour of the Extract function.
type ExtractOptions struct {
	UserFlag    string // Jira user to analyse; empty means the authenticated user
	MinComments int    // target minimum number of comments to collect
	MinUpdates  int    // target minimum number of updated issues to collect
	MaxWindow   string // maximum lookback window, e.g. "6m" or "2w"
	NoCache     bool   // if true, bypass any HTTP caches
}

// InsufficientDataError is returned when the collected data does not meet the
// hard minimum thresholds required to produce a useful extraction.
type InsufficientDataError struct {
	Found    DataPoints
	Required DataPoints
}

func (e *InsufficientDataError) Error() string {
	return fmt.Sprintf(
		"insufficient data: found %d comments (need %d) and %d issues (need %d)",
		e.Found.Comments, e.Required.Comments,
		e.Found.IssuesCreated, e.Required.IssuesCreated,
	)
}

// ---------------------------------------------------------------------------
// Defaults
// ---------------------------------------------------------------------------

// DefaultExtractOptions returns sensible default extraction options.
func DefaultExtractOptions() ExtractOptions {
	return ExtractOptions{
		MinComments: 50,
		MinUpdates:  30,
		MaxWindow:   "6m",
	}
}

// ---------------------------------------------------------------------------
// Main orchestrator
// ---------------------------------------------------------------------------

const extractVersion = "1"

// hardMinComments is the absolute minimum number of comments needed.
const hardMinComments = 10

// hardMinIssues is the absolute minimum number of issues needed.
const hardMinIssues = 5

// initialWindow is the starting lookback window for the adaptive loop.
const initialWindow = 14 * 24 * time.Hour // 2 weeks

// Extract runs the full extraction pipeline: it resolves the target user,
// collects activity data over an adaptive time window, runs all analysers,
// and returns a populated Extraction document.
func Extract(c *client.Client, opts ExtractOptions) (*Extraction, error) {
	if c == nil {
		return nil, fmt.Errorf("extract: client must not be nil")
	}

	// Resolve user.
	user, err := ResolveUser(c, opts.UserFlag)
	if err != nil {
		return nil, fmt.Errorf("extract: resolve user: %w", err)
	}

	maxWindow, err := parseWindowDuration(opts.MaxWindow)
	if err != nil {
		return nil, fmt.Errorf("extract: invalid MaxWindow %q: %w", opts.MaxWindow, err)
	}

	// Adaptive window: start at 2 weeks, double until targets met or max reached.
	now := time.Now().UTC()
	var (
		rawComments []RawComment
		createdIssues []CreatedIssue
		changelog     []ChangelogEntry
		window        = initialWindow
		from, to      time.Time
	)

	for {
		if window > maxWindow {
			window = maxWindow
		}
		to = now
		from = now.Add(-window)

		fromStr := from.Format("2006-01-02")
		toStr := to.Format("2006-01-02")

		rawComments, err = FetchUserComments(c, user.AccountID, fromStr, toStr)
		if err != nil {
			return nil, fmt.Errorf("extract: fetch comments: %w", err)
		}

		createdIssues, err = FetchUserIssues(c, user.AccountID, fromStr, toStr)
		if err != nil {
			return nil, fmt.Errorf("extract: fetch issues: %w", err)
		}

		changelog, err = FetchUserChangelog(c, user.AccountID, fromStr, toStr)
		if err != nil {
			return nil, fmt.Errorf("extract: fetch changelog: %w", err)
		}

		// Check if targets are met.
		if len(rawComments) >= opts.MinComments && len(createdIssues) >= opts.MinUpdates {
			break
		}

		// If we've already used the max window, stop regardless.
		if window >= maxWindow {
			break
		}

		// Double the window.
		window *= 2
	}

	// Check hard minimums.
	found := DataPoints{
		Comments:      len(rawComments),
		IssuesCreated: len(createdIssues),
		Transitions:   len(changelog),
	}
	required := DataPoints{
		Comments:      hardMinComments,
		IssuesCreated: hardMinIssues,
	}
	if found.Comments < hardMinComments || found.IssuesCreated < hardMinIssues {
		return nil, &InsufficientDataError{Found: found, Required: required}
	}

	// ---------------------------------------------------------------------------
	// Run all analysers.
	// ---------------------------------------------------------------------------

	// Extract plain-text slices for writing analysis.
	commentTexts := make([]string, len(rawComments))
	for i, c := range rawComments {
		commentTexts[i] = c.Text
	}

	descriptionTexts := make([]string, len(createdIssues))
	for i, issue := range createdIssues {
		descriptionTexts[i] = issue.Description
	}

	// Build IssueFields slice for field preference analysis.
	issueFields := make([]IssueFields, len(createdIssues))
	// issueFields contains zero-value structs since CreatedIssue doesn't carry
	// priority/labels/components; AnalyzeFieldPreferences gracefully handles this.

	// Build CommentRecords for interaction analysis.
	commentRecords := make([]CommentRecord, len(rawComments))
	for i, rc := range rawComments {
		commentRecords[i] = CommentRecord{
			IssueOwner: rc.Issue, // approximate: owner = issue key
			Author:     rc.Author,
			Timestamp:  rc.Date,
			Text:       rc.Text,
		}
	}

	// Collect timestamps from all activity for collaboration analysis.
	timestamps := make([]string, 0, len(rawComments)+len(changelog))
	for _, rc := range rawComments {
		if rc.Date != "" {
			timestamps = append(timestamps, rc.Date)
		}
	}
	for _, ce := range changelog {
		if ce.Timestamp != "" {
			timestamps = append(timestamps, ce.Timestamp)
		}
	}

	// Writing
	commentStats := AnalyzeComments(commentTexts)
	descriptionStats := AnalyzeDescriptions(descriptionTexts)

	// Workflow
	transitionPatterns := AnalyzeTransitions(changelog)
	fieldPreferences := AnalyzeFieldPreferences(issueFields)
	issueCreation := AnalyzeIssueCreation(createdIssues)

	// Interaction
	responsePatterns := AnalyzeResponsePatterns(commentRecords, user.AccountID)
	mentionHabits := AnalyzeMentionHabits(commentTexts)
	collaboration := AnalyzeCollaboration(timestamps)

	// Examples
	commentExamples := SelectCommentExamples(rawComments, 10)
	descriptionExamples := SelectDescriptionExamples(createdIssues, 3)

	extraction := &Extraction{
		Version: extractVersion,
		Meta: ExtractionMeta{
			User:        user.AccountID,
			DisplayName: user.DisplayName,
			ExtractedAt: now,
			Window: TimeWindow{
				From: from,
				To:   to,
			},
			DataPoints: found,
		},
		Writing: WritingAnalysis{
			Comments:     commentStats,
			Descriptions: descriptionStats,
		},
		Workflow: WorkflowAnalysis{
			TransitionPatterns: transitionPatterns,
			FieldPreferences:   fieldPreferences,
			IssueCreation:      issueCreation,
		},
		Interaction: InteractionAnalysis{
			ResponsePatterns: responsePatterns,
			MentionHabits:    mentionHabits,
			Collaboration:    collaboration,
		},
		Examples: ExtractionExamples{
			Comments:     commentExamples,
			Descriptions: descriptionExamples,
		},
	}

	return extraction, nil
}

// ---------------------------------------------------------------------------
// parseWindowDuration
// ---------------------------------------------------------------------------

// parseWindowDuration parses a window string like "2w", "1m", "6m" into a
// time.Duration. An empty string is treated as "6m". Returns an error for any
// unrecognised or non-positive format.
func parseWindowDuration(s string) (time.Duration, error) {
	if s == "" {
		return 180 * 24 * time.Hour, nil // default 6 months
	}

	if len(s) < 2 {
		return 0, fmt.Errorf("invalid window duration %q: must be <number><unit> where unit is w or m", s)
	}

	unit := s[len(s)-1]
	numStr := s[:len(s)-1]

	n, err := strconv.Atoi(numStr)
	if err != nil || n <= 0 {
		return 0, fmt.Errorf("invalid window duration %q: number part must be a positive integer", s)
	}

	switch unit {
	case 'w':
		return time.Duration(n) * 7 * 24 * time.Hour, nil
	case 'm':
		return time.Duration(n) * 30 * 24 * time.Hour, nil
	default:
		return 0, fmt.Errorf("invalid window duration %q: unit must be 'w' (weeks) or 'm' (months)", s)
	}
}
