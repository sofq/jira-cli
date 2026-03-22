package yolo

import (
	"testing"
)

func TestNewPolicyDisabled(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Enabled = false

	p, err := NewPolicy(cfg)
	if err != nil {
		t.Fatalf("NewPolicy: unexpected error: %v", err)
	}
	if p.IsEnabled() {
		t.Error("disabled policy: IsEnabled() should return false")
	}
	if p.Allowed("workflow comment") {
		t.Error("disabled policy: Allowed() should return false for any operation")
	}
	if p.Allowed("issue get") {
		t.Error("disabled policy: Allowed() should return false for any operation")
	}
}

func TestNewPolicySafeTier(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Enabled = true
	cfg.Scope = "safe"

	p, err := NewPolicy(cfg)
	if err != nil {
		t.Fatalf("NewPolicy: unexpected error: %v", err)
	}
	if !p.IsEnabled() {
		t.Error("enabled policy: IsEnabled() should return true")
	}

	cases := []struct {
		op      string
		allowed bool
	}{
		{"workflow comment", true},
		{"workflow transition", true},
		{"workflow assign", true},
		{"workflow log-work", true},
		{"workflow create", false},
		{"issue edit", false},
		{"issue get", false},
		{"issue delete", false},
		{"bulk create", false},
		{"raw GET", false},
	}
	for _, tc := range cases {
		t.Run(tc.op, func(t *testing.T) {
			got := p.Allowed(tc.op)
			if got != tc.allowed {
				t.Errorf("Allowed(%q) = %v, want %v", tc.op, got, tc.allowed)
			}
		})
	}
}

func TestNewPolicyStandardTier(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Enabled = true
	cfg.Scope = "standard"

	p, err := NewPolicy(cfg)
	if err != nil {
		t.Fatalf("NewPolicy: unexpected error: %v", err)
	}

	allowed := []string{
		"workflow comment", "workflow transition", "workflow assign", "workflow log-work",
		"workflow create", "issue edit", "workflow link", "workflow sprint",
	}
	denied := []string{
		"issue get", "issue delete", "bulk create", "raw GET", "project search",
	}

	for _, op := range allowed {
		t.Run("allow/"+op, func(t *testing.T) {
			if !p.Allowed(op) {
				t.Errorf("Allowed(%q) = false, want true", op)
			}
		})
	}
	for _, op := range denied {
		t.Run("deny/"+op, func(t *testing.T) {
			if p.Allowed(op) {
				t.Errorf("Allowed(%q) = true, want false", op)
			}
		})
	}
}

func TestNewPolicyFullTier(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Enabled = true
	cfg.Scope = "full"

	p, err := NewPolicy(cfg)
	if err != nil {
		t.Fatalf("NewPolicy: unexpected error: %v", err)
	}

	// Full tier allows everything except denied patterns.
	cases := []struct {
		op      string
		allowed bool
	}{
		{"workflow comment", true},
		{"issue get", true},
		{"project search", true},
		// denied: "* delete*"
		{"issue delete", false},
		{"project delete", false},
		// denied: "bulk *"
		{"bulk create", false},
		{"bulk update", false},
		// denied: "raw *"
		{"raw GET", false},
		{"raw POST", false},
	}
	for _, tc := range cases {
		t.Run(tc.op, func(t *testing.T) {
			got := p.Allowed(tc.op)
			if got != tc.allowed {
				t.Errorf("Allowed(%q) = %v, want %v", tc.op, got, tc.allowed)
			}
		})
	}
}

func TestNewPolicyCustomAllowed(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Enabled = true
	cfg.Scope = "safe"
	cfg.AllowedActions = []string{"issue get", "project search"}

	p, err := NewPolicy(cfg)
	if err != nil {
		t.Fatalf("NewPolicy: unexpected error: %v", err)
	}

	// With custom AllowedActions, only those ops are allowed (intersection
	// with scope tier means only ops in BOTH scope and custom are allowed).
	// Since "issue get" is not in safe tier, it is not allowed.
	// "workflow comment" is in safe tier but not in custom, so also not allowed.
	cases := []struct {
		op      string
		allowed bool
	}{
		{"issue get", false},       // in custom, not in safe tier
		{"project search", false},  // in custom, not in safe tier
		{"workflow comment", false}, // in safe tier, not in custom
	}
	for _, tc := range cases {
		t.Run(tc.op, func(t *testing.T) {
			got := p.Allowed(tc.op)
			if got != tc.allowed {
				t.Errorf("Allowed(%q) = %v, want %v", tc.op, got, tc.allowed)
			}
		})
	}
}

func TestNewPolicyCustomDenied(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Enabled = true
	cfg.Scope = "standard"
	cfg.DeniedActions = []string{"workflow create", "issue edit"}

	p, err := NewPolicy(cfg)
	if err != nil {
		t.Fatalf("NewPolicy: unexpected error: %v", err)
	}

	cases := []struct {
		op      string
		allowed bool
	}{
		{"workflow comment", true},
		{"workflow transition", true},
		{"workflow create", false},  // in standard but explicitly denied
		{"issue edit", false},       // in standard but explicitly denied
		{"workflow link", true},
		{"issue get", false},        // not in standard tier
	}
	for _, tc := range cases {
		t.Run(tc.op, func(t *testing.T) {
			got := p.Allowed(tc.op)
			if got != tc.allowed {
				t.Errorf("Allowed(%q) = %v, want %v", tc.op, got, tc.allowed)
			}
		})
	}
}

func TestNewPolicyGlobMatching(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Enabled = true
	cfg.Scope = "full"
	cfg.DeniedActions = []string{"issue *"}

	p, err := NewPolicy(cfg)
	if err != nil {
		t.Fatalf("NewPolicy: unexpected error: %v", err)
	}

	cases := []struct {
		op      string
		allowed bool
	}{
		{"workflow comment", true},
		{"issue get", false},
		{"issue edit", false},
		{"issue delete", false},
		{"project search", true},
	}
	for _, tc := range cases {
		t.Run(tc.op, func(t *testing.T) {
			got := p.Allowed(tc.op)
			if got != tc.allowed {
				t.Errorf("Allowed(%q) = %v, want %v", tc.op, got, tc.allowed)
			}
		})
	}
}

func TestNewPolicyUnknownScope(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Enabled = true
	cfg.Scope = "nonexistent"

	p, err := NewPolicy(cfg)
	if err != nil {
		t.Fatalf("NewPolicy: unexpected error: %v", err)
	}
	// Unknown scope with no custom allowed actions → deny everything.
	if p.Allowed("workflow comment") {
		t.Error("unknown scope: Allowed() should return false")
	}
}

func TestNewPolicyInvalidGlob(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Enabled = true
	cfg.Scope = "safe"
	cfg.AllowedActions = []string{"[invalid"}

	_, err := NewPolicy(cfg)
	if err == nil {
		t.Error("NewPolicy with invalid glob: expected error, got nil")
	}
}
