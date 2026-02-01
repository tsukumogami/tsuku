package discover

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"
)

// GenerateConfig holds parameters for registry generation.
type GenerateConfig struct {
	SeedsDir   string
	QueueFile  string
	RecipesDir string
	Output     string
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

	// Build registry
	reg := buildRegistry(valid)

	// Write output
	if cfg.Output != "" {
		if err := writeRegistry(cfg.Output, reg); err != nil {
			return nil, fmt.Errorf("write output: %w", err)
		}
	}

	return &GenerateResult{
		Total:    len(merged),
		Valid:    len(valid),
		Failures: failures,
	}, nil
}

// ValidateExisting validates an existing discovery.json file.
func ValidateExisting(path string, validators map[string]Validator) (*GenerateResult, error) {
	reg, err := LoadRegistry(path)
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

func buildRegistry(entries []SeedEntry) *registryFile {
	tools := make(map[string]RegistryEntry, len(entries))
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
		tools[e.Name] = entry
	}
	return &registryFile{
		SchemaVersion: 1,
		Tools:         tools,
	}
}

// registryFile is the on-disk format for discovery.json.
type registryFile struct {
	SchemaVersion int                      `json:"schema_version"`
	Tools         map[string]RegistryEntry `json:"tools"`
}

func writeRegistry(path string, reg *registryFile) error {
	data, err := json.MarshalIndent(reg, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	return os.WriteFile(path, data, 0644)
}
