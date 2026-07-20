package config

import (
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"time"
)

// Config holds runtime configuration for the gateway. Values are sourced from
// the environment so the same binary can run unchanged across environments.
type Config struct {
	// Env identifies the deployment environment (e.g. "dev", "staging", "prod").
	Env string
	// HTTPAddr is the address the HTTP server binds to.
	HTTPAddr string
	// ReadTimeout bounds the time spent reading a request, including its body.
	ReadTimeout time.Duration
	// WriteTimeout bounds the time spent writing a response.
	WriteTimeout time.Duration
	// ShutdownTimeout bounds graceful shutdown before connections are forced closed.
	ShutdownTimeout time.Duration
	// LogLevel controls structured log verbosity ("debug", "info", "warn", "error").
	LogLevel string

	// AnthropicAPIKey enables the Anthropic provider when non-empty.
	AnthropicAPIKey string
	// OpenAIAPIKey enables the OpenAI provider when non-empty.
	OpenAIAPIKey string
	// AWSRegion, AWSAccessKeyID, and AWSSecretAccessKey enable the Bedrock
	// provider when all three are non-empty. AWSSessionToken is optional and
	// only required for temporary (STS) credentials.
	AWSRegion          string
	AWSAccessKeyID     string
	AWSSecretAccessKey string
	AWSSessionToken    string

	// Routes maps a logical model name to an ordered fallback chain of
	// provider targets, decoded from the GATEWAY_ROUTES JSON env var.
	Routes map[string][]RouteTarget

	// Pricing overrides or extends the built-in per-model USD rates used for
	// cost attribution, decoded from the GATEWAY_PRICING JSON env var.
	Pricing map[string]map[string]PriceEntry
}

// RouteTarget names one hop (provider + provider-native model id) in a
// fallback chain for a logical model.
type RouteTarget struct {
	Provider string `json:"provider"`
	Model    string `json:"model"`
}

// PriceEntry is the USD rate charged per 1M input/output tokens for one
// provider/model pair.
type PriceEntry struct {
	InputPer1M  float64 `json:"input_per_1m"`
	OutputPer1M float64 `json:"output_per_1m"`
}

// Load reads configuration from the environment, applying defaults for any
// value that is not set. It returns an error only when a provided value is
// malformed.
func Load() (*Config, error) {
	routes, err := getRoutes("GATEWAY_ROUTES")
	if err != nil {
		return nil, err
	}

	pricing, err := getPricing("GATEWAY_PRICING")
	if err != nil {
		return nil, err
	}

	cfg := &Config{
		Env:             getString("GATEWAY_ENV", "dev"),
		HTTPAddr:        getString("GATEWAY_HTTP_ADDR", ":8080"),
		ReadTimeout:     getDuration("GATEWAY_READ_TIMEOUT", 15*time.Second),
		WriteTimeout:    getDuration("GATEWAY_WRITE_TIMEOUT", 30*time.Second),
		ShutdownTimeout: getDuration("GATEWAY_SHUTDOWN_TIMEOUT", 20*time.Second),
		LogLevel:        getString("GATEWAY_LOG_LEVEL", "info"),

		AnthropicAPIKey:    getString("ANTHROPIC_API_KEY", ""),
		OpenAIAPIKey:       getString("OPENAI_API_KEY", ""),
		AWSRegion:          getString("AWS_REGION", ""),
		AWSAccessKeyID:     getString("AWS_ACCESS_KEY_ID", ""),
		AWSSecretAccessKey: getString("AWS_SECRET_ACCESS_KEY", ""),
		AWSSessionToken:    getString("AWS_SESSION_TOKEN", ""),

		Routes:  routes,
		Pricing: pricing,
	}

	if err := cfg.validate(); err != nil {
		return nil, err
	}
	return cfg, nil
}

func (c *Config) validate() error {
	if c.HTTPAddr == "" {
		return fmt.Errorf("config: GATEWAY_HTTP_ADDR must not be empty")
	}
	switch c.LogLevel {
	case "debug", "info", "warn", "error":
	default:
		return fmt.Errorf("config: invalid GATEWAY_LOG_LEVEL %q", c.LogLevel)
	}
	for name, targets := range c.Routes {
		if len(targets) == 0 {
			return fmt.Errorf("config: GATEWAY_ROUTES entry %q has no targets", name)
		}
		for _, t := range targets {
			if t.Provider == "" || t.Model == "" {
				return fmt.Errorf("config: GATEWAY_ROUTES entry %q has a target with an empty provider or model", name)
			}
		}
	}
	for providerName, models := range c.Pricing {
		for model, price := range models {
			if price.InputPer1M < 0 || price.OutputPer1M < 0 {
				return fmt.Errorf("config: GATEWAY_PRICING entry %s/%s has a negative rate", providerName, model)
			}
		}
	}
	return nil
}

// getRoutes decodes a JSON object mapping logical model names to fallback
// chains, e.g. {"chat-default":[{"provider":"anthropic","model":"claude-opus-4-8"}]}.
// An unset or empty value yields no routes.
func getRoutes(key string) (map[string][]RouteTarget, error) {
	v, ok := os.LookupEnv(key)
	if !ok || v == "" {
		return nil, nil
	}
	var routes map[string][]RouteTarget
	if err := json.Unmarshal([]byte(v), &routes); err != nil {
		return nil, fmt.Errorf("config: invalid %s: %w", key, err)
	}
	return routes, nil
}

// getPricing decodes a JSON object mapping provider name to a map of model to
// per-1M-token USD rate, e.g.
// {"anthropic":{"claude-opus-4-8":{"input_per_1m":15,"output_per_1m":75}}}.
// An unset or empty value yields no overrides.
func getPricing(key string) (map[string]map[string]PriceEntry, error) {
	v, ok := os.LookupEnv(key)
	if !ok || v == "" {
		return nil, nil
	}
	var pricing map[string]map[string]PriceEntry
	if err := json.Unmarshal([]byte(v), &pricing); err != nil {
		return nil, fmt.Errorf("config: invalid %s: %w", key, err)
	}
	return pricing, nil
}

func getString(key, fallback string) string {
	if v, ok := os.LookupEnv(key); ok && v != "" {
		return v
	}
	return fallback
}

func getDuration(key string, fallback time.Duration) time.Duration {
	v, ok := os.LookupEnv(key)
	if !ok || v == "" {
		return fallback
	}
	// Accept both Go duration strings ("30s") and bare integer seconds ("30").
	if d, err := time.ParseDuration(v); err == nil {
		return d
	}
	if secs, err := strconv.Atoi(v); err == nil {
		return time.Duration(secs) * time.Second
	}
	return fallback
}
