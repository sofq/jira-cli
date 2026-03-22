package character

import (
	"testing"

	"github.com/sofq/jira-cli/internal/avatar"
)

func makeProfile() *avatar.Profile {
	return &avatar.Profile{
		Version:     "1",
		User:        "jdoe",
		DisplayName: "Jane Doe",
		StyleGuide: avatar.StyleGuide{
			Writing:     "profile writing",
			Workflow:    "profile workflow",
			Interaction: "profile interaction",
		},
		Defaults: avatar.ProfileDefaults{
			Priority:               "Medium",
			Labels:                 []string{"frontend", "ux"},
			Components:             []string{"web"},
			AssignSelfOnTransition: true,
		},
		Examples: []avatar.ProfileExample{
			{Context: "bug", Source: "PROJ-1", Text: "Repro steps: ..."},
			{Context: "feature", Source: "PROJ-2", Text: "As a user, ..."},
		},
	}
}

func TestFromProfileStyleGuide(t *testing.T) {
	p := makeProfile()
	c := FromProfile(p)

	if c.StyleGuide.Writing != p.StyleGuide.Writing {
		t.Errorf("Writing: got %q, want %q", c.StyleGuide.Writing, p.StyleGuide.Writing)
	}
	if c.StyleGuide.Workflow != p.StyleGuide.Workflow {
		t.Errorf("Workflow: got %q, want %q", c.StyleGuide.Workflow, p.StyleGuide.Workflow)
	}
	if c.StyleGuide.Interaction != p.StyleGuide.Interaction {
		t.Errorf("Interaction: got %q, want %q", c.StyleGuide.Interaction, p.StyleGuide.Interaction)
	}
}

func TestFromProfileDefaults(t *testing.T) {
	p := makeProfile()
	c := FromProfile(p)

	if c.Defaults.Priority != p.Defaults.Priority {
		t.Errorf("Priority: got %q, want %q", c.Defaults.Priority, p.Defaults.Priority)
	}
	if c.Defaults.AssignSelfOnTransition != p.Defaults.AssignSelfOnTransition {
		t.Errorf("AssignSelfOnTransition: got %v, want %v", c.Defaults.AssignSelfOnTransition, p.Defaults.AssignSelfOnTransition)
	}
	if len(c.Defaults.Labels) != len(p.Defaults.Labels) {
		t.Errorf("Labels: got %v, want %v", c.Defaults.Labels, p.Defaults.Labels)
	}
	if len(c.Defaults.Components) != len(p.Defaults.Components) {
		t.Errorf("Components: got %v, want %v", c.Defaults.Components, p.Defaults.Components)
	}
}

func TestFromProfileExamples(t *testing.T) {
	p := makeProfile()
	c := FromProfile(p)

	if len(c.Examples) != len(p.Examples) {
		t.Fatalf("Examples len: got %d, want %d", len(c.Examples), len(p.Examples))
	}
	for i, ex := range p.Examples {
		if c.Examples[i].Context != ex.Context {
			t.Errorf("Example[%d].Context: got %q, want %q", i, c.Examples[i].Context, ex.Context)
		}
		if c.Examples[i].Text != ex.Text {
			t.Errorf("Example[%d].Text: got %q, want %q", i, c.Examples[i].Text, ex.Text)
		}
	}
}

func TestFromProfileSource(t *testing.T) {
	c := FromProfile(makeProfile())
	if c.Source != SourceAvatar {
		t.Errorf("Source: got %q, want %q", c.Source, SourceAvatar)
	}
}

func TestFromProfileVersion(t *testing.T) {
	c := FromProfile(makeProfile())
	if c.Version != "1" {
		t.Errorf("Version: got %q, want 1", c.Version)
	}
}

func TestFromProfileName(t *testing.T) {
	p := makeProfile()
	c := FromProfile(p)
	// DisplayName "Jane Doe" → slugified → "jane-doe"
	if c.Name != "jane-doe" {
		t.Errorf("Name: got %q, want %q", c.Name, "jane-doe")
	}
}

func TestSlugify(t *testing.T) {
	cases := []struct {
		input string
		want  string
	}{
		{"Jane Doe", "jane-doe"},
		{"  leading trailing  ", "leading-trailing"},
		{"Hello World!", "hello-world"},
		{"my-profile", "my-profile"},
		{"ALL CAPS", "all-caps"},
		{"under_score", "underscore"}, // underscores are non-alphanumeric and get stripped
		{"special!@#$chars", "specialchars"},
		{"multiple   spaces", "multiple---spaces"},
		{"", ""},
	}
	for _, tc := range cases {
		t.Run(tc.input, func(t *testing.T) {
			got := slugify(tc.input)
			if got != tc.want {
				t.Errorf("slugify(%q): got %q, want %q", tc.input, got, tc.want)
			}
		})
	}
}
