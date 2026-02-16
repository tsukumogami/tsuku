package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/tsukumogami/tsuku/internal/batch"
	"github.com/tsukumogami/tsuku/internal/builders"
	"github.com/tsukumogami/tsuku/internal/discover"
	"github.com/tsukumogami/tsuku/internal/seed"
)

func main() {
	source := flag.String("source", "", "package source (homebrew, cargo)")
	limit := flag.Int("limit", 100, "max packages to fetch")
	recipesDir := flag.String("recipes-dir", "", "path to registry recipes directory (skip packages with existing recipes)")
	embeddedDir := flag.String("embedded-dir", "", "path to embedded recipes directory (skip packages with existing recipes)")
	queuePath := flag.String("queue", "data/queues/priority-queue.json", "path to unified queue")
	auditDir := flag.String("audit-dir", "data/disambiguations/audit", "path to audit log directory")
	disambiguate := flag.Bool("disambiguate", true, "run disambiguation for new packages")
	freshness := flag.Int("freshness", 30, "re-disambiguate entries older than N days")
	flag.Parse()

	if *source == "" {
		fmt.Fprintln(os.Stderr, "error: -source is required")
		flag.Usage()
		os.Exit(1)
	}

	switch *source {
	case "homebrew":
		runHomebrew(*limit, *recipesDir, *embeddedDir)
	case "cargo":
		runCargo(*limit, *recipesDir, *embeddedDir, *queuePath, *auditDir, *disambiguate, *freshness)
	default:
		fmt.Fprintf(os.Stderr, "error: unsupported source %q\n", *source)
		os.Exit(1)
	}
}

// runHomebrew is the original homebrew path: writes to per-ecosystem file.
func runHomebrew(limit int, recipesDir, embeddedDir string) {
	output := "data/queues/priority-queue-homebrew.json"
	src := &seed.HomebrewSource{}

	queue, err := seed.Load(output)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}

	packages, err := src.Fetch(limit)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}

	if recipesDir != "" || embeddedDir != "" {
		var skipped []string
		packages, skipped = seed.FilterExistingRecipes(packages, recipesDir, embeddedDir)
		if len(skipped) > 0 {
			fmt.Fprintf(os.Stderr, "Skipped %d packages with existing recipes: %v\n", len(skipped), skipped)
		}
	}

	added := queue.Merge(packages)
	if err := queue.Save(output); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}

	fmt.Fprintf(os.Stderr, "Merged %d new packages (%d total)\n", added, len(queue.Packages))
}

// runCargo discovers popular CLI crates, runs disambiguation, and writes
// to the unified queue. Also performs freshness checking on existing entries.
func runCargo(limit int, recipesDir, embeddedDir, queuePath, auditDir string, disambiguateFlag bool, freshnessDays int) {
	ctx := context.Background()
	seedingRun := time.Now().UTC()

	// Load unified queue.
	unifiedQueue, err := batch.LoadUnifiedQueue(queuePath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error loading queue: %v\n", err)
		os.Exit(1)
	}

	// Build a name->source lookup for detecting re-disambiguations.
	existingSources := make(map[string]string, len(unifiedQueue.Entries))
	for _, e := range unifiedQueue.Entries {
		existingSources[e.Name] = e.Source
	}

	// Discover candidates from crates.io.
	cargoBuilder := builders.NewCargoBuilder(nil)
	candidates, err := cargoBuilder.Discover(ctx, limit)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error discovering crates: %v\n", err)
		os.Exit(1)
	}
	fmt.Fprintf(os.Stderr, "Discovered %d candidates from crates.io\n", len(candidates))

	// Build a set of discovered candidate names for trigger 3 (new audit candidate).
	discoveredNames := make(map[string]string, len(candidates))
	for _, c := range candidates {
		discoveredNames[c.Name] = "cargo:" + c.Name
	}

	// Convert to seed.Package for filtering.
	now := seedingRun.Format(time.RFC3339)
	var packages []seed.Package
	for _, c := range candidates {
		tier := seed.AssignTier(c.Name, c.Downloads, "cargo")
		packages = append(packages, seed.Package{
			ID:      "cargo:" + c.Name,
			Source:  "cargo:" + c.Name,
			Name:    c.Name,
			Tier:    tier,
			Status:  "pending",
			AddedAt: now,
		})
	}

	// Filter packages that already have recipes.
	if recipesDir != "" || embeddedDir != "" {
		var skipped []string
		packages, skipped = seed.FilterExistingRecipes(packages, recipesDir, embeddedDir)
		if len(skipped) > 0 {
			fmt.Fprintf(os.Stderr, "Filtered %d packages with existing recipes\n", len(skipped))
		}
	}

	// Filter packages already in the unified queue (name-based).
	packages = seed.FilterByName(packages, unifiedQueue)
	fmt.Fprintf(os.Stderr, "After filtering: %d new packages\n", len(packages))

	// Resolve the absolute audit directory path relative to the queue file.
	absAuditDir := auditDir
	if !filepath.IsAbs(absAuditDir) {
		absAuditDir, _ = filepath.Abs(absAuditDir)
	}

	// Build disambiguator (shared between new packages and freshness checking).
	var d *seed.Disambiguator
	if disambiguateFlag {
		// In a full implementation, all 8 builders would be provided.
		d = seed.NewDisambiguator(
			[]builders.EcosystemProber{cargoBuilder},
			30*time.Second,
		)
	}

	// --- Process new packages ---
	var entries []batch.QueueEntry
	var disambiguated, failed, audited int

	if len(packages) > 0 {
		if d != nil {
			for _, pkg := range packages {
				rr, err := d.Resolve(ctx, pkg.Name)
				if err != nil {
					fmt.Fprintf(os.Stderr, "  disambiguation error for %s: %v\n", pkg.Name, err)
					failed++
					entries = append(entries, seed.ToQueueEntry(pkg, nil))
					continue
				}
				disambiguated++
				entries = append(entries, seed.ToQueueEntry(pkg, rr.Selected))

				if rr.Selected != nil {
					auditEntry := buildAuditEntry(pkg.Name, rr, existingSources, seedingRun)
					if writeErr := seed.WriteAuditEntry(absAuditDir, auditEntry); writeErr != nil {
						fmt.Fprintf(os.Stderr, "  audit write error for %s: %v\n", pkg.Name, writeErr)
					} else {
						audited++
					}
				}
			}
		} else {
			for _, pkg := range packages {
				entries = append(entries, seed.ToQueueEntry(pkg, nil))
			}
		}
	}

	// Merge new entries into unified queue.
	added := 0
	existingNames := make(map[string]bool, len(unifiedQueue.Entries))
	for _, e := range unifiedQueue.Entries {
		existingNames[e.Name] = true
	}
	for _, entry := range entries {
		if !existingNames[entry.Name] {
			unifiedQueue.Entries = append(unifiedQueue.Entries, entry)
			existingNames[entry.Name] = true
			added++
		}
	}

	// --- Freshness checking on existing entries ---
	freshnessCfg := seed.FreshnessConfig{
		ThresholdDays: freshnessDays,
		Now:           seedingRun,
	}
	var sourceChanges []seed.SourceChange
	var curatedInvalid []seed.CuratedInvalid
	var curatedSkipped, staleRefreshed int

	curatedValidator := &seed.CuratedSourceValidator{}

	for i := range unifiedQueue.Entries {
		entry := &unifiedQueue.Entries[i]

		// Skip success entries entirely.
		if seed.ShouldSkip(*entry) {
			continue
		}

		// Curated entries: validate source, never re-disambiguate.
		if seed.IsCurated(*entry) {
			curatedSkipped++
			if err := curatedValidator.Validate(entry.Source); err != nil {
				curatedInvalid = append(curatedInvalid, seed.CuratedInvalid{
					Package: entry.Name,
					Source:  entry.Source,
					Error:   err.Error(),
				})
			}
			continue
		}

		// Check if this entry needs re-disambiguation.
		var auditEntry *seed.AuditEntry
		auditEntry, _ = seed.ReadAuditEntry(absAuditDir, entry.Name)

		// Determine the discovered source for trigger 3.
		discoveredSource := discoveredNames[entry.Name]

		if !seed.NeedsRedisambiguation(*entry, freshnessCfg, auditEntry, discoveredSource) {
			continue
		}

		// Entry needs re-disambiguation.
		if d == nil {
			// Disambiguation disabled; just update the timestamp.
			seed.UpdateDisambiguatedAt(entry, seedingRun)
			staleRefreshed++
			continue
		}

		rr, err := d.Resolve(ctx, entry.Name)
		if err != nil {
			fmt.Fprintf(os.Stderr, "  freshness re-disambiguation error for %s: %v\n", entry.Name, err)
			continue
		}

		if rr.Selected == nil {
			// No match found; update timestamp but don't change source.
			seed.UpdateDisambiguatedAt(entry, seedingRun)
			staleRefreshed++
			continue
		}

		newSource := rr.Selected.Builder + ":" + rr.Selected.Source
		selectionReason := rr.Selected.Metadata.SelectionReason

		// Apply selection result (pending vs requires_manual).
		seed.ApplySelectionResult(entry, selectionReason)

		// Check for source change.
		if newSource != entry.Source {
			change, modified := seed.ApplySourceChange(entry, newSource, seedingRun)
			sourceChanges = append(sourceChanges, change)
			if modified {
				fmt.Fprintf(os.Stderr, "  source change (auto-accepted): %s -> %s for %s\n",
					change.Old, change.New, entry.Name)
			} else {
				fmt.Fprintf(os.Stderr, "  source change (flagged): %s -> %s for %s (priority %d)\n",
					change.Old, change.New, entry.Name, entry.Priority)
			}
		}

		// Update disambiguated_at timestamp.
		seed.UpdateDisambiguatedAt(entry, seedingRun)
		staleRefreshed++

		// Write updated audit entry.
		newAuditEntry := buildAuditEntry(entry.Name, rr, existingSources, seedingRun)
		// Mark high_risk for priority_fallback.
		if selectionReason == discover.SelectionPriorityFallback {
			newAuditEntry.HighRisk = true
		}
		if writeErr := seed.WriteAuditEntry(absAuditDir, newAuditEntry); writeErr != nil {
			fmt.Fprintf(os.Stderr, "  audit write error for %s: %v\n", entry.Name, writeErr)
		}
	}

	if err := batch.SaveUnifiedQueue(queuePath, unifiedQueue); err != nil {
		fmt.Fprintf(os.Stderr, "error saving queue: %v\n", err)
		os.Exit(1)
	}

	fmt.Fprintf(os.Stderr, "Merged %d new entries into unified queue (%d total)\n", added, len(unifiedQueue.Entries))
	if disambiguateFlag {
		fmt.Fprintf(os.Stderr, "Disambiguation: %d resolved, %d failed, %d audited\n", disambiguated, failed, audited)
	}
	fmt.Fprintf(os.Stderr, "Freshness: %d refreshed, %d curated skipped, %d curated invalid, %d source changes\n",
		staleRefreshed, curatedSkipped, len(curatedInvalid), len(sourceChanges))
}

// buildAuditEntry constructs an AuditEntry from a disambiguation result.
// If the package already existed in the queue (re-disambiguation),
// previous_source is set to its current source.
func buildAuditEntry(name string, rr *discover.ResolveResult, existingSources map[string]string, seedingRun time.Time) seed.AuditEntry {
	now := time.Now().UTC()

	// Build the DisambiguationRecord from the selected result.
	selected := rr.Selected
	record := batch.DisambiguationRecord{
		Tool:            name,
		Selected:        selected.Builder + ":" + selected.Source,
		SelectionReason: selected.Metadata.SelectionReason,
		DownloadsRatio:  selected.Metadata.DownloadsRatio,
		HighRisk:        selected.Metadata.SelectionReason == discover.SelectionPriorityFallback,
	}

	// Collect alternative sources from the disambiguation metadata.
	for _, alt := range selected.Metadata.Alternatives {
		record.Alternatives = append(record.Alternatives, alt.Builder+":"+alt.Source)
	}

	// Convert successful probe outcomes to audit probe results.
	var probeResults []seed.AuditProbeResult
	for _, probe := range rr.AllProbes {
		if probe.Err != nil || probe.Result == nil {
			continue
		}
		probeResults = append(probeResults, seed.AuditProbeResult{
			Source:        probe.BuilderName + ":" + probe.Result.Source,
			Downloads:     probe.Result.Downloads,
			VersionCount:  probe.Result.VersionCount,
			HasRepository: probe.Result.HasRepository,
		})
	}

	// Check if this is a re-disambiguation (package already in queue).
	var previousSource *string
	if src, ok := existingSources[name]; ok {
		previousSource = &src
	}

	return seed.AuditEntry{
		DisambiguationRecord: record,
		ProbeResults:         probeResults,
		PreviousSource:       previousSource,
		DisambiguatedAt:      now,
		SeedingRun:           seedingRun,
	}
}
