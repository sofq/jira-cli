// Package avatar contains types and logic for the avatar feature, which
// analyses a user's Jira activity to generate a writing/workflow profile.
package avatar

import "time"

// ---------------------------------------------------------------------------
// Extraction Schema (Phase 1)
// ---------------------------------------------------------------------------

// Extraction is the top-level document produced by the extraction phase.
type Extraction struct {
	Version     string              `json:"version"`
	Meta        ExtractionMeta      `json:"meta"`
	Writing     WritingAnalysis     `json:"writing"`
	Workflow    WorkflowAnalysis    `json:"workflow"`
	Interaction InteractionAnalysis `json:"interaction"`
	Examples    ExtractionExamples  `json:"examples"`
}

// ExtractionMeta records who was analysed and over what period.
type ExtractionMeta struct {
	User        string     `json:"user"`
	DisplayName string     `json:"display_name"`
	ExtractedAt time.Time  `json:"extracted_at"`
	Window      TimeWindow `json:"window"`
	DataPoints  DataPoints `json:"data_points"`
}

// TimeWindow represents a half-open [From, To) time range.
type TimeWindow struct {
	From time.Time `json:"from"`
	To   time.Time `json:"to"`
}

// DataPoints counts the raw events that were analysed.
type DataPoints struct {
	Comments      int `json:"comments"`
	IssuesCreated int `json:"issues_created"`
	Transitions   int `json:"transitions"`
	Worklogs      int `json:"worklogs"`
}

// WritingAnalysis aggregates statistics about the user's written content.
type WritingAnalysis struct {
	Comments     CommentStats     `json:"comments"`
	Descriptions DescriptionStats `json:"descriptions"`
}

// CommentStats holds metrics derived from issue comments.
type CommentStats struct {
	AvgLengthWords    float64          `json:"avg_length_words"`
	MedianLengthWords float64          `json:"median_length_words"`
	LengthDist        LengthDist       `json:"length_dist"`
	Formatting        FormattingStats  `json:"formatting"`
	Vocabulary        VocabularyStats  `json:"vocabulary"`
	ToneSignals       ToneSignals      `json:"tone_signals"`
	SentencePatterns  SentencePatterns `json:"sentence_patterns"`
	FormalityScore    float64          `json:"formality_score"`
}

// SentencePatterns captures how the user structures sentences.
type SentencePatterns struct {
	FragmentRatio     float64 `json:"fragment_ratio"`      // ratio of fragments (no verb, ≤4 words) to total sentences
	AvgWordsPerSent   float64 `json:"avg_words_per_sent"`  // average words per sentence
	UsesContractions  float64 `json:"uses_contractions"`   // ratio of comments with contractions
	StartsWithSubject float64 `json:"starts_with_subject"` // ratio starting with I/We/The/This
}

// LengthDist breaks down the percentage of short/medium/long texts.
type LengthDist struct {
	ShortPct  float64 `json:"short_pct"`
	MediumPct float64 `json:"medium_pct"`
	LongPct   float64 `json:"long_pct"`
}

// FormattingStats records the fraction of texts that use various formatting elements.
type FormattingStats struct {
	UsesBullets    float64 `json:"uses_bullets"`
	UsesCodeBlocks float64 `json:"uses_code_blocks"`
	UsesHeadings   float64 `json:"uses_headings"`
	UsesEmoji      float64 `json:"uses_emoji"`
	UsesMentions   float64 `json:"uses_mentions"`
}

// VocabularyStats captures repeated phrases and idiomatic language.
type VocabularyStats struct {
	CommonPhrases []string `json:"common_phrases"`
	Jargon        []string `json:"jargon"`
	SignOffs      []string `json:"sign_offs"`
}

// ToneSignals measures stylistic ratios in the user's writing.
type ToneSignals struct {
	QuestionRatio    float64 `json:"question_ratio"`
	ExclamationRatio float64 `json:"exclamation_ratio"`
	FirstPersonRatio float64 `json:"first_person_ratio"`
	ImperativeRatio  float64 `json:"imperative_ratio"`
}

// DescriptionStats holds metrics derived from issue descriptions.
type DescriptionStats struct {
	AvgLengthWords    float64         `json:"avg_length_words"`
	StructurePatterns []string        `json:"structure_patterns"`
	Formatting        FormattingStats `json:"formatting"`
}

// WorkflowAnalysis aggregates statistics about the user's workflow behaviour.
type WorkflowAnalysis struct {
	FieldPreferences   FieldPreferences   `json:"field_preferences"`
	TransitionPatterns TransitionPatterns `json:"transition_patterns"`
	IssueCreation      IssueCreation      `json:"issue_creation"`
}

// FieldPreferences records which issue fields the user typically fills in.
type FieldPreferences struct {
	AlwaysSets       []string `json:"always_sets"`
	RarelySets       []string `json:"rarely_sets"`
	DefaultPriority  string   `json:"default_priority"`
	CommonLabels     []string `json:"common_labels"`
	CommonComponents []string `json:"common_components"`
}

// TransitionPatterns captures how the user moves issues through the board.
type TransitionPatterns struct {
	AvgTimeInStatus         map[string]string `json:"avg_time_in_status"`
	CommonSequences         [][]string        `json:"common_sequences"`
	AssignsBeforeTransition bool              `json:"assigns_before_transition"`
}

// IssueCreation records patterns in how the user creates issues.
type IssueCreation struct {
	TypesCreated        map[string]float64 `json:"types_created"`
	AvgSubtasksPerStory float64            `json:"avg_subtasks_per_story"`
}

// InteractionAnalysis aggregates statistics about the user's interaction style.
type InteractionAnalysis struct {
	ResponsePatterns  ResponsePatterns  `json:"response_patterns"`
	MentionHabits     MentionHabits     `json:"mention_habits"`
	EscalationSignals EscalationSignals `json:"escalation_signals"`
	Collaboration     Collaboration     `json:"collaboration"`
}

// ResponsePatterns captures how quickly and often the user replies.
type ResponsePatterns struct {
	MedianReplyTime       string  `json:"median_reply_time"`
	RepliesToOwnIssuesPct float64 `json:"replies_to_own_issues_pct"`
	RepliesToOthersPct    float64 `json:"replies_to_others_pct"`
	AvgThreadDepth        float64 `json:"avg_thread_depth"`
}

// MentionHabits records who the user mentions and in what contexts.
type MentionHabits struct {
	FrequentlyMentions []string `json:"frequently_mentions"`
	MentionContext     []string `json:"mention_context"`
}

// EscalationSignals describes how the user signals urgency or blockers.
type EscalationSignals struct {
	BlockerKeywords             []string `json:"blocker_keywords"`
	EscalationPattern           string   `json:"escalation_pattern"`
	AvgCommentsBeforeEscalation float64  `json:"avg_comments_before_escalation"`
}

// Collaboration records scheduling and collaboration patterns.
type Collaboration struct {
	ActiveHours      ActiveHours    `json:"active_hours"`
	PeakActivityDays []string       `json:"peak_activity_days"`
	WorklogHabits    WorklogHabits  `json:"worklog_habits"`
}

// ActiveHours describes when the user is typically active.
type ActiveHours struct {
	Start    string `json:"start"`
	End      string `json:"end"`
	Timezone string `json:"timezone"`
}

// WorklogHabits describes how the user logs work.
type WorklogHabits struct {
	LogsDaily         bool    `json:"logs_daily"`
	AvgEntriesPerWeek float64 `json:"avg_entries_per_week"`
}

// ExtractionExamples holds representative text samples from the extraction.
type ExtractionExamples struct {
	Comments     []CommentExample     `json:"comments"`
	Descriptions []DescriptionExample `json:"descriptions"`
}

// CommentExample is a representative comment from the analysed period.
type CommentExample struct {
	Issue   string `json:"issue"`
	Date    string `json:"date"`
	Text    string `json:"text"`
	Context string `json:"context"`
}

// DescriptionExample is a representative issue description from the analysed period.
type DescriptionExample struct {
	Issue string `json:"issue"`
	Type  string `json:"type"`
	Text  string `json:"text"`
}

// ---------------------------------------------------------------------------
// Profile Schema (Phase 2)
// ---------------------------------------------------------------------------

// Profile is the top-level persona document produced from an Extraction.
type Profile struct {
	Version     string            `yaml:"version"      json:"version"`
	User        string            `yaml:"user"         json:"user"`
	DisplayName string            `yaml:"display_name" json:"display_name"`
	GeneratedAt string            `yaml:"generated_at" json:"generated_at"`
	Engine      string            `yaml:"engine"       json:"engine"`
	StyleGuide  StyleGuide        `yaml:"style_guide"  json:"style_guide"`
	Overrides   map[string]string `yaml:"overrides"    json:"overrides,omitempty"`
	Defaults    ProfileDefaults   `yaml:"defaults"     json:"defaults"`
	Examples    []ProfileExample  `yaml:"examples"     json:"examples,omitempty"`
}

// StyleGuide holds prose guidance for writing, workflow, and interaction.
type StyleGuide struct {
	Writing     string `yaml:"writing"     json:"writing"`
	Workflow    string `yaml:"workflow"    json:"workflow"`
	Interaction string `yaml:"interaction" json:"interaction"`
}

// ProfileDefaults holds preferred default values for issue fields.
type ProfileDefaults struct {
	Priority               string   `yaml:"priority"                 json:"priority"`
	Labels                 []string `yaml:"labels"                   json:"labels,omitempty"`
	Components             []string `yaml:"components"               json:"components,omitempty"`
	AssignSelfOnTransition bool     `yaml:"assign_self_on_transition" json:"assign_self_on_transition"`
}

// ProfileExample is a representative text sample included in the profile.
type ProfileExample struct {
	Context string `yaml:"context" json:"context"`
	Source  string `yaml:"source"  json:"source"`
	Text    string `yaml:"text"    json:"text"`
}

// ---------------------------------------------------------------------------
// Shared Input Types (used by analyzers and fetcher)
// ---------------------------------------------------------------------------

// RawComment is a comment as returned by the Jira API, before analysis.
type RawComment struct {
	Issue    string `json:"issue"`
	Date     string `json:"date"`
	Text     string `json:"text"`
	Author   string `json:"author"`
	Reporter string `json:"reporter"` // accountID of the issue reporter (for own-vs-others analysis)
}

// ChangelogEntry represents a single field change in an issue's history.
type ChangelogEntry struct {
	Issue     string `json:"issue"`
	Timestamp string `json:"timestamp"`
	Author    string `json:"author"`
	Field     string `json:"field"`
	From      string `json:"from"`
	To        string `json:"to"`
}

// IssueFields holds selected field values for an issue.
type IssueFields struct {
	Priority   string   `json:"priority"`
	Labels     []string `json:"labels"`
	Components []string `json:"components"`
	FixVersion string   `json:"fix_version"`
}

// CreatedIssue records summary information about an issue the user created.
type CreatedIssue struct {
	Key          string   `json:"key"`
	Type         string   `json:"type"`
	SubtaskCount int      `json:"subtask_count"`
	Description  string   `json:"description"`
	Priority     string   `json:"priority"`
	Labels       []string `json:"labels"`
	Components   []string `json:"components"`
	FixVersion   string   `json:"fix_version"`
}

// CommentRecord represents a comment in a thread, with parent context.
type CommentRecord struct {
	IssueOwner      string `json:"issue_owner"`
	Author          string `json:"author"`
	Timestamp       string `json:"timestamp"`
	ParentTimestamp string `json:"parent_timestamp"`
	Text            string `json:"text"`
}
