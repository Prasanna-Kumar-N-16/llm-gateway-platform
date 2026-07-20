package gateway

import (
	"io"
	"log/slog"
	"testing"

	"github.com/Prasanna-Kumar-N-16/llm-gateway-platform/internal/config"
	"github.com/Prasanna-Kumar-N-16/llm-gateway-platform/pkg/provider"
)

func testLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func TestBuildNoProvidersConfigured(t *testing.T) {
	_, _, err := Build(&config.Config{}, testLogger())
	if err == nil {
		t.Fatal("expected an error when no provider is configured")
	}
}

func TestBuildRegistersConfiguredProviders(t *testing.T) {
	cfg := &config.Config{
		AnthropicAPIKey: "test-key",
		Routes: map[string][]config.RouteTarget{
			"chat-default": {{Provider: "anthropic", Model: "claude-opus-4-8"}},
		},
	}

	r, pricer, err := Build(cfg, testLogger())
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if r == nil {
		t.Fatal("expected a non-nil router")
	}
	if pricer == nil {
		t.Fatal("expected a non-nil pricing calculator")
	}
}

func TestBuildRouteReferencesUnregisteredProvider(t *testing.T) {
	cfg := &config.Config{
		AnthropicAPIKey: "test-key",
		Routes: map[string][]config.RouteTarget{
			"chat-default": {{Provider: "openai", Model: "gpt-4o"}},
		},
	}

	_, _, err := Build(cfg, testLogger())
	if err == nil {
		t.Fatal("expected an error when a route references an unregistered provider")
	}
}

func TestBuildAppliesPricingOverride(t *testing.T) {
	cfg := &config.Config{
		AnthropicAPIKey: "test-key",
		Pricing: map[string]map[string]config.PriceEntry{
			"anthropic": {"claude-opus-4-8": {InputPer1M: 1, OutputPer1M: 2}},
		},
	}

	_, pricer, err := Build(cfg, testLogger())
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	got := pricer.Cost(provider.Anthropic, "claude-opus-4-8", provider.Usage{InputTokens: 1_000_000, OutputTokens: 1_000_000})
	if !got.Known || got.TotalUSD != 3 {
		t.Errorf("got %+v, want the overridden rate (TotalUSD=3)", got)
	}
}

func TestBuildRejectsNegativePricingOverride(t *testing.T) {
	cfg := &config.Config{
		AnthropicAPIKey: "test-key",
		Pricing: map[string]map[string]config.PriceEntry{
			"anthropic": {"claude-opus-4-8": {InputPer1M: -1, OutputPer1M: 2}},
		},
	}

	_, _, err := Build(cfg, testLogger())
	if err == nil {
		t.Fatal("expected an error for a negative pricing override")
	}
}
