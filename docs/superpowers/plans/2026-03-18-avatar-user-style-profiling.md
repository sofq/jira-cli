# Avatar — User Style Profiling Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build a two-phase system (`extract` + `build`) that analyzes a Jira user's activity and generates a natural language style profile for AI agents to act as the user's avatar.

**Architecture:** Phase 1 (`internal/avatar/`) fetches Jira data via the existing client and computes structured analysis (writing stats, workflow patterns, interaction habits). Phase 2 generates a profile using either Go templates (local engine) or a user-configured external LLM command. CLI commands live in `cmd/avatar.go`, following the same patterns as `cmd/workflow.go`.

**Tech Stack:** Go 1.25, Cobra CLI, existing `internal/client` for Jira API, `gopkg.in/yaml.v3` for profile YAML, `crypto/sha256` for user hashing, `os/exec` for LLM command piping.

**Spec:** `docs/superpowers/specs/2026-03-18-avatar-user-style-profiling-design.md`

---

## File Structure

### New Files

| File | Responsibility |
|------|---------------|
| `internal/avatar/types.go` | Extraction + Profile schema structs + shared input types (RawComment, ChangelogEntry, IssueFields, CreatedIssue, CommentRecord) |
| `internal/avatar/storage.go` | Avatar directory resolution, read/write extraction & profile files, cache management |
| `internal/avatar/fetch.go` | Jira API data fetching with adaptive time window |
| `internal/avatar/analyze_writing.go` | Comment/description text analysis (word counts, formatting, vocabulary, tone) |
| `internal/avatar/analyze_workflow.go` | Transition patterns, field preferences, issue creation habits |
| `internal/avatar/analyze_interaction.go` | Response times, mention habits, escalation patterns, collaboration |
| `internal/avatar/select_examples.go` | Cluster comments by context, pick representative examples |
| `internal/avatar/extract.go` | Orchestrator: fetch → analyze → select examples → write extraction.json |
| `internal/avatar/build_local.go` | Local engine: Go template-based profile generation |
| `internal/avatar/build_llm.go` | LLM engine: pipe extraction to external command, validate output |
| `internal/avatar/build.go` | Build orchestrator: engine selection, consent, fallback |
| `internal/avatar/prompt.go` | Format profile for agent consumption (prose/json/both, sections, redaction) |
| `internal/avatar/prompts/build_profile.txt` | Default LLM system prompt |
| `cmd/avatar.go` | Cobra commands: extract, build, prompt, show, edit, refresh, status |
| `internal/avatar/types_test.go` | Schema struct tests |
| `internal/avatar/storage_test.go` | Storage path resolution, read/write tests |
| `internal/avatar/analyze_writing_test.go` | Writing analysis unit tests |
| `internal/avatar/analyze_workflow_test.go` | Workflow analysis unit tests |
| `internal/avatar/analyze_interaction_test.go` | Interaction analysis unit tests |
| `internal/avatar/select_examples_test.go` | Example selection unit tests |
| `internal/avatar/fetch_test.go` | Fetch integration tests with mock HTTP server |
| `internal/avatar/extract_test.go` | End-to-end extraction tests |
| `internal/avatar/build_local_test.go` | Local engine tests + golden files |
| `internal/avatar/build_llm_test.go` | LLM engine tests with mock command |
| `internal/avatar/prompt_test.go` | Prompt formatting tests |
| `cmd/avatar_test.go` | CLI command tests |
| `internal/avatar/testdata/extraction_sample.json` | Golden file: sample extraction |
| `internal/avatar/testdata/profile_expected.yaml` | Golden file: expected local engine output |

### Modified Files

| File | Change |
|------|--------|
| `internal/config/config.go` | Add `AvatarConfig` struct, add `Avatar` field to `Profile` |
| `cmd/root.go` | Register `avatarCmd`, skip client injection for `avatar status`/`avatar show`/`avatar edit` |

---

## Task 1: Schema Types & Config Integration

**Files:**
- Create: `internal/avatar/types.go`
- Modify: `internal/config/config.go:29-36`
- Test: `internal/avatar/types_test.go`
- Test: `internal/config/config_test.go` (existing)

- [ ] **Step 1: Write failing test for AvatarConfig serialization**

```go
// internal/avatar/types_test.go
package avatar

import (
	"encoding/json"
	"testing"

	"gopkg.in/yaml.v3"
)

func TestExtractionJSON(t *testing.T) {
	e := &Extraction{
		Version: 1,
		Meta: ExtractionMeta{
			User:        "test@example.com",
			DisplayName: "Test User",
			ExtractedAt: "2026-03-18T10:00:00Z",
			Window:      TimeWindow{From: "2026-01-18", To: "2026-03-18"},
			DataPoints:  DataPoints{Comments: 62, IssuesCreated: 18, Transitions: 145, Worklogs: 34},
		},
	}
	data, err := json.Marshal(e)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var got Extraction
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if got.Version != 1 {
		t.Errorf("version = %d, want 1", got.Version)
	}
	if got.Meta.User != "test@example.com" {
		t.Errorf("user = %q, want %q", got.Meta.User, "test@example.com")
	}
}

func TestProfileYAML(t *testing.T) {
	p := &Profile{
		Version:     1,
		User:        "test@example.com",
		DisplayName: "Test User",
		GeneratedAt: "2026-03-18T10:05:00Z",
		Engine:      "local",
		StyleGuide: StyleGuide{
			Writing:     "Writes concise comments.",
			Workflow:    "Always sets priority.",
			Interaction: "Responds quickly.",
		},
		Overrides: []string{"Be professional"},
		Defaults: ProfileDefaults{
			Priority: "Medium",
			Labels:   []string{"backend"},
		},
	}
	data, err := yaml.Marshal(p)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var got Profile
	if err := yaml.Unmarshal(data, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if got.StyleGuide.Writing != "Writes concise comments." {
		t.Errorf("writing = %q", got.StyleGuide.Writing)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd /Users/quan.hoang/quanhh/quanhoang/jira-cli-v2 && go test ./internal/avatar/ -run TestExtraction -v`
Expected: FAIL — package does not exist

- [ ] **Step 3: Write types.go with all schema structs**

```go
// internal/avatar/types.go
package avatar

// --- Extraction Schema (Phase 1) ---

type Extraction struct {
	Version     int             `json:"version"`
	Meta        ExtractionMeta  `json:"meta"`
	Writing     WritingAnalysis `json:"writing"`
	Workflow    WorkflowAnalysis `json:"workflow"`
	Interaction InteractionAnalysis `json:"interaction"`
	Examples    ExtractionExamples `json:"examples"`
}

type ExtractionMeta struct {
	User        string     `json:"user"`
	DisplayName string     `json:"display_name"`
	ExtractedAt string     `json:"extracted_at"`
	Window      TimeWindow `json:"window"`
	DataPoints  DataPoints `json:"data_points"`
}

type TimeWindow struct {
	From string `json:"from"`
	To   string `json:"to"`
}

type DataPoints struct {
	Comments      int `json:"comments"`
	IssuesCreated int `json:"issues_created"`
	Transitions   int `json:"transitions"`
	Worklogs      int `json:"worklogs"`
}

type WritingAnalysis struct {
	Comments     CommentStats     `json:"comments"`
	Descriptions DescriptionStats `json:"descriptions"`
}

type CommentStats struct {
	AvgLengthWords    float64          `json:"avg_length_words"`
	MedianLengthWords int              `json:"median_length_words"`
	LengthDist        LengthDist       `json:"length_distribution"`
	Formatting        FormattingStats  `json:"formatting"`
	Vocabulary        VocabularyStats  `json:"vocabulary"`
	ToneSignals       ToneSignals      `json:"tone_signals"`
}

type LengthDist struct {
	ShortPct  float64 `json:"short_pct"`
	MediumPct float64 `json:"medium_pct"`
	LongPct   float64 `json:"long_pct"`
}

type FormattingStats struct {
	UsesBullets    float64 `json:"uses_bullets"`
	UsesCodeBlocks float64 `json:"uses_code_blocks"`
	UsesHeadings   float64 `json:"uses_headings"`
	UsesEmoji      float64 `json:"uses_emoji"`
	UsesMentions   float64 `json:"uses_mentions"`
}

type VocabularyStats struct {
	CommonPhrases []string `json:"common_phrases"`
	Jargon        []string `json:"jargon"`
	SignOffs      []string `json:"sign_offs"`
}

type ToneSignals struct {
	QuestionRatio    float64 `json:"question_ratio"`
	ExclamationRatio float64 `json:"exclamation_ratio"`
	FirstPersonRatio float64 `json:"first_person_ratio"`
	ImperativeRatio  float64 `json:"imperative_ratio"`
}

type DescriptionStats struct {
	AvgLengthWords    float64         `json:"avg_length_words"`
	StructurePatterns []string        `json:"structure_patterns"`
	Formatting        FormattingStats `json:"formatting"`
}

type WorkflowAnalysis struct {
	FieldPreferences   FieldPreferences   `json:"field_preferences"`
	TransitionPatterns TransitionPatterns `json:"transition_patterns"`
	IssueCreation      IssueCreation      `json:"issue_creation"`
}

type FieldPreferences struct {
	AlwaysSets       []string `json:"always_sets"`
	RarelySets       []string `json:"rarely_sets"`
	DefaultPriority  string   `json:"default_priority"`
	CommonLabels     []string `json:"common_labels"`
	CommonComponents []string `json:"common_components"`
}

type TransitionPatterns struct {
	AvgTimeInStatus        map[string]string `json:"avg_time_in_status"`
	CommonSequences        [][]string        `json:"common_sequences"`
	AssignsBeforeTransition bool             `json:"assigns_before_transition"`
}

type IssueCreation struct {
	TypesCreated       map[string]float64 `json:"types_created"`
	AvgSubtasksPerStory float64           `json:"avg_subtasks_per_story"`
}

type InteractionAnalysis struct {
	ResponsePatterns ResponsePatterns `json:"response_patterns"`
	MentionHabits    MentionHabits    `json:"mention_habits"`
	EscalationSignals EscalationSignals `json:"escalation_signals"`
	Collaboration    Collaboration    `json:"collaboration"`
}

type ResponsePatterns struct {
	MedianReplyTime       string  `json:"median_reply_time"`
	RepliesToOwnIssuesPct float64 `json:"replies_to_own_issues_pct"`
	RepliesToOthersPct    float64 `json:"replies_to_others_pct"`
	AvgThreadDepth        float64 `json:"avg_thread_depth"`
}

type MentionHabits struct {
	FrequentlyMentions []string `json:"frequently_mentions"`
	MentionContext     string   `json:"mention_context"`
}

type EscalationSignals struct {
	BlockerKeywords            []string `json:"blocker_keywords"`
	EscalationPattern          string   `json:"escalation_pattern"`
	AvgCommentsBeforeEscalation float64 `json:"avg_comments_before_escalation"`
}

type Collaboration struct {
	ActiveHours      ActiveHours    `json:"active_hours"`
	PeakActivityDays []string       `json:"peak_activity_days"`
	WorklogHabits    WorklogHabits  `json:"worklog_habits"`
}

type ActiveHours struct {
	Start    string `json:"start"`
	End      string `json:"end"`
	Timezone string `json:"timezone"`
}

type WorklogHabits struct {
	LogsDaily         bool    `json:"logs_daily"`
	AvgEntriesPerWeek float64 `json:"avg_entries_per_week"`
}

type ExtractionExamples struct {
	Comments     []CommentExample     `json:"comments"`
	Descriptions []DescriptionExample `json:"descriptions"`
}

type CommentExample struct {
	Issue   string `json:"issue"`
	Date    string `json:"date"`
	Text    string `json:"text"`
	Context string `json:"context"`
}

type DescriptionExample struct {
	Issue string `json:"issue"`
	Type  string `json:"type"`
	Text  string `json:"text"`
}

// --- Profile Schema (Phase 2) ---

type Profile struct {
	Version     int              `yaml:"version" json:"version"`
	User        string           `yaml:"user" json:"user"`
	DisplayName string           `yaml:"display_name" json:"display_name"`
	GeneratedAt string           `yaml:"generated_at" json:"generated_at"`
	Engine      string           `yaml:"engine" json:"engine"`
	StyleGuide  StyleGuide       `yaml:"style_guide" json:"style_guide"`
	Overrides   []string         `yaml:"overrides,omitempty" json:"overrides,omitempty"`
	Defaults    ProfileDefaults  `yaml:"defaults" json:"defaults"`
	Examples    []ProfileExample `yaml:"examples" json:"examples"`
}

type StyleGuide struct {
	Writing     string `yaml:"writing" json:"writing"`
	Workflow    string `yaml:"workflow" json:"workflow"`
	Interaction string `yaml:"interaction" json:"interaction"`
}

type ProfileDefaults struct {
	Priority               string   `yaml:"priority,omitempty" json:"priority,omitempty"`
	Labels                 []string `yaml:"labels,omitempty" json:"labels,omitempty"`
	Components             []string `yaml:"components,omitempty" json:"components,omitempty"`
	AssignSelfOnTransition bool     `yaml:"assign_self_on_transition,omitempty" json:"assign_self_on_transition,omitempty"`
}

type ProfileExample struct {
	Context string `yaml:"context" json:"context"`
	Source  string `yaml:"source" json:"source"`
	Text    string `yaml:"text" json:"text"`
}

// --- Shared Input Types (used by analyzers and fetcher) ---

// RawComment is a comment extracted from Jira before analysis.
type RawComment struct {
	Issue  string `json:"issue"`
	Date   string `json:"date"`
	Text   string `json:"text"`
	Author string `json:"author"`
}

// ChangelogEntry is a single field change from Jira's changelog.
type ChangelogEntry struct {
	Issue     string `json:"issue"`
	Timestamp string `json:"timestamp"`
	Author    string `json:"author"`
	Field     string `json:"field"`
	From      string `json:"from"`
	To        string `json:"to"`
}

// IssueFields holds the fields set on a created/updated issue.
type IssueFields struct {
	Priority   string   `json:"priority"`
	Labels     []string `json:"labels"`
	Components []string `json:"components"`
	FixVersion string   `json:"fix_version"`
}

// CreatedIssue holds info about an issue created by the user.
type CreatedIssue struct {
	Key          string `json:"key"`
	Type         string `json:"type"`
	SubtaskCount int    `json:"subtask_count"`
	Description  string `json:"description"`
}

// CommentRecord holds a comment with context for interaction analysis.
type CommentRecord struct {
	IssueOwner      string `json:"issue_owner"`
	Author          string `json:"author"`
	Timestamp       string `json:"timestamp"`
	ParentTimestamp string `json:"parent_timestamp"` // timestamp of the comment being replied to
	Text            string `json:"text"`
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd /Users/quan.hoang/quanhh/quanhoang/jira-cli-v2 && go test ./internal/avatar/ -run "TestExtraction|TestProfile" -v`
Expected: PASS

- [ ] **Step 5: Write failing test for AvatarConfig in config package**

Add to existing `internal/config/config_test.go`:

```go
func TestAvatarConfigRoundTrip(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.json")
	cfg := &Config{
		Profiles: map[string]Profile{
			"default": {
				BaseURL: "https://test.atlassian.net",
				Auth:    AuthConfig{Type: "basic", Username: "u", Token: "t"},
				Avatar: &AvatarConfig{
					Enabled:     true,
					Engine:      "llm",
					LLMCmd:      "claude -p 'test'",
					MinComments: 50,
					MinUpdates:  30,
					MaxWindow:   "6m",
					Overrides:   map[string]string{"tone": "professional"},
				},
			},
		},
		DefaultProfile: "default",
	}
	if err := SaveTo(cfg, cfgPath); err != nil {
		t.Fatalf("SaveTo: %v", err)
	}
	loaded, err := LoadFrom(cfgPath)
	if err != nil {
		t.Fatalf("LoadFrom: %v", err)
	}
	p := loaded.Profiles["default"]
	if p.Avatar == nil {
		t.Fatal("avatar config is nil after round-trip")
	}
	if p.Avatar.Engine != "llm" {
		t.Errorf("engine = %q, want %q", p.Avatar.Engine, "llm")
	}
	if p.Avatar.MinComments != 50 {
		t.Errorf("min_comments = %d, want 50", p.Avatar.MinComments)
	}
}
```

- [ ] **Step 6: Run test to verify it fails**

Run: `cd /Users/quan.hoang/quanhh/quanhoang/jira-cli-v2 && go test ./internal/config/ -run TestAvatarConfig -v`
Expected: FAIL — AvatarConfig undefined

- [ ] **Step 7: Add AvatarConfig to config.go**

In `internal/config/config.go`, add after the `AuthConfig` struct:

```go
type AvatarConfig struct {
	Enabled     bool              `json:"enabled,omitempty"`
	Engine      string            `json:"engine,omitempty"`
	LLMCmd      string            `json:"llm_cmd,omitempty"`
	MinComments int               `json:"min_comments,omitempty"`
	MinUpdates  int               `json:"min_updates,omitempty"`
	MaxWindow   string            `json:"max_window,omitempty"`
	Overrides   map[string]string `json:"overrides,omitempty"`
}
```

Add to the `Profile` struct:

```go
Avatar *AvatarConfig `json:"avatar,omitempty"`
```

- [ ] **Step 8: Run all config tests**

Run: `cd /Users/quan.hoang/quanhh/quanhoang/jira-cli-v2 && go test ./internal/config/ -v`
Expected: ALL PASS

- [ ] **Step 9: Commit**

```bash
git add internal/avatar/types.go internal/avatar/types_test.go internal/config/config.go internal/config/config_test.go
git commit -m "feat(avatar): add extraction and profile schema types, integrate AvatarConfig into profile"
```

---

## Task 2: Storage Layer

**Files:**
- Create: `internal/avatar/storage.go`
- Test: `internal/avatar/storage_test.go`

- [ ] **Step 1: Write failing tests for storage operations**

```go
// internal/avatar/storage_test.go
package avatar

import (
	"os"
	"path/filepath"
	"testing"
)

func TestAvatarDir(t *testing.T) {
	dir := AvatarDir("abc123def456")
	if dir == "" {
		t.Fatal("AvatarDir returned empty string")
	}
	// Should end with avatars/<hash>
	if filepath.Base(filepath.Dir(dir)) != "avatars" {
		t.Errorf("dir = %q, expected parent to be 'avatars'", dir)
	}
}

func TestUserHash(t *testing.T) {
	h1 := UserHash("5b10ac8d82e05b22cc7d4ef5")
	h2 := UserHash("5b10ac8d82e05b22cc7d4ef5")
	h3 := UserHash("different-account-id")
	if len(h1) != 16 {
		t.Errorf("hash length = %d, want 16", len(h1))
	}
	if h1 != h2 {
		t.Error("same input should produce same hash")
	}
	if h1 == h3 {
		t.Error("different input should produce different hash")
	}
}

func TestSaveAndLoadExtraction(t *testing.T) {
	dir := t.TempDir()
	e := &Extraction{Version: 1, Meta: ExtractionMeta{User: "test@example.com"}}
	if err := SaveExtraction(dir, e); err != nil {
		t.Fatalf("save: %v", err)
	}
	got, err := LoadExtraction(dir)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if got.Meta.User != "test@example.com" {
		t.Errorf("user = %q", got.Meta.User)
	}
}

func TestSaveAndLoadProfile(t *testing.T) {
	dir := t.TempDir()
	p := &Profile{Version: 1, User: "test@example.com", Engine: "local"}
	if err := SaveProfile(dir, p); err != nil {
		t.Fatalf("save: %v", err)
	}
	got, err := LoadProfile(dir)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if got.Engine != "local" {
		t.Errorf("engine = %q", got.Engine)
	}
}

func TestLoadExtractionNotFound(t *testing.T) {
	dir := t.TempDir()
	_, err := LoadExtraction(dir)
	if err == nil {
		t.Fatal("expected error for missing extraction")
	}
}

func TestProfileAge(t *testing.T) {
	dir := t.TempDir()
	p := &Profile{Version: 1, GeneratedAt: "2026-01-01T00:00:00Z"}
	if err := SaveProfile(dir, p); err != nil {
		t.Fatalf("save: %v", err)
	}
	days, err := ProfileAgeDays(dir)
	if err != nil {
		t.Fatalf("age: %v", err)
	}
	if days < 1 {
		t.Errorf("age = %d, expected > 0", days)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd /Users/quan.hoang/quanhh/quanhoang/jira-cli-v2 && go test ./internal/avatar/ -run "TestAvatarDir|TestUserHash|TestSaveAnd|TestLoadExtraction|TestProfileAge" -v`
Expected: FAIL

- [ ] **Step 3: Implement storage.go**

```go
// internal/avatar/storage.go
package avatar

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"gopkg.in/yaml.v3"
)

const (
	extractionFile = "extraction.json"
	profileFile    = "profile.yaml"
	cacheDir       = "cache"
)

// avatarsBaseDir returns the base directory for all avatar data.
func avatarsBaseDir() string {
	dir, err := os.UserConfigDir()
	if err != nil {
		home, _ := os.UserHomeDir()
		return filepath.Join(home, ".config", "jr", "avatars")
	}
	return filepath.Join(dir, "jr", "avatars")
}

// AvatarDir returns the directory for a specific user's avatar data.
func AvatarDir(userHash string) string {
	return filepath.Join(avatarsBaseDir(), userHash)
}

// UserHash computes a truncated SHA-256 hash of the Jira accountId.
func UserHash(accountID string) string {
	h := sha256.Sum256([]byte(accountID))
	return fmt.Sprintf("%x", h[:8]) // 16 hex chars
}

// SaveExtraction writes the extraction to disk as JSON.
func SaveExtraction(dir string, e *Extraction) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("create avatar dir: %w", err)
	}
	data, err := json.MarshalIndent(e, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal extraction: %w", err)
	}
	return os.WriteFile(filepath.Join(dir, extractionFile), data, 0o600)
}

// LoadExtraction reads the extraction from disk.
func LoadExtraction(dir string) (*Extraction, error) {
	data, err := os.ReadFile(filepath.Join(dir, extractionFile))
	if err != nil {
		return nil, fmt.Errorf("read extraction: %w", err)
	}
	var e Extraction
	if err := json.Unmarshal(data, &e); err != nil {
		return nil, fmt.Errorf("unmarshal extraction: %w", err)
	}
	return &e, nil
}

// SaveProfile writes the profile to disk as YAML.
func SaveProfile(dir string, p *Profile) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("create avatar dir: %w", err)
	}
	data, err := yaml.Marshal(p)
	if err != nil {
		return fmt.Errorf("marshal profile: %w", err)
	}
	return os.WriteFile(filepath.Join(dir, profileFile), data, 0o600)
}

// LoadProfile reads the profile from disk.
func LoadProfile(dir string) (*Profile, error) {
	data, err := os.ReadFile(filepath.Join(dir, profileFile))
	if err != nil {
		return nil, fmt.Errorf("read profile: %w", err)
	}
	var p Profile
	if err := yaml.Unmarshal(data, &p); err != nil {
		return nil, fmt.Errorf("unmarshal profile: %w", err)
	}
	return &p, nil
}

// ProfileExists checks if a profile file exists in the given directory.
func ProfileExists(dir string) bool {
	_, err := os.Stat(filepath.Join(dir, profileFile))
	return err == nil
}

// ExtractionExists checks if an extraction file exists in the given directory.
func ExtractionExists(dir string) bool {
	_, err := os.Stat(filepath.Join(dir, extractionFile))
	return err == nil
}

// ProfileAgeDays returns the age of the profile in days based on its generated_at field.
func ProfileAgeDays(dir string) (int, error) {
	p, err := LoadProfile(dir)
	if err != nil {
		return 0, err
	}
	t, err := time.Parse(time.RFC3339, p.GeneratedAt)
	if err != nil {
		return 0, fmt.Errorf("parse generated_at: %w", err)
	}
	return int(time.Since(t).Hours() / 24), nil
}

// CacheDir returns the cache directory path for a user's avatar.
func CacheDir(avatarDir string) string {
	return filepath.Join(avatarDir, cacheDir)
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd /Users/quan.hoang/quanhh/quanhoang/jira-cli-v2 && go test ./internal/avatar/ -run "TestAvatarDir|TestUserHash|TestSaveAnd|TestLoadExtraction|TestProfileAge" -v`
Expected: ALL PASS

- [ ] **Step 5: Commit**

```bash
git add internal/avatar/storage.go internal/avatar/storage_test.go
git commit -m "feat(avatar): add storage layer for extraction and profile files"
```

---

## Task 3: Writing Analyzer

**Files:**
- Create: `internal/avatar/analyze_writing.go`
- Test: `internal/avatar/analyze_writing_test.go`

- [ ] **Step 1: Write failing tests**

```go
// internal/avatar/analyze_writing_test.go
package avatar

import "testing"

func TestAnalyzeComments(t *testing.T) {
	comments := []string{
		"Fixed the auth timeout. Deployed to staging.",
		"- item 1\n- item 2\n- item 3",
		"Looks good, thanks!",
		"@alice can you check this? Blocked by PROJ-100.",
		"```\nerror: connection refused\n```\nNeed to investigate.",
	}
	stats := AnalyzeComments(comments)
	if stats.MedianLengthWords == 0 {
		t.Error("median should be > 0")
	}
	if stats.Formatting.UsesBullets == 0 {
		t.Error("should detect bullet usage")
	}
	if stats.Formatting.UsesCodeBlocks == 0 {
		t.Error("should detect code block usage")
	}
	if stats.Formatting.UsesMentions == 0 {
		t.Error("should detect @mention usage")
	}
	if stats.ToneSignals.QuestionRatio == 0 {
		t.Error("should detect questions")
	}
}

func TestAnalyzeComments_Empty(t *testing.T) {
	stats := AnalyzeComments(nil)
	if stats.AvgLengthWords != 0 {
		t.Errorf("avg = %f, want 0 for empty input", stats.AvgLengthWords)
	}
}

func TestWordCount(t *testing.T) {
	tests := []struct {
		input string
		want  int
	}{
		{"hello world", 2},
		{"  spaced  out  words  ", 3},
		{"", 0},
		{"single", 1},
	}
	for _, tt := range tests {
		got := wordCount(tt.input)
		if got != tt.want {
			t.Errorf("wordCount(%q) = %d, want %d", tt.input, got, tt.want)
		}
	}
}

func TestExtractCommonPhrases(t *testing.T) {
	comments := []string{
		"looks good to me",
		"looks good, thanks",
		"let me check this",
		"let me check with the team",
		"something else entirely",
	}
	phrases := extractCommonPhrases(comments, 2)
	if len(phrases) == 0 {
		t.Error("should find common phrases")
	}
}

func TestDetectSignOffs(t *testing.T) {
	comments := []string{
		"Fixed the bug. thanks",
		"Updated the PR. lmk",
		"Deployed to staging. thanks",
		"No sign-off here",
	}
	signOffs := detectSignOffs(comments)
	if len(signOffs) == 0 {
		t.Error("should detect sign-offs")
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd /Users/quan.hoang/quanhh/quanhoang/jira-cli-v2 && go test ./internal/avatar/ -run "TestAnalyzeComments|TestWordCount|TestExtractCommon|TestDetectSign" -v`
Expected: FAIL

- [ ] **Step 3: Implement analyze_writing.go**

```go
// internal/avatar/analyze_writing.go
package avatar

import (
	"math"
	"regexp"
	"sort"
	"strings"
)

var (
	bulletRe    = regexp.MustCompile(`(?m)^[\s]*[-*•]\s`)
	codeBlockRe = regexp.MustCompile("(?s)```.*?```")
	headingRe   = regexp.MustCompile(`(?m)^#{1,6}\s`)
	emojiRe     = regexp.MustCompile(`[\x{1F600}-\x{1F64F}]|[\x{1F300}-\x{1F5FF}]|[\x{1F680}-\x{1F6FF}]|[\x{2600}-\x{26FF}]|[\x{2700}-\x{27BF}]`)
	mentionRe   = regexp.MustCompile(`@\w+`)
	questionRe  = regexp.MustCompile(`\?`)
	exclamRe    = regexp.MustCompile(`!`)
	firstPerRe  = regexp.MustCompile(`(?i)\b(I|I'm|I'll|I've|I'd|my|me|mine)\b`)
	imperativeRe = regexp.MustCompile(`(?i)^(fix|add|update|remove|check|deploy|merge|test|run|set|move)\b`)
)

// AnalyzeComments computes writing stats from a slice of plain-text comments.
func AnalyzeComments(comments []string) CommentStats {
	if len(comments) == 0 {
		return CommentStats{}
	}
	var (
		wordCounts []int
		totalWords int
		bullets, codeBlocks, headings, emojis, mentions int
		questions, exclamations, firstPerson, imperatives int
		totalSentences int
	)
	for _, c := range comments {
		wc := wordCount(c)
		wordCounts = append(wordCounts, wc)
		totalWords += wc
		if bulletRe.MatchString(c) {
			bullets++
		}
		if codeBlockRe.MatchString(c) {
			codeBlocks++
		}
		if headingRe.MatchString(c) {
			headings++
		}
		if emojiRe.MatchString(c) {
			emojis++
		}
		if mentionRe.MatchString(c) {
			mentions++
		}
		sentences := splitSentences(c)
		totalSentences += len(sentences)
		for _, s := range sentences {
			if questionRe.MatchString(s) {
				questions++
			}
			if exclamRe.MatchString(s) {
				exclamations++
			}
			if firstPerRe.MatchString(s) {
				firstPerson++
			}
			trimmed := strings.TrimSpace(s)
			if imperativeRe.MatchString(trimmed) {
				imperatives++
			}
		}
	}
	n := float64(len(comments))
	safeSentences := math.Max(float64(totalSentences), 1)
	sort.Ints(wordCounts)
	median := wordCounts[len(wordCounts)/2]

	shortThreshold := 20
	longThreshold := 80
	var short, medium, long int
	for _, wc := range wordCounts {
		switch {
		case wc <= shortThreshold:
			short++
		case wc >= longThreshold:
			long++
		default:
			medium++
		}
	}

	return CommentStats{
		AvgLengthWords:    float64(totalWords) / n,
		MedianLengthWords: median,
		LengthDist: LengthDist{
			ShortPct:  float64(short) / n * 100,
			MediumPct: float64(medium) / n * 100,
			LongPct:   float64(long) / n * 100,
		},
		Formatting: FormattingStats{
			UsesBullets:    float64(bullets) / n,
			UsesCodeBlocks: float64(codeBlocks) / n,
			UsesHeadings:   float64(headings) / n,
			UsesEmoji:      float64(emojis) / n,
			UsesMentions:   float64(mentions) / n,
		},
		Vocabulary: VocabularyStats{
			CommonPhrases: extractCommonPhrases(comments, 10),
			Jargon:        extractJargon(comments),
			SignOffs:       detectSignOffs(comments),
		},
		ToneSignals: ToneSignals{
			QuestionRatio:    float64(questions) / safeSentences,
			ExclamationRatio: float64(exclamations) / safeSentences,
			FirstPersonRatio: float64(firstPerson) / safeSentences,
			ImperativeRatio:  float64(imperatives) / safeSentences,
		},
	}
}

// AnalyzeDescriptions computes description-specific stats.
func AnalyzeDescriptions(descriptions []string) DescriptionStats {
	if len(descriptions) == 0 {
		return DescriptionStats{}
	}
	var totalWords int
	var bullets, headings int
	patterns := map[string]int{}
	for _, d := range descriptions {
		totalWords += wordCount(d)
		if bulletRe.MatchString(d) {
			bullets++
		}
		if headingRe.MatchString(d) {
			headings++
		}
		lower := strings.ToLower(d)
		if strings.Contains(lower, "acceptance criteria") || strings.Contains(lower, "ac:") {
			patterns["acceptance_criteria"]++
		}
		if strings.Contains(lower, "steps to reproduce") || strings.Contains(lower, "repro") {
			patterns["steps_to_reproduce"]++
		}
		if strings.Contains(lower, "background") || strings.Contains(lower, "context") {
			patterns["background"]++
		}
	}
	n := float64(len(descriptions))
	var structPatterns []string
	for p := range patterns {
		structPatterns = append(structPatterns, p)
	}
	sort.Strings(structPatterns)
	return DescriptionStats{
		AvgLengthWords:    float64(totalWords) / n,
		StructurePatterns: structPatterns,
		Formatting: FormattingStats{
			UsesBullets:  float64(bullets) / n,
			UsesHeadings: float64(headings) / n,
		},
	}
}

func wordCount(s string) int {
	return len(strings.Fields(s))
}

func splitSentences(s string) []string {
	// Simple sentence splitting on . ? ! followed by space or end
	re := regexp.MustCompile(`[.?!]+\s*`)
	parts := re.Split(s, -1)
	var result []string
	for _, p := range parts {
		if strings.TrimSpace(p) != "" {
			result = append(result, strings.TrimSpace(p))
		}
	}
	if len(result) == 0 && strings.TrimSpace(s) != "" {
		return []string{strings.TrimSpace(s)}
	}
	return result
}

func extractCommonPhrases(comments []string, maxPhrases int) []string {
	// Extract 2-grams and 3-grams, count occurrences
	ngrams := map[string]int{}
	for _, c := range comments {
		words := strings.Fields(strings.ToLower(c))
		for i := 0; i < len(words)-1; i++ {
			bigram := words[i] + " " + words[i+1]
			ngrams[bigram]++
			if i < len(words)-2 {
				trigram := words[i] + " " + words[i+1] + " " + words[i+2]
				ngrams[trigram]++
			}
		}
	}
	// Filter to those appearing in >= 2 comments, sort by frequency
	type kv struct {
		key   string
		count int
	}
	var filtered []kv
	for k, v := range ngrams {
		if v >= 2 {
			filtered = append(filtered, kv{k, v})
		}
	}
	sort.Slice(filtered, func(i, j int) bool { return filtered[i].count > filtered[j].count })
	var result []string
	for i, kv := range filtered {
		if i >= maxPhrases {
			break
		}
		result = append(result, kv.key)
	}
	return result
}

// Stop words commonly excluded from jargon detection
var stopWords = map[string]bool{
	"the": true, "a": true, "an": true, "is": true, "are": true, "was": true,
	"were": true, "be": true, "been": true, "being": true, "have": true,
	"has": true, "had": true, "do": true, "does": true, "did": true,
	"will": true, "would": true, "could": true, "should": true, "may": true,
	"might": true, "can": true, "shall": true, "to": true, "of": true,
	"in": true, "for": true, "on": true, "with": true, "at": true,
	"by": true, "from": true, "as": true, "into": true, "through": true,
	"and": true, "but": true, "or": true, "nor": true, "not": true,
	"so": true, "yet": true, "this": true, "that": true, "these": true,
	"those": true, "it": true, "its": true, "i": true, "me": true,
	"my": true, "we": true, "our": true, "you": true, "your": true,
	"he": true, "she": true, "they": true, "them": true, "their": true,
}

func extractJargon(comments []string) []string {
	wordFreq := map[string]int{}
	for _, c := range comments {
		for _, w := range strings.Fields(strings.ToLower(c)) {
			w = strings.Trim(w, ".,;:!?\"'()[]{}")
			if len(w) > 3 && !stopWords[w] {
				wordFreq[w]++
			}
		}
	}
	type kv struct {
		key   string
		count int
	}
	var words []kv
	for k, v := range wordFreq {
		if v >= 3 {
			words = append(words, kv{k, v})
		}
	}
	sort.Slice(words, func(i, j int) bool { return words[i].count > words[j].count })
	var result []string
	for i, w := range words {
		if i >= 10 {
			break
		}
		result = append(result, w.key)
	}
	return result
}

func detectSignOffs(comments []string) []string {
	signOffCandidates := []string{"thanks", "thank you", "cheers", "lmk", "let me know", "ty", "thx", "regards"}
	found := map[string]int{}
	for _, c := range comments {
		lower := strings.ToLower(c)
		// Check last 30 chars for sign-off patterns
		tail := lower
		if len(tail) > 30 {
			tail = tail[len(tail)-30:]
		}
		for _, so := range signOffCandidates {
			if strings.Contains(tail, so) {
				found[so]++
			}
		}
	}
	var result []string
	for so, count := range found {
		if count >= 2 {
			result = append(result, so)
		}
	}
	sort.Strings(result)
	return result
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd /Users/quan.hoang/quanhh/quanhoang/jira-cli-v2 && go test ./internal/avatar/ -run "TestAnalyzeComments|TestWordCount|TestExtractCommon|TestDetectSign" -v`
Expected: ALL PASS

- [ ] **Step 5: Commit**

```bash
git add internal/avatar/analyze_writing.go internal/avatar/analyze_writing_test.go
git commit -m "feat(avatar): add writing style analyzer"
```

---

## Task 4: Workflow Analyzer

**Files:**
- Create: `internal/avatar/analyze_workflow.go`
- Test: `internal/avatar/analyze_workflow_test.go`

- [ ] **Step 1: Write failing tests**

```go
// internal/avatar/analyze_workflow_test.go
package avatar

import "testing"

func TestAnalyzeTransitions(t *testing.T) {
	entries := []ChangelogEntry{
		{Timestamp: "2026-03-01T09:00:00Z", Field: "status", From: "To Do", To: "In Progress"},
		{Timestamp: "2026-03-01T17:00:00Z", Field: "status", From: "In Progress", To: "In Review"},
		{Timestamp: "2026-03-01T21:00:00Z", Field: "status", From: "In Review", To: "Done"},
		{Timestamp: "2026-03-02T09:00:00Z", Field: "status", From: "To Do", To: "In Progress"},
		{Timestamp: "2026-03-02T11:00:00Z", Field: "status", From: "In Progress", To: "Done"},
	}
	patterns := AnalyzeTransitions(entries)
	if len(patterns.CommonSequences) == 0 {
		t.Error("should find common sequences")
	}
	if len(patterns.AvgTimeInStatus) == 0 {
		t.Error("should compute avg time in status")
	}
}

func TestAnalyzeFieldPreferences(t *testing.T) {
	issues := []IssueFields{
		{Priority: "High", Labels: []string{"backend"}, Components: []string{"api"}, FixVersion: ""},
		{Priority: "Medium", Labels: []string{"backend"}, Components: []string{"api"}, FixVersion: ""},
		{Priority: "Medium", Labels: []string{"frontend"}, Components: nil, FixVersion: "1.0"},
	}
	prefs := AnalyzeFieldPreferences(issues)
	if prefs.DefaultPriority != "Medium" {
		t.Errorf("default priority = %q, want Medium", prefs.DefaultPriority)
	}
	if len(prefs.CommonLabels) == 0 {
		t.Error("should find common labels")
	}
}

func TestAnalyzeIssueCreation(t *testing.T) {
	issues := []CreatedIssue{
		{Type: "Bug", SubtaskCount: 0},
		{Type: "Bug", SubtaskCount: 0},
		{Type: "Story", SubtaskCount: 3},
		{Type: "Task", SubtaskCount: 0},
	}
	creation := AnalyzeIssueCreation(issues)
	if creation.TypesCreated["Bug"] != 0.5 {
		t.Errorf("bug ratio = %f, want 0.5", creation.TypesCreated["Bug"])
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd /Users/quan.hoang/quanhh/quanhoang/jira-cli-v2 && go test ./internal/avatar/ -run "TestAnalyzeTransitions|TestAnalyzeField|TestAnalyzeIssue" -v`
Expected: FAIL

- [ ] **Step 3: Implement analyze_workflow.go**

Implement the `ChangelogEntry`, `IssueFields`, `CreatedIssue` input types and the three analysis functions: `AnalyzeTransitions`, `AnalyzeFieldPreferences`, `AnalyzeIssueCreation`. Each function takes raw data and returns the corresponding schema struct from `types.go`.

Key logic:
- `AnalyzeTransitions`: group sequential status changes per issue into sequences, compute time between transitions, find most common sequences
- `AnalyzeFieldPreferences`: count which fields are set across issues, find mode for priority, count label/component frequency
- `AnalyzeIssueCreation`: count issue types as ratios, compute avg subtasks for stories

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd /Users/quan.hoang/quanhh/quanhoang/jira-cli-v2 && go test ./internal/avatar/ -run "TestAnalyzeTransitions|TestAnalyzeField|TestAnalyzeIssue" -v`
Expected: ALL PASS

- [ ] **Step 5: Commit**

```bash
git add internal/avatar/analyze_workflow.go internal/avatar/analyze_workflow_test.go
git commit -m "feat(avatar): add workflow pattern analyzer"
```

---

## Task 5: Interaction Analyzer

**Files:**
- Create: `internal/avatar/analyze_interaction.go`
- Test: `internal/avatar/analyze_interaction_test.go`

- [ ] **Step 1: Write failing tests**

```go
// internal/avatar/analyze_interaction_test.go
package avatar

import "testing"

func TestAnalyzeResponsePatterns(t *testing.T) {
	comments := []CommentRecord{
		{IssueOwner: "user@co.com", Author: "user@co.com", Timestamp: "2026-03-01T10:00:00Z", ParentTimestamp: "2026-03-01T08:00:00Z"},
		{IssueOwner: "user@co.com", Author: "user@co.com", Timestamp: "2026-03-01T14:00:00Z", ParentTimestamp: "2026-03-01T13:00:00Z"},
		{IssueOwner: "other@co.com", Author: "user@co.com", Timestamp: "2026-03-02T10:00:00Z", ParentTimestamp: "2026-03-02T09:00:00Z"},
	}
	patterns := AnalyzeResponsePatterns(comments, "user@co.com")
	if patterns.RepliesToOwnIssuesPct == 0 {
		t.Error("should detect replies to own issues")
	}
}

func TestAnalyzeMentionHabits(t *testing.T) {
	comments := []string{
		"@alice can you review this?",
		"@alice blocked on your approval",
		"@bob fyi",
		"no mentions here",
	}
	habits := AnalyzeMentionHabits(comments)
	if len(habits.FrequentlyMentions) == 0 {
		t.Error("should detect frequently mentioned users")
	}
}

func TestAnalyzeCollaboration(t *testing.T) {
	timestamps := []string{
		"2026-03-04T09:30:00+09:00", // Tuesday
		"2026-03-04T14:00:00+09:00",
		"2026-03-05T10:00:00+09:00", // Wednesday
		"2026-03-06T16:00:00+09:00", // Thursday
	}
	collab := AnalyzeCollaboration(timestamps)
	if collab.ActiveHours.Start == "" {
		t.Error("should detect active hours")
	}
	if len(collab.PeakActivityDays) == 0 {
		t.Error("should detect peak days")
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd /Users/quan.hoang/quanhh/quanhoang/jira-cli-v2 && go test ./internal/avatar/ -run "TestAnalyzeResponse|TestAnalyzeMention|TestAnalyzeCollab" -v`
Expected: FAIL

- [ ] **Step 3: Implement analyze_interaction.go**

Input types: `CommentRecord` (with issue owner, author, timestamps for reply-time calculation).

Key logic:
- `AnalyzeResponsePatterns`: compute reply times (diff between parent comment and reply), separate own-issue vs others-issue replies
- `AnalyzeMentionHabits`: regex extract @mentions, count frequency, determine context (review/blocker keywords nearby)
- `AnalyzeCollaboration`: parse timestamps to determine active hour range, day-of-week distribution
- `AnalyzeEscalation`: detect blocker keywords, count comments before status change to "Blocked" or reassignment

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd /Users/quan.hoang/quanhh/quanhoang/jira-cli-v2 && go test ./internal/avatar/ -run "TestAnalyzeResponse|TestAnalyzeMention|TestAnalyzeCollab" -v`
Expected: ALL PASS

- [ ] **Step 5: Commit**

```bash
git add internal/avatar/analyze_interaction.go internal/avatar/analyze_interaction_test.go
git commit -m "feat(avatar): add interaction pattern analyzer"
```

---

## Task 6: Example Selector

**Files:**
- Create: `internal/avatar/select_examples.go`
- Test: `internal/avatar/select_examples_test.go`

- [ ] **Step 1: Write failing tests**

```go
// internal/avatar/select_examples_test.go
package avatar

import "testing"

func TestSelectExamples(t *testing.T) {
	comments := []RawComment{
		{Issue: "P-1", Date: "2026-03-01", Text: "Fixed the bug, deployed to staging."},
		{Issue: "P-2", Date: "2026-03-02", Text: "Blocked by P-100. Waiting on API schema."},
		{Issue: "P-3", Date: "2026-03-03", Text: "Looks good. Two things:\n- Fix error handling\n- Add test"},
		{Issue: "P-4", Date: "2026-03-04", Text: "Updated the PR with fixes."},
		{Issue: "P-5", Date: "2026-03-05", Text: "Is this ready for review?"},
	}
	examples := SelectCommentExamples(comments, 3)
	if len(examples) == 0 {
		t.Fatal("should select at least one example")
	}
	if len(examples) > 3 {
		t.Errorf("selected %d examples, max is 3", len(examples))
	}
	// Check diversity — should have different contexts
	contexts := map[string]bool{}
	for _, e := range examples {
		contexts[e.Context] = true
	}
	if len(contexts) < 2 {
		t.Error("examples should cover multiple contexts")
	}
}

func TestClassifyComment(t *testing.T) {
	tests := []struct {
		text string
		want string
	}{
		{"Fixed the bug. Deployed to staging.", "status_update"},
		{"Blocked by PROJ-100. Waiting on approval.", "blocker_report"},
		{"Looks good. Minor nit on line 42.", "review_feedback"},
		{"What's the expected behavior here?", "question"},
		{"Updated the config file.", "status_update"},
	}
	for _, tt := range tests {
		got := classifyComment(tt.text)
		if got != tt.want {
			t.Errorf("classifyComment(%q) = %q, want %q", tt.text, got, tt.want)
		}
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd /Users/quan.hoang/quanhh/quanhoang/jira-cli-v2 && go test ./internal/avatar/ -run "TestSelectExamples|TestClassifyComment" -v`
Expected: FAIL

- [ ] **Step 3: Implement select_examples.go**

Key logic:
- `classifyComment`: keyword-based context classification (blocker keywords → "blocker_report", review keywords → "review_feedback", question marks → "question", default → "status_update")
- `SelectCommentExamples`: group by context, from each group pick the comment closest to group's median word count, cap at maxExamples total with diversity preference
- `SelectDescriptionExamples`: pick one per issue type, closest to median length, cap at 3

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd /Users/quan.hoang/quanhh/quanhoang/jira-cli-v2 && go test ./internal/avatar/ -run "TestSelectExamples|TestClassifyComment" -v`
Expected: ALL PASS

- [ ] **Step 5: Commit**

```bash
git add internal/avatar/select_examples.go internal/avatar/select_examples_test.go
git commit -m "feat(avatar): add example selection with context classification"
```

---

## Task 7: Data Fetcher (Jira API Integration)

**Files:**
- Create: `internal/avatar/fetch.go`
- Test: `internal/avatar/fetch_test.go`

- [ ] **Step 1: Write failing tests with mock HTTP server**

```go
// internal/avatar/fetch_test.go
package avatar

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/sofq/jira-cli/internal/client"
	"github.com/sofq/jira-cli/internal/config"
)

func newTestClient(serverURL string, stdout, stderr *bytes.Buffer) *client.Client {
	return &client.Client{
		BaseURL:    serverURL,
		Auth:       config.AuthConfig{Type: "basic", Username: "u", Token: "t"},
		HTTPClient: &http.Client{},
		Stdout:     stdout,
		Stderr:     stderr,
		Paginate:   true,
	}
}

func TestResolveCurrentUser(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/rest/api/3/myself" {
			w.Write([]byte(`{"accountId":"abc123","emailAddress":"test@co.com","displayName":"Test User"}`))
			return
		}
		w.WriteHeader(404)
	}))
	defer ts.Close()
	var stdout, stderr bytes.Buffer
	c := newTestClient(ts.URL, &stdout, &stderr)
	user, err := ResolveUser(c, "")
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if user.AccountID != "abc123" {
		t.Errorf("accountId = %q", user.AccountID)
	}
}

func TestFetchUserComments(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Mock search endpoint returning issues with comments
		if r.URL.Path == "/rest/api/3/search/jql" {
			w.Write([]byte(`{"issues":[{"key":"P-1","fields":{"comment":{"comments":[{"author":{"accountId":"abc123"},"body":{"content":[{"content":[{"text":"test comment","type":"text"}],"type":"paragraph"}],"type":"doc","version":1},"created":"2026-03-01T10:00:00.000+0000"}]}}}],"nextPageToken":null,"isLast":true}`))
			return
		}
		w.WriteHeader(404)
	}))
	defer ts.Close()
	var stdout, stderr bytes.Buffer
	c := newTestClient(ts.URL, &stdout, &stderr)
	comments, err := FetchUserComments(c, "abc123", "2026-01-01", "2026-03-18")
	if err != nil {
		t.Fatalf("fetch: %v", err)
	}
	if len(comments) == 0 {
		t.Error("should return at least one comment")
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd /Users/quan.hoang/quanhh/quanhoang/jira-cli-v2 && go test ./internal/avatar/ -run "TestResolveCurrent|TestFetchUser" -v`
Expected: FAIL

- [ ] **Step 3: Implement fetch.go**

```go
// internal/avatar/fetch.go
package avatar

import (
	"encoding/json"
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"strings"

	"github.com/sofq/jira-cli/internal/client"
)

// JiraUser holds resolved user info.
type JiraUser struct {
	AccountID   string `json:"accountId"`
	Email       string `json:"emailAddress"`
	DisplayName string `json:"displayName"`
}

// RawComment holds a comment extracted from Jira.
type RawComment struct {
	Issue  string `json:"issue"`
	Date   string `json:"date"`
	Text   string `json:"text"`
	Author string `json:"author"`
}

// ResolveUser resolves the target user. If userFlag is empty, uses /myself.
// If userFlag is provided, searches by email or accountId.
func ResolveUser(c *client.Client, userFlag string) (*JiraUser, error) {
	if userFlag == "" {
		body, exitCode := c.Fetch(context.Background(), "GET", "/rest/api/3/myself", nil)
		if exitCode != 0 {
			return nil, fmt.Errorf("failed to fetch current user (exit %d)", exitCode)
		}
		var u JiraUser
		if err := json.Unmarshal(body, &u); err != nil {
			return nil, fmt.Errorf("parse myself response: %w", err)
		}
		return &u, nil
	}
	// Try user search
	query := url.Values{"query": {userFlag}}
	body, exitCode := c.Fetch(context.Background(), "GET", "/rest/api/3/user/search?"+query.Encode(), nil)
	if exitCode != 0 {
		return nil, fmt.Errorf("user search failed (exit %d)", exitCode)
	}
	var users []JiraUser
	if err := json.Unmarshal(body, &users); err != nil {
		return nil, fmt.Errorf("parse user search: %w", err)
	}
	if len(users) == 0 {
		return nil, fmt.Errorf("no user found matching %q", userFlag)
	}
	return &users[0], nil
}

// FetchUserComments fetches comments authored by the user within the time window.
// Uses reporter/assignee JQL to find relevant issues, then filters comments client-side by accountID.
// This works for any user, not just the authenticated user (currentUser() only matches auth user).
func FetchUserComments(c *client.Client, accountID, from, to string) ([]RawComment, error) {
	jql := fmt.Sprintf("(reporter = %q OR assignee was %q) AND updated >= %q AND updated <= %q ORDER BY updated DESC", accountID, accountID, from, to)
	body, exitCode := c.Fetch(context.Background(), "POST", "/rest/api/3/search/jql",
		strings.NewReader(fmt.Sprintf(`{"jql":%q,"fields":["comment"],"maxResults":50}`, jql)))
	if exitCode != 0 {
		return nil, fmt.Errorf("search failed (exit %d)", exitCode)
	}
	var result struct {
		Issues []struct {
			Key    string `json:"key"`
			Fields struct {
				Comment struct {
					Comments []struct {
						Author struct {
							AccountID string `json:"accountId"`
						} `json:"author"`
						Body    json.RawMessage `json:"body"`
						Created string          `json:"created"`
					} `json:"comments"`
				} `json:"comment"`
			} `json:"fields"`
		} `json:"issues"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("parse search: %w", err)
	}
	var comments []RawComment
	for _, issue := range result.Issues {
		for _, c := range issue.Fields.Comment.Comments {
			if c.Author.AccountID == accountID {
				text := extractTextFromADF(c.Body)
				comments = append(comments, RawComment{
					Issue:  issue.Key,
					Date:   c.Created,
					Text:   text,
					Author: c.Author.AccountID,
				})
			}
		}
	}
	return comments, nil
}

// extractTextFromADF extracts plain text from Atlassian Document Format JSON.
func extractTextFromADF(adf json.RawMessage) string {
	var doc struct {
		Content []struct {
			Content []struct {
				Text string `json:"text"`
				Type string `json:"type"`
			} `json:"content"`
			Type string `json:"type"`
		} `json:"content"`
	}
	if err := json.Unmarshal(adf, &doc); err != nil {
		return string(adf) // fallback to raw
	}
	var parts []string
	for _, block := range doc.Content {
		var line []string
		for _, inline := range block.Content {
			if inline.Text != "" {
				line = append(line, inline.Text)
			}
		}
		if len(line) > 0 {
			parts = append(parts, strings.Join(line, ""))
		}
	}
	return strings.Join(parts, "\n")
}
```

Note: This is the initial implementation. Additional fetch functions (`FetchUserIssues`, `FetchUserChangelog`, `FetchUserWorklogs`) follow the same pattern — JQL query, parse response, filter by accountID.

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd /Users/quan.hoang/quanhh/quanhoang/jira-cli-v2 && go test ./internal/avatar/ -run "TestResolveCurrent|TestFetchUser" -v`
Expected: ALL PASS

- [ ] **Step 5: Add remaining fetch functions (issues, changelog, worklogs) with tests**

Follow the same pattern: mock HTTP server, test each function independently.

- [ ] **Step 6: Commit**

```bash
git add internal/avatar/fetch.go internal/avatar/fetch_test.go
git commit -m "feat(avatar): add Jira API data fetcher with adaptive window"
```

---

## Task 8: Extraction Orchestrator

**Files:**
- Create: `internal/avatar/extract.go`
- Test: `internal/avatar/extract_test.go`

- [ ] **Step 1: Write failing integration test**

```go
// internal/avatar/extract_test.go
package avatar

import (
	"testing"
)

func TestExtract_InsufficientData(t *testing.T) {
	// Test with a mock server that returns very little data
	// Should return an error indicating insufficient data
	opts := ExtractOptions{
		MinComments: 10,
		MinUpdates:  5,
		MaxWindow:   "1m",
	}
	_, err := Extract(nil, "", opts) // nil client should fail gracefully
	if err == nil {
		t.Error("expected error with nil client")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd /Users/quan.hoang/quanhh/quanhoang/jira-cli-v2 && go test ./internal/avatar/ -run TestExtract -v`
Expected: FAIL

- [ ] **Step 3: Implement extract.go**

```go
// internal/avatar/extract.go
package avatar

import (
	"fmt"
	"time"

	"github.com/sofq/jira-cli/internal/client"
)

// ExtractOptions configures the extraction process.
type ExtractOptions struct {
	UserFlag    string
	MinComments int
	MinUpdates  int
	MaxWindow   string // e.g., "6m" for 6 months
	NoCache     bool
}

// DefaultExtractOptions returns sensible defaults.
func DefaultExtractOptions() ExtractOptions {
	return ExtractOptions{
		MinComments: 50,
		MinUpdates:  30,
		MaxWindow:   "6m",
	}
}

// Extract performs the full extraction pipeline.
func Extract(c *client.Client, userFlag string, opts ExtractOptions) (*Extraction, error) {
	if c == nil {
		return nil, fmt.Errorf("client is required")
	}
	// 1. Resolve user
	user, err := ResolveUser(c, userFlag)
	if err != nil {
		return nil, fmt.Errorf("resolve user: %w", err)
	}
	// 2. Adaptive window: start at 2 weeks, double until targets met or max reached
	maxDur, err := parseWindowDuration(opts.MaxWindow)
	if err != nil {
		return nil, fmt.Errorf("parse max_window: %w", err)
	}
	now := time.Now().UTC()
	windowDur := 14 * 24 * time.Hour // 2 weeks
	var comments []RawComment
	var issues []IssueFields
	var changelog []ChangelogEntry
	for windowDur <= maxDur {
		from := now.Add(-windowDur).Format("2006-01-02")
		to := now.Format("2006-01-02")
		comments, err = FetchUserComments(c, user.AccountID, from, to)
		if err != nil {
			return nil, fmt.Errorf("fetch comments: %w", err)
		}
		// TODO: fetch issues, changelog, worklogs in parallel
		if len(comments) >= opts.MinComments {
			break
		}
		windowDur *= 2
	}
	// 3. Check minimum thresholds
	if len(comments) < 10 {
		return nil, &InsufficientDataError{
			Found:    DataPoints{Comments: len(comments)},
			Required: DataPoints{Comments: 10, IssuesCreated: 5},
		}
	}
	// 4. Run analyzers
	commentTexts := make([]string, len(comments))
	for i, c := range comments {
		commentTexts[i] = c.Text
	}
	from := now.Add(-windowDur).Format("2006-01-02")
	extraction := &Extraction{
		Version: 1,
		Meta: ExtractionMeta{
			User:        user.Email,
			DisplayName: user.DisplayName,
			ExtractedAt: now.Format(time.RFC3339),
			Window:      TimeWindow{From: from, To: now.Format("2006-01-02")},
			DataPoints:  DataPoints{Comments: len(comments)},
		},
		Writing: WritingAnalysis{
			Comments: AnalyzeComments(commentTexts),
		},
		Examples: ExtractionExamples{
			Comments: SelectCommentExamples(comments, 10),
		},
	}
	return extraction, nil
}

// InsufficientDataError is returned when not enough data is found.
type InsufficientDataError struct {
	Found    DataPoints
	Required DataPoints
}

func (e *InsufficientDataError) Error() string {
	return fmt.Sprintf("insufficient data: found %d comments (need %d), %d issues (need %d)",
		e.Found.Comments, e.Required.Comments, e.Found.IssuesCreated, e.Required.IssuesCreated)
}

func parseWindowDuration(s string) (time.Duration, error) {
	if s == "" {
		return 6 * 30 * 24 * time.Hour, nil // default 6 months
	}
	// Support: "2w", "1m", "3m", "6m"
	if len(s) < 2 {
		return 0, fmt.Errorf("invalid window: %q", s)
	}
	unit := s[len(s)-1]
	numStr := s[:len(s)-1]
	var num int
	if _, err := fmt.Sscanf(numStr, "%d", &num); err != nil {
		return 0, fmt.Errorf("invalid window number: %q", s)
	}
	switch unit {
	case 'w':
		return time.Duration(num) * 7 * 24 * time.Hour, nil
	case 'm':
		return time.Duration(num) * 30 * 24 * time.Hour, nil
	default:
		return 0, fmt.Errorf("unsupported window unit: %c (use w or m)", unit)
	}
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd /Users/quan.hoang/quanhh/quanhoang/jira-cli-v2 && go test ./internal/avatar/ -run TestExtract -v`
Expected: ALL PASS

- [ ] **Step 5: Add full integration test with mock server returning realistic data**

Test the complete pipeline: mock server → extraction JSON → validate all fields populated.

- [ ] **Step 6: Commit**

```bash
git add internal/avatar/extract.go internal/avatar/extract_test.go
git commit -m "feat(avatar): add extraction orchestrator with adaptive window"
```

---

## Task 9: Local Profile Builder

**Files:**
- Create: `internal/avatar/build_local.go`
- Create: `internal/avatar/build.go`
- Test: `internal/avatar/build_local_test.go`
- Create: `internal/avatar/testdata/extraction_sample.json`
- Create: `internal/avatar/testdata/profile_expected.yaml`

- [ ] **Step 1: Create golden file test data**

Create `internal/avatar/testdata/extraction_sample.json` with a complete realistic extraction. Create `internal/avatar/testdata/profile_expected.yaml` with the expected local engine output.

- [ ] **Step 2: Write failing tests**

```go
// internal/avatar/build_local_test.go
package avatar

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestBuildLocal(t *testing.T) {
	extraction := &Extraction{
		Version: 1,
		Meta:    ExtractionMeta{User: "test@co.com", DisplayName: "Test User"},
		Writing: WritingAnalysis{
			Comments: CommentStats{
				AvgLengthWords:    32,
				MedianLengthWords: 24,
				Formatting:        FormattingStats{UsesBullets: 0.65, UsesCodeBlocks: 0.12},
				Vocabulary:        VocabularyStats{SignOffs: []string{"thanks"}},
				ToneSignals:       ToneSignals{QuestionRatio: 0.18},
			},
		},
		Workflow: WorkflowAnalysis{
			FieldPreferences: FieldPreferences{
				AlwaysSets:      []string{"priority", "labels"},
				DefaultPriority: "Medium",
				CommonLabels:    []string{"backend"},
			},
		},
	}
	profile, err := BuildLocal(extraction, nil)
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	if profile.Version != 1 {
		t.Errorf("version = %d", profile.Version)
	}
	if profile.StyleGuide.Writing == "" {
		t.Error("writing section is empty")
	}
	if !strings.Contains(profile.StyleGuide.Writing, "Test User") {
		t.Error("writing section should mention user name")
	}
	if profile.Defaults.Priority != "Medium" {
		t.Errorf("default priority = %q", profile.Defaults.Priority)
	}
}

func TestBuildLocal_GoldenFile(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("testdata", "extraction_sample.json"))
	if err != nil {
		t.Fatalf("golden file must exist: %v", err)
	}
	var extraction Extraction
	if err := json.Unmarshal(data, &extraction); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	profile, err := BuildLocal(&extraction, nil)
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	// Verify profile is well-formed
	if profile.StyleGuide.Writing == "" || profile.StyleGuide.Workflow == "" || profile.StyleGuide.Interaction == "" {
		t.Error("all style guide sections should be non-empty")
	}
}
```

- [ ] **Step 3: Run tests to verify they fail**

Run: `cd /Users/quan.hoang/quanhh/quanhoang/jira-cli-v2 && go test ./internal/avatar/ -run "TestBuildLocal" -v`
Expected: FAIL

- [ ] **Step 4: Implement build_local.go and build.go**

`build_local.go`: Go `text/template` engine that generates prose from extraction stats. Three template functions (one per section) that produce style guide text.

`build.go`: Orchestrator that selects engine based on config, runs build, applies overrides, validates output.

```go
// internal/avatar/build.go
package avatar

import (
	"fmt"

	"github.com/sofq/jira-cli/internal/config"
)

// BuildOptions configures the profile build.
type BuildOptions struct {
	Engine string // "local" or "llm"
	LLMCmd string
	Yes    bool // skip consent prompt
}

// Build generates a Profile from an Extraction.
func Build(extraction *Extraction, avatarCfg *config.AvatarConfig, opts BuildOptions) (*Profile, error) {
	engine := opts.Engine
	if engine == "" && avatarCfg != nil {
		engine = avatarCfg.Engine
	}
	if engine == "" {
		engine = "local"
	}
	var overrides []string
	if avatarCfg != nil {
		for _, v := range avatarCfg.Overrides {
			overrides = append(overrides, v)
		}
	}
	switch engine {
	case "local":
		return BuildLocal(extraction, overrides)
	case "llm":
		llmCmd := opts.LLMCmd
		if llmCmd == "" && avatarCfg != nil {
			llmCmd = avatarCfg.LLMCmd
		}
		if llmCmd == "" {
			return nil, fmt.Errorf("llm engine requires --llm-cmd or avatar.llm_cmd in config")
		}
		profile, err := BuildLLM(extraction, llmCmd, overrides)
		if err != nil {
			// Fallback to local engine
			// Caller must pass stderr writer; log fallback warning
		// fmt.Fprintf(stderr, `{"warning":"llm_fallback","reason":%q}`, err.Error())
			return BuildLocal(extraction, overrides)
		}
		return profile, nil
	default:
		return nil, fmt.Errorf("unknown engine: %q (use 'local' or 'llm')", engine)
	}
}
```

- [ ] **Step 5: Run tests to verify they pass**

Run: `cd /Users/quan.hoang/quanhh/quanhoang/jira-cli-v2 && go test ./internal/avatar/ -run "TestBuildLocal" -v`
Expected: ALL PASS

- [ ] **Step 6: Commit**

```bash
git add internal/avatar/build.go internal/avatar/build_local.go internal/avatar/build_local_test.go internal/avatar/testdata/
git commit -m "feat(avatar): add local profile builder with Go templates"
```

---

## Task 10: LLM Profile Builder

**Files:**
- Create: `internal/avatar/build_llm.go`
- Create: `internal/avatar/prompts/build_profile.txt`
- Test: `internal/avatar/build_llm_test.go`

- [ ] **Step 1: Write failing tests**

```go
// internal/avatar/build_llm_test.go
package avatar

import (
	"os"
	"path/filepath"
	"testing"
)

func TestBuildLLM_MockCommand(t *testing.T) {
	// Create a mock script that echoes valid profile YAML
	dir := t.TempDir()
	script := filepath.Join(dir, "mock_llm.sh")
	os.WriteFile(script, []byte(`#!/bin/sh
cat <<'EOF'
version: 1
user: "test@co.com"
display_name: "Test User"
generated_at: "2026-03-18T10:00:00Z"
engine: "llm"
style_guide:
  writing: "Writes concise comments."
  workflow: "Sets priority."
  interaction: "Responds quickly."
defaults:
  priority: "Medium"
examples: []
EOF
`), 0o755)

	extraction := &Extraction{Version: 1, Meta: ExtractionMeta{User: "test@co.com"}}
	profile, err := BuildLLM(extraction, script, nil)
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	if profile.Engine != "llm" {
		t.Errorf("engine = %q", profile.Engine)
	}
}

func TestBuildLLM_InvalidOutput(t *testing.T) {
	dir := t.TempDir()
	script := filepath.Join(dir, "bad_llm.sh")
	os.WriteFile(script, []byte("#!/bin/sh\necho 'not valid yaml: [[['\n"), 0o755)

	extraction := &Extraction{Version: 1}
	_, err := BuildLLM(extraction, script, nil)
	if err == nil {
		t.Error("expected error for invalid YAML output")
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd /Users/quan.hoang/quanhh/quanhoang/jira-cli-v2 && go test ./internal/avatar/ -run "TestBuildLLM" -v`
Expected: FAIL

- [ ] **Step 3: Implement build_llm.go**

Key logic: marshal extraction to JSON, pipe to `exec.Command(llmCmd)` via stdin, capture stdout, unmarshal YAML, validate profile schema. Also create `internal/avatar/prompts/build_profile.txt` with the default LLM prompt.

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd /Users/quan.hoang/quanhh/quanhoang/jira-cli-v2 && go test ./internal/avatar/ -run "TestBuildLLM" -v`
Expected: ALL PASS

- [ ] **Step 5: Commit**

```bash
git add internal/avatar/build_llm.go internal/avatar/build_llm_test.go internal/avatar/prompts/
git commit -m "feat(avatar): add LLM profile builder with external command piping"
```

---

## Task 11: Prompt Formatter

**Files:**
- Create: `internal/avatar/prompt.go`
- Test: `internal/avatar/prompt_test.go`

- [ ] **Step 1: Write failing tests**

```go
// internal/avatar/prompt_test.go
package avatar

import (
	"strings"
	"testing"
)

func TestFormatPrompt_Prose(t *testing.T) {
	p := &Profile{
		Version:     1,
		DisplayName: "Test User",
		StyleGuide: StyleGuide{
			Writing:     "Writes concisely.",
			Workflow:    "Sets priority.",
			Interaction: "Responds fast.",
		},
		Overrides: []string{"No emoji"},
		Examples: []ProfileExample{
			{Context: "status_update", Source: "P-1", Text: "Fixed the bug."},
		},
	}
	out := FormatPrompt(p, PromptOptions{Format: "prose"})
	if !strings.Contains(out, "Writes concisely") {
		t.Error("should contain writing section")
	}
	if !strings.Contains(out, "No emoji") {
		t.Error("should contain overrides")
	}
	if !strings.Contains(out, "Fixed the bug") {
		t.Error("should contain examples")
	}
}

func TestFormatPrompt_SectionFilter(t *testing.T) {
	p := &Profile{
		StyleGuide: StyleGuide{
			Writing:     "Writing section.",
			Workflow:    "Workflow section.",
			Interaction: "Interaction section.",
		},
	}
	out := FormatPrompt(p, PromptOptions{Format: "prose", Sections: []string{"writing"}})
	if !strings.Contains(out, "Writing section") {
		t.Error("should contain writing")
	}
	if strings.Contains(out, "Workflow section") {
		t.Error("should NOT contain workflow when filtered")
	}
}

func TestFormatPrompt_Redact(t *testing.T) {
	p := &Profile{
		StyleGuide: StyleGuide{Writing: "Collaborates with alice@co.com on PROJ-123."},
		Examples:   []ProfileExample{{Source: "PROJ-456", Text: "email bob@co.com"}},
	}
	out := FormatPrompt(p, PromptOptions{Format: "prose", Redact: true})
	if strings.Contains(out, "alice@co.com") {
		t.Error("should redact email")
	}
	if strings.Contains(out, "PROJ-123") {
		t.Error("should redact issue key")
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd /Users/quan.hoang/quanhh/quanhoang/jira-cli-v2 && go test ./internal/avatar/ -run "TestFormatPrompt" -v`
Expected: FAIL

- [ ] **Step 3: Implement prompt.go**

```go
// internal/avatar/prompt.go
package avatar

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
)

// PromptOptions configures prompt output.
type PromptOptions struct {
	Format   string   // "prose", "json", "both"
	Sections []string // filter to specific sections; empty = all
	Redact   bool
}

var (
	emailRedactRe = regexp.MustCompile(`\b[A-Za-z0-9._%+-]+@[A-Za-z0-9.-]+\.[A-Za-z]{2,}\b`)
	issueRedactRe = regexp.MustCompile(`\b[A-Z][A-Z0-9]+-\d+\b`)
)

// FormatPrompt formats a Profile for agent consumption.
func FormatPrompt(p *Profile, opts PromptOptions) string {
	if opts.Format == "" {
		opts.Format = "prose"
	}
	switch opts.Format {
	case "json":
		data, _ := json.MarshalIndent(p, "", "  ")
		return maybeRedact(string(data), opts.Redact)
	case "both":
		prose := formatProse(p, opts.Sections)
		data, _ := json.MarshalIndent(p, "", "  ")
		return maybeRedact(prose+"\n\n---\n\n"+string(data), opts.Redact)
	default: // prose
		return maybeRedact(formatProse(p, opts.Sections), opts.Redact)
	}
}

func formatProse(p *Profile, sections []string) string {
	include := func(name string) bool {
		if len(sections) == 0 {
			return true
		}
		for _, s := range sections {
			if s == name {
				return true
			}
		}
		return false
	}
	var b strings.Builder
	if p.DisplayName != "" {
		fmt.Fprintf(&b, "# Style Profile: %s\n\n", p.DisplayName)
	}
	if include("writing") && p.StyleGuide.Writing != "" {
		fmt.Fprintf(&b, "## Writing Style\n%s\n\n", p.StyleGuide.Writing)
	}
	if include("workflow") && p.StyleGuide.Workflow != "" {
		fmt.Fprintf(&b, "## Workflow Patterns\n%s\n\n", p.StyleGuide.Workflow)
	}
	if include("interaction") && p.StyleGuide.Interaction != "" {
		fmt.Fprintf(&b, "## Interaction Patterns\n%s\n\n", p.StyleGuide.Interaction)
	}
	if len(p.Overrides) > 0 {
		b.WriteString("## Hard Rules (always follow)\n")
		for _, o := range p.Overrides {
			fmt.Fprintf(&b, "- %s\n", o)
		}
		b.WriteString("\n")
	}
	if len(p.Examples) > 0 {
		b.WriteString("## Representative Examples\n\n")
		for _, e := range p.Examples {
			fmt.Fprintf(&b, "### %s (from %s)\n%s\n\n", e.Context, e.Source, e.Text)
		}
	}
	return strings.TrimSpace(b.String())
}

func maybeRedact(s string, redact bool) string {
	if !redact {
		return s
	}
	s = emailRedactRe.ReplaceAllString(s, "[EMAIL]")
	s = issueRedactRe.ReplaceAllString(s, "[ISSUE]")
	return s
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd /Users/quan.hoang/quanhh/quanhoang/jira-cli-v2 && go test ./internal/avatar/ -run "TestFormatPrompt" -v`
Expected: ALL PASS

- [ ] **Step 5: Commit**

```bash
git add internal/avatar/prompt.go internal/avatar/prompt_test.go
git commit -m "feat(avatar): add prompt formatter with prose/json/redact modes"
```

---

## Task 12: CLI Commands

**Files:**
- Create: `cmd/avatar.go`
- Modify: `cmd/root.go:220-231` (register avatarCmd)
- Test: `cmd/avatar_test.go`

- [ ] **Step 1: Write failing tests for CLI commands**

```go
// cmd/avatar_test.go
package cmd

import (
	"bytes"
	"context"
	"encoding/json"
	"testing"

	"github.com/sofq/jira-cli/internal/client"
	"github.com/spf13/cobra"
)

func TestAvatarStatusCmd_NoProfile(t *testing.T) {
	var stdout, stderr bytes.Buffer
	c := newTestClient("http://unused", &stdout, &stderr)
	cmd := newAvatarStatusCmd(c, t.TempDir())
	err := cmd.RunE(cmd, nil)
	if err != nil {
		// Check that stdout has {"exists": false}
	}
	var result map[string]any
	if err := json.Unmarshal(stdout.Bytes(), &result); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if result["exists"] != false {
		t.Errorf("exists = %v, want false", result["exists"])
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd /Users/quan.hoang/quanhh/quanhoang/jira-cli-v2 && go test ./cmd/ -run TestAvatarStatus -v`
Expected: FAIL

- [ ] **Step 3: Implement cmd/avatar.go**

Follow the same patterns as `cmd/workflow.go`:
- Parent `avatarCmd` with subcommands: `extractCmd`, `buildCmd`, `promptCmd`, `showCmd`, `editCmd`, `refreshCmd`, `statusCmd`
- Each command uses `RunE`, extracts client from context (for commands that need it), writes JSON to stdout
- `extract` and `build` need the client; `prompt`, `show`, `status`, `edit` work from local files only
- Register all flags per the spec's command interface

- [ ] **Step 4: Register avatarCmd in root.go**

Add to `cmd/root.go` init section:

```go
mergeCommand(rootCmd, avatarCmd)
```

Add "avatar" to the list of commands that skip client injection for subcommands that don't need it (status, show, prompt, edit).

- [ ] **Step 5: Run tests to verify they pass**

Run: `cd /Users/quan.hoang/quanhh/quanhoang/jira-cli-v2 && go test ./cmd/ -run TestAvatar -v`
Expected: ALL PASS

- [ ] **Step 6: Run full test suite**

Run: `cd /Users/quan.hoang/quanhh/quanhoang/jira-cli-v2 && go test ./... -count=1`
Expected: ALL PASS

- [ ] **Step 7: Commit**

```bash
git add cmd/avatar.go cmd/avatar_test.go cmd/root.go
git commit -m "feat(avatar): add CLI commands (extract, build, prompt, show, edit, refresh, status)"
```

---

## Task 13: End-to-End Integration Test

**Files:**
- Test: `internal/avatar/e2e_test.go`

- [ ] **Step 1: Write end-to-end test**

Test the full pipeline with a mock Jira server:
1. Mock server returns realistic user, issues, comments, changelog data
2. Call `Extract` → verify extraction.json is well-formed
3. Call `Build` with local engine → verify profile.yaml is well-formed
4. Call `FormatPrompt` → verify output contains all expected sections

- [ ] **Step 2: Run test**

Run: `cd /Users/quan.hoang/quanhh/quanhoang/jira-cli-v2 && go test ./internal/avatar/ -run TestE2E -v`
Expected: PASS

- [ ] **Step 3: Run full test suite + lint**

Run: `cd /Users/quan.hoang/quanhh/quanhoang/jira-cli-v2 && make test && make lint`
Expected: ALL PASS

- [ ] **Step 4: Commit**

```bash
git add internal/avatar/e2e_test.go
git commit -m "test(avatar): add end-to-end integration test"
```

---

## Summary

| Task | Description | Estimated Steps |
|------|-------------|-----------------|
| 1 | Schema types & config integration | 9 |
| 2 | Storage layer | 5 |
| 3 | Writing analyzer | 5 |
| 4 | Workflow analyzer | 5 |
| 5 | Interaction analyzer | 5 |
| 6 | Example selector | 5 |
| 7 | Data fetcher (Jira API) | 6 |
| 8 | Extraction orchestrator | 6 |
| 9 | Local profile builder | 6 |
| 10 | LLM profile builder | 5 |
| 11 | Prompt formatter | 5 |
| 12 | CLI commands | 7 |
| 13 | E2E integration test | 4 |

**Total: 13 tasks, ~73 steps**

Tasks 1-2 must be completed first (foundation). Tasks 3-6 are independent and can be parallelized (all shared input types live in types.go from Task 1). Task 7 depends on types (Task 1). Task 8 depends on 3-7. Tasks 9-11 depend on 1-2. Task 12 depends on all internal packages. Task 13 is the final validation.

## Important Implementation Notes

**Context parameter**: All calls to `c.Fetch()` MUST pass `context.Background()` (not `nil`) as the first argument. Passing nil will panic in `http.NewRequestWithContext`.

**Stderr writer**: The `Build` orchestrator and CLI commands must pass an `io.Writer` for stderr warnings (LLM fallback, stale profile, consent). Never use `fmt.Fprintf(nil, ...)`. Accept `stderr io.Writer` as a parameter or use `os.Stderr`.

**Client skip in root.go**: The existing `skipClientCommands` mechanism skips ALL subcommands when a parent is listed. For avatar, add individual subcommand names to the skip map: `"status": true, "show": true, "prompt": true, "edit": true`. These names won't conflict because the skip check also verifies the parent is "avatar". Alternatively, `extract` and `build` can handle their own client setup in RunE if needed.

**JQL for non-self users**: `currentUser()` in JQL only matches the authenticated user. When `--user` specifies a different user, use the workaround: search for issues where the user is reporter/assignee, then filter comments client-side by `accountId`. Or use the Jira REST API to search for issues updated by the user.

**Consent mechanism for LLM engine**: On first `jr avatar build --engine llm`, check if `avatar.llm_consent` is set in the profile config. If not, print consent prompt to stderr, read stdin for y/n, and save `"llm_consent": true` to config. The `--yes` flag bypasses this.

**Parallel fetch**: Use `sync.WaitGroup` with a `chan struct{}` semaphore of size 3 to cap concurrent requests. Fetch comments, issues, changelog, and worklogs concurrently.

**Stale profile warning**: `FormatPrompt` should accept a `dir` parameter to check `ProfileAgeDays()`. If >30 days, emit `{"warning":"profile_stale","age_days":N}` to stderr before writing the prompt to stdout.

**N-gram counting**: Count bigrams/trigrams per-comment (deduplicate within a single comment before incrementing the global counter) to avoid one verbose comment inflating phrase frequency.
