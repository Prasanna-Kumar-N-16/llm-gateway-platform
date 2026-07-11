package provider

import (
	"errors"
	"fmt"
	"testing"
)

func TestNewHTTPErrorRetryClassification(t *testing.T) {
	tests := []struct {
		status    int
		retryable bool
	}{
		{400, false},
		{401, false},
		{404, false},
		{429, true},
		{500, true},
		{502, true},
		{503, true},
		{504, true},
		{529, true},
	}
	for _, tt := range tests {
		err := NewHTTPError(OpenAI, tt.status, "code", "message")
		if err.Retryable != tt.retryable {
			t.Errorf("status %d: Retryable = %v, want %v", tt.status, err.Retryable, tt.retryable)
		}
		if !IsRetryable(err) == tt.retryable {
			t.Errorf("status %d: IsRetryable disagrees with Retryable field", tt.status)
		}
	}
}

func TestTransportErrorIsRetryable(t *testing.T) {
	err := NewTransportError(Anthropic, errors.New("dial tcp: timeout"))
	if !IsRetryable(err) {
		t.Fatal("transport error should be retryable")
	}
	if !errors.Is(err, err.Unwrap()) {
		t.Error("Unwrap should expose the wrapped cause")
	}
}

func TestIsRetryableWithWrappedError(t *testing.T) {
	base := NewHTTPError(OpenAI, 503, "", "unavailable")
	wrapped := fmt.Errorf("router: %w", base)
	if !IsRetryable(wrapped) {
		t.Error("IsRetryable should unwrap to find the provider error")
	}
}

func TestIsRetryablePlainError(t *testing.T) {
	if IsRetryable(errors.New("boom")) {
		t.Error("a plain error must not be retryable")
	}
}

func TestUsageTotalTokens(t *testing.T) {
	u := Usage{InputTokens: 10, OutputTokens: 25}
	if got := u.TotalTokens(); got != 35 {
		t.Errorf("TotalTokens = %d, want 35", got)
	}
}
