// Package openai adapts the gateway's unified provider contract onto the
// OpenAI Chat Completions API (POST /v1/chat/completions).
package openai

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/Prasanna-Kumar-N-16/llm-gateway-platform/pkg/provider"
)

const (
	defaultBaseURL     = "https://api.openai.com/v1"
	chatCompletionPath = "/chat/completions"
)

// Client is an OpenAI-backed provider.
type Client struct {
	apiKey  string
	baseURL string
	http    provider.Doer
}

// Option configures a Client.
type Option func(*Client)

// WithBaseURL overrides the API base URL (useful for tests, Azure, or proxies).
func WithBaseURL(u string) Option { return func(c *Client) { c.baseURL = strings.TrimRight(u, "/") } }

// WithHTTPClient injects a custom transport.
func WithHTTPClient(d provider.Doer) Option { return func(c *Client) { c.http = d } }

// New constructs an OpenAI client. The API key is required.
func New(apiKey string, opts ...Option) (*Client, error) {
	if apiKey == "" {
		return nil, fmt.Errorf("openai: api key is required")
	}
	c := &Client{
		apiKey:  apiKey,
		baseURL: defaultBaseURL,
		http:    provider.DefaultHTTPClient(),
	}
	for _, opt := range opts {
		opt(c)
	}
	return c, nil
}

// Name reports the provider identity.
func (c *Client) Name() provider.Name { return provider.OpenAI }

type chatRequest struct {
	Model       string    `json:"model"`
	Messages    []wireMsg `json:"messages"`
	MaxTokens   int       `json:"max_tokens,omitempty"`
	Temperature *float64  `json:"temperature,omitempty"`
	User        string    `json:"user,omitempty"`
}

type wireMsg struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type chatResponse struct {
	Model   string `json:"model"`
	Choices []struct {
		Message      wireMsg `json:"message"`
		FinishReason string  `json:"finish_reason"`
	} `json:"choices"`
	Usage struct {
		PromptTokens     int `json:"prompt_tokens"`
		CompletionTokens int `json:"completion_tokens"`
	} `json:"usage"`
}

type errorEnvelope struct {
	Error struct {
		Type    string `json:"type"`
		Code    string `json:"code"`
		Message string `json:"message"`
	} `json:"error"`
}

// Chat performs a single non-streaming Chat Completions call.
func (c *Client) Chat(ctx context.Context, req *provider.ChatRequest) (*provider.ChatResponse, error) {
	msgs := make([]wireMsg, 0, len(req.Messages)+1)
	if req.System != "" {
		msgs = append(msgs, wireMsg{Role: string(provider.RoleSystem), Content: req.System})
	}
	for _, m := range req.Messages {
		msgs = append(msgs, wireMsg{Role: string(m.Role), Content: m.Content})
	}

	body := chatRequest{
		Model:       req.Model,
		Messages:    msgs,
		MaxTokens:   req.MaxTokens,
		Temperature: req.Temperature,
		User:        req.Metadata["user_id"],
	}

	raw, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("openai: marshal request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+chatCompletionPath, bytes.NewReader(raw))
	if err != nil {
		return nil, fmt.Errorf("openai: build request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+c.apiKey)

	resp, err := c.http.Do(httpReq)
	if err != nil {
		return nil, provider.NewTransportError(provider.OpenAI, err)
	}
	defer resp.Body.Close()

	payload, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, provider.NewTransportError(provider.OpenAI, err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, parseError(resp.StatusCode, payload)
	}

	var out chatResponse
	if err := json.Unmarshal(payload, &out); err != nil {
		return nil, fmt.Errorf("openai: decode response: %w", err)
	}
	if len(out.Choices) == 0 {
		return nil, fmt.Errorf("openai: response contained no choices")
	}

	return &provider.ChatResponse{
		Provider:     provider.OpenAI,
		Model:        out.Model,
		Content:      out.Choices[0].Message.Content,
		FinishReason: mapFinishReason(out.Choices[0].FinishReason),
		Usage: provider.Usage{
			InputTokens:  out.Usage.PromptTokens,
			OutputTokens: out.Usage.CompletionTokens,
		},
	}, nil
}

func parseError(status int, payload []byte) error {
	var env errorEnvelope
	code, message := "", strings.TrimSpace(string(payload))
	if json.Unmarshal(payload, &env) == nil && env.Error.Message != "" {
		code = env.Error.Code
		if code == "" {
			code = env.Error.Type
		}
		message = env.Error.Message
	}
	return provider.NewHTTPError(provider.OpenAI, status, code, message)
}

func mapFinishReason(r string) provider.FinishReason {
	switch r {
	case "stop":
		return provider.FinishStop
	case "length":
		return provider.FinishLength
	case "tool_calls", "function_call":
		return provider.FinishToolUse
	case "content_filter":
		return provider.FinishContentFilter
	default:
		return provider.FinishOther
	}
}
