package discover

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
)

// GenerateConfig holds parameters for registry generation.
type GenerateConfig struct {
	SeedsDir   string
	QueueFile  string
	RecipesDir string
	OutputDir  string
	Validators map[string]Validator
	Verbose    bool
}

// GenerateResult summarizes a generation run.
type GenerateResult struct {
	Total    int
	Valid    int
	Failures []ValidationResult
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

	// Validate if validators provided
	var valid []SeedEntry
	var failures []ValidationResult
	if len(cfg.Validators) > 0 {
		valid, failures = ValidateEntries(merged, cfg.Validators)
	} else {
		valid = merged
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
		Total:    len(merged),
		Valid:    len(valid),
		Failures: failures,
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
		if e.Repo != "" {
			entry.Repo = e.Repo
		}
		if e.Disambiguation {
			entry.Disambiguation = true
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
