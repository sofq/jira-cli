package retry

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestDo_SucceedsFirstTry(t *testing.T) {
	calls := 0
	err := Do(context.Background(), func() error {
		calls++
		return nil
	}, Config{MaxRetries: 3, BaseDelay: time.Millisecond})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if calls != 1 {
		t.Fatalf("expected 1 call, got %d", calls)
	}
}

func TestDo_RetriesOnRetryableError(t *testing.T) {
	calls := 0
	err := Do(context.Background(), func() error {
		calls++
		if calls < 3 {
			return &RetryableError{Err: errors.New("transient")}
		}
		return nil
	}, Config{MaxRetries: 5, BaseDelay: time.Millisecond})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if calls != 3 {
		t.Fatalf("expected 3 calls, got %d", calls)
	}
}

func TestDo_NoRetryOnNonRetryable(t *testing.T) {
	calls := 0
	err := Do(context.Background(), func() error {
		calls++
		return errors.New("permanent")
	}, Config{MaxRetries: 3, BaseDelay: time.Millisecond})
	if err == nil {
		t.Fatal("expected error")
	}
	if calls != 1 {
		t.Fatalf("expected 1 call, got %d", calls)
	}
}

func TestDo_ExhaustsRetries(t *testing.T) {
	calls := 0
	err := Do(context.Background(), func() error {
		calls++
		return &RetryableError{Err: errors.New("always fails")}
	}, Config{MaxRetries: 3, BaseDelay: time.Millisecond})
	if err == nil {
		t.Fatal("expected error")
	}
	if calls != 4 { // 1 initial + 3 retries
		t.Fatalf("expected 4 calls, got %d", calls)
	}
}

func TestDo_RespectsRetryAfter(t *testing.T) {
	calls := 0
	start := time.Now()
	err := Do(context.Background(), func() error {
		calls++
		if calls < 2 {
			return &RetryableError{Err: errors.New("transient"), RetryAfter: 5 * time.Millisecond}
		}
		return nil
	}, Config{MaxRetries: 3, BaseDelay: time.Second}) // BaseDelay is large; RetryAfter must override
	elapsed := time.Since(start)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if calls != 2 {
		t.Fatalf("expected 2 calls, got %d", calls)
	}
	// Should have slept ~5ms, not ~1s from BaseDelay
	if elapsed >= 500*time.Millisecond {
		t.Errorf("expected RetryAfter to override BaseDelay; elapsed=%v", elapsed)
	}
}

func TestDo_RespectsContextCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	calls := 0
	// Cancel context after first attempt
	err := Do(ctx, func() error {
		calls++
		if calls == 1 {
			cancel()
		}
		return &RetryableError{Err: errors.New("transient"), RetryAfter: time.Millisecond}
	}, Config{MaxRetries: 5, BaseDelay: time.Millisecond})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context.Canceled, got %v", err)
	}
}

func TestRetryableError_Error(t *testing.T) {
	re := &RetryableError{Err: errors.New("something went wrong")}
	if got := re.Error(); got != "something went wrong" {
		t.Errorf("Error() = %q, want %q", got, "something went wrong")
	}
}

func TestRetryableError_Unwrap(t *testing.T) {
	inner := errors.New("inner error")
	re := &RetryableError{Err: inner}
	if got := re.Unwrap(); got != inner {
		t.Errorf("Unwrap() = %v, want %v", got, inner)
	}
}

func TestBackoff_CapsAtMaxDelay(t *testing.T) {
	base := time.Millisecond
	max := 4 * time.Millisecond
	// attempt=10 → base * 2^10 = 1024ms, far above max of 4ms
	got := backoff(10, base, max)
	lo := time.Duration(float64(max) * 0.75)
	hi := time.Duration(float64(max) * 1.25)
	if got < lo || got > hi {
		t.Errorf("backoff with capped delay = %v, want in [%v, %v]", got, lo, hi)
	}
}

func TestShouldRetryStatus(t *testing.T) {
	tests := []struct {
		status int
		want   bool
	}{
		{429, true},
		{500, true},
		{502, true},
		{503, true},
		{504, true},
		{400, false},
		{401, false},
		{404, false},
		{200, false},
	}
	for _, tt := range tests {
		if got := ShouldRetryStatus(tt.status); got != tt.want {
			t.Errorf("ShouldRetryStatus(%d) = %v, want %v", tt.status, got, tt.want)
		}
	}
}
