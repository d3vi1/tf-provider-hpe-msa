package msa

import (
	"context"
	"math"
	"math/rand"
	"time"
)

type RetryConfig struct {
	MaxAttempts int
	MinBackoff  time.Duration
	MaxBackoff  time.Duration
	Jitter      float64
}

func (r RetryConfig) withDefaults(defaultAttempts int) RetryConfig {
	if r.MaxAttempts == 0 {
		r.MaxAttempts = defaultAttempts
	}
	if r.MinBackoff == 0 {
		r.MinBackoff = 200 * time.Millisecond
	}
	if r.MaxBackoff == 0 {
		r.MaxBackoff = 2 * time.Second
	}
	if r.Jitter == 0 {
		r.Jitter = 0.2
	}
	return r
}

func doWithRetry(ctx context.Context, config RetryConfig, fn func() (bool, error)) error {
	var lastErr error

	for attempt := 1; attempt <= config.MaxAttempts; attempt++ {
		retry, err := fn()
		if err == nil {
			return nil
		}
		lastErr = err
		if !retry || attempt == config.MaxAttempts {
			break
		}

		wait := backoffDuration(config, attempt)
		timer := time.NewTimer(wait)
		select {
		case <-ctx.Done():
			timer.Stop()
			return ctx.Err()
		case <-timer.C:
		}
	}

	return lastErr
}

func backoffDuration(config RetryConfig, attempt int) time.Duration {
	base := float64(config.MinBackoff) * math.Pow(2, float64(attempt-1))
	max := float64(config.MaxBackoff)
	if base > max {
		base = max
	}

	jitter := 1 + (rand.Float64()*2-1)*config.Jitter
	if jitter < 0 {
		jitter = 0
	}

	return time.Duration(base * jitter)
}

func isRetryableStatus(status int) bool {
	switch status {
	case 429, 500, 502, 503, 504:
		return true
	default:
		return false
	}
}
