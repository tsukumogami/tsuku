package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/tsukumogami/tsuku/internal/batch"
	"github.com/tsukumogami/tsuku/internal/builders"
	"github.com/tsukumogami/tsuku/internal/seed"
)

func main() {
	source := flag.String("source", "", "package source (homebrew, cargo)")
	limit := flag.Int("limit", 100, "max packages to fetch")
	recipesDir := flag.String("recipes-dir", "", "path to registry recipes directory (skip packages with existing recipes)")
	embeddedDir := flag.String("embedded-dir", "", "path to embedded recipes directory (skip packages with existing recipes)")
	queuePath := flag.String("queue", "data/queues/priority-queue.json", "path to unified queue")
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
		runCargo(*limit, *recipesDir, *embeddedDir, *queuePath, *disambiguate)
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
func runCargo(limit int, recipesDir, embeddedDir, queuePath string, disambiguateFlag bool) {
	ctx := context.Background()

	// Load unified queue.
	unifiedQueue, err := batch.LoadUnifiedQueue(queuePath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error loading queue: %v\n", err)
		os.Exit(1)
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
	now := time.Now().UTC().Format(time.RFC3339)
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

	// Convert to queue entries, optionally running disambiguation.
	var entries []batch.QueueEntry
	var disambiguated, failed int

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
		fmt.Fprintf(os.Stderr, "Disambiguation: %d resolved, %d failed\n", disambiguated, failed)
	}
}
