package builders

import "context"

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
	SessionBuilder
	Probe(ctx context.Context, name string) (*ProbeResult, error)
}
