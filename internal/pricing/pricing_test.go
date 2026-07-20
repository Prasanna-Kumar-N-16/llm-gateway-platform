package pricing

import (
	"testing"

	"github.com/Prasanna-Kumar-N-16/llm-gateway-platform/pkg/provider"
)

func TestCostKnownModel(t *testing.T) {
	c := New(DefaultTable())
	got := c.Cost(provider.Anthropic, "claude-opus-4-8", provider.Usage{InputTokens: 1_000_000, OutputTokens: 1_000_000})
	if !got.Known {
		t.Fatal("expected a known price for claude-opus-4-8")
	}
	if got.InputUSD != 15 || got.OutputUSD != 75 || got.TotalUSD != 90 {
		t.Errorf("got %+v, want InputUSD=15 OutputUSD=75 TotalUSD=90", got)
	}
}

func TestCostUnknownModel(t *testing.T) {
	c := New(DefaultTable())
	got := c.Cost(provider.Anthropic, "not-a-real-model", provider.Usage{InputTokens: 1000, OutputTokens: 1000})
	if got.Known {
		t.Fatal("expected Known=false for an unpriced model")
	}
	if got.TotalUSD != 0 {
		t.Errorf("TotalUSD = %v, want 0 for an unknown price", got.TotalUSD)
	}
}

func TestCostNilCalculator(t *testing.T) {
	var c *Calculator
	got := c.Cost(provider.Anthropic, "claude-opus-4-8", provider.Usage{InputTokens: 1000, OutputTokens: 1000})
	if got.Known {
		t.Fatal("expected Known=false for a nil calculator")
	}
}

func TestMergeOverridesExistingModel(t *testing.T) {
	base := Table{provider.Anthropic: {"claude-opus-4-8": {InputPer1M: 15, OutputPer1M: 75}}}
	override := Table{provider.Anthropic: {"claude-opus-4-8": {InputPer1M: 1, OutputPer1M: 2}}}

	merged := base.Merge(override)
	got := merged[provider.Anthropic]["claude-opus-4-8"]
	if got.InputPer1M != 1 || got.OutputPer1M != 2 {
		t.Errorf("got %+v, want the override price", got)
	}
	// base must be unmodified.
	if base[provider.Anthropic]["claude-opus-4-8"].InputPer1M != 15 {
		t.Error("Merge mutated the base table")
	}
}

func TestMergeAddsNewProvider(t *testing.T) {
	base := Table{}
	override := Table{provider.OpenAI: {"gpt-4o": {InputPer1M: 2.5, OutputPer1M: 10}}}

	merged := base.Merge(override)
	if merged[provider.OpenAI]["gpt-4o"].InputPer1M != 2.5 {
		t.Errorf("expected the new provider/model to be present after merge")
	}
}

func TestValidateRejectsNegativeRate(t *testing.T) {
	table := Table{provider.Anthropic: {"claude-opus-4-8": {InputPer1M: -1, OutputPer1M: 75}}}
	if err := table.Validate(); err == nil {
		t.Fatal("expected an error for a negative rate")
	}
}

func TestValidateAcceptsDefaultTable(t *testing.T) {
	if err := DefaultTable().Validate(); err != nil {
		t.Errorf("DefaultTable() should be valid: %v", err)
	}
}
