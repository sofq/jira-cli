package yolo

import (
	"fmt"

	"github.com/sofq/jira-cli/internal/character"
)

// Action is the output of the decision engine: a concrete operation to perform
// on a Jira issue in response to an event.
type Action struct {
	Action   string `json:"action"`
	To       string `json:"to,omitempty"`
	Text     string `json:"text,omitempty"`
	Assignee string `json:"assignee,omitempty"`
	Reason   string `json:"reason,omitempty"`
}

// DecideRuleBased looks up the reaction for eventType in the composed
// character's Reactions map and converts it to an Action. It returns (Action,
// true) when a matching reaction is found, and (Action{}, false) when the
// character is nil, has no reactions, or has no reaction for the given event
// type.
//
// The event type is used as the map key directly (not via ReactionKey), since
// Character.Reactions stores plain event names as keys (e.g. "blocked",
// "assigned").
func DecideRuleBased(ch *character.ComposedCharacter, eventType string) (Action, bool) {
	if ch == nil {
		return Action{}, false
	}
	if ch.Reactions == nil {
		return Action{}, false
	}
	r, ok := ch.Reactions[eventType]
	if !ok {
		return Action{}, false
	}
	a := Action{
		Action:   r.Action,
		To:       r.To,
		Text:     r.Text,
		Assignee: r.Assignee,
		Reason:   fmt.Sprintf("character rule: reaction for event %q", eventType),
	}
	return a, true
}
