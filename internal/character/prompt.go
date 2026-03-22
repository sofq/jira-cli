package character

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
)

// PromptOptions controls how FormatPrompt renders a ComposedCharacter.
type PromptOptions struct {
	// Format is one of "prose" (default), "json", or "both".
	Format string

	// Sections filters output to specific sections when Format is "prose" or
	// "both". Valid values: "writing", "workflow", "interaction", "examples".
	// An empty slice means all sections are included.
	Sections []string

	// Redact replaces email addresses and Jira issue keys with placeholders.
	Redact bool
}

var (
	reEmail    = regexp.MustCompile(`[a-zA-Z0-9._%+\-]+@[a-zA-Z0-9.\-]+\.[a-zA-Z]{2,}`)
	reIssueKey = regexp.MustCompile(`\b[A-Z][A-Z0-9]+-[0-9]+\b`)
)

// FormatPrompt renders c according to opts. The zero value of PromptOptions
// produces a full prose output with no redaction.
func FormatPrompt(c *ComposedCharacter, opts PromptOptions) string {
	format := opts.Format
	if format == "" {
		format = "prose"
	}

	switch format {
	case "json":
		return renderJSON(c, opts)
	case "both":
		prose := renderProse(c, opts)
		j := renderJSON(c, opts)
		return prose + "\n---\n\n" + j
	default: // "prose"
		return renderProse(c, opts)
	}
}

// sectionEnabled reports whether section is enabled given the filter list.
// An empty filter means all sections are enabled.
func sectionEnabled(section string, filter []string) bool {
	if len(filter) == 0 {
		return true
	}
	for _, s := range filter {
		if strings.EqualFold(s, section) {
			return true
		}
	}
	return false
}

// capitalize returns s with its first byte uppercased.
func capitalize(s string) string {
	if s == "" {
		return s
	}
	return strings.ToUpper(s[:1]) + s[1:]
}

func renderProse(c *ComposedCharacter, opts PromptOptions) string {
	var sb strings.Builder

	fmt.Fprintf(&sb, "# Character: %s\n", c.Name)
	if c.Description != "" {
		sb.WriteString("\n")
		sb.WriteString(c.Description)
		sb.WriteString("\n")
	}

	writeSG := func(key, text string) {
		if !sectionEnabled(key, opts.Sections) {
			return
		}
		fmt.Fprintf(&sb, "\n## %s\n\n%s\n", capitalize(key), text)
	}

	writeSG("writing", c.StyleGuide.Writing)
	writeSG("workflow", c.StyleGuide.Workflow)
	writeSG("interaction", c.StyleGuide.Interaction)

	if sectionEnabled("examples", opts.Sections) && len(c.Examples) > 0 {
		sb.WriteString("\n## Examples\n")
		for _, ex := range c.Examples {
			sb.WriteString("\n")
			if ex.Context != "" {
				fmt.Fprintf(&sb, "**%s**\n\n", ex.Context)
			}
			// Blockquote each line of the example text.
			for _, line := range strings.Split(ex.Text, "\n") {
				fmt.Fprintf(&sb, "> %s\n", line)
			}
		}
	}

	result := sb.String()
	if opts.Redact {
		result = redact(result)
	}
	return result
}

func renderJSON(c *ComposedCharacter, opts PromptOptions) string {
	data, err := json.MarshalIndent(c.Character, "", "  ")
	if err != nil {
		return "{}"
	}
	result := string(data)
	if opts.Redact {
		result = redact(result)
	}
	return result
}

func redact(s string) string {
	s = reEmail.ReplaceAllString(s, "[EMAIL]")
	s = reIssueKey.ReplaceAllString(s, "[ISSUE]")
	return s
}
