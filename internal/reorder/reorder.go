// Package reorder re-orders priority queue entries within each tier based on
// transitive blocking impact. Entries that unblock the most other packages
// are moved earlier within their tier, so the batch pipeline processes
// high-leverage recipes first.
package reorder

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"github.com/tsukumogami/tsuku/internal/batch"
	"github.com/tsukumogami/tsuku/internal/blocker"
)

// Options configures the reorder operation.
type Options struct {
	QueueFile   string // Path to unified priority-queue.json
	FailuresDir string // Directory containing failures JSONL files
	OutputFile  string // Path to write reordered queue (empty = overwrite QueueFile)
	DryRun      bool   // If true, print changes without writing
}

// Result summarizes what the reorder operation did.
type Result struct {
	TotalEntries int            // Total entries in queue
	Reordered    int            // Entries whose position changed
	ByTier       map[int]int    // Entries per tier
	TopScores    []ScoredEntry  // Top entries by blocking score (up to 10)
	EntriesMoved map[int][]Move // Per-tier list of position changes
}

// ScoredEntry pairs an entry name with its blocking impact score.
type ScoredEntry struct {
	Name  string `json:"name"`
	Score int    `json:"score"`
	Tier  int    `json:"tier"`
}

// Move records a position change for a single entry within its tier.
type Move struct {
	Name string `json:"name"`
	From int    `json:"from"` // 0-based position within tier before reorder
	To   int    `json:"to"`   // 0-based position within tier after reorder
}

// failureRecord represents one line in failures JSONL. Supports both legacy
// batch format (with failures array) and per-recipe format.
type failureRecord struct {
	SchemaVersion int              `json:"schema_version"`
	Ecosystem     string           `json:"ecosystem,omitempty"`
	Failures      []packageFailure `json:"failures,omitempty"`
	// Per-recipe format fields
	Recipe    string   `json:"recipe,omitempty"`
	Category  string   `json:"category,omitempty"`
	BlockedBy []string `json:"blocked_by,omitempty"`
}

// packageFailure is a single failure entry in the legacy batch format.
type packageFailure struct {
	PackageID string   `json:"package_id"`
	Category  string   `json:"category"`
	BlockedBy []string `json:"blocked_by,omitempty"`
}

// Run loads the queue and failure data, computes blocking scores, reorders
// entries within each tier by descending score, and writes the result.
func Run(opts Options) (*Result, error) {
	queue, err := batch.LoadUnifiedQueue(opts.QueueFile)
	if err != nil {
		return nil, fmt.Errorf("load queue: %w", err)
	}

	if len(queue.Entries) == 0 {
		return &Result{ByTier: map[int]int{}, EntriesMoved: map[int][]Move{}}, nil
	}

	// Build blocker map from failure data
	blockers, err := loadBlockerMap(opts.FailuresDir)
	if err != nil {
		// Non-fatal: if no failure data exists, all scores are 0 and
		// the queue retains its alphabetical ordering within tiers.
		blockers = make(map[string][]string)
	}

	// Compute blocking scores for each entry
	scores := computeScores(queue.Entries, blockers)

	// Snapshot original positions by tier for diffing
	origByTier := groupByTier(queue.Entries)

	// Sort entries: by tier ascending, then by score descending, then alphabetical
	sort.SliceStable(queue.Entries, func(i, j int) bool {
		if queue.Entries[i].Priority != queue.Entries[j].Priority {
			return queue.Entries[i].Priority < queue.Entries[j].Priority
		}
		si := scores[queue.Entries[i].Name]
		sj := scores[queue.Entries[j].Name]
		if si != sj {
			return si > sj // Higher score first
		}
		return queue.Entries[i].Name < queue.Entries[j].Name
	})

	// Compute result
	newByTier := groupByTier(queue.Entries)
	result := buildResult(queue.Entries, scores, origByTier, newByTier)

	if opts.DryRun {
		return result, nil
	}

	// Write output
	outputPath := opts.OutputFile
	if outputPath == "" {
		outputPath = opts.QueueFile
	}
	if err := batch.SaveUnifiedQueue(outputPath, queue); err != nil {
		return nil, fmt.Errorf("save queue: %w", err)
	}

	return result, nil
}

// computeScores computes the transitive blocking impact score for each entry.
// The score for an entry is the total number of packages that are transitively
// blocked by that entry's name appearing in blocked_by fields.
func computeScores(entries []batch.QueueEntry, blockers map[string][]string) map[string]int {
	pkgToBare := blocker.BuildPkgToBare(blockers)
	memo := make(map[string]int)
	scores := make(map[string]int, len(entries))
	for _, e := range entries {
		if _, isBlocker := blockers[e.Name]; isBlocker {
			scores[e.Name] = blocker.ComputeTransitiveBlockers(e.Name, blockers, pkgToBare, memo)
		}
	}

	return scores
}

// loadBlockerMap reads all JSONL files in a directory and builds a map of
// dependency -> list of blocked package IDs, combining data from both legacy
// batch format and per-recipe format.
func loadBlockerMap(dir string) (map[string][]string, error) {
	pattern := filepath.Join(dir, "*.jsonl")
	files, err := filepath.Glob(pattern)
	if err != nil {
		return nil, fmt.Errorf("glob failures: %w", err)
	}

	if len(files) == 0 {
		return nil, fmt.Errorf("no failure files found in %s", dir)
	}

	blockers := make(map[string][]string)
	for _, path := range files {
		if err := loadBlockersFromFile(path, blockers); err != nil {
			continue // Skip files that can't be read
		}
	}
	return blockers, nil
}

// loadBlockersFromFile reads a single JSONL file and populates the blocker map.
func loadBlockersFromFile(path string, blockers map[string][]string) error {
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}

		var record failureRecord
		if err := json.Unmarshal(line, &record); err != nil {
			continue // Skip malformed lines
		}

		// Handle legacy batch format with failures array
		for _, f := range record.Failures {
			for _, dep := range f.BlockedBy {
				blockers[dep] = append(blockers[dep], f.PackageID)
			}
		}

		// Handle per-recipe format
		if record.Recipe != "" && len(record.BlockedBy) > 0 {
			eco := record.Ecosystem
			if eco == "" {
				eco = "homebrew"
			}
			pkgID := eco + ":" + record.Recipe
			for _, dep := range record.BlockedBy {
				blockers[dep] = append(blockers[dep], pkgID)
			}
		}
	}

	return scanner.Err()
}

// groupByTier returns a map from tier to the ordered list of entry names.
func groupByTier(entries []batch.QueueEntry) map[int][]string {
	result := make(map[int][]string)
	for _, e := range entries {
		result[e.Priority] = append(result[e.Priority], e.Name)
	}
	return result
}

// buildResult computes the Result struct by comparing original and new tier orderings.
func buildResult(entries []batch.QueueEntry, scores map[string]int, origByTier, newByTier map[int][]string) *Result {
	result := &Result{
		TotalEntries: len(entries),
		ByTier:       make(map[int]int),
		EntriesMoved: make(map[int][]Move),
	}

	// Count entries per tier
	for tier, names := range newByTier {
		result.ByTier[tier] = len(names)
	}

	// Compute position changes per tier
	reordered := 0
	for tier, origNames := range origByTier {
		newNames := newByTier[tier]
		origPos := make(map[string]int, len(origNames))
		for i, name := range origNames {
			origPos[name] = i
		}

		for newIdx, name := range newNames {
			oldIdx := origPos[name]
			if oldIdx != newIdx {
				result.EntriesMoved[tier] = append(result.EntriesMoved[tier], Move{
					Name: name,
					From: oldIdx,
					To:   newIdx,
				})
				reordered++
			}
		}
	}
	result.Reordered = reordered

	// Top scores (up to 10)
	type scoredEntry struct {
		name  string
		score int
		tier  int
	}
	var allScored []scoredEntry
	for _, e := range entries {
		if s := scores[e.Name]; s > 0 {
			allScored = append(allScored, scoredEntry{name: e.Name, score: s, tier: e.Priority})
		}
	}
	// Deduplicate (same name can appear once)
	seen := make(map[string]bool)
	var unique []scoredEntry
	for _, s := range allScored {
		if !seen[s.name] {
			seen[s.name] = true
			unique = append(unique, s)
		}
	}
	sort.Slice(unique, func(i, j int) bool {
		if unique[i].score != unique[j].score {
			return unique[i].score > unique[j].score
		}
		return unique[i].name < unique[j].name
	})
	limit := 10
	if len(unique) < limit {
		limit = len(unique)
	}
	result.TopScores = make([]ScoredEntry, limit)
	for i := 0; i < limit; i++ {
		result.TopScores[i] = ScoredEntry{
			Name:  unique[i].name,
			Score: unique[i].score,
			Tier:  unique[i].tier,
		}
	}

	return result
}
