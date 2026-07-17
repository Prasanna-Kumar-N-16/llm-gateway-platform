package provider

import (
	"errors"
	"fmt"
	"net/http"
)

// Error is a normalized provider failure. It carries enough information for the
// router to decide whether to retry the same provider, fail over to another, or
// surface the error to the caller.
type Error struct {
	// Provider is the backend that produced the error.
	Provider Name
	// Status is the HTTP status code, or 0 for transport-level failures.
	Status int
	// Code is the provider's machine-readable error type, when available.
	Code string
	// Message is a human-readable description.
	Message string
	// Retryable indicates the request may succeed if retried, possibly after a
	// backoff, on the same provider.
	Retryable bool
	// wrapped is the underlying cause, if any.
	wrapped error
}

func (e *Error) Error() string {
	if e.Status != 0 {
		return fmt.Sprintf("provider %s: status %d: %s", e.Provider, e.Status, e.Message)
	}
	return fmt.Sprintf("provider %s: %s", e.Provider, e.Message)
}

func (e *Error) Unwrap() error { return e.wrapped }

// IsRetryable reports whether err represents a retryable provider failure.
func IsRetryable(err error) bool {
	var pErr *Error
	if errors.As(err, &pErr) {
		return pErr.Retryable
	}
	return false
}

// NewHTTPError builds an Error from an HTTP response status, classifying it as
// retryable per the provider error conventions (429 and 5xx are retryable).
func NewHTTPError(p Name, status int, code, message string) *Error {
	return &Error{
		Provider:  p,
		Status:    status,
		Code:      code,
		Message:   message,
		Retryable: retryableStatus(status),
	}
}

// NewTransportError builds a retryable Error for a connection-level failure
// (DNS, dial, TLS, timeout) where no HTTP response was received.
func NewTransportError(p Name, err error) *Error {
	return &Error{
		Provider:  p,
		Message:   fmt.Sprintf("transport error: %v", err),
		Retryable: true,
		wrapped:   err,
	}
}

// retryableStatus reports whether an HTTP status warrants a retry. 429 (rate
// limited), 500, 502, 503, 504, and 529 (overloaded) are transient.
func retryableStatus(status int) bool {
	switch status {
	case http.StatusTooManyRequests,
		http.StatusInternalServerError,
		http.StatusBadGateway,
		http.StatusServiceUnavailable,
		http.StatusGatewayTimeout,
		529: // Anthropic "overloaded_error"
		return true
	default:
		return false
	}
}
