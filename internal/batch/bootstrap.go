package batch

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/BurntSushi/toml"
)

// BootstrapConfig holds paths and options for the bootstrap migration.
type BootstrapConfig struct {
	RecipesDir   string // Path to the recipes/ directory
	CuratedPath  string // Path to data/disambiguations/curated.jsonl
	HomebrewPath string // Path to data/queues/priority-queue-homebrew.json
	OutputPath   string // Path to write data/queues/priority-queue.json
}

// BootstrapResult holds counts from each phase of the migration.
type BootstrapResult struct {
	RecipeEntries   int // Entries created from scanning recipes
	CuratedEntries  int // Entries added from curated overrides (not already in queue)
	HomebrewEntries int // Entries added from homebrew queue (not already in queue)
	TotalEntries    int // Total entries in the output queue
}

// UnifiedQueue is the output format for the bootstrapped queue.
type UnifiedQueue struct {
	SchemaVersion int          `json:"schema_version"`
	UpdatedAt     string       `json:"updated_at"`
	Entries       []QueueEntry `json:"entries"`
}

// curatedFile is the JSON structure of data/disambiguations/curated.jsonl.
type curatedFile struct {
	SchemaVersion   int                     `json:"schema_version"`
	Ecosystem       string                  `json:"ecosystem"`
	Environment     string                  `json:"environment"`
	UpdatedAt       string                  `json:"updated_at"`
	Disambiguations []curatedDisambiguation `json:"disambiguations"`
}

type curatedDisambiguation struct {
	Tool            string   `json:"tool"`
	Selected        string   `json:"selected"`
	Alternatives    []string `json:"alternatives"`
	SelectionReason string   `json:"selection_reason"`
	HighRisk        bool     `json:"high_risk"`
}

// homebrewQueue matches the seed.PriorityQueue format used in
// data/queues/priority-queue-homebrew.json.
type homebrewQueue struct {
	SchemaVersion int               `json:"schema_version"`
	UpdatedAt     string            `json:"updated_at"`
	Tiers         map[string]string `json:"tiers,omitempty"`
	Packages      []homebrewPackage `json:"packages"`
}

type homebrewPackage struct {
	ID      string `json:"id"`
	Source  string `json:"source"`
	Name    string `json:"name"`
	Tier    int    `json:"tier"`
	Status  string `json:"status"`
	AddedAt string `json:"added_at"`
}

// recipeMinimal is a minimal TOML recipe structure for source extraction.
// We only need metadata, version, and steps; we skip full validation since
// the recipe loader handles that separately.
type recipeMinimal struct {
	Metadata recipeMetadataMinimal `toml:"metadata"`
	Version  recipeVersionMinimal  `toml:"version"`
	Steps    []toml.Primitive      `toml:"steps"`
}

type recipeMetadataMinimal struct {
	Name string `toml:"name"`
}

type recipeVersionMinimal struct {
	GitHubRepo string `toml:"github_repo"`
}

// recipeStepMinimal holds just the fields we need to determine the source.
type recipeStepMinimal struct {
	Action       string `toml:"action"`
	Formula      string `toml:"formula"`
	Repo         string `toml:"repo"`
	Crate        string `toml:"crate"`
	Package      string `toml:"package"`
	Gem          string `toml:"gem"`
	Module       string `toml:"module"`
	Distribution string `toml:"distribution"`
}

// Bootstrap runs the full migration from existing data to the unified queue.
// It processes three data sources in order:
//  1. Recipes (status: success, confidence: curated)
//  2. Curated overrides (status: pending, confidence: curated, skipping duplicates)
//  3. Homebrew queue (status: pending, confidence: auto, skipping duplicates)
func Bootstrap(cfg BootstrapConfig) (*BootstrapResult, error) {
	seen := make(map[string]bool)
	var entries []QueueEntry

	// Phase 1: Scan recipes
	recipeEntries, err := scanRecipes(cfg.RecipesDir)
	if err != nil {
		return nil, fmt.Errorf("scan recipes: %w", err)
	}
	for _, e := range recipeEntries {
		seen[e.Name] = true
		entries = append(entries, e)
	}

	// Phase 2: Import curated overrides
	curatedCount := 0
	curatedEntries, err := parseCurated(cfg.CuratedPath)
	if err != nil {
		return nil, fmt.Errorf("parse curated: %w", err)
	}
	for _, e := range curatedEntries {
		if !seen[e.Name] {
			seen[e.Name] = true
			entries = append(entries, e)
			curatedCount++
		}
	}

	// Phase 3: Convert homebrew queue
	homebrewCount := 0
	homebrewEntries, err := parseHomebrew(cfg.HomebrewPath)
	if err != nil {
		return nil, fmt.Errorf("parse homebrew: %w", err)
	}
	for _, e := range homebrewEntries {
		if !seen[e.Name] {
			seen[e.Name] = true
			entries = append(entries, e)
			homebrewCount++
		}
	}

	// Sort entries by priority (ascending), then name (alphabetical).
	sort.Slice(entries, func(i, j int) bool {
		if entries[i].Priority != entries[j].Priority {
			return entries[i].Priority < entries[j].Priority
		}
		return entries[i].Name < entries[j].Name
	})

	// Write the output queue
	queue := UnifiedQueue{
		SchemaVersion: 1,
		UpdatedAt:     time.Now().UTC().Format(time.RFC3339),
		Entries:       entries,
	}

	if err := writeQueue(cfg.OutputPath, &queue); err != nil {
		return nil, fmt.Errorf("write queue: %w", err)
	}

	return &BootstrapResult{
		RecipeEntries:   len(recipeEntries),
		CuratedEntries:  curatedCount,
		HomebrewEntries: homebrewCount,
		TotalEntries:    len(entries),
	}, nil
}

// scanRecipes walks the recipes directory and extracts a QueueEntry for each
// recipe. The primary source is determined from the first ecosystem-indicating
// step action in the recipe file.
func scanRecipes(recipesDir string) ([]QueueEntry, error) {
	var entries []QueueEntry

	err := filepath.Walk(recipesDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() || !strings.HasSuffix(path, ".toml") {
			return nil
		}

		entry, err := recipeToEntry(path)
		if err != nil {
			// Log and skip recipes that can't be parsed rather than
			// aborting the entire migration.
			fmt.Fprintf(os.Stderr, "warning: skipping %s: %v\n", path, err)
			return nil
		}
		if entry != nil {
			entries = append(entries, *entry)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return entries, nil
}

// recipeToEntry parses a recipe TOML file and creates a QueueEntry.
// Returns nil if no source can be extracted from the recipe steps.
func recipeToEntry(path string) (*QueueEntry, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read file: %w", err)
	}

	var meta toml.MetaData
	var raw recipeMinimal
	meta, err = toml.Decode(string(data), &raw)
	if err != nil {
		return nil, fmt.Errorf("parse TOML: %w", err)
	}

	name := raw.Metadata.Name
	if name == "" {
		return nil, fmt.Errorf("recipe has no name")
	}

	// Extract source from the first ecosystem-indicating step
	source := ""
	for _, prim := range raw.Steps {
		var step recipeStepMinimal
		if err := meta.PrimitiveDecode(prim, &step); err != nil {
			continue
		}
		source = sourceFromStep(step)
		if source != "" {
			break
		}
	}

	// Fallback: if no ecosystem step found, use the version section's github_repo.
	// This covers recipes that use generic download/extract steps but resolve
	// versions from GitHub releases (e.g., HashiCorp tools, Go SDK).
	if source == "" && raw.Version.GitHubRepo != "" {
		source = "github:" + raw.Version.GitHubRepo
	}

	if source == "" {
		return nil, fmt.Errorf("no source found in steps")
	}

	return &QueueEntry{
		Name:       name,
		Source:     source,
		Priority:   3, // Default; will be overridden by homebrew data if available
		Status:     StatusSuccess,
		Confidence: ConfidenceCurated,
	}, nil
}

// sourceFromStep extracts the ecosystem:identifier source string from a recipe step.
func sourceFromStep(step recipeStepMinimal) string {
	switch step.Action {
	case "homebrew":
		if step.Formula != "" {
			return "homebrew:" + step.Formula
		}
	case "github_archive", "github_file":
		if step.Repo != "" {
			return "github:" + step.Repo
		}
	case "cargo_install":
		if step.Crate != "" {
			return "cargo:" + step.Crate
		}
	case "npm_install":
		if step.Package != "" {
			return "npm:" + step.Package
		}
	case "pipx_install":
		if step.Package != "" {
			return "pypi:" + step.Package
		}
	case "gem_install":
		if step.Gem != "" {
			return "rubygems:" + step.Gem
		}
	case "go_install":
		if step.Module != "" {
			return "go:" + step.Module
		}
	case "nix_install":
		if step.Package != "" {
			return "nix:" + step.Package
		}
	case "cpan_install":
		if step.Distribution != "" {
			return "cpan:" + step.Distribution
		}
	case "download_archive":
		// download_archive with a repo field indicates a GitHub source
		if step.Repo != "" {
			return "github:" + step.Repo
		}
	}
	return ""
}

// parseCurated reads the curated overrides file and returns QueueEntry values
// for each disambiguation. These entries are pending (not yet processed).
func parseCurated(path string) ([]QueueEntry, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read file: %w", err)
	}

	var cf curatedFile
	if err := json.Unmarshal(data, &cf); err != nil {
		return nil, fmt.Errorf("parse JSON: %w", err)
	}

	var entries []QueueEntry
	for _, d := range cf.Disambiguations {
		if d.Tool == "" || d.Selected == "" {
			continue
		}
		entries = append(entries, QueueEntry{
			Name:       d.Tool,
			Source:     d.Selected,
			Priority:   1, // Curated overrides are high-priority tools
			Status:     StatusPending,
			Confidence: ConfidenceCurated,
		})
	}
	return entries, nil
}

// parseHomebrew reads the homebrew priority queue and converts each package
// to the unified QueueEntry format.
func parseHomebrew(path string) ([]QueueEntry, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read file: %w", err)
	}

	var hq homebrewQueue
	if err := json.Unmarshal(data, &hq); err != nil {
		return nil, fmt.Errorf("parse JSON: %w", err)
	}

	var entries []QueueEntry
	for _, pkg := range hq.Packages {
		priority := pkg.Tier
		if priority < 1 || priority > 3 {
			priority = 3
		}

		entries = append(entries, QueueEntry{
			Name:       pkg.Name,
			Source:     "homebrew:" + pkg.Name,
			Priority:   priority,
			Status:     StatusPending,
			Confidence: ConfidenceAuto,
		})
	}
	return entries, nil
}

// writeQueue serializes the unified queue to JSON and writes it to disk.
func writeQueue(path string, queue *UnifiedQueue) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("create directory: %w", err)
	}

	data, err := json.MarshalIndent(queue, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal JSON: %w", err)
	}
	data = append(data, '\n')

	return os.WriteFile(path, data, 0644)
}
