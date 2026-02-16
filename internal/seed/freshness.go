package seed

import (
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/tsukumogami/tsuku/internal/batch"
)

// FreshnessConfig controls freshness checking thresholds and behavior.
type FreshnessConfig struct {
	// ThresholdDays is the maximum age (in days) before a disambiguation
	// is considered stale. Entries with nil or older disambiguated_at are
	// flagged for re-disambiguation.
	ThresholdDays int

	// Now is the reference time for freshness calculations.
	// If zero, time.Now().UTC() is used.
	Now time.Time
}

// SourceChange records a detected source change from re-disambiguation.
type SourceChange struct {
	Package      string `json:"package"`
	Old          string `json:"old"`
	New          string `json:"new"`
	Priority     int    `json:"priority"`
	AutoAccepted bool   `json:"auto_accepted"`
}

// CuratedInvalid records a curated entry whose source failed validation.
type CuratedInvalid struct {
	Package string `json:"package"`
	Source  string `json:"source"`
	Error   string `json:"error"`
}

// FreshnessResult collects the outcomes of freshness checking across
// all queue entries.
type FreshnessResult struct {
	// StaleEntries are indices into the queue that need re-disambiguation.
	StaleEntries []int

	// SourceChanges collects detected source changes after re-disambiguation.
	SourceChanges []SourceChange

	// CuratedInvalid collects curated entries whose sources are broken.
	CuratedInvalid []CuratedInvalid

	// CuratedSkipped is the count of curated entries skipped from re-disambiguation.
	CuratedSkipped int

	// Refreshed is the count of entries that were actually re-disambiguated.
	Refreshed int
}

// freshnessThreshold returns the cutoff time: entries older than this
// are considered stale.
func freshnessThreshold(cfg FreshnessConfig) time.Time {
	now := cfg.Now
	if now.IsZero() {
		now = time.Now().UTC()
	}
	return now.AddDate(0, 0, -cfg.ThresholdDays)
}

// IsStale checks whether a queue entry should be re-disambiguated based
// on the staleness trigger. Returns true when:
//   - disambiguated_at is nil
//   - disambiguated_at is older than the freshness threshold
func IsStale(entry batch.QueueEntry, cfg FreshnessConfig) bool {
	if entry.DisambiguatedAt == nil {
		return true
	}
	return entry.DisambiguatedAt.Before(freshnessThreshold(cfg))
}

// IsFailuresStale checks the combined failures+stale trigger.
// Returns true when failure_count >= 3 AND the entry is stale.
func IsFailuresStale(entry batch.QueueEntry, cfg FreshnessConfig) bool {
	return entry.FailureCount >= 3 && IsStale(entry, cfg)
}

// IsNewAuditCandidate checks whether a discovered source is not yet present
// in the package's audit probe results. This indicates a new ecosystem was
// discovered that wasn't probed in the last disambiguation.
func IsNewAuditCandidate(entry batch.QueueEntry, auditEntry *AuditEntry, discoveredSource string) bool {
	if discoveredSource == "" {
		return false
	}
	return !HasSource(auditEntry, discoveredSource)
}

// ShouldSkip returns true if the entry should be excluded from freshness
// checking entirely. Entries with status "success" or confidence "curated"
// are skipped from re-disambiguation.
func ShouldSkip(entry batch.QueueEntry) bool {
	return entry.Status == batch.StatusSuccess
}

// IsCurated returns true if the entry has curated confidence and should
// be validated rather than re-disambiguated.
func IsCurated(entry batch.QueueEntry) bool {
	return entry.Confidence == batch.ConfidenceCurated
}

// NeedsRedisambiguation determines whether a queue entry should be
// re-disambiguated. It checks all three triggers and returns true if
// any trigger fires. The entry must not be skipped (success) or curated.
//
// The auditEntry and discoveredSource parameters are optional -- pass nil
// and "" respectively when trigger 3 (new audit candidate) is not applicable.
func NeedsRedisambiguation(entry batch.QueueEntry, cfg FreshnessConfig, auditEntry *AuditEntry, discoveredSource string) bool {
	if ShouldSkip(entry) {
		return false
	}
	if IsCurated(entry) {
		return false
	}

	// Trigger 1: staleness
	if IsStale(entry, cfg) {
		return true
	}

	// Trigger 2: failures + stale (already covered by trigger 1 if stale,
	// but this enables the caller to know which trigger fired)
	if IsFailuresStale(entry, cfg) {
		return true
	}

	// Trigger 3: new audit candidate
	if IsNewAuditCandidate(entry, auditEntry, discoveredSource) {
		return true
	}

	return false
}

// ApplySourceChange handles a source change detected during re-disambiguation.
// For priority 1-2 entries, the existing source is kept and the proposed change
// is recorded. For priority 3, the source is updated and failure state is reset.
//
// Returns the SourceChange record and a boolean indicating whether the queue
// entry was modified.
func ApplySourceChange(entry *batch.QueueEntry, newSource string, now time.Time) (SourceChange, bool) {
	change := SourceChange{
		Package:  entry.Name,
		Old:      entry.Source,
		New:      newSource,
		Priority: entry.Priority,
	}

	if entry.Priority <= 2 {
		// Priority 1-2: do NOT update the queue entry. Keep existing source.
		change.AutoAccepted = false
		return change, false
	}

	// Priority 3: auto-accept the source change.
	entry.Source = newSource
	entry.FailureCount = 0
	entry.NextRetryAt = nil
	change.AutoAccepted = true
	return change, true
}

// UpdateDisambiguatedAt sets the disambiguated_at timestamp on the entry
// after any re-disambiguation (regardless of whether the source changed).
func UpdateDisambiguatedAt(entry *batch.QueueEntry, now time.Time) {
	t := now
	entry.DisambiguatedAt = &t
}

// ApplySelectionResult updates a queue entry's status based on the
// disambiguation selection_reason. priority_fallback results get
// status "requires_manual"; other clear results get "pending".
func ApplySelectionResult(entry *batch.QueueEntry, selectionReason string) {
	if selectionReason == batch.SelectionPriorityFallback {
		entry.Status = batch.StatusRequiresManual
	} else {
		entry.Status = batch.StatusPending
	}
}

// CuratedSourceValidator checks whether curated sources still exist
// via HTTP HEAD requests.
type CuratedSourceValidator struct {
	// Client is the HTTP client used for HEAD requests.
	// If nil, http.DefaultClient is used.
	Client *http.Client
}

// ecosystemAPIURL returns the API URL for checking if a source exists.
// The source format is "ecosystem:name" (e.g., "homebrew:ripgrep").
func ecosystemAPIURL(source string) (string, error) {
	parts := strings.SplitN(source, ":", 2)
	if len(parts) != 2 {
		return "", fmt.Errorf("invalid source format: %q", source)
	}

	ecosystem, name := parts[0], parts[1]
	switch ecosystem {
	case "homebrew":
		return "https://formulae.brew.sh/api/formula/" + name + ".json", nil
	case "cargo", "crates.io":
		return "https://crates.io/api/v1/crates/" + name, nil
	case "npm":
		return "https://registry.npmjs.org/" + name, nil
	case "pypi":
		return "https://pypi.org/pypi/" + name + "/json", nil
	case "rubygems":
		return "https://rubygems.org/api/v1/gems/" + name + ".json", nil
	case "github":
		// GitHub source format: "github:owner/repo"
		return "https://api.github.com/repos/" + name, nil
	default:
		return "", fmt.Errorf("unknown ecosystem %q in source %q", ecosystem, source)
	}
}

// Validate performs an HTTP HEAD request to check if the curated source
// still exists. Returns nil if valid, or an error describing the problem.
func (v *CuratedSourceValidator) Validate(source string) error {
	apiURL, err := ecosystemAPIURL(source)
	if err != nil {
		return err
	}

	client := v.Client
	if client == nil {
		client = http.DefaultClient
	}

	return validateURL(client, apiURL)
}

// validateURL performs an HTTP HEAD request against the given URL
// and returns an error if the response indicates the resource is missing
// or the request fails.
func validateURL(client *http.Client, url string) error {
	resp, err := client.Head(url)
	if err != nil {
		return fmt.Errorf("connection error: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return fmt.Errorf("404 Not Found")
	}
	if resp.StatusCode >= 400 {
		return fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	return nil
}
