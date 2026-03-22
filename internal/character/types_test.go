package character

import (
	"encoding/json"
	"testing"

	"gopkg.in/yaml.v3"
)

func TestSourceConstants(t *testing.T) {
	cases := []struct {
		name string
		val  string
	}{
		{"SourceAvatar", SourceAvatar},
		{"SourceClone", SourceClone},
		{"SourceManual", SourceManual},
		{"SourceTemplate", SourceTemplate},
		{"SourcePersona", SourcePersona},
	}
	expected := map[string]string{
		"SourceAvatar":   "avatar",
		"SourceClone":    "clone",
		"SourceManual":   "manual",
		"SourceTemplate": "template",
		"SourcePersona":  "persona",
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if tc.val != expected[tc.name] {
				t.Errorf("got %q, want %q", tc.val, expected[tc.name])
			}
		})
	}
}

func TestCharacterYAMLRoundTrip(t *testing.T) {
	original := Character{
		Version:     "1",
		Name:        "test-character",
		Source:      SourceManual,
		Description: "A test character",
		StyleGuide: StyleGuide{
			Writing:     "Be concise.",
			Workflow:    "Move tickets promptly.",
			Interaction: "Be polite.",
		},
		Defaults: Defaults{
			Priority:               "Medium",
			Labels:                 []string{"bug", "urgent"},
			Components:             []string{"backend"},
			AssignSelfOnTransition: true,
		},
		Examples: []Example{
			{Context: "bug report", Text: "Steps to reproduce: ..."},
		},
		Reactions: map[string]Reaction{
			"blocked": {
				Action:   "comment",
				Text:     "Blocked on dependencies.",
				To:       "",
				Assignee: "",
			},
		},
	}

	data, err := yaml.Marshal(&original)
	if err != nil {
		t.Fatalf("yaml.Marshal: %v", err)
	}

	var got Character
	if err := yaml.Unmarshal(data, &got); err != nil {
		t.Fatalf("yaml.Unmarshal: %v", err)
	}

	if got.Version != original.Version {
		t.Errorf("Version: got %q, want %q", got.Version, original.Version)
	}
	if got.Name != original.Name {
		t.Errorf("Name: got %q, want %q", got.Name, original.Name)
	}
	if got.Source != original.Source {
		t.Errorf("Source: got %q, want %q", got.Source, original.Source)
	}
	if got.Description != original.Description {
		t.Errorf("Description: got %q, want %q", got.Description, original.Description)
	}
	if got.StyleGuide.Writing != original.StyleGuide.Writing {
		t.Errorf("StyleGuide.Writing: got %q, want %q", got.StyleGuide.Writing, original.StyleGuide.Writing)
	}
	if got.StyleGuide.Workflow != original.StyleGuide.Workflow {
		t.Errorf("StyleGuide.Workflow: got %q, want %q", got.StyleGuide.Workflow, original.StyleGuide.Workflow)
	}
	if got.StyleGuide.Interaction != original.StyleGuide.Interaction {
		t.Errorf("StyleGuide.Interaction: got %q, want %q", got.StyleGuide.Interaction, original.StyleGuide.Interaction)
	}
	if got.Defaults.Priority != original.Defaults.Priority {
		t.Errorf("Defaults.Priority: got %q, want %q", got.Defaults.Priority, original.Defaults.Priority)
	}
	if got.Defaults.AssignSelfOnTransition != original.Defaults.AssignSelfOnTransition {
		t.Errorf("Defaults.AssignSelfOnTransition: got %v, want %v", got.Defaults.AssignSelfOnTransition, original.Defaults.AssignSelfOnTransition)
	}
	if len(got.Examples) != 1 || got.Examples[0].Context != "bug report" {
		t.Errorf("Examples: got %+v", got.Examples)
	}
	if r, ok := got.Reactions["blocked"]; !ok || r.Action != "comment" {
		t.Errorf("Reactions[blocked]: got %+v", got.Reactions["blocked"])
	}
}

func TestCharacterJSONRoundTrip(t *testing.T) {
	original := Character{
		Version: "1",
		Name:    "json-character",
		Source:  SourceTemplate,
		StyleGuide: StyleGuide{
			Writing: "Be brief.",
		},
	}

	data, err := json.Marshal(&original)
	if err != nil {
		t.Fatalf("json.Marshal: %v", err)
	}

	var got Character
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("json.Unmarshal: %v", err)
	}

	if got.Name != original.Name {
		t.Errorf("Name: got %q, want %q", got.Name, original.Name)
	}
	if got.Source != original.Source {
		t.Errorf("Source: got %q, want %q", got.Source, original.Source)
	}
	if got.StyleGuide.Writing != original.StyleGuide.Writing {
		t.Errorf("StyleGuide.Writing: got %q, want %q", got.StyleGuide.Writing, original.StyleGuide.Writing)
	}
}

func TestComposedCharacterFields(t *testing.T) {
	c := ComposedCharacter{
		Base:      "base-name",
		Overrides: map[string]string{"writing": "override-writing"},
		Character: Character{
			Name:   "composed",
			Source: SourceClone,
		},
	}

	if c.Base != "base-name" {
		t.Errorf("Base: got %q, want %q", c.Base, "base-name")
	}
	if c.Overrides["writing"] != "override-writing" {
		t.Errorf("Overrides[writing]: got %q", c.Overrides["writing"])
	}
	if c.Character.Name != "composed" {
		t.Errorf("Character.Name: got %q", c.Character.Name)
	}
}
