// Package dashboard generates pipeline status data for the web dashboard.
package dashboard

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"time"

	"github.com/tsukumogami/tsuku/internal/seed"
)

// Options configures dashboard generation.
type Options struct {
	QueueFile    string // Path to priority-queue.json
	FailuresFile string // Path to failures JSONL file
	MetricsFile  string // Path to batch-runs.jsonl
	OutputFile   string // Path to output dashboard.json
}

// DefaultOptions returns options with default file paths.
func DefaultOptions() Options {
	return Options{
		QueueFile:    "data/priority-queue.json",
		FailuresFile: "data/failures/homebrew.jsonl",
		MetricsFile:  "data/metrics/batch-runs.jsonl",
		OutputFile:   "website/pipeline/dashboard.json",
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

// Generate reads pipeline data files and produces dashboard.json.
func Generate(opts Options) error {
	dash := Dashboard{
		GeneratedAt: time.Now().UTC().Format(time.RFC3339),
		Failures:    make(map[string]int),
	}

	// Load failures first to get details for packages
	blockerCounts, failureCounts, failureDetails, err := loadFailures(opts.FailuresFile)
	if err != nil {
		// Non-fatal: failures file might not exist yet
		blockerCounts = make(map[string][]string)
		failureCounts = make(map[string]int)
		failureDetails = make(map[string]FailureDetails)
	}
	dash.Blockers = computeTopBlockers(blockerCounts, 10)
	dash.Failures = failureCounts

	// Load queue
	queue, err := seed.Load(opts.QueueFile)
	if err != nil {
		return fmt.Errorf("load queue: %w", err)
	}
	dash.Queue = computeQueueStatus(queue, failureDetails)

	// Load metrics
	runs, err := loadMetrics(opts.MetricsFile)
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

func computeTopBlockers(blockers map[string][]string, limit int) []Blocker {
	result := make([]Blocker, 0, len(blockers))
	for dep, packages := range blockers {
		// Deduplicate packages
		seen := make(map[string]bool)
		unique := make([]string, 0)
		for _, pkg := range packages {
			if !seen[pkg] {
				seen[pkg] = true
				unique = append(unique, pkg)
			}
		}

		b := Blocker{
			Dependency: dep,
			Count:      len(unique),
		}
		// Keep first 5 packages
		if len(unique) > 5 {
			b.Packages = unique[:5]
		} else {
			b.Packages = unique
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
