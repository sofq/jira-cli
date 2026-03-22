package yolo

// Canonical event type constants. These are the 11 event types that the yolo
// decision engine recognises. They map to reaction keys in a character profile
// via ReactionKey.
const (
	EventAssigned        = "assigned"
	EventUnassigned      = "unassigned"
	EventCommentAdded    = "comment_added"
	EventCommentMention  = "comment_mention"
	EventStatusChange    = "status_change"
	EventPriorityChange  = "priority_change"
	EventFieldChange     = "field_change"
	EventIssueCreated    = "issue_created"
	EventReviewRequested = "review_requested"
	EventBlocked         = "blocked"
	EventSprintChange    = "sprint_change"
)

// allEventTypes is the ordered list of all canonical event type strings.
var allEventTypes = []string{
	EventAssigned,
	EventUnassigned,
	EventCommentAdded,
	EventCommentMention,
	EventStatusChange,
	EventPriorityChange,
	EventFieldChange,
	EventIssueCreated,
	EventReviewRequested,
	EventBlocked,
	EventSprintChange,
}

// validEventSet is used for O(1) ValidEventType lookups.
var validEventSet map[string]bool

func init() {
	validEventSet = make(map[string]bool, len(allEventTypes))
	for _, e := range allEventTypes {
		validEventSet[e] = true
	}
}

// AllEventTypes returns a slice containing all 11 canonical event type
// constants. The returned slice is a copy; callers may modify it freely.
func AllEventTypes() []string {
	result := make([]string, len(allEventTypes))
	copy(result, allEventTypes)
	return result
}

// ValidEventType reports whether s is one of the 11 canonical event types.
func ValidEventType(s string) bool {
	return validEventSet[s]
}

// ReactionKey returns the key used to look up a reaction in a character profile
// for the given event type. The key is the event type prefixed with "on_".
func ReactionKey(eventType string) string {
	return "on_" + eventType
}
