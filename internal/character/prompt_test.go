package character

import (
	"strings"
	"testing"
)

func sampleComposed() *ComposedCharacter {
	c := &ComposedCharacter{
		Base:      "tester",
		Overrides: map[string]string{},
		Character: Character{
			Version:     "1",
			Name:        "tester",
			Source:      SourceManual,
			Description: "A test character.",
			StyleGuide: StyleGuide{
				Writing:     "Write clearly and concisely.",
				Workflow:    "Follow the process.",
				Interaction: "Be respectful.",
			},
			Examples: []Example{
				{Context: "status update", Text: "Completed the fix for PROJ-42. See user@example.com for details."},
			},
		},
	}
	return c
}

func TestFormatPrompt_Prose(t *testing.T) {
	c := sampleComposed()
	got := FormatPrompt(c, PromptOptions{Format: "prose"})

	checks := []string{
		"# Character: tester",
		"## Writing",
		"Write clearly and concisely.",
		"## Workflow",
		"Follow the process.",
		"## Interaction",
		"Be respectful.",
		"## Examples",
		"> Completed the fix for PROJ-42.",
	}
	for _, want := range checks {
		if !strings.Contains(got, want) {
			t.Errorf("prose output missing %q\ngot:\n%s", want, got)
		}
	}
}

func TestFormatPrompt_DefaultFormatIsProse(t *testing.T) {
	c := sampleComposed()
	got := FormatPrompt(c, PromptOptions{})
	if !strings.Contains(got, "# Character:") {
		t.Errorf("default format should be prose, got:\n%s", got)
	}
	if strings.HasPrefix(got, "{") {
		t.Errorf("default format should not be JSON, got:\n%s", got)
	}
}

func TestFormatPrompt_JSON(t *testing.T) {
	c := sampleComposed()
	got := FormatPrompt(c, PromptOptions{Format: "json"})

	if !strings.HasPrefix(strings.TrimSpace(got), "{") {
		t.Errorf("json format should start with '{', got:\n%s", got)
	}
	if strings.Contains(got, "# Character:") {
		t.Errorf("json format should not contain prose header, got:\n%s", got)
	}
	if !strings.Contains(got, `"name"`) {
		t.Errorf("json output missing 'name' field, got:\n%s", got)
	}
}

func TestFormatPrompt_Both(t *testing.T) {
	c := sampleComposed()
	got := FormatPrompt(c, PromptOptions{Format: "both"})

	if !strings.Contains(got, "# Character:") {
		t.Errorf("both format missing prose header, got:\n%s", got)
	}
	if !strings.Contains(got, "---") {
		t.Errorf("both format missing separator '---', got:\n%s", got)
	}
	if !strings.Contains(got, `"name"`) {
		t.Errorf("both format missing json 'name' field, got:\n%s", got)
	}
}

func TestFormatPrompt_Redact(t *testing.T) {
	c := sampleComposed()
	got := FormatPrompt(c, PromptOptions{Format: "prose", Redact: true})

	if strings.Contains(got, "user@example.com") {
		t.Errorf("redacted output should not contain email address, got:\n%s", got)
	}
	if !strings.Contains(got, "[EMAIL]") {
		t.Errorf("redacted output should contain [EMAIL] placeholder, got:\n%s", got)
	}
	if strings.Contains(got, "PROJ-42") {
		t.Errorf("redacted output should not contain issue key, got:\n%s", got)
	}
	if !strings.Contains(got, "[ISSUE]") {
		t.Errorf("redacted output should contain [ISSUE] placeholder, got:\n%s", got)
	}
}

func TestFormatPrompt_SectionFilter(t *testing.T) {
	c := sampleComposed()
	got := FormatPrompt(c, PromptOptions{Format: "prose", Sections: []string{"writing"}})

	if !strings.Contains(got, "## Writing") {
		t.Errorf("filtered output should contain Writing section, got:\n%s", got)
	}
	if strings.Contains(got, "## Workflow") {
		t.Errorf("filtered output should not contain Workflow section, got:\n%s", got)
	}
	if strings.Contains(got, "## Interaction") {
		t.Errorf("filtered output should not contain Interaction section, got:\n%s", got)
	}
	if strings.Contains(got, "## Examples") {
		t.Errorf("filtered output should not contain Examples section, got:\n%s", got)
	}
}

func TestFormatPrompt_SectionFilterMultiple(t *testing.T) {
	c := sampleComposed()
	got := FormatPrompt(c, PromptOptions{Format: "prose", Sections: []string{"workflow", "examples"}})

	if strings.Contains(got, "## Writing") {
		t.Errorf("filtered output should not contain Writing section, got:\n%s", got)
	}
	if !strings.Contains(got, "## Workflow") {
		t.Errorf("filtered output should contain Workflow section, got:\n%s", got)
	}
	if strings.Contains(got, "## Interaction") {
		t.Errorf("filtered output should not contain Interaction section, got:\n%s", got)
	}
	if !strings.Contains(got, "## Examples") {
		t.Errorf("filtered output should contain Examples section, got:\n%s", got)
	}
}

func TestFormatPrompt_NoExamples(t *testing.T) {
	c := sampleComposed()
	c.Examples = nil
	got := FormatPrompt(c, PromptOptions{Format: "prose"})

	if strings.Contains(got, "## Examples") {
		t.Errorf("output with no examples should not contain Examples section, got:\n%s", got)
	}
}

func TestFormatPrompt_RedactJSON(t *testing.T) {
	c := sampleComposed()
	got := FormatPrompt(c, PromptOptions{Format: "json", Redact: true})

	if strings.Contains(got, "user@example.com") {
		t.Errorf("redacted json should not contain email, got:\n%s", got)
	}
	if strings.Contains(got, "PROJ-42") {
		t.Errorf("redacted json should not contain issue key, got:\n%s", got)
	}
}
