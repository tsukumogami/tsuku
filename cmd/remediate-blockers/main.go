// Command remediate-blockers patches misclassified failure records in
// data/failures/*.jsonl files and updates the corresponding queue entries.
//
// Many legacy failure records have category "validation_failed" with empty
// blocked_by, despite containing "recipe X not found in registry" in their
// message text. This tool extracts the dependency names using the same regex
// pattern as the orchestrator (internal/batch/orchestrator.go), patches the
// records in place, flips corresponding queue entries from "failed" to
// "blocked", and regenerates the pipeline dashboard.
//
// The tool is idempotent: running it twice produces no changes on the second
// run.
package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/tsukumogami/tsuku/internal/batch"
	"github.com/tsukumogami/tsuku/internal/dashboard"
)

// reNotFoundInRegistry matches "recipe X not found in registry" in error
// messages. This is the same pattern as reNotFoundInRegistry in
// internal/batch/orchestrator.go and cmd/tsuku/install.go.
var reNotFoundInRegistry = regexp.MustCompile(`recipe (\S+) not found in registry`)

// remediationStats tracks what the tool changed, for the summary report.
type remediationStats struct {
	FilesScanned   int
	RecordsScanned int
	RecordsUpdated int
	QueueFlipped   int
	UniqueDeps     map[string]bool
	RemediatedPkgs map[string]bool // package IDs that were remediated
}

func main() {
	failuresDir := "data/failures"
	queuePath := "data/queues/priority-queue.json"

	stats := &remediationStats{
		UniqueDeps:     make(map[string]bool),
		RemediatedPkgs: make(map[string]bool),
	}

	// Phase 1: Scan and patch failure JSONL files.
	if err := remediateFailures(failuresDir, stats); err != nil {
		fmt.Fprintf(os.Stderr, "error remediating failures: %v\n", err)
		os.Exit(1)
	}

	// Phase 2: Update queue entries from "failed" to "blocked".
	if err := remediateQueue(queuePath, stats); err != nil {
		fmt.Fprintf(os.Stderr, "error remediating queue: %v\n", err)
		os.Exit(1)
	}

	// Phase 3: Regenerate dashboard.
	if err := regenerateDashboard(); err != nil {
		fmt.Fprintf(os.Stderr, "error regenerating dashboard: %v\n", err)
		os.Exit(1)
	}

	// Print summary report.
	depNames := sortedKeys(stats.UniqueDeps)
	fmt.Printf("Remediation summary:\n")
	fmt.Printf("  files scanned:    %d\n", stats.FilesScanned)
	fmt.Printf("  records scanned:  %d\n", stats.RecordsScanned)
	fmt.Printf("  records updated:  %d\n", stats.RecordsUpdated)
	fmt.Printf("  queue flipped:    %d\n", stats.QueueFlipped)
	fmt.Printf("  dependency names: %d\n", len(depNames))
	if len(depNames) > 0 {
		fmt.Printf("  dependencies:     %s\n", strings.Join(depNames, ", "))
	}
}

// remediateFailures scans all JSONL files in dir and patches records that
// have dependency-related messages but wrong classification.
func remediateFailures(dir string, stats *remediationStats) error {
	pattern := filepath.Join(dir, "*.jsonl")
	files, err := filepath.Glob(pattern)
	if err != nil {
		return fmt.Errorf("glob failures: %w", err)
	}

	sort.Strings(files)

	for _, path := range files {
		if err := remediateFile(path, stats); err != nil {
			return fmt.Errorf("remediate %s: %w", path, err)
		}
	}

	return nil
}

// remediateFile processes a single JSONL file, patching records in place.
// It reads all lines, patches those that need it, and writes the file back
// only if changes were made.
func remediateFile(path string, stats *remediationStats) error {
	stats.FilesScanned++

	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read file: %w", err)
	}

	scanner := bufio.NewScanner(strings.NewReader(string(data)))
	// Increase buffer for large legacy format lines.
	buf := make([]byte, 0, 64*1024)
	scanner.Buffer(buf, 1024*1024)

	var lines []string
	changed := false

	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			lines = append(lines, line)
			continue
		}

		patched, wasChanged, err := remediateLine(line, stats)
		if err != nil {
			// Skip lines that can't be parsed; preserve them unchanged.
			lines = append(lines, line)
			continue
		}
		if wasChanged {
			changed = true
		}
		lines = append(lines, patched)
	}

	if err := scanner.Err(); err != nil {
		return fmt.Errorf("scan file: %w", err)
	}

	if !changed {
		return nil
	}

	// Write patched file back. Preserve trailing newline.
	output := strings.Join(lines, "\n")
	if !strings.HasSuffix(output, "\n") {
		output += "\n"
	}
	return os.WriteFile(path, []byte(output), 0644)
}

// remediateLine processes a single JSON line and returns the (potentially
// patched) JSON string plus a flag indicating if changes were made.
func remediateLine(line string, stats *remediationStats) (string, bool, error) {
	// Try to detect format. Legacy batch format has "failures" array.
	// Per-recipe format has "recipe" field. We need to handle both.
	var raw map[string]json.RawMessage
	if err := json.Unmarshal([]byte(line), &raw); err != nil {
		return line, false, err
	}

	// Check for legacy batch format (has "failures" key).
	if _, hasFailures := raw["failures"]; hasFailures {
		return remediateLegacyLine(line, stats)
	}

	// Per-recipe format: skip (no message field to extract from).
	// Still count as scanned.
	if _, hasRecipe := raw["recipe"]; hasRecipe {
		stats.RecordsScanned++
	}
	return line, false, nil
}

// legacyRecord mirrors the legacy batch failure JSONL format for
// in-place patching. We use json.RawMessage for fields we don't
// modify to avoid reformatting.
type legacyRecord struct {
	SchemaVersion int             `json:"schema_version"`
	Ecosystem     string          `json:"ecosystem,omitempty"`
	Environment   string          `json:"environment,omitempty"`
	UpdatedAt     string          `json:"updated_at,omitempty"`
	Failures      []legacyFailure `json:"failures"`
}

type legacyFailure struct {
	PackageID string   `json:"package_id"`
	Category  string   `json:"category"`
	BlockedBy []string `json:"blocked_by,omitempty"`
	Message   string   `json:"message"`
	Timestamp string   `json:"timestamp"`
}

// remediateLegacyLine handles legacy batch format lines containing a
// failures[] array with message fields.
func remediateLegacyLine(line string, stats *remediationStats) (string, bool, error) {
	var record legacyRecord
	if err := json.Unmarshal([]byte(line), &record); err != nil {
		return line, false, err
	}

	lineChanged := false
	for i := range record.Failures {
		f := &record.Failures[i]
		stats.RecordsScanned++

		// Skip records already correctly categorized.
		if isAlreadyRemediated(f.Category, f.BlockedBy) {
			continue
		}

		// Extract dependency names from message.
		deps := extractDeps(f.Message)
		if len(deps) == 0 {
			continue
		}

		// Patch the record.
		f.Category = "missing_dep"
		f.BlockedBy = deps
		lineChanged = true
		stats.RecordsUpdated++
		stats.RemediatedPkgs[f.PackageID] = true
		for _, dep := range deps {
			stats.UniqueDeps[dep] = true
		}
	}

	if !lineChanged {
		return line, false, nil
	}

	patched, err := json.Marshal(record)
	if err != nil {
		return line, false, err
	}
	return string(patched), true, nil
}

// isAlreadyRemediated returns true if a failure record is already correctly
// categorized for dependency tracking purposes.
func isAlreadyRemediated(category string, blockedBy []string) bool {
	return (category == "missing_dep" || category == "recipe_not_found") && len(blockedBy) > 0
}

// extractDeps pulls dependency names from a message using the "recipe X not
// found in registry" regex. Names are deduplicated and validated.
func extractDeps(message string) []string {
	matches := reNotFoundInRegistry.FindAllStringSubmatch(message, -1)
	if len(matches) == 0 {
		return nil
	}

	seen := make(map[string]bool)
	var deps []string
	for _, m := range matches {
		name := m[1]
		if !isValidDependencyName(name) {
			continue
		}
		if !seen[name] {
			seen[name] = true
			deps = append(deps, name)
		}
	}
	return deps
}

// isValidDependencyName rejects names containing path traversal or injection
// characters. Same logic as internal/batch/orchestrator.go:isValidDependencyName.
func isValidDependencyName(name string) bool {
	if strings.Contains(name, "/") || strings.Contains(name, "\\") ||
		strings.Contains(name, "..") || strings.Contains(name, "<") ||
		strings.Contains(name, ">") {
		return false
	}
	return name != ""
}

// remediateQueue loads the unified queue and flips entries from "failed" to
// "blocked" when their source matches a remediated failure record's package_id.
func remediateQueue(queuePath string, stats *remediationStats) error {
	queue, err := batch.LoadUnifiedQueue(queuePath)
	if err != nil {
		return fmt.Errorf("load queue: %w", err)
	}

	changed := false
	for i := range queue.Entries {
		entry := &queue.Entries[i]
		if entry.Status != batch.StatusFailed {
			continue
		}
		if !stats.RemediatedPkgs[entry.Source] {
			continue
		}
		entry.Status = batch.StatusBlocked
		stats.QueueFlipped++
		changed = true
	}

	if !changed {
		return nil
	}

	return batch.SaveUnifiedQueue(queuePath, queue)
}

// regenerateDashboard calls the dashboard generation logic with default paths.
func regenerateDashboard() error {
	opts := dashboard.DefaultOptions()
	return dashboard.Generate(opts)
}

// sortedKeys returns the sorted keys of a bool map.
func sortedKeys(m map[string]bool) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
