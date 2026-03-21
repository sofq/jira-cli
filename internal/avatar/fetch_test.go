package avatar

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/sofq/jira-cli/internal/client"
	"github.com/sofq/jira-cli/internal/config"
)

func newTestAvatarClient(serverURL string) *client.Client {
	var stdout, stderr bytes.Buffer
	return &client.Client{
		BaseURL:    serverURL,
		Auth:       config.AuthConfig{Type: "basic", Username: "u", Token: "t"},
		HTTPClient: &http.Client{},
		Stdout:     &stdout,
		Stderr:     &stderr,
		Paginate:   true,
	}
}

func TestResolveCurrentUser(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/rest/api/3/myself" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]string{
			"accountId":    "abc123",
			"emailAddress": "user@example.com",
			"displayName":  "Test User",
		})
	}))
	defer srv.Close()

	c := newTestAvatarClient(srv.URL)
	user, err := ResolveUser(context.Background(), c, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if user.AccountID != "abc123" {
		t.Errorf("AccountID = %q, want %q", user.AccountID, "abc123")
	}
	if user.Email != "user@example.com" {
		t.Errorf("Email = %q, want %q", user.Email, "user@example.com")
	}
	if user.DisplayName != "Test User" {
		t.Errorf("DisplayName = %q, want %q", user.DisplayName, "Test User")
	}
}

func TestResolveUserByEmail(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/rest/api/3/user/search" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode([]map[string]string{
			{
				"accountId":    "xyz789",
				"emailAddress": "other@example.com",
				"displayName":  "Other User",
			},
		})
	}))
	defer srv.Close()

	c := newTestAvatarClient(srv.URL)
	user, err := ResolveUser(context.Background(), c, "other@example.com")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if user.AccountID != "xyz789" {
		t.Errorf("AccountID = %q, want %q", user.AccountID, "xyz789")
	}
	if user.Email != "other@example.com" {
		t.Errorf("Email = %q, want %q", user.Email, "other@example.com")
	}
}

func TestResolveUserNotFound(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode([]map[string]string{})
	}))
	defer srv.Close()

	c := newTestAvatarClient(srv.URL)
	_, err := ResolveUser(context.Background(), c, "nobody@example.com")
	if err == nil {
		t.Fatal("expected error for empty search results, got nil")
	}
}

func TestFetchUserComments(t *testing.T) {
	// ADF body for a comment
	adfBody := map[string]interface{}{
		"type":    "doc",
		"version": 1,
		"content": []interface{}{
			map[string]interface{}{
				"type": "paragraph",
				"content": []interface{}{
					map[string]interface{}{
						"type": "text",
						"text": "Hello world",
					},
				},
			},
		},
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"issues": []interface{}{
				map[string]interface{}{
					"key": "PROJ-1",
					"fields": map[string]interface{}{
						"comment": map[string]interface{}{
							"comments": []interface{}{
								map[string]interface{}{
									"author":  map[string]string{"accountId": "abc123"},
									"created": "2025-01-01T10:00:00.000+0000",
									"body":    adfBody,
								},
								// comment from different user — should be filtered out
								map[string]interface{}{
									"author":  map[string]string{"accountId": "other"},
									"created": "2025-01-01T11:00:00.000+0000",
									"body":    adfBody,
								},
							},
						},
					},
				},
			},
		})
	}))
	defer srv.Close()

	c := newTestAvatarClient(srv.URL)
	comments, err := FetchUserComments(context.Background(), c, "abc123", "2025-01-01", "2025-01-31")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(comments) != 1 {
		t.Fatalf("expected 1 comment, got %d", len(comments))
	}
	if comments[0].Issue != "PROJ-1" {
		t.Errorf("Issue = %q, want %q", comments[0].Issue, "PROJ-1")
	}
	if comments[0].Text != "Hello world" {
		t.Errorf("Text = %q, want %q", comments[0].Text, "Hello world")
	}
	if comments[0].Author != "abc123" {
		t.Errorf("Author = %q, want %q", comments[0].Author, "abc123")
	}
}

func TestFetchUserChangelog_FiltersOtherAuthors(t *testing.T) {
	const accountID = "user-abc"

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"issues": []interface{}{
				map[string]interface{}{
					"key": "PROJ-1",
					"changelog": map[string]interface{}{
						"histories": []interface{}{
							// History from accountID — should be included
							map[string]interface{}{
								"author":  map[string]string{"accountId": accountID},
								"created": "2025-01-15T10:00:00Z",
								"items": []interface{}{
									map[string]interface{}{
										"field":      "status",
										"fromString": "To Do",
										"toString":   "In Progress",
									},
								},
							},
							// History from a different user — should be filtered out (exercises the continue path)
							map[string]interface{}{
								"author":  map[string]string{"accountId": "other-user"},
								"created": "2025-01-15T11:00:00Z",
								"items": []interface{}{
									map[string]interface{}{
										"field":      "status",
										"fromString": "In Progress",
										"toString":   "Done",
									},
								},
							},
						},
					},
				},
			},
		})
	}))
	defer srv.Close()

	c := newTestAvatarClient(srv.URL)
	entries, err := FetchUserChangelog(context.Background(), c, accountID, "2025-01-01", "2025-01-31")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Only 1 entry from accountID should be returned
	if len(entries) != 1 {
		t.Fatalf("expected 1 changelog entry (other-user filtered), got %d", len(entries))
	}
	if entries[0].Author != accountID {
		t.Errorf("expected author %q, got %q", accountID, entries[0].Author)
	}
}

func TestFetchUserChangelog_ExpandAsQueryParam(t *testing.T) {
	const accountID = "abc123"

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		// Verify expand is passed as a query parameter, not in the body.
		if got := r.URL.Query().Get("expand"); got != "changelog" {
			t.Errorf("expected expand=changelog as query param, got %q", got)
		}

		// Verify expand is NOT in the request body (which the new
		// /search/jql endpoint rejects with 400).
		var body map[string]interface{}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("failed to decode request body: %v", err)
		}
		if _, ok := body["expand"]; ok {
			t.Error("expand should not be in the request body — the new /search/jql endpoint rejects it")
		}

		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"issues": []interface{}{
				map[string]interface{}{
					"key": "PROJ-1",
					"changelog": map[string]interface{}{
						"histories": []interface{}{
							map[string]interface{}{
								"author":  map[string]string{"accountId": accountID},
								"created": "2025-01-15T10:00:00Z",
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
				},
			},
		})
	}))
	defer srv.Close()

	c := newTestAvatarClient(srv.URL)
	entries, err := FetchUserChangelog(context.Background(), c, accountID, "2025-01-01", "2025-01-31")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("expected 1 changelog entry, got %d", len(entries))
	}
	if entries[0].Field != "status" {
		t.Errorf("Field = %q, want %q", entries[0].Field, "status")
	}
	if entries[0].From != "To Do" {
		t.Errorf("From = %q, want %q", entries[0].From, "To Do")
	}
	if entries[0].To != "In Progress" {
		t.Errorf("To = %q, want %q", entries[0].To, "In Progress")
	}
}

func TestResolveUser_Myself_ServerError(t *testing.T) {
	// Server returns 500 for /myself → exitCode != 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "internal server error", http.StatusInternalServerError)
	}))
	defer srv.Close()

	c := newTestAvatarClient(srv.URL)
	_, err := ResolveUser(context.Background(), c, "")
	if err == nil {
		t.Fatal("expected error for 500 response from /myself, got nil")
	}
}

func TestResolveUser_Search_ServerError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "internal server error", http.StatusInternalServerError)
	}))
	defer srv.Close()

	c := newTestAvatarClient(srv.URL)
	_, err := ResolveUser(context.Background(), c, "test@example.com")
	if err == nil {
		t.Fatal("expected error for 500 response from user search, got nil")
	}
}

func TestFetchUserComments_ServerError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "internal server error", http.StatusInternalServerError)
	}))
	defer srv.Close()

	c := newTestAvatarClient(srv.URL)
	_, err := FetchUserComments(context.Background(), c, "abc123", "2025-01-01", "2025-01-31")
	if err == nil {
		t.Fatal("expected error for 500 response, got nil")
	}
}

func TestFetchUserIssues_ServerError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "internal server error", http.StatusInternalServerError)
	}))
	defer srv.Close()

	c := newTestAvatarClient(srv.URL)
	_, err := FetchUserIssues(context.Background(), c, "abc123", "2025-01-01", "2025-01-31")
	if err == nil {
		t.Fatal("expected error for 500 response, got nil")
	}
}

func TestFetchUserChangelog_ServerError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "internal server error", http.StatusInternalServerError)
	}))
	defer srv.Close()

	c := newTestAvatarClient(srv.URL)
	_, err := FetchUserChangelog(context.Background(), c, "abc123", "2025-01-01", "2025-01-31")
	if err == nil {
		t.Fatal("expected error for 500 response, got nil")
	}
}

func TestResolveUser_Myself_ParseError(t *testing.T) {
	// Server returns invalid JSON for /myself
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`not-valid-json`))
	}))
	defer srv.Close()

	c := newTestAvatarClient(srv.URL)
	_, err := ResolveUser(context.Background(), c, "")
	if err == nil {
		t.Fatal("expected error for invalid JSON response from /myself, got nil")
	}
}

func TestResolveUser_Search_ParseError(t *testing.T) {
	// Server returns invalid JSON for user search
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`not-valid-json`))
	}))
	defer srv.Close()

	c := newTestAvatarClient(srv.URL)
	_, err := ResolveUser(context.Background(), c, "test@example.com")
	if err == nil {
		t.Fatal("expected error for invalid JSON response from user search, got nil")
	}
}

func TestFetchUserComments_ParseError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`not-valid-json`))
	}))
	defer srv.Close()

	c := newTestAvatarClient(srv.URL)
	_, err := FetchUserComments(context.Background(), c, "abc123", "2025-01-01", "2025-01-31")
	if err == nil {
		t.Fatal("expected error for invalid JSON response, got nil")
	}
}

func TestFetchUserIssues_ParseError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`not-valid-json`))
	}))
	defer srv.Close()

	c := newTestAvatarClient(srv.URL)
	_, err := FetchUserIssues(context.Background(), c, "abc123", "2025-01-01", "2025-01-31")
	if err == nil {
		t.Fatal("expected error for invalid JSON response, got nil")
	}
}

func TestFetchUserChangelog_ParseError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`not-valid-json`))
	}))
	defer srv.Close()

	c := newTestAvatarClient(srv.URL)
	_, err := FetchUserChangelog(context.Background(), c, "abc123", "2025-01-01", "2025-01-31")
	if err == nil {
		t.Fatal("expected error for invalid JSON response, got nil")
	}
}

func TestExtractNodeText_TextNode(t *testing.T) {
	// Direct text node
	raw := json.RawMessage(`{"type":"text","text":"hello"}`)
	got := extractNodeText(raw)
	if got != "hello" {
		t.Errorf("extractNodeText(text node) = %q, want %q", got, "hello")
	}
}

func TestExtractNodeText_InvalidJSON(t *testing.T) {
	raw := json.RawMessage(`not-json`)
	got := extractNodeText(raw)
	if got != "" {
		t.Errorf("extractNodeText(invalid JSON) = %q, want empty", got)
	}
}

func TestExtractNodeText_EmptyContent(t *testing.T) {
	// Node with no content and not a text type
	raw := json.RawMessage(`{"type":"hardBreak"}`)
	got := extractNodeText(raw)
	if got != "" {
		t.Errorf("extractNodeText(hardBreak) = %q, want empty", got)
	}
}

func TestExtractNodeText_NestedParagraph(t *testing.T) {
	raw := json.RawMessage(`{
		"type": "paragraph",
		"content": [
			{"type": "text", "text": "foo"},
			{"type": "text", "text": "bar"}
		]
	}`)
	got := extractNodeText(raw)
	if got != "foobar" {
		t.Errorf("extractNodeText(paragraph) = %q, want %q", got, "foobar")
	}
}

func TestExtractTextFromADF_EmptyInput(t *testing.T) {
	// Empty RawMessage (zero length)
	got := extractTextFromADF(json.RawMessage{})
	if got != "" {
		t.Errorf("extractTextFromADF(empty) = %q, want empty", got)
	}
}

func TestExtractTextFromADF_NoContent(t *testing.T) {
	// Valid doc but content with no text nodes
	raw := json.RawMessage(`{"type":"doc","version":1,"content":[{"type":"hardBreak"}]}`)
	got := extractTextFromADF(raw)
	// No paragraphs extracted → falls back to raw string
	if got == "" {
		t.Error("expected non-empty fallback for doc with no text content")
	}
}

func TestExtractTextFromADF(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "empty",
			input: `null`,
			want:  "",
		},
		{
			name: "simple paragraph",
			input: `{
				"type": "doc",
				"version": 1,
				"content": [
					{
						"type": "paragraph",
						"content": [
							{"type": "text", "text": "Hello "},
							{"type": "text", "text": "world"}
						]
					}
				]
			}`,
			want: "Hello world",
		},
		{
			name: "multiple paragraphs",
			input: `{
				"type": "doc",
				"version": 1,
				"content": [
					{
						"type": "paragraph",
						"content": [{"type": "text", "text": "First"}]
					},
					{
						"type": "paragraph",
						"content": [{"type": "text", "text": "Second"}]
					}
				]
			}`,
			want: "First\nSecond",
		},
		{
			name:  "invalid json fallback",
			input: `not-json`,
			want:  "not-json",
		},
		{
			name: "nested content",
			input: `{
				"type": "doc",
				"version": 1,
				"content": [
					{
						"type": "bulletList",
						"content": [
							{
								"type": "listItem",
								"content": [
									{
										"type": "paragraph",
										"content": [{"type": "text", "text": "item one"}]
									}
								]
							}
						]
					}
				]
			}`,
			want: "item one",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := extractTextFromADF(json.RawMessage(tc.input))
			if got != tc.want {
				t.Errorf("extractTextFromADF() = %q, want %q", got, tc.want)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Pagination tests
// ---------------------------------------------------------------------------

func TestFetchUserComments_Pagination(t *testing.T) {
	const accountID = "abc123"
	callCount := 0

	adfBody := map[string]interface{}{
		"type": "doc", "version": 1,
		"content": []interface{}{
			map[string]interface{}{
				"type": "paragraph",
				"content": []interface{}{
					map[string]interface{}{"type": "text", "text": "page comment"},
				},
			},
		},
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		w.Header().Set("Content-Type", "application/json")
		resp := map[string]interface{}{
			"issues": []interface{}{
				map[string]interface{}{
					"key": "PROJ-1",
					"fields": map[string]interface{}{
						"comment": map[string]interface{}{
							"comments": []interface{}{
								map[string]interface{}{
									"author":  map[string]string{"accountId": accountID},
									"created": "2025-01-01T10:00:00.000+0000",
									"body":    adfBody,
								},
							},
						},
					},
				},
			},
		}
		if callCount == 1 {
			resp["nextPageToken"] = "token-page-2"
		}
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	c := newTestAvatarClient(srv.URL)
	comments, err := FetchUserComments(context.Background(), c, accountID, "2025-01-01", "2025-01-31")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if callCount != 2 {
		t.Errorf("expected 2 HTTP calls for pagination, got %d", callCount)
	}
	if len(comments) != 2 {
		t.Fatalf("expected 2 comments (one per page), got %d", len(comments))
	}
}

func TestFetchUserIssues_Pagination(t *testing.T) {
	const accountID = "abc123"
	callCount := 0

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		w.Header().Set("Content-Type", "application/json")
		resp := map[string]interface{}{
			"issues": []interface{}{
				map[string]interface{}{
					"key": "PROJ-1",
					"fields": map[string]interface{}{
						"issuetype":   map[string]string{"name": "Bug"},
						"subtasks":    []interface{}{},
						"description": nil,
						"priority":    nil,
						"labels":      []string{},
						"components":  []interface{}{},
						"fixVersions": []interface{}{},
					},
				},
			},
		}
		if callCount == 1 {
			resp["nextPageToken"] = "token-page-2"
		}
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	c := newTestAvatarClient(srv.URL)
	issues, err := FetchUserIssues(context.Background(), c, accountID, "2025-01-01", "2025-01-31")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if callCount != 2 {
		t.Errorf("expected 2 HTTP calls for pagination, got %d", callCount)
	}
	if len(issues) != 2 {
		t.Fatalf("expected 2 issues (one per page), got %d", len(issues))
	}
}

func TestFetchUserChangelog_Pagination(t *testing.T) {
	const accountID = "abc123"
	callCount := 0

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		w.Header().Set("Content-Type", "application/json")
		resp := map[string]interface{}{
			"issues": []interface{}{
				map[string]interface{}{
					"key": "PROJ-1",
					"changelog": map[string]interface{}{
						"histories": []interface{}{
							map[string]interface{}{
								"author":  map[string]string{"accountId": accountID},
								"created": "2025-01-15T10:00:00Z",
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
				},
			},
		}
		if callCount == 1 {
			resp["nextPageToken"] = "token-page-2"
		}
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	c := newTestAvatarClient(srv.URL)
	entries, err := FetchUserChangelog(context.Background(), c, accountID, "2025-01-01", "2025-01-31")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if callCount != 2 {
		t.Errorf("expected 2 HTTP calls for pagination, got %d", callCount)
	}
	if len(entries) != 2 {
		t.Fatalf("expected 2 changelog entries (one per page), got %d", len(entries))
	}
}

func TestFetchUserWorklogs_Pagination(t *testing.T) {
	const accountID = "abc123"
	callCount := 0

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		w.Header().Set("Content-Type", "application/json")
		resp := map[string]interface{}{
			"issues": []interface{}{
				map[string]interface{}{
					"fields": map[string]interface{}{
						"worklog": map[string]interface{}{
							"worklogs": []interface{}{
								map[string]interface{}{
									"author":           map[string]string{"accountId": accountID},
									"started":          "2025-01-15T10:00:00.000+0000",
									"timeSpentSeconds": 3600,
								},
							},
						},
					},
				},
			},
		}
		if callCount == 1 {
			resp["nextPageToken"] = "token-page-2"
		}
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	c := newTestAvatarClient(srv.URL)
	worklogs, err := FetchUserWorklogs(context.Background(), c, accountID, "2025-01-01", "2025-01-31")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if callCount != 2 {
		t.Errorf("expected 2 HTTP calls for pagination, got %d", callCount)
	}
	if len(worklogs) != 2 {
		t.Fatalf("expected 2 worklog entries (one per page), got %d", len(worklogs))
	}
}

// ---------------------------------------------------------------------------
// FetchUserWorklogs — happy path and error tests
// ---------------------------------------------------------------------------

func TestFetchUserWorklogs_Basic(t *testing.T) {
	const accountID = "abc123"

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"issues": []interface{}{
				map[string]interface{}{
					"fields": map[string]interface{}{
						"worklog": map[string]interface{}{
							"worklogs": []interface{}{
								map[string]interface{}{
									"author":           map[string]string{"accountId": accountID},
									"started":          "2025-01-15T10:00:00.000+0000",
									"timeSpentSeconds": 3600,
								},
								// Different user — filtered out.
								map[string]interface{}{
									"author":           map[string]string{"accountId": "other"},
									"started":          "2025-01-15T11:00:00.000+0000",
									"timeSpentSeconds": 1800,
								},
							},
						},
					},
				},
			},
		})
	}))
	defer srv.Close()

	c := newTestAvatarClient(srv.URL)
	worklogs, err := FetchUserWorklogs(context.Background(), c, accountID, "2025-01-01", "2025-01-31")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(worklogs) != 1 {
		t.Fatalf("expected 1 worklog (other-user filtered), got %d", len(worklogs))
	}
	if worklogs[0].DurationSeconds != 3600 {
		t.Errorf("DurationSeconds = %d, want 3600", worklogs[0].DurationSeconds)
	}
	if worklogs[0].Date != "2025-01-15" {
		t.Errorf("Date = %q, want %q", worklogs[0].Date, "2025-01-15")
	}
}

func TestFetchUserWorklogs_DateTruncation(t *testing.T) {
	const accountID = "abc123"

	tests := []struct {
		name     string
		started  string
		wantDate string
	}{
		{"full ISO timestamp", "2025-06-20T14:30:00.000+0000", "2025-06-20"},
		{"exactly 10 chars", "2025-06-20", "2025-06-20"},
		{"short value kept as-is", "2025-06", "2025-06"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				_ = json.NewEncoder(w).Encode(map[string]interface{}{
					"issues": []interface{}{
						map[string]interface{}{
							"fields": map[string]interface{}{
								"worklog": map[string]interface{}{
									"worklogs": []interface{}{
										map[string]interface{}{
											"author":           map[string]string{"accountId": accountID},
											"started":          tc.started,
											"timeSpentSeconds": 900,
										},
									},
								},
							},
						},
					},
				})
			}))
			defer srv.Close()

			c := newTestAvatarClient(srv.URL)
			worklogs, err := FetchUserWorklogs(context.Background(), c, accountID, "2025-01-01", "2025-12-31")
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if len(worklogs) != 1 {
				t.Fatalf("expected 1 worklog, got %d", len(worklogs))
			}
			if worklogs[0].Date != tc.wantDate {
				t.Errorf("Date = %q, want %q", worklogs[0].Date, tc.wantDate)
			}
		})
	}
}

func TestFetchUserWorklogs_ServerError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "internal server error", http.StatusInternalServerError)
	}))
	defer srv.Close()

	c := newTestAvatarClient(srv.URL)
	_, err := FetchUserWorklogs(context.Background(), c, "abc123", "2025-01-01", "2025-01-31")
	if err == nil {
		t.Fatal("expected error for 500 response, got nil")
	}
}

func TestFetchUserWorklogs_ParseError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`not-valid-json`))
	}))
	defer srv.Close()

	c := newTestAvatarClient(srv.URL)
	_, err := FetchUserWorklogs(context.Background(), c, "abc123", "2025-01-01", "2025-01-31")
	if err == nil {
		t.Fatal("expected error for invalid JSON response, got nil")
	}
}

// ---------------------------------------------------------------------------
// FetchUserIssues — components and fixVersions branches
// ---------------------------------------------------------------------------

func TestFetchUserIssues_ComponentsAndFixVersions(t *testing.T) {
	const accountID = "abc123"

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"issues": []interface{}{
				map[string]interface{}{
					"key": "PROJ-1",
					"fields": map[string]interface{}{
						"issuetype":   map[string]string{"name": "Story"},
						"subtasks":    []interface{}{},
						"description": nil,
						"priority":    nil,
						"labels":      []string{"backend"},
						"components": []interface{}{
							map[string]string{"name": "auth"},
							map[string]string{"name": ""},
						},
						"fixVersions": []interface{}{
							map[string]string{"name": "v1.2.0"},
							map[string]string{"name": "v1.3.0"},
						},
					},
				},
			},
		})
	}))
	defer srv.Close()

	c := newTestAvatarClient(srv.URL)
	issues, err := FetchUserIssues(context.Background(), c, accountID, "2025-01-01", "2025-01-31")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(issues) != 1 {
		t.Fatalf("expected 1 issue, got %d", len(issues))
	}
	if len(issues[0].Components) != 1 || issues[0].Components[0] != "auth" {
		t.Errorf("Components = %v, want [auth]", issues[0].Components)
	}
	if issues[0].FixVersion != "v1.2.0" {
		t.Errorf("FixVersion = %q, want %q", issues[0].FixVersion, "v1.2.0")
	}
}
