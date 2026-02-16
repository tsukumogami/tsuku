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
		runCargo(*limit, *recipesDir, *embeddedDir, *queuePath, *auditDir, *disambiguate)
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
// to the unified queue.
func runCargo(limit int, recipesDir, embeddedDir, queuePath, auditDir string, disambiguateFlag bool) {
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

	if len(packages) == 0 {
		fmt.Fprintf(os.Stderr, "No new packages to add\n")
		return
	}

	// Resolve the absolute audit directory path relative to the queue file.
	absAuditDir := auditDir
	if !filepath.IsAbs(absAuditDir) {
		absAuditDir, _ = filepath.Abs(absAuditDir)
	}

	// Convert to queue entries, optionally running disambiguation.
	var entries []batch.QueueEntry
	var disambiguated, failed, audited int

	if disambiguateFlag {
		// Build disambiguator with the cargo builder as one of the probers.
		// In a full implementation, all 8 builders would be provided.
		d := seed.NewDisambiguator(
			[]builders.EcosystemProber{cargoBuilder},
			30*time.Second,
		)

		for _, pkg := range packages {
			rr, err := d.Resolve(ctx, pkg.Name)
			if err != nil {
				fmt.Fprintf(os.Stderr, "  disambiguation error for %s: %v\n", pkg.Name, err)
				failed++
				// Fall back to discovery ecosystem source.
				entries = append(entries, seed.ToQueueEntry(pkg, nil))
				continue
			}
			disambiguated++
			entries = append(entries, seed.ToQueueEntry(pkg, rr.Selected))

			// Write audit entry for this disambiguation decision.
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

	// Merge into unified queue.
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

	if err := batch.SaveUnifiedQueue(queuePath, unifiedQueue); err != nil {
		fmt.Fprintf(os.Stderr, "error saving queue: %v\n", err)
		os.Exit(1)
	}

	fmt.Fprintf(os.Stderr, "Merged %d new entries into unified queue (%d total)\n", added, len(unifiedQueue.Entries))
	if disambiguateFlag {
		fmt.Fprintf(os.Stderr, "Disambiguation: %d resolved, %d failed, %d audited\n", disambiguated, failed, audited)
	}
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
