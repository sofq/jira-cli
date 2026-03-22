package character

import (
	"strings"
	"unicode"

	"github.com/sofq/jira-cli/internal/avatar"
)

// FromProfile converts an avatar.Profile to a Character. The character's Name
// is derived by slugifying the profile's DisplayName. Source is set to
// SourceAvatar. The profile's Source field on each example is dropped.
func FromProfile(p *avatar.Profile) *Character {
	c := &Character{
		Version: "1",
		Name:    slugify(p.DisplayName),
		Source:  SourceAvatar,
		StyleGuide: StyleGuide{
			Writing:     p.StyleGuide.Writing,
			Workflow:    p.StyleGuide.Workflow,
			Interaction: p.StyleGuide.Interaction,
		},
		Defaults: Defaults{
			Priority:               p.Defaults.Priority,
			Labels:                 p.Defaults.Labels,
			Components:             p.Defaults.Components,
			AssignSelfOnTransition: p.Defaults.AssignSelfOnTransition,
		},
	}

	if len(p.Examples) > 0 {
		c.Examples = make([]Example, len(p.Examples))
		for i, ex := range p.Examples {
			c.Examples[i] = Example{
				Context: ex.Context,
				Text:    ex.Text,
			}
		}
	}

	return c
}

// slugify converts s to a URL-friendly lowercase slug: spaces become hyphens
// and all characters that are neither alphanumeric nor a hyphen are removed.
// Leading and trailing spaces are trimmed first.
func slugify(s string) string {
	s = strings.TrimSpace(s)
	s = strings.ToLower(s)

	var b strings.Builder
	b.Grow(len(s))
	for _, r := range s {
		switch {
		case unicode.IsLetter(r) || unicode.IsDigit(r):
			b.WriteRune(r)
		case r == ' ':
			b.WriteRune('-')
		case r == '-':
			b.WriteRune('-')
		// all other characters are dropped
		}
	}
	return b.String()
}
