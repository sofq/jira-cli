package character

// Compose merges base with the provided section-level overrides and returns a
// ComposedCharacter. The overrides map keys must be one of "writing",
// "workflow", or "interaction"; the corresponding StyleGuide section in the
// result is replaced by the section from the override Character.
//
// Defaults, Examples, and Reactions always come from base — section overrides
// do not affect them. If a key maps to a nil Character it is silently ignored.
func Compose(base *Character, overrides map[string]*Character) ComposedCharacter {
	// Start with a shallow copy of the base.
	result := *base

	overrideNames := make(map[string]string)

	for section, ov := range overrides {
		if ov == nil {
			continue
		}
		switch section {
		case "writing":
			result.StyleGuide.Writing = ov.StyleGuide.Writing
			overrideNames[section] = ov.Name
		case "workflow":
			result.StyleGuide.Workflow = ov.StyleGuide.Workflow
			overrideNames[section] = ov.Name
		case "interaction":
			result.StyleGuide.Interaction = ov.StyleGuide.Interaction
			overrideNames[section] = ov.Name
		}
	}

	return ComposedCharacter{
		Base:      base.Name,
		Overrides: overrideNames,
		Character: result,
	}
}
