// Package blocker computes transitive blocking impact for dependency graphs.
// It is used by both the pipeline dashboard and the queue reorder tool to
// determine how many packages are blocked (directly and transitively) by a
// given dependency.
package blocker

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// FailureRecord represents one line in failures JSONL. Supports both legacy
// batch format (with failures array) and per-recipe format.
type FailureRecord struct {
	SchemaVersion int              `json:"schema_version"`
	Ecosystem     string           `json:"ecosystem,omitempty"`
	Failures      []PackageFailure `json:"failures,omitempty"`
	// Per-recipe format fields
	Recipe    string   `json:"recipe,omitempty"`
	Category  string   `json:"category,omitempty"`
	BlockedBy []string `json:"blocked_by,omitempty"`
}

// PackageFailure is a single failure entry in the legacy batch format.
type PackageFailure struct {
	PackageID string   `json:"package_id"`
	Category  string   `json:"category"`
	BlockedBy []string `json:"blocked_by,omitempty"`
}

// LoadBlockerMap reads all JSONL files in a directory and builds a map of
// dependency -> list of blocked package IDs, combining data from both legacy
// batch format and per-recipe format.
func LoadBlockerMap(dir string) (map[string][]string, error) {
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
// Uses bufio.Scanner with an increased buffer (1MB max) to handle failure records
// that can exceed the default 64KB line limit for large batches.
func loadBlockersFromFile(path string, blockers map[string][]string) error {
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 0, 64*1024), 1<<20)
	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}

		var record FailureRecord
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

// ComputeTransitiveBlockers computes the total number of packages blocked by a
// dependency, both directly and transitively. Uses memo map with 0-initialization
// for cycle detection: when a dependency is first visited, memo[dep] is set to 0
// (in-progress). If the same dep is encountered again during recursion, the 0 is
// returned, breaking the cycle.
//
// Parameters:
//   - dep: the dependency name to compute blockers for
//   - blockers: map from dependency name to list of blocked package IDs
//   - pkgToBare: map from fully-qualified package ID to bare name
//   - memo: memoization map (shared across calls; caller should create once)
func ComputeTransitiveBlockers(dep string, blockers map[string][]string, pkgToBare map[string]string, memo map[string]int) int {
	if count, ok := memo[dep]; ok {
		return count // 0 if in-progress (cycle)
	}
	// Mark in-progress
	memo[dep] = 0

	// Deduplicate blocked packages for this dependency
	seen := make(map[string]bool)
	total := 0
	for _, pkgID := range blockers[dep] {
		if seen[pkgID] {
			continue
		}
		seen[pkgID] = true
		total++ // Direct dependent
		// Check if this package itself blocks others
		bare := pkgToBare[pkgID]
		if _, isBlocker := blockers[bare]; isBlocker && bare != dep {
			total += ComputeTransitiveBlockers(bare, blockers, pkgToBare, memo)
		}
	}
	memo[dep] = total
	return total
}

// BuildPkgToBare builds a reverse index mapping fully-qualified package IDs
// (e.g., "homebrew:ffmpeg") to their bare names (e.g., "ffmpeg"). This lets
// transitive lookups match blocked package IDs against blocker map keys.
func BuildPkgToBare(blockers map[string][]string) map[string]string {
	pkgToBare := make(map[string]string)
	for _, pkgs := range blockers {
		for _, pkgID := range pkgs {
			if idx := strings.Index(pkgID, ":"); idx >= 0 {
				pkgToBare[pkgID] = pkgID[idx+1:]
			} else {
				pkgToBare[pkgID] = pkgID
			}
		}
	}
	return pkgToBare
}
