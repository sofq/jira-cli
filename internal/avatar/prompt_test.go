package avatar

import (
	"encoding/json"
	"strings"
	"testing"
)

func testProfile() *Profile {
	return &Profile{
		Version:     "1",
		User:        "jdoe",
		DisplayName: "Jane Doe",
		GeneratedAt: "2026-01-01T00:00:00Z",
		Engine:      "test",
		StyleGuide: StyleGuide{
			Writing:     "Write clearly and concisely.",
			Workflow:    "Always set priority before transitioning.",
			Interaction: "Mention assignees when blocked.",
		},
		Overrides: map[string]string{
			"tone": "professional",
		},
		Examples: []ProfileExample{
			{
				Context: "bug report",
				Source:  "PROJ-42",
				Text:    "Steps to reproduce: ...",
			},
		},
	}
}

func TestFormatPrompt_Prose(t *testing.T) {
	p := testProfile()
	opts := PromptOptions{Format: "prose"}
	out := FormatPrompt(p, opts)

	if !strings.Contains(out, "# Style Profile: Jane Doe") {
		t.Errorf("expected header, got:\n%s", out)
	}
	if !strings.Contains(out, "## Writing Style") {
		t.Errorf("expected Writing Style section, got:\n%s", out)
	}
	if !strings.Contains(out, "Write clearly and concisely.") {
		t.Errorf("expected writing content, got:\n%s", out)
	}
	if !strings.Contains(out, "## Hard Rules (always follow)") {
		t.Errorf("expected Hard Rules section, got:\n%s", out)
	}
	if !strings.Contains(out, "professional") {
		t.Errorf("expected override value, got:\n%s", out)
	}
	if !strings.Contains(out, "## Representative Examples") {
		t.Errorf("expected Examples section, got:\n%s", out)
	}
	if !strings.Contains(out, "bug report") {
		t.Errorf("expected example context, got:\n%s", out)
	}
	if !strings.Contains(out, "PROJ-42") {
		t.Errorf("expected example source, got:\n%s", out)
	}
}

func TestFormatPrompt_JSON(t *testing.T) {
	p := testProfile()
	opts := PromptOptions{Format: "json"}
	out := FormatPrompt(p, opts)

	var result map[string]interface{}
	if err := json.Unmarshal([]byte(out), &result); err != nil {
		t.Errorf("expected valid JSON, got error: %v\noutput:\n%s", err, out)
	}
	if result["display_name"] != "Jane Doe" {
		t.Errorf("expected display_name Jane Doe in JSON, got: %v", result["display_name"])
	}
}

func TestFormatPrompt_Both(t *testing.T) {
	p := testProfile()
	opts := PromptOptions{Format: "both"}
	out := FormatPrompt(p, opts)

	if !strings.Contains(out, "# Style Profile: Jane Doe") {
		t.Errorf("expected prose header in 'both' output, got:\n%s", out)
	}
	if !strings.Contains(out, "---") {
		t.Errorf("expected separator '---' in 'both' output, got:\n%s", out)
	}
	parts := strings.SplitN(out, "\n\n---\n\n", 2)
	if len(parts) != 2 {
		t.Fatalf("expected two parts separated by '\\n\\n---\\n\\n', got:\n%s", out)
	}
	var result map[string]interface{}
	if err := json.Unmarshal([]byte(parts[1]), &result); err != nil {
		t.Errorf("expected valid JSON after separator, got error: %v", err)
	}
}

func TestFormatPrompt_SectionFilter(t *testing.T) {
	p := testProfile()
	opts := PromptOptions{
		Format:   "prose",
		Sections: []string{"writing"},
	}
	out := FormatPrompt(p, opts)

	if !strings.Contains(out, "## Writing Style") {
		t.Errorf("expected Writing Style section, got:\n%s", out)
	}
	if strings.Contains(out, "## Workflow Patterns") {
		t.Errorf("expected NO Workflow Patterns section, got:\n%s", out)
	}
	if strings.Contains(out, "## Interaction Patterns") {
		t.Errorf("expected NO Interaction Patterns section, got:\n%s", out)
	}
}

func TestFormatPrompt_Redact(t *testing.T) {
	p := testProfile()
	p.StyleGuide.Writing = "Contact jane.doe@example.com for details."
	p.StyleGuide.Workflow = "See PROJ-123 for context."
	opts := PromptOptions{Format: "prose", Redact: true}
	out := FormatPrompt(p, opts)

	if strings.Contains(out, "jane.doe@example.com") {
		t.Errorf("expected email to be redacted, got:\n%s", out)
	}
	if !strings.Contains(out, "[EMAIL]") {
		t.Errorf("expected [EMAIL] placeholder, got:\n%s", out)
	}
	if strings.Contains(out, "PROJ-123") {
		t.Errorf("expected issue key to be redacted, got:\n%s", out)
	}
	if !strings.Contains(out, "[ISSUE]") {
		t.Errorf("expected [ISSUE] placeholder, got:\n%s", out)
	}
}

func TestFormatPrompt_RedactPreservesStructure(t *testing.T) {
	p := testProfile()
	p.StyleGuide.Writing = "Email alice@corp.io about PROJ-99."
	opts := PromptOptions{Format: "prose", Redact: true}
	out := FormatPrompt(p, opts)

	if !strings.Contains(out, "# Style Profile:") {
		t.Errorf("expected header to be present after redaction, got:\n%s", out)
	}
	if !strings.Contains(out, "## Writing Style") {
		t.Errorf("expected Writing Style section after redaction, got:\n%s", out)
	}
}
