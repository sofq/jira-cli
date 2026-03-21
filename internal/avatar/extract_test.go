package avatar

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// ---------------------------------------------------------------------------
// parseWindowDuration
// ---------------------------------------------------------------------------

func TestParseWindowDuration(t *testing.T) {
	tests := []struct {
		input   string
		want    time.Duration
		wantErr bool
	}{
		{"2w", 14 * 24 * time.Hour, false},
		{"1m", 30 * 24 * time.Hour, false},
		{"3m", 90 * 24 * time.Hour, false},
		{"6m", 180 * 24 * time.Hour, false},
		{"", 180 * 24 * time.Hour, false},
		{"abc", 0, true},
		{"0w", 0, true},
		{"1x", 0, true},
		{"-1m", 0, true},
		{"w", 0, true},   // single char (len < 2)
		{"1", 0, true},   // single char, no unit
	}
	for _, tc := range tests {
		t.Run(tc.input, func(t *testing.T) {
			got, err := parseWindowDuration(tc.input)
			if tc.wantErr {
				if err == nil {
					t.Errorf("parseWindowDuration(%q): expected error, got nil", tc.input)
				}
				return
			}
			if err != nil {
				t.Fatalf("parseWindowDuration(%q): unexpected error: %v", tc.input, err)
			}
			if got != tc.want {
				t.Errorf("parseWindowDuration(%q) = %v, want %v", tc.input, got, tc.want)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// DefaultExtractOptions
// ---------------------------------------------------------------------------

func TestDefaultExtractOptions(t *testing.T) {
	opts := DefaultExtractOptions()
	if opts.MinComments != 50 {
		t.Errorf("MinComments = %d, want 50", opts.MinComments)
	}
	if opts.MinUpdates != 30 {
		t.Errorf("MinUpdates = %d, want 30", opts.MinUpdates)
	}
	if opts.MaxWindow != "6m" {
		t.Errorf("MaxWindow = %q, want %q", opts.MaxWindow, "6m")
	}
	if opts.UserFlag != "" {
		t.Errorf("UserFlag = %q, want empty string", opts.UserFlag)
	}
}

// ---------------------------------------------------------------------------
// InsufficientDataError
// ---------------------------------------------------------------------------

func TestInsufficientDataError(t *testing.T) {
	err := &InsufficientDataError{
		Found:    DataPoints{Comments: 5, IssuesCreated: 2},
		Required: DataPoints{Comments: 10, IssuesCreated: 5},
	}
	msg := err.Error()
	if msg == "" {
		t.Fatal("Error() returned empty string")
	}
	// Must mention found and required values
	if !strings.Contains(msg, "5") {
		t.Errorf("error message %q should contain found comments count (5)", msg)
	}
	if !strings.Contains(msg, "2") {
		t.Errorf("error message %q should contain found issues count (2)", msg)
	}
	if !strings.Contains(msg, "10") {
		t.Errorf("error message %q should contain required comments count (10)", msg)
	}
}

// ---------------------------------------------------------------------------
// Extract — JQL upper date bound includes today
// ---------------------------------------------------------------------------

func TestExtract_JQLUpperDateIncludesToday(t *testing.T) {
	const accountID = "user-abc"

	// Track the "to" dates sent in JQL queries.
	var jqlQueries []string

	makeADF := func(text string) interface{} {
		return map[string]interface{}{
			"type": "doc", "version": 1,
			"content": []interface{}{
				map[string]interface{}{
					"type": "paragraph",
					"content": []interface{}{
						map[string]interface{}{"type": "text", "text": text},
					},
				},
			},
		}
	}

	// Build enough data to pass hard minimums.
	comments := make([]interface{}, 15)
	for i := range comments {
		comments[i] = map[string]interface{}{
			"author":  map[string]string{"accountId": accountID},
			"created": "2026-03-18T10:00:00Z",
			"body":    makeADF(fmt.Sprintf("Sample comment number %d for testing.", i+1)),
		}
	}
	issues := make([]interface{}, 5)
	for i := range issues {
		issueComments := comments
		if i > 0 {
			issueComments = nil
		}
		issues[i] = map[string]interface{}{
			"key": fmt.Sprintf("PROJ-%d", i+1),
			"fields": map[string]interface{}{
				"issuetype":   map[string]string{"name": "Task"},
				"subtasks":    []interface{}{},
				"description": makeADF("Description text."),
				"comment":     map[string]interface{}{"comments": issueComments},
			},
			"changelog": map[string]interface{}{"histories": []interface{}{}},
		}
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/rest/api/3/myself":
			_ = json.NewEncoder(w).Encode(map[string]string{
				"accountId": accountID, "emailAddress": "t@t.com", "displayName": "T",
			})
		case "/rest/api/3/search/jql":
			// Capture the JQL from the request body.
			var body map[string]interface{}
			_ = json.NewDecoder(r.Body).Decode(&body)
			if jql, ok := body["jql"].(string); ok {
				jqlQueries = append(jqlQueries, jql)
			}
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"issues": issues})
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	c := newTestAvatarClient(srv.URL)
	_, err := Extract(context.Background(), c, ExtractOptions{
		MinComments: 1, MinUpdates: 1, MaxWindow: "2w",
	})
	if err != nil {
		t.Fatalf("Extract returned error: %v", err)
	}

	// The upper date bound in JQL should be tomorrow (today + 1 day) to
	// ensure issues updated/created today are included. Jira treats
	// date-only strings as exclusive upper bounds.
	today := time.Now().UTC().Format("2006-01-02")
	tomorrow := time.Now().UTC().Add(24 * time.Hour).Format("2006-01-02")

	for _, jql := range jqlQueries {
		// Check both "updated <=" and "created <=" patterns.
		for _, field := range []string{"updated", "created"} {
			pattern := fmt.Sprintf(`%s <= "%s"`, field, today)
			if strings.Contains(jql, pattern) {
				t.Errorf("JQL uses today as upper bound for %s (would exclude today's items):\n%s", field, jql)
			}
		}
		// At least one of updated/created should use tomorrow.
		hasUpdatedTomorrow := strings.Contains(jql, fmt.Sprintf(`updated <= "%s"`, tomorrow))
		hasCreatedTomorrow := strings.Contains(jql, fmt.Sprintf(`created <= "%s"`, tomorrow))
		if !hasUpdatedTomorrow && !hasCreatedTomorrow {
			t.Errorf("JQL should use tomorrow as upper bound, got:\n%s", jql)
		}
	}
}

// ---------------------------------------------------------------------------
// Extract — insufficient data
// ---------------------------------------------------------------------------

func TestExtract_InsufficientData(t *testing.T) {
	const accountID = "user-abc"

	// Server returns only 2 comments and 2 issues — below hard minimums (10 comments, 5 issues).
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/rest/api/3/myself":
			_ = json.NewEncoder(w).Encode(map[string]string{
				"accountId": accountID, "emailAddress": "t@t.com", "displayName": "T",
			})
		case "/rest/api/3/search/jql":
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"issues": []interface{}{
					map[string]interface{}{
						"key": "PROJ-1",
						"fields": map[string]interface{}{
							"issuetype":   map[string]string{"name": "Bug"},
							"subtasks":    []interface{}{},
							"description": nil,
							"comment": map[string]interface{}{
								"comments": []interface{}{
									map[string]interface{}{
										"author":  map[string]string{"accountId": accountID},
										"created": "2025-01-01T10:00:00Z",
										"body":    nil,
									},
								},
							},
						},
						"changelog": map[string]interface{}{"histories": []interface{}{}},
					},
				},
			})
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	c := newTestAvatarClient(srv.URL)
	_, err := Extract(context.Background(), c, ExtractOptions{
		MinComments: 50, MinUpdates: 30, MaxWindow: "1w",
	})
	if err == nil {
		t.Fatal("expected InsufficientDataError, got nil")
	}
	// Should be an InsufficientDataError
	var insErr *InsufficientDataError
	if !func() bool {
		insErr, _ = err.(*InsufficientDataError)
		return insErr != nil
	}() {
		t.Errorf("expected *InsufficientDataError, got %T: %v", err, err)
	}
}

// ---------------------------------------------------------------------------
// Extract — invalid MaxWindow
// ---------------------------------------------------------------------------

// ---------------------------------------------------------------------------
// Extract — fetch errors
// ---------------------------------------------------------------------------

func TestExtract_FetchUserFails(t *testing.T) {
	// Server returns 500 for /myself, causing ResolveUser to fail
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "server error", http.StatusInternalServerError)
	}))
	defer srv.Close()

	c := newTestAvatarClient(srv.URL)
	_, err := Extract(context.Background(), c, ExtractOptions{MaxWindow: "1w"})
	if err == nil {
		t.Fatal("expected error when ResolveUser fails, got nil")
	}
}

func TestExtract_FetchIssuesFails(t *testing.T) {
	const accountID = "user-abc"
	callCount := 0

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/rest/api/3/myself" {
			_ = json.NewEncoder(w).Encode(map[string]string{
				"accountId": accountID, "emailAddress": "t@t.com", "displayName": "T",
			})
			return
		}
		// First call (FetchUserComments) succeeds, second (FetchUserIssues) fails
		callCount++
		if callCount == 2 {
			http.Error(w, "server error", http.StatusInternalServerError)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]interface{}{"issues": []interface{}{}})
	}))
	defer srv.Close()

	c := newTestAvatarClient(srv.URL)
	_, err := Extract(context.Background(), c, ExtractOptions{MaxWindow: "1w"})
	if err == nil {
		t.Fatal("expected error when FetchUserIssues fails, got nil")
	}
}

func TestExtract_FetchChangelogFails(t *testing.T) {
	const accountID = "user-abc"
	callCount := 0

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/rest/api/3/myself" {
			_ = json.NewEncoder(w).Encode(map[string]string{
				"accountId": accountID, "emailAddress": "t@t.com", "displayName": "T",
			})
			return
		}
		// First two calls succeed, third (FetchUserChangelog) fails
		callCount++
		if callCount == 3 {
			http.Error(w, "server error", http.StatusInternalServerError)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]interface{}{"issues": []interface{}{}})
	}))
	defer srv.Close()

	c := newTestAvatarClient(srv.URL)
	_, err := Extract(context.Background(), c, ExtractOptions{MaxWindow: "1w"})
	if err == nil {
		t.Fatal("expected error when FetchUserChangelog fails, got nil")
	}
}

func TestExtract_FetchCommentsFails(t *testing.T) {
	const accountID = "user-abc"
	callCount := 0

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/rest/api/3/myself" {
			_ = json.NewEncoder(w).Encode(map[string]string{
				"accountId": accountID, "emailAddress": "t@t.com", "displayName": "T",
			})
			return
		}
		// First call succeeds (comments), second fails (issues or changelog)
		callCount++
		if callCount == 1 {
			// First search/jql call — return error
			http.Error(w, "server error", http.StatusInternalServerError)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]interface{}{"issues": []interface{}{}})
	}))
	defer srv.Close()

	c := newTestAvatarClient(srv.URL)
	_, err := Extract(context.Background(), c, ExtractOptions{MaxWindow: "1w"})
	if err == nil {
		t.Fatal("expected error when FetchUserComments fails, got nil")
	}
}

// ---------------------------------------------------------------------------
// Extract — adaptive window (window doubling)
// ---------------------------------------------------------------------------

func TestExtract_WindowDoubling(t *testing.T) {
	const accountID = "user-abc"

	// Build enough issues to eventually satisfy hard minimums but NOT soft minimums
	// on first iteration (2 week window). Use MinComments=100 to force doubling.
	makeADF := func(text string) interface{} {
		return map[string]interface{}{
			"type": "doc", "version": 1,
			"content": []interface{}{
				map[string]interface{}{
					"type": "paragraph",
					"content": []interface{}{
						map[string]interface{}{"type": "text", "text": text},
					},
				},
			},
		}
	}

	// Build 15 comments and 5 issues — enough to pass hard minimum (10 comments, 5 issues)
	// but NOT soft minimum (MinComments=100). The loop will double the window until max.
	comments := make([]interface{}, 15)
	for i := range comments {
		comments[i] = map[string]interface{}{
			"author":  map[string]string{"accountId": accountID},
			"created": "2025-01-15T10:00:00Z",
			"body":    makeADF(fmt.Sprintf("Comment %d for testing purposes.", i+1)),
		}
	}
	issues := make([]interface{}, 5)
	for i := range issues {
		issueComments := comments
		if i > 0 {
			issueComments = nil
		}
		issues[i] = map[string]interface{}{
			"key": fmt.Sprintf("PROJ-%d", i+1),
			"fields": map[string]interface{}{
				"issuetype":   map[string]string{"name": "Task"},
				"subtasks":    []interface{}{},
				"description": makeADF("Task description."),
				"comment":     map[string]interface{}{"comments": issueComments},
			},
			"changelog": map[string]interface{}{"histories": []interface{}{}},
		}
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/rest/api/3/myself":
			_ = json.NewEncoder(w).Encode(map[string]string{
				"accountId": accountID, "emailAddress": "t@t.com", "displayName": "T",
			})
		case "/rest/api/3/search/jql":
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"issues": issues})
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	c := newTestAvatarClient(srv.URL)
	// Use MinComments=100 to ensure soft minimum is never met, forcing window to max
	_, err := Extract(context.Background(), c, ExtractOptions{
		MinComments: 100, MinUpdates: 1, MaxWindow: "1m",
	})
	// Should return InsufficientDataError since we never reach 100 comments
	// (hard min is 10, we always get 15, so it should succeed with the data)
	// Actually: found.Comments=15 >= hardMinComments=10 AND found.IssuesCreated=5 >= hardMinIssues=5
	// So it should succeed even though soft minimum is not met
	if err != nil {
		t.Fatalf("Extract with window doubling returned error: %v", err)
	}
}

func TestExtract_InvalidMaxWindow(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/rest/api/3/myself" {
			_ = json.NewEncoder(w).Encode(map[string]string{
				"accountId": "u", "emailAddress": "u@u.com", "displayName": "U",
			})
		}
	}))
	defer srv.Close()

	c := newTestAvatarClient(srv.URL)
	_, err := Extract(context.Background(), c, ExtractOptions{MaxWindow: "bad-window"})
	if err == nil {
		t.Fatal("expected error for invalid MaxWindow, got nil")
	}
}

// ---------------------------------------------------------------------------
// Extract — nil client
// ---------------------------------------------------------------------------

func TestExtract_NilClient(t *testing.T) {
	_, err := Extract(context.Background(), nil, DefaultExtractOptions())
	if err == nil {
		t.Fatal("expected error for nil client, got nil")
	}
}

// ---------------------------------------------------------------------------
// Extract — mock server integration test
// ---------------------------------------------------------------------------

func TestExtract_WithMockServer(t *testing.T) {
	// ADF helper for building comment / description bodies
	makeADF := func(text string) interface{} {
		return map[string]interface{}{
			"type":    "doc",
			"version": 1,
			"content": []interface{}{
				map[string]interface{}{
					"type": "paragraph",
					"content": []interface{}{
						map[string]interface{}{
							"type": "text",
							"text": text,
						},
					},
				},
			},
		}
	}

	const accountID = "user-abc"
	const displayName = "Test User"

	// Build a mock search response that works for both comments and issues calls.
	// We need >=10 comments (hard min) and >=5 issues (hard min).
	makeMockIssues := func() interface{} {
		// 15 comments on the first issue, enough to pass hardMinComments=10.
		comments := make([]interface{}, 0, 15)
		for i := 0; i < 15; i++ {
			comments = append(comments, map[string]interface{}{
				"author":  map[string]string{"accountId": accountID},
				"created": "2025-01-15T10:00:00Z",
				"body":    makeADF("This is a sample comment for analysis purposes."),
			})
		}

		// 5 issues to satisfy hardMinIssues=5.
		issues := make([]interface{}, 0, 5)
		for i := 1; i <= 5; i++ {
			issueComments := comments
			if i > 1 {
				issueComments = nil // only first issue has comments
			}
			issues = append(issues, map[string]interface{}{
				"key": fmt.Sprintf("PROJ-%d", i),
				"fields": map[string]interface{}{
					"issuetype":   map[string]string{"name": "Story"},
					"subtasks":    []interface{}{},
					"description": makeADF("This is a description with acceptance criteria."),
					"comment": map[string]interface{}{
						"comments": issueComments,
					},
				},
				"changelog": map[string]interface{}{
					"histories": []interface{}{
						map[string]interface{}{
							"author":  map[string]string{"accountId": accountID},
							"created": "2025-01-16T09:00:00Z",
							"items": []interface{}{
								map[string]interface{}{
									"field":      "status",
									"fromString": "To Do",
									"toString":   "In Progress",
								},
							},
						},
					},
				},
			})
		}
		return map[string]interface{}{"issues": issues}
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/rest/api/3/myself":
			_ = json.NewEncoder(w).Encode(map[string]string{
				"accountId":    accountID,
				"emailAddress": "test@example.com",
				"displayName":  displayName,
			})
		case "/rest/api/3/search/jql":
			_ = json.NewEncoder(w).Encode(makeMockIssues())
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	c := newTestAvatarClient(srv.URL)

	// Use low minimums so we pass the hard-minimum check with the mock data.
	opts := ExtractOptions{
		UserFlag:    "",
		MinComments: 5,
		MinUpdates:  1,
		MaxWindow:   "6m",
	}

	extraction, err := Extract(context.Background(), c, opts)
	if err != nil {
		t.Fatalf("Extract returned error: %v", err)
	}
	if extraction == nil {
		t.Fatal("Extract returned nil extraction")
	}
	if extraction.Meta.User != accountID {
		t.Errorf("Meta.User = %q, want %q", extraction.Meta.User, accountID)
	}
	if extraction.Meta.DisplayName != displayName {
		t.Errorf("Meta.DisplayName = %q, want %q", extraction.Meta.DisplayName, displayName)
	}
	if extraction.Version == "" {
		t.Error("Version should not be empty")
	}
	// Writing section should be populated
	if extraction.Writing.Comments.AvgLengthWords == 0 {
		t.Error("Writing.Comments.AvgLengthWords should be non-zero")
	}
	// Meta data points should be recorded
	if extraction.Meta.DataPoints.Comments == 0 {
		t.Error("Meta.DataPoints.Comments should be non-zero")
	}
}
