package seed

import (
	"time"

	"github.com/tsukumogami/tsuku/internal/batch"
	"github.com/tsukumogami/tsuku/internal/discover"
)

// Per-ecosystem download thresholds for tier-2 assignment.
const (
	cargoTier2Threshold    = 100000  // > 100K recent downloads
	npmTier2Threshold      = 500000  // > 500K weekly downloads
	rubygemsTier2Threshold = 1000000 // > 1M total downloads
)

// AssignTier determines the priority tier for a discovered package.
//   - Tier 1 if the name is in the curated tier1Formulas set.
//   - Tier 2 if downloads exceed the per-ecosystem threshold.
//   - Tier 3 otherwise (including all PyPI candidates, which lack download data).
func AssignTier(name string, downloads int, ecosystem string) int {
	if IsTier1(name) {
		return 1
	}

	switch ecosystem {
	case "cargo":
		if downloads > cargoTier2Threshold {
			return 2
		}
	case "npm":
		if downloads > npmTier2Threshold {
			return 2
		}
	case "rubygems":
		if downloads > rubygemsTier2Threshold {
			return 2
		}
	}

	return 3
}

// ToQueueEntry converts a seed.Package to a batch.QueueEntry.
// If disambiguated is non-nil, its source is used; otherwise the package's
// discovery ecosystem source is used as a fallback.
//
// Status is set to "pending" for clear disambiguation outcomes, or
// "requires_manual" when the selection was a priority_fallback (no clear winner).
func ToQueueEntry(pkg Package, disambiguated *discover.DiscoveryResult) batch.QueueEntry {
	now := time.Now().UTC()

	source := pkg.Source
	status := batch.StatusPending
	confidence := batch.ConfidenceAuto

	if disambiguated != nil {
		source = disambiguated.Builder + ":" + disambiguated.Source
		if disambiguated.Metadata.SelectionReason == discover.SelectionPriorityFallback {
			status = batch.StatusRequiresManual
		}
	}

	return batch.QueueEntry{
		Name:            pkg.Name,
		Source:          source,
		Priority:        pkg.Tier,
		Status:          status,
		Confidence:      confidence,
		DisambiguatedAt: &now,
		FailureCount:    0,
	}
}
