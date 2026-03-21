package avatar

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/sofq/jira-cli/internal/client"
	"github.com/sofq/jira-cli/internal/config"
)

// newE2EClient creates a test client pointed at the given mock server URL.
func newE2EClient(serverURL string) *client.Client {
	var stdout, stderr bytes.Buffer
	return &client.Client{
		BaseURL:    serverURL,
		Auth:       config.AuthConfig{Type: "basic", Username: "u", Token: "t"},
		HTTPClient: &http.Client{},
		Stdout:     &stdout,
		Stderr:     &stderr,
		Paginate:   false, // mock doesn't paginate
	}
}

// makeADFNode builds a minimal ADF document JSON string for a given text.
func makeADFNode(text string) interface{} {
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

// buildE2EMockSearchResponse builds a realistic Jira search response with
// multiple issues, comments, and changelogs attributed to accountID.
func buildE2EMockSearchResponse(accountID string) interface{} {
	issues := []interface{}{
		// Issue 1 — Bug with several comments and a changelog entry
		map[string]interface{}{
			"key": "TEST-1",
			"fields": map[string]interface{}{
				"issuetype":   map[string]string{"name": "Bug"},
				"priority":    map[string]string{"name": "High"},
				"labels":      []string{"backend"},
				"components":  []interface{}{map[string]string{"name": "api"}},
				"status":      map[string]string{"name": "Done"},
				"subtasks":    []interface{}{},
				"description": makeADFNode("Login fails on mobile. Steps to reproduce: open app, tap login, observe error."),
				"comment": map[string]interface{}{
					"comments": []interface{}{
						map[string]interface{}{
							"author":  map[string]string{"accountId": accountID},
							"created": "2026-03-01T10:00:00.000+0000",
							"body":    makeADFNode("Fixed the auth timeout. Deployed to staging."),
						},
						map[string]interface{}{
							"author":  map[string]string{"accountId": accountID},
							"created": "2026-03-02T14:00:00.000+0000",
							"body":    makeADFNode("Looks good, merging now. Thanks for the review."),
						},
						map[string]interface{}{
							"author":  map[string]string{"accountId": "other-user"},
							"created": "2026-03-01T11:00:00.000+0000",
							"body":    makeADFNode("Can you also check the web flow?"),
						},
					},
				},
			},
			"changelog": map[string]interface{}{
				"histories": []interface{}{
					map[string]interface{}{
						"author":  map[string]string{"accountId": accountID},
						"created": "2026-03-01T09:00:00.000+0000",
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
		// Issue 2 — Story
		map[string]interface{}{
			"key": "TEST-2",
			"fields": map[string]interface{}{
				"issuetype":   map[string]string{"name": "Story"},
				"priority":    map[string]string{"name": "Medium"},
				"labels":      []string{"frontend", "ux"},
				"components":  []interface{}{map[string]string{"name": "ui"}},
				"status":      map[string]string{"name": "In Progress"},
				"subtasks":    []interface{}{},
				"description": makeADFNode("As a user, I want to reset my password via email so that I can recover my account."),
				"comment": map[string]interface{}{
					"comments": []interface{}{
						map[string]interface{}{
							"author":  map[string]string{"accountId": accountID},
							"created": "2026-03-03T09:00:00.000+0000",
							"body":    makeADFNode("Added the email template. Needs QA sign-off before merging."),
						},
						map[string]interface{}{
							"author":  map[string]string{"accountId": accountID},
							"created": "2026-03-04T15:00:00.000+0000",
							"body":    makeADFNode("QA passed. Deploying to production tonight."),
						},
					},
				},
			},
			"changelog": map[string]interface{}{
				"histories": []interface{}{
					map[string]interface{}{
						"author":  map[string]string{"accountId": accountID},
						"created": "2026-03-03T08:30:00.000+0000",
						"items": []interface{}{
							map[string]interface{}{
								"field":      "status",
								"fromString": "In Progress",
								"toString":   "Review",
							},
						},
					},
				},
			},
		},
		// Issue 3 — Task
		map[string]interface{}{
			"key": "TEST-3",
			"fields": map[string]interface{}{
				"issuetype":   map[string]string{"name": "Task"},
				"priority":    map[string]string{"name": "Low"},
				"labels":      []string{},
				"components":  []interface{}{},
				"status":      map[string]string{"name": "To Do"},
				"subtasks":    []interface{}{},
				"description": makeADFNode("Refactor the authentication module to reduce duplication."),
				"comment": map[string]interface{}{
					"comments": []interface{}{
						map[string]interface{}{
							"author":  map[string]string{"accountId": accountID},
							"created": "2026-03-05T11:00:00.000+0000",
							"body":    makeADFNode("Identified three areas of duplication. Will start with the token refresh logic."),
						},
						map[string]interface{}{
							"author":  map[string]string{"accountId": accountID},
							"created": "2026-03-06T10:00:00.000+0000",
							"body":    makeADFNode("Token refresh refactored. Tests passing. Moving on to session handling."),
						},
					},
				},
			},
			"changelog": map[string]interface{}{
				"histories": []interface{}{},
			},
		},
		// Issue 4 — Bug
		map[string]interface{}{
			"key": "TEST-4",
			"fields": map[string]interface{}{
				"issuetype":   map[string]string{"name": "Bug"},
				"priority":    map[string]string{"name": "Critical"},
				"labels":      []string{"production", "urgent"},
				"components":  []interface{}{map[string]string{"name": "api"}},
				"status":      map[string]string{"name": "Done"},
				"subtasks":    []interface{}{},
				"description": makeADFNode("API returns 500 when uploading files larger than 10MB."),
				"comment": map[string]interface{}{
					"comments": []interface{}{
						map[string]interface{}{
							"author":  map[string]string{"accountId": accountID},
							"created": "2026-03-07T09:00:00.000+0000",
							"body":    makeADFNode("Root cause found: missing file size validation in the upload handler. Fix is straightforward."),
						},
						map[string]interface{}{
							"author":  map[string]string{"accountId": accountID},
							"created": "2026-03-08T14:00:00.000+0000",
							"body":    makeADFNode("Patched and deployed hotfix to production. Monitoring for recurrence."),
						},
					},
				},
			},
			"changelog": map[string]interface{}{
				"histories": []interface{}{
					map[string]interface{}{
						"author":  map[string]string{"accountId": accountID},
						"created": "2026-03-07T08:00:00.000+0000",
						"items": []interface{}{
							map[string]interface{}{
								"field":      "status",
								"fromString": "To Do",
								"toString":   "In Progress",
							},
						},
					},
					map[string]interface{}{
						"author":  map[string]string{"accountId": accountID},
						"created": "2026-03-08T16:00:00.000+0000",
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
		// Issue 5 — Story
		map[string]interface{}{
			"key": "TEST-5",
			"fields": map[string]interface{}{
				"issuetype":   map[string]string{"name": "Story"},
				"priority":    map[string]string{"name": "Medium"},
				"labels":      []string{"backend"},
				"components":  []interface{}{map[string]string{"name": "database"}},
				"status":      map[string]string{"name": "Done"},
				"subtasks":    []interface{}{},
				"description": makeADFNode("Migrate user table to use UUIDs as primary keys instead of sequential integers."),
				"comment": map[string]interface{}{
					"comments": []interface{}{
						map[string]interface{}{
							"author":  map[string]string{"accountId": accountID},
							"created": "2026-03-09T10:00:00.000+0000",
							"body":    makeADFNode("Migration script written and tested on staging. Ready for review."),
						},
						map[string]interface{}{
							"author":  map[string]string{"accountId": accountID},
							"created": "2026-03-10T09:00:00.000+0000",
							"body":    makeADFNode("Migration completed successfully. All foreign key constraints updated."),
						},
					},
				},
			},
			"changelog": map[string]interface{}{
				"histories": []interface{}{
					map[string]interface{}{
						"author":  map[string]string{"accountId": accountID},
						"created": "2026-03-09T09:00:00.000+0000",
						"items": []interface{}{
							map[string]interface{}{
								"field":      "status",
								"fromString": "Review",
								"toString":   "Done",
							},
						},
					},
				},
			},
		},
	}

	return map[string]interface{}{
		"issues":        issues,
		"nextPageToken": nil,
		"isLast":        true,
	}
}

func TestE2E_FullPipeline(t *testing.T) {
	const accountID = "abc123"
	const displayName = "Test User"

	// -----------------------------------------------------------------------
	// 1. Set up mock HTTP server
	// -----------------------------------------------------------------------
	mockSearchResp := buildE2EMockSearchResponse(accountID)

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
			_ = json.NewEncoder(w).Encode(mockSearchResp)
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	c := newE2EClient(srv.URL)

	// -----------------------------------------------------------------------
	// 2. Run full Extract pipeline
	// -----------------------------------------------------------------------
	opts := ExtractOptions{
		UserFlag:    "",
		MinComments: 1,
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

	// -----------------------------------------------------------------------
	// 3. Verify extraction
	// -----------------------------------------------------------------------
	if extraction.Version != "1" {
		t.Errorf("Version = %q, want %q", extraction.Version, "1")
	}

	// Meta fields
	if extraction.Meta.User != accountID {
		t.Errorf("Meta.User = %q, want %q", extraction.Meta.User, accountID)
	}
	if extraction.Meta.DisplayName != displayName {
		t.Errorf("Meta.DisplayName = %q, want %q", extraction.Meta.DisplayName, displayName)
	}
	if extraction.Meta.ExtractedAt.IsZero() {
		t.Error("Meta.ExtractedAt should not be zero")
	}
	if extraction.Meta.Window.From.IsZero() || extraction.Meta.Window.To.IsZero() {
		t.Error("Meta.Window From/To should not be zero")
	}
	if extraction.Meta.DataPoints.Comments == 0 {
		t.Error("Meta.DataPoints.Comments should be non-zero")
	}

	// Writing.Comments stats
	if extraction.Writing.Comments.AvgLengthWords == 0 {
		t.Error("Writing.Comments.AvgLengthWords should be non-zero")
	}

	// Examples.Comments
	if len(extraction.Examples.Comments) == 0 {
		t.Error("Examples.Comments should be non-empty")
	}

	// -----------------------------------------------------------------------
	// 4. Run Build (local engine)
	// -----------------------------------------------------------------------
	profile, err := Build(extraction, nil, BuildOptions{Engine: "local"})
	if err != nil {
		t.Fatalf("Build returned error: %v", err)
	}
	if profile == nil {
		t.Fatal("Build returned nil profile")
	}

	if profile.Version != "1" {
		t.Errorf("Profile.Version = %q, want %q", profile.Version, "1")
	}
	if profile.Engine != "local" {
		t.Errorf("Profile.Engine = %q, want %q", profile.Engine, "local")
	}
	if profile.StyleGuide.Writing == "" {
		t.Error("StyleGuide.Writing should be non-empty")
	}
	if profile.StyleGuide.Workflow == "" {
		t.Error("StyleGuide.Workflow should be non-empty")
	}
	if profile.StyleGuide.Interaction == "" {
		t.Error("StyleGuide.Interaction should be non-empty")
	}

	// -----------------------------------------------------------------------
	// 5. Run FormatPrompt
	// -----------------------------------------------------------------------
	output := FormatPrompt(profile, PromptOptions{Format: "prose"})
	if output == "" {
		t.Error("FormatPrompt returned empty string")
	}
	if !strings.Contains(output, "Style Profile") {
		t.Errorf("FormatPrompt output missing %q, got: %s", "Style Profile", output)
	}
	if !strings.Contains(output, "Writing Style") {
		t.Errorf("FormatPrompt output missing %q, got: %s", "Writing Style", output)
	}

	// -----------------------------------------------------------------------
	// 6. Save/Load round-trip
	// -----------------------------------------------------------------------
	tmpDir, err := os.MkdirTemp("", "avatar-e2e-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer func() { _ = os.RemoveAll(tmpDir) }()

	// Save extraction
	if err := SaveExtraction(tmpDir, extraction); err != nil {
		t.Fatalf("SaveExtraction returned error: %v", err)
	}

	// Load extraction and verify equality
	loadedExtraction, err := LoadExtraction(tmpDir)
	if err != nil {
		t.Fatalf("LoadExtraction returned error: %v", err)
	}
	if loadedExtraction.Version != extraction.Version {
		t.Errorf("loaded Version = %q, want %q", loadedExtraction.Version, extraction.Version)
	}
	if loadedExtraction.Meta.User != extraction.Meta.User {
		t.Errorf("loaded Meta.User = %q, want %q", loadedExtraction.Meta.User, extraction.Meta.User)
	}
	if loadedExtraction.Meta.DisplayName != extraction.Meta.DisplayName {
		t.Errorf("loaded Meta.DisplayName = %q, want %q", loadedExtraction.Meta.DisplayName, extraction.Meta.DisplayName)
	}
	if loadedExtraction.Meta.DataPoints.Comments != extraction.Meta.DataPoints.Comments {
		t.Errorf("loaded DataPoints.Comments = %d, want %d",
			loadedExtraction.Meta.DataPoints.Comments, extraction.Meta.DataPoints.Comments)
	}

	// Save profile
	if err := SaveProfile(tmpDir, profile); err != nil {
		t.Fatalf("SaveProfile returned error: %v", err)
	}

	// Load profile and verify equality
	loadedProfile, err := LoadProfile(tmpDir)
	if err != nil {
		t.Fatalf("LoadProfile returned error: %v", err)
	}
	if loadedProfile.Version != profile.Version {
		t.Errorf("loaded Profile.Version = %q, want %q", loadedProfile.Version, profile.Version)
	}
	if loadedProfile.Engine != profile.Engine {
		t.Errorf("loaded Profile.Engine = %q, want %q", loadedProfile.Engine, profile.Engine)
	}
	if loadedProfile.StyleGuide.Writing != profile.StyleGuide.Writing {
		t.Errorf("loaded StyleGuide.Writing mismatch:\ngot:  %q\nwant: %q",
			loadedProfile.StyleGuide.Writing, profile.StyleGuide.Writing)
	}
	if loadedProfile.StyleGuide.Workflow != profile.StyleGuide.Workflow {
		t.Errorf("loaded StyleGuide.Workflow mismatch:\ngot:  %q\nwant: %q",
			loadedProfile.StyleGuide.Workflow, profile.StyleGuide.Workflow)
	}
	if loadedProfile.StyleGuide.Interaction != profile.StyleGuide.Interaction {
		t.Errorf("loaded StyleGuide.Interaction mismatch:\ngot:  %q\nwant: %q",
			loadedProfile.StyleGuide.Interaction, profile.StyleGuide.Interaction)
	}
}
