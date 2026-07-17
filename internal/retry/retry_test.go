package retry

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/Prasanna-Kumar-N-16/llm-gateway-platform/pkg/provider"
)

func fastPolicy(max int) Policy {
	return Policy{MaxAttempts: max, BaseDelay: time.Millisecond, MaxDelay: 2 * time.Millisecond, Jitter: false}
}

func TestDoSucceedsFirstTry(t *testing.T) {
	calls := 0
	err := Do(context.Background(), fastPolicy(3), func(context.Context) error {
		calls++
		return nil
	})
	if err != nil || calls != 1 {
		t.Fatalf("err=%v calls=%d, want nil/1", err, calls)
	}
}

func TestDoRetriesRetryableThenSucceeds(t *testing.T) {
	calls := 0
	err := Do(context.Background(), fastPolicy(3), func(context.Context) error {
		calls++
		if calls < 3 {
			return provider.NewHTTPError(provider.OpenAI, 503, "", "unavailable")
		}
		return nil
	})
	if err != nil {
		t.Fatalf("err = %v, want nil", err)
	}
	if calls != 3 {
		t.Errorf("calls = %d, want 3", calls)
	}
}

func TestDoStopsOnNonRetryable(t *testing.T) {
	calls := 0
	err := Do(context.Background(), fastPolicy(5), func(context.Context) error {
		calls++
		return provider.NewHTTPError(provider.OpenAI, 400, "", "bad request")
	})
	if err == nil {
		t.Fatal("expected error")
	}
	if calls != 1 {
		t.Errorf("calls = %d, want 1 (no retry on 400)", calls)
	}
}

func TestDoExhaustsAttempts(t *testing.T) {
	calls := 0
	err := Do(context.Background(), fastPolicy(3), func(context.Context) error {
		calls++
		return provider.NewHTTPError(provider.OpenAI, 503, "", "unavailable")
	})
	if !provider.IsRetryable(err) {
		t.Errorf("final error should still be the retryable one: %v", err)
	}
	if calls != 3 {
		t.Errorf("calls = %d, want 3", calls)
	}
}

func TestDoHonorsContextCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	calls := 0
	err := Do(ctx, Policy{MaxAttempts: 5, BaseDelay: time.Hour}, func(context.Context) error {
		calls++
		cancel() // cancel during the first attempt; backoff should abort
		return provider.NewHTTPError(provider.OpenAI, 503, "", "unavailable")
	})
	if !errors.Is(err, context.Canceled) {
		t.Errorf("err = %v, want context.Canceled", err)
	}
	if calls != 1 {
		t.Errorf("calls = %d, want 1", calls)
	}
}

func TestBackoffGrowsAndCaps(t *testing.T) {
	p := Policy{BaseDelay: 100 * time.Millisecond, MaxDelay: 300 * time.Millisecond}
	if d := p.backoff(1); d != 100*time.Millisecond {
		t.Errorf("backoff(1) = %v, want 100ms", d)
	}
	if d := p.backoff(2); d != 200*time.Millisecond {
		t.Errorf("backoff(2) = %v, want 200ms", d)
	}
	if d := p.backoff(3); d != 300*time.Millisecond {
		t.Errorf("backoff(3) = %v, want 300ms (capped)", d)
	}
	if d := p.backoff(9); d != 300*time.Millisecond {
		t.Errorf("backoff(9) = %v, want 300ms (capped)", d)
	}
}
