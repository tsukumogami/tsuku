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
	Succeeded int       `json:"succeeded"`
	Failed    int       `json:"failed"`
	Blocked   int       `json:"blocked"`
	Timestamp time.Time `json:"timestamp"`

	// Recipes is the list of generated recipe file paths.
	Recipes []string `json:"-"`

	// Failures is the list of failure records for this run.
	Failures []FailureRecord `json:"-"`
}

// Summary returns a markdown summary of the batch run for use in PR descriptions.
func (r *BatchResult) Summary() string {
	s := fmt.Sprintf("Batch run for **%s** on %s.\n\n", r.Ecosystem, r.Timestamp.Format("2006-01-02"))
	s += "| Metric | Count |\n|--------|-------|\n"
	s += fmt.Sprintf("| Succeeded | %d |\n", r.Succeeded)
	s += fmt.Sprintf("| Failed | %d |\n", r.Failed)
	s += fmt.Sprintf("| Blocked | %d |\n", r.Blocked)
	s += fmt.Sprintf("| **Total** | **%d** |\n", r.Total)

	if len(r.Recipes) > 0 {
		s += "\n### Recipes added\n\n"
		for _, p := range r.Recipes {
			s += fmt.Sprintf("- `%s`\n", filepath.Base(p))
		}
	}

	if len(r.Failures) > 0 {
		s += "\n### Failures\n\n"
		for _, f := range r.Failures {
			if f.Category == "missing_dep" && len(f.BlockedBy) > 0 {
				s += fmt.Sprintf("- **%s**: blocked by %v\n", f.PackageID, f.BlockedBy)
			} else {
				s += fmt.Sprintf("- **%s**: %s\n", f.PackageID, f.Category)
			}
		}
	}

	return s
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

// WriteFailures writes failure records to data/failures/<ecosystem>-<timestamp>.jsonl.
// Each call writes one JSON line containing all failures from a single batch
// run for one environment. Uses timestamped filenames to eliminate append-only
// conflicts when parallel workflows run simultaneously.
func WriteFailures(dir, ecosystem string, failures []FailureRecord) error {
	if len(failures) == 0 {
		return nil
	}

	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("create failures dir: %w", err)
	}

	timestamp := time.Now().UTC().Format("2006-01-02T15:04:05Z")
	record := FailureFile{
		SchemaVersion: 1,
		Ecosystem:     ecosystem,
		Environment:   "linux-glibc-x86_64",
		UpdatedAt:     timestamp,
		Failures:      failures,
	}

	data, err := json.Marshal(record)
	if err != nil {
		return fmt.Errorf("marshal failures: %w", err)
	}
	data = append(data, '\n')

	// Write to timestamped file instead of appending to shared file
	filename := fmt.Sprintf("%s-%s.jsonl", ecosystem, timestamp)
	path := filepath.Join(dir, filename)

	if err := os.WriteFile(path, data, 0644); err != nil {
		return fmt.Errorf("write failures: %w", err)
	}

	return nil
}
