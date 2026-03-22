package character

import "testing"

func baseCharacter() *Character {
	return &Character{
		Version:     "1",
		Name:        "base",
		Source:      SourceManual,
		Description: "base description",
		StyleGuide: StyleGuide{
			Writing:     "base writing",
			Workflow:    "base workflow",
			Interaction: "base interaction",
		},
		Defaults: Defaults{
			Priority:               "High",
			Labels:                 []string{"base-label"},
			AssignSelfOnTransition: true,
		},
		Examples: []Example{
			{Context: "base-ctx", Text: "base-text"},
		},
		Reactions: map[string]Reaction{
			"blocked": {Action: "comment", Text: "blocked!"},
		},
	}
}

func overrideCharacter(name, writing, workflow, interaction string) *Character {
	return &Character{
		Name:   name,
		Source: SourceClone,
		StyleGuide: StyleGuide{
			Writing:     writing,
			Workflow:    workflow,
			Interaction: interaction,
		},
		Defaults: Defaults{
			Priority: "Low", // should NOT affect composed result
		},
		Examples: []Example{
			{Context: "override-ctx", Text: "override-text"}, // should NOT affect result
		},
	}
}

func TestComposeSingleNoOverrides(t *testing.T) {
	base := baseCharacter()
	composed := Compose(base, nil)

	if composed.Base != "base" {
		t.Errorf("Base: got %q, want %q", composed.Base, "base")
	}
	if composed.StyleGuide.Writing != "base writing" {
		t.Errorf("Writing: got %q, want %q", composed.StyleGuide.Writing, "base writing")
	}
	if composed.StyleGuide.Workflow != "base workflow" {
		t.Errorf("Workflow: got %q, want %q", composed.StyleGuide.Workflow, "base workflow")
	}
	if composed.StyleGuide.Interaction != "base interaction" {
		t.Errorf("Interaction: got %q, want %q", composed.StyleGuide.Interaction, "base interaction")
	}
	if composed.Defaults.Priority != "High" {
		t.Errorf("Defaults.Priority: got %q, want %q", composed.Defaults.Priority, "High")
	}
	if len(composed.Examples) != 1 || composed.Examples[0].Context != "base-ctx" {
		t.Errorf("Examples: got %+v", composed.Examples)
	}
	if r, ok := composed.Reactions["blocked"]; !ok || r.Text != "blocked!" {
		t.Errorf("Reactions: got %+v", composed.Reactions)
	}
}

func TestComposeOverrideWritingOnly(t *testing.T) {
	base := baseCharacter()
	writingOverride := overrideCharacter("writing-override", "new writing style", "", "")

	composed := Compose(base, map[string]*Character{
		"writing": writingOverride,
	})

	if composed.StyleGuide.Writing != "new writing style" {
		t.Errorf("Writing: got %q, want %q", composed.StyleGuide.Writing, "new writing style")
	}
	// Workflow and interaction still come from base.
	if composed.StyleGuide.Workflow != "base workflow" {
		t.Errorf("Workflow: got %q, want %q", composed.StyleGuide.Workflow, "base workflow")
	}
	if composed.StyleGuide.Interaction != "base interaction" {
		t.Errorf("Interaction: got %q, want %q", composed.StyleGuide.Interaction, "base interaction")
	}
	// Defaults come from base, NOT override.
	if composed.Defaults.Priority != "High" {
		t.Errorf("Defaults.Priority: got %q, want base High", composed.Defaults.Priority)
	}
	// Examples come from base, NOT override.
	if len(composed.Examples) != 1 || composed.Examples[0].Context != "base-ctx" {
		t.Errorf("Examples: got %+v", composed.Examples)
	}
	// Overrides map records which section was overridden and by whom.
	if composed.Overrides["writing"] != "writing-override" {
		t.Errorf("Overrides[writing]: got %q", composed.Overrides["writing"])
	}
}

func TestComposeOverrideMultipleSections(t *testing.T) {
	base := baseCharacter()
	wfOverride := overrideCharacter("workflow-override", "", "new workflow", "")
	iaOverride := overrideCharacter("interaction-override", "", "", "new interaction")

	composed := Compose(base, map[string]*Character{
		"workflow":    wfOverride,
		"interaction": iaOverride,
	})

	// Writing unchanged.
	if composed.StyleGuide.Writing != "base writing" {
		t.Errorf("Writing: got %q", composed.StyleGuide.Writing)
	}
	if composed.StyleGuide.Workflow != "new workflow" {
		t.Errorf("Workflow: got %q", composed.StyleGuide.Workflow)
	}
	if composed.StyleGuide.Interaction != "new interaction" {
		t.Errorf("Interaction: got %q", composed.StyleGuide.Interaction)
	}
	if composed.Overrides["workflow"] != "workflow-override" {
		t.Errorf("Overrides[workflow]: got %q", composed.Overrides["workflow"])
	}
	if composed.Overrides["interaction"] != "interaction-override" {
		t.Errorf("Overrides[interaction]: got %q", composed.Overrides["interaction"])
	}
}

func TestComposeOverrideAllSections(t *testing.T) {
	base := baseCharacter()
	all := overrideCharacter("all-override", "new writing", "new workflow", "new interaction")

	composed := Compose(base, map[string]*Character{
		"writing":     all,
		"workflow":    all,
		"interaction": all,
	})

	if composed.StyleGuide.Writing != "new writing" {
		t.Errorf("Writing: got %q", composed.StyleGuide.Writing)
	}
	if composed.StyleGuide.Workflow != "new workflow" {
		t.Errorf("Workflow: got %q", composed.StyleGuide.Workflow)
	}
	if composed.StyleGuide.Interaction != "new interaction" {
		t.Errorf("Interaction: got %q", composed.StyleGuide.Interaction)
	}
	// Defaults and examples still from base.
	if composed.Defaults.Priority != "High" {
		t.Errorf("Defaults.Priority: got %q", composed.Defaults.Priority)
	}
	if len(composed.Examples) != 1 {
		t.Errorf("Examples: got %d", len(composed.Examples))
	}
}

func TestComposeNilOverrideCharacterIgnored(t *testing.T) {
	base := baseCharacter()
	// Passing nil value for a key should be handled gracefully.
	composed := Compose(base, map[string]*Character{
		"writing": nil,
	})

	// Writing should fall back to base since override value is nil.
	if composed.StyleGuide.Writing != "base writing" {
		t.Errorf("Writing: got %q, want base writing", composed.StyleGuide.Writing)
	}
}
