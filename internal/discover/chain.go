package discover

import (
	"context"
	"log"
)

// ChainResolver tries resolver stages in order, stopping at the first match.
// Soft errors (API timeouts, rate limits) are logged and the next stage is tried.
// Hard errors (context cancellation, budget exhaustion) stop the chain.
type ChainResolver struct {
	stages []Resolver
}

// NewChainResolver creates a resolver that tries stages in order.
func NewChainResolver(stages ...Resolver) *ChainResolver {
	return &ChainResolver{stages: stages}
}

// Resolve tries each stage in order. Returns the first non-nil result.
// Returns NotFoundError if all stages miss.
func (c *ChainResolver) Resolve(ctx context.Context, toolName string) (*DiscoveryResult, error) {
	normalized, err := NormalizeName(toolName)
	if err != nil {
		return nil, err
	}

	for _, stage := range c.stages {
		result, err := stage.Resolve(ctx, normalized)
		if err != nil {
			if ctx.Err() != nil || isFatalError(err) {
				return nil, err
			}
			// Soft error: log and try the next stage.
			log.Printf("discover: stage error for %q: %v", normalized, err)
			continue
		}
		if result != nil {
			return result, nil
		}
		// nil result, nil error: soft miss, try next stage.
	}
	return nil, &NotFoundError{Tool: toolName}
}
