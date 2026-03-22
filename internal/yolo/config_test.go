package yolo

import (
	"testing"
)

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()

	if cfg.Enabled {
		t.Error("DefaultConfig: Enabled should be false")
	}
	if cfg.Scope != "safe" {
		t.Errorf("DefaultConfig: Scope got %q, want %q", cfg.Scope, "safe")
	}
	if cfg.Tag {
		t.Error("DefaultConfig: Tag should be false")
	}
	if cfg.TagText != "[via jr]" {
		t.Errorf("DefaultConfig: TagText got %q, want %q", cfg.TagText, "[via jr]")
	}
	if cfg.RateLimit.PerHour != 30 {
		t.Errorf("DefaultConfig: RateLimit.PerHour got %d, want 30", cfg.RateLimit.PerHour)
	}
	if cfg.RateLimit.Burst != 10 {
		t.Errorf("DefaultConfig: RateLimit.Burst got %d, want 10", cfg.RateLimit.Burst)
	}
	if !cfg.RequireAudit {
		t.Error("DefaultConfig: RequireAudit should be true")
	}
	if cfg.DecisionEngine != "rules" {
		t.Errorf("DefaultConfig: DecisionEngine got %q, want %q", cfg.DecisionEngine, "rules")
	}
}

func TestScopeTierOps(t *testing.T) {
	cases := []struct {
		tier     string
		contains []string
		notIn    []string
	}{
		{
			tier:     "safe",
			contains: []string{"workflow comment", "workflow transition", "workflow assign", "workflow log-work"},
			notIn:    []string{"workflow create", "issue edit", "*"},
		},
		{
			tier:     "standard",
			contains: []string{"workflow comment", "workflow transition", "workflow assign", "workflow log-work", "workflow create", "issue edit", "workflow link", "workflow sprint"},
			notIn:    []string{"*"},
		},
		{
			tier:     "full",
			contains: []string{"*"},
			notIn:    []string{},
		},
		{
			tier:     "unknown",
			contains: []string{},
			notIn:    []string{},
		},
	}

	for _, tc := range cases {
		t.Run(tc.tier, func(t *testing.T) {
			ops := ScopeTierOps(tc.tier)
			opSet := make(map[string]bool)
			for _, op := range ops {
				opSet[op] = true
			}
			for _, want := range tc.contains {
				if !opSet[want] {
					t.Errorf("tier %q: expected %q in ops, got %v", tc.tier, want, ops)
				}
			}
			for _, notWant := range tc.notIn {
				if opSet[notWant] {
					t.Errorf("tier %q: did not expect %q in ops", tc.tier, notWant)
				}
			}
		})
	}
}

func TestScopeTierDenied(t *testing.T) {
	cases := []struct {
		tier     string
		contains []string
		empty    bool
	}{
		{
			tier:  "safe",
			empty: true,
		},
		{
			tier:  "standard",
			empty: true,
		},
		{
			tier:     "full",
			contains: []string{"* delete*", "bulk *", "raw *"},
			empty:    false,
		},
		{
			tier:  "unknown",
			empty: true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.tier, func(t *testing.T) {
			denied := ScopeTierDenied(tc.tier)
			if tc.empty && len(denied) != 0 {
				t.Errorf("tier %q: expected empty denied list, got %v", tc.tier, denied)
			}
			if !tc.empty {
				deniedSet := make(map[string]bool)
				for _, d := range denied {
					deniedSet[d] = true
				}
				for _, want := range tc.contains {
					if !deniedSet[want] {
						t.Errorf("tier %q: expected %q in denied, got %v", tc.tier, want, denied)
					}
				}
			}
		})
	}
}

func TestConfigJSONTags(t *testing.T) {
	// Verify that Config has the correct zero values before defaults are applied.
	var cfg Config
	if cfg.Enabled {
		t.Error("zero Config: Enabled should be false")
	}
	if cfg.Scope != "" {
		t.Errorf("zero Config: Scope should be empty string, got %q", cfg.Scope)
	}
	if cfg.RateLimit.PerHour != 0 {
		t.Errorf("zero Config: RateLimit.PerHour should be 0, got %d", cfg.RateLimit.PerHour)
	}
}

func TestScopeTierOpsStandardIncludesSafe(t *testing.T) {
	safe := ScopeTierOps("safe")
	standard := ScopeTierOps("standard")

	standardSet := make(map[string]bool)
	for _, op := range standard {
		standardSet[op] = true
	}

	for _, op := range safe {
		if !standardSet[op] {
			t.Errorf("standard tier missing safe op %q", op)
		}
	}
}
