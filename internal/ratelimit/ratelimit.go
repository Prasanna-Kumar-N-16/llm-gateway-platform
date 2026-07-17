// Package ratelimit implements a concurrency-safe token-bucket rate limiter
// used to protect providers from exceeding their per-minute request budgets.
package ratelimit

import (
	"context"
	"sync"
	"time"
)

// Limiter is a token-bucket rate limiter. Tokens refill continuously at a fixed
// rate up to a burst capacity. It is safe for concurrent use.
type Limiter struct {
	mu     sync.Mutex
	rate   float64 // tokens added per second
	burst  float64 // maximum tokens
	tokens float64
	last   time.Time
	now    func() time.Time
}

// New returns a Limiter permitting ratePerSecond sustained requests with the
// given burst capacity. The bucket starts full.
func New(ratePerSecond float64, burst int) *Limiter {
	if ratePerSecond <= 0 {
		ratePerSecond = 1
	}
	if burst < 1 {
		burst = 1
	}
	return &Limiter{
		rate:   ratePerSecond,
		burst:  float64(burst),
		tokens: float64(burst),
		last:   time.Now(),
		now:    time.Now,
	}
}

// Allow reports whether a request may proceed now, consuming one token if so.
func (l *Limiter) Allow() bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.refill()
	if l.tokens >= 1 {
		l.tokens--
		return true
	}
	return false
}

// Wait blocks until a token is available or ctx is cancelled. It consumes one
// token on success.
func (l *Limiter) Wait(ctx context.Context) error {
	for {
		l.mu.Lock()
		l.refill()
		if l.tokens >= 1 {
			l.tokens--
			l.mu.Unlock()
			return nil
		}
		deficit := 1 - l.tokens
		wait := time.Duration(deficit / l.rate * float64(time.Second))
		l.mu.Unlock()

		if wait <= 0 {
			wait = time.Millisecond
		}
		timer := time.NewTimer(wait)
		select {
		case <-ctx.Done():
			timer.Stop()
			return ctx.Err()
		case <-timer.C:
		}
	}
}

// refill adds tokens accrued since the last update. Caller must hold l.mu.
func (l *Limiter) refill() {
	now := l.now()
	elapsed := now.Sub(l.last).Seconds()
	if elapsed <= 0 {
		return
	}
	l.last = now
	l.tokens += elapsed * l.rate
	if l.tokens > l.burst {
		l.tokens = l.burst
	}
}
