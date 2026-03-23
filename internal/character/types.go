// Package character defines types and logic for managing reusable character
// profiles that shape how the CLI agent writes comments, transitions issues,
// and interacts with collaborators.
package character

// Source constants describe where a Character originated.
const (
	SourceClone    = "clone"
	SourceManual   = "manual"
	SourceTemplate = "template"
	SourcePersona  = "persona"
)

// Character is a reusable persona document that carries style guidance,
// workflow defaults, representative examples, and contextual reactions.
type Character struct {
	Version     string              `yaml:"version"              json:"version"`
	Name        string              `yaml:"name"                 json:"name"`
	Source      string              `yaml:"source"               json:"source"`
	Description string              `yaml:"description"          json:"description"`
	StyleGuide  StyleGuide          `yaml:"style_guide"          json:"style_guide"`
	Defaults    Defaults            `yaml:"defaults,omitempty"   json:"defaults,omitempty"`
	Examples    []Example           `yaml:"examples,omitempty"   json:"examples,omitempty"`
	Reactions   map[string]Reaction `yaml:"reactions,omitempty"  json:"reactions,omitempty"`
}

// StyleGuide holds prose guidance for the three behavioural dimensions.
type StyleGuide struct {
	Writing     string `yaml:"writing"     json:"writing"`
	Workflow    string `yaml:"workflow"    json:"workflow"`
	Interaction string `yaml:"interaction" json:"interaction"`
}

// Defaults holds preferred default values for Jira issue fields.
type Defaults struct {
	Priority               string   `yaml:"priority,omitempty"                  json:"priority,omitempty"`
	Labels                 []string `yaml:"labels,omitempty"                    json:"labels,omitempty"`
	Components             []string `yaml:"components,omitempty"                json:"components,omitempty"`
	AssignSelfOnTransition bool     `yaml:"assign_self_on_transition,omitempty" json:"assign_self_on_transition,omitempty"`
}

// Example is a representative text sample used to illustrate the character's
// writing style.
type Example struct {
	Context string `yaml:"context" json:"context"`
	Text    string `yaml:"text"    json:"text"`
}

// Reaction describes an automated action to take in response to a named event.
type Reaction struct {
	Action   string `yaml:"action"            json:"action"`
	To       string `yaml:"to,omitempty"      json:"to,omitempty"`
	Text     string `yaml:"text,omitempty"    json:"text,omitempty"`
	Assignee string `yaml:"assignee,omitempty" json:"assignee,omitempty"`
}

// ComposedCharacter is the result of merging a base character with one or more
// section-level overrides. Base records the name of the base character and
// Overrides maps section names (writing, workflow, interaction) to the name of
// the character that supplied that section.
type ComposedCharacter struct {
	Base      string
	Overrides map[string]string
	Character
}
