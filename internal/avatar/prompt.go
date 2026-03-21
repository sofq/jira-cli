package avatar

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
)

// PromptOptions controls how a Profile is formatted for agent consumption.
type PromptOptions struct {
	Format   string   // "prose", "json", "both"; default is "prose"
	Sections []string // filter to specific sections; empty = all
	Redact   bool
}

var (
	reEmail    = regexp.MustCompile(`\b[A-Za-z0-9._%+-]+@[A-Za-z0-9.-]+\.[A-Za-z]{2,}\b`)
	reIssueKey = regexp.MustCompile(`\b[A-Z][A-Z0-9]+-\d+\b`)
)

// FormatPrompt formats a Profile for agent consumption according to opts.
func FormatPrompt(p *Profile, opts PromptOptions) string {
	format := opts.Format
	if format == "" {
		format = "prose"
	}

	switch format {
	case "json":
		b, _ := json.MarshalIndent(p, "", "  ")
		return maybeRedact(string(b), opts.Redact)
	case "both":
		prose := formatProse(p, opts.Sections)
		b, _ := json.MarshalIndent(p, "", "  ")
		combined := prose + "\n\n---\n\n" + string(b)
		return maybeRedact(combined, opts.Redact)
	default: // "prose"
		return maybeRedact(formatProse(p, opts.Sections), opts.Redact)
	}
}

// sectionIncluded returns true when name is in the sections filter, or the
// filter is empty (meaning "all sections").
func sectionIncluded(name string, sections []string) bool {
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

// formatProse renders the profile as human-readable Markdown prose.
func formatProse(p *Profile, sections []string) string {
	var sb strings.Builder

	fmt.Fprintf(&sb, "# Style Profile: %s\n", p.DisplayName)

	if sectionIncluded("writing", sections) && p.StyleGuide.Writing != "" {
		fmt.Fprintf(&sb, "\n## Writing Style\n%s\n", p.StyleGuide.Writing)
	}
	if sectionIncluded("workflow", sections) && p.StyleGuide.Workflow != "" {
		fmt.Fprintf(&sb, "\n## Workflow Patterns\n%s\n", p.StyleGuide.Workflow)
	}
	if sectionIncluded("interaction", sections) && p.StyleGuide.Interaction != "" {
		fmt.Fprintf(&sb, "\n## Interaction Patterns\n%s\n", p.StyleGuide.Interaction)
	}

	if len(p.Overrides) > 0 {
		fmt.Fprintf(&sb, "\n## Hard Rules (always follow)\n")
		for k, v := range p.Overrides {
			fmt.Fprintf(&sb, "- %s: %s\n", k, v)
		}
	}

	if len(p.Examples) > 0 {
		fmt.Fprintf(&sb, "\n## Representative Examples\n")
		for _, ex := range p.Examples {
			fmt.Fprintf(&sb, "### %s (from %s)\n%s\n", ex.Context, ex.Source, ex.Text)
		}
	}

	return strings.TrimRight(sb.String(), " \t\n") + "\n"
}

// maybeRedact replaces PII (emails, Jira issue keys) with placeholders when
// redact is true.
func maybeRedact(s string, redact bool) string {
	if !redact {
		return s
	}
	s = reEmail.ReplaceAllString(s, "[EMAIL]")
	s = reIssueKey.ReplaceAllString(s, "[ISSUE]")
	return s
}
