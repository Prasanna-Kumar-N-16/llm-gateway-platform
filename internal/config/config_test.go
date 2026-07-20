package config

import (
	"testing"
	"time"
)

func TestLoadDefaults(t *testing.T) {
	t.Setenv("GATEWAY_ENV", "")
	t.Setenv("GATEWAY_HTTP_ADDR", "")
	t.Setenv("GATEWAY_LOG_LEVEL", "")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() returned error: %v", err)
	}
	if cfg.Env != "dev" {
		t.Errorf("Env = %q, want %q", cfg.Env, "dev")
	}
	if cfg.HTTPAddr != ":8080" {
		t.Errorf("HTTPAddr = %q, want %q", cfg.HTTPAddr, ":8080")
	}
	if cfg.ReadTimeout != 15*time.Second {
		t.Errorf("ReadTimeout = %v, want %v", cfg.ReadTimeout, 15*time.Second)
	}
}

func TestLoadInvalidLogLevel(t *testing.T) {
	t.Setenv("GATEWAY_LOG_LEVEL", "verbose")

	if _, err := Load(); err == nil {
		t.Fatal("Load() expected error for invalid log level, got nil")
	}
}

func TestLoadRoutes(t *testing.T) {
	t.Setenv("GATEWAY_ROUTES", `{"chat-default":[{"provider":"anthropic","model":"claude-opus-4-8"},{"provider":"openai","model":"gpt-4o"}]}`)

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() returned error: %v", err)
	}
	targets, ok := cfg.Routes["chat-default"]
	if !ok {
		t.Fatal("expected a \"chat-default\" route")
	}
	if len(targets) != 2 || targets[0].Provider != "anthropic" || targets[1].Model != "gpt-4o" {
		t.Errorf("unexpected targets: %+v", targets)
	}
}

func TestLoadRoutesInvalidJSON(t *testing.T) {
	t.Setenv("GATEWAY_ROUTES", `not json`)

	if _, err := Load(); err == nil {
		t.Fatal("Load() expected error for invalid GATEWAY_ROUTES JSON, got nil")
	}
}

func TestLoadRoutesEmptyTarget(t *testing.T) {
	t.Setenv("GATEWAY_ROUTES", `{"chat-default":[{"provider":"","model":"gpt-4o"}]}`)

	if _, err := Load(); err == nil {
		t.Fatal("Load() expected error for a route target with an empty provider, got nil")
	}
}

func TestLoadPricing(t *testing.T) {
	t.Setenv("GATEWAY_PRICING", `{"anthropic":{"claude-opus-4-8":{"input_per_1m":1,"output_per_1m":2}}}`)

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() returned error: %v", err)
	}
	entry, ok := cfg.Pricing["anthropic"]["claude-opus-4-8"]
	if !ok {
		t.Fatal("expected a pricing entry for anthropic/claude-opus-4-8")
	}
	if entry.InputPer1M != 1 || entry.OutputPer1M != 2 {
		t.Errorf("unexpected entry: %+v", entry)
	}
}

func TestLoadPricingInvalidJSON(t *testing.T) {
	t.Setenv("GATEWAY_PRICING", `not json`)

	if _, err := Load(); err == nil {
		t.Fatal("Load() expected error for invalid GATEWAY_PRICING JSON, got nil")
	}
}

func TestLoadPricingNegativeRate(t *testing.T) {
	t.Setenv("GATEWAY_PRICING", `{"anthropic":{"claude-opus-4-8":{"input_per_1m":-1,"output_per_1m":2}}}`)

	if _, err := Load(); err == nil {
		t.Fatal("Load() expected error for a negative pricing rate")
	}
}

func TestGetDuration(t *testing.T) {
	tests := []struct {
		name  string
		value string
		want  time.Duration
	}{
		{"go duration", "45s", 45 * time.Second},
		{"bare seconds", "30", 30 * time.Second},
		{"empty falls back", "", 10 * time.Second},
		{"garbage falls back", "abc", 10 * time.Second},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("TEST_DURATION", tt.value)
			if got := getDuration("TEST_DURATION", 10*time.Second); got != tt.want {
				t.Errorf("getDuration(%q) = %v, want %v", tt.value, got, tt.want)
			}
		})
	}
}
