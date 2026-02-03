package discover

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/tsukumogami/tsuku/internal/builders"
)

// EcosystemProbe resolves tool names by querying ecosystem registries in parallel.
// This is the second stage of the resolver chain.
type EcosystemProbe struct {
	probers  []builders.EcosystemProber
	timeout  time.Duration
	priority map[string]int // builder name → priority rank (lower = better)
	filter   *QualityFilter
}

// NewEcosystemProbe creates a resolver that queries ecosystem builders in parallel.
// The timeout applies to all probers collectively via a shared context.
func NewEcosystemProbe(probers []builders.EcosystemProber, timeout time.Duration) *EcosystemProbe {
	return &EcosystemProbe{
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
}

// probeOutcome holds the result from a single builder's Probe() call.
type probeOutcome struct {
	builderName string
	result      *builders.ProbeResult
	err         error
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

	// Close channel after all goroutines finish.
	go func() {
		wg.Wait()
		close(ch)
	}()

	var matches []probeOutcome
	var hardErrors int

	for outcome := range ch {
		if outcome.err != nil {
			hardErrors++
			continue
		}
		if outcome.result == nil {
			continue
		}
		// Exact name match filter (case-insensitive).
		if !strings.EqualFold(outcome.result.Source, toolName) {
			continue
		}
		// Quality filter: reject low-quality matches.
		if ok, _ := p.filter.Accept(outcome.builderName, outcome.result); !ok {
			continue
		}
		matches = append(matches, outcome)
	}

	if len(matches) == 0 {
		if hardErrors == len(p.probers) {
			return nil, fmt.Errorf("all %d ecosystem probers failed", hardErrors)
		}
		return nil, nil
	}

	// Sort by priority (lower number = higher priority).
	sort.Slice(matches, func(i, j int) bool {
		pi := p.priority[matches[i].builderName]
		pj := p.priority[matches[j].builderName]
		// Unknown builders get lowest priority.
		if pi == 0 {
			pi = 999
		}
		if pj == 0 {
			pj = 999
		}
		return pi < pj
	})

	best := matches[0]
	return &DiscoveryResult{
		Builder:    best.builderName,
		Source:     best.result.Source,
		Confidence: ConfidenceEcosystem,
		Reason:     fmt.Sprintf("found in %s ecosystem", best.builderName),
		Metadata: Metadata{
			Downloads: best.result.Downloads,
		},
	}, nil
}
