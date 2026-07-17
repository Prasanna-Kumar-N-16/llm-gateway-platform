package openai

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Prasanna-Kumar-N-16/llm-gateway-platform/pkg/provider"
)

func TestChatSuccess(t *testing.T) {
	var gotBody chatRequest
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != chatCompletionPath {
			t.Errorf("path = %q, want %q", r.URL.Path, chatCompletionPath)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer secret" {
			t.Errorf("Authorization = %q, want %q", got, "Bearer secret")
		}
		body, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(body, &gotBody)

		_, _ = io.WriteString(w, `{
			"model": "gpt-4o",
			"choices": [{"message": {"role":"assistant","content":"hi there"}, "finish_reason": "stop"}],
			"usage": {"prompt_tokens": 5, "completion_tokens": 2}
		}`)
	}))
	defer srv.Close()

	c, err := New("secret", WithBaseURL(srv.URL))
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	resp, err := c.Chat(context.Background(), &provider.ChatRequest{
		Model:     "gpt-4o",
		System:    "be nice",
		MaxTokens: 50,
		Messages:  []provider.Message{{Role: provider.RoleUser, Content: "hi"}},
	})
	if err != nil {
		t.Fatalf("Chat: %v", err)
	}

	if resp.Content != "hi there" {
		t.Errorf("Content = %q", resp.Content)
	}
	if resp.Provider != provider.OpenAI {
		t.Errorf("Provider = %q, want openai", resp.Provider)
	}
	if resp.Usage.InputTokens != 5 || resp.Usage.OutputTokens != 2 {
		t.Errorf("Usage = %+v", resp.Usage)
	}
	// System prompt is prepended as a system-role message for OpenAI.
	if len(gotBody.Messages) != 2 || gotBody.Messages[0].Role != "system" {
		t.Errorf("messages = %+v, want system prepended", gotBody.Messages)
	}
}

func TestChatRateLimitedIsRetryable(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
		_, _ = io.WriteString(w, `{"error":{"type":"rate_limit_error","code":"rate_limit_exceeded","message":"slow down"}}`)
	}))
	defer srv.Close()

	c, _ := New("secret", WithBaseURL(srv.URL))
	_, err := c.Chat(context.Background(), &provider.ChatRequest{Model: "m", MaxTokens: 10})
	if !provider.IsRetryable(err) {
		t.Errorf("429 should be retryable: %v", err)
	}
}

func TestChatEmptyChoices(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, `{"model":"m","choices":[],"usage":{}}`)
	}))
	defer srv.Close()

	c, _ := New("secret", WithBaseURL(srv.URL))
	if _, err := c.Chat(context.Background(), &provider.ChatRequest{Model: "m", MaxTokens: 10}); err == nil {
		t.Fatal("expected error for empty choices")
	}
}
