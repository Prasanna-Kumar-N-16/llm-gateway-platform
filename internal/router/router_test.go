package router

import (
	"context"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/Prasanna-Kumar-N-16/llm-gateway-platform/internal/ratelimit"
	"github.com/Prasanna-Kumar-N-16/llm-gateway-platform/internal/retry"
	"github.com/Prasanna-Kumar-N-16/llm-gateway-platform/pkg/provider"
)

// fakeProvider is a scriptable provider for router tests.
type fakeProvider struct {
	name  provider.Name
	calls int
	fn    func(*provider.ChatRequest) (*provider.ChatResponse, error)
}

func (f *fakeProvider) Name() provider.Name { return f.name }

func (f *fakeProvider) Chat(_ context.Context, req *provider.ChatRequest) (*provider.ChatResponse, error) {
	f.calls++
	return f.fn(req)
}

func quietRouter(opts ...Option) *Router {
	base := []Option{
		WithLogger(slog.New(slog.NewTextHandler(io.Discard, nil))),
		WithRetryPolicy(retry.Policy{MaxAttempts: 2, BaseDelay: time.Millisecond, Jitter: false}),
	}
	return New(append(base, opts...)...)
}

func okProvider(name provider.Name, content string) *fakeProvider {
	return &fakeProvider{name: name, fn: func(req *provider.ChatRequest) (*provider.ChatResponse, error) {
		return &provider.ChatResponse{Provider: name, Model: req.Model, Content: content}, nil
	}}
}

func TestChatPrimarySucceeds(t *testing.T) {
	r := quietRouter()
	primary := okProvider(provider.Anthropic, "hello")
	r.Register(primary)
	r.AddRoute("chat-default", Route{Targets: []Target{
		{Provider: provider.Anthropic, Model: "claude-opus-4-8"},
	}})

	res, err := r.Chat(context.Background(), &provider.ChatRequest{Model: "chat-default", MaxTokens: 10})
	if err != nil {
		t.Fatalf("Chat: %v", err)
	}
	if res.Response.Content != "hello" || res.Target.Provider != provider.Anthropic {
		t.Errorf("unexpected result: %+v", res)
	}
	if res.Target.Model != "claude-opus-4-8" {
		t.Errorf("target model = %q", res.Target.Model)
	}
}

func TestChatFailsOverOnRetryableError(t *testing.T) {
	r := quietRouter()
	primary := &fakeProvider{name: provider.Anthropic, fn: func(*provider.ChatRequest) (*provider.ChatResponse, error) {
		return nil, provider.NewHTTPError(provider.Anthropic, 503, "", "down")
	}}
	secondary := okProvider(provider.OpenAI, "backup answer")
	r.Register(primary)
	r.Register(secondary)
	r.AddRoute("chat-default", Route{Targets: []Target{
		{Provider: provider.Anthropic, Model: "claude-opus-4-8"},
		{Provider: provider.OpenAI, Model: "gpt-4o"},
	}})

	res, err := r.Chat(context.Background(), &provider.ChatRequest{Model: "chat-default", MaxTokens: 10})
	if err != nil {
		t.Fatalf("Chat: %v", err)
	}
	if res.Target.Provider != provider.OpenAI || res.Response.Content != "backup answer" {
		t.Errorf("expected failover to openai, got %+v", res)
	}
	// Primary should have been retried (2 attempts) before failover.
	if primary.calls != 2 {
		t.Errorf("primary calls = %d, want 2 (retried then failed over)", primary.calls)
	}
}

func TestChatDoesNotFailOverOnClientError(t *testing.T) {
	r := quietRouter()
	primary := &fakeProvider{name: provider.Anthropic, fn: func(*provider.ChatRequest) (*provider.ChatResponse, error) {
		return nil, provider.NewHTTPError(provider.Anthropic, 400, "invalid_request_error", "bad")
	}}
	secondary := okProvider(provider.OpenAI, "should not be reached")
	r.Register(primary)
	r.Register(secondary)
	r.AddRoute("chat-default", Route{Targets: []Target{
		{Provider: provider.Anthropic, Model: "claude-opus-4-8"},
		{Provider: provider.OpenAI, Model: "gpt-4o"},
	}})

	_, err := r.Chat(context.Background(), &provider.ChatRequest{Model: "chat-default", MaxTokens: 10})
	if err == nil {
		t.Fatal("expected error, got success")
	}
	if secondary.calls != 0 {
		t.Errorf("secondary must not be tried on a 400: calls = %d", secondary.calls)
	}
	if primary.calls != 1 {
		t.Errorf("primary should not be retried on 400: calls = %d", primary.calls)
	}
}

func TestChatRateLimitFailsOver(t *testing.T) {
	r := quietRouter()
	primary := okProvider(provider.Anthropic, "primary")
	secondary := okProvider(provider.OpenAI, "secondary")
	r.Register(primary)
	r.Register(secondary)

	// Exhausted limiter on the primary.
	limiter := ratelimit.New(1, 1)
	if !limiter.Allow() {
		t.Fatal("setup: expected first token")
	}
	r.SetLimiter(provider.Anthropic, limiter)

	r.AddRoute("chat-default", Route{Targets: []Target{
		{Provider: provider.Anthropic, Model: "a"},
		{Provider: provider.OpenAI, Model: "b"},
	}})

	res, err := r.Chat(context.Background(), &provider.ChatRequest{Model: "chat-default", MaxTokens: 10})
	if err != nil {
		t.Fatalf("Chat: %v", err)
	}
	if res.Target.Provider != provider.OpenAI {
		t.Errorf("expected failover past rate-limited primary, got %s", res.Target.Provider)
	}
	if primary.calls != 0 {
		t.Errorf("rate-limited primary must not be invoked: calls = %d", primary.calls)
	}
}

func TestChatUsesDefaultRoute(t *testing.T) {
	r := quietRouter()
	r.Register(okProvider(provider.OpenAI, "default"))
	r.SetDefaultRoute(Route{Targets: []Target{{Provider: provider.OpenAI, Model: "gpt-4o"}}})

	res, err := r.Chat(context.Background(), &provider.ChatRequest{Model: "unmapped-model", MaxTokens: 10})
	if err != nil {
		t.Fatalf("Chat: %v", err)
	}
	if res.Response.Content != "default" {
		t.Errorf("expected default route to serve, got %+v", res)
	}
}

func TestChatNoRoute(t *testing.T) {
	r := quietRouter()
	if _, err := r.Chat(context.Background(), &provider.ChatRequest{Model: "x", MaxTokens: 10}); err == nil {
		t.Fatal("expected no-route error")
	}
}

func TestChatAllTargetsFail(t *testing.T) {
	r := quietRouter()
	primary := &fakeProvider{name: provider.Anthropic, fn: func(*provider.ChatRequest) (*provider.ChatResponse, error) {
		return nil, provider.NewHTTPError(provider.Anthropic, 503, "", "down")
	}}
	r.Register(primary)
	r.AddRoute("chat-default", Route{Targets: []Target{{Provider: provider.Anthropic, Model: "a"}}})

	_, err := r.Chat(context.Background(), &provider.ChatRequest{Model: "chat-default", MaxTokens: 10})
	var routeErr *RouteError
	if !asRouteError(err, &routeErr) {
		t.Fatalf("expected *RouteError, got %T", err)
	}
	if len(routeErr.Failures) != 1 {
		t.Errorf("failures = %d, want 1", len(routeErr.Failures))
	}
}

func asRouteError(err error, target **RouteError) bool {
	re, ok := err.(*RouteError)
	if ok {
		*target = re
	}
	return ok
}
