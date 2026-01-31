package batch

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// BatchResult holds the outcome of a batch generation run.
type BatchResult struct {
	BatchID   string    `json:"batch_id"`
	Ecosystem string    `json:"ecosystem"`
	Total     int       `json:"total"`
	Generated int       `json:"generated"`
	Failed    int       `json:"failed"`
	Timestamp time.Time `json:"timestamp"`

	// Recipes is the list of generated recipe file paths.
	Recipes []string `json:"-"`

	// Failures is the list of failure records for this run.
	Failures []FailureRecord `json:"-"`
}

// FailureRecord represents a single package generation failure.
type FailureRecord struct {
	PackageID string    `json:"package_id"`
	Category  string    `json:"category"`
	BlockedBy []string  `json:"blocked_by,omitempty"`
	Message   string    `json:"message"`
	Timestamp time.Time `json:"timestamp"`
}

// FailureFile is the top-level structure for failure JSONL entries, matching
// data/schemas/failure-record.schema.json.
type FailureFile struct {
	SchemaVersion int             `json:"schema_version"`
	Ecosystem     string          `json:"ecosystem"`
	Environment   string          `json:"environment"`
	UpdatedAt     string          `json:"updated_at"`
	Failures      []FailureRecord `json:"failures"`
}

// WriteFailures appends failure records to data/failures/<ecosystem>.jsonl.
// Each call appends one JSON line containing all failures from a single batch
// run for one environment.
func WriteFailures(dir, ecosystem string, failures []FailureRecord) error {
	if len(failures) == 0 {
		return nil
	}

	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("create failures dir: %w", err)
	}

	record := FailureFile{
		SchemaVersion: 1,
		Ecosystem:     ecosystem,
		Environment:   "linux-glibc-x86_64",
		UpdatedAt:     time.Now().UTC().Format(time.RFC3339),
		Failures:      failures,
	}

	data, err := json.Marshal(record)
	if err != nil {
		return fmt.Errorf("marshal failures: %w", err)
	}
	data = append(data, '\n')

	path := filepath.Join(dir, ecosystem+".jsonl")
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return fmt.Errorf("open failures file: %w", err)
	}
	defer f.Close()

	if _, err := f.Write(data); err != nil {
		return fmt.Errorf("write failures: %w", err)
	}

	return nil
}
