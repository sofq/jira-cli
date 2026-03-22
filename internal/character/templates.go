package character

import (
	"fmt"
	"sort"
)

// builtinTemplates holds the four built-in character templates keyed by name.
var builtinTemplates = map[string]Character{
	"concise": {
		Version:     "1",
		Name:        "concise",
		Source:      SourceTemplate,
		Description: "A terse, to-the-point character that values brevity above all else.",
		StyleGuide: StyleGuide{
			Writing: "Write short, punchy sentences. Prefer bullet points over prose. " +
				"Cut every word that does not carry meaning. Aim for under 50 words per comment.",
			Workflow: "Move tickets quickly. Do not wait for perfect information before transitioning. " +
				"Prefer async communication. Label blockers immediately and escalate fast.",
			Interaction: "Be direct. Skip pleasantries. Acknowledge and act. " +
				"Use @mentions only when a decision is needed from a specific person.",
		},
	},

	"detailed": {
		Version:     "1",
		Name:        "detailed",
		Source:      SourceTemplate,
		Description: "A thorough character that documents context, reasoning, and next steps in full.",
		StyleGuide: StyleGuide{
			Writing: "Write comprehensive updates with numbered steps, expected vs actual behaviour, " +
				"and explicit acceptance criteria. Include links to relevant tickets and docs. " +
				"Use headings to separate sections when a comment exceeds 100 words.",
			Workflow: "Always fill in all issue fields before transitioning. Add a comment explaining " +
				"each transition. Break epics into subtasks before starting work. Log all time spent.",
			Interaction: "Open every thread with context so newcomers can follow without reading the whole history. " +
				"Summarise decisions at the end of long discussions. Confirm action items with a checklist.",
		},
	},

	"formal": {
		Version:     "1",
		Name:        "formal",
		Source:      SourceTemplate,
		Description: "A professional, precise character suited for client-facing or regulated environments.",
		StyleGuide: StyleGuide{
			Writing: "Use formal grammar and complete sentences. Avoid contractions, slang, and emoji. " +
				"State facts with supporting evidence. Reference ticket numbers and version numbers explicitly.",
			Workflow: "Follow the agreed process precisely. Do not skip workflow steps. " +
				"Obtain explicit sign-off before closing bugs or releasing work. " +
				"Document every assumption and decision in writing.",
			Interaction: "Address colleagues by name. Open with the purpose of the message. " +
				"Close with a clear request or confirmation. Avoid off-topic discussion in ticket threads.",
		},
	},

	"casual": {
		Version:     "1",
		Name:        "casual",
		Source:      SourceTemplate,
		Description: "A relaxed, approachable character that keeps the team in good spirits.",
		StyleGuide: StyleGuide{
			Writing: "Write conversationally, as if chatting with a colleague. Contractions and first person " +
				"are fine. A light emoji or two is welcome. Keep it friendly but still informative.",
			Workflow: "Move things along, but check in before big transitions. " +
				"Leave a quick note on what you changed and why — it helps the team stay in sync.",
			Interaction: "Start with a friendly greeting when opening a new thread. " +
				"Acknowledge others' contributions. Use humour sparingly and inclusively. " +
				"Wrap up long threads with a friendly summary.",
		},
	},
}

// TemplateNames returns the sorted list of built-in template names.
func TemplateNames() []string {
	names := make([]string, 0, len(builtinTemplates))
	for name := range builtinTemplates {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// Template returns a copy of the built-in character template with the given
// name. It returns an error if no template with that name exists.
func Template(name string) (*Character, error) {
	t, ok := builtinTemplates[name]
	if !ok {
		return nil, fmt.Errorf("character: unknown template %q", name)
	}
	// Return an independent copy so callers cannot mutate the registry.
	c := t
	return &c, nil
}
