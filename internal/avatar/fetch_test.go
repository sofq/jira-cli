package avatar

import (
	"bytes"
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
		json.NewEncoder(w).Encode(map[string]string{
			"accountId":    "abc123",
			"emailAddress": "user@example.com",
			"displayName":  "Test User",
		})
	}))
	defer srv.Close()

	c := newTestAvatarClient(srv.URL)
	user, err := ResolveUser(c, "")
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
		json.NewEncoder(w).Encode([]map[string]string{
			{
				"accountId":    "xyz789",
				"emailAddress": "other@example.com",
				"displayName":  "Other User",
			},
		})
	}))
	defer srv.Close()

	c := newTestAvatarClient(srv.URL)
	user, err := ResolveUser(c, "other@example.com")
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
		json.NewEncoder(w).Encode([]map[string]string{})
	}))
	defer srv.Close()

	c := newTestAvatarClient(srv.URL)
	_, err := ResolveUser(c, "nobody@example.com")
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
		json.NewEncoder(w).Encode(map[string]interface{}{
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
	comments, err := FetchUserComments(c, "abc123", "2025-01-01", "2025-01-31")
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
