// Package anthropic adapts the gateway's unified provider contract onto the
// Anthropic Messages API (POST /v1/messages).
package anthropic

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
	defaultBaseURL    = "https://api.anthropic.com"
	defaultAPIVersion = "2023-06-01"
	messagesPath      = "/v1/messages"
)

// Client is an Anthropic-backed provider.
type Client struct {
	apiKey     string
	baseURL    string
	apiVersion string
	http       provider.Doer
}

// Option configures a Client.
type Option func(*Client)

// WithBaseURL overrides the API base URL (useful for tests and proxies).
func WithBaseURL(u string) Option { return func(c *Client) { c.baseURL = strings.TrimRight(u, "/") } }

// WithHTTPClient injects a custom transport.
func WithHTTPClient(d provider.Doer) Option { return func(c *Client) { c.http = d } }

// WithAPIVersion overrides the anthropic-version header.
func WithAPIVersion(v string) Option { return func(c *Client) { c.apiVersion = v } }

// New constructs an Anthropic client. The API key is required.
func New(apiKey string, opts ...Option) (*Client, error) {
	if apiKey == "" {
		return nil, fmt.Errorf("anthropic: api key is required")
	}
	c := &Client{
		apiKey:     apiKey,
		baseURL:    defaultBaseURL,
		apiVersion: defaultAPIVersion,
		http:       provider.DefaultHTTPClient(),
	}
	for _, opt := range opts {
		opt(c)
	}
	return c, nil
}

// Name reports the provider identity.
func (c *Client) Name() provider.Name { return provider.Anthropic }

// wire request/response types mirror the Messages API shape.
type messagesRequest struct {
	Model     string       `json:"model"`
	MaxTokens int          `json:"max_tokens"`
	System    string       `json:"system,omitempty"`
	Messages  []wireMsg    `json:"messages"`
	Metadata  *wireMetaReq `json:"metadata,omitempty"`
}

type wireMsg struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type wireMetaReq struct {
	UserID string `json:"user_id,omitempty"`
}

type messagesResponse struct {
	Model      string        `json:"model"`
	StopReason string        `json:"stop_reason"`
	Content    []contentPart `json:"content"`
	Usage      wireUsage     `json:"usage"`
}

type contentPart struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

type wireUsage struct {
	InputTokens  int `json:"input_tokens"`
	OutputTokens int `json:"output_tokens"`
}

type errorEnvelope struct {
	Error struct {
		Type    string `json:"type"`
		Message string `json:"message"`
	} `json:"error"`
}

// Chat performs a single non-streaming Messages API call.
//
// Note: sampling parameters (temperature) are intentionally not forwarded.
// Current Claude models (Opus 4.8, Sonnet 5, etc.) reject them with a 400, so
// the gateway steers behavior through prompting rather than sampling knobs.
func (c *Client) Chat(ctx context.Context, req *provider.ChatRequest) (*provider.ChatResponse, error) {
	if req.MaxTokens <= 0 {
		return nil, fmt.Errorf("anthropic: max_tokens must be positive")
	}

	body := messagesRequest{
		Model:     req.Model,
		MaxTokens: req.MaxTokens,
		System:    req.System,
		Messages:  make([]wireMsg, 0, len(req.Messages)),
	}
	for _, m := range req.Messages {
		// The Messages API takes the system prompt as a top-level field, not a
		// message. Fold any stray system-role messages into System.
		if m.Role == provider.RoleSystem {
			body.System = joinSystem(body.System, m.Content)
			continue
		}
		body.Messages = append(body.Messages, wireMsg{Role: string(m.Role), Content: m.Content})
	}
	if uid := req.Metadata["user_id"]; uid != "" {
		body.Metadata = &wireMetaReq{UserID: uid}
	}

	raw, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("anthropic: marshal request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+messagesPath, bytes.NewReader(raw))
	if err != nil {
		return nil, fmt.Errorf("anthropic: build request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("x-api-key", c.apiKey)
	httpReq.Header.Set("anthropic-version", c.apiVersion)

	resp, err := c.http.Do(httpReq)
	if err != nil {
		return nil, provider.NewTransportError(provider.Anthropic, err)
	}
	defer resp.Body.Close()

	payload, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, provider.NewTransportError(provider.Anthropic, err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, parseError(resp.StatusCode, payload)
	}

	var out messagesResponse
	if err := json.Unmarshal(payload, &out); err != nil {
		return nil, fmt.Errorf("anthropic: decode response: %w", err)
	}

	return &provider.ChatResponse{
		Provider:     provider.Anthropic,
		Model:        out.Model,
		Content:      concatText(out.Content),
		FinishReason: mapStopReason(out.StopReason),
		Usage: provider.Usage{
			InputTokens:  out.Usage.InputTokens,
			OutputTokens: out.Usage.OutputTokens,
		},
	}, nil
}

func parseError(status int, payload []byte) error {
	var env errorEnvelope
	code, message := "", strings.TrimSpace(string(payload))
	if json.Unmarshal(payload, &env) == nil && env.Error.Message != "" {
		code = env.Error.Type
		message = env.Error.Message
	}
	return provider.NewHTTPError(provider.Anthropic, status, code, message)
}

func concatText(parts []contentPart) string {
	var b strings.Builder
	for _, p := range parts {
		if p.Type == "text" {
			b.WriteString(p.Text)
		}
	}
	return b.String()
}

func joinSystem(existing, add string) string {
	if existing == "" {
		return add
	}
	return existing + "\n\n" + add
}

func mapStopReason(r string) provider.FinishReason {
	switch r {
	case "end_turn", "stop_sequence":
		return provider.FinishStop
	case "max_tokens":
		return provider.FinishLength
	case "tool_use":
		return provider.FinishToolUse
	case "refusal":
		return provider.FinishContentFilter
	default:
		return provider.FinishOther
	}
}
