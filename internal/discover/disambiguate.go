package discover

import "sort"

// rankProbeResults sorts matches by downloads DESC, version count DESC, priority ASC.
// Priority is used as a tiebreaker when popularity signals are equal.
func rankProbeResults(matches []probeOutcome, priority map[string]int) {
	sort.Slice(matches, func(i, j int) bool {
		a, b := matches[i], matches[j]

		// Primary: downloads DESC
		if a.result.Downloads != b.result.Downloads {
			return a.result.Downloads > b.result.Downloads
		}

		// Secondary: version count DESC
		if a.result.VersionCount != b.result.VersionCount {
			return a.result.VersionCount > b.result.VersionCount
		}

		// Tertiary: priority ASC (lower = better)
		pi := priority[a.builderName]
		pj := priority[b.builderName]
		if pi == 0 {
			pi = 999 // Unknown builders get lowest priority
		}
		if pj == 0 {
			pj = 999
		}
		return pi < pj
	})
}

// isClearWinner checks if the first match dominates the second.
// A clear winner must have:
//   - >= 10x downloads of the runner-up
//   - version count >= 3 (sustained activity)
//   - a linked repository (source transparency)
//
// Returns false if popularity data is missing from either match.
func isClearWinner(first, second probeOutcome) bool {
	// Require download data from both matches
	if first.result.Downloads == 0 || second.result.Downloads == 0 {
		return false
	}

	// Require secondary signals for auto-selection
	if first.result.VersionCount < 3 {
		return false
	}
	if !first.result.HasRepository {
		return false
	}

	// Check 10x threshold
	return first.result.Downloads >= second.result.Downloads*10
}

// disambiguate ranks multiple probe results and selects the best one.
// Returns the selected result if there's a clear winner, or AmbiguousMatchError otherwise.
//
// Selection logic:
//   - Single match: auto-select (no threshold checks needed)
//   - Clear winner (>10x gap + secondary signals): auto-select
//   - Close matches or missing popularity: return AmbiguousMatchError
func disambiguate(toolName string, matches []probeOutcome, priority map[string]int) (*DiscoveryResult, error) {
	if len(matches) == 0 {
		return nil, nil
	}

	// Single match: auto-select without checking thresholds
	if len(matches) == 1 {
		return toDiscoveryResult(matches[0]), nil
	}

	// Multiple matches: rank and check for clear winner
	rankProbeResults(matches, priority)

	if isClearWinner(matches[0], matches[1]) {
		return toDiscoveryResult(matches[0]), nil
	}

	// No clear winner: return ambiguous error for downstream handling
	return nil, &AmbiguousMatchError{
		Tool:    toolName,
		Matches: toDiscoveryMatches(matches),
	}
}

// toDiscoveryResult converts a probeOutcome to a DiscoveryResult.
func toDiscoveryResult(outcome probeOutcome) *DiscoveryResult {
	return &DiscoveryResult{
		Builder:    outcome.builderName,
		Source:     outcome.result.Source,
		Confidence: ConfidenceEcosystem,
		Reason:     "found in " + outcome.builderName + " ecosystem",
		Metadata: Metadata{
			Downloads:     outcome.result.Downloads,
			VersionCount:  outcome.result.VersionCount,
			HasRepository: outcome.result.HasRepository,
		},
	}
}

// toDiscoveryMatches converts probeOutcomes to DiscoveryMatches for error display.
func toDiscoveryMatches(matches []probeOutcome) []DiscoveryMatch {
	result := make([]DiscoveryMatch, len(matches))
	for i, m := range matches {
		result[i] = DiscoveryMatch{
			Builder:       m.builderName,
			Source:        m.result.Source,
			Downloads:     m.result.Downloads,
			VersionCount:  m.result.VersionCount,
			HasRepository: m.result.HasRepository,
		}
	}
	return result
}
