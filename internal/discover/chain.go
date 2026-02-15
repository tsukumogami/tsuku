package discover

import (
	"context"
	"fmt"
	"time"

	"github.com/tsukumogami/tsuku/internal/log"
	"github.com/tsukumogami/tsuku/internal/telemetry"
)

// LLMAvailability represents whether LLM discovery can be attempted.
type LLMAvailability struct {
	DeterministicOnly bool // --deterministic-only flag was set
	HasAPIKey         bool // ANTHROPIC_API_KEY is configured
}

// ChainResolver tries resolver stages in order, stopping at the first match.
// Soft errors (API timeouts, rate limits) are logged and the next stage is tried.
// Hard errors (context cancellation, budget exhaustion) stop the chain.
type ChainResolver struct {
	stages          []Resolver
	telemetry       *telemetry.Client
	logger          log.Logger
	llmAvailability LLMAvailability
	registry        *DiscoveryRegistry
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

// WithLogger sets the logger for verbose output.
func (c *ChainResolver) WithLogger(l log.Logger) *ChainResolver {
	c.logger = l
	return c
}

// WithLLMAvailability sets the LLM availability state for error message selection.
func (c *ChainResolver) WithLLMAvailability(avail LLMAvailability) *ChainResolver {
	c.llmAvailability = avail
	return c
}

// WithRegistry sets the discovery registry for typosquatting detection.
func (c *ChainResolver) WithRegistry(reg *DiscoveryRegistry) *ChainResolver {
	c.registry = reg
	return c
}

// Resolve tries each stage in order. Returns the first non-nil result.
// Returns NotFoundError if all stages miss, or ConfigurationError if
// LLM discovery was unavailable due to configuration.
func (c *ChainResolver) Resolve(ctx context.Context, toolName string) (*DiscoveryResult, error) {
	normalized, err := NormalizeName(toolName)
	if err != nil {
		return nil, err
	}

	// Check for typosquatting before probing stages
	if warning := CheckTyposquat(normalized, c.registry); warning != nil {
		log.Default().Warn(fmt.Sprintf(
			"Typosquat warning: %q is similar to %q (distance %d, source: %s)",
			warning.Requested, warning.Similar, warning.Distance, warning.Source,
		))
	}

	start := time.Now()
	c.logInfo("Checking discovery registry for '%s'...", normalized)

	stageCount := len(c.stages)
	for i, stage := range c.stages {
		result, err := stage.Resolve(ctx, normalized)
		if err != nil {
			// Handle budget exceeded as a fatal error
			if err == ErrBudgetExceeded {
				c.emitBudgetExceededEvent(normalized, start)
				return nil, err
			}
			if ctx.Err() != nil || isFatalError(err) {
				c.emitErrorEvent(normalized, "internal", start)
				return nil, err
			}
			// Soft error: log and try the next stage.
			log.Default().Warn(fmt.Sprintf("discover: stage error for %q: %v", normalized, err))
			continue
		}
		if result != nil {
			c.logStageHit(result)
			c.emitHitEvent(normalized, result, start)
			return result, nil
		}
		// nil result, nil error: soft miss, try next stage.
		c.logStageMiss(i, stageCount)
	}

	c.logInfo("Could not find '%s' in any source", normalized)
	c.emitNotFoundEvent(normalized, start)
	return nil, c.notFoundError(toolName)
}

// notFoundError returns the appropriate error type based on LLM availability.
func (c *ChainResolver) notFoundError(tool string) error {
	if c.llmAvailability.DeterministicOnly {
		return &ConfigurationError{Tool: tool, Reason: "deterministic_only"}
	}
	if !c.llmAvailability.HasAPIKey {
		return &ConfigurationError{Tool: tool, Reason: "no_api_key"}
	}
	return &NotFoundError{Tool: tool}
}

// logInfo logs an INFO message if a logger is configured.
func (c *ChainResolver) logInfo(format string, args ...any) {
	if c.logger != nil {
		c.logger.Info(fmt.Sprintf(format, args...))
	}
}

// logStageHit logs successful discovery with stage-appropriate messaging.
func (c *ChainResolver) logStageHit(result *DiscoveryResult) {
	if c.logger == nil {
		return
	}
	switch result.Confidence {
	case ConfidenceRegistry:
		c.logger.Info(fmt.Sprintf("Found in registry: %s", result.Source))
	case ConfidenceEcosystem:
		c.logger.Info(fmt.Sprintf("Found in %s (%s)", result.Builder, formatMetadata(result.Metadata)))
	case ConfidenceLLM:
		c.logger.Info(fmt.Sprintf("Found via web search: %s/%s", result.Builder, result.Source))
	}
}

// logStageMiss logs stage miss with guidance about what happens next.
func (c *ChainResolver) logStageMiss(stageIndex, stageCount int) {
	if c.logger == nil {
		return
	}
	remaining := stageCount - stageIndex - 1
	switch remaining {
	case 2: // Registry missed, ecosystem and LLM remain
		c.logger.Info("Not in registry, probing package ecosystems...")
	case 1: // Ecosystem missed, LLM remains
		c.logger.Info("No ecosystem match, trying web search...")
	}
}

// formatMetadata returns a human-readable summary of metadata for verbose output.
func formatMetadata(m Metadata) string {
	if m.Downloads > 0 {
		return fmt.Sprintf("%d downloads", m.Downloads)
	}
	if m.Stars > 0 {
		return fmt.Sprintf("%d stars", m.Stars)
	}
	return "no stats"
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
		// Use enriched event if LLM metrics are available
		if result.LLMMetrics != nil {
			event = telemetry.NewDiscoveryLLMHitEventWithMetrics(
				toolName, result.Builder, result.Source, durationMs,
				result.LLMMetrics.InputTokens, result.LLMMetrics.OutputTokens,
				result.LLMMetrics.Cost, result.LLMMetrics.Provider, result.LLMMetrics.Turns,
			)
		} else {
			event = telemetry.NewDiscoveryLLMHitEvent(toolName, result.Builder, result.Source, durationMs)
		}
	default:
		return
	}
	c.telemetry.SendDiscovery(event)
}

// emitBudgetExceededEvent sends a telemetry event when LLM budget was exceeded.
func (c *ChainResolver) emitBudgetExceededEvent(toolName string, start time.Time) {
	if c.telemetry == nil {
		return
	}
	c.telemetry.SendDiscovery(telemetry.NewDiscoveryLLMBudgetExceededEvent(toolName, time.Since(start).Milliseconds()))
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
