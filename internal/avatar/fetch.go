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
func ResolveUser(c *client.Client, userFlag string) (*JiraUser, error) {
	if userFlag == "" {
		body, exitCode := c.Fetch(context.Background(), "GET", "/rest/api/3/myself", nil)
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
	body, exitCode := c.Fetch(context.Background(), "GET", path, nil)
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
func FetchUserComments(c *client.Client, accountID, from, to string) ([]RawComment, error) {
	aid := escapeJQLString(accountID)
	jql := fmt.Sprintf(
		`(reporter = "%s" OR assignee was "%s") AND updated >= "%s" AND updated <= "%s" ORDER BY updated DESC`,
		aid, aid, from, to,
	)
	reqBytes, _ := json.Marshal(map[string]interface{}{
		"jql":        jql,
		"fields":     []string{"comment"},
		"maxResults": 50,
	})

	body, exitCode := c.Fetch(context.Background(), "POST", "/rest/api/3/search/jql",
		strings.NewReader(string(reqBytes)))
	if exitCode != 0 {
		return nil, fmt.Errorf("failed to fetch issues for comments (exit %d)", exitCode)
	}

	// Parse the search response.
	var searchResp struct {
		Issues []struct {
			Key    string `json:"key"`
			Fields struct {
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
	}
	if err := json.Unmarshal(body, &searchResp); err != nil {
		return nil, fmt.Errorf("failed to parse comment search response: %w", err)
	}

	var results []RawComment
	for _, issue := range searchResp.Issues {
		for _, c := range issue.Fields.Comment.Comments {
			if c.Author.AccountID != accountID {
				continue
			}
			text := extractTextFromADF(c.Body)
			results = append(results, RawComment{
				Issue:  issue.Key,
				Date:   c.Created,
				Text:   text,
				Author: accountID,
			})
		}
	}
	return results, nil
}

// FetchUserIssues fetches issues reported by accountID created within [from, to].
func FetchUserIssues(c *client.Client, accountID, from, to string) ([]CreatedIssue, error) {
	aid := escapeJQLString(accountID)
	jql := fmt.Sprintf(`reporter = "%s" AND created >= "%s" AND created <= "%s"`,
		aid, from, to)
	reqBytes, _ := json.Marshal(map[string]interface{}{
		"jql":        jql,
		"fields":     []string{"key", "issuetype", "subtasks", "description"},
		"maxResults": 50,
	})

	body, exitCode := c.Fetch(context.Background(), "POST", "/rest/api/3/search/jql",
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
			} `json:"fields"`
		} `json:"issues"`
	}
	if err := json.Unmarshal(body, &searchResp); err != nil {
		return nil, fmt.Errorf("failed to parse issues response: %w", err)
	}

	results := make([]CreatedIssue, 0, len(searchResp.Issues))
	for _, issue := range searchResp.Issues {
		desc := extractTextFromADF(issue.Fields.Description)
		results = append(results, CreatedIssue{
			Key:          issue.Key,
			Type:         issue.Fields.IssueType.Name,
			SubtaskCount: len(issue.Fields.Subtasks),
			Description:  desc,
		})
	}
	return results, nil
}

// FetchUserChangelog fetches changelog entries authored by accountID on issues
// updated within [from, to].
func FetchUserChangelog(c *client.Client, accountID, from, to string) ([]ChangelogEntry, error) {
	aid := escapeJQLString(accountID)
	jql := fmt.Sprintf(
		`(reporter = "%s" OR assignee was "%s") AND updated >= "%s" AND updated <= "%s"`,
		aid, aid, from, to,
	)
	reqBytes, _ := json.Marshal(map[string]interface{}{
		"jql":        jql,
		"maxResults": 50,
	})

	body, exitCode := c.Fetch(context.Background(), "POST", "/rest/api/3/search/jql?expand=changelog",
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
	}
	if err := json.Unmarshal(body, &searchResp); err != nil {
		return nil, fmt.Errorf("failed to parse changelog response: %w", err)
	}

	var results []ChangelogEntry
	for _, issue := range searchResp.Issues {
		for _, history := range issue.Changelog.Histories {
			if history.Author.AccountID != accountID {
				continue
			}
			for _, item := range history.Items {
				results = append(results, ChangelogEntry{
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
	return results, nil
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

