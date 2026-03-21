package avatar

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"strings"

	"github.com/sofq/jira-cli/internal/client"
)

// JiraUser represents a Jira user as returned by the API.
type JiraUser struct {
	AccountID   string `json:"accountId"`
	Email       string `json:"emailAddress"`
	DisplayName string `json:"displayName"`
}

// ResolveUser resolves the target Jira user.
// If userFlag is empty it fetches the authenticated user (/rest/api/3/myself).
// If userFlag is set it searches for matching users and returns the first result.
func ResolveUser(ctx context.Context, c *client.Client, userFlag string) (*JiraUser, error) {
	if userFlag == "" {
		body, exitCode := c.Fetch(ctx, "GET", "/rest/api/3/myself", nil)
		if exitCode != 0 {
			return nil, fmt.Errorf("failed to fetch current user (exit %d)", exitCode)
		}
		var user JiraUser
		if err := json.Unmarshal(body, &user); err != nil {
			return nil, fmt.Errorf("failed to parse current user: %w", err)
		}
		return &user, nil
	}

	path := "/rest/api/3/user/search?query=" + url.QueryEscape(userFlag)
	body, exitCode := c.Fetch(ctx, "GET", path, nil)
	if exitCode != 0 {
		return nil, fmt.Errorf("failed to search for user %q (exit %d)", userFlag, exitCode)
	}
	var users []JiraUser
	if err := json.Unmarshal(body, &users); err != nil {
		return nil, fmt.Errorf("failed to parse user search results: %w", err)
	}
	if len(users) == 0 {
		return nil, fmt.Errorf("no user found matching %q", userFlag)
	}
	return &users[0], nil
}

// escapeJQLString escapes a value for interpolation inside a JQL double-quoted string.
// It escapes backslashes and double quotes to prevent JQL injection.
func escapeJQLString(s string) string {
	s = strings.ReplaceAll(s, `\`, `\\`)
	s = strings.ReplaceAll(s, `"`, `\"`)
	return s
}

// FetchUserComments fetches comments authored by accountID on issues updated
// within the [from, to] date window.
func FetchUserComments(ctx context.Context, c *client.Client, accountID, from, to string) ([]RawComment, error) {
	aid := escapeJQLString(accountID)
	jql := fmt.Sprintf(
		`(reporter = "%s" OR assignee was "%s") AND updated >= "%s" AND updated <= "%s" ORDER BY updated DESC`,
		aid, aid, escapeJQLString(from), escapeJQLString(to),
	)
	var allResults []RawComment
	nextPageToken := ""

	for {
		reqMap := map[string]interface{}{
			"jql":        jql,
			"fields":     []string{"comment", "reporter"},
			"maxResults": 50,
		}
		if nextPageToken != "" {
			reqMap["nextPageToken"] = nextPageToken
		}
		reqBytes, _ := json.Marshal(reqMap)

		body, exitCode := c.Fetch(ctx, "POST", "/rest/api/3/search/jql",
			strings.NewReader(string(reqBytes)))
		if exitCode != 0 {
			return nil, fmt.Errorf("failed to fetch issues for comments (exit %d)", exitCode)
		}

		// Parse the search response.
		var searchResp struct {
			Issues []struct {
				Key    string `json:"key"`
				Fields struct {
					Reporter struct {
						AccountID string `json:"accountId"`
					} `json:"reporter"`
					Comment struct {
						Comments []struct {
							Author struct {
								AccountID string `json:"accountId"`
							} `json:"author"`
							Created string          `json:"created"`
							Body    json.RawMessage `json:"body"`
						} `json:"comments"`
					} `json:"comment"`
				} `json:"fields"`
			} `json:"issues"`
			NextPageToken string `json:"nextPageToken"`
		}
		if err := json.Unmarshal(body, &searchResp); err != nil {
			return nil, fmt.Errorf("failed to parse comment search response: %w", err)
		}

		for _, issue := range searchResp.Issues {
			reporter := issue.Fields.Reporter.AccountID
			for _, c := range issue.Fields.Comment.Comments {
				if c.Author.AccountID != accountID {
					continue
				}
				text := extractTextFromADF(c.Body)
				allResults = append(allResults, RawComment{
					Issue:    issue.Key,
					Date:     c.Created,
					Text:     text,
					Author:   accountID,
					Reporter: reporter,
				})
			}
		}

		if searchResp.NextPageToken == "" {
			break
		}
		nextPageToken = searchResp.NextPageToken
	}
	return allResults, nil
}

// FetchUserIssues fetches issues reported by accountID created within [from, to].
func FetchUserIssues(ctx context.Context, c *client.Client, accountID, from, to string) ([]CreatedIssue, error) {
	aid := escapeJQLString(accountID)
	jql := fmt.Sprintf(`reporter = "%s" AND created >= "%s" AND created <= "%s"`,
		aid, escapeJQLString(from), escapeJQLString(to))

	var allResults []CreatedIssue
	nextPageToken := ""

	for {
		reqMap := map[string]interface{}{
			"jql":        jql,
			"fields":     []string{"key", "issuetype", "subtasks", "description", "priority", "labels", "components", "fixVersions"},
			"maxResults": 50,
		}
		if nextPageToken != "" {
			reqMap["nextPageToken"] = nextPageToken
		}
		reqBytes, _ := json.Marshal(reqMap)

		body, exitCode := c.Fetch(ctx, "POST", "/rest/api/3/search/jql",
			strings.NewReader(string(reqBytes)))
		if exitCode != 0 {
			return nil, fmt.Errorf("failed to fetch issues (exit %d)", exitCode)
		}

		var searchResp struct {
			Issues []struct {
				Key    string `json:"key"`
				Fields struct {
					IssueType struct {
						Name string `json:"name"`
					} `json:"issuetype"`
					Subtasks    []json.RawMessage `json:"subtasks"`
					Description json.RawMessage   `json:"description"`
					Priority    *struct {
						Name string `json:"name"`
					} `json:"priority"`
					Labels     []string `json:"labels"`
					Components []struct {
						Name string `json:"name"`
					} `json:"components"`
					FixVersions []struct {
						Name string `json:"name"`
					} `json:"fixVersions"`
				} `json:"fields"`
			} `json:"issues"`
			NextPageToken string `json:"nextPageToken"`
		}
		if err := json.Unmarshal(body, &searchResp); err != nil {
			return nil, fmt.Errorf("failed to parse issues response: %w", err)
		}

		for _, issue := range searchResp.Issues {
			desc := extractTextFromADF(issue.Fields.Description)

			priority := ""
			if issue.Fields.Priority != nil {
				priority = issue.Fields.Priority.Name
			}

			var components []string
			for _, c := range issue.Fields.Components {
				if c.Name != "" {
					components = append(components, c.Name)
				}
			}

			fixVersion := ""
			if len(issue.Fields.FixVersions) > 0 {
				fixVersion = issue.Fields.FixVersions[0].Name
			}

			allResults = append(allResults, CreatedIssue{
				Key:          issue.Key,
				Type:         issue.Fields.IssueType.Name,
				SubtaskCount: len(issue.Fields.Subtasks),
				Description:  desc,
				Priority:     priority,
				Labels:       issue.Fields.Labels,
				Components:   components,
				FixVersion:   fixVersion,
			})
		}

		if searchResp.NextPageToken == "" {
			break
		}
		nextPageToken = searchResp.NextPageToken
	}
	return allResults, nil
}

// FetchUserChangelog fetches changelog entries authored by accountID on issues
// updated within [from, to].
func FetchUserChangelog(ctx context.Context, c *client.Client, accountID, from, to string) ([]ChangelogEntry, error) {
	aid := escapeJQLString(accountID)
	jql := fmt.Sprintf(
		`(reporter = "%s" OR assignee was "%s") AND updated >= "%s" AND updated <= "%s"`,
		aid, aid, escapeJQLString(from), escapeJQLString(to),
	)
	var allResults []ChangelogEntry
	nextPageToken := ""

	for {
		reqMap := map[string]interface{}{
			"jql":        jql,
			"maxResults": 50,
		}
		if nextPageToken != "" {
			reqMap["nextPageToken"] = nextPageToken
		}
		reqBytes, _ := json.Marshal(reqMap)

		body, exitCode := c.Fetch(ctx, "POST", "/rest/api/3/search/jql?expand=changelog",
			strings.NewReader(string(reqBytes)))
		if exitCode != 0 {
			return nil, fmt.Errorf("failed to fetch changelog (exit %d)", exitCode)
		}

		var searchResp struct {
			Issues []struct {
				Key       string `json:"key"`
				Changelog struct {
					Histories []struct {
						Author struct {
							AccountID string `json:"accountId"`
						} `json:"author"`
						Created string `json:"created"`
						Items   []struct {
							Field      string `json:"field"`
							FromString string `json:"fromString"`
							ToString   string `json:"toString"`
						} `json:"items"`
					} `json:"histories"`
				} `json:"changelog"`
			} `json:"issues"`
			NextPageToken string `json:"nextPageToken"`
		}
		if err := json.Unmarshal(body, &searchResp); err != nil {
			return nil, fmt.Errorf("failed to parse changelog response: %w", err)
		}

		for _, issue := range searchResp.Issues {
			for _, history := range issue.Changelog.Histories {
				if history.Author.AccountID != accountID {
					continue
				}
				for _, item := range history.Items {
					allResults = append(allResults, ChangelogEntry{
						Issue:     issue.Key,
						Timestamp: history.Created,
						Author:    accountID,
						Field:     item.Field,
						From:      item.FromString,
						To:        item.ToString,
					})
				}
			}
		}

		if searchResp.NextPageToken == "" {
			break
		}
		nextPageToken = searchResp.NextPageToken
	}
	return allResults, nil
}

// FetchUserWorklogs fetches worklog entries authored by accountID on issues
// updated within [from, to].
func FetchUserWorklogs(ctx context.Context, c *client.Client, accountID, from, to string) ([]WorklogEntry, error) {
	aid := escapeJQLString(accountID)
	jql := fmt.Sprintf(
		`(reporter = "%s" OR assignee was "%s") AND updated >= "%s" AND updated <= "%s"`,
		aid, aid, escapeJQLString(from), escapeJQLString(to),
	)

	var allResults []WorklogEntry
	nextPageToken := ""

	for {
		reqMap := map[string]interface{}{
			"jql":        jql,
			"fields":     []string{"worklog"},
			"maxResults": 50,
		}
		if nextPageToken != "" {
			reqMap["nextPageToken"] = nextPageToken
		}
		reqBytes, _ := json.Marshal(reqMap)

		body, exitCode := c.Fetch(ctx, "POST", "/rest/api/3/search/jql",
			strings.NewReader(string(reqBytes)))
		if exitCode != 0 {
			return nil, fmt.Errorf("failed to fetch worklogs (exit %d)", exitCode)
		}

		var searchResp struct {
			Issues []struct {
				Fields struct {
					Worklog struct {
						Worklogs []struct {
							Author struct {
								AccountID string `json:"accountId"`
							} `json:"author"`
							Started          string `json:"started"`
							TimeSpentSeconds int    `json:"timeSpentSeconds"`
						} `json:"worklogs"`
					} `json:"worklog"`
				} `json:"fields"`
			} `json:"issues"`
			NextPageToken string `json:"nextPageToken"`
		}
		if err := json.Unmarshal(body, &searchResp); err != nil {
			return nil, fmt.Errorf("failed to parse worklog response: %w", err)
		}

		for _, issue := range searchResp.Issues {
			for _, wl := range issue.Fields.Worklog.Worklogs {
				if wl.Author.AccountID != accountID {
					continue
				}
				// Parse date from the started field (e.g. "2025-01-15T10:00:00.000+0000")
				date := wl.Started
				if len(date) >= 10 {
					date = date[:10]
				}
				allResults = append(allResults, WorklogEntry{
					Date:            date,
					DurationSeconds: wl.TimeSpentSeconds,
				})
			}
		}

		if searchResp.NextPageToken == "" {
			break
		}
		nextPageToken = searchResp.NextPageToken
	}
	return allResults, nil
}

// extractTextFromADF extracts plain text from an Atlassian Document Format (ADF)
// JSON blob. Paragraphs are joined with newlines. Falls back to the raw string
// representation if parsing fails or the input is not ADF.
func extractTextFromADF(adf json.RawMessage) string {
	if len(adf) == 0 || string(adf) == "null" {
		return ""
	}

	var doc struct {
		Type    string            `json:"type"`
		Content []json.RawMessage `json:"content"`
	}
	if err := json.Unmarshal(adf, &doc); err != nil {
		return string(adf)
	}

	var paragraphs []string
	for _, block := range doc.Content {
		text := extractNodeText(block)
		if text != "" {
			paragraphs = append(paragraphs, text)
		}
	}
	if len(paragraphs) == 0 {
		return string(adf)
	}
	return strings.Join(paragraphs, "\n")
}

// extractNodeText recursively extracts plain text from an ADF node.
func extractNodeText(raw json.RawMessage) string {
	var node struct {
		Type    string            `json:"type"`
		Text    string            `json:"text"`
		Content []json.RawMessage `json:"content"`
	}
	if err := json.Unmarshal(raw, &node); err != nil {
		return ""
	}
	if node.Type == "text" {
		return node.Text
	}
	var parts []string
	for _, child := range node.Content {
		if t := extractNodeText(child); t != "" {
			parts = append(parts, t)
		}
	}
	return strings.Join(parts, "")
}

