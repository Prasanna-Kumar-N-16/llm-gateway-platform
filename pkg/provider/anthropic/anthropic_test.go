package anthropic

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
	var gotBody messagesRequest
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != messagesPath {
			t.Errorf("path = %q, want %q", r.URL.Path, messagesPath)
		}
		if got := r.Header.Get("x-api-key"); got != "secret" {
			t.Errorf("x-api-key = %q, want %q", got, "secret")
		}
		if got := r.Header.Get("anthropic-version"); got != defaultAPIVersion {
			t.Errorf("anthropic-version = %q, want %q", got, defaultAPIVersion)
		}
		body, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(body, &gotBody)

		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{
			"model": "claude-opus-4-8",
			"stop_reason": "end_turn",
			"content": [{"type":"text","text":"Hello "},{"type":"text","text":"world"}],
			"usage": {"input_tokens": 12, "output_tokens": 3}
		}`)
	}))
	defer srv.Close()

	c, err := New("secret", WithBaseURL(srv.URL))
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	resp, err := c.Chat(context.Background(), &provider.ChatRequest{
		Model:     "claude-opus-4-8",
		System:    "be terse",
		MaxTokens: 100,
		Messages: []provider.Message{
			{Role: provider.RoleUser, Content: "hi"},
		},
	})
	if err != nil {
		t.Fatalf("Chat: %v", err)
	}

	if resp.Content != "Hello world" {
		t.Errorf("Content = %q, want %q", resp.Content, "Hello world")
	}
	if resp.Provider != provider.Anthropic {
		t.Errorf("Provider = %q, want anthropic", resp.Provider)
	}
	if resp.FinishReason != provider.FinishStop {
		t.Errorf("FinishReason = %q, want stop", resp.FinishReason)
	}
	if resp.Usage.InputTokens != 12 || resp.Usage.OutputTokens != 3 {
		t.Errorf("Usage = %+v, want {12 3}", resp.Usage)
	}
	// The system prompt must be a top-level field, not a message.
	if gotBody.System != "be terse" {
		t.Errorf("request System = %q, want %q", gotBody.System, "be terse")
	}
	if len(gotBody.Messages) != 1 {
		t.Errorf("request carried %d messages, want 1", len(gotBody.Messages))
	}
}

func TestChatFoldsSystemRoleMessage(t *testing.T) {
	var gotBody messagesRequest
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(body, &gotBody)
		_, _ = io.WriteString(w, `{"model":"m","stop_reason":"end_turn","content":[],"usage":{}}`)
	}))
	defer srv.Close()

	c, _ := New("secret", WithBaseURL(srv.URL))
	_, err := c.Chat(context.Background(), &provider.ChatRequest{
		Model:     "m",
		MaxTokens: 10,
		Messages: []provider.Message{
			{Role: provider.RoleSystem, Content: "sys"},
			{Role: provider.RoleUser, Content: "hi"},
		},
	})
	if err != nil {
		t.Fatalf("Chat: %v", err)
	}
	if gotBody.System != "sys" {
		t.Errorf("System = %q, want folded system-role message", gotBody.System)
	}
	if len(gotBody.Messages) != 1 || gotBody.Messages[0].Role != "user" {
		t.Errorf("system-role message should not appear in messages: %+v", gotBody.Messages)
	}
}

func TestChatHTTPErrorIsRetryable(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
		_, _ = io.WriteString(w, `{"error":{"type":"overloaded_error","message":"overloaded"}}`)
	}))
	defer srv.Close()

	c, _ := New("secret", WithBaseURL(srv.URL))
	_, err := c.Chat(context.Background(), &provider.ChatRequest{Model: "m", MaxTokens: 10})
	if err == nil {
		t.Fatal("expected error")
	}
	if !provider.IsRetryable(err) {
		t.Errorf("503 should be retryable, got %v", err)
	}
	var pErr *provider.Error
	if !asProviderError(err, &pErr) || pErr.Code != "overloaded_error" {
		t.Errorf("error code not parsed: %v", err)
	}
}

func TestChatClientErrorNotRetryable(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = io.WriteString(w, `{"error":{"type":"invalid_request_error","message":"bad"}}`)
	}))
	defer srv.Close()

	c, _ := New("secret", WithBaseURL(srv.URL))
	_, err := c.Chat(context.Background(), &provider.ChatRequest{Model: "m", MaxTokens: 10})
	if provider.IsRetryable(err) {
		t.Errorf("400 must not be retryable: %v", err)
	}
}

func TestNewRequiresAPIKey(t *testing.T) {
	if _, err := New(""); err == nil {
		t.Fatal("expected error for empty api key")
	}
}

func asProviderError(err error, target **provider.Error) bool {
	pe, ok := err.(*provider.Error)
	if ok {
		*target = pe
	}
	return ok
}
