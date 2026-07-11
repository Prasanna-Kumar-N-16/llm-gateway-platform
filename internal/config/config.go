package config

import (
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
}

// Load reads configuration from the environment, applying defaults for any
// value that is not set. It returns an error only when a provided value is
// malformed.
func Load() (*Config, error) {
	cfg := &Config{
		Env:             getString("GATEWAY_ENV", "dev"),
		HTTPAddr:        getString("GATEWAY_HTTP_ADDR", ":8080"),
		ReadTimeout:     getDuration("GATEWAY_READ_TIMEOUT", 15*time.Second),
		WriteTimeout:    getDuration("GATEWAY_WRITE_TIMEOUT", 30*time.Second),
		ShutdownTimeout: getDuration("GATEWAY_SHUTDOWN_TIMEOUT", 20*time.Second),
		LogLevel:        getString("GATEWAY_LOG_LEVEL", "info"),
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
	return nil
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
