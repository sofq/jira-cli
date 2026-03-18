package policy_test

import (
	"testing"

	"github.com/sofq/jira-cli/internal/policy"
)

func TestNewFromConfig_AllowOnly(t *testing.T) {
	p, err := policy.NewFromConfig([]string{"issue get", "search *"}, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if p == nil {
		t.Fatal("expected non-nil policy")
	}
}

func TestNewFromConfig_DenyOnly(t *testing.T) {
	p, err := policy.NewFromConfig(nil, []string{"* delete*", "bulk *"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if p == nil {
		t.Fatal("expected non-nil policy")
	}
}

func TestNewFromConfig_BothSetReturnsError(t *testing.T) {
	_, err := policy.NewFromConfig([]string{"issue get"}, []string{"bulk *"})
	if err == nil {
		t.Fatal("expected error when both allowed and denied are set")
	}
}

func TestNewFromConfig_NeitherSet(t *testing.T) {
	p, err := policy.NewFromConfig(nil, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if p != nil {
		t.Fatal("expected nil policy when neither set")
	}
}

func TestCheck_AllowMode(t *testing.T) {
	p, _ := policy.NewFromConfig([]string{"issue get", "issue edit", "search *", "workflow *"}, nil)

	tests := []struct {
		op      string
		allowed bool
	}{
		{"issue get", true},
		{"issue edit", true},
		{"issue delete", false},
		{"search search-and-reconsile-issues-using-jql", true},
		{"workflow transition", true},
		{"bulk submit-delete", false},
		{"raw POST", false},
	}
	for _, tt := range tests {
		t.Run(tt.op, func(t *testing.T) {
			err := p.Check(tt.op)
			if tt.allowed && err != nil {
				t.Errorf("expected allowed, got error: %v", err)
			}
			if !tt.allowed && err == nil {
				t.Errorf("expected denied, got nil")
			}
		})
	}
}

func TestCheck_DenyMode(t *testing.T) {
	p, _ := policy.NewFromConfig(nil, []string{"* delete*", "bulk *", "raw *"})

	tests := []struct {
		op      string
		allowed bool
	}{
		{"issue get", true},
		{"issue create-issue", true},
		{"issue delete", false},
		{"issue delete-comment", false},
		{"bulk submit-delete", false},
		{"raw POST", false},
		{"raw GET", false},
		{"workflow transition", true},
	}
	for _, tt := range tests {
		t.Run(tt.op, func(t *testing.T) {
			err := p.Check(tt.op)
			if tt.allowed && err != nil {
				t.Errorf("expected allowed, got error: %v", err)
			}
			if !tt.allowed && err == nil {
				t.Errorf("expected denied, got nil")
			}
		})
	}
}

func TestCheck_NilPolicy(t *testing.T) {
	var p *policy.Policy
	if err := p.Check("anything"); err != nil {
		t.Errorf("nil policy should allow everything, got: %v", err)
	}
}
