// Package provider defines the vendor-agnostic contract the gateway uses to
// talk to large language model backends, along with the shared request and
// response types every adapter maps to and from.
package provider

import "context"

// Name identifies a backend provider.
type Name string

const (
	Anthropic Name = "anthropic"
	OpenAI    Name = "openai"
	Bedrock   Name = "bedrock"
)

// Role is the author of a message in a conversation.
type Role string

const (
	RoleSystem    Role = "system"
	RoleUser      Role = "user"
	RoleAssistant Role = "assistant"
)

// FinishReason is the normalized reason generation stopped. Provider-specific
// values are mapped onto this closed set by each adapter.
type FinishReason string

const (
	FinishStop          FinishReason = "stop"           // natural end of turn
	FinishLength        FinishReason = "length"         // hit the max token cap
	FinishToolUse       FinishReason = "tool_use"       // model requested a tool
	FinishContentFilter FinishReason = "content_filter" // refused or filtered
	FinishOther         FinishReason = "other"
)

// Message is a single turn in a conversation.
type Message struct {
	Role    Role   `json:"role"`
	Content string `json:"content"`
}

// ChatRequest is the unified inference request accepted by every provider.
// Adapters translate it into the provider's native wire format.
type ChatRequest struct {
	// Model is the provider-native model identifier (e.g. "claude-opus-4-8").
	Model string `json:"model"`
	// System is an optional system prompt applied ahead of Messages.
	System string `json:"system,omitempty"`
	// Messages is the ordered conversation, excluding the system prompt.
	Messages []Message `json:"messages"`
	// MaxTokens bounds the generated output. Required by some providers.
	MaxTokens int `json:"max_tokens"`
	// Temperature, when non-nil, requests a sampling temperature. Adapters may
	// ignore it for models that reject sampling parameters.
	Temperature *float64 `json:"temperature,omitempty"`
	// Metadata carries caller attribution (team, feature, request id) used for
	// cost accounting and tracing. It is never sent to the provider.
	Metadata map[string]string `json:"-"`
}

// Usage reports token consumption for a single request.
type Usage struct {
	InputTokens  int `json:"input_tokens"`
	OutputTokens int `json:"output_tokens"`
}

// TotalTokens returns the combined input and output token count.
func (u Usage) TotalTokens() int { return u.InputTokens + u.OutputTokens }

// ChatResponse is the unified inference response returned by every provider.
type ChatResponse struct {
	// Provider is the backend that served the request.
	Provider Name `json:"provider"`
	// Model echoes the model that produced the response.
	Model string `json:"model"`
	// Content is the assistant's text output.
	Content string `json:"content"`
	// FinishReason is the normalized stop reason.
	FinishReason FinishReason `json:"finish_reason"`
	// Usage reports token consumption.
	Usage Usage `json:"usage"`
}

// Provider is the contract every backend adapter implements. Implementations
// must be safe for concurrent use.
type Provider interface {
	// Name reports the provider identity.
	Name() Name
	// Chat performs a single, non-streaming inference request. It must honor
	// ctx cancellation and return a *Error for any provider-side failure.
	Chat(ctx context.Context, req *ChatRequest) (*ChatResponse, error)
}
