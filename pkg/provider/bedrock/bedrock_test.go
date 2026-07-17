package bedrock

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Prasanna-Kumar-N-16/llm-gateway-platform/pkg/provider"
)

// recordingSigner captures that signing happened without real credentials.
type recordingSigner struct{ called bool }

func (s *recordingSigner) Sign(req *http.Request, payload []byte, service, region string, t time.Time) error {
	s.called = true
	req.Header.Set("Authorization", "AWS4-HMAC-SHA256 test")
	return nil
}

func TestChatSuccess(t *testing.T) {
	var gotBody invokeRequest
	var gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		if r.Header.Get("Authorization") == "" {
			t.Error("request was not signed")
		}
		body, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(body, &gotBody)
		_, _ = io.WriteString(w, `{
			"stop_reason": "end_turn",
			"content": [{"type":"text","text":"from bedrock"}],
			"usage": {"input_tokens": 8, "output_tokens": 4}
		}`)
	}))
	defer srv.Close()

	signer := &recordingSigner{}
	c, err := New("us-east-1", signer, WithBaseURL(srv.URL))
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	resp, err := c.Chat(context.Background(), &provider.ChatRequest{
		Model:     "anthropic.claude-opus-4-8",
		System:    "sys",
		MaxTokens: 128,
		Messages:  []provider.Message{{Role: provider.RoleUser, Content: "hi"}},
	})
	if err != nil {
		t.Fatalf("Chat: %v", err)
	}

	if !signer.called {
		t.Error("signer was not invoked")
	}
	if resp.Content != "from bedrock" {
		t.Errorf("Content = %q", resp.Content)
	}
	if resp.Provider != provider.Bedrock {
		t.Errorf("Provider = %q, want bedrock", resp.Provider)
	}
	if resp.Model != "anthropic.claude-opus-4-8" {
		t.Errorf("Model = %q", resp.Model)
	}
	if gotBody.AnthropicVersion != bedrockAnthropicVersion {
		t.Errorf("anthropic_version = %q, want %q", gotBody.AnthropicVersion, bedrockAnthropicVersion)
	}
	if !strings.HasPrefix(gotPath, "/model/") || !strings.HasSuffix(gotPath, "/invoke") {
		t.Errorf("path = %q, want /model/{id}/invoke", gotPath)
	}
}

func TestChatServerErrorIsRetryable(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = io.WriteString(w, `{"message":"internal"}`)
	}))
	defer srv.Close()

	c, _ := New("us-east-1", &recordingSigner{}, WithBaseURL(srv.URL))
	_, err := c.Chat(context.Background(), &provider.ChatRequest{Model: "m", MaxTokens: 10})
	if !provider.IsRetryable(err) {
		t.Errorf("500 should be retryable: %v", err)
	}
}

func TestNewValidation(t *testing.T) {
	if _, err := New("", &recordingSigner{}); err == nil {
		t.Error("expected error for empty region")
	}
	if _, err := New("us-east-1", nil); err == nil {
		t.Error("expected error for nil signer")
	}
}
