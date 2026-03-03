// Package markfailures reads failure JSONL data and updates queue entry
// statuses to reflect actual failure and blocked states. This bridges the
// gap between the batch orchestrator (which tracks failures locally) and
// the queue on main (which only gets success transitions from
// update-queue-status.yml).
//
// The package is designed for idempotent execution: running it multiple
// times on the same failure data produces the same result. It uses
// total failure count comparison (JSONL records vs queue entry's
// failure_count) to avoid re-marking entries from stale data.
package markfailures

import (
	"bufio"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/tsukumogami/tsuku/internal/batch"
)

// PackageFailureSummary aggregates failure information for a single package
// across all JSONL files.
type PackageFailureSummary struct {
	TotalFailures   int      // count of failure records across all JSONL files
	HasMissingDep   bool     // any missing_dep category failure
	BlockedBy       []string // union of blocked_by deps (deduplicated)
	HasOtherFailure bool     // any non-missing_dep failure
}

// Result summarizes the outcome of a mark-failures operation.
type Result struct {
	MarkedFailed  int      // entries set from pending to failed
	MarkedBlocked int      // entries set from pending to blocked
	Retried       int      // entries flipped from failed to pending (backoff expired)
	Details       []Change // per-entry changes
}

// Change records a single status transition.
type Change struct {
	Name      string // entry name
	FromState string // previous status
	ToState   string // new status
}

// backoffBase is the base duration for exponential backoff.
// Each failure doubles: 1h, 2h, 4h, 8h, ... capped at maxBackoff.
const backoffBase = 1 * time.Hour

// maxBackoff caps the retry delay at 7 days.
const maxBackoff = 7 * 24 * time.Hour

// failureRecord mirrors blocker.FailureRecord but is local to avoid
// coupling the two packages. Supports both legacy batch format and
// per-recipe format.
type failureRecord struct {
	SchemaVersion int              `json:"schema_version"`
	Ecosystem     string           `json:"ecosystem,omitempty"`
	Failures      []packageFailure `json:"failures,omitempty"`
	// Per-recipe format fields
	Recipe    string   `json:"recipe,omitempty"`
	Category  string   `json:"category,omitempty"`
	BlockedBy []string `json:"blocked_by,omitempty"`
}

type packageFailure struct {
	PackageID string   `json:"package_id"`
	Category  string   `json:"category"`
	BlockedBy []string `json:"blocked_by,omitempty"`
}

// LoadFailureMap reads all JSONL files in dir and builds a per-package
// failure summary. Keys are bare package names (ecosystem prefix stripped).
func LoadFailureMap(dir string) (map[string]*PackageFailureSummary, error) {
	pattern := filepath.Join(dir, "*.jsonl")
	files, err := filepath.Glob(pattern)
	if err != nil {
		return nil, fmt.Errorf("glob failures: %w", err)
	}

	result := make(map[string]*PackageFailureSummary)
	for _, path := range files {
		if err := loadFailuresFromFile(path, result); err != nil {
			continue // skip files that can't be read
		}
	}
	return result, nil
}

// loadFailuresFromFile reads a single JSONL file and populates the summary map.
func loadFailuresFromFile(path string, summaries map[string]*PackageFailureSummary) error {
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 0, 64*1024), 1<<20) // 1MB max line
	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}

		var record failureRecord
		if err := json.Unmarshal(line, &record); err != nil {
			continue // skip malformed lines
		}

		// Legacy batch format: failures array
		for _, f := range record.Failures {
			name := bareName(f.PackageID)
			addFailure(summaries, name, f.Category, f.BlockedBy)
		}

		// Per-recipe format
		if record.Recipe != "" && record.Category != "" {
			addFailure(summaries, record.Recipe, record.Category, record.BlockedBy)
		}
	}

	return scanner.Err()
}

// addFailure records a single failure for a package in the summary map.
func addFailure(summaries map[string]*PackageFailureSummary, name, category string, blockedBy []string) {
	s, ok := summaries[name]
	if !ok {
		s = &PackageFailureSummary{}
		summaries[name] = s
	}

	s.TotalFailures++

	if category == "missing_dep" {
		s.HasMissingDep = true
		for _, dep := range blockedBy {
			if !containsString(s.BlockedBy, dep) {
				s.BlockedBy = append(s.BlockedBy, dep)
			}
		}
	} else {
		s.HasOtherFailure = true
	}
}

// Run reads failure data and updates queue entry statuses. It performs two
// operations:
//
//  1. Mark new failures: pending entries with more failures in JSONL than
//     their failure_count get set to failed or blocked.
//  2. Expire backoffs: failed entries whose next_retry_at has passed get
//     flipped back to pending.
//
// It modifies the queue in place. The caller handles I/O.
func Run(queue *batch.UnifiedQueue, failuresDir string) (*Result, error) {
	failureMap, err := LoadFailureMap(failuresDir)
	if err != nil {
		return &Result{}, nil // no failure data is not an error
	}

	now := time.Now().UTC()
	result := &Result{}

	for i := range queue.Entries {
		entry := &queue.Entries[i]

		switch entry.Status {
		case batch.StatusPending:
			markPendingEntry(entry, failureMap, now, result)
		case batch.StatusFailed:
			expireBackoff(entry, now, result)
		}
	}

	return result, nil
}

// markPendingEntry checks a pending entry against failure data and marks
// it as failed or blocked if new failures exist.
func markPendingEntry(entry *batch.QueueEntry, failureMap map[string]*PackageFailureSummary, now time.Time, result *Result) {
	summary, ok := failureMap[entry.Name]
	if !ok {
		return // no failure data for this entry
	}

	// Only mark if there are new failures beyond what's already counted.
	// This prevents the idempotency cycle: once marked and retried,
	// the same failure data won't trigger re-marking.
	if summary.TotalFailures <= entry.FailureCount {
		return
	}

	entry.FailureCount = summary.TotalFailures

	if summary.HasMissingDep && len(summary.BlockedBy) > 0 {
		entry.Status = batch.StatusBlocked
		result.MarkedBlocked++
		result.Details = append(result.Details, Change{
			Name:      entry.Name,
			FromState: batch.StatusPending,
			ToState:   batch.StatusBlocked,
		})
	} else {
		entry.Status = batch.StatusFailed
		retryAt := computeRetryAt(now, entry.FailureCount)
		entry.NextRetryAt = &retryAt
		result.MarkedFailed++
		result.Details = append(result.Details, Change{
			Name:      entry.Name,
			FromState: batch.StatusPending,
			ToState:   batch.StatusFailed,
		})
	}
}

// expireBackoff flips a failed entry back to pending if its backoff has expired.
func expireBackoff(entry *batch.QueueEntry, now time.Time, result *Result) {
	if entry.NextRetryAt != nil && entry.NextRetryAt.After(now) {
		return // backoff still active
	}

	entry.Status = batch.StatusPending
	result.Retried++
	result.Details = append(result.Details, Change{
		Name:      entry.Name,
		FromState: batch.StatusFailed,
		ToState:   batch.StatusPending,
	})
}

// computeRetryAt returns the next retry time using exponential backoff.
// Formula: now + min(base * 2^(failures-1), maxBackoff)
func computeRetryAt(now time.Time, failureCount int) time.Time {
	exp := failureCount - 1
	if exp < 0 {
		exp = 0
	}
	delay := time.Duration(float64(backoffBase) * math.Pow(2, float64(exp)))
	if delay > maxBackoff {
		delay = maxBackoff
	}
	return now.Add(delay)
}

// bareName extracts the bare name from a fully-qualified package ID.
// For "homebrew:ffmpeg" it returns "ffmpeg". For "ffmpeg" it returns "ffmpeg".
func bareName(pkgID string) string {
	if idx := strings.Index(pkgID, ":"); idx >= 0 {
		return pkgID[idx+1:]
	}
	return pkgID
}

// containsString reports whether s is in the slice.
func containsString(slice []string, s string) bool {
	for _, v := range slice {
		if v == s {
			return true
		}
	}
	return false
}
