package changelog

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/sofq/jira-cli/internal/duration"
)

// Change represents a single field change from the Jira changelog.
// From and To are typed as `any` (not *string) so that JSON marshaling
// produces `"from": null` rather than omitting the field — matching the
// output format specified in the design doc.
type Change struct {
	Timestamp string `json:"timestamp"`
	Author    string `json:"author"`
	Field     string `json:"field"`
	From      any    `json:"from"`
	To        any    `json:"to"`
}

// Result is the top-level output of the diff command.
type Result struct {
	Issue   string   `json:"issue"`
	Changes []Change `json:"changes"`
}

// jiraChangelog is the response from GET /rest/api/3/issue/{key}/changelog.
type jiraChangelog struct {
	Values []jiraHistoryEntry `json:"values"`
}

// jiraHistoryEntry is a single history entry in the Jira changelog.
type jiraHistoryEntry struct {
	Created string `json:"created"`
	Author  struct {
		EmailAddress string `json:"emailAddress"`
		DisplayName  string `json:"displayName"`
	} `json:"author"`
	Items []jiraChangeItem `json:"items"`
}

// jiraChangeItem is a single field change within a history entry.
type jiraChangeItem struct {
	Field      string `json:"field"`
	FromString string `json:"fromString"`
	ToString   string `json:"toString"`
}

// Options controls changelog filtering.
type Options struct {
	Since string    // duration (e.g. "2h") or ISO date (e.g. "2025-01-01")
	Field string    // filter to a specific field name
	Now   time.Time // reference time for duration-based --since; zero means time.Now()
}

// Parse parses raw Jira changelog JSON and returns flattened, filtered changes.
func Parse(issueKey string, raw []byte, opts Options) (*Result, error) {
	var cl jiraChangelog
	if err := json.Unmarshal(raw, &cl); err != nil {
		return nil, fmt.Errorf("failed to parse changelog: %w", err)
	}

	var sinceTime time.Time
	if opts.Since != "" {
		var err error
		sinceTime, err = parseSince(opts.Since, opts.Now)
		if err != nil {
			return nil, err
		}
	}

	fieldFilter := strings.ToLower(opts.Field)

	var changes []Change
	for _, entry := range cl.Values {
		// Filter by time.
		if !sinceTime.IsZero() {
			entryTime, err := parseJiraTime(entry.Created)
			if err != nil {
				continue
			}
			if entryTime.Before(sinceTime) {
				continue
			}
		}

		author := entry.Author.EmailAddress
		if author == "" {
			author = entry.Author.DisplayName
		}

		for _, item := range entry.Items {
			if fieldFilter != "" && strings.ToLower(item.Field) != fieldFilter {
				continue
			}

			change := Change{
				Timestamp: entry.Created,
				Author:    author,
				Field:     item.Field,
			}
			if item.FromString == "" {
				change.From = nil
			} else {
				change.From = item.FromString
			}
			if item.ToString == "" {
				change.To = nil
			} else {
				change.To = item.ToString
			}
			changes = append(changes, change)
		}
	}

	if changes == nil {
		changes = []Change{}
	}

	return &Result{
		Issue:   issueKey,
		Changes: changes,
	}, nil
}

// parseSince parses a --since value as either a duration ("2h", "1d") or ISO date.
// The now parameter is the reference time for duration subtraction; if zero, time.Now() is used.
func parseSince(s string, now time.Time) (time.Time, error) {
	// Try ISO date formats first.
	for _, layout := range []string{
		time.RFC3339,
		"2006-01-02T15:04:05",
		"2006-01-02",
	} {
		if t, err := time.Parse(layout, s); err == nil {
			return t, nil
		}
	}

	// Try duration format via the shared duration parser (e.g. "2h", "30m", "1d").
	secs, err := duration.Parse(s)
	if err != nil {
		return time.Time{}, fmt.Errorf("invalid --since value %q: expected duration (e.g. 2h, 1d) or date (e.g. 2025-01-01)", s)
	}
	if now.IsZero() {
		now = time.Now()
	}
	return now.Add(-time.Duration(secs) * time.Second), nil
}

// parseJiraTime parses Jira's timestamp format.
func parseJiraTime(s string) (time.Time, error) {
	// Jira uses RFC3339 with milliseconds.
	for _, layout := range []string{
		time.RFC3339,
		"2006-01-02T15:04:05.000-0700",
		"2006-01-02T15:04:05.000Z",
		"2006-01-02T15:04:05Z",
	} {
		if t, err := time.Parse(layout, s); err == nil {
			return t, nil
		}
	}
	return time.Time{}, fmt.Errorf("cannot parse Jira timestamp %q", s)
}
