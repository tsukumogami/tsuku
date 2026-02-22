// Package requeue flips blocked queue entries to pending when their
// missing dependency recipes have been resolved. A dependency is
// considered resolved if its name appears as a "success" entry in
// the unified queue.
package requeue

import (
	"github.com/tsukumogami/tsuku/internal/batch"
	"github.com/tsukumogami/tsuku/internal/blocker"
)

// Result summarizes the outcome of a requeue operation.
type Result struct {
	Requeued  int      // Number of entries flipped from blocked to pending
	Remaining int      // Number of entries still blocked
	Details   []Change // Per-entry changes for flipped entries
}

// Change records a single entry that was flipped from blocked to pending.
type Change struct {
	Name       string   // Entry name
	ResolvedBy []string // Blocker names that were resolved
}

// Run checks each blocked entry in the queue and flips it to pending if all
// its dependency blockers appear as "success" entries in the queue. It modifies
// the queue in place and does not perform any queue I/O (the caller loads and
// saves the queue).
//
// The blocker map is loaded from JSONL failure data in failuresDir using the
// shared function in internal/blocker. The map is then inverted to build a
// reverse index: for each blocked package, which dependencies block it.
func Run(queue *batch.UnifiedQueue, failuresDir string) (*Result, error) {
	// Load blocker map: dependency_name -> []blocked_package_id
	blockerMap, err := blocker.LoadBlockerMap(failuresDir)
	if err != nil {
		return nil, err
	}

	// Build reverse index: entry_name -> []dependency_names
	// The blocker map keys are dependency names, values are package IDs
	// like "homebrew:ffmpeg". We need to map entry names (bare names like
	// "ffmpeg") to the dependency names that block them.
	reverseIndex := buildReverseIndex(blockerMap)

	// Build resolved set from queue entries with status "success"
	resolved := make(map[string]bool)
	for _, entry := range queue.Entries {
		if entry.Status == batch.StatusSuccess {
			resolved[entry.Name] = true
		}
	}

	result := &Result{}

	for i := range queue.Entries {
		entry := &queue.Entries[i]
		if entry.Status != batch.StatusBlocked {
			continue
		}

		deps, hasDeps := reverseIndex[entry.Name]
		if !hasDeps {
			// No failure record for this blocked entry. This can happen when
			// failure data has aged out or was never recorded. The entry stays
			// blocked since we can't determine what's blocking it.
			result.Remaining++
			continue
		}

		// Check if all blockers are resolved
		var resolvedDeps []string
		allResolved := true
		for _, dep := range deps {
			if resolved[dep] {
				resolvedDeps = append(resolvedDeps, dep)
			} else {
				allResolved = false
			}
		}

		if allResolved {
			entry.Status = batch.StatusPending
			result.Requeued++
			result.Details = append(result.Details, Change{
				Name:       entry.Name,
				ResolvedBy: resolvedDeps,
			})
		} else {
			result.Remaining++
		}
	}

	return result, nil
}

// buildReverseIndex inverts the blocker map (dependency -> []package_id) to
// produce a map of bare_package_name -> []dependency_names. Package IDs in
// the blocker map are fully qualified (e.g., "homebrew:ffmpeg"), so we strip
// the ecosystem prefix to get the bare name that matches queue entry names.
func buildReverseIndex(blockerMap map[string][]string) map[string][]string {
	reverse := make(map[string][]string)
	for dep, pkgIDs := range blockerMap {
		for _, pkgID := range pkgIDs {
			bare := bareName(pkgID)
			// Avoid adding duplicate dependencies for the same package.
			// This can happen when multiple failure records reference the
			// same dep for the same package.
			if !containsString(reverse[bare], dep) {
				reverse[bare] = append(reverse[bare], dep)
			}
		}
	}
	return reverse
}

// bareName extracts the bare name from a fully-qualified package ID.
// For "homebrew:ffmpeg" it returns "ffmpeg". For "ffmpeg" it returns "ffmpeg".
func bareName(pkgID string) string {
	for i := 0; i < len(pkgID); i++ {
		if pkgID[i] == ':' {
			return pkgID[i+1:]
		}
	}
	return pkgID
}

// containsString reports whether s is in the slice.
func containsString(slice []string, s string) bool {
	for _, v := range slice {
		if v == s {
			return true
		}
	}
	return false
}
