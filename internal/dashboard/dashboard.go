// Package dashboard generates pipeline status data for the web dashboard.
package dashboard

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"time"

	"github.com/tsukumogami/tsuku/internal/batch"
)

// Options configures dashboard generation.
type Options struct {
	QueueFile          string // Path to unified priority-queue.json file
	FailuresDir        string // Directory containing failures JSONL files
	MetricsDir         string // Directory containing metrics JSONL files
	DisambiguationsDir string // Directory containing disambiguation JSONL files
	ControlFile        string // Path to batch-control.json for circuit breaker state
	OutputFile         string // Path to output dashboard.json
}

// DefaultOptions returns options with default file paths.
func DefaultOptions() Options {
	return Options{
		QueueFile:          "data/queues/priority-queue.json",
		FailuresDir:        "data/failures",
		MetricsDir:         "data/metrics",
		DisambiguationsDir: "data/disambiguations",
		ControlFile:        "batch-control.json",
		OutputFile:         "website/pipeline/dashboard.json",
	}
}

// Dashboard is the output JSON structure.
type Dashboard struct {
	GeneratedAt     string                `json:"generated_at"`
	Queue           QueueStatus           `json:"queue"`
	Blockers        []Blocker             `json:"blockers"`
	Failures        map[string]int        `json:"failures"`
	FailureDetails  []FailureDetail       `json:"failure_details,omitempty"`
	Health          *HealthStatus         `json:"health,omitempty"`
	Runs            []RunSummary          `json:"runs,omitempty"`
	Disambiguations *DisambiguationStatus `json:"disambiguations,omitempty"`
	Curated         []CuratedOverride     `json:"curated,omitempty"`
}

// CuratedOverride represents a queue entry with curated confidence,
// surfaced in the dashboard for visibility into manual source selections.
type CuratedOverride struct {
	Name             string `json:"name"`
	Source           string `json:"source"`
	Reason           string `json:"reason,omitempty"`
	ValidationStatus string `json:"validation_status"` // "valid", "invalid", "unknown"
}

// HealthStatus summarizes pipeline health including circuit breaker state
// and run tracking across ecosystems.
type HealthStatus struct {
	Ecosystems           map[string]EcosystemHealth `json:"ecosystems"`
	LastRun              *RunInfo                   `json:"last_run"`
	LastSuccessfulRun    *RunInfo                   `json:"last_successful_run"`
	RunsSinceLastSuccess int                        `json:"runs_since_last_success"`
	HoursSinceLastRun    int                        `json:"hours_since_last_run"`
}

// EcosystemHealth represents the circuit breaker state for a single ecosystem.
type EcosystemHealth struct {
	BreakerState string `json:"breaker_state"`
	Failures     int    `json:"failures"`
	LastFailure  string `json:"last_failure,omitempty"`
	OpensAt      string `json:"opens_at,omitempty"`
}

// RunInfo summarizes a single batch run for health tracking purposes.
type RunInfo struct {
	BatchID       string         `json:"batch_id"`
	Ecosystems    map[string]int `json:"ecosystems,omitempty"`
	Timestamp     string         `json:"timestamp"`
	Succeeded     int            `json:"succeeded"`
	Failed        int            `json:"failed"`
	Total         int            `json:"total"`
	RecipesMerged int            `json:"recipes_merged,omitempty"`
}

// DisambiguationStatus summarizes disambiguation activity across batch runs.
type DisambiguationStatus struct {
	Total      int                    `json:"total"`       // Total tools with disambiguation decisions
	ByReason   map[string]int         `json:"by_reason"`   // Count by selection reason
	HighRisk   int                    `json:"high_risk"`   // Count of priority_fallback selections
	NeedReview []string               `json:"need_review"` // Tools with HighRisk=true (for human review)
	Entries    []DisambiguationRecord `json:"entries"`     // Full list of disambiguation records
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
	ID           string   `json:"id"`
	Name         string   `json:"name"`
	Ecosystem    string   `json:"ecosystem"`
	Priority     int      `json:"priority"`
	Category     string   `json:"category,omitempty"`      // For failed packages
	BlockedBy    []string `json:"blocked_by,omitempty"`    // For blocked packages
	FailureCount int      `json:"failure_count"`           // Consecutive failures from unified queue
	NextRetryAt  string   `json:"next_retry_at,omitempty"` // Earliest retry time (ISO 8601)
}

// Blocker represents a dependency blocking packages.
type Blocker struct {
	Dependency string   `json:"dependency"`
	Count      int      `json:"count"`
	Packages   []string `json:"packages"` // First 5 package names
}

// RunSummary summarizes a batch run.
type RunSummary struct {
	BatchID    string         `json:"batch_id"`
	Ecosystems map[string]int `json:"ecosystems,omitempty"`
	Total      int            `json:"total"`
	Merged     int            `json:"merged"`
	Rate       float64        `json:"rate"`
	Duration   int            `json:"duration,omitempty"`
	Timestamp  string         `json:"timestamp"`
}

// FailureRecord represents one line in failures JSONL.
// Supports both legacy batch format and per-recipe format.
type FailureRecord struct {
	SchemaVersion int              `json:"schema_version"`
	Ecosystem     string           `json:"ecosystem,omitempty"`
	Environment   string           `json:"environment,omitempty"`
	UpdatedAt     string           `json:"updated_at,omitempty"`
	Timestamp     string           `json:"timestamp,omitempty"` // Per-recipe format timestamp
	Failures      []PackageFailure `json:"failures,omitempty"`
	// Per-recipe format fields
	Recipe    string   `json:"recipe,omitempty"`
	Platform  string   `json:"platform,omitempty"`
	Category  string   `json:"category,omitempty"`
	ExitCode  int      `json:"exit_code,omitempty"`
	BlockedBy []string `json:"blocked_by,omitempty"` // Added for missing_dep tracking
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
// Supports both old format (ecosystem string) and new format (ecosystems object).
type MetricsRecord struct {
	BatchID         string         `json:"batch_id"`
	Ecosystem       string         `json:"ecosystem,omitempty"`
	Ecosystems      map[string]int `json:"ecosystems,omitempty"`
	Total           int            `json:"total"`
	Generated       int            `json:"generated"`
	Merged          int            `json:"merged"`
	Excluded        int            `json:"excluded"`
	Constrained     int            `json:"constrained"`
	Timestamp       string         `json:"timestamp"`
	DurationSeconds int            `json:"duration_seconds"`
}

// DisambiguationFile represents one line in disambiguation JSONL files.
type DisambiguationFile struct {
	SchemaVersion   int                    `json:"schema_version"`
	Ecosystem       string                 `json:"ecosystem"`
	Environment     string                 `json:"environment"`
	UpdatedAt       string                 `json:"updated_at"`
	Disambiguations []DisambiguationRecord `json:"disambiguations"`
}

// DisambiguationRecord represents a single disambiguation decision.
type DisambiguationRecord struct {
	Tool            string   `json:"tool"`
	Selected        string   `json:"selected"`
	Alternatives    []string `json:"alternatives"`
	SelectionReason string   `json:"selection_reason"`
	DownloadsRatio  float64  `json:"downloads_ratio,omitempty"`
	HighRisk        bool     `json:"high_risk"`
}

// loadQueue loads the unified queue from a single file.
// Returns an empty queue if the file doesn't exist. Returns an error if the
// file contains entries that fail validation (e.g. legacy seed format files
// with missing source or invalid priority), or if a non-empty file produces
// zero entries (likely a legacy format mismatch).
func loadQueue(path string) (*batch.UnifiedQueue, error) {
	q, err := batch.LoadUnifiedQueue(path)
	if err != nil {
		return nil, err
	}
	// Detect legacy format: file exists and has content, but no entries parsed.
	// This happens when a seed-format file (with "packages" key) is loaded by
	// the unified loader (which expects "entries" key).
	if len(q.Entries) == 0 {
		info, statErr := os.Stat(path)
		if statErr == nil && info.Size() > 2 { // >2 bytes rules out "{}"
			return nil, fmt.Errorf("queue file %s has content but no entries parsed; check format (expected unified schema with \"entries\" key)", path)
		}
	}
	for i, entry := range q.Entries {
		if err := entry.Validate(); err != nil {
			return nil, fmt.Errorf("queue entry %d (%s): %w", i, entry.Name, err)
		}
	}
	return q, nil
}

// loadCurated filters the unified queue for entries with curated confidence
// and maps them to CuratedOverride records. The ValidationStatus is derived
// from the entry's queue status: "success" and "pending" map to "valid",
// "failed" maps to "invalid", and everything else maps to "unknown".
func loadCurated(queue *batch.UnifiedQueue) []CuratedOverride {
	var curated []CuratedOverride
	for _, entry := range queue.Entries {
		if entry.Confidence != batch.ConfidenceCurated {
			continue
		}

		var validationStatus string
		switch entry.Status {
		case batch.StatusSuccess, batch.StatusPending:
			validationStatus = "valid"
		case batch.StatusFailed:
			validationStatus = "invalid"
		default:
			validationStatus = "unknown"
		}

		curated = append(curated, CuratedOverride{
			Name:             entry.Name,
			Source:           entry.Source,
			ValidationStatus: validationStatus,
		})
	}
	return curated
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

	// Load unified queue
	queue, err := loadQueue(opts.QueueFile)
	if err != nil {
		return fmt.Errorf("load queue: %w", err)
	}
	dash.Queue = computeQueueStatus(queue, failureDetails)

	// Load curated overrides from queue
	if curated := loadCurated(queue); len(curated) > 0 {
		dash.Curated = curated
	}

	// Load individual failure detail records
	detailRecords, detailErr := loadFailureDetailRecords(opts.FailuresDir, queue)
	if detailErr == nil && len(detailRecords) > 0 {
		dash.FailureDetails = detailRecords
	}

	// Load metrics
	runs, metricsRecords, err := loadMetricsFromDir(opts.MetricsDir)
	if err == nil && len(runs) > 0 {
		// Take last 10, newest first (runs are sorted by timestamp in loadMetricsFromDir)
		if len(runs) > 10 {
			runs = runs[len(runs)-10:]
		}
		// Reverse to newest first
		for i, j := 0, len(runs)-1; i < j; i, j = i+1, j-1 {
			runs[i], runs[j] = runs[j], runs[i]
		}
		dash.Runs = runs
	}

	// Load health status
	health, err := loadHealth(opts.ControlFile, metricsRecords)
	if err == nil && health != nil {
		dash.Health = health
	}

	// Load disambiguations
	disambiguations, err := loadDisambiguationsFromDir(opts.DisambiguationsDir)
	if err == nil && disambiguations != nil && disambiguations.Total > 0 {
		dash.Disambiguations = disambiguations
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

func computeQueueStatus(queue *batch.UnifiedQueue, failureDetails map[string]FailureDetails) QueueStatus {
	status := QueueStatus{
		Total:    len(queue.Entries),
		ByStatus: make(map[string]int),
		ByTier:   make(map[int]map[string]int),
		Packages: make(map[string][]PackageInfo),
	}

	for _, entry := range queue.Entries {
		status.ByStatus[entry.Status]++

		if _, ok := status.ByTier[entry.Priority]; !ok {
			status.ByTier[entry.Priority] = make(map[string]int)
		}
		status.ByTier[entry.Priority][entry.Status]++

		// Build package info with failure details if available.
		// ID is constructed as "ecosystem:name" to match failure detail keys.
		id := entry.Source
		var nextRetry string
		if entry.NextRetryAt != nil {
			nextRetry = entry.NextRetryAt.Format("2006-01-02T15:04:05Z")
		}
		info := PackageInfo{
			ID:           id,
			Name:         entry.Name,
			Ecosystem:    entry.Ecosystem(),
			Priority:     entry.Priority,
			FailureCount: entry.FailureCount,
			NextRetryAt:  nextRetry,
		}

		if details, ok := failureDetails[id]; ok {
			info.Category = details.Category
			info.BlockedBy = details.BlockedBy
		}

		status.Packages[entry.Status] = append(status.Packages[entry.Status], info)
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

			// Track blocked_by for missing_dep failures in per-recipe format
			if len(record.BlockedBy) > 0 {
				eco := record.Ecosystem
				if eco == "" {
					eco = "homebrew" // last-resort fallback for pre-unified records
				}
				pkgID := eco + ":" + record.Recipe
				details[pkgID] = FailureDetails{
					Category:  record.Category,
					BlockedBy: record.BlockedBy,
				}
				for _, dep := range record.BlockedBy {
					blockers[dep] = append(blockers[dep], pkgID)
				}
			}
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

func loadMetrics(path string) ([]RunSummary, []MetricsRecord, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, nil, err
	}

	var runs []RunSummary
	var records []MetricsRecord

	// Try streaming decoder first for pretty-printed JSON objects
	decoder := json.NewDecoder(bytes.NewReader(data))
	for {
		var record MetricsRecord
		if err := decoder.Decode(&record); err != nil {
			if err == io.EOF {
				break
			}
			// If streaming fails, fall back to line-by-line parsing
			runs, records = parseMetricsLineByLine(data)
			break
		}

		if record.BatchID == "" {
			continue
		}

		rate := 0.0
		if record.Total > 0 {
			rate = float64(record.Merged) / float64(record.Total)
		}

		records = append(records, record)
		runs = append(runs, RunSummary{
			BatchID:    record.BatchID,
			Ecosystems: resolveEcosystems(record),
			Total:      record.Total,
			Merged:     record.Merged,
			Rate:       rate,
			Duration:   record.DurationSeconds,
			Timestamp:  record.Timestamp,
		})
	}

	return runs, records, nil
}

// parseMetricsLineByLine handles JSONL format where each line is a JSON object.
// Malformed lines are skipped.
func parseMetricsLineByLine(data []byte) ([]RunSummary, []MetricsRecord) {
	var runs []RunSummary
	var records []MetricsRecord
	scanner := bufio.NewScanner(bytes.NewReader(data))
	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}

		var record MetricsRecord
		if err := json.Unmarshal(line, &record); err != nil {
			continue // Skip malformed lines
		}

		if record.BatchID == "" {
			continue
		}

		rate := 0.0
		if record.Total > 0 {
			rate = float64(record.Merged) / float64(record.Total)
		}

		records = append(records, record)
		runs = append(runs, RunSummary{
			BatchID:    record.BatchID,
			Ecosystems: resolveEcosystems(record),
			Total:      record.Total,
			Merged:     record.Merged,
			Rate:       rate,
			Duration:   record.DurationSeconds,
			Timestamp:  record.Timestamp,
		})
	}
	return runs, records
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
func loadMetricsFromDir(dir string) ([]RunSummary, []MetricsRecord, error) {
	var allRuns []RunSummary
	var allRecords []MetricsRecord

	// Glob for all JSONL files that start with "batch-runs"
	pattern := filepath.Join(dir, "batch-runs*.jsonl")
	files, err := filepath.Glob(pattern)
	if err != nil {
		return nil, nil, fmt.Errorf("glob metrics: %w", err)
	}

	if len(files) == 0 {
		return nil, nil, fmt.Errorf("no metrics files found")
	}

	// Aggregate across all files
	for _, path := range files {
		runs, records, err := loadMetrics(path)
		if err != nil {
			continue // Skip files that can't be read
		}
		allRuns = append(allRuns, runs...)
		allRecords = append(allRecords, records...)
	}

	// Sort by timestamp so newest runs are last regardless of file load order.
	// The legacy batch-runs.jsonl (no timestamp in filename) sorts after
	// timestamped files alphabetically, which would otherwise put old runs
	// at the end of the slice.
	sort.Slice(allRuns, func(i, j int) bool {
		return allRuns[i].Timestamp < allRuns[j].Timestamp
	})
	sort.Slice(allRecords, func(i, j int) bool {
		return allRecords[i].Timestamp < allRecords[j].Timestamp
	})

	return allRuns, allRecords, nil
}

// loadDisambiguationsFromDir aggregates disambiguation records across all JSONL files
// in a directory and computes summary statistics.
func loadDisambiguationsFromDir(dir string) (*DisambiguationStatus, error) {
	// Glob for all JSONL files in the directory
	pattern := filepath.Join(dir, "*.jsonl")
	files, err := filepath.Glob(pattern)
	if err != nil {
		return nil, fmt.Errorf("glob disambiguations: %w", err)
	}

	if len(files) == 0 {
		return nil, nil // No disambiguation files yet
	}

	status := &DisambiguationStatus{
		ByReason:   make(map[string]int),
		NeedReview: []string{},
		Entries:    []DisambiguationRecord{},
	}

	// Track seen tools to avoid double-counting across files
	seenTools := make(map[string]bool)

	for _, path := range files {
		records, err := loadDisambiguationFile(path)
		if err != nil {
			continue // Skip files that can't be read
		}

		for _, rec := range records {
			// Skip if we've already counted this tool
			if seenTools[rec.Tool] {
				continue
			}
			seenTools[rec.Tool] = true

			status.Total++
			status.ByReason[rec.SelectionReason]++
			status.Entries = append(status.Entries, rec)

			if rec.HighRisk {
				status.HighRisk++
				status.NeedReview = append(status.NeedReview, rec.Tool)
			}
		}
	}

	// Sort entries by tool name for stable output
	sort.Slice(status.Entries, func(i, j int) bool {
		return status.Entries[i].Tool < status.Entries[j].Tool
	})

	// Sort NeedReview for stable output
	sort.Strings(status.NeedReview)

	return status, nil
}

// loadDisambiguationFile reads a single disambiguation JSONL file.
func loadDisambiguationFile(path string) ([]DisambiguationRecord, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	var allRecords []DisambiguationRecord
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}

		var df DisambiguationFile
		if err := json.Unmarshal(line, &df); err != nil {
			continue // Skip malformed lines
		}

		allRecords = append(allRecords, df.Disambiguations...)
	}

	return allRecords, scanner.Err()
}

// batchControlFile represents the top-level structure of batch-control.json.
type batchControlFile struct {
	CircuitBreaker map[string]circuitBreakerState `json:"circuit_breaker"`
}

// circuitBreakerState represents a single ecosystem's circuit breaker entry.
type circuitBreakerState struct {
	State       string `json:"state"`
	Failures    int    `json:"failures"`
	LastFailure string `json:"last_failure,omitempty"`
	OpensAt     string `json:"opens_at,omitempty"`
}

// loadHealth reads batch-control.json for circuit breaker state and scans
// metrics records to compute run tracking fields. Returns nil if the control
// file doesn't exist and no metrics records are available.
func loadHealth(controlFile string, records []MetricsRecord) (*HealthStatus, error) {
	var ecosystems map[string]EcosystemHealth

	// Read circuit breaker state from batch-control.json
	data, err := os.ReadFile(controlFile)
	if err == nil {
		var control batchControlFile
		if err := json.Unmarshal(data, &control); err != nil {
			return nil, fmt.Errorf("parse control file: %w", err)
		}
		ecosystems = make(map[string]EcosystemHealth, len(control.CircuitBreaker))
		for name, cb := range control.CircuitBreaker {
			ecosystems[name] = EcosystemHealth{
				BreakerState: cb.State,
				Failures:     cb.Failures,
				LastFailure:  cb.LastFailure,
				OpensAt:      cb.OpensAt,
			}
		}
	} else if !os.IsNotExist(err) {
		return nil, fmt.Errorf("read control file: %w", err)
	}

	// If there are no records and no control file, nothing to report
	if ecosystems == nil && len(records) == 0 {
		return nil, nil
	}

	health := &HealthStatus{
		Ecosystems: ecosystems,
	}
	if health.Ecosystems == nil {
		health.Ecosystems = make(map[string]EcosystemHealth)
	}

	if len(records) == 0 {
		return health, nil
	}

	// Sort records by timestamp to find last run and last successful run.
	// Work on a copy to avoid mutating the caller's slice.
	sorted := make([]MetricsRecord, len(records))
	copy(sorted, records)
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i].Timestamp < sorted[j].Timestamp
	})

	// Last run is the most recent record
	last := sorted[len(sorted)-1]
	health.LastRun = metricsToRunInfo(last)

	// Compute hours since last run
	if ts, err := time.Parse(time.RFC3339, last.Timestamp); err == nil {
		hours := int(time.Since(ts).Hours())
		if hours < 0 {
			hours = 0
		}
		health.HoursSinceLastRun = hours
	}

	// Find last successful run (merged > 0) and count runs since
	runsSince := 0
	for i := len(sorted) - 1; i >= 0; i-- {
		rec := sorted[i]
		if rec.Merged > 0 {
			health.LastSuccessfulRun = metricsToRunInfo(rec)
			health.RunsSinceLastSuccess = runsSince
			break
		}
		runsSince++
	}
	// If no successful run found, runs_since_last_success = total records
	if health.LastSuccessfulRun == nil {
		health.RunsSinceLastSuccess = len(sorted)
	}

	return health, nil
}

// resolveEcosystems normalizes ecosystem data from a MetricsRecord.
// New format records have an ecosystems map; old format records have a single
// ecosystem string which is synthesized into {ecosystem: total}.
func resolveEcosystems(rec MetricsRecord) map[string]int {
	if len(rec.Ecosystems) > 0 {
		return rec.Ecosystems
	}
	if rec.Ecosystem != "" {
		return map[string]int{rec.Ecosystem: rec.Total}
	}
	return nil
}

// metricsToRunInfo converts a MetricsRecord into a RunInfo.
func metricsToRunInfo(rec MetricsRecord) *RunInfo {
	failed := rec.Total - rec.Merged
	if failed < 0 {
		failed = 0
	}
	return &RunInfo{
		BatchID:       rec.BatchID,
		Ecosystems:    resolveEcosystems(rec),
		Timestamp:     rec.Timestamp,
		Succeeded:     rec.Merged,
		Failed:        failed,
		Total:         rec.Total,
		RecipesMerged: rec.Merged,
	}
}
