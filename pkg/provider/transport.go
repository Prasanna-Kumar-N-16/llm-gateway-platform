package provider

import (
	"net/http"
	"time"
)

// Doer is the subset of *http.Client the adapters depend on. Accepting an
// interface keeps adapters unit-testable without a live network by allowing a
// fake transport to be injected.
type Doer interface {
	Do(req *http.Request) (*http.Response, error)
}

// DefaultHTTPClient returns an *http.Client with sane production timeouts for
// LLM traffic, where responses can legitimately take tens of seconds.
func DefaultHTTPClient() *http.Client {
	return &http.Client{
		Timeout: 120 * time.Second,
		Transport: &http.Transport{
			MaxIdleConns:        100,
			MaxIdleConnsPerHost: 10,
			IdleConnTimeout:     90 * time.Second,
		},
	}
}
