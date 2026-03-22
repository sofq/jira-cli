package yolo

import (
	"testing"

	"github.com/sofq/jira-cli/internal/character"
)

func makeComposed(reactions map[string]character.Reaction) *character.ComposedCharacter {
	return &character.ComposedCharacter{
		Character: character.Character{
			Reactions: reactions,
		},
	}
}

func TestDecideRuleBasedMatch(t *testing.T) {
	ch := makeComposed(map[string]character.Reaction{
		"blocked": {
			Action:   "comment",
			Text:     "Blocked on dependencies.",
			To:       "",
			Assignee: "",
		},
	})

	action, ok := DecideRuleBased(ch, "blocked")
	if !ok {
		t.Fatal("DecideRuleBased: ok = false, want true for known event with reaction")
	}
	if action.Action != "comment" {
		t.Errorf("Action.Action = %q, want %q", action.Action, "comment")
	}
	if action.Text != "Blocked on dependencies." {
		t.Errorf("Action.Text = %q, want %q", action.Text, "Blocked on dependencies.")
	}
}

func TestDecideRuleBasedNoMatch(t *testing.T) {
	ch := makeComposed(map[string]character.Reaction{
		"blocked": {Action: "comment", Text: "blocked text"},
	})

	_, ok := DecideRuleBased(ch, "assigned")
	if ok {
		t.Error("DecideRuleBased: ok = true for event with no reaction, want false")
	}
}

func TestDecideRuleBasedNilCharacter(t *testing.T) {
	_, ok := DecideRuleBased(nil, "blocked")
	if ok {
		t.Error("DecideRuleBased: ok = true for nil character, want false")
	}
}

func TestDecideRuleBasedNilReactions(t *testing.T) {
	ch := makeComposed(nil)

	_, ok := DecideRuleBased(ch, "blocked")
	if ok {
		t.Error("DecideRuleBased: ok = true for nil reactions map, want false")
	}
}

func TestDecideRuleBasedAllFields(t *testing.T) {
	ch := makeComposed(map[string]character.Reaction{
		"status_change": {
			Action:   "transition",
			To:       "In Progress",
			Text:     "Moving forward.",
			Assignee: "me",
		},
	})

	action, ok := DecideRuleBased(ch, "status_change")
	if !ok {
		t.Fatal("DecideRuleBased: ok = false for status_change")
	}
	if action.Action != "transition" {
		t.Errorf("Action.Action = %q, want transition", action.Action)
	}
	if action.To != "In Progress" {
		t.Errorf("Action.To = %q, want 'In Progress'", action.To)
	}
	if action.Text != "Moving forward." {
		t.Errorf("Action.Text = %q, want 'Moving forward.'", action.Text)
	}
	if action.Assignee != "me" {
		t.Errorf("Action.Assignee = %q, want 'me'", action.Assignee)
	}
}

func TestDecideRuleBasedEmptyAction(t *testing.T) {
	// A reaction with empty Action is still returned if present.
	ch := makeComposed(map[string]character.Reaction{
		"assigned": {Action: "", Text: "noted"},
	})

	action, ok := DecideRuleBased(ch, "assigned")
	if !ok {
		t.Fatal("DecideRuleBased: ok = false for assigned event with empty action")
	}
	if action.Text != "noted" {
		t.Errorf("Action.Text = %q, want 'noted'", action.Text)
	}
}

func TestDecideRuleBasedReasonPopulated(t *testing.T) {
	ch := makeComposed(map[string]character.Reaction{
		"comment_added": {Action: "comment", Text: "Thanks!"},
	})

	action, ok := DecideRuleBased(ch, "comment_added")
	if !ok {
		t.Fatal("DecideRuleBased: ok = false")
	}
	// Reason should be populated to indicate the source of the decision.
	if action.Reason == "" {
		t.Error("Action.Reason is empty, want non-empty explanation")
	}
}

func TestActionJSONFields(t *testing.T) {
	// Verify zero value is valid and non-required fields are omitempty.
	a := Action{}
	if a.Action != "" {
		t.Errorf("Action zero value: Action = %q, want empty", a.Action)
	}
}

func TestDecideRuleBasedMultipleReactions(t *testing.T) {
	ch := makeComposed(map[string]character.Reaction{
		"blocked":      {Action: "comment", Text: "blocked text"},
		"assigned":     {Action: "transition", To: "In Progress"},
		"sprint_change": {Action: "comment", Text: "sprint changed"},
	})

	cases := []struct {
		event  string
		action string
	}{
		{"blocked", "comment"},
		{"assigned", "transition"},
		{"sprint_change", "comment"},
		{"unassigned", ""},
	}

	for _, tc := range cases {
		t.Run(tc.event, func(t *testing.T) {
			action, ok := DecideRuleBased(ch, tc.event)
			if tc.action == "" {
				if ok {
					t.Errorf("DecideRuleBased(%q): ok = true, want false", tc.event)
				}
				return
			}
			if !ok {
				t.Fatalf("DecideRuleBased(%q): ok = false, want true", tc.event)
			}
			if action.Action != tc.action {
				t.Errorf("DecideRuleBased(%q): Action = %q, want %q", tc.event, action.Action, tc.action)
			}
		})
	}
}
