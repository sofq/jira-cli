package yolo

import (
	"fmt"
	"path"
)

// Policy enforces operation-level access control for the yolo execution engine.
// A disabled policy denies all operations. An enabled policy applies scope tier
// patterns plus optional custom allowed/denied patterns.
type Policy struct {
	enabled bool
	allowed []string // glob patterns — operation must match at least one
	denied  []string // glob patterns — deny wins if matched
}

// NewPolicy constructs a Policy from a Config. Custom AllowedActions and
// DeniedActions are additive restrictions on top of the scope tier:
//   - AllowedActions further restricts the scope tier (intersection)
//   - DeniedActions further blocks operations the scope tier would allow
func NewPolicy(cfg Config) (*Policy, error) {
	p := &Policy{enabled: cfg.Enabled}
	if !cfg.Enabled {
		return p, nil
	}

	// Validate all custom patterns before use.
	for _, pat := range cfg.AllowedActions {
		if _, err := path.Match(pat, ""); err != nil {
			return nil, fmt.Errorf("yolo: invalid allowed_actions glob %q: %w", pat, err)
		}
	}
	for _, pat := range cfg.DeniedActions {
		if _, err := path.Match(pat, ""); err != nil {
			return nil, fmt.Errorf("yolo: invalid denied_actions glob %q: %w", pat, err)
		}
	}

	// Build the effective allowed set:
	// - Start from scope tier ops.
	// - If custom AllowedActions are provided, intersect (only ops in BOTH lists
	//   are kept).
	scopeOps := ScopeTierOps(cfg.Scope)

	if len(cfg.AllowedActions) > 0 {
		// Intersection: an op must appear in both scopeOps AND custom list.
		// We check by keeping only custom patterns that also match something
		// in the scope tier. In practice we build an allowed list that is the
		// intersection of the two sets (literal strings only; globs in custom
		// are matched against scope ops).
		var intersected []string
		for _, custom := range cfg.AllowedActions {
			for _, scopeOp := range scopeOps {
				if matched, _ := path.Match(custom, scopeOp); matched {
					intersected = append(intersected, scopeOp)
					break
				}
			}
		}
		p.allowed = intersected
	} else {
		p.allowed = scopeOps
	}

	// Build the effective denied set:
	// Tier-level denied patterns come first, then custom DeniedActions.
	tierDenied := ScopeTierDenied(cfg.Scope)
	p.denied = make([]string, 0, len(tierDenied)+len(cfg.DeniedActions))
	p.denied = append(p.denied, tierDenied...)
	p.denied = append(p.denied, cfg.DeniedActions...)

	return p, nil
}

// IsEnabled reports whether the policy is active.
func (p *Policy) IsEnabled() bool {
	return p.enabled
}

// Allowed reports whether the given operation is permitted by this policy.
// A disabled policy denies all operations. For an enabled policy, denied
// patterns are checked first (deny wins), then allowed patterns.
func (p *Policy) Allowed(operation string) bool {
	if !p.enabled {
		return false
	}

	// Deny wins: check denied patterns first.
	for _, pat := range p.denied {
		if matched, _ := path.Match(pat, operation); matched {
			return false
		}
	}

	// Check allowed patterns.
	for _, pat := range p.allowed {
		if matched, _ := path.Match(pat, operation); matched {
			return true
		}
	}

	return false
}
