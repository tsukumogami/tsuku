package discover

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/tsukumogami/tsuku/internal/builders"
)

// EcosystemProbe resolves tool names by querying ecosystem registries in parallel.
// This is the second stage of the resolver chain.
type EcosystemProbe struct {
	probers               []builders.EcosystemProber
	timeout               time.Duration
	priority              map[string]int // builder name → priority rank (lower = better)
	filter                *QualityFilter
	confirmDisambiguation ConfirmDisambiguationFunc // optional callback for interactive mode
	forceDeterministic    bool                      // select deterministically even without clear winner
}

// EcosystemProbeOption configures an EcosystemProbe.
type EcosystemProbeOption func(*EcosystemProbe)

// WithConfirmDisambiguation sets a callback for interactive disambiguation.
// When provided, close matches prompt the user to select instead of returning
// AmbiguousMatchError.
func WithConfirmDisambiguation(fn ConfirmDisambiguationFunc) EcosystemProbeOption {
	return func(p *EcosystemProbe) {
		p.confirmDisambiguation = fn
	}
}

// WithForceDeterministic enables deterministic selection even without a clear winner.
// When enabled, close matches select the first ranked result (by priority) and mark
// the selection as "priority_fallback" with HighRisk metadata. Use this for batch mode
// where all decisions must be deterministic and tracked for later human review.
func WithForceDeterministic() EcosystemProbeOption {
	return func(p *EcosystemProbe) {
		p.forceDeterministic = true
	}
}

// NewEcosystemProbe creates a resolver that queries ecosystem builders in parallel.
// The timeout applies to all probers collectively via a shared context.
func NewEcosystemProbe(probers []builders.EcosystemProber, timeout time.Duration, opts ...EcosystemProbeOption) *EcosystemProbe {
	p := &EcosystemProbe{
		probers: probers,
		timeout: timeout,
		priority: map[string]int{
			"cask":      1,
			"homebrew":  2,
			"crates.io": 3,
			"pypi":      4,
			"npm":       5,
			"rubygems":  6,
			"go":        7,
			"cpan":      8,
		},
		filter: NewQualityFilter(),
	}
	for _, opt := range opts {
		opt(p)
	}
	return p
}

// probeOutcome holds the result from a single builder's Probe() call.
type probeOutcome struct {
	builderName string
	result      *builders.ProbeResult
	err         error
}

// probeAll runs all probers in parallel and collects their outcomes.
// Returns the full list of outcomes and the count of hard errors.
func (p *EcosystemProbe) probeAll(ctx context.Context, toolName string) ([]probeOutcome, int) {
	ch := make(chan probeOutcome, len(p.probers))
	var wg sync.WaitGroup

	for _, prober := range p.probers {
		wg.Add(1)
		go func(pr builders.EcosystemProber) {
			defer wg.Done()
			result, err := pr.Probe(ctx, toolName)
			ch <- probeOutcome{
				builderName: pr.Name(),
				result:      result,
				err:         err,
			}
		}(prober)
	}

	go func() {
		wg.Wait()
		close(ch)
	}()

	var all []probeOutcome
	var hardErrors int
	for outcome := range ch {
		all = append(all, outcome)
		if outcome.err != nil {
			hardErrors++
		}
	}
	return all, hardErrors
}

// filterMatches returns outcomes that pass name matching and quality filters.
func (p *EcosystemProbe) filterMatches(outcomes []probeOutcome, toolName string) []probeOutcome {
	var matches []probeOutcome
	for _, outcome := range outcomes {
		if outcome.err != nil || outcome.result == nil {
			continue
		}
		if !strings.EqualFold(outcome.result.Source, toolName) {
			continue
		}
		if ok, _ := p.filter.Accept(outcome.builderName, outcome.result); !ok {
			continue
		}
		matches = append(matches, outcome)
	}
	return matches
}

// Resolve queries all ecosystem builders in parallel and returns the best match.
// Returns (nil, nil) if no builder finds the tool. Returns an error only if all
// builders fail with hard errors.
func (p *EcosystemProbe) Resolve(ctx context.Context, toolName string) (*DiscoveryResult, error) {
	if len(p.probers) == 0 {
		return nil, nil
	}

	ctx, cancel := context.WithTimeout(ctx, p.timeout)
	defer cancel()

	outcomes, hardErrors := p.probeAll(ctx, toolName)
	matches := p.filterMatches(outcomes, toolName)

	if len(matches) == 0 {
		if hardErrors == len(p.probers) {
			return nil, fmt.Errorf("all %d ecosystem probers failed", hardErrors)
		}
		return nil, nil
	}

	// Disambiguate: rank by popularity and check for clear winner.
	return disambiguate(toolName, matches, p.priority, p.confirmDisambiguation, p.forceDeterministic)
}

// ProbeOutcome holds the result from a single builder's Probe() call.
// This is the exported counterpart of the internal probeOutcome type,
// used by ResolveWithDetails() to expose all probe results for audit logging.
type ProbeOutcome struct {
	BuilderName string
	Result      *builders.ProbeResult
	Err         error
}

// ResolveResult holds both the selected disambiguation result and the raw
// probe outcomes from all ecosystems. Used by the seeding pipeline for
// audit logging.
type ResolveResult struct {
	Selected  *DiscoveryResult
	AllProbes []ProbeOutcome
}

// ResolveWithDetails queries all ecosystem builders in parallel and returns
// both the selected result and all probe outcomes (including non-matches
// and errors). This is the audit-friendly variant of Resolve().
func (p *EcosystemProbe) ResolveWithDetails(ctx context.Context, toolName string) (*ResolveResult, error) {
	if len(p.probers) == 0 {
		return &ResolveResult{}, nil
	}

	ctx, cancel := context.WithTimeout(ctx, p.timeout)
	defer cancel()

	outcomes, hardErrors := p.probeAll(ctx, toolName)

	// Convert internal outcomes to exported type.
	allProbes := make([]ProbeOutcome, len(outcomes))
	for i, o := range outcomes {
		allProbes[i] = ProbeOutcome{
			BuilderName: o.builderName,
			Result:      o.result,
			Err:         o.err,
		}
	}

	matches := p.filterMatches(outcomes, toolName)

	if len(matches) == 0 {
		if hardErrors == len(p.probers) {
			return nil, fmt.Errorf("all %d ecosystem probers failed", hardErrors)
		}
		return &ResolveResult{AllProbes: allProbes}, nil
	}

	selected, err := disambiguate(toolName, matches, p.priority, p.confirmDisambiguation, p.forceDeterministic)
	if err != nil {
		return nil, err
	}

	return &ResolveResult{
		Selected:  selected,
		AllProbes: allProbes,
	}, nil
}
