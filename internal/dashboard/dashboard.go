// Package dashboard generates pipeline status data for the web dashboard.
package dashboard

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"time"

	"github.com/tsukumogami/tsuku/internal/seed"
)

// Options configures dashboard generation.
type Options struct {
	QueueFile   string // Path to priority-queue.json or queues directory
	FailuresDir string // Directory containing failures JSONL files
	MetricsDir  string // Directory containing metrics JSONL files
	OutputFile  string // Path to output dashboard.json
}

// DefaultOptions returns options with default file paths.
func DefaultOptions() Options {
	return Options{
		QueueFile:   "data/queues",
		FailuresDir: "data/failures",
		MetricsDir:  "data/metrics",
		OutputFile:  "website/pipeline/dashboard.json",
	}
}

// Dashboard is the output JSON structure.
type Dashboard struct {
	GeneratedAt string         `json:"generated_at"`
	Queue       QueueStatus    `json:"queue"`
	Blockers    []Blocker      `json:"blockers"`
	Failures    map[string]int `json:"failures"`
	Runs        []RunSummary   `json:"runs,omitempty"`
}

// QueueStatus summarizes queue state.
type QueueStatus struct {
	Total    int                      `json:"total"`
	ByStatus map[string]int           `json:"by_status"`
	ByTier   map[int]map[string]int   `json:"by_tier"`
	Packages map[string][]PackageInfo `json:"packages"` // Packages grouped by status
}

// PackageInfo contains details about a package for display.
type PackageInfo struct {
	ID        string   `json:"id"`
	Name      string   `json:"name"`
	Ecosystem string   `json:"ecosystem"`
	Priority  int      `json:"priority"`
	Category  string   `json:"category,omitempty"`   // For failed packages
	BlockedBy []string `json:"blocked_by,omitempty"` // For blocked packages
}

// Blocker represents a dependency blocking packages.
type Blocker struct {
	Dependency string   `json:"dependency"`
	Count      int      `json:"count"`
	Packages   []string `json:"packages"` // First 5 package names
}

// RunSummary summarizes a batch run.
type RunSummary struct {
	BatchID   string  `json:"batch_id"`
	Total     int     `json:"total"`
	Merged    int     `json:"merged"`
	Rate      float64 `json:"rate"`
	Timestamp string  `json:"timestamp"`
}

// FailureRecord represents one line in failures JSONL (legacy batch format).
type FailureRecord struct {
	SchemaVersion int              `json:"schema_version"`
	Ecosystem     string           `json:"ecosystem,omitempty"`
	Environment   string           `json:"environment,omitempty"`
	UpdatedAt     string           `json:"updated_at,omitempty"`
	Failures      []PackageFailure `json:"failures,omitempty"`
	// Per-recipe format fields
	Recipe   string `json:"recipe,omitempty"`
	Platform string `json:"platform,omitempty"`
	Category string `json:"category,omitempty"`
	ExitCode int    `json:"exit_code,omitempty"`
}

// PackageFailure is a single failure entry in the legacy batch format.
type PackageFailure struct {
	PackageID string   `json:"package_id"`
	Category  string   `json:"category"`
	BlockedBy []string `json:"blocked_by,omitempty"`
	Message   string   `json:"message"`
	Timestamp string   `json:"timestamp"`
}

// MetricsRecord represents one line in batch-runs.jsonl.
type MetricsRecord struct {
	BatchID         string `json:"batch_id"`
	Ecosystem       string `json:"ecosystem"`
	Total           int    `json:"total"`
	Generated       int    `json:"generated"`
	Merged          int    `json:"merged"`
	Excluded        int    `json:"excluded"`
	Constrained     int    `json:"constrained"`
	Timestamp       string `json:"timestamp"`
	DurationSeconds int    `json:"duration_seconds"`
}

// loadQueueFromPathOrDir loads queue from a file or aggregates all queues from a directory.
func loadQueueFromPathOrDir(path string) (*seed.PriorityQueue, error) {
	info, err := os.Stat(path)
	if err != nil {
		return nil, err
	}

	// Single file case
	if !info.IsDir() {
		return seed.Load(path)
	}

	// Directory case: aggregate all ecosystem queue files
	pattern := filepath.Join(path, "priority-queue-*.json")
	files, err := filepath.Glob(pattern)
	if err != nil {
		return nil, fmt.Errorf("glob queues: %w", err)
	}

	if len(files) == 0 {
		return nil, fmt.Errorf("no queue files found in %s", path)
	}

	// Load and merge all queues
	aggregated := &seed.PriorityQueue{
		SchemaVersion: 1,
		Packages:      []seed.Package{},
		UpdatedAt:     time.Now().UTC().Format(time.RFC3339),
	}

	for _, file := range files {
		q, err := seed.Load(file)
		if err != nil {
			continue // Skip malformed files
		}
		aggregated.Packages = append(aggregated.Packages, q.Packages...)
		// Use the most recent update time
		if q.UpdatedAt > aggregated.UpdatedAt {
			aggregated.UpdatedAt = q.UpdatedAt
		}
	}

	return aggregated, nil
}

// Generate reads pipeline data files and produces dashboard.json.
func Generate(opts Options) error {
	dash := Dashboard{
		GeneratedAt: time.Now().UTC().Format(time.RFC3339),
		Failures:    make(map[string]int),
	}

	// Load failures first to get details for packages
	blockerCounts, failureCounts, failureDetails, err := loadFailuresFromDir(opts.FailuresDir)
	if err != nil {
		// Non-fatal: failures directory might not exist yet
		blockerCounts = make(map[string][]string)
		failureCounts = make(map[string]int)
		failureDetails = make(map[string]FailureDetails)
	}
	dash.Blockers = computeTopBlockers(blockerCounts, 10)
	dash.Failures = failureCounts

	// Load queue (from file or directory)
	queue, err := loadQueueFromPathOrDir(opts.QueueFile)
	if err != nil {
		return fmt.Errorf("load queue: %w", err)
	}
	dash.Queue = computeQueueStatus(queue, failureDetails)

	// Load metrics
	runs, err := loadMetricsFromDir(opts.MetricsDir)
	if err == nil && len(runs) > 0 {
		// Take last 10, newest first
		if len(runs) > 10 {
			runs = runs[len(runs)-10:]
		}
		// Reverse to newest first
		for i, j := 0, len(runs)-1; i < j; i, j = i+1, j-1 {
			runs[i], runs[j] = runs[j], runs[i]
		}
		dash.Runs = runs
	}

	// Write output
	data, err := json.MarshalIndent(dash, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal dashboard: %w", err)
	}
	data = append(data, '\n')

	if err := os.WriteFile(opts.OutputFile, data, 0644); err != nil {
		return fmt.Errorf("write dashboard: %w", err)
	}

	return nil
}

func computeQueueStatus(queue *seed.PriorityQueue, failureDetails map[string]FailureDetails) QueueStatus {
	status := QueueStatus{
		Total:    len(queue.Packages),
		ByStatus: make(map[string]int),
		ByTier:   make(map[int]map[string]int),
		Packages: make(map[string][]PackageInfo),
	}

	for _, pkg := range queue.Packages {
		status.ByStatus[pkg.Status]++

		if _, ok := status.ByTier[pkg.Tier]; !ok {
			status.ByTier[pkg.Tier] = make(map[string]int)
		}
		status.ByTier[pkg.Tier][pkg.Status]++

		// Build package info with failure details if available
		info := PackageInfo{
			ID:        pkg.ID,
			Name:      pkg.Name,
			Ecosystem: pkg.Source,
			Priority:  pkg.Tier,
		}

		if details, ok := failureDetails[pkg.ID]; ok {
			info.Category = details.Category
			info.BlockedBy = details.BlockedBy
		}

		status.Packages[pkg.Status] = append(status.Packages[pkg.Status], info)
	}

	// Sort packages by priority (lower number = higher priority)
	for s := range status.Packages {
		sort.Slice(status.Packages[s], func(i, j int) bool {
			if status.Packages[s][i].Priority != status.Packages[s][j].Priority {
				return status.Packages[s][i].Priority < status.Packages[s][j].Priority
			}
			return status.Packages[s][i].Name < status.Packages[s][j].Name
		})
	}

	return status
}

// FailureDetails holds failure information for a package.
type FailureDetails struct {
	Category  string
	BlockedBy []string
}

func loadFailures(path string) (map[string][]string, map[string]int, map[string]FailureDetails, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, nil, nil, err
	}
	defer file.Close()

	blockers := make(map[string][]string)      // dependency -> list of blocked packages
	categories := make(map[string]int)         // category -> count
	details := make(map[string]FailureDetails) // package ID -> failure details

	scanner := bufio.NewScanner(file)
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
		if len(record.Failures) > 0 {
			for _, f := range record.Failures {
				categories[f.Category]++
				details[f.PackageID] = FailureDetails{
					Category:  f.Category,
					BlockedBy: f.BlockedBy,
				}
				for _, dep := range f.BlockedBy {
					blockers[dep] = append(blockers[dep], f.PackageID)
				}
			}
		}

		// Handle per-recipe format
		if record.Recipe != "" && record.Category != "" {
			categories[record.Category]++
		}
	}

	return blockers, categories, details, scanner.Err()
}

// computeTransitiveBlockers computes all packages blocked by a dependency (directly or indirectly).
func computeTransitiveBlockers(dep string, blockers map[string][]string, memo map[string][]string) []string {
	if result, ok := memo[dep]; ok {
		return result
	}

	blocked := make(map[string]bool)
	// Add directly blocked packages
	for _, pkg := range blockers[dep] {
		blocked[pkg] = true
		// Recursively add packages blocked by this package
		for _, transitive := range computeTransitiveBlockers(pkg, blockers, memo) {
			blocked[transitive] = true
		}
	}

	// Convert to slice
	result := make([]string, 0, len(blocked))
	for pkg := range blocked {
		result = append(result, pkg)
	}
	memo[dep] = result
	return result
}

func computeTopBlockers(blockers map[string][]string, limit int) []Blocker {
	memo := make(map[string][]string)
	result := make([]Blocker, 0, len(blockers))

	for dep := range blockers {
		unique := computeTransitiveBlockers(dep, blockers, memo)

		b := Blocker{
			Dependency: dep,
			Count:      len(unique),
		}
		// Keep first 5 packages (sorted by name for stability)
		packages := make([]string, len(unique))
		copy(packages, unique)
		sort.Strings(packages)
		if len(packages) > 5 {
			b.Packages = packages[:5]
		} else {
			b.Packages = packages
		}
		result = append(result, b)
	}

	// Sort by count descending
	sort.Slice(result, func(i, j int) bool {
		return result[i].Count > result[j].Count
	})

	if len(result) > limit {
		result = result[:limit]
	}
	return result
}

func loadMetrics(path string) ([]RunSummary, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	var runs []RunSummary
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}

		var record MetricsRecord
		if err := json.Unmarshal(line, &record); err != nil {
			continue
		}

		rate := 0.0
		if record.Total > 0 {
			rate = float64(record.Merged) / float64(record.Total)
		}

		runs = append(runs, RunSummary{
			BatchID:   record.BatchID,
			Total:     record.Total,
			Merged:    record.Merged,
			Rate:      rate,
			Timestamp: record.Timestamp,
		})
	}

	return runs, scanner.Err()
}

// loadFailuresFromDir aggregates failures across all JSONL files in a directory.
// Supports both timestamped files (homebrew-2026-02-06T14:30:00Z.jsonl) and legacy
// single files (homebrew.jsonl, failures.jsonl) for backward compatibility.
func loadFailuresFromDir(dir string) (map[string][]string, map[string]int, map[string]FailureDetails, error) {
	blockers := make(map[string][]string)
	categories := make(map[string]int)
	details := make(map[string]FailureDetails)

	// Glob for all JSONL files in the directory
	pattern := filepath.Join(dir, "*.jsonl")
	files, err := filepath.Glob(pattern)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("glob failures: %w", err)
	}

	if len(files) == 0 {
		return blockers, categories, details, fmt.Errorf("no failure files found")
	}

	// Aggregate across all files
	for _, path := range files {
		b, c, d, err := loadFailures(path)
		if err != nil {
			continue // Skip files that can't be read
		}

		// Merge blockers
		for dep, pkgs := range b {
			blockers[dep] = append(blockers[dep], pkgs...)
		}

		// Merge categories
		for cat, count := range c {
			categories[cat] += count
		}

		// Merge details (later files override earlier ones)
		for id, det := range d {
			details[id] = det
		}
	}

	return blockers, categories, details, nil
}

// loadMetricsFromDir aggregates metrics across all JSONL files in a directory.
// Supports both timestamped files (batch-runs-2026-02-06T14:30:00Z.jsonl) and legacy
// files (batch-runs.jsonl) for backward compatibility.
func loadMetricsFromDir(dir string) ([]RunSummary, error) {
	var allRuns []RunSummary

	// Glob for all JSONL files that start with "batch-runs"
	pattern := filepath.Join(dir, "batch-runs*.jsonl")
	files, err := filepath.Glob(pattern)
	if err != nil {
		return nil, fmt.Errorf("glob metrics: %w", err)
	}

	if len(files) == 0 {
		return nil, fmt.Errorf("no metrics files found")
	}

	// Aggregate across all files
	for _, path := range files {
		runs, err := loadMetrics(path)
		if err != nil {
			continue // Skip files that can't be read
		}
		allRuns = append(allRuns, runs...)
	}

	return allRuns, nil
}
