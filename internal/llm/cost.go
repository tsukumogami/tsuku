// Package llm provides a client for interacting with Claude for recipe generation.
package llm

import "fmt"

// Usage tracks token consumption across LLM API calls.
type Usage struct {
	InputTokens  int
	OutputTokens int
}

// Pricing constants for Claude Sonnet 4 (per 1M tokens in USD).
const (
	inputPricePerMillion  = 3.0  // $3 per 1M input tokens
	outputPricePerMillion = 15.0 // $15 per 1M output tokens
)

// Add accumulates usage from another Usage into this one.
func (u *Usage) Add(other Usage) {
	u.InputTokens += other.InputTokens
	u.OutputTokens += other.OutputTokens
}

// Cost returns the estimated cost in USD based on Claude Sonnet 4 pricing.
func (u Usage) Cost() float64 {
	inputCost := float64(u.InputTokens) * inputPricePerMillion / 1_000_000
	outputCost := float64(u.OutputTokens) * outputPricePerMillion / 1_000_000
	return inputCost + outputCost
}

// String returns a human-readable summary of token usage and cost.
func (u Usage) String() string {
	return fmt.Sprintf("tokens: %d in / %d out, cost: $%.4f",
		u.InputTokens, u.OutputTokens, u.Cost())
}
