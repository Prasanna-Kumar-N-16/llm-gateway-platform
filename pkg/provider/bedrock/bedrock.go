// Package bedrock adapts the gateway's unified provider contract onto Amazon
// Bedrock's InvokeModel endpoint for Anthropic Claude models. Requests are
// signed with AWS Signature V4; the signer is injectable so the request and
// response mapping can be unit-tested without live AWS credentials.
package bedrock

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/Prasanna-Kumar-N-16/llm-gateway-platform/pkg/provider"
)

const (
	bedrockAnthropicVersion = "bedrock-2023-05-31"
	serviceName             = "bedrock"
)

// Signer signs an outbound HTTP request in place. The default implementation is
// AWS Signature V4; tests inject a no-op signer.
type Signer interface {
	Sign(req *http.Request, payload []byte, service, region string, t time.Time) error
}

// Client is an Amazon Bedrock-backed provider serving Anthropic Claude models.
type Client struct {
	region  string
	baseURL string
	signer  Signer
	http    provider.Doer
	now     func() time.Time
}

// Option configures a Client.
type Option func(*Client)

// WithBaseURL overrides the runtime endpoint (useful for tests and VPC endpoints).
func WithBaseURL(u string) Option { return func(c *Client) { c.baseURL = strings.TrimRight(u, "/") } }

// WithHTTPClient injects a custom transport.
func WithHTTPClient(d provider.Doer) Option { return func(c *Client) { c.http = d } }

// WithSigner overrides the request signer.
func WithSigner(s Signer) Option { return func(c *Client) { c.signer = s } }

// WithClock overrides the time source (used for deterministic signing in tests).
func WithClock(now func() time.Time) Option { return func(c *Client) { c.now = now } }

// New constructs a Bedrock client for the given region and signer. The signer
// carries the AWS credentials; see SigV4Signer for the default.
func New(region string, signer Signer, opts ...Option) (*Client, error) {
	if region == "" {
		return nil, fmt.Errorf("bedrock: region is required")
	}
	if signer == nil {
		return nil, fmt.Errorf("bedrock: signer is required")
	}
	c := &Client{
		region:  region,
		baseURL: fmt.Sprintf("https://bedrock-runtime.%s.amazonaws.com", region),
		signer:  signer,
		http:    provider.DefaultHTTPClient(),
		now:     time.Now,
	}
	for _, opt := range opts {
		opt(c)
	}
	return c, nil
}

// Name reports the provider identity.
func (c *Client) Name() provider.Name { return provider.Bedrock }

type invokeRequest struct {
	AnthropicVersion string    `json:"anthropic_version"`
	MaxTokens        int       `json:"max_tokens"`
	System           string    `json:"system,omitempty"`
	Messages         []wireMsg `json:"messages"`
}

type wireMsg struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type invokeResponse struct {
	StopReason string `json:"stop_reason"`
	Content    []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	} `json:"content"`
	Usage struct {
		InputTokens  int `json:"input_tokens"`
		OutputTokens int `json:"output_tokens"`
	} `json:"usage"`
}

type errorEnvelope struct {
	Message string `json:"message"`
}

// Chat invokes a Bedrock-hosted Claude model. req.Model is the Bedrock model
// identifier (e.g. "anthropic.claude-opus-4-8" or a cross-region inference
// profile ARN).
func (c *Client) Chat(ctx context.Context, req *provider.ChatRequest) (*provider.ChatResponse, error) {
	if req.MaxTokens <= 0 {
		return nil, fmt.Errorf("bedrock: max_tokens must be positive")
	}

	body := invokeRequest{
		AnthropicVersion: bedrockAnthropicVersion,
		MaxTokens:        req.MaxTokens,
		System:           req.System,
		Messages:         make([]wireMsg, 0, len(req.Messages)),
	}
	for _, m := range req.Messages {
		if m.Role == provider.RoleSystem {
			body.System = joinSystem(body.System, m.Content)
			continue
		}
		body.Messages = append(body.Messages, wireMsg{Role: string(m.Role), Content: m.Content})
	}

	raw, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("bedrock: marshal request: %w", err)
	}

	endpoint := fmt.Sprintf("%s/model/%s/invoke", c.baseURL, url.PathEscape(req.Model))
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(raw))
	if err != nil {
		return nil, fmt.Errorf("bedrock: build request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Accept", "application/json")

	if err := c.signer.Sign(httpReq, raw, serviceName, c.region, c.now().UTC()); err != nil {
		return nil, fmt.Errorf("bedrock: sign request: %w", err)
	}

	resp, err := c.http.Do(httpReq)
	if err != nil {
		return nil, provider.NewTransportError(provider.Bedrock, err)
	}
	defer resp.Body.Close()

	payload, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, provider.NewTransportError(provider.Bedrock, err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, parseError(resp.StatusCode, payload)
	}

	var out invokeResponse
	if err := json.Unmarshal(payload, &out); err != nil {
		return nil, fmt.Errorf("bedrock: decode response: %w", err)
	}

	return &provider.ChatResponse{
		Provider:     provider.Bedrock,
		Model:        req.Model,
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
	message := strings.TrimSpace(string(payload))
	if json.Unmarshal(payload, &env) == nil && env.Message != "" {
		message = env.Message
	}
	return provider.NewHTTPError(provider.Bedrock, status, "", message)
}

func concatText(parts []struct {
	Type string `json:"type"`
	Text string `json:"text"`
}) string {
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
