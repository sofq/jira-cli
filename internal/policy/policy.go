package policy

import (
	"fmt"
	"path"
)

// Policy enforces operation-level access control per profile.
// A nil *Policy allows all operations (unrestricted mode).
type Policy struct {
	allowedOps []string // glob patterns — if set, only matching ops are allowed
	deniedOps  []string // glob patterns — if set, matching ops are denied
	mode       string   // "allow", "deny"
}

// NewFromConfig creates a Policy from config fields. Returns nil if both
// slices are empty (unrestricted). Returns error if both are non-empty.
func NewFromConfig(allowed, denied []string) (*Policy, error) {
	hasAllow := len(allowed) > 0
	hasDeny := len(denied) > 0

	if hasAllow && hasDeny {
		return nil, fmt.Errorf("profile cannot have both allowed_operations and denied_operations; use one or the other")
	}
	if !hasAllow && !hasDeny {
		return nil, nil
	}

	// Validate all patterns are well-formed.
	all := allowed
	if hasDeny {
		all = denied
	}
	for _, pattern := range all {
		if _, err := path.Match(pattern, ""); err != nil {
			return nil, fmt.Errorf("invalid glob pattern %q: %w", pattern, err)
		}
	}

	p := &Policy{}
	if hasAllow {
		p.allowedOps = allowed
		p.mode = "allow"
	} else {
		p.deniedOps = denied
		p.mode = "deny"
	}
	return p, nil
}

// Check returns nil if the operation is permitted, or an error describing why
// it was denied. A nil *Policy allows everything.
func (p *Policy) Check(operation string) error {
	if p == nil {
		return nil
	}

	switch p.mode {
	case "allow":
		for _, pattern := range p.allowedOps {
			if matched, _ := path.Match(pattern, operation); matched {
				return nil
			}
		}
		return &DeniedError{Operation: operation, Reason: "not in allowed_operations"}
	case "deny":
		for _, pattern := range p.deniedOps {
			if matched, _ := path.Match(pattern, operation); matched {
				return &DeniedError{Operation: operation, Reason: "matched denied_operations pattern: " + pattern}
			}
		}
		return nil
	}
	return nil
}

// DeniedError is returned when an operation is blocked by policy.
type DeniedError struct {
	Operation string
	Reason    string
}

func (e *DeniedError) Error() string {
	return fmt.Sprintf("operation %q denied: %s", e.Operation, e.Reason)
}
