package discover

import (
	"context"
	"fmt"
)

// RegistryLookup resolves tool names against the discovery registry.
// This is the first stage of the resolver chain: instant, no network, no API keys.
type RegistryLookup struct {
	registry *DiscoveryRegistry
}

// NewRegistryLookup creates a registry lookup resolver.
// Returns an error if registry is nil.
func NewRegistryLookup(registry *DiscoveryRegistry) (*RegistryLookup, error) {
	if registry == nil {
		return nil, fmt.Errorf("discovery registry is nil")
	}
	return &RegistryLookup{registry: registry}, nil
}

// Resolve looks up the tool name in the discovery registry.
// Returns (nil, nil) on miss — the chain continues to the next stage.
func (r *RegistryLookup) Resolve(_ context.Context, toolName string) (*DiscoveryResult, error) {
	entry, ok := r.registry.Lookup(toolName)
	if !ok {
		return nil, nil
	}
	return &DiscoveryResult{
		Builder:    entry.Builder,
		Source:     entry.Source,
		Confidence: ConfidenceRegistry,
		Reason:     fmt.Sprintf("found in discovery registry (builder: %s, source: %s)", entry.Builder, entry.Source),
	}, nil
}
