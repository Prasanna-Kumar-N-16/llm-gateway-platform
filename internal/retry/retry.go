// Package retry provides bounded retry with exponential backoff and jitter for
// transient provider failures.
package retry

import (
	"context"
	"math/rand"
	"time"

	"github.com/Prasanna-Kumar-N-16/llm-gateway-platform/pkg/provider"
)

// Policy configures retry behavior.
type Policy struct {
	// MaxAttempts is the total number of tries, including the first. A value
	// <= 1 disables retrying.
	MaxAttempts int
	// BaseDelay is the backoff before the second attempt; it doubles each time.
	BaseDelay time.Duration
	// MaxDelay caps the backoff between attempts.
	MaxDelay time.Duration
	// Jitter, when true, randomizes each delay in [delay/2, delay] to avoid
	// synchronized retry storms across callers.
	Jitter bool
}

// DefaultPolicy returns a conservative policy suitable for LLM traffic.
func DefaultPolicy() Policy {
	return Policy{
		MaxAttempts: 3,
		BaseDelay:   200 * time.Millisecond,
		MaxDelay:    5 * time.Second,
		Jitter:      true,
	}
}

// Do invokes fn, retrying while it returns a retryable provider error and
// attempts remain. It stops early on a non-retryable error, on success, or when
// ctx is cancelled. The last error is returned when attempts are exhausted.
func Do(ctx context.Context, p Policy, fn func(context.Context) error) error {
	attempts := p.MaxAttempts
	if attempts < 1 {
		attempts = 1
	}

	var err error
	for attempt := 1; attempt <= attempts; attempt++ {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return ctxErr
		}

		err = fn(ctx)
		if err == nil {
			return nil
		}
		if !provider.IsRetryable(err) || attempt == attempts {
			return err
		}

		if waitErr := sleep(ctx, p.backoff(attempt)); waitErr != nil {
			return waitErr
		}
	}
	return err
}

// backoff returns the delay before the (attempt+1)th try.
func (p Policy) backoff(attempt int) time.Duration {
	delay := p.BaseDelay
	for i := 1; i < attempt; i++ {
		delay *= 2
		if p.MaxDelay > 0 && delay > p.MaxDelay {
			delay = p.MaxDelay
			break
		}
	}
	if p.MaxDelay > 0 && delay > p.MaxDelay {
		delay = p.MaxDelay
	}
	if p.Jitter && delay > 0 {
		half := delay / 2
		delay = half + time.Duration(rand.Int63n(int64(half)+1))
	}
	return delay
}

func sleep(ctx context.Context, d time.Duration) error {
	if d <= 0 {
		return nil
	}
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}
