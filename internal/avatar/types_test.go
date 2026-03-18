package avatar_test

import (
	"encoding/json"
	"testing"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/sofq/jira-cli/internal/avatar"
)

// TestExtractionJSONRoundTrip verifies that Extraction serialises to JSON and
// deserialises back with all fields intact.
func TestExtractionJSONRoundTrip(t *testing.T) {
	from := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	to := time.Date(2025, 3, 1, 0, 0, 0, 0, time.UTC)
	extractedAt := time.Date(2025, 3, 1, 12, 0, 0, 0, time.UTC)

	original := avatar.Extraction{
		Version: "1",
		Meta: avatar.ExtractionMeta{
			User:        "user@example.com",
			DisplayName: "Test User",
			ExtractedAt: extractedAt,
			Window: avatar.TimeWindow{
				From: from,
				To:   to,
			},
			DataPoints: avatar.DataPoints{
				Comments:    42,
				IssuesCreated: 10,
				Transitions: 20,
				Worklogs:    5,
			},
		},
		Writing: avatar.WritingAnalysis{
			Comments: avatar.CommentStats{
				AvgLengthWords:    55.5,
				MedianLengthWords: 45.0,
				LengthDist: avatar.LengthDist{
					ShortPct:  0.2,
					MediumPct: 0.5,
					LongPct:   0.3,
				},
				Formatting: avatar.FormattingStats{
					UsesBullets:    0.4,
					UsesCodeBlocks: 0.1,
					UsesHeadings:   0.05,
					UsesEmoji:      0.0,
					UsesMentions:   0.2,
				},
				Vocabulary: avatar.VocabularyStats{
					CommonPhrases: []string{"LGTM", "please review"},
					Jargon:        []string{"spike", "retro"},
					SignOffs:      []string{"Thanks"},
				},
				ToneSignals: avatar.ToneSignals{
					QuestionRatio:    0.15,
					ExclamationRatio: 0.05,
					FirstPersonRatio: 0.3,
					ImperativeRatio:  0.2,
				},
			},
			Descriptions: avatar.DescriptionStats{
				AvgLengthWords:    120.0,
				StructurePatterns: []string{"AC: ...", "Steps:"},
				Formatting: avatar.FormattingStats{
					UsesBullets:    0.7,
					UsesCodeBlocks: 0.2,
					UsesHeadings:   0.3,
					UsesEmoji:      0.0,
					UsesMentions:   0.1,
				},
			},
		},
		Workflow: avatar.WorkflowAnalysis{
			FieldPreferences: avatar.FieldPreferences{
				AlwaysSets:       []string{"priority", "labels"},
				RarelySets:       []string{"fix_version"},
				DefaultPriority:  "Medium",
				CommonLabels:     []string{"backend"},
				CommonComponents: []string{"auth"},
			},
			TransitionPatterns: avatar.TransitionPatterns{
				AvgTimeInStatus:         map[string]string{"In Progress": "2d", "Review": "1d"},
				CommonSequences:         [][]string{{"To Do", "In Progress", "Done"}},
				AssignsBeforeTransition: true,
			},
			IssueCreation: avatar.IssueCreation{
				TypesCreated:       map[string]float64{"Story": 5, "Bug": 3},
				AvgSubtasksPerStory: 2.5,
			},
		},
		Interaction: avatar.InteractionAnalysis{
			ResponsePatterns: avatar.ResponsePatterns{
				MedianReplyTime:       "2h",
				RepliesToOwnIssuesPct: 0.6,
				RepliesToOthersPct:    0.4,
				AvgThreadDepth:        3.0,
			},
			MentionHabits: avatar.MentionHabits{
				FrequentlyMentions: []string{"alice", "bob"},
				MentionContext:     []string{"for review", "FYI"},
			},
			EscalationSignals: avatar.EscalationSignals{
				BlockerKeywords:            []string{"blocked", "urgent"},
				EscalationPattern:          "add blocker label then ping manager",
				AvgCommentsBeforeEscalation: 3.0,
			},
			Collaboration: avatar.Collaboration{
				ActiveHours: avatar.ActiveHours{
					Start:    "09:00",
					End:      "18:00",
					Timezone: "UTC",
				},
				PeakActivityDays: []string{"Monday", "Tuesday"},
				WorklogHabits: avatar.WorklogHabits{
					LogsDaily:        true,
					AvgEntriesPerWeek: 5.0,
				},
			},
		},
		Examples: avatar.ExtractionExamples{
			Comments: []avatar.CommentExample{
				{Issue: "PROJ-1", Date: "2025-01-10", Text: "Looks good", Context: "review"},
			},
			Descriptions: []avatar.DescriptionExample{
				{Issue: "PROJ-2", Type: "Story", Text: "As a user I want..."},
			},
		},
	}

	data, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("json.Marshal: %v", err)
	}

	var decoded avatar.Extraction
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("json.Unmarshal: %v", err)
	}

	// Spot-check key fields.
	if decoded.Version != original.Version {
		t.Errorf("Version = %q, want %q", decoded.Version, original.Version)
	}
	if decoded.Meta.User != original.Meta.User {
		t.Errorf("Meta.User = %q, want %q", decoded.Meta.User, original.Meta.User)
	}
	if decoded.Meta.DataPoints.Comments != original.Meta.DataPoints.Comments {
		t.Errorf("DataPoints.Comments = %d, want %d", decoded.Meta.DataPoints.Comments, original.Meta.DataPoints.Comments)
	}
	if decoded.Writing.Comments.AvgLengthWords != original.Writing.Comments.AvgLengthWords {
		t.Errorf("Comments.AvgLengthWords = %f, want %f", decoded.Writing.Comments.AvgLengthWords, original.Writing.Comments.AvgLengthWords)
	}
	if len(decoded.Writing.Comments.Vocabulary.CommonPhrases) != 2 {
		t.Errorf("CommonPhrases len = %d, want 2", len(decoded.Writing.Comments.Vocabulary.CommonPhrases))
	}
	if decoded.Workflow.TransitionPatterns.AssignsBeforeTransition != true {
		t.Error("AssignsBeforeTransition should be true")
	}
	if decoded.Workflow.IssueCreation.AvgSubtasksPerStory != 2.5 {
		t.Errorf("AvgSubtasksPerStory = %f, want 2.5", decoded.Workflow.IssueCreation.AvgSubtasksPerStory)
	}
	if decoded.Interaction.Collaboration.ActiveHours.Timezone != "UTC" {
		t.Errorf("ActiveHours.Timezone = %q, want UTC", decoded.Interaction.Collaboration.ActiveHours.Timezone)
	}
	if len(decoded.Examples.Comments) != 1 {
		t.Errorf("Examples.Comments len = %d, want 1", len(decoded.Examples.Comments))
	}
	if len(decoded.Examples.Descriptions) != 1 {
		t.Errorf("Examples.Descriptions len = %d, want 1", len(decoded.Examples.Descriptions))
	}
}

// TestProfileYAMLRoundTrip verifies that Profile serialises to YAML and
// deserialises back with all fields intact.
func TestProfileYAMLRoundTrip(t *testing.T) {
	original := avatar.Profile{
		Version:     "1",
		User:        "user@example.com",
		DisplayName: "Test User",
		GeneratedAt: "2025-03-01T12:00:00Z",
		Engine:      "claude-3-5-sonnet",
		StyleGuide: avatar.StyleGuide{
			Writing:     "Be concise and direct.",
			Workflow:    "Always set priority.",
			Interaction: "Mention reviewers explicitly.",
		},
		Overrides: map[string]string{
			"greeting": "Hi there",
		},
		Defaults: avatar.ProfileDefaults{
			Priority:               "Medium",
			Labels:                 []string{"backend"},
			Components:             []string{"auth"},
			AssignSelfOnTransition: true,
		},
		Examples: []avatar.ProfileExample{
			{Context: "code review comment", Source: "PROJ-1", Text: "LGTM, merging."},
		},
	}

	data, err := yaml.Marshal(original)
	if err != nil {
		t.Fatalf("yaml.Marshal: %v", err)
	}

	var decoded avatar.Profile
	if err := yaml.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("yaml.Unmarshal: %v", err)
	}

	if decoded.Version != original.Version {
		t.Errorf("Version = %q, want %q", decoded.Version, original.Version)
	}
	if decoded.User != original.User {
		t.Errorf("User = %q, want %q", decoded.User, original.User)
	}
	if decoded.Engine != original.Engine {
		t.Errorf("Engine = %q, want %q", decoded.Engine, original.Engine)
	}
	if decoded.StyleGuide.Writing != original.StyleGuide.Writing {
		t.Errorf("StyleGuide.Writing = %q, want %q", decoded.StyleGuide.Writing, original.StyleGuide.Writing)
	}
	if decoded.Defaults.Priority != original.Defaults.Priority {
		t.Errorf("Defaults.Priority = %q, want %q", decoded.Defaults.Priority, original.Defaults.Priority)
	}
	if !decoded.Defaults.AssignSelfOnTransition {
		t.Error("Defaults.AssignSelfOnTransition should be true")
	}
	if len(decoded.Defaults.Labels) != 1 || decoded.Defaults.Labels[0] != "backend" {
		t.Errorf("Defaults.Labels = %v, want [backend]", decoded.Defaults.Labels)
	}
	if v, ok := decoded.Overrides["greeting"]; !ok || v != "Hi there" {
		t.Errorf("Overrides[greeting] = %q, want 'Hi there'", v)
	}
	if len(decoded.Examples) != 1 || decoded.Examples[0].Source != "PROJ-1" {
		t.Errorf("Examples = %v, want 1 item with Source=PROJ-1", decoded.Examples)
	}
}

// TestProfileJSONRoundTrip verifies that Profile also round-trips via JSON
// (Profile types carry both yaml and json tags).
func TestProfileJSONRoundTrip(t *testing.T) {
	original := avatar.Profile{
		Version:     "1",
		User:        "user@example.com",
		DisplayName: "Test User",
		GeneratedAt: "2025-03-01T12:00:00Z",
		Engine:      "claude-3-5-sonnet",
		StyleGuide: avatar.StyleGuide{
			Writing:     "Be concise.",
			Workflow:    "Set priority.",
			Interaction: "Mention reviewers.",
		},
		Defaults: avatar.ProfileDefaults{
			Priority: "High",
			Labels:   []string{"frontend"},
		},
	}

	data, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("json.Marshal: %v", err)
	}

	var decoded avatar.Profile
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("json.Unmarshal: %v", err)
	}

	if decoded.Engine != original.Engine {
		t.Errorf("Engine = %q, want %q", decoded.Engine, original.Engine)
	}
	if decoded.Defaults.Priority != original.Defaults.Priority {
		t.Errorf("Defaults.Priority = %q, want %q", decoded.Defaults.Priority, original.Defaults.Priority)
	}
}

// TestSharedInputTypes verifies that shared input types compile and can be used.
func TestSharedInputTypes(t *testing.T) {
	rc := avatar.RawComment{
		Issue:  "PROJ-1",
		Date:   "2025-01-10",
		Text:   "A comment",
		Author: "user@example.com",
	}
	if rc.Issue != "PROJ-1" {
		t.Errorf("RawComment.Issue = %q, want PROJ-1", rc.Issue)
	}

	ce := avatar.ChangelogEntry{
		Issue:     "PROJ-2",
		Timestamp: "2025-01-11T10:00:00Z",
		Author:    "user@example.com",
		Field:     "status",
		From:      "To Do",
		To:        "In Progress",
	}
	if ce.Field != "status" {
		t.Errorf("ChangelogEntry.Field = %q, want status", ce.Field)
	}

	iFields := avatar.IssueFields{
		Priority:   "High",
		Labels:     []string{"bug"},
		Components: []string{"auth"},
		FixVersion: "v1.2",
	}
	if iFields.Priority != "High" {
		t.Errorf("IssueFields.Priority = %q, want High", iFields.Priority)
	}

	ci := avatar.CreatedIssue{
		Key:          "PROJ-3",
		Type:         "Bug",
		SubtaskCount: 2,
		Description:  "A bug report",
	}
	if ci.Key != "PROJ-3" {
		t.Errorf("CreatedIssue.Key = %q, want PROJ-3", ci.Key)
	}

	cr := avatar.CommentRecord{
		IssueOwner:      "alice@example.com",
		Author:          "bob@example.com",
		Timestamp:       "2025-01-12T09:00:00Z",
		ParentTimestamp: "2025-01-12T08:50:00Z",
		Text:            "Reply text",
	}
	if cr.Author != "bob@example.com" {
		t.Errorf("CommentRecord.Author = %q, want bob@example.com", cr.Author)
	}
}
