// Package yolo implements the "You Only Live Once" execution policy for jr.
// It gates autonomous agent actions behind policy checks, rate limits, and
// optional decision engines (rule-based or LLM-assisted).
package yolo

// Config holds all settings for the yolo execution policy.
type Config struct {
	Enabled        bool              `json:"enabled,omitempty"`
	Scope          string            `json:"scope,omitempty"`
	Character      string            `json:"character,omitempty"`
	Compose        map[string]string `json:"compose,omitempty"`
	Tag            bool              `json:"tag,omitempty"`
	TagText        string            `json:"tag_text,omitempty"`
	RateLimit      RateLimitConfig   `json:"rate_limit,omitempty"`
	AllowedActions []string          `json:"allowed_actions,omitempty"`
	DeniedActions  []string          `json:"denied_actions,omitempty"`
	RequireAudit   bool              `json:"require_audit,omitempty"`
	DecisionEngine string            `json:"decision_engine,omitempty"`
	LLMCmd         string            `json:"llm_cmd,omitempty"`
	LLMTimeout     string            `json:"llm_timeout,omitempty"`
}

// RateLimitConfig configures the token bucket rate limiter.
type RateLimitConfig struct {
	PerHour int `json:"per_hour,omitempty"`
	Burst   int `json:"burst,omitempty"`
}

// DefaultConfig returns a Config with safe defaults. The policy is disabled by
// default; callers must explicitly opt in by setting Enabled = true.
func DefaultConfig() Config {
	return Config{
		Enabled:        false,
		Scope:          "safe",
		Tag:            false,
		TagText:        "[via jr]",
		RateLimit:      RateLimitConfig{PerHour: 30, Burst: 10},
		RequireAudit:   true,
		DecisionEngine: "rules",
	}
}

// safeOps is the base set of operations allowed under the "safe" scope tier.
var safeOps = []string{
	"workflow comment",
	"workflow transition",
	"workflow assign",
	"workflow log-work",
}

// standardExtraOps are the additional operations added by the "standard" tier
// on top of the safe tier.
var standardExtraOps = []string{
	"workflow create",
	"issue edit",
	"workflow link",
	"workflow sprint",
}

// fullDeniedOps are operations always blocked under the "full" scope tier
// to prevent destructive or privileged actions.
var fullDeniedOps = []string{
	"* delete*",
	"bulk *",
	"raw *",
}

// ScopeTierOps returns the allowed operation patterns for the given scope tier.
// Returns nil for unknown tiers.
func ScopeTierOps(tier string) []string {
	switch tier {
	case "safe":
		result := make([]string, len(safeOps))
		copy(result, safeOps)
		return result
	case "standard":
		result := make([]string, 0, len(safeOps)+len(standardExtraOps))
		result = append(result, safeOps...)
		result = append(result, standardExtraOps...)
		return result
	case "full":
		return []string{"*"}
	default:
		return nil
	}
}

// ScopeTierDenied returns the denied operation patterns for the given scope
// tier. Only "full" tier has denied patterns; all others return nil.
func ScopeTierDenied(tier string) []string {
	switch tier {
	case "full":
		result := make([]string, len(fullDeniedOps))
		copy(result, fullDeniedOps)
		return result
	default:
		return nil
	}
}
