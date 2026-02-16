package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/tsukumogami/tsuku/internal/batch"
	"github.com/tsukumogami/tsuku/internal/builders"
	"github.com/tsukumogami/tsuku/internal/discover"
	"github.com/tsukumogami/tsuku/internal/seed"
)

// validSources is the set of valid -source flag values.
var validSources = map[string]bool{
	"homebrew": true,
	"cargo":    true,
	"npm":      true,
	"pypi":     true,
	"rubygems": true,
	"all":      true,
}

// allDiscoverySources lists the sources run when -source all is specified.
// Homebrew uses the existing HomebrewSource (seed.Source interface), not
// EcosystemDiscoverer, because its analytics endpoint has a different shape.
var allDiscoverySources = []string{"homebrew", "cargo", "npm", "pypi", "rubygems"}

// seedingRunsPath is the default path for the seeding run history file.
const seedingRunsPath = "data/metrics/seeding-runs.jsonl"

// config holds the parsed command-line flags.
type config struct {
	source       string
	limit        int
	queuePath    string
	recipesDir   string
	embeddedDir  string
	disambiguate bool
	freshness    int
	auditDir     string
	dryRun       bool
	verbose      bool
}

func main() {
	os.Exit(run(os.Args[1:]))
}

// run parses flags, executes the seeding logic, writes the summary JSON to
// stdout, and returns the exit code. Extracted from main() for testability.
func run(args []string) int {
	cfg, err := parseFlags(args)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		return 1
	}

	return execute(cfg)
}

// parseFlags parses command-line flags and returns a validated config.
func parseFlags(args []string) (*config, error) {
	fs := flag.NewFlagSet("seed-queue", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)

	cfg := &config{}
	fs.StringVar(&cfg.source, "source", "all", "ecosystem source (homebrew|cargo|npm|pypi|rubygems|all)")
	fs.IntVar(&cfg.limit, "limit", 500, "max packages per source")
	fs.StringVar(&cfg.queuePath, "queue", "data/queues/priority-queue.json", "path to unified priority queue")
	fs.StringVar(&cfg.recipesDir, "recipes-dir", "recipes", "path to recipes directory")
	fs.StringVar(&cfg.embeddedDir, "embedded-dir", "", "path to embedded recipes directory")
	fs.BoolVar(&cfg.disambiguate, "disambiguate", true, "run disambiguation for new packages")
	fs.IntVar(&cfg.freshness, "freshness", 30, "re-disambiguate entries older than N days")
	fs.StringVar(&cfg.auditDir, "audit-dir", "data/disambiguations/audit", "path to audit log directory")
	fs.BoolVar(&cfg.dryRun, "dry-run", false, "print changes without writing any files")
	fs.BoolVar(&cfg.verbose, "verbose", false, "print per-package progress to stderr")

	if err := fs.Parse(args); err != nil {
		return nil, err
	}

	if !validSources[cfg.source] {
		return nil, fmt.Errorf("invalid -source %q; must be one of: homebrew, cargo, npm, pypi, rubygems, all", cfg.source)
	}

	if cfg.limit < 0 {
		return nil, fmt.Errorf("invalid -limit %d; must be >= 0", cfg.limit)
	}

	if cfg.freshness < 0 {
		return nil, fmt.Errorf("invalid -freshness %d; must be >= 0", cfg.freshness)
	}

	return cfg, nil
}

// execute runs the seeding logic and returns the exit code (0, 1, or 2).
func execute(cfg *config) int {
	ctx := context.Background()
	seedingRun := time.Now().UTC()
	summary := seed.NewSeedingSummary()

	// Load unified queue.
	unifiedQueue, err := batch.LoadUnifiedQueue(cfg.queuePath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error loading queue: %v\n", err)
		summary.Errors = append(summary.Errors, fmt.Sprintf("load queue: %v", err))
		writeSummary(summary)
		return 1
	}

	// Build a name->source lookup for detecting re-disambiguations.
	existingSources := make(map[string]string, len(unifiedQueue.Entries))
	for _, e := range unifiedQueue.Entries {
		existingSources[e.Name] = e.Source
	}

	// Resolve absolute audit directory path.
	absAuditDir := cfg.auditDir
	if !filepath.IsAbs(absAuditDir) {
		absAuditDir, _ = filepath.Abs(absAuditDir)
	}

	// Determine which sources to process.
	sources := []string{cfg.source}
	if cfg.source == "all" {
		sources = allDiscoverySources
	}

	// Build all ecosystem builders once, shared between discovery and disambiguation.
	cargoBuilder := builders.NewCargoBuilder(nil)
	npmBuilder := builders.NewNpmBuilder(nil)
	pypiBuilder := builders.NewPyPIBuilder(nil)
	gemBuilder := builders.NewGemBuilder(nil)

	// Map source names to their discoverer-capable builders.
	discoverers := map[string]builders.EcosystemDiscoverer{
		"cargo":    cargoBuilder,
		"npm":      npmBuilder,
		"pypi":     pypiBuilder,
		"rubygems": gemBuilder,
	}

	// Build disambiguator with all probers (for disambiguation across ecosystems).
	var d *seed.Disambiguator
	if cfg.disambiguate {
		allProbers := []builders.EcosystemProber{
			cargoBuilder,
			npmBuilder,
			pypiBuilder,
			gemBuilder,
			builders.NewHomebrewBuilder(),
			builders.NewCaskBuilder(nil),
			builders.NewGoBuilder(nil),
			builders.NewCPANBuilder(nil),
		}
		d = seed.NewDisambiguator(allProbers, 30*time.Second)
	}

	// Track all discovered names across sources for trigger 3 (new audit candidate).
	discoveredNames := make(map[string]string)

	// --- Process each source ---
	for _, src := range sources {
		var srcErr error

		if src == "homebrew" {
			srcErr = processHomebrew(ctx, cfg, unifiedQueue, existingSources, absAuditDir, d, discoveredNames, summary, seedingRun)
		} else {
			disc, ok := discoverers[src]
			if !ok {
				summary.SourcesFailed = append(summary.SourcesFailed, src)
				summary.Errors = append(summary.Errors, fmt.Sprintf("%s: unknown discoverer", src))
				continue
			}
			srcErr = processDiscoverer(ctx, cfg, src, disc, unifiedQueue, existingSources, absAuditDir, d, discoveredNames, summary, seedingRun)
		}

		if srcErr != nil {
			summary.SourcesFailed = append(summary.SourcesFailed, src)
			summary.Errors = append(summary.Errors, fmt.Sprintf("%s: %v", src, srcErr))
		} else {
			summary.SourcesProcessed = append(summary.SourcesProcessed, src)
		}
	}

	// --- Freshness checking on existing entries ---
	freshnessCfg := seed.FreshnessConfig{
		ThresholdDays: cfg.freshness,
		Now:           seedingRun,
	}
	curatedValidator := &seed.CuratedSourceValidator{}

	for i := range unifiedQueue.Entries {
		entry := &unifiedQueue.Entries[i]

		if seed.ShouldSkip(*entry) {
			continue
		}

		if seed.IsCurated(*entry) {
			summary.CuratedSkipped++
			if err := curatedValidator.Validate(entry.Source); err != nil {
				summary.CuratedInvalid = append(summary.CuratedInvalid, seed.CuratedInvalid{
					Package: entry.Name,
					Source:  entry.Source,
					Error:   err.Error(),
				})
			}
			continue
		}

		var auditEntry *seed.AuditEntry
		auditEntry, _ = seed.ReadAuditEntry(absAuditDir, entry.Name)

		discoveredSource := discoveredNames[entry.Name]

		if !seed.NeedsRedisambiguation(*entry, freshnessCfg, auditEntry, discoveredSource) {
			continue
		}

		if d == nil {
			seed.UpdateDisambiguatedAt(entry, seedingRun)
			summary.StaleRefreshed++
			continue
		}

		if cfg.verbose {
			fmt.Fprintf(os.Stderr, "  freshness: re-disambiguating %s\n", entry.Name)
		}

		rr, resolveErr := d.Resolve(ctx, entry.Name)
		if resolveErr != nil {
			if cfg.verbose {
				fmt.Fprintf(os.Stderr, "  freshness error for %s: %v\n", entry.Name, resolveErr)
			}
			continue
		}

		if rr.Selected == nil {
			seed.UpdateDisambiguatedAt(entry, seedingRun)
			summary.StaleRefreshed++
			continue
		}

		newSource := rr.Selected.Builder + ":" + rr.Selected.Source
		selectionReason := rr.Selected.Metadata.SelectionReason

		seed.ApplySelectionResult(entry, selectionReason)

		if newSource != entry.Source {
			change, modified := seed.ApplySourceChange(entry, newSource, seedingRun)
			summary.SourceChanges = append(summary.SourceChanges, change)
			if cfg.verbose {
				if modified {
					fmt.Fprintf(os.Stderr, "  source change (auto-accepted): %s -> %s for %s\n",
						change.Old, change.New, entry.Name)
				} else {
					fmt.Fprintf(os.Stderr, "  source change (flagged): %s -> %s for %s (priority %d)\n",
						change.Old, change.New, entry.Name, entry.Priority)
				}
			}
		}

		seed.UpdateDisambiguatedAt(entry, seedingRun)
		summary.StaleRefreshed++

		if !cfg.dryRun {
			newAuditEntry := buildAuditEntry(entry.Name, rr, existingSources, seedingRun)
			if selectionReason == discover.SelectionPriorityFallback {
				newAuditEntry.HighRisk = true
			}
			if writeErr := seed.WriteAuditEntry(absAuditDir, newAuditEntry); writeErr != nil {
				if cfg.verbose {
					fmt.Fprintf(os.Stderr, "  audit write error for %s: %v\n", entry.Name, writeErr)
				}
			}
		}
	}

	// --- Write outputs (unless dry-run) ---
	if !cfg.dryRun {
		if err := batch.SaveUnifiedQueue(cfg.queuePath, unifiedQueue); err != nil {
			fmt.Fprintf(os.Stderr, "error saving queue: %v\n", err)
			summary.Errors = append(summary.Errors, fmt.Sprintf("save queue: %v", err))
			writeSummary(summary)
			return 1
		}

		if err := seed.AppendSeedingRun(seedingRunsPath, summary.ToRunEntry(seedingRun)); err != nil {
			fmt.Fprintf(os.Stderr, "error appending seeding run: %v\n", err)
			// Non-fatal: the queue was already saved.
			summary.Errors = append(summary.Errors, fmt.Sprintf("append seeding run: %v", err))
		}
	}

	// Write summary JSON to stdout.
	writeSummary(summary)

	// Exit code selection: 0=success, 1=fatal, 2=partial failure.
	if len(summary.SourcesFailed) > 0 {
		if len(summary.SourcesProcessed) > 0 {
			return 2 // Some succeeded, some failed.
		}
		return 1 // All sources failed.
	}
	return 0
}

// processHomebrew fetches from Homebrew analytics and merges into the queue.
func processHomebrew(ctx context.Context, cfg *config, queue *batch.UnifiedQueue, existingSources map[string]string, auditDir string, d *seed.Disambiguator, discoveredNames map[string]string, summary *seed.SeedingSummary, seedingRun time.Time) error {
	_ = ctx // Homebrew source doesn't use context currently.

	src := &seed.HomebrewSource{}
	packages, err := src.Fetch(cfg.limit)
	if err != nil {
		return fmt.Errorf("fetch: %v", err)
	}

	if cfg.verbose {
		fmt.Fprintf(os.Stderr, "  homebrew: discovered %d candidates\n", len(packages))
	}

	// Track discovered names for trigger 3.
	for _, p := range packages {
		discoveredNames[p.Name] = "homebrew:" + p.Name
	}

	// Filter packages with existing recipes.
	if cfg.recipesDir != "" || cfg.embeddedDir != "" {
		var skipped []string
		packages, skipped = seed.FilterExistingRecipes(packages, cfg.recipesDir, cfg.embeddedDir)
		if cfg.verbose && len(skipped) > 0 {
			fmt.Fprintf(os.Stderr, "  homebrew: filtered %d packages with existing recipes\n", len(skipped))
		}
	}

	// Filter packages already in the queue (name-based).
	packages = seed.FilterByName(packages, queue)

	if cfg.verbose {
		fmt.Fprintf(os.Stderr, "  homebrew: %d new packages after filtering\n", len(packages))
	}

	added := mergeNewPackages(ctx, cfg, packages, "homebrew", queue, existingSources, auditDir, d, summary, seedingRun)
	summary.NewPackages += added

	return nil
}

// processDiscoverer runs discovery, filtering, and disambiguation for a
// builder that implements EcosystemDiscoverer.
func processDiscoverer(ctx context.Context, cfg *config, sourceName string, disc builders.EcosystemDiscoverer, queue *batch.UnifiedQueue, existingSources map[string]string, auditDir string, d *seed.Disambiguator, discoveredNames map[string]string, summary *seed.SeedingSummary, seedingRun time.Time) error {
	candidates, err := disc.Discover(ctx, cfg.limit)
	if err != nil {
		return fmt.Errorf("discover: %v", err)
	}

	if cfg.verbose {
		fmt.Fprintf(os.Stderr, "  %s: discovered %d candidates\n", sourceName, len(candidates))
	}

	// Track discovered names for trigger 3.
	for _, c := range candidates {
		discoveredNames[c.Name] = sourceName + ":" + c.Name
	}

	// Convert to seed.Package for filtering.
	now := seedingRun.Format(time.RFC3339)
	packages := make([]seed.Package, 0, len(candidates))
	for _, c := range candidates {
		tier := seed.AssignTier(c.Name, c.Downloads, sourceName)
		packages = append(packages, seed.Package{
			ID:      sourceName + ":" + c.Name,
			Source:  sourceName + ":" + c.Name,
			Name:    c.Name,
			Tier:    tier,
			Status:  "pending",
			AddedAt: now,
		})
	}

	// Filter packages with existing recipes.
	if cfg.recipesDir != "" || cfg.embeddedDir != "" {
		var skipped []string
		packages, skipped = seed.FilterExistingRecipes(packages, cfg.recipesDir, cfg.embeddedDir)
		if cfg.verbose && len(skipped) > 0 {
			fmt.Fprintf(os.Stderr, "  %s: filtered %d packages with existing recipes\n", sourceName, len(skipped))
		}
	}

	// Filter packages already in the queue (name-based).
	packages = seed.FilterByName(packages, queue)

	if cfg.verbose {
		fmt.Fprintf(os.Stderr, "  %s: %d new packages after filtering\n", sourceName, len(packages))
	}

	added := mergeNewPackages(ctx, cfg, packages, sourceName, queue, existingSources, auditDir, d, summary, seedingRun)
	summary.NewPackages += added

	return nil
}

// mergeNewPackages disambiguates (if enabled) and merges new packages into
// the unified queue. Returns the count of actually added entries.
func mergeNewPackages(ctx context.Context, cfg *config, packages []seed.Package, sourceName string, queue *batch.UnifiedQueue, existingSources map[string]string, auditDir string, d *seed.Disambiguator, summary *seed.SeedingSummary, seedingRun time.Time) int {
	var entries []batch.QueueEntry

	if d != nil {
		for _, pkg := range packages {
			if cfg.verbose {
				fmt.Fprintf(os.Stderr, "  %s: disambiguating %s\n", sourceName, pkg.Name)
			}

			rr, err := d.Resolve(ctx, pkg.Name)
			if err != nil {
				if cfg.verbose {
					fmt.Fprintf(os.Stderr, "  %s: disambiguation error for %s: %v\n", sourceName, pkg.Name, err)
				}
				entries = append(entries, seed.ToQueueEntry(pkg, nil))
				continue
			}
			entries = append(entries, seed.ToQueueEntry(pkg, rr.Selected))

			if rr.Selected != nil && !cfg.dryRun {
				auditEntry := buildAuditEntry(pkg.Name, rr, existingSources, seedingRun)
				if writeErr := seed.WriteAuditEntry(auditDir, auditEntry); writeErr != nil {
					if cfg.verbose {
						fmt.Fprintf(os.Stderr, "  %s: audit write error for %s: %v\n", sourceName, pkg.Name, writeErr)
					}
				}
			}
		}
	} else {
		for _, pkg := range packages {
			entries = append(entries, seed.ToQueueEntry(pkg, nil))
		}
	}

	// Merge into queue (name-based dedup).
	existingNames := make(map[string]bool, len(queue.Entries))
	for _, e := range queue.Entries {
		existingNames[strings.ToLower(e.Name)] = true
	}

	added := 0
	for _, entry := range entries {
		if !existingNames[strings.ToLower(entry.Name)] {
			queue.Entries = append(queue.Entries, entry)
			existingNames[strings.ToLower(entry.Name)] = true
			added++
		}
	}

	return added
}

// writeSummary writes the summary JSON to stdout.
func writeSummary(summary *seed.SeedingSummary) {
	data, err := json.Marshal(summary)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error marshaling summary: %v\n", err)
		return
	}
	_, _ = fmt.Fprintln(os.Stdout, string(data))
}

// buildAuditEntry constructs an AuditEntry from a disambiguation result.
// If the package already existed in the queue (re-disambiguation),
// previous_source is set to its current source.
func buildAuditEntry(name string, rr *discover.ResolveResult, existingSources map[string]string, seedingRun time.Time) seed.AuditEntry {
	now := time.Now().UTC()

	selected := rr.Selected
	record := batch.DisambiguationRecord{
		Tool:            name,
		Selected:        selected.Builder + ":" + selected.Source,
		SelectionReason: selected.Metadata.SelectionReason,
		DownloadsRatio:  selected.Metadata.DownloadsRatio,
		HighRisk:        selected.Metadata.SelectionReason == discover.SelectionPriorityFallback,
	}

	for _, alt := range selected.Metadata.Alternatives {
		record.Alternatives = append(record.Alternatives, alt.Builder+":"+alt.Source)
	}

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
