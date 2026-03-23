package character

import "fmt"

// validSources is the set of recognised source values.
var validSources = map[string]bool{
	SourceClone:    true,
	SourceManual:   true,
	SourceTemplate: true,
	SourcePersona:  true,
}

// Validate checks that c contains the required fields and that the source
// value (when non-empty) is one of the declared constants. It returns the
// first validation error encountered, or nil when c is valid.
func Validate(c *Character) error {
	if c.Version == "" {
		return fmt.Errorf("character: version is required")
	}
	if c.Name == "" {
		return fmt.Errorf("character: name is required")
	}
	if c.Source != "" && !validSources[c.Source] {
		return fmt.Errorf("character: invalid source %q", c.Source)
	}
	if c.StyleGuide.Writing == "" {
		return fmt.Errorf("character: style_guide.writing is required")
	}
	if c.StyleGuide.Workflow == "" {
		return fmt.Errorf("character: style_guide.workflow is required")
	}
	if c.StyleGuide.Interaction == "" {
		return fmt.Errorf("character: style_guide.interaction is required")
	}
	return nil
}
