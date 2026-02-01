package discover

import "context"

// LLMDiscovery resolves tool names via LLM web search as a last resort.
// This is the third and final stage of the resolver chain.
//
// Stub: always returns (nil, nil). Implementation deferred to its own design.
type LLMDiscovery struct{}

// Resolve is a stub that always misses. Real implementation will invoke the
// LLM with web search, verify results against the GitHub API, and require
// user confirmation.
func (d *LLMDiscovery) Resolve(_ context.Context, _ string) (*DiscoveryResult, error) {
	return nil, nil
}
