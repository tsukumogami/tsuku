package discover

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"github.com/tsukumogami/tsuku/internal/builders"
)

// GenerateConfig holds parameters for registry generation.
type GenerateConfig struct {
	SeedsDir   string
	QueueFile  string
	RecipesDir string
	OutputDir  string
	Validators map[string]Validator
	Probers    map[string]builders.EcosystemProber // builder name → prober for quality metadata
	Verbose    bool
}

// GenerateResult summarizes a generation run.
type GenerateResult struct {
	Total      int
	Graduated  int
	Valid      int
	Probed     int
	Rejected   int
	Failures   []ValidationResult
	Backfills  []BackfillResult
	Rejections []QualityRejection
}

// QualityRejection records an entry rejected by the QualityFilter.
type QualityRejection struct {
	Entry  SeedEntry
	Reason string
}

// Generate runs the full pipeline: load seeds + queue, merge, validate, sort, write.
func Generate(cfg GenerateConfig) (*GenerateResult, error) {
	// Load seed entries from directory
	var seeds []SeedEntry
	if cfg.SeedsDir != "" {
		var err error
		seeds, err = LoadSeedDir(cfg.SeedsDir)
		if err != nil {
			return nil, fmt.Errorf("load seeds: %w", err)
		}
	}

	// Load priority queue entries
	var queueEntries []SeedEntry
	if cfg.QueueFile != "" {
		pq, err := LoadPriorityQueue(cfg.QueueFile)
		if err != nil {
			return nil, fmt.Errorf("load queue: %w", err)
		}
		queueEntries = PriorityQueueToSeedEntries(pq)
	}

	// Merge: seeds override queue entries on name collision
	merged := MergeSeedEntries(queueEntries, seeds)

	if len(merged) == 0 {
		return nil, fmt.Errorf("no entries to process (seeds dir and queue both empty)")
	}

	// Graduate entries that already have recipes
	var graduated int
	var backfills []BackfillResult
	if cfg.RecipesDir != "" {
		gr, err := GraduateEntries(merged, cfg.RecipesDir)
		if err != nil {
			return nil, fmt.Errorf("graduate entries: %w", err)
		}
		graduated = len(gr.Graduated)
		backfills = gr.Backfills
		merged = gr.Kept
	}

	// Validate if validators provided
	var valid []SeedEntry
	var failures []ValidationResult
	if len(cfg.Validators) > 0 {
		valid, failures = ValidateEntries(merged, cfg.Validators)
	} else {
		valid = merged
	}

	// Probe entries for quality metadata and filter out low-quality matches.
	var probed, rejected int
	var rejections []QualityRejection
	if len(cfg.Probers) > 0 {
		valid, probed, rejections = ProbeAndFilter(context.Background(), valid, cfg.Probers, cfg.Verbose)
		rejected = len(rejections)
	}

	// Sort by name for stable output
	sort.Slice(valid, func(i, j int) bool {
		return valid[i].Name < valid[j].Name
	})

	// Write output directory
	if cfg.OutputDir != "" {
		if err := WriteRegistryDir(cfg.OutputDir, valid); err != nil {
			return nil, fmt.Errorf("write output: %w", err)
		}
	}

	return &GenerateResult{
		Total:      len(merged) + graduated,
		Graduated:  graduated,
		Valid:      len(valid),
		Probed:     probed,
		Rejected:   rejected,
		Failures:   failures,
		Backfills:  backfills,
		Rejections: rejections,
	}, nil
}

// ValidateExisting validates an existing discovery registry directory.
func ValidateExisting(dir string, validators map[string]Validator) (*GenerateResult, error) {
	reg, err := LoadRegistryDir(dir)
	if err != nil {
		return nil, fmt.Errorf("load registry: %w", err)
	}

	var entries []SeedEntry
	for name, entry := range reg.Tools {
		entries = append(entries, SeedEntry{
			Name:    name,
			Builder: entry.Builder,
			Source:  entry.Source,
		})
	}

	valid, failures := ValidateEntries(entries, validators)
	return &GenerateResult{
		Total:    len(entries),
		Valid:    len(valid),
		Failures: failures,
	}, nil
}

// ProbeAndFilter calls Probe() on each entry that has a matching prober, enriches
// the entry with quality metadata, and filters out entries that fail the QualityFilter.
// Entries whose builder has no prober are passed through unchanged.
func ProbeAndFilter(ctx context.Context, entries []SeedEntry, probers map[string]builders.EcosystemProber, verbose bool) (accepted []SeedEntry, probed int, rejections []QualityRejection) {
	filter := NewQualityFilter()

	for _, e := range entries {
		prober, ok := probers[e.Builder]
		if !ok {
			// No prober for this builder; pass through.
			accepted = append(accepted, e)
			continue
		}

		result, err := prober.Probe(ctx, e.Source)
		if err != nil || result == nil {
			// Probe failed or returned nil (not found). Pass through without
			// quality metadata rather than rejecting -- the entry was already
			// validated for existence by the Validator step.
			accepted = append(accepted, e)
			continue
		}
		probed++

		// Apply quality filter.
		ok, reason := filter.Accept(e.Builder, result)
		if !ok {
			if verbose {
				fmt.Fprintf(os.Stderr, "  rejected %s (%s/%s): %s\n", e.Name, e.Builder, e.Source, reason)
			}
			rejections = append(rejections, QualityRejection{Entry: e, Reason: reason})
			continue
		}

		// Enrich entry with quality metadata from ProbeResult.
		e.Downloads = result.Downloads
		e.VersionCount = result.VersionCount
		e.HasRepository = result.HasRepository
		accepted = append(accepted, e)
	}
	return accepted, probed, rejections
}

// WriteRegistryDir writes one JSON file per entry into a nested directory tree.
// Path: {dir}/{first-letter}/{first-two-letters}/{name}.json
func WriteRegistryDir(dir string, entries []SeedEntry) error {
	for _, e := range entries {
		entry := RegistryEntry{
			Builder: e.Builder,
			Source:  e.Source,
		}
		if e.Binary != "" {
			entry.Binary = e.Binary
		}
		if e.Description != "" {
			entry.Description = e.Description
		}
		if e.Homepage != "" {
			entry.Homepage = e.Homepage
		}
		if e.Repo != "" {
			entry.Repo = e.Repo
		}
		if e.Disambiguation {
			entry.Disambiguation = true
		}
		if e.Downloads > 0 {
			entry.Downloads = e.Downloads
		}
		if e.VersionCount > 0 {
			entry.VersionCount = e.VersionCount
		}
		if e.HasRepository {
			entry.HasRepository = true
		}

		relPath := RegistryEntryPath(e.Name)
		fullPath := filepath.Join(dir, relPath)
		if err := os.MkdirAll(filepath.Dir(fullPath), 0755); err != nil {
			return fmt.Errorf("create directory for %s: %w", e.Name, err)
		}

		data, err := json.MarshalIndent(entry, "", "  ")
		if err != nil {
			return fmt.Errorf("marshal %s: %w", e.Name, err)
		}
		data = append(data, '\n')
		if err := os.WriteFile(fullPath, data, 0644); err != nil {
			return fmt.Errorf("write %s: %w", e.Name, err)
		}
	}
	return nil
}
