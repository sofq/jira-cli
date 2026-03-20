package retry

import (
	"math"
	"math/rand/v2"
	"time"
)

// RetryableError wraps an error to indicate it should be retried.
type RetryableError struct {
	Err        error
	RetryAfter time.Duration // 0 means use backoff
}

func (e *RetryableError) Error() string { return e.Err.Error() }
func (e *RetryableError) Unwrap() error { return e.Err }

// Config controls retry behavior.
type Config struct {
	MaxRetries int           // max number of retries (0 = no retries)
	BaseDelay  time.Duration // initial delay between retries
	MaxDelay   time.Duration // cap on delay (0 = 30s)
}

// Do executes fn, retrying on RetryableError up to MaxRetries times.
// Non-RetryableError errors are returned immediately.
func Do(fn func() error, cfg Config) error {
	if cfg.MaxDelay == 0 {
		cfg.MaxDelay = 30 * time.Second
	}

	var lastErr error
	for attempt := 0; attempt <= cfg.MaxRetries; attempt++ {
		err := fn()
		if err == nil {
			return nil
		}

		var retryable *RetryableError
		if !isRetryable(err, &retryable) {
			return err
		}
		lastErr = retryable.Err

		if attempt == cfg.MaxRetries {
			break
		}

		delay := retryable.RetryAfter
		if delay == 0 {
			delay = backoff(attempt, cfg.BaseDelay, cfg.MaxDelay)
		}
		time.Sleep(delay)
	}
	return lastErr
}

func isRetryable(err error, target **RetryableError) bool {
	re, ok := err.(*RetryableError)
	if ok {
		*target = re
		return true
	}
	return false
}

func backoff(attempt int, base, max time.Duration) time.Duration {
	delay := time.Duration(float64(base) * math.Pow(2, float64(attempt)))
	if delay > max {
		delay = max
	}
	// Add jitter: 75%-125% of calculated delay
	jitter := 0.75 + rand.Float64()*0.5 // #nosec G404 -- jitter does not need cryptographic randomness
	return time.Duration(float64(delay) * jitter)
}

// ShouldRetryStatus returns true for HTTP status codes that are typically transient.
func ShouldRetryStatus(status int) bool {
	switch status {
	case 429, 500, 502, 503, 504:
		return true
	default:
		return false
	}
}
