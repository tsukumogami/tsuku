package discover

import (
	"context"
	"fmt"
	"time"

	"github.com/tsukumogami/tsuku/internal/log"
	"github.com/tsukumogami/tsuku/internal/telemetry"
)

// ChainResolver tries resolver stages in order, stopping at the first match.
// Soft errors (API timeouts, rate limits) are logged and the next stage is tried.
// Hard errors (context cancellation, budget exhaustion) stop the chain.
type ChainResolver struct {
	stages    []Resolver
	telemetry *telemetry.Client
}

// NewChainResolver creates a resolver that tries stages in order.
func NewChainResolver(stages ...Resolver) *ChainResolver {
	return &ChainResolver{stages: stages}
}

// WithTelemetry sets the telemetry client for discovery event emission.
func (c *ChainResolver) WithTelemetry(tc *telemetry.Client) *ChainResolver {
	c.telemetry = tc
	return c
}

// Resolve tries each stage in order. Returns the first non-nil result.
// Returns NotFoundError if all stages miss.
func (c *ChainResolver) Resolve(ctx context.Context, toolName string) (*DiscoveryResult, error) {
	normalized, err := NormalizeName(toolName)
	if err != nil {
		return nil, err
	}

	start := time.Now()

	for _, stage := range c.stages {
		result, err := stage.Resolve(ctx, normalized)
		if err != nil {
			if ctx.Err() != nil || isFatalError(err) {
				c.emitErrorEvent(normalized, "internal", start)
				return nil, err
			}
			// Soft error: log and try the next stage.
			log.Default().Warn(fmt.Sprintf("discover: stage error for %q: %v", normalized, err))
			continue
		}
		if result != nil {
			c.emitHitEvent(normalized, result, start)
			return result, nil
		}
		// nil result, nil error: soft miss, try next stage.
	}
	c.emitNotFoundEvent(normalized, start)
	return nil, &NotFoundError{Tool: toolName}
}

// emitHitEvent sends a telemetry event for a successful discovery.
func (c *ChainResolver) emitHitEvent(toolName string, result *DiscoveryResult, start time.Time) {
	if c.telemetry == nil {
		return
	}
	durationMs := time.Since(start).Milliseconds()
	var event telemetry.DiscoveryEvent
	switch result.Confidence {
	case ConfidenceRegistry:
		event = telemetry.NewDiscoveryRegistryHitEvent(toolName, result.Builder, result.Source, durationMs)
	case ConfidenceEcosystem:
		event = telemetry.NewDiscoveryEcosystemHitEvent(toolName, result.Builder, result.Source, durationMs)
	case ConfidenceLLM:
		event = telemetry.NewDiscoveryLLMHitEvent(toolName, result.Builder, result.Source, durationMs)
	default:
		return
	}
	c.telemetry.SendDiscovery(event)
}

// emitNotFoundEvent sends a telemetry event when no stage found a match.
func (c *ChainResolver) emitNotFoundEvent(toolName string, start time.Time) {
	if c.telemetry == nil {
		return
	}
	c.telemetry.SendDiscovery(telemetry.NewDiscoveryNotFoundEvent(toolName, time.Since(start).Milliseconds()))
}

// emitErrorEvent sends a telemetry event when a fatal error occurred.
func (c *ChainResolver) emitErrorEvent(toolName, errorCategory string, start time.Time) {
	if c.telemetry == nil {
		return
	}
	c.telemetry.SendDiscovery(telemetry.NewDiscoveryErrorEvent(toolName, errorCategory, time.Since(start).Milliseconds()))
}
