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
// An entry's blocking dependencies come from legacy failure records whose
// package_id is the entry's source, plus per-recipe records under the entry's
// name (blocker.LoadBlockerIndex). Matching legacy records by bare name (the
// part of the package_id after the colon) instead would miss any entry whose
// name differs from its source's identifier, such as hub (github:github/hub).
func Run(queue *batch.UnifiedQueue, failuresDir string) (*Result, error) {
	index, err := blocker.LoadBlockerIndex(failuresDir)
	if err != nil {
		return nil, err
	}

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

		deps := index.For(entry.Source, entry.Name)
		if len(deps) == 0 {
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
