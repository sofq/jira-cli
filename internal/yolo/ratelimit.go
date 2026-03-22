package yolo

import (
	"encoding/json"
	"os"
	"sync"
	"time"
)

// rateLimiterState is the persisted state for a single profile.
type rateLimiterState struct {
	Tokens     float64   `json:"tokens"`
	LastRefill time.Time `json:"last_refill"`
}

// RateLimiter implements a thread-safe token bucket rate limiter. Tokens are
// refilled continuously at a rate of PerHour/3600 tokens per second. The
// bucket holds at most Burst tokens. State is optionally persisted to disk as
// a JSON map keyed by profile name, so limits survive process restarts.
type RateLimiter struct {
	mu         sync.Mutex
	cfg        RateLimitConfig
	profile    string
	stateFile  string
	tokens     float64
	lastRefill time.Time
}

// NewRateLimiter creates a RateLimiter for the given profile. If stateFile is
// non-empty, the persisted state for the profile is loaded from that file.
func NewRateLimiter(cfg RateLimitConfig, profile, stateFile string) *RateLimiter {
	rl := &RateLimiter{
		cfg:        cfg,
		profile:    profile,
		stateFile:  stateFile,
		tokens:     float64(cfg.Burst),
		lastRefill: time.Now(),
	}
	if stateFile != "" {
		rl.load()
	}
	return rl
}

// load reads the state file and applies the stored state for the profile.
// Errors are silently ignored — the limiter starts fresh if the file is
// missing or unreadable.
func (rl *RateLimiter) load() {
	data, err := os.ReadFile(rl.stateFile)
	if err != nil {
		return
	}
	var m map[string]rateLimiterState
	if err := json.Unmarshal(data, &m); err != nil {
		return
	}
	if s, ok := m[rl.profile]; ok {
		// Cap tokens to burst in case config changed.
		burst := float64(rl.cfg.Burst)
		tokens := s.Tokens
		if tokens > burst {
			tokens = burst
		}
		if tokens < 0 {
			tokens = 0
		}
		rl.tokens = tokens
		rl.lastRefill = s.LastRefill
	}
}

// refill adds tokens based on elapsed time since the last refill. Must be
// called with rl.mu held.
func (rl *RateLimiter) refill() {
	if rl.cfg.PerHour <= 0 {
		return
	}
	now := time.Now()
	elapsed := now.Sub(rl.lastRefill).Seconds()
	rate := float64(rl.cfg.PerHour) / 3600.0 // tokens per second
	rl.tokens += elapsed * rate
	burst := float64(rl.cfg.Burst)
	if rl.tokens > burst {
		rl.tokens = burst
	}
	rl.lastRefill = now
}

// Allow consumes one token and reports whether the operation is permitted.
// Returns false when no tokens are available.
func (rl *RateLimiter) Allow() bool {
	rl.mu.Lock()
	defer rl.mu.Unlock()
	rl.refill()
	if rl.tokens < 1 {
		return false
	}
	rl.tokens--
	return true
}

// Remaining returns the number of whole tokens currently available.
func (rl *RateLimiter) Remaining() int {
	rl.mu.Lock()
	defer rl.mu.Unlock()
	rl.refill()
	t := int(rl.tokens)
	if t < 0 {
		return 0
	}
	return t
}

// Save persists the current state for the profile to the state file. It is a
// no-op when stateFile is empty. The file is read-modified-written atomically
// to support multiple profiles sharing a single state file.
func (rl *RateLimiter) Save() error {
	if rl.stateFile == "" {
		return nil
	}

	rl.mu.Lock()
	snap := rateLimiterState{
		Tokens:     rl.tokens,
		LastRefill: rl.lastRefill,
	}
	rl.mu.Unlock()

	// Read existing state, update our profile entry, then write back.
	m := make(map[string]rateLimiterState)
	data, err := os.ReadFile(rl.stateFile)
	if err == nil {
		// Ignore unmarshal errors — start with empty map.
		_ = json.Unmarshal(data, &m)
	}
	m[rl.profile] = snap

	out, err := json.Marshal(m)
	if err != nil {
		return err
	}
	return os.WriteFile(rl.stateFile, out, 0o600)
}
