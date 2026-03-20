package avatar

import (
	"os"
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

// ---------------------------------------------------------------------------
// lengthDescription
// ---------------------------------------------------------------------------

func TestLengthDescription(t *testing.T) {
	tests := []struct {
		median float64
		want   string
	}{
		{0, "short"},
		{10, "short"},
		{19.9, "short"},
		{20, "medium-length"},
		{50, "medium-length"},
		{59.9, "medium-length"},
		{60, "long"},
		{100, "long"},
	}
	for _, tc := range tests {
		got := lengthDescription(tc.median)
		if got != tc.want {
			t.Errorf("lengthDescription(%v) = %q, want %q", tc.median, got, tc.want)
		}
	}
}

// ---------------------------------------------------------------------------
// parseOverrides
// ---------------------------------------------------------------------------

func TestParseOverrides(t *testing.T) {
	// nil / empty returns nil
	if got := parseOverrides(nil); got != nil {
		t.Errorf("parseOverrides(nil) = %v, want nil", got)
	}
	if got := parseOverrides([]string{}); got != nil {
		t.Errorf("parseOverrides([]) = %v, want nil", got)
	}

	// valid pairs
	m := parseOverrides([]string{"key=value", "a=b=c"})
	if m == nil {
		t.Fatal("expected non-nil map")
	}
	if m["key"] != "value" {
		t.Errorf("m[key] = %q, want %q", m["key"], "value")
	}
	// second '=' is preserved in value
	if m["a"] != "b=c" {
		t.Errorf("m[a] = %q, want %q", m["a"], "b=c")
	}

	// entry missing '=' is ignored; if all invalid, returns nil
	got := parseOverrides([]string{"no-equals-sign"})
	if got != nil {
		t.Errorf("expected nil for no-equals-sign, got %v", got)
	}
}

// ---------------------------------------------------------------------------
// BuildLocal — display name fallback
// ---------------------------------------------------------------------------

func TestBuildLocal_UsesUserWhenDisplayNameEmpty(t *testing.T) {
	extraction := &Extraction{
		Meta: ExtractionMeta{
			User:        "fallback.user",
			DisplayName: "", // empty — should fall back to User
		},
	}
	profile, err := BuildLocal(extraction, nil)
	if err != nil {
		t.Fatalf("BuildLocal returned error: %v", err)
	}
	if profile.DisplayName != "fallback.user" {
		t.Errorf("DisplayName = %q, want %q", profile.DisplayName, "fallback.user")
	}
	if !strings.Contains(profile.StyleGuide.Writing, "fallback.user") {
		t.Errorf("Writing should mention 'fallback.user', got: %s", profile.StyleGuide.Writing)
	}
}

func TestBuildLocal_LongComments(t *testing.T) {
	// Test lengthDescription "long" path by setting MedianLengthWords >= 60
	extraction := &Extraction{
		Meta: ExtractionMeta{
			User:        "longwriter",
			DisplayName: "Long Writer",
		},
		Writing: WritingAnalysis{
			Comments: CommentStats{
				MedianLengthWords: 100,
			},
		},
	}
	profile, err := BuildLocal(extraction, nil)
	if err != nil {
		t.Fatalf("BuildLocal returned error: %v", err)
	}
	if !strings.Contains(profile.StyleGuide.Writing, "long") {
		t.Errorf("expected 'long' in Writing section for MedianLengthWords=100, got: %s", profile.StyleGuide.Writing)
	}
}

func TestBuildLocal_MediumComments(t *testing.T) {
	// Test lengthDescription "medium-length" path
	extraction := &Extraction{
		Meta: ExtractionMeta{
			User:        "medwriter",
			DisplayName: "Med Writer",
		},
		Writing: WritingAnalysis{
			Comments: CommentStats{
				MedianLengthWords: 40,
			},
		},
	}
	profile, err := BuildLocal(extraction, nil)
	if err != nil {
		t.Fatalf("BuildLocal returned error: %v", err)
	}
	if !strings.Contains(profile.StyleGuide.Writing, "medium-length") {
		t.Errorf("expected 'medium-length' in Writing section for MedianLengthWords=40, got: %s", profile.StyleGuide.Writing)
	}
}

func TestBuildLocal_InteractionBranches(t *testing.T) {
	// Test interaction section: RepliesToOthersPct > 0.5 branch
	extraction := &Extraction{
		Meta: ExtractionMeta{
			User:        "otheruser",
			DisplayName: "Other User",
		},
		Interaction: InteractionAnalysis{
			ResponsePatterns: ResponsePatterns{
				MedianReplyTime:       "1h",
				RepliesToOwnIssuesPct: 0.3,
				RepliesToOthersPct:    0.7,
			},
		},
	}
	profile, err := BuildLocal(extraction, nil)
	if err != nil {
		t.Fatalf("BuildLocal returned error: %v", err)
	}
	if !strings.Contains(profile.StyleGuide.Interaction, "Other User") {
		t.Errorf("Interaction section should mention 'Other User': %s", profile.StyleGuide.Interaction)
	}
}

func TestBuildLocal_ActiveHoursWithTimezone(t *testing.T) {
	// Test buildInteractionSection with ActiveHours.Start/End/Timezone set
	extraction := &Extraction{
		Meta: ExtractionMeta{
			User:        "tzuser",
			DisplayName: "TZ User",
		},
		Interaction: InteractionAnalysis{
			ResponsePatterns: ResponsePatterns{
				MedianReplyTime: "30m",
			},
			Collaboration: Collaboration{
				ActiveHours: ActiveHours{
					Start:    "08:00",
					End:      "17:00",
					Timezone: "America/New_York",
				},
				PeakActivityDays: []string{"Monday"},
			},
		},
	}
	profile, err := BuildLocal(extraction, nil)
	if err != nil {
		t.Fatalf("BuildLocal returned error: %v", err)
	}
	if !strings.Contains(profile.StyleGuide.Interaction, "08:00") {
		t.Errorf("Interaction should contain active hours start '08:00': %s", profile.StyleGuide.Interaction)
	}
}

func TestBuildLocal_ActiveHoursNoTimezone(t *testing.T) {
	// Test buildInteractionSection with ActiveHours but no Timezone
	extraction := &Extraction{
		Meta: ExtractionMeta{
			User:        "notzuser",
			DisplayName: "NoTZ User",
		},
		Interaction: InteractionAnalysis{
			ResponsePatterns: ResponsePatterns{
				MedianReplyTime: "30m",
			},
			Collaboration: Collaboration{
				ActiveHours: ActiveHours{
					Start: "09:00",
					End:   "18:00",
					// no Timezone
				},
			},
		},
	}
	profile, err := BuildLocal(extraction, nil)
	if err != nil {
		t.Fatalf("BuildLocal returned error: %v", err)
	}
	if !strings.Contains(profile.StyleGuide.Interaction, "09:00") {
		t.Errorf("Interaction should contain active hours '09:00': %s", profile.StyleGuide.Interaction)
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

func TestBuild_LLMWithCmd(t *testing.T) {
	// Build with engine=llm and a real mock script.
	extraction := makeTestExtraction()
	dir := t.TempDir()

	// Write a valid profile YAML as mock LLM output.
	script := dir + "/llm.sh"
	scriptContent := `#!/bin/sh
cat <<'YAML'
version: 1
user: "test@co.com"
display_name: "Test User"
generated_at: "2026-03-18T10:00:00Z"
engine: "llm"
style_guide:
  writing: "Writes concise comments."
  workflow: "Sets priority always."
  interaction: "Responds quickly."
defaults:
  priority: "Medium"
examples: []
YAML
`
	if err := os.WriteFile(script, []byte(scriptContent), 0o755); err != nil {
		t.Fatalf("write mock script: %v", err)
	}

	avatarCfg := &config.AvatarConfig{}
	profile, err := Build(extraction, avatarCfg, BuildOptions{Engine: "llm", LLMCmd: script})
	if err != nil {
		t.Fatalf("Build with llm engine returned error: %v", err)
	}
	if profile.Engine != "llm" {
		t.Errorf("expected Engine=llm, got %q", profile.Engine)
	}
}

func TestBuild_WithOverrides(t *testing.T) {
	extraction := makeTestExtraction()
	avatarCfg := &config.AvatarConfig{
		Engine: "local",
		Overrides: map[string]string{
			"tone": "casual",
		},
	}

	profile, err := Build(extraction, avatarCfg, BuildOptions{})
	if err != nil {
		t.Fatalf("Build returned error: %v", err)
	}
	if profile.Overrides == nil {
		t.Fatal("expected non-nil Overrides")
	}
	if profile.Overrides["tone"] != "casual" {
		t.Errorf("expected Overrides[tone]=casual, got %q", profile.Overrides["tone"])
	}
}

func TestBuild_NilAvatarCfg(t *testing.T) {
	extraction := makeTestExtraction()

	// nil avatarCfg should use "local" as default engine
	profile, err := Build(extraction, nil, BuildOptions{})
	if err != nil {
		t.Fatalf("Build with nil config returned error: %v", err)
	}
	if profile.Engine != "local" {
		t.Errorf("expected Engine=local with nil config, got %q", profile.Engine)
	}
}

func TestBuild_EngineFromConfig(t *testing.T) {
	extraction := makeTestExtraction()
	avatarCfg := &config.AvatarConfig{Engine: "local"}

	profile, err := Build(extraction, avatarCfg, BuildOptions{})
	if err != nil {
		t.Fatalf("Build returned error: %v", err)
	}
	if profile.Engine != "local" {
		t.Errorf("expected Engine=local from config, got %q", profile.Engine)
	}
}

// TestBuildLocal_ReplyBias_OwnIssues verifies the first reply-bias branch:
// when RepliesToOwnIssuesPct > 0.5, the interaction text mentions own issues.
func TestBuildLocal_ReplyBias_OwnIssues(t *testing.T) {
	extraction := &Extraction{
		Meta: ExtractionMeta{
			User:        "ownuser",
			DisplayName: "Own User",
		},
		Interaction: InteractionAnalysis{
			ResponsePatterns: ResponsePatterns{
				MedianReplyTime:       "1h",
				RepliesToOwnIssuesPct: 0.8,
				RepliesToOthersPct:    0.2,
			},
		},
	}
	profile, err := BuildLocal(extraction, nil)
	if err != nil {
		t.Fatalf("BuildLocal returned error: %v", err)
	}
	if !strings.Contains(profile.StyleGuide.Interaction, "own issues") {
		t.Errorf("expected 'own issues' in Interaction for RepliesToOwnIssuesPct=0.8, got: %s",
			profile.StyleGuide.Interaction)
	}
}

// TestBuildLocal_ReplyBias_OthersIssues verifies the second reply-bias branch:
// when RepliesToOthersPct > 0.5, the interaction text mentions others' issues.
func TestBuildLocal_ReplyBias_OthersIssues(t *testing.T) {
	extraction := &Extraction{
		Meta: ExtractionMeta{
			User:        "engager",
			DisplayName: "Engager User",
		},
		Interaction: InteractionAnalysis{
			ResponsePatterns: ResponsePatterns{
				MedianReplyTime:       "2h",
				RepliesToOwnIssuesPct: 0.2,
				RepliesToOthersPct:    0.9,
			},
		},
	}
	profile, err := BuildLocal(extraction, nil)
	if err != nil {
		t.Fatalf("BuildLocal returned error: %v", err)
	}
	if !strings.Contains(profile.StyleGuide.Interaction, "Engager User") {
		t.Errorf("expected display name in Interaction section, got: %s", profile.StyleGuide.Interaction)
	}
	if !strings.Contains(profile.StyleGuide.Interaction, "90") {
		t.Errorf("expected percentage '90' in Interaction for RepliesToOthersPct=0.9, got: %s",
			profile.StyleGuide.Interaction)
	}
}

// TestBuildLocal_ReplyBias_Neither verifies that when neither pct > 0.5,
// no reply-bias sentence is emitted.
func TestBuildLocal_ReplyBias_Neither(t *testing.T) {
	extraction := &Extraction{
		Meta: ExtractionMeta{
			User:        "balanced",
			DisplayName: "Balanced User",
		},
		Interaction: InteractionAnalysis{
			ResponsePatterns: ResponsePatterns{
				MedianReplyTime:       "3h",
				RepliesToOwnIssuesPct: 0.5,
				RepliesToOthersPct:    0.5,
			},
		},
	}
	profile, err := BuildLocal(extraction, nil)
	if err != nil {
		t.Fatalf("BuildLocal returned error: %v", err)
	}
	// Neither branch triggers — Balanced User should appear (from MedianReplyTime line)
	// but neither "own issues" nor "others' issues" phrasing should appear.
	if strings.Contains(profile.StyleGuide.Interaction, "own issues") {
		t.Errorf("did not expect 'own issues' when RepliesToOwnIssuesPct=0.5: %s",
			profile.StyleGuide.Interaction)
	}
	if strings.Contains(profile.StyleGuide.Interaction, "others' issues") {
		t.Errorf("did not expect 'others' issues' when RepliesToOthersPct=0.5: %s",
			profile.StyleGuide.Interaction)
	}
}

func TestBuild_LLMRequiresCmd(t *testing.T) {
	extraction := makeTestExtraction()
	avatarCfg := &config.AvatarConfig{}

	_, err := Build(extraction, avatarCfg, BuildOptions{Engine: "llm"})
	if err == nil {
		t.Fatal("expected error for llm engine without llm-cmd, got nil")
	}
	if !strings.Contains(err.Error(), "llm-cmd") {
		t.Errorf("expected error to mention 'llm-cmd', got: %v", err)
	}
}
