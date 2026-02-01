package discover

import "context"

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
