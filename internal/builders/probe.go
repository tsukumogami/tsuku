package builders

import "context"

// ProbeResult holds metadata from an ecosystem registry query.
// A nil result from Probe() means the package was not found.
type ProbeResult struct {
	Source        string // Builder-specific source arg
	Downloads     int    // Recent downloads (0 if unavailable)
	VersionCount  int    // Number of published versions (0 if unavailable)
	HasRepository bool   // Whether the package has a linked source repository
}

// EcosystemProber extends SessionBuilder with metadata for discovery.
// Builders that implement this interface participate in the ecosystem probe stage.
// Probe returns a non-nil ProbeResult if the package exists, or nil if not found.
type EcosystemProber interface {
	SessionBuilder
	Probe(ctx context.Context, name string) (*ProbeResult, error)
}
