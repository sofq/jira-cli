package avatar

import (
	"strings"
	"testing"

	"github.com/sofq/jira-cli/internal/config"
)

func makeTestExtraction() *Extraction {
	return &Extraction{
		Version: "1",
		Meta: ExtractionMeta{
			User:        "john.doe",
			DisplayName: "John Doe",
		},
		Writing: WritingAnalysis{
			Comments: CommentStats{
				AvgLengthWords:    45.0,
				MedianLengthWords: 38.0,
				LengthDist: LengthDist{
					ShortPct:  0.2,
					MediumPct: 0.6,
					LongPct:   0.2,
				},
				Formatting: FormattingStats{
					UsesBullets:    0.6,
					UsesCodeBlocks: 0.15,
					UsesEmoji:      0.03,
					UsesHeadings:   0.05,
					UsesMentions:   0.3,
				},
				Vocabulary: VocabularyStats{
					CommonPhrases: []string{"looks good", "see below"},
					SignOffs:      []string{"Thanks", "Cheers"},
				},
			},
			Descriptions: DescriptionStats{
				AvgLengthWords:    80.0,
				StructurePatterns: []string{"Steps to reproduce", "Expected vs Actual"},
				Formatting: FormattingStats{
					UsesBullets:    0.7,
					UsesCodeBlocks: 0.4,
				},
			},
		},
		Workflow: WorkflowAnalysis{
			FieldPreferences: FieldPreferences{
				AlwaysSets:      []string{"priority", "labels"},
				DefaultPriority: "Medium",
				CommonLabels:    []string{"backend", "auth"},
			},
			TransitionPatterns: TransitionPatterns{
				CommonSequences:         [][]string{{"To Do", "In Progress", "Done"}},
				AssignsBeforeTransition: true,
			},
			IssueCreation: IssueCreation{
				TypesCreated: map[string]float64{"Bug": 0.5, "Story": 0.3, "Task": 0.2},
			},
		},
		Interaction: InteractionAnalysis{
			ResponsePatterns: ResponsePatterns{
				MedianReplyTime:       "2h",
				RepliesToOwnIssuesPct: 0.6,
				RepliesToOthersPct:    0.4,
			},
			MentionHabits: MentionHabits{
				FrequentlyMentions: []string{"alice", "bob"},
			},
			Collaboration: Collaboration{
				ActiveHours: ActiveHours{
					Start:    "09:00",
					End:      "18:00",
					Timezone: "UTC",
				},
				PeakActivityDays: []string{"Tuesday", "Wednesday"},
			},
		},
		Examples: ExtractionExamples{
			Comments: []CommentExample{
				{Issue: "PROJ-1", Date: "2025-01-01", Text: "Looks good to me.", Context: "review"},
				{Issue: "PROJ-2", Date: "2025-01-02", Text: "See below for details.", Context: "question"},
			},
		},
	}
}

func TestBuildLocal(t *testing.T) {
	extraction := makeTestExtraction()
	overrides := []string{"tone=formal", "sign_off=Best regards"}

	profile, err := BuildLocal(extraction, overrides)
	if err != nil {
		t.Fatalf("BuildLocal returned error: %v", err)
	}

	// Version and engine
	if profile.Version != "1" {
		t.Errorf("expected Version=1, got %q", profile.Version)
	}
	if profile.Engine != "local" {
		t.Errorf("expected Engine=local, got %q", profile.Engine)
	}

	// User fields
	if profile.User != "john.doe" {
		t.Errorf("expected User=john.doe, got %q", profile.User)
	}
	if profile.DisplayName != "John Doe" {
		t.Errorf("expected DisplayName=John Doe, got %q", profile.DisplayName)
	}

	// GeneratedAt is set
	if profile.GeneratedAt == "" {
		t.Error("expected GeneratedAt to be set")
	}

	// StyleGuide sections are non-empty and mention the user's name
	if profile.StyleGuide.Writing == "" {
		t.Error("expected StyleGuide.Writing to be non-empty")
	}
	if !strings.Contains(profile.StyleGuide.Writing, "John Doe") {
		t.Errorf("expected StyleGuide.Writing to mention 'John Doe', got: %s", profile.StyleGuide.Writing)
	}

	if profile.StyleGuide.Workflow == "" {
		t.Error("expected StyleGuide.Workflow to be non-empty")
	}
	if !strings.Contains(profile.StyleGuide.Workflow, "John Doe") {
		t.Errorf("expected StyleGuide.Workflow to mention 'John Doe', got: %s", profile.StyleGuide.Workflow)
	}

	if profile.StyleGuide.Interaction == "" {
		t.Error("expected StyleGuide.Interaction to be non-empty")
	}
	if !strings.Contains(profile.StyleGuide.Interaction, "John Doe") {
		t.Errorf("expected StyleGuide.Interaction to mention 'John Doe', got: %s", profile.StyleGuide.Interaction)
	}

	// Writing section mentions bullet points (ratio > 0.5)
	if !strings.Contains(profile.StyleGuide.Writing, "bullet") {
		t.Errorf("expected Writing section to mention bullets (ratio=0.6), got: %s", profile.StyleGuide.Writing)
	}

	// Writing section mentions emoji rarity (ratio < 0.05)
	if !strings.Contains(strings.ToLower(profile.StyleGuide.Writing), "emoji") {
		t.Errorf("expected Writing section to mention emoji rarity, got: %s", profile.StyleGuide.Writing)
	}

	// Writing section mentions code blocks (ratio > 0.1)
	if !strings.Contains(strings.ToLower(profile.StyleGuide.Writing), "code block") {
		t.Errorf("expected Writing section to mention code blocks (ratio=0.15), got: %s", profile.StyleGuide.Writing)
	}

	// Writing section mentions sign-offs
	if !strings.Contains(profile.StyleGuide.Writing, "Thanks") {
		t.Errorf("expected Writing section to mention sign-off 'Thanks', got: %s", profile.StyleGuide.Writing)
	}

	// Defaults from extraction
	if profile.Defaults.Priority != "Medium" {
		t.Errorf("expected Defaults.Priority=Medium, got %q", profile.Defaults.Priority)
	}
	if len(profile.Defaults.Labels) == 0 {
		t.Error("expected Defaults.Labels to be populated")
	}

	// Examples converted
	if len(profile.Examples) != 2 {
		t.Errorf("expected 2 examples, got %d", len(profile.Examples))
	}

	// Overrides passed through
	if len(profile.Overrides) != 2 {
		t.Errorf("expected 2 overrides, got %d", len(profile.Overrides))
	}
	if profile.Overrides["tone"] != "formal" {
		t.Errorf("expected overrides[tone]=formal, got %q", profile.Overrides["tone"])
	}
}

func TestBuildLocal_EmptyExtraction(t *testing.T) {
	extraction := &Extraction{
		Meta: ExtractionMeta{
			DisplayName: "Empty User",
			User:        "empty.user",
		},
	}

	profile, err := BuildLocal(extraction, nil)
	if err != nil {
		t.Fatalf("BuildLocal returned error for empty extraction: %v", err)
	}

	if profile.Version != "1" {
		t.Errorf("expected Version=1, got %q", profile.Version)
	}
	if profile.Engine != "local" {
		t.Errorf("expected Engine=local, got %q", profile.Engine)
	}
	if profile.StyleGuide.Writing == "" {
		t.Error("expected non-empty Writing section even for empty extraction")
	}
	if profile.StyleGuide.Workflow == "" {
		t.Error("expected non-empty Workflow section even for empty extraction")
	}
	if profile.StyleGuide.Interaction == "" {
		t.Error("expected non-empty Interaction section even for empty extraction")
	}
	if profile.Overrides != nil {
		t.Errorf("expected nil overrides for empty input, got %v", profile.Overrides)
	}
}

func TestBuild_DefaultsToLocal(t *testing.T) {
	extraction := makeTestExtraction()
	avatarCfg := &config.AvatarConfig{} // no engine set

	profile, err := Build(extraction, avatarCfg, BuildOptions{})
	if err != nil {
		t.Fatalf("Build returned error: %v", err)
	}
	if profile.Engine != "local" {
		t.Errorf("expected Engine=local (default), got %q", profile.Engine)
	}
}

func TestBuild_UnknownEngine(t *testing.T) {
	extraction := makeTestExtraction()
	avatarCfg := &config.AvatarConfig{}

	_, err := Build(extraction, avatarCfg, BuildOptions{Engine: "unknown"})
	if err == nil {
		t.Fatal("expected error for unknown engine, got nil")
	}
	if !strings.Contains(err.Error(), "unknown") {
		t.Errorf("expected error to mention 'unknown', got: %v", err)
	}
}

func TestBuild_LLMNotImplemented(t *testing.T) {
	extraction := makeTestExtraction()
	avatarCfg := &config.AvatarConfig{}

	_, err := Build(extraction, avatarCfg, BuildOptions{Engine: "llm"})
	if err == nil {
		t.Fatal("expected error for llm engine (not yet implemented), got nil")
	}
	if !strings.Contains(err.Error(), "llm") {
		t.Errorf("expected error to mention 'llm', got: %v", err)
	}
}
