package yolo

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestRateLimiterAllowUntilExhausted(t *testing.T) {
	cfg := RateLimitConfig{PerHour: 60, Burst: 5}
	rl := NewRateLimiter(cfg, "default", "")

	// Should allow exactly Burst requests before exhausting.
	for i := 0; i < 5; i++ {
		if !rl.Allow() {
			t.Fatalf("Allow() = false on request %d, want true", i+1)
		}
	}

	// Next request must be denied.
	if rl.Allow() {
		t.Error("Allow() = true after burst exhausted, want false")
	}
}

func TestRateLimiterRemaining(t *testing.T) {
	cfg := RateLimitConfig{PerHour: 60, Burst: 10}
	rl := NewRateLimiter(cfg, "default", "")

	if got := rl.Remaining(); got != 10 {
		t.Errorf("Remaining() = %d, want 10 (full burst)", got)
	}

	rl.Allow()
	if got := rl.Remaining(); got != 9 {
		t.Errorf("Remaining() = %d, want 9 after one Allow()", got)
	}

	rl.Allow()
	rl.Allow()
	if got := rl.Remaining(); got != 7 {
		t.Errorf("Remaining() = %d, want 7 after three Allow() calls", got)
	}
}

func TestRateLimiterRemainingNeverNegative(t *testing.T) {
	cfg := RateLimitConfig{PerHour: 60, Burst: 2}
	rl := NewRateLimiter(cfg, "default", "")

	rl.Allow()
	rl.Allow()
	rl.Allow() // denied, but remaining should stay 0 not go negative

	if got := rl.Remaining(); got < 0 {
		t.Errorf("Remaining() = %d, want >= 0", got)
	}
	if got := rl.Remaining(); got != 0 {
		t.Errorf("Remaining() = %d, want 0 after burst exhausted", got)
	}
}

func TestRateLimiterPersistenceRoundTrip(t *testing.T) {
	dir := t.TempDir()
	stateFile := filepath.Join(dir, "rl_state.json")

	cfg := RateLimitConfig{PerHour: 60, Burst: 10}
	rl := NewRateLimiter(cfg, "myprofile", stateFile)

	// Consume 3 tokens.
	rl.Allow()
	rl.Allow()
	rl.Allow()

	if err := rl.Save(); err != nil {
		t.Fatalf("Save(): %v", err)
	}

	// Reload from state file.
	rl2 := NewRateLimiter(cfg, "myprofile", stateFile)
	if got := rl2.Remaining(); got != 7 {
		t.Errorf("after reload: Remaining() = %d, want 7", got)
	}
}

func TestRateLimiterPersistenceMultiProfile(t *testing.T) {
	dir := t.TempDir()
	stateFile := filepath.Join(dir, "rl_state.json")

	cfg := RateLimitConfig{PerHour: 60, Burst: 10}

	rlA := NewRateLimiter(cfg, "profileA", stateFile)
	rlA.Allow()
	rlA.Allow()
	if err := rlA.Save(); err != nil {
		t.Fatalf("Save profileA: %v", err)
	}

	rlB := NewRateLimiter(cfg, "profileB", stateFile)
	rlB.Allow()
	if err := rlB.Save(); err != nil {
		t.Fatalf("Save profileB: %v", err)
	}

	// Reload A — should have 8 remaining.
	rlA2 := NewRateLimiter(cfg, "profileA", stateFile)
	if got := rlA2.Remaining(); got != 8 {
		t.Errorf("profileA after reload: Remaining() = %d, want 8", got)
	}

	// Reload B — should have 9 remaining.
	rlB2 := NewRateLimiter(cfg, "profileB", stateFile)
	if got := rlB2.Remaining(); got != 9 {
		t.Errorf("profileB after reload: Remaining() = %d, want 9", got)
	}
}

func TestRateLimiterSaveCreatesFile(t *testing.T) {
	dir := t.TempDir()
	stateFile := filepath.Join(dir, "new_state.json")

	cfg := RateLimitConfig{PerHour: 60, Burst: 5}
	rl := NewRateLimiter(cfg, "default", stateFile)
	rl.Allow()

	if err := rl.Save(); err != nil {
		t.Fatalf("Save(): %v", err)
	}

	data, err := os.ReadFile(stateFile)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}

	var state map[string]rateLimiterState
	if err := json.Unmarshal(data, &state); err != nil {
		t.Fatalf("json.Unmarshal: %v", err)
	}
	if _, ok := state["default"]; !ok {
		t.Error("saved state missing 'default' profile key")
	}
}

func TestRateLimiterSaveNoFile(t *testing.T) {
	cfg := RateLimitConfig{PerHour: 60, Burst: 5}
	// Empty stateFile means no persistence; Save should be a no-op.
	rl := NewRateLimiter(cfg, "default", "")
	rl.Allow()

	if err := rl.Save(); err != nil {
		t.Errorf("Save() with empty stateFile: %v", err)
	}
}

func TestRateLimiterRefillAfterTime(t *testing.T) {
	// perHour=3600 means 1 token per second (3600/3600).
	cfg := RateLimitConfig{PerHour: 3600, Burst: 2}
	rl := NewRateLimiter(cfg, "default", "")

	// Exhaust all tokens.
	rl.Allow()
	rl.Allow()
	if rl.Allow() {
		t.Fatal("Allow() = true, want false after burst exhausted")
	}

	// Manually advance the last refill time by 2 seconds to simulate refill.
	rl.mu.Lock()
	rl.lastRefill = rl.lastRefill.Add(-2 * time.Second)
	rl.mu.Unlock()

	// Should have refilled at least 1 token.
	// (2s * 3600/3600 tokens/s = 2 tokens, capped at burst=2)
	if !rl.Allow() {
		t.Error("Allow() = false after time-based refill, want true")
	}
}

func TestRateLimiterZeroBurst(t *testing.T) {
	cfg := RateLimitConfig{PerHour: 60, Burst: 0}
	rl := NewRateLimiter(cfg, "default", "")

	// With zero burst, every request should be denied.
	if rl.Allow() {
		t.Error("Allow() = true with zero burst, want false")
	}
	if rl.Remaining() != 0 {
		t.Errorf("Remaining() = %d, want 0 with zero burst", rl.Remaining())
	}
}

func TestRateLimiterConcurrentSafety(t *testing.T) {
	cfg := RateLimitConfig{PerHour: 3600, Burst: 100}
	rl := NewRateLimiter(cfg, "default", "")

	// Run 50 goroutines each making 3 Allow() calls.
	done := make(chan struct{})
	for i := 0; i < 50; i++ {
		go func() {
			rl.Allow()
			rl.Allow()
			rl.Allow()
			done <- struct{}{}
		}()
	}
	for i := 0; i < 50; i++ {
		<-done
	}

	// Remaining must be non-negative and not exceed burst.
	rem := rl.Remaining()
	if rem < 0 || rem > 100 {
		t.Errorf("Remaining() = %d after concurrent calls, want [0, 100]", rem)
	}
}
