package discover

import (
	"context"

	"github.com/tsukumogami/tsuku/internal/builders"
)

// ProbeResult holds metadata from an ecosystem registry query.
type ProbeResult struct {
	Exists    bool
	Downloads int    // Monthly downloads (0 if unavailable)
	Age       int    // Days since first publish (0 if unavailable)
	Source    string // Builder-specific source arg
}

// EcosystemProber extends SessionBuilder with metadata for discovery.
// Builders that implement this interface participate in the ecosystem probe stage.
type EcosystemProber interface {
	builders.SessionBuilder
	Probe(ctx context.Context, name string) (*ProbeResult, error)
}

// EcosystemProbe resolves tool names by querying ecosystem registries in parallel.
// This is the second stage of the resolver chain.
//
// Stub: always returns (nil, nil). Implementation deferred to its own design.
type EcosystemProbe struct{}

// Resolve is a stub that always misses. Real implementation will query all
// EcosystemProber builders in parallel with a shared timeout.
func (p *EcosystemProbe) Resolve(_ context.Context, _ string) (*DiscoveryResult, error) {
	return nil, nil
}
