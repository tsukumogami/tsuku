package discover

import (
	"context"
	"fmt"
	"os"
)

// RegistryLookup resolves tool names against the discovery registry directory.
// This is the first stage of the resolver chain: instant, no network, no API keys.
type RegistryLookup struct {
	dir string
}

// NewRegistryLookup creates a registry lookup resolver that reads from a directory.
// The directory contains per-tool JSON files in a nested structure.
func NewRegistryLookup(dir string) (*RegistryLookup, error) {
	if dir == "" {
		return nil, fmt.Errorf("discovery registry directory is empty")
	}
	return &RegistryLookup{dir: dir}, nil
}

// Resolve looks up the tool name in the discovery registry directory.
// Returns (nil, nil) on miss — the chain continues to the next stage.
func (r *RegistryLookup) Resolve(_ context.Context, toolName string) (*DiscoveryResult, error) {
	entry, err := LoadRegistryEntry(r.dir, toolName)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		// Unwrap to check for os.ErrNotExist in wrapped errors
		if pathErr, ok := err.(*os.PathError); ok && os.IsNotExist(pathErr) {
			return nil, nil
		}
		return nil, nil
	}
	return &DiscoveryResult{
		Builder:    entry.Builder,
		Source:     entry.Source,
		Confidence: ConfidenceRegistry,
		Reason:     fmt.Sprintf("found in discovery registry (builder: %s, source: %s)", entry.Builder, entry.Source),
	}, nil
}
