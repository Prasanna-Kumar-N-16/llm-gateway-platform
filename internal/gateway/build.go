// Package gateway wires configuration into a ready-to-use router: it
// constructs a provider client for each configured backend, registers it, and
// installs the configured routing table.
package gateway

import (
	"fmt"
	"log/slog"

	"github.com/Prasanna-Kumar-N-16/llm-gateway-platform/internal/config"
	"github.com/Prasanna-Kumar-N-16/llm-gateway-platform/internal/router"
	"github.com/Prasanna-Kumar-N-16/llm-gateway-platform/pkg/provider"
	"github.com/Prasanna-Kumar-N-16/llm-gateway-platform/pkg/provider/anthropic"
	"github.com/Prasanna-Kumar-N-16/llm-gateway-platform/pkg/provider/bedrock"
	"github.com/Prasanna-Kumar-N-16/llm-gateway-platform/pkg/provider/openai"
)

// Build constructs a Router from cfg: it registers a provider client for each
// backend with credentials configured, then installs cfg.Routes. It returns
// an error if no provider is configured, or if a route references a provider
// that was not registered.
func Build(cfg *config.Config, log *slog.Logger) (*router.Router, error) {
	r := router.New(router.WithLogger(log))

	registered := make(map[provider.Name]bool)

	if cfg.AnthropicAPIKey != "" {
		c, err := anthropic.New(cfg.AnthropicAPIKey)
		if err != nil {
			return nil, fmt.Errorf("gateway: anthropic: %w", err)
		}
		r.Register(c)
		registered[provider.Anthropic] = true
	}

	if cfg.OpenAIAPIKey != "" {
		c, err := openai.New(cfg.OpenAIAPIKey)
		if err != nil {
			return nil, fmt.Errorf("gateway: openai: %w", err)
		}
		r.Register(c)
		registered[provider.OpenAI] = true
	}

	if cfg.AWSRegion != "" && cfg.AWSAccessKeyID != "" && cfg.AWSSecretAccessKey != "" {
		signer, err := bedrock.NewSigV4Signer(bedrock.Credentials{
			AccessKeyID:     cfg.AWSAccessKeyID,
			SecretAccessKey: cfg.AWSSecretAccessKey,
			SessionToken:    cfg.AWSSessionToken,
		})
		if err != nil {
			return nil, fmt.Errorf("gateway: bedrock: %w", err)
		}
		c, err := bedrock.New(cfg.AWSRegion, signer)
		if err != nil {
			return nil, fmt.Errorf("gateway: bedrock: %w", err)
		}
		r.Register(c)
		registered[provider.Bedrock] = true
	}

	if len(registered) == 0 {
		return nil, fmt.Errorf("gateway: no provider configured; set ANTHROPIC_API_KEY, OPENAI_API_KEY, or AWS credentials")
	}

	for name, targets := range cfg.Routes {
		route := router.Route{Targets: make([]router.Target, len(targets))}
		for i, t := range targets {
			p := provider.Name(t.Provider)
			if !registered[p] {
				return nil, fmt.Errorf("gateway: route %q references unregistered provider %q", name, t.Provider)
			}
			route.Targets[i] = router.Target{Provider: p, Model: t.Model}
		}
		r.AddRoute(name, route)
	}

	return r, nil
}
