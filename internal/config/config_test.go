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
