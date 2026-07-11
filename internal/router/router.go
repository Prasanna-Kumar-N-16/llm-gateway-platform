// Package router implements the gateway's request routing engine: it maps a
// logical model name onto an ordered chain of provider targets, applies
// per-provider rate limiting and retry, and fails over to the next target when
// a provider is unavailable.
package router

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/Prasanna-Kumar-N-16/llm-gateway-platform/internal/ratelimit"
	"github.com/Prasanna-Kumar-N-16/llm-gateway-platform/internal/retry"
	"github.com/Prasanna-Kumar-N-16/llm-gateway-platform/pkg/provider"
)

// Target names a concrete provider and the model to invoke on it.
type Target struct {
	Provider provider.Name
	Model    string
}

// Route is an ordered fallback chain. Targets are tried in order until one
// succeeds.
type Route struct {
	Targets []Target
}

// Router dispatches unified chat requests across registered providers.
type Router struct {
	mu        sync.RWMutex
	providers map[provider.Name]provider.Provider
	limiters  map[provider.Name]*ratelimit.Limiter
	routes    map[string]Route
	fallback  Route

	retry    retry.Policy
	failover func(error) bool
	log      *slog.Logger
}

// Option configures a Router.
type Option func(*Router)

// WithRetryPolicy sets the per-target retry policy.
func WithRetryPolicy(p retry.Policy) Option { return func(r *Router) { r.retry = p } }

// WithLogger sets the structured logger.
func WithLogger(l *slog.Logger) Option { return func(r *Router) { r.log = l } }

// WithFailoverPredicate customizes which errors trigger failover to the next
// target. The default fails over on retryable (availability) errors only, so
// client errors such as HTTP 400 short-circuit instead of wasting spend on
// other providers.
func WithFailoverPredicate(fn func(error) bool) Option {
	return func(r *Router) { r.failover = fn }
}

// New constructs an empty Router.
func New(opts ...Option) *Router {
	r := &Router{
		providers: make(map[provider.Name]provider.Provider),
		limiters:  make(map[provider.Name]*ratelimit.Limiter),
		routes:    make(map[string]Route),
		retry:     retry.DefaultPolicy(),
		failover:  provider.IsRetryable,
		log:       slog.Default(),
	}
	for _, opt := range opts {
		opt(r)
	}
	return r
}

// Register makes a provider available for routing.
func (r *Router) Register(p provider.Provider) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.providers[p.Name()] = p
}

// SetLimiter attaches a rate limiter to a provider. Requests to that provider
// that cannot obtain a token fail over to the next target.
func (r *Router) SetLimiter(name provider.Name, l *ratelimit.Limiter) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.limiters[name] = l
}

// AddRoute registers a fallback chain for a logical model name.
func (r *Router) AddRoute(logicalModel string, route Route) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.routes[logicalModel] = route
}

// SetDefaultRoute sets the chain used when a request's model has no explicit
// route.
func (r *Router) SetDefaultRoute(route Route) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.fallback = route
}

// Result reports which target ultimately served a request.
type Result struct {
	Response *provider.ChatResponse
	Target   Target
	Attempts int
}

// RouteError aggregates the failures from every target that was tried.
type RouteError struct {
	LogicalModel string
	Failures     []TargetFailure
}

// TargetFailure records why a single target did not serve the request.
type TargetFailure struct {
	Target Target
	Err    error
}

func (e *RouteError) Error() string {
	return fmt.Sprintf("router: all %d target(s) failed for model %q", len(e.Failures), e.LogicalModel)
}

// Unwrap exposes the final underlying failure for errors.Is/As.
func (e *RouteError) Unwrap() error {
	if len(e.Failures) == 0 {
		return nil
	}
	return e.Failures[len(e.Failures)-1].Err
}

// Chat routes req through the resolved fallback chain and returns the first
// successful response.
func (r *Router) Chat(ctx context.Context, req *provider.ChatRequest) (*Result, error) {
	route, ok := r.resolve(req.Model)
	if !ok || len(route.Targets) == 0 {
		return nil, fmt.Errorf("router: no route for model %q", req.Model)
	}

	routeErr := &RouteError{LogicalModel: req.Model}

	for _, target := range route.Targets {
		if err := ctx.Err(); err != nil {
			return nil, err
		}

		p, limiter := r.lookup(target.Provider)
		if p == nil {
			routeErr.record(target, fmt.Errorf("provider %q is not registered", target.Provider))
			continue
		}
		if limiter != nil && !limiter.Allow() {
			r.log.Warn("router: target rate limited, failing over",
				slog.String("provider", string(target.Provider)),
				slog.String("model", target.Model))
			routeErr.record(target, provider.NewHTTPError(target.Provider, 429, "rate_limited", "local rate limit exceeded"))
			continue
		}

		sub := cloneForTarget(req, target)
		var resp *provider.ChatResponse
		var attempts int
		start := time.Now()

		err := retry.Do(ctx, r.retry, func(ctx context.Context) error {
			attempts++
			var callErr error
			resp, callErr = p.Chat(ctx, sub)
			return callErr
		})

		if err == nil {
			r.log.Info("router: request served",
				slog.String("provider", string(target.Provider)),
				slog.String("model", target.Model),
				slog.Int("attempts", attempts),
				slog.Duration("latency", time.Since(start)))
			return &Result{Response: resp, Target: target, Attempts: attempts}, nil
		}

		// Context cancellation is terminal — do not keep failing over.
		if ctx.Err() != nil || errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			routeErr.record(target, err)
			return nil, routeErr
		}

		routeErr.record(target, err)
		if !r.failover(err) {
			// A non-availability failure (e.g. bad request) will not be fixed by
			// another provider; return it immediately.
			return nil, routeErr
		}
		r.log.Warn("router: target failed, failing over",
			slog.String("provider", string(target.Provider)),
			slog.String("model", target.Model),
			slog.Int("attempts", attempts),
			slog.Any("error", err))
	}

	return nil, routeErr
}

func (r *Router) resolve(logicalModel string) (Route, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if route, ok := r.routes[logicalModel]; ok {
		return route, true
	}
	if len(r.fallback.Targets) > 0 {
		return r.fallback, true
	}
	return Route{}, false
}

func (r *Router) lookup(name provider.Name) (provider.Provider, *ratelimit.Limiter) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.providers[name], r.limiters[name]
}

func (e *RouteError) record(t Target, err error) {
	e.Failures = append(e.Failures, TargetFailure{Target: t, Err: err})
}

func cloneForTarget(req *provider.ChatRequest, t Target) *provider.ChatRequest {
	sub := *req
	sub.Model = t.Model
	// Messages and Metadata are read-only for adapters; sharing is safe.
	return &sub
}
