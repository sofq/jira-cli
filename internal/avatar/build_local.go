package avatar

import (
	"bytes"
	"fmt"
	"sort"
	"strings"
	"text/template"
	"time"
)

// writingTmplSrc is the Go template for generating the writing section.
const writingTmplSrc = `{{.DisplayName}} writes {{.LengthDesc}} comments — typically {{.MedianWords}} words.{{if .SentenceStyle}}
{{.SentenceStyle}}{{end}}{{if .FormalityDesc}}
Tone: {{.FormalityDesc}}.{{end}}{{if .BulletHigh}}
Frequently uses bullet points.{{end}}{{if .NoBullets}}
Never uses bullet points.{{end}}{{if .EmojiRare}}
Never uses emoji.{{end}}{{if .CodeHigh}}
Uses code blocks when referencing specific errors or config.{{end}}{{if .NoCode}}
Never uses code blocks.{{end}}{{if .NoHeadings}}
Never uses headings in comments.{{end}}{{if .NoMentions}}
Never uses @mentions.{{end}}{{if .SignOffs}}
Ends messages with {{.SignOffs}}.{{end}}{{if .StructurePatterns}}
Descriptions follow structured patterns: {{.StructurePatterns}}.{{end}}{{if .EmptyDescriptions}}
Typically leaves issue descriptions empty.{{end}}`

// workflowTmplSrc is the Go template for generating the workflow section.
const workflowTmplSrc = `{{.DisplayName}} always sets {{.AlwaysSets}} when creating issues.{{if .DefaultPriority}}
Default priority is {{.DefaultPriority}}.{{end}}{{if .CommonLabels}}
Common labels used: {{.CommonLabels}}.{{end}}{{if .TransitionSeq}}
Typical transition sequence: {{.TransitionSeq}}.{{end}}{{if .TopIssueTypes}}
Most commonly creates: {{.TopIssueTypes}}.{{end}}`

// interactionTmplSrc is the Go template for generating the interaction section.
const interactionTmplSrc = `{{.DisplayName}} has a median reply time of {{.MedianReplyTime}}.{{if .ReplyBias}}
{{.ReplyBias}}{{end}}{{if .Collaborators}}
Frequently mentions: {{.Collaborators}}.{{end}}{{if .EscalationStyle}}
{{.EscalationStyle}}{{end}}{{if .ActiveHours}}
Active hours: {{.ActiveHours}}.{{end}}{{if .PeakDays}}
Peak activity days: {{.PeakDays}}.{{end}}`

// writingData is the template data for the writing section.
type writingData struct {
	DisplayName       string
	LengthDesc        string
	MedianWords       int
	SentenceStyle     string
	FormalityDesc     string
	BulletHigh        bool
	NoBullets         bool
	EmojiRare         bool
	CodeHigh          bool
	NoCode            bool
	NoHeadings        bool
	NoMentions        bool
	SignOffs          string
	StructurePatterns string
	EmptyDescriptions bool
}

// workflowData is the template data for the workflow section.
type workflowData struct {
	DisplayName     string
	AlwaysSets      string
	DefaultPriority string
	CommonLabels    string
	TransitionSeq   string
	TopIssueTypes   string
}

// interactionData is the template data for the interaction section.
type interactionData struct {
	DisplayName     string
	MedianReplyTime string
	ReplyBias       string
	Collaborators   string
	EscalationStyle string
	ActiveHours     string
	PeakDays        string
}

var (
	writingTmpl     = template.Must(template.New("writing").Parse(writingTmplSrc))
	workflowTmpl    = template.Must(template.New("workflow").Parse(workflowTmplSrc))
	interactionTmpl = template.Must(template.New("interaction").Parse(interactionTmplSrc))
)

// lengthDescription returns a human-readable description of comment length.
func lengthDescription(median float64) string {
	switch {
	case median < 20:
		return "short"
	case median < 60:
		return "medium-length"
	default:
		return "long"
	}
}

// renderTemplate renders a template with given data into a string.
func renderTemplate(tmpl *template.Template, data any) (string, error) {
	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		return "", fmt.Errorf("render template %q: %w", tmpl.Name(), err)
	}
	return buf.String(), nil
}

// buildWritingSection generates the writing guidance prose.
func buildWritingSection(displayName string, w WritingAnalysis) (string, error) {
	comments := w.Comments
	desc := w.Descriptions

	signOffs := ""
	if len(comments.Vocabulary.SignOffs) > 0 {
		signOffs = strings.Join(comments.Vocabulary.SignOffs, ", ")
	}

	structurePatterns := ""
	if len(desc.StructurePatterns) > 0 {
		structurePatterns = strings.Join(desc.StructurePatterns, "; ")
	}

	// Sentence style description
	sentenceStyle := ""
	sp := comments.SentencePatterns
	if sp.FragmentRatio > 0.5 {
		sentenceStyle = "Prefers short fragments over full sentences (e.g. \"Done. Merging.\")."
	} else if sp.AvgWordsPerSent > 15 {
		sentenceStyle = "Writes in complete, detailed sentences."
	} else if sp.AvgWordsPerSent > 0 {
		sentenceStyle = fmt.Sprintf("Average sentence length: %.0f words.", sp.AvgWordsPerSent)
	}

	// Formality description
	formalityDesc := ""
	switch {
	case comments.FormalityScore >= 0.7:
		formalityDesc = "formal and professional"
	case comments.FormalityScore >= 0.45:
		formalityDesc = "neutral/balanced"
	case comments.FormalityScore >= 0.3:
		formalityDesc = "casual and direct"
	case comments.FormalityScore > 0:
		formalityDesc = "very casual and informal"
	}

	// Empty descriptions detection
	emptyDescriptions := desc.AvgLengthWords < 2

	data := writingData{
		DisplayName:       displayName,
		LengthDesc:        lengthDescription(comments.MedianLengthWords),
		MedianWords:       int(comments.MedianLengthWords),
		SentenceStyle:     sentenceStyle,
		FormalityDesc:     formalityDesc,
		BulletHigh:        comments.Formatting.UsesBullets > 0.5,
		NoBullets:         comments.Formatting.UsesBullets == 0,
		EmojiRare:         comments.Formatting.UsesEmoji < 0.05,
		CodeHigh:          comments.Formatting.UsesCodeBlocks > 0.1,
		NoCode:            comments.Formatting.UsesCodeBlocks == 0,
		NoHeadings:        comments.Formatting.UsesHeadings == 0,
		NoMentions:        comments.Formatting.UsesMentions == 0,
		SignOffs:          signOffs,
		StructurePatterns: structurePatterns,
		EmptyDescriptions: emptyDescriptions,
	}

	return renderTemplate(writingTmpl, data)
}

// topIssueTypes returns the top N issue types by frequency.
func topIssueTypes(types map[string]float64, n int) []string {
	type kv struct {
		key string
		val float64
	}
	var sorted []kv
	for k, v := range types {
		sorted = append(sorted, kv{k, v})
	}
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[j].val < sorted[i].val
	})
	var result []string
	for i := 0; i < n && i < len(sorted); i++ {
		result = append(result, sorted[i].key)
	}
	return result
}

// buildWorkflowSection generates the workflow guidance prose.
func buildWorkflowSection(displayName string, wf WorkflowAnalysis) (string, error) {
	fp := wf.FieldPreferences

	alwaysSets := "fields"
	if len(fp.AlwaysSets) > 0 {
		alwaysSets = strings.Join(fp.AlwaysSets, ", ")
	}

	commonLabels := ""
	if len(fp.CommonLabels) > 0 {
		commonLabels = strings.Join(fp.CommonLabels, ", ")
	}

	transitionSeq := ""
	if len(wf.TransitionPatterns.CommonSequences) > 0 {
		transitionSeq = strings.Join(wf.TransitionPatterns.CommonSequences[0], " → ")
	}

	topTypes := topIssueTypes(wf.IssueCreation.TypesCreated, 3)
	topIssueTypesStr := strings.Join(topTypes, ", ")

	data := workflowData{
		DisplayName:     displayName,
		AlwaysSets:      alwaysSets,
		DefaultPriority: fp.DefaultPriority,
		CommonLabels:    commonLabels,
		TransitionSeq:   transitionSeq,
		TopIssueTypes:   topIssueTypesStr,
	}

	return renderTemplate(workflowTmpl, data)
}

// buildInteractionSection generates the interaction guidance prose.
func buildInteractionSection(displayName string, ia InteractionAnalysis) (string, error) {
	rp := ia.ResponsePatterns

	replyBias := ""
	if rp.RepliesToOwnIssuesPct > 0.5 {
		replyBias = fmt.Sprintf("%s tends to reply more on their own issues (%.0f%%).", displayName, rp.RepliesToOwnIssuesPct*100)
	} else if rp.RepliesToOthersPct > 0.5 {
		replyBias = fmt.Sprintf("%s frequently engages on others' issues (%.0f%%).", displayName, rp.RepliesToOthersPct*100)
	}


	collaborators := ""
	if len(ia.MentionHabits.FrequentlyMentions) > 0 {
		collaborators = strings.Join(ia.MentionHabits.FrequentlyMentions, ", ")
	}

	escalationStyle := ""
	es := ia.EscalationSignals
	if len(es.BlockerKeywords) > 0 {
		escalationStyle = fmt.Sprintf("Uses escalation keywords: %s.", strings.Join(es.BlockerKeywords, ", "))
		if es.AvgCommentsBeforeEscalation > 0 {
			escalationStyle += fmt.Sprintf(" Typically escalates after ~%.0f comments.", es.AvgCommentsBeforeEscalation)
		}
	}

	activeHours := ""
	ah := ia.Collaboration.ActiveHours
	if ah.Start != "" && ah.End != "" {
		activeHours = ah.Start + "–" + ah.End
		if ah.Timezone != "" {
			activeHours += " " + ah.Timezone
		}
	}

	peakDays := ""
	if len(ia.Collaboration.PeakActivityDays) > 0 {
		peakDays = strings.Join(ia.Collaboration.PeakActivityDays, ", ")
	}

	data := interactionData{
		DisplayName:     displayName,
		MedianReplyTime: rp.MedianReplyTime,
		ReplyBias:       replyBias,
		Collaborators:   collaborators,
		EscalationStyle: escalationStyle,
		ActiveHours:     activeHours,
		PeakDays:        peakDays,
	}

	return renderTemplate(interactionTmpl, data)
}

// parseOverrides converts a slice of "key=value" strings into a map.
func parseOverrides(overrides []string) map[string]string {
	if len(overrides) == 0 {
		return nil
	}
	m := make(map[string]string, len(overrides))
	for _, o := range overrides {
		parts := strings.SplitN(o, "=", 2)
		if len(parts) == 2 {
			m[parts[0]] = parts[1]
		}
	}
	if len(m) == 0 {
		return nil
	}
	return m
}

// BuildLocal generates a Profile from an Extraction using Go text/template.
func BuildLocal(extraction *Extraction, overrides []string) (*Profile, error) {
	displayName := extraction.Meta.DisplayName
	if displayName == "" {
		displayName = extraction.Meta.User
	}

	writing, err := buildWritingSection(displayName, extraction.Writing)
	if err != nil {
		return nil, fmt.Errorf("build writing section: %w", err)
	}

	workflow, err := buildWorkflowSection(displayName, extraction.Workflow)
	if err != nil {
		return nil, fmt.Errorf("build workflow section: %w", err)
	}

	interaction, err := buildInteractionSection(displayName, extraction.Interaction)
	if err != nil {
		return nil, fmt.Errorf("build interaction section: %w", err)
	}

	// Convert CommentExample → ProfileExample
	var examples []ProfileExample
	for _, ce := range extraction.Examples.Comments {
		examples = append(examples, ProfileExample{
			Context: ce.Context,
			Source:  ce.Issue,
			Text:    ce.Text,
		})
	}

	fp := extraction.Workflow.FieldPreferences

	profile := &Profile{
		Version:     "1",
		User:        extraction.Meta.User,
		DisplayName: displayName,
		GeneratedAt: time.Now().UTC().Format(time.RFC3339),
		Engine:      "local",
		StyleGuide: StyleGuide{
			Writing:     writing,
			Workflow:    workflow,
			Interaction: interaction,
		},
		Defaults: ProfileDefaults{
			Priority:               fp.DefaultPriority,
			Labels:                 fp.CommonLabels,
			Components:             fp.CommonComponents,
			AssignSelfOnTransition: extraction.Workflow.TransitionPatterns.AssignsBeforeTransition,
		},
		Examples:  examples,
		Overrides: parseOverrides(overrides),
	}

	return profile, nil
}
