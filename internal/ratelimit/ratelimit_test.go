package ratelimit

import (
	"context"
	"testing"
	"time"
)

func TestAllowConsumesBurstThenDenies(t *testing.T) {
	l := New(1, 3)
	// Freeze time so no refill occurs mid-test.
	now := time.Now()
	l.now = func() time.Time { return now }
	l.last = now
	l.tokens = 3

	for i := 0; i < 3; i++ {
		if !l.Allow() {
			t.Fatalf("burst token %d should be allowed", i)
		}
	}
	if l.Allow() {
		t.Fatal("4th request should be denied — burst exhausted")
	}
}

func TestRefillOverTime(t *testing.T) {
	l := New(10, 5) // 10 tokens/sec
	now := time.Now()
	l.now = func() time.Time { return now }
	l.last = now
	l.tokens = 0

	// Advance 300ms → 3 tokens should accrue.
	now = now.Add(300 * time.Millisecond)
	if !l.Allow() || !l.Allow() || !l.Allow() {
		t.Fatal("expected 3 tokens to have refilled")
	}
	if l.Allow() {
		t.Fatal("only 3 tokens should have refilled")
	}
}

func TestRefillCapsAtBurst(t *testing.T) {
	l := New(100, 2)
	now := time.Now()
	l.now = func() time.Time { return now }
	l.last = now
	l.tokens = 0

	now = now.Add(10 * time.Second) // would add 1000 tokens uncapped
	l.mu.Lock()
	l.refill()
	tokens := l.tokens
	l.mu.Unlock()
	if tokens != 2 {
		t.Errorf("tokens = %v, want capped at burst 2", tokens)
	}
}

func TestWaitReturnsWhenTokenAvailable(t *testing.T) {
	l := New(1000, 1) // fast refill
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := l.Wait(ctx); err != nil {
		t.Fatalf("Wait should succeed immediately: %v", err)
	}
}

func TestWaitHonorsContext(t *testing.T) {
	l := New(1, 1)
	// Drain the single token.
	if !l.Allow() {
		t.Fatal("first token should be available")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	if err := l.Wait(ctx); err == nil {
		t.Fatal("Wait should return context error when starved")
	}
}
