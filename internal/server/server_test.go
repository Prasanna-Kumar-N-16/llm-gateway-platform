package server

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Prasanna-Kumar-N-16/llm-gateway-platform/internal/config"
	"github.com/Prasanna-Kumar-N-16/llm-gateway-platform/internal/retry"
	"github.com/Prasanna-Kumar-N-16/llm-gateway-platform/internal/router"
	"github.com/Prasanna-Kumar-N-16/llm-gateway-platform/pkg/provider"
)

func testLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func testServer(t *testing.T) *Server {
	t.Helper()
	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("config.Load: %v", err)
	}
	return New(cfg, testLogger(), router.New(router.WithLogger(testLogger())))
}

// fakeProvider is a scriptable provider used to test the chat endpoint
// without making a real upstream call.
type fakeProvider struct {
	name provider.Name
	fn   func(*provider.ChatRequest) (*provider.ChatResponse, error)
}

func (f *fakeProvider) Name() provider.Name { return f.name }

func (f *fakeProvider) Chat(_ context.Context, req *provider.ChatRequest) (*provider.ChatResponse, error) {
	return f.fn(req)
}

// chatServer builds a Server whose router serves "chat-default" from a
// single fake provider target.
func chatServer(t *testing.T, fn func(*provider.ChatRequest) (*provider.ChatResponse, error)) *Server {
	t.Helper()
	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("config.Load: %v", err)
	}
	rtr := router.New(
		router.WithLogger(testLogger()),
		router.WithRetryPolicy(retry.Policy{MaxAttempts: 1}),
	)
	rtr.Register(&fakeProvider{name: provider.Anthropic, fn: fn})
	rtr.AddRoute("chat-default", router.Route{Targets: []router.Target{
		{Provider: provider.Anthropic, Model: "claude-opus-4-8"},
	}})
	return New(cfg, testLogger(), rtr)
}

func TestHealthEndpoint(t *testing.T) {
	s := testServer(t)

	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	rec := httptest.NewRecorder()
	s.routes.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	var body map[string]any
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	if body["status"] != "ok" {
		t.Errorf("status field = %v, want %q", body["status"], "ok")
	}
}

func TestReadyEndpoint(t *testing.T) {
	s := testServer(t)

	req := httptest.NewRequest(http.MethodGet, "/readyz", nil)
	rec := httptest.NewRecorder()
	s.routes.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
}

func postChat(s *Server, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewBufferString(body))
	rec := httptest.NewRecorder()
	s.routes.ServeHTTP(rec, req)
	return rec
}

func TestChatCompletionsSuccess(t *testing.T) {
	s := chatServer(t, func(req *provider.ChatRequest) (*provider.ChatResponse, error) {
		return &provider.ChatResponse{
			Provider:     provider.Anthropic,
			Model:        req.Model,
			Content:      "hello there",
			FinishReason: provider.FinishStop,
			Usage:        provider.Usage{InputTokens: 3, OutputTokens: 2},
		}, nil
	})

	rec := postChat(s, `{"model":"chat-default","max_tokens":10,"messages":[{"role":"user","content":"hi"}]}`)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusOK, rec.Body.String())
	}
	var body chatCompletionResponse
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	if body.Content != "hello there" || body.Provider != provider.Anthropic {
		t.Errorf("unexpected body: %+v", body)
	}
	if body.Attempts != 1 {
		t.Errorf("attempts = %d, want 1", body.Attempts)
	}
}

func TestChatCompletionsInvalidRequest(t *testing.T) {
	s := chatServer(t, func(*provider.ChatRequest) (*provider.ChatResponse, error) {
		t.Fatal("provider should not be called for an invalid request")
		return nil, nil
	})

	cases := []string{
		`not json`,
		`{"messages":[{"role":"user","content":"hi"}]}`,
		`{"model":"chat-default","messages":[]}`,
	}
	for _, body := range cases {
		rec := postChat(s, body)
		if rec.Code != http.StatusBadRequest {
			t.Errorf("body %q: status = %d, want %d", body, rec.Code, http.StatusBadRequest)
		}
	}
}

func TestChatCompletionsUnknownModel(t *testing.T) {
	s := chatServer(t, func(*provider.ChatRequest) (*provider.ChatResponse, error) {
		t.Fatal("provider should not be called for an unrouted model")
		return nil, nil
	})

	rec := postChat(s, `{"model":"does-not-exist","max_tokens":10,"messages":[{"role":"user","content":"hi"}]}`)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusBadRequest, rec.Body.String())
	}
}

func TestChatCompletionsUpstreamFailure(t *testing.T) {
	s := chatServer(t, func(*provider.ChatRequest) (*provider.ChatResponse, error) {
		return nil, provider.NewHTTPError(provider.Anthropic, http.StatusServiceUnavailable, "overloaded", "try again")
	})

	rec := postChat(s, `{"model":"chat-default","max_tokens":10,"messages":[{"role":"user","content":"hi"}]}`)
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusServiceUnavailable, rec.Body.String())
	}
}
