package seed

import (
	"context"
	"time"

	"github.com/tsukumogami/tsuku/internal/builders"
	"github.com/tsukumogami/tsuku/internal/discover"
)

// Disambiguator wraps discover.EcosystemProbe for batch seeding.
// It runs disambiguation in deterministic mode (no interactive prompts)
// and returns detailed probe outcomes for audit logging.
type Disambiguator struct {
	probe *discover.EcosystemProbe
}

// NewDisambiguator creates a Disambiguator that probes all provided
// ecosystem builders in parallel. It uses WithForceDeterministic() so
// close matches select the first ranked result rather than prompting.
func NewDisambiguator(probers []builders.EcosystemProber, timeout time.Duration) *Disambiguator {
	probe := discover.NewEcosystemProbe(
		probers,
		timeout,
		discover.WithForceDeterministic(),
	)
	return &Disambiguator{probe: probe}
}

// Resolve runs disambiguation for the given tool name across all ecosystems.
// Returns the full ResolveResult including probe outcomes for audit logging.
func (d *Disambiguator) Resolve(ctx context.Context, name string) (*discover.ResolveResult, error) {
	return d.probe.ResolveWithDetails(ctx, name)
}
