// Package pricing attributes a USD cost to a completed chat request based on
// its provider, model, and token usage, so spend can be tracked per team,
// model, and provider (see README "FinOps").
package pricing

import (
	"fmt"

	"github.com/Prasanna-Kumar-N-16/llm-gateway-platform/pkg/provider"
)

// ModelPrice is the USD rate charged per 1M input/output tokens.
type ModelPrice struct {
	InputPer1M  float64
	OutputPer1M float64
}

// Table maps a provider to its per-model pricing.
type Table map[provider.Name]map[string]ModelPrice

// Cost is the USD spend attributed to a single chat request.
type Cost struct {
	InputUSD  float64 `json:"input_usd"`
	OutputUSD float64 `json:"output_usd"`
	TotalUSD  float64 `json:"total_usd"`
	// Known is false when no price is on file for the provider/model, in
	// which case every USD field is zero rather than a silent underestimate.
	Known bool `json:"known"`
}

// DefaultTable returns the built-in price list for well-known models, in USD
// per 1M tokens. Ratesheet as of 2026; override or extend via GATEWAY_PRICING.
func DefaultTable() Table {
	return Table{
		provider.Anthropic: {
			"claude-opus-4-8":  {InputPer1M: 15, OutputPer1M: 75},
			"claude-sonnet-5":  {InputPer1M: 3, OutputPer1M: 15},
			"claude-haiku-4-5": {InputPer1M: 0.8, OutputPer1M: 4},
		},
		provider.OpenAI: {
			"gpt-4o":      {InputPer1M: 2.5, OutputPer1M: 10},
			"gpt-4o-mini": {InputPer1M: 0.15, OutputPer1M: 0.6},
		},
		provider.Bedrock: {
			"anthropic.claude-opus-4-8": {InputPer1M: 15, OutputPer1M: 75},
			"anthropic.claude-sonnet-5": {InputPer1M: 3, OutputPer1M: 15},
		},
	}
}

// Calculator computes request cost from a price Table.
type Calculator struct {
	table Table
}

// New constructs a Calculator from table. A nil or empty table makes every
// Cost lookup report Known: false.
func New(table Table) *Calculator {
	return &Calculator{table: table}
}

// Cost attributes a USD cost to usage for the given provider/model. It
// returns Cost{Known: false} rather than an error when no price is on file,
// since a missing rate should never fail the request that earned it.
func (c *Calculator) Cost(name provider.Name, model string, usage provider.Usage) Cost {
	if c == nil {
		return Cost{}
	}
	byModel, ok := c.table[name]
	if !ok {
		return Cost{}
	}
	price, ok := byModel[model]
	if !ok {
		return Cost{}
	}
	in := float64(usage.InputTokens) / 1_000_000 * price.InputPer1M
	out := float64(usage.OutputTokens) / 1_000_000 * price.OutputPer1M
	return Cost{InputUSD: in, OutputUSD: out, TotalUSD: in + out, Known: true}
}

// Merge overlays override onto the receiver, returning a new Table. A price
// for a given provider+model in override replaces the base entry entirely.
func (t Table) Merge(override Table) Table {
	merged := make(Table, len(t))
	for name, models := range t {
		merged[name] = make(map[string]ModelPrice, len(models))
		for model, price := range models {
			merged[name][model] = price
		}
	}
	for name, models := range override {
		if merged[name] == nil {
			merged[name] = make(map[string]ModelPrice, len(models))
		}
		for model, price := range models {
			merged[name][model] = price
		}
	}
	return merged
}

// Validate reports an error if any entry has a negative rate.
func (t Table) Validate() error {
	for name, models := range t {
		for model, price := range models {
			if price.InputPer1M < 0 || price.OutputPer1M < 0 {
				return fmt.Errorf("pricing: %s/%s has a negative rate", name, model)
			}
		}
	}
	return nil
}
