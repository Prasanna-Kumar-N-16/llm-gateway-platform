package gateway

import (
	"io"
	"log/slog"
	"testing"

	"github.com/Prasanna-Kumar-N-16/llm-gateway-platform/internal/config"
)

func testLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func TestBuildNoProvidersConfigured(t *testing.T) {
	_, err := Build(&config.Config{}, testLogger())
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

	r, err := Build(cfg, testLogger())
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if r == nil {
		t.Fatal("expected a non-nil router")
	}
}

func TestBuildRouteReferencesUnregisteredProvider(t *testing.T) {
	cfg := &config.Config{
		AnthropicAPIKey: "test-key",
		Routes: map[string][]config.RouteTarget{
			"chat-default": {{Provider: "openai", Model: "gpt-4o"}},
		},
	}

	_, err := Build(cfg, testLogger())
	if err == nil {
		t.Fatal("expected an error when a route references an unregistered provider")
	}
}
