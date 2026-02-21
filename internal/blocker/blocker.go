// Package blocker computes transitive blocking impact for dependency graphs.
// It is used by both the pipeline dashboard and the queue reorder tool to
// determine how many packages are blocked (directly and transitively) by a
// given dependency.
package blocker

import "strings"

// ComputeTransitiveBlockers computes the total number of packages blocked by a
// dependency, both directly and transitively. Uses memo map with 0-initialization
// for cycle detection: when a dependency is first visited, memo[dep] is set to 0
// (in-progress). If the same dep is encountered again during recursion, the 0 is
// returned, breaking the cycle.
//
// Parameters:
//   - dep: the dependency name to compute blockers for
//   - blockers: map from dependency name to list of blocked package IDs
//   - pkgToBare: map from fully-qualified package ID to bare name
//   - memo: memoization map (shared across calls; caller should create once)
func ComputeTransitiveBlockers(dep string, blockers map[string][]string, pkgToBare map[string]string, memo map[string]int) int {
	if count, ok := memo[dep]; ok {
		return count // 0 if in-progress (cycle)
	}
	// Mark in-progress
	memo[dep] = 0

	// Deduplicate blocked packages for this dependency
	seen := make(map[string]bool)
	total := 0
	for _, pkgID := range blockers[dep] {
		if seen[pkgID] {
			continue
		}
		seen[pkgID] = true
		total++ // Direct dependent
		// Check if this package itself blocks others
		bare := pkgToBare[pkgID]
		if _, isBlocker := blockers[bare]; isBlocker && bare != dep {
			total += ComputeTransitiveBlockers(bare, blockers, pkgToBare, memo)
		}
	}
	memo[dep] = total
	return total
}

// BuildPkgToBare builds a reverse index mapping fully-qualified package IDs
// (e.g., "homebrew:ffmpeg") to their bare names (e.g., "ffmpeg"). This lets
// transitive lookups match blocked package IDs against blocker map keys.
func BuildPkgToBare(blockers map[string][]string) map[string]string {
	pkgToBare := make(map[string]string)
	for _, pkgs := range blockers {
		for _, pkgID := range pkgs {
			if idx := strings.Index(pkgID, ":"); idx >= 0 {
				pkgToBare[pkgID] = pkgID[idx+1:]
			} else {
				pkgToBare[pkgID] = pkgID
			}
		}
	}
	return pkgToBare
}
