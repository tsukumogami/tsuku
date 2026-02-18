package batch

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// QueueEntry represents a single entry in the unified priority queue.
// Each entry has a pre-resolved source field so the batch orchestrator
// can use it directly without runtime disambiguation.
type QueueEntry struct {
	// Name is the tool name (e.g., "ripgrep"). Used for display and deduplication.
	Name string `json:"name"`

	// Source is the pre-resolved source in ecosystem:identifier format
	// (e.g., "cargo:ripgrep", "github:sharkdp/bat").
	Source string `json:"source"`

	// Priority determines processing order: 1=critical, 2=popular, 3=standard.
	Priority int `json:"priority"`

	// Status tracks queue processing state. See Status* constants.
	Status string `json:"status"`

	// Confidence indicates how the source was selected.
	// "auto" means the disambiguation algorithm chose it;
	// "curated" means an expert specified it manually.
	Confidence string `json:"confidence"`

	// DisambiguatedAt records when disambiguation was last run for this entry.
	// Used for freshness checking (stale after 30 days).
	DisambiguatedAt *time.Time `json:"disambiguated_at"`

	// FailureCount tracks consecutive failures for exponential backoff.
	FailureCount int `json:"failure_count"`

	// NextRetryAt is the earliest time this entry can be retried.
	// Nil means no backoff is active and the entry is eligible immediately.
	NextRetryAt *time.Time `json:"next_retry_at"`
}

// Queue entry status values.
const (
	StatusPending        = "pending"         // Ready for batch processing
	StatusSuccess        = "success"         // Recipe generated and merged
	StatusFailed         = "failed"          // Generation failed (will retry with backoff)
	StatusBlocked        = "blocked"         // Blocked by missing dependency
	StatusRequiresManual = "requires_manual" // Needs human intervention
	StatusExcluded       = "excluded"        // Permanently excluded from processing
)

// validStatuses is the set of allowed status values.
var validStatuses = map[string]bool{
	StatusPending:        true,
	StatusSuccess:        true,
	StatusFailed:         true,
	StatusBlocked:        true,
	StatusRequiresManual: true,
	StatusExcluded:       true,
}

// Queue entry confidence values.
const (
	ConfidenceAuto    = "auto"    // Disambiguation algorithm selected the source
	ConfidenceCurated = "curated" // Expert manually specified the source
)

// validConfidences is the set of allowed confidence values.
var validConfidences = map[string]bool{
	ConfidenceAuto:    true,
	ConfidenceCurated: true,
}

// Validate checks that the QueueEntry fields satisfy required constraints.
// It returns an error describing all violations found, or nil if the entry
// is valid.
func (e *QueueEntry) Validate() error {
	var errs []string

	if strings.TrimSpace(e.Name) == "" {
		errs = append(errs, "name must not be empty")
	}

	if strings.TrimSpace(e.Source) == "" {
		errs = append(errs, "source must not be empty")
	} else {
		// Validate that the ecosystem prefix doesn't contain path traversal characters.
		eco := e.Ecosystem()
		if strings.ContainsAny(eco, "/\\") || strings.Contains(eco, "..") {
			errs = append(errs, fmt.Sprintf("ecosystem prefix %q contains path traversal characters", eco))
		}
	}

	if e.Priority < 1 || e.Priority > 3 {
		errs = append(errs, fmt.Sprintf("priority must be 1, 2, or 3, got %d", e.Priority))
	}

	if !validStatuses[e.Status] {
		errs = append(errs, fmt.Sprintf("invalid status %q", e.Status))
	}

	if !validConfidences[e.Confidence] {
		errs = append(errs, fmt.Sprintf("invalid confidence %q", e.Confidence))
	}

	if e.FailureCount < 0 {
		errs = append(errs, fmt.Sprintf("failure_count must not be negative, got %d", e.FailureCount))
	}

	if len(errs) > 0 {
		return fmt.Errorf("invalid queue entry: %s", strings.Join(errs, "; "))
	}
	return nil
}

// Ecosystem extracts the ecosystem prefix from the Source field.
// For example, "cargo:ripgrep" returns "cargo".
func (e *QueueEntry) Ecosystem() string {
	if idx := strings.Index(e.Source, ":"); idx >= 0 {
		return e.Source[:idx]
	}
	return ""
}

// LoadUnifiedQueue reads a unified queue from disk. Returns an empty queue
// if the file doesn't exist.
func LoadUnifiedQueue(path string) (*UnifiedQueue, error) {
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return &UnifiedQueue{
			SchemaVersion: 1,
			Entries:       []QueueEntry{},
		}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read queue: %w", err)
	}
	var q UnifiedQueue
	if err := json.Unmarshal(data, &q); err != nil {
		return nil, fmt.Errorf("parse queue: %w", err)
	}
	return &q, nil
}

// SaveUnifiedQueue writes the unified queue to disk as formatted JSON.
func SaveUnifiedQueue(path string, queue *UnifiedQueue) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("create directory: %w", err)
	}

	queue.UpdatedAt = time.Now().UTC().Format(time.RFC3339)
	data, err := json.MarshalIndent(queue, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal queue: %w", err)
	}
	data = append(data, '\n')
	return os.WriteFile(path, data, 0644)
}
