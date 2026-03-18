package avatar

import (
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
	if opts.NoCache {
		t.Errorf("NoCache = true, want false")
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
// Extract — nil client
// ---------------------------------------------------------------------------

func TestExtract_NilClient(t *testing.T) {
	_, err := Extract(nil, DefaultExtractOptions())
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
			json.NewEncoder(w).Encode(map[string]string{
				"accountId":    accountID,
				"emailAddress": "test@example.com",
				"displayName":  displayName,
			})
		case "/rest/api/3/search/jql":
			json.NewEncoder(w).Encode(makeMockIssues())
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

	extraction, err := Extract(c, opts)
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
